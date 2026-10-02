package comparecmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type identityEvaluatorFunc func(context.Context, jevIdentityRequest) (jevIdentityResponse, error)

func (evaluate identityEvaluatorFunc) evaluate(ctx context.Context, request jevIdentityRequest) (jevIdentityResponse, error) {
	return evaluate(ctx, request)
}

func identityTestNumber(value float64) *float64 { return &value }

func identityTestResponse(request jevIdentityRequest, choice string, probability, same float64) jevIdentityResponse {
	answers := map[string]jevIdentityAnswer{}
	for key, question := range request.Questions {
		if question.Type == "choice" {
			probabilities := map[string]*float64{}
			for option := range question.Criteria {
				value := (1 - probability) / float64(len(question.Criteria)-1)
				if option == choice {
					value = probability
				}
				probabilities[option] = identityTestNumber(value)
			}
			answers[key] = jevIdentityAnswer{Type: "choice", Choice: choice, Probabilities: probabilities, Confidence: identityTestNumber(0.99)}
		} else {
			value := 0.01
			if key == "same_"+choice {
				value = same
			}
			answers[key] = jevIdentityAnswer{Type: "noul", Noul: identityTestNumber(value)}
		}
	}
	return jevIdentityResponse{Model: defaultIdentityModel, Answers: answers, Usage: &jevIdentityUsage{InputTokens: 100, OutputTokens: 10}}
}

func identityTestComparison() compareReport {
	root := compareSnapshotNode{ID: 1, OriginalIndex: 0, Ref: "@e1", Fingerprint: "main-orders", Role: "main", Name: "Orders", Visible: true, StructurePath: "html:1>body:1>main:1"}
	old := compareSnapshot{URL: "https://old.example.com/?private=DO_NOT_SEND_PAGE_URL", Title: "DO_NOT_SEND_TITLE", Text: "DO_NOT_SEND_PAGE_TEXT", Nodes: []compareSnapshotNode{
		root,
		{ID: 2, OriginalIndex: 1, Ref: "@e2", Fingerprint: "old-save", Role: "button", Name: "Save", Label: "Order 123", Value: "DO_NOT_SEND_VALUE", CSS: map[string]string{"color": "DO_NOT_SEND_CSS"}, Visible: true, Enabled: true, Invokable: true, StructurePath: root.StructurePath + ">form:1>button:1", Href: "https://user:DO_NOT_SEND_USERINFO@example.com/orders/123?token=DO_NOT_SEND_QUERY#DO_NOT_SEND_FRAGMENT"},
	}}
	next := compareSnapshot{URL: "https://new.example.com/", Nodes: []compareSnapshotNode{
		root,
		{ID: 2, OriginalIndex: 1, Ref: "@e2", Fingerprint: "new-submit", Role: "button", Name: "Submit", Label: "Order 123", Visible: true, Enabled: true, Invokable: true, StructurePath: root.StructurePath + ">form:1>button:1"},
		{ID: 3, OriginalIndex: 2, Ref: "@e3", Fingerprint: "new-cancel", Role: "button", Name: "Cancel", Label: "Order 456", Visible: true, Enabled: true, Invokable: true, StructurePath: root.StructurePath + ">form:2>button:1"},
	}}
	return buildCompareReportWithDecisionMatches(old, next, nil, compareMatchModeExact, true, nil)
}

func identityTestArguments(t *testing.T, report compareReport) identitySuggestionArguments {
	t.Helper()
	directory := t.TempDir()
	path := filepath.Join(directory, "compare.json")
	if err := writeCompareJSON(path, report); err != nil {
		t.Fatal(err)
	}
	return identitySuggestionArguments{
		CompareJSON: path, Output: filepath.Join(directory, "pairs.jsonl"), Model: defaultIdentityModel,
		MinProbability: 0.95, MinMargin: 0.2, Limit: 20, MaxCandidates: 5, Timeout: 30000, JSON: true,
	}
}

func TestIdentityRequestsUseBoundedSemanticContext(t *testing.T) {
	comparison := identityTestComparison()
	args := identityTestArguments(t, comparison)
	tasks, err := buildIdentityTasks(comparison, args)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks: %+v, %v", tasks, err)
	}
	request := buildJevIdentityRequest(tasks[0], args.Model)
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "DO_NOT_SEND") || strings.Contains(string(data), "@e") || strings.Contains(string(data), "old-save") {
		t.Fatalf("request included fields outside the semantic allowlist: %s", data)
	}
	if request.State.Old.Node.HrefPath != "/orders/123" || len(request.State.Old.Ancestors) != 1 || request.State.Old.Ancestors[0].Name != "Orders" {
		t.Fatalf("missing local context: %+v", request.State.Old)
	}
	if len(request.State.Candidates) != 2 || len(request.Questions) != 3 || len(request.Questions["correspondence"].Criteria) != 4 {
		t.Fatalf("expected candidates, independent verification, none and unknown: %+v", request)
	}
	long := strings.Repeat("注文", identityFeatureLength)
	features := buildIdentityFeatures(compareSnapshotNode{Name: long})
	if len([]rune(features.Name)) != identityFeatureLength+3 || !strings.HasSuffix(features.Name, "...") {
		t.Fatalf("feature was not bounded on Unicode characters: %q", features.Name)
	}
	for _, href := range []string{"javascript:alert(1)", "mailto:person@example.com"} {
		if identityHrefPath(href) != "" {
			t.Fatalf("unexpected URL detail sent for %q", href)
		}
	}
}

func TestSuggestDecisionsDryRunNeedsNoKeyOrDaemon(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "")
	args := identityTestArguments(t, identityTestComparison())
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"suggest-decisions", "--compare-json", args.CompareJSON, "--output", args.Output, "--dry-run"}, &stdout, &stderr, nil)
	if code != 0 {
		t.Fatalf("dry-run failed: %s", stderr.String())
	}
	var report identitySuggestionReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if !report.DryRun || len(report.Requests) != 1 || report.Summary.High != 0 || report.Output != "" {
		t.Fatalf("unexpected dry-run report: %+v", report)
	}
	for _, path := range []string{args.Output, args.Output + ".review.json"} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("dry-run created output %q", path)
		}
	}
}

func TestSuggestDecisionsPromotesOnlyOnExplicitRequestAndRetainsDiffs(t *testing.T) {
	for _, promote := range []bool{false, true} {
		t.Run(map[bool]string{false: "review", true: "promote"}[promote], func(t *testing.T) {
			comparison := identityTestComparison()
			args := identityTestArguments(t, comparison)
			args.Promote = promote
			evaluator := identityEvaluatorFunc(func(_ context.Context, request jevIdentityRequest) (jevIdentityResponse, error) {
				return identityTestResponse(request, "candidate_1", 0.99, 0.99), nil
			})
			var stdout, stderr bytes.Buffer
			if code := runIdentitySuggestions(context.Background(), args, &stdout, &stderr, evaluator); code != 0 {
				t.Fatal(stderr.String())
			}
			decisions, err := loadCompareDecisions(args.Output)
			if err != nil || len(decisions) != 1 {
				t.Fatalf("decisions: %+v, %v", decisions, err)
			}
			expected := "tentative"
			if promote {
				expected = "high"
			}
			decision := decisions[0]
			if decision.Confidence != expected || decision.OldFingerprint != "old-save" || decision.NewFingerprint != "new-submit" || decision.DecidedBy != "jev:"+defaultIdentityModel {
				t.Fatalf("unexpected pinned decision: %+v", decision)
			}
			var report identitySuggestionReport
			if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			input, _ := os.ReadFile(args.CompareJSON)
			if report.CompareSHA256 != fmt.Sprintf("%x", sha256.Sum256(input)) || len(report.Judgments) != 1 || report.Judgments[0].RequestSHA256 == "" || report.Judgments[0].Response.Model != defaultIdentityModel || len(report.Requests) != 0 {
				t.Fatalf("missing provenance or unexpected state: %+v", report)
			}
			for _, path := range []string{args.Output, args.Output + ".review.json"} {
				info, err := os.Stat(path)
				if err != nil || info.Mode().Perm() != 0o600 {
					t.Fatalf("output permissions: %v, %v", info, err)
				}
			}
			matches, err := compareResolveDecisionMatches(decisions, comparison.Old.Nodes, comparison.New.Nodes)
			if err != nil {
				t.Fatal(err)
			}
			if !promote && len(matches) != 0 {
				t.Fatal("tentative suggestion became an applied match")
			}
			if promote {
				result := buildCompareReportWithDecisionMatches(comparison.Old, comparison.New, nil, compareMatchModeExact, true, matches)
				if len(matches) != 1 || result.Summary.MissingNodes != 0 || result.Summary.TextChanged == 0 || result.Summary.Same {
					t.Fatalf("pair suggestion hid the changed text: %+v", result)
				}
				comparison.New.Nodes[1].Fingerprint = "changed-after-review"
				if _, err := compareResolveDecisionMatches(decisions, comparison.Old.Nodes, comparison.New.Nodes); err == nil {
					t.Fatal("stale fingerprint was accepted")
				}
			}
		})
	}
}

func TestIdentityPromotionUsesAbsoluteAndRelativeProbabilities(t *testing.T) {
	for _, test := range []struct {
		name, choice              string
		probability, same, margin float64
		confidence                string
	}{
		{"low choice", "candidate_1", 0.6, 0.99, 0.2, "tentative"},
		{"low same element", "candidate_1", 0.99, 0.1, 0.2, "tentative"},
		{"insufficient margin", "candidate_1", 0.99, 0.99, 1, "tentative"},
		{"no correspondence", identityNoMatch, 0.99, 0.99, 0.2, "unknown"},
		{"insufficient evidence", identityUnknown, 0.99, 0.99, 0.2, "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := identityTestArguments(t, identityTestComparison())
			args.Promote, args.MinMargin = true, test.margin
			var stdout, stderr bytes.Buffer
			evaluate := identityEvaluatorFunc(func(_ context.Context, request jevIdentityRequest) (jevIdentityResponse, error) {
				return identityTestResponse(request, test.choice, test.probability, test.same), nil
			})
			if code := runIdentitySuggestions(context.Background(), args, &stdout, &stderr, evaluate); code != 0 {
				t.Fatal(stderr.String())
			}
			decisions, err := loadCompareDecisions(args.Output)
			if err != nil || len(decisions) != 1 || decisions[0].Confidence != test.confidence || decisions[0].Kind != "pair" {
				t.Fatalf("unexpected decision: %+v, %v", decisions, err)
			}
		})
	}
}

func TestIdentityConflictingProposalsStayUnknown(t *testing.T) {
	comparison := identityTestComparison()
	duplicate := comparison.Old.Nodes[1]
	duplicate.ID, duplicate.OriginalIndex, duplicate.Ref, duplicate.Fingerprint = 3, 2, "@e3", "old-other-save"
	comparison.Old.Nodes = append(comparison.Old.Nodes, duplicate)
	comparison = buildCompareReportWithDecisionMatches(comparison.Old, comparison.New, nil, compareMatchModeExact, true, nil)
	args := identityTestArguments(t, comparison)
	args.Promote = true
	var stdout, stderr bytes.Buffer
	evaluate := identityEvaluatorFunc(func(_ context.Context, request jevIdentityRequest) (jevIdentityResponse, error) {
		return identityTestResponse(request, "candidate_1", 0.99, 0.99), nil
	})
	if code := runIdentitySuggestions(context.Background(), args, &stdout, &stderr, evaluate); code != 0 {
		t.Fatal(stderr.String())
	}
	decisions, err := loadCompareDecisions(args.Output)
	if err != nil || len(decisions) != 2 {
		t.Fatalf("decisions: %+v, %v", decisions, err)
	}
	for _, decision := range decisions {
		if decision.Confidence != "unknown" || decision.New != "?" || decision.NewFingerprint != "" {
			t.Fatalf("conflicting proposal survived: %+v", decision)
		}
	}
}

func TestIdentitySuggestionsFailWithoutReplacingOutputs(t *testing.T) {
	for _, test := range []string{"existing decisions", "existing review", "same output", "stale debug", "duplicate refs", "matched reuse", "provider failure", "invalid response", "cancellation"} {
		t.Run(test, func(t *testing.T) {
			comparison := identityTestComparison()
			args := identityTestArguments(t, comparison)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			evaluate := identityEvaluatorFunc(func(_ context.Context, request jevIdentityRequest) (jevIdentityResponse, error) {
				calls++
				if test == "provider failure" {
					return jevIdentityResponse{}, errors.New("provider unavailable")
				}
				response := identityTestResponse(request, "candidate_1", 0.99, 0.99)
				if test == "invalid response" {
					response.Answers = nil
				}
				if test == "cancellation" {
					cancel()
				}
				return response, nil
			})
			preserved := ""
			switch test {
			case "existing decisions":
				preserved = args.Output
			case "existing review":
				preserved = args.Output + ".review.json"
			case "same output":
				args.OutputJSON = args.Output
			case "stale debug":
				comparison.MatchingDebug.UnmatchedOld[0].Fingerprint = "stale"
			case "duplicate refs":
				comparison.New.Nodes[2].Ref = comparison.New.Nodes[1].Ref
			case "matched reuse":
				comparison.MatchingDebug.Matches = append(comparison.MatchingDebug.Matches, compareMatchingDebugMatch{Old: comparison.MatchingDebug.UnmatchedOld[0]})
			}
			if preserved != "" {
				if err := os.WriteFile(preserved, []byte("user content"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := writeCompareJSON(args.CompareJSON, comparison); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			if code := runIdentitySuggestions(ctx, args, &stdout, &stderr, evaluate); code != 1 {
				t.Fatalf("expected failure, got %d: %s", code, stdout.String())
			}
			for _, path := range []string{args.Output, args.Output + ".review.json"} {
				if path == preserved {
					data, err := os.ReadFile(path)
					if err != nil || string(data) != "user content" {
						t.Fatalf("existing output was modified: %s, %v", data, err)
					}
				} else if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("failed run left an output: %q", path)
				}
			}
			if test != "provider failure" && test != "invalid response" && test != "cancellation" && calls != 0 {
				t.Fatal("invalid input reached remote evaluation")
			}
		})
	}
}

func TestIdentityOutputWriteFailureRemovesOnlyNewFiles(t *testing.T) {
	directory := t.TempDir()
	first, second := filepath.Join(directory, "pairs.jsonl"), filepath.Join(directory, "existing.json")
	if err := os.WriteFile(second, []byte("user content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeIdentityOutputFiles([]identityOutputFile{{Path: first, Data: []byte("new")}, {Path: second, Data: []byte("replace")}}); err == nil {
		t.Fatal("existing output was accepted")
	}
	if _, err := os.Stat(first); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed write left a partial decision file")
	}
	data, _ := os.ReadFile(second)
	if string(data) != "user content" {
		t.Fatal("existing output was modified")
	}
}

func TestNagiIdentitySuggestionValidation(t *testing.T) {
	base := []string{"compare", "suggest-decisions", "--compare-json", "compare.json", "--output", "pairs.jsonl"}
	for _, options := range [][]string{
		{"--min-probability", "NaN"}, {"--min-probability", "Inf"}, {"--min-probability", "0.5"}, {"--min-probability", "1.1"},
		{"--min-margin", "NaN"}, {"--min-margin", "-0.1"}, {"--min-margin", "1.1"},
		{"--limit", "0"}, {"--limit", "201"}, {"--max-candidates", "0"}, {"--max-candidates", "21"},
		{"--timeout", "0"}, {"--timeout", "300001"}, {"--model", ""}, {"--model", "jev-"}, {"--model", "other-model"},
	} {
		if _, err := newNagiCompareRoot().Parse(append(append([]string{}, base...), options...)); err == nil {
			t.Fatalf("invalid options accepted: %q", options)
		}
	}
	for _, args := range [][]string{
		{"compare", "suggest-decisions"},
		{"compare", "suggest-decisions", "--compare-json", "compare.json"},
		{"compare", "suggest-decisions", "--dry-run"},
	} {
		if _, err := newNagiCompareRoot().Parse(args); err == nil {
			t.Fatalf("missing required options accepted: %q", args)
		}
	}
	parsed, err := newNagiCompareRoot().Parse(append(base, "--model", "jev-latest", "--min-probability", "0.98", "--min-margin", "0.4", "--limit", "7", "--max-candidates", "3", "--timeout", "1234", "--promote"))
	if err != nil {
		t.Fatal(err)
	}
	args := identitySuggestionArgumentsFromInvocation(parsed.Invocation())
	if args.Model != "jev-latest" || args.MinProbability != 0.98 || args.MinMargin != 0.4 || args.Limit != 7 || args.MaxCandidates != 3 || args.Timeout != 1234 || !args.Promote {
		t.Fatalf("CLI options did not reach the suggestion configuration: %+v", args)
	}
}
