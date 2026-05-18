package comparecmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mayahiro/nexus/internal/api"
)

func TestCompareExactModePreservesFingerprintBehavior(t *testing.T) {
	report := buildCompareReport(
		compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "button|Save", Role: "button", Label: "Save", Name: "Save", Visible: true, Enabled: true, Invokable: true},
			},
		},
		compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "button|Submit", Role: "button", Label: "Submit", Name: "Submit", Visible: true, Enabled: true, Invokable: true},
			},
		},
		nil,
		compareMatchModeExact,
	)

	if report.Summary.MissingNodes != 1 || report.Summary.NewNodes != 1 {
		t.Fatalf("expected missing and new node findings, got %+v", report.Summary)
	}
	if report.Summary.TextChanged != 0 {
		t.Fatalf("exact mode should not pair renamed nodes: %+v", report.Findings)
	}
}

func TestCompareStableModeMatchesSameTestID(t *testing.T) {
	report := buildCompareReport(
		compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "button|Save", Role: "button", Label: "Save", Name: "Save", TestID: "primary-action", Visible: true, Enabled: true, Invokable: true},
			},
		},
		compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "button|Submit", Role: "button", Label: "Submit", Name: "Submit", TestID: "primary-action", Visible: true, Enabled: true, Invokable: true},
			},
		},
		nil,
		compareMatchModeStable,
	)

	if report.Summary.MissingNodes != 0 || report.Summary.NewNodes != 0 {
		t.Fatalf("stable mode should match by testid: %+v", report.Summary)
	}
	if report.Summary.TextChanged != 1 || len(report.Findings) != 1 {
		t.Fatalf("expected one text_changed finding: %+v", report)
	}
	finding := report.Findings[0]
	if finding.Kind != "text_changed" || finding.Field != "name" {
		t.Fatalf("unexpected stable finding: %+v", finding)
	}
	if finding.MatchedBy != "stable:testid" || !slices.Contains(finding.MatchReasons, "testid") {
		t.Fatalf("expected stable testid metadata: %+v", finding)
	}
	if report.Summary.StableMatches != 1 || report.Summary.ExactMatches != 0 || report.Summary.HeuristicMatches != 0 {
		t.Fatalf("expected stable match summary: %+v", report.Summary)
	}
}

func TestCompareStableModeDoesNotMatchAmbiguousRepeatedKeys(t *testing.T) {
	oldNodes := []compareSnapshotNode{
		{Fingerprint: "old-a", Role: "button", Label: "Save", Name: "Save", Visible: true, Enabled: true, Invokable: true},
		{Fingerprint: "old-b", Role: "button", Label: "Save", Name: "Save", Visible: true, Enabled: true, Invokable: true},
	}
	newNodes := []compareSnapshotNode{
		{Fingerprint: "new-a", Role: "button", Label: "Save", Name: "Save", Visible: true, Enabled: true, Invokable: true},
		{Fingerprint: "new-b", Role: "button", Label: "Save", Name: "Save", Visible: true, Enabled: true, Invokable: true},
	}

	result := compareMatchNodes(oldNodes, newNodes, compareMatchModeStable)

	if len(result.Matches) != 0 {
		t.Fatalf("stable mode should not match ambiguous role-name keys: %+v", result.Matches)
	}
	if len(result.UnmatchedOld) != 2 || len(result.UnmatchedNew) != 2 {
		t.Fatalf("expected all nodes to remain unmatched: %+v", result)
	}
	if result.AmbiguousSkipped == 0 {
		t.Fatalf("expected ambiguous stable keys to be counted: %+v", result)
	}
}

func TestCompareHeuristicModeMatchesHighConfidenceNode(t *testing.T) {
	report := buildCompareReport(
		compareSnapshot{
			Nodes: []compareSnapshotNode{
				{
					Fingerprint:   "button|Save changes",
					Role:          "button",
					Label:         "Save changes",
					Name:          "Save changes",
					Visible:       true,
					Enabled:       true,
					Invokable:     true,
					OriginalIndex: 0,
					Tag:           "button",
					MatchBounds:   &api.Rect{X: 100, Y: 100, W: 120, H: 40},
				},
			},
		},
		compareSnapshot{
			Nodes: []compareSnapshotNode{
				{
					Fingerprint:   "button|Save",
					Role:          "button",
					Label:         "Save",
					Name:          "Save",
					Visible:       true,
					Enabled:       true,
					Invokable:     true,
					OriginalIndex: 0,
					Tag:           "button",
					MatchBounds:   &api.Rect{X: 105, Y: 104, W: 120, H: 40},
				},
			},
		},
		nil,
		compareMatchModeHeuristic,
	)

	if report.Summary.MissingNodes != 0 || report.Summary.NewNodes != 0 || report.Summary.TextChanged != 1 {
		t.Fatalf("heuristic mode should rescue the renamed button: %+v", report.Summary)
	}
	finding := report.Findings[0]
	if finding.MatchedBy != "heuristic" || finding.MatchScore < compareHeuristicMinimumScore {
		t.Fatalf("expected heuristic metadata: %+v", finding)
	}
	if !slices.Contains(finding.MatchReasons, "similar-name") {
		t.Fatalf("expected similar-name reason: %+v", finding)
	}
	if report.Summary.HeuristicMatches != 1 || report.Summary.StableMatches != 0 {
		t.Fatalf("expected heuristic match summary: %+v", report.Summary)
	}
}

func TestCompareHeuristicModeAvoidsCrossRoleMatch(t *testing.T) {
	report := buildCompareReport(
		compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "button|Submit", Role: "button", Label: "Submit", Name: "Submit", Visible: true, Enabled: true, Invokable: true, OriginalIndex: 0},
			},
		},
		compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "link|Submit", Role: "link", Label: "Submit", Text: "Submit", Visible: true, Enabled: true, Invokable: true, OriginalIndex: 0},
			},
		},
		nil,
		compareMatchModeHeuristic,
	)

	if report.Summary.MissingNodes != 1 || report.Summary.NewNodes != 1 || report.Summary.TextChanged != 0 {
		t.Fatalf("heuristic mode should not match cross-role nodes: %+v", report.Summary)
	}
}

func TestCompareHistogramModeMatchesWithinAnchoredRegions(t *testing.T) {
	report := buildCompareReport(
		compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "heading|Billing", Role: "heading", Label: "Billing", Name: "Billing", Text: "Billing", Visible: true, Enabled: true, OriginalIndex: 0, Tag: "h2"},
				{Fingerprint: "button|Save", Role: "button", Label: "Save", Name: "Save", Visible: true, Enabled: true, Invokable: true, OriginalIndex: 1, Tag: "button"},
				{Fingerprint: "heading|Profile", Role: "heading", Label: "Profile", Name: "Profile", Text: "Profile", Visible: true, Enabled: true, OriginalIndex: 2, Tag: "h2"},
				{Fingerprint: "button|Save", Role: "button", Label: "Save", Name: "Save", Visible: true, Enabled: true, Invokable: true, OriginalIndex: 3, Tag: "button"},
			},
		},
		compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "heading|Billing", Role: "heading", Label: "Billing", Name: "Billing", Text: "Billing", Visible: true, Enabled: true, OriginalIndex: 0, Tag: "h2"},
				{Fingerprint: "button|Save changes", Role: "button", Label: "Save changes", Name: "Save changes", Visible: true, Enabled: true, Invokable: true, OriginalIndex: 1, Tag: "button"},
				{Fingerprint: "heading|Profile", Role: "heading", Label: "Profile", Name: "Profile", Text: "Profile", Visible: true, Enabled: true, OriginalIndex: 2, Tag: "h2"},
				{Fingerprint: "button|Save", Role: "button", Label: "Save", Name: "Save", Visible: true, Enabled: true, Invokable: true, OriginalIndex: 3, Tag: "button"},
			},
		},
		nil,
		compareMatchModeHistogram,
	)

	if report.Summary.MissingNodes != 0 || report.Summary.NewNodes != 0 || report.Summary.TextChanged != 1 {
		t.Fatalf("histogram mode should match the renamed node inside anchors: %+v", report.Summary)
	}
	if report.Summary.HistogramMatches == 0 || report.Summary.HeuristicMatches != 0 {
		t.Fatalf("expected histogram match summary: %+v", report.Summary)
	}
	finding := report.Findings[0]
	if finding.MatchedBy != "histogram:heuristic" || finding.MatchScore < compareHeuristicMinimumScore {
		t.Fatalf("expected histogram heuristic metadata: %+v", finding)
	}
	if !slices.Contains(finding.MatchReasons, "anchor-region") {
		t.Fatalf("expected anchor-region reason: %+v", finding)
	}
}

func TestCompareMatchingDebugIncludesHistogramDetails(t *testing.T) {
	report := buildCompareReportWithDebug(
		compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "heading|Billing", Role: "heading", Label: "Billing", Name: "Billing", Text: "Billing", Visible: true, Enabled: true, OriginalIndex: 0, Tag: "h2"},
				{Fingerprint: "button|Save", Role: "button", Label: "Save", Name: "Save", Visible: true, Enabled: true, Invokable: true, OriginalIndex: 1, Tag: "button"},
				{Fingerprint: "heading|Profile", Role: "heading", Label: "Profile", Name: "Profile", Text: "Profile", Visible: true, Enabled: true, OriginalIndex: 2, Tag: "h2"},
			},
		},
		compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "heading|Billing", Role: "heading", Label: "Billing", Name: "Billing", Text: "Billing", Visible: true, Enabled: true, OriginalIndex: 0, Tag: "h2"},
				{Fingerprint: "button|Save changes", Role: "button", Label: "Save changes", Name: "Save changes", Visible: true, Enabled: true, Invokable: true, OriginalIndex: 1, Tag: "button"},
				{Fingerprint: "heading|Profile", Role: "heading", Label: "Profile", Name: "Profile", Text: "Profile", Visible: true, Enabled: true, OriginalIndex: 2, Tag: "h2"},
			},
		},
		nil,
		compareMatchModeHistogram,
		true,
	)

	debug := report.MatchingDebug
	if debug == nil {
		t.Fatal("expected matching debug")
	}
	if debug.Mode != compareMatchModeHistogram || debug.OldNodes != 3 || debug.NewNodes != 3 || debug.MatchedNodes != 3 {
		t.Fatalf("unexpected matching debug summary: %+v", debug)
	}
	hasHeuristicMatch := false
	for _, match := range debug.Matches {
		if match.MatchedBy == "histogram:heuristic" {
			hasHeuristicMatch = true
		}
	}
	if len(debug.Matches) != 3 || !hasHeuristicMatch {
		t.Fatalf("expected matching debug pairs: %+v", debug.Matches)
	}
	if len(debug.Anchors) != 2 {
		t.Fatalf("expected heading anchors: %+v", debug.Anchors)
	}
	if debug.Anchors[0].KeyKind != "role-name" || debug.Anchors[0].Old.Label != "Billing" {
		t.Fatalf("unexpected first anchor: %+v", debug.Anchors[0])
	}
	if len(debug.Regions) != 1 || debug.Regions[0].HeuristicMatches != 1 {
		t.Fatalf("expected one heuristic region: %+v", debug.Regions)
	}
	if len(debug.UnmatchedOld) != 0 || len(debug.UnmatchedNew) != 0 {
		t.Fatalf("expected no unmatched nodes: %+v", debug)
	}
}

func TestCompareMatchingDebugIncludesAmbiguousCandidates(t *testing.T) {
	report := buildCompareReportWithDebug(
		compareSnapshot{
			Nodes: []compareSnapshotNode{
				{
					Fingerprint:   "button|Save",
					Role:          "button",
					Label:         "Save",
					Name:          "Save",
					Visible:       true,
					Enabled:       true,
					Invokable:     true,
					OriginalIndex: 0,
					Tag:           "button",
					MatchBounds:   &api.Rect{X: 100, Y: 100, W: 120, H: 40},
				},
			},
		},
		compareSnapshot{
			Nodes: []compareSnapshotNode{
				{
					Fingerprint:   "button|Save changes",
					Ref:           "@e2",
					Role:          "button",
					Label:         "Save changes",
					Name:          "Save changes",
					Visible:       true,
					Enabled:       true,
					Invokable:     true,
					OriginalIndex: 0,
					Tag:           "button",
					MatchBounds:   &api.Rect{X: 100, Y: 100, W: 120, H: 40},
				},
				{
					Fingerprint:   "button|Save updates",
					Ref:           "@e3",
					Role:          "button",
					Label:         "Save updates",
					Name:          "Save updates",
					Visible:       true,
					Enabled:       true,
					Invokable:     true,
					OriginalIndex: 1,
					Tag:           "button",
					MatchBounds:   &api.Rect{X: 102, Y: 100, W: 120, H: 40},
				},
			},
		},
		nil,
		compareMatchModeHeuristic,
		true,
	)

	debug := report.MatchingDebug
	if debug == nil || len(debug.AmbiguousCandidates) != 1 {
		t.Fatalf("expected one ambiguous candidate entry: %+v", debug)
	}
	candidate := debug.AmbiguousCandidates[0]
	if candidate.ReasonSkipped == "" || len(candidate.NewCandidates) != 2 {
		t.Fatalf("expected candidate details: %+v", candidate)
	}
	if candidate.NewCandidates[0].Score == 0 || !slices.Contains(candidate.NewCandidates[0].SharedKeys, "bbox-near") {
		t.Fatalf("expected scored candidate evidence: %+v", candidate.NewCandidates)
	}
}

func TestCompareHighConfidenceDecisionPairsNodes(t *testing.T) {
	oldSnapshot := compareSnapshot{
		Nodes: []compareSnapshotNode{
			{Fingerprint: "button|Save", Ref: "@e1", Role: "button", Label: "Save", Name: "Save", Visible: true, Enabled: true, Invokable: true},
		},
	}
	newSnapshot := compareSnapshot{
		Nodes: []compareSnapshotNode{
			{Fingerprint: "button|Submit", Ref: "@e2", Role: "button", Label: "Submit", Name: "Submit", Visible: true, Enabled: true, Invokable: true},
		},
	}
	decisionMatches, err := compareResolvePairDecisionMatches(
		[]compareDecision{{Kind: "pair", Old: "@e1", New: "@e2", Confidence: "high"}},
		oldSnapshot.Nodes,
		newSnapshot.Nodes,
	)
	if err != nil {
		t.Fatalf("expected decision to resolve: %v", err)
	}
	report := buildCompareReportWithDecisionMatches(oldSnapshot, newSnapshot, nil, compareMatchModeExact, true, decisionMatches)

	if report.Summary.MissingNodes != 0 || report.Summary.NewNodes != 0 || report.Summary.TextChanged != 1 {
		t.Fatalf("decision pair should suppress missing/new and compare matched node: %+v", report.Summary)
	}
	if report.Summary.DecisionMatches != 1 {
		t.Fatalf("expected decision match summary: %+v", report.Summary)
	}
	if report.Findings[0].MatchedBy != "decision:pair" {
		t.Fatalf("expected decision metadata: %+v", report.Findings[0])
	}
	if report.MatchingDebug == nil || len(report.MatchingDebug.Matches) != 1 || report.MatchingDebug.Matches[0].MatchedBy != "decision:pair" {
		t.Fatalf("expected decision in matching debug: %+v", report.MatchingDebug)
	}
}

func TestCompareHighConfidenceSubtreePairMatchesOrderedChildren(t *testing.T) {
	oldSnapshot := compareSnapshot{
		Nodes: []compareSnapshotNode{
			{ID: 1, Children: []int{2, 3}, Fingerprint: "old-list", Ref: "@e1", Role: "list", Label: "Jobs", OriginalIndex: 0, Visible: true},
			{ID: 2, Fingerprint: "old-a", Ref: "@e2", Role: "listitem", Label: "Engineer", OriginalIndex: 1, Visible: true},
			{ID: 3, Fingerprint: "old-b", Ref: "@e3", Role: "listitem", Label: "Designer", OriginalIndex: 2, Visible: true},
		},
	}
	newSnapshot := compareSnapshot{
		Nodes: []compareSnapshotNode{
			{ID: 10, Children: []int{11, 12}, Fingerprint: "new-grid", Ref: "@e10", Role: "generic", Label: "Jobs", OriginalIndex: 0, Visible: true},
			{ID: 11, Fingerprint: "new-a", Ref: "@e11", Role: "link", Label: "Engineer", OriginalIndex: 1, Visible: true, Invokable: true},
			{ID: 12, Fingerprint: "new-b", Ref: "@e12", Role: "link", Label: "Designer", OriginalIndex: 2, Visible: true, Invokable: true},
		},
	}
	decisionMatches, err := compareResolveDecisionMatches(
		[]compareDecision{{Kind: "subtree_pair", Old: "@e1", New: "@e10", Confidence: "high", MatchKind: "ordered_children", Count: 2}},
		oldSnapshot.Nodes,
		newSnapshot.Nodes,
	)
	if err != nil {
		t.Fatalf("expected subtree decision to resolve: %v", err)
	}
	report := buildCompareReportWithDecisionMatches(oldSnapshot, newSnapshot, nil, compareMatchModeExact, true, decisionMatches)

	if len(decisionMatches) != 3 || report.Summary.DecisionMatches != 3 || report.Summary.MissingNodes != 0 || report.Summary.NewNodes != 0 {
		t.Fatalf("expected root and ordered children to be decision matched: matches=%+v summary=%+v", decisionMatches, report.Summary)
	}
	if report.MatchingDebug == nil || len(report.MatchingDebug.Matches) != 3 || report.MatchingDebug.Matches[1].MatchedBy != "decision:subtree_pair" {
		t.Fatalf("expected subtree decision in matching debug: %+v", report.MatchingDebug)
	}
}

func TestValidateCompareDecisionsDetectsDuplicateHighPair(t *testing.T) {
	report := compareReport{
		Old: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "old-save", Ref: "@e1", Role: "button", Label: "Save", Name: "Save"},
			},
		},
		New: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "new-save", Ref: "@e2", Role: "button", Label: "Save", Name: "Save"},
				{Fingerprint: "new-submit", Ref: "@e3", Role: "button", Label: "Submit", Name: "Submit"},
			},
		},
	}
	validation := validateCompareDecisions([]compareDecision{
		{Kind: "pair", Old: "@e1", New: "@e2", Confidence: "high", Line: 1},
		{Kind: "pair", Old: "@e1", New: "@e3", Confidence: "high", Line: 2},
	}, &report)

	if validation.Summary.Errors == 0 {
		t.Fatalf("expected duplicate high pair error: %+v", validation)
	}
	if validation.Summary.HighPairs != 2 || !validation.Summary.CompareJSONUsed {
		t.Fatalf("expected validation summary to count high pairs: %+v", validation.Summary)
	}
}

func TestValidateCompareDecisionsChecksSubtreePairCount(t *testing.T) {
	report := compareReport{
		Old: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{ID: 1, Children: []int{2}, Fingerprint: "old-list", Ref: "@e1", Role: "list"},
				{ID: 2, Fingerprint: "old-a", Ref: "@e2", Role: "listitem", OriginalIndex: 1},
			},
		},
		New: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{ID: 10, Children: []int{11}, Fingerprint: "new-list", Ref: "@e10", Role: "generic"},
				{ID: 11, Fingerprint: "new-a", Ref: "@e11", Role: "link", OriginalIndex: 1},
			},
		},
	}
	validation := validateCompareDecisions([]compareDecision{
		{Kind: "subtree_pair", Old: "@e1", New: "@e10", Confidence: "high", MatchKind: "ordered_children", Count: 2, Line: 1},
	}, &report)

	if validation.Summary.Errors == 0 || validation.Summary.SubtreePairs != 1 {
		t.Fatalf("expected subtree count validation error: %+v", validation)
	}
}

func TestRunCompareValidateDecisions(t *testing.T) {
	dir := t.TempDir()
	decisionsPath := filepath.Join(dir, "pair-decisions.jsonl")
	comparePath := filepath.Join(dir, "compare.json")
	if err := os.WriteFile(decisionsPath, []byte(`{"kind":"pair","old":"@e1","new":"@e2","confidence":"high"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeIndentedJSONFile(comparePath, compareReport{
		Old: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "old-save", Ref: "@e1", Role: "button", Label: "Save", Name: "Save"},
			},
		},
		New: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "new-save", Ref: "@e2", Role: "button", Label: "Save", Name: "Save"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	code := runCompareValidateDecisions([]string{"--decisions-file", decisionsPath, "--compare-json", comparePath, "--json"}, &stdout, &stdout)
	if code != 0 {
		t.Fatalf("expected validation to pass: %d\n%s", code, stdout.String())
	}
	var report compareDecisionValidationReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("expected validation json: %v\n%s", err, stdout.String())
	}
	if report.Summary.Errors != 0 || report.Summary.HighPairs != 1 || !report.Summary.CompareJSONUsed {
		t.Fatalf("unexpected validation report: %+v", report)
	}
}

func TestRunCompareValidateDecisionsRejectsUnknownKind(t *testing.T) {
	dir := t.TempDir()
	decisionsPath := filepath.Join(dir, "bad-decisions.jsonl")
	if err := os.WriteFile(decisionsPath, []byte(`{"kind":"removed","old":"@e1","confidence":"high"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	code := runCompareValidateDecisions([]string{"--decisions-file", decisionsPath}, &stdout, &stdout)
	if code == 0 {
		t.Fatalf("expected validation to reject unsupported kind:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `unsupported kind "removed"`) {
		t.Fatalf("expected unsupported kind error, got:\n%s", stdout.String())
	}
}

func TestRunCompareNormalizeDecisionsWritesOutput(t *testing.T) {
	dir := t.TempDir()
	decisionsPath := filepath.Join(dir, "decisions.jsonl")
	outputPath := filepath.Join(dir, "decisions.normalized.jsonl")
	input := strings.Join([]string{
		`{"kind":"PAIR","old":" @e1 ","new":"@e2","confidence":"HIGH","reason":" first "}`,
		`{"kind":"pair","old":"@e1","new":"@e2","confidence":"high","reason":"duplicate"}`,
		`{"kind":"accepted_finding","finding_id":" text_changed:abc123 ","confidence":"HIGH","reason":" ok "}`,
	}, "\n") + "\n"
	if err := os.WriteFile(decisionsPath, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	code := runCompareNormalizeDecisions([]string{"--decisions-file", decisionsPath, "--output", outputPath, "--json"}, &stdout, &stdout)
	if code != 0 {
		t.Fatalf("expected normalize to pass: %d\n%s", code, stdout.String())
	}
	var report compareDecisionNormalizeReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("expected normalize json: %v\n%s", err, stdout.String())
	}
	if report.Summary.InputDecisions != 3 || report.Summary.OutputDecisions != 2 || report.Summary.DuplicatesRemoved != 1 {
		t.Fatalf("unexpected normalize summary: %+v", report.Summary)
	}
	outputBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(outputBytes)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two normalized decisions, got %q", string(outputBytes))
	}
	var first compareDecision
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("expected normalized jsonl: %v\n%s", err, lines[0])
	}
	if first.SchemaVersion != 1 || first.Kind != "pair" || first.Old != "@e1" || first.Confidence != "high" || first.Reason != "first" {
		t.Fatalf("unexpected normalized decision: %+v", first)
	}
}

func TestRunCompareNormalizeDecisionsReportsStaleFindingID(t *testing.T) {
	dir := t.TempDir()
	decisionsPath := filepath.Join(dir, "decisions.jsonl")
	comparePath := filepath.Join(dir, "compare.json")
	if err := os.WriteFile(decisionsPath, []byte(`{"kind":"accepted_finding","finding_id":"missing_node:stale","confidence":"high"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeIndentedJSONFile(comparePath, compareReport{
		Findings: []compareFinding{
			{Kind: "missing_node", FindingID: "missing_node:current"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	code := runCompareNormalizeDecisions([]string{"--decisions-file", decisionsPath, "--compare-json", comparePath, "--json"}, &stdout, &stdout)
	if code == 0 {
		t.Fatalf("expected stale finding_id to fail:\n%s", stdout.String())
	}
	var report compareDecisionNormalizeReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("expected normalize json: %v\n%s", err, stdout.String())
	}
	if report.Summary.Errors != 1 || !report.Summary.CompareJSONUsed || len(report.Issues) != 1 || report.Issues[0].Field != "finding_id" {
		t.Fatalf("expected stale finding_id issue: %+v", report)
	}
}

func TestValidateCompareDecisionsReportsUnknownKind(t *testing.T) {
	validation := validateCompareDecisions([]compareDecision{
		{Kind: "removed", Old: "@e1", Confidence: "high", Line: 1},
		{Kind: "pair", Old: "@e1", New: "@e2", Confidence: "high", Line: 2},
	}, nil)

	if validation.Summary.Errors != 1 {
		t.Fatalf("expected unknown kind error: %+v", validation)
	}
	if validation.Summary.HighPairs != 1 {
		t.Fatalf("unsupported kind should not count as a high pair: %+v", validation.Summary)
	}
}

func TestCompareDecisionsTemplateWritesAmbiguousCandidateStubs(t *testing.T) {
	debug := &compareMatchingDebug{
		AmbiguousCandidates: []compareMatchingDebugAmbiguousCandidate{
			{
				Old: compareMatchingDebugNode{
					Ref:         "@e1",
					Fingerprint: "old-save",
				},
				ReasonSkipped: "candidate margin below threshold",
				NewCandidates: []compareMatchingDebugCandidateOption{
					{Node: compareMatchingDebugNode{Ref: "@e2"}},
					{Node: compareMatchingDebugNode{Ref: "@e3"}},
				},
			},
		},
	}

	var buffer bytes.Buffer
	if err := printCompareDecisionsTemplate(&buffer, debug); err != nil {
		t.Fatalf("expected template to render: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one template line, got %q", buffer.String())
	}
	var decision compareDecision
	if err := json.Unmarshal([]byte(lines[0]), &decision); err != nil {
		t.Fatalf("expected jsonl template line: %v\n%s", err, lines[0])
	}
	if decision.Kind != "pair" || decision.Old != "@e1" || decision.New != "?" || decision.Confidence != "unknown" {
		t.Fatalf("unexpected template decision: %+v", decision)
	}
	if !strings.Contains(decision.Note, "@e2") || !strings.Contains(decision.Note, "@e3") {
		t.Fatalf("expected candidate refs in note: %+v", decision)
	}
}

func TestCompareFindingDecisionsTemplateWritesReviewStubs(t *testing.T) {
	report := compareReport{
		Findings: []compareFinding{
			{Kind: "missing_node", FindingID: "missing_node:aaa111", Severity: "critical", Impact: "missing_primary_action", Locator: "role=button", Role: "button", Label: "Submit"},
			{Kind: "layout_changed", FindingID: "layout_changed:bbb222", Severity: "warning", Impact: "layout_changed", Field: "bounds"},
			{Kind: "css_changed", FindingID: "css_changed:ccc333", Severity: "info", Impact: "css_changed"},
			{Kind: "text_changed", Severity: "critical"},
		},
	}

	var buffer bytes.Buffer
	if err := printCompareFindingDecisionsTemplate(&buffer, report); err != nil {
		t.Fatalf("expected finding template to render: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected critical and warning templates, got %d lines:\n%s", len(lines), buffer.String())
	}
	var first compareDecision
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("expected first jsonl line: %v\n%s", err, lines[0])
	}
	if first.Kind != "regression_finding" || first.FindingID != "missing_node:aaa111" || first.Confidence != "unknown" || !strings.Contains(first.Note, "Submit") {
		t.Fatalf("unexpected first finding decision stub: %+v", first)
	}
	var second compareDecision
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("expected second jsonl line: %v\n%s", err, lines[1])
	}
	if second.Kind != "accepted_finding" || second.FindingID != "layout_changed:bbb222" || second.Confidence != "unknown" {
		t.Fatalf("unexpected second finding decision stub: %+v", second)
	}
}

func TestAcceptedRemovedDecisionDowngradesMissingFinding(t *testing.T) {
	report := buildCompareReportWithDecisions(
		compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "button|Save", Ref: "@e1", Role: "button", Label: "Save", Name: "Save", Visible: true, Enabled: true, Invokable: true},
			},
		},
		compareSnapshot{},
		nil,
		compareMatchModeExact,
		false,
		nil,
		compareDecisionEffects{
			Old: map[int]compareDecisionEffect{0: compareDecisionEffectFor("accepted_removed")},
		},
	)

	if report.Summary.MissingNodes != 1 || report.Summary.Info != 1 || report.Summary.Critical != 0 {
		t.Fatalf("expected accepted removed to become info missing finding: %+v", report.Summary)
	}
	if len(report.Findings) != 1 {
		t.Fatalf("expected one missing finding, got %+v", report.Findings)
	}
	finding := report.Findings[0]
	if finding.MatchedBy != "decision:accepted_removed" || finding.Severity != "info" || finding.Impact != "accepted_removed" || finding.DecisionKind != "accepted_removed" {
		t.Fatalf("expected accepted removed metadata: %+v", finding)
	}
}

func TestCompareFindingsIncludeStableIDs(t *testing.T) {
	oldSnapshot := compareSnapshot{Title: "Old title"}
	newSnapshot := compareSnapshot{Title: "New title"}

	first := buildCompareReport(oldSnapshot, newSnapshot, nil, compareMatchModeExact)
	second := buildCompareReport(oldSnapshot, newSnapshot, nil, compareMatchModeExact)

	if len(first.Findings) != 1 || len(second.Findings) != 1 {
		t.Fatalf("expected one title finding: first=%+v second=%+v", first.Findings, second.Findings)
	}
	if first.Findings[0].FindingID == "" || first.Findings[0].FindingID != second.Findings[0].FindingID {
		t.Fatalf("expected stable finding id: first=%+v second=%+v", first.Findings[0], second.Findings[0])
	}
	if !strings.HasPrefix(first.Findings[0].FindingID, "title_changed:") {
		t.Fatalf("expected kind-prefixed finding id: %+v", first.Findings[0])
	}
}

func TestAcceptedFindingDecisionDowngradesExistingFinding(t *testing.T) {
	oldSnapshot := compareSnapshot{
		Nodes: []compareSnapshotNode{
			{Fingerprint: "button|save", Ref: "@e1", Role: "button", Label: "Save", Name: "Save", Visible: true, Enabled: true, Invokable: true},
		},
	}
	newSnapshot := compareSnapshot{
		Nodes: []compareSnapshotNode{
			{Fingerprint: "button|save", Ref: "@e2", Role: "button", Label: "Save changes", Name: "Save changes", Visible: true, Enabled: true, Invokable: true},
		},
	}
	base := buildCompareReport(oldSnapshot, newSnapshot, nil, compareMatchModeExact)
	if len(base.Findings) != 1 || base.Findings[0].FindingID == "" {
		t.Fatalf("expected one text finding with id: %+v", base.Findings)
	}

	report := buildCompareReportWithDecisionEffects(
		oldSnapshot,
		newSnapshot,
		nil,
		compareMatchModeExact,
		false,
		nil,
		compareDecisionEffects{},
		compareFindingDecisionEffects{
			ByID: map[string]compareDecisionEffect{
				base.Findings[0].FindingID: compareDecisionEffectFor("accepted_finding"),
			},
		},
	)

	if report.Summary.TextChanged != 1 || report.Summary.Info != 1 || report.Summary.Critical != 0 {
		t.Fatalf("expected accepted finding to become info: %+v", report.Summary)
	}
	finding := report.Findings[0]
	if finding.FindingID != base.Findings[0].FindingID || finding.Severity != "info" || finding.Impact != "accepted_finding" || finding.DecisionKind != "accepted_finding" {
		t.Fatalf("expected accepted finding metadata: %+v", finding)
	}
}

func TestValidateCompareDecisionsChecksFindingID(t *testing.T) {
	report := compareReport{
		Findings: []compareFinding{
			{Kind: "text_changed", FindingID: "text_changed:abc123"},
		},
	}
	validation := validateCompareDecisions([]compareDecision{
		{Kind: "accepted_finding", FindingID: "text_changed:abc123", Line: 1},
		{Kind: "regression_finding", FindingID: "missing_node:def456", Line: 2},
	}, &report)

	if validation.Summary.AcceptedFindings != 1 || validation.Summary.RegressionFindings != 1 || !validation.Summary.CompareJSONUsed {
		t.Fatalf("expected finding decision counts: %+v", validation.Summary)
	}
	if validation.Summary.Errors != 1 {
		t.Fatalf("expected missing finding_id error: %+v", validation)
	}
	if len(validation.Issues) != 1 || validation.Issues[0].Field != "finding_id" {
		t.Fatalf("expected finding_id validation issue: %+v", validation.Issues)
	}
}

func TestNormalizeCompareMatchMode(t *testing.T) {
	for _, value := range []string{"", "exact", "stable", "heuristic", "histogram", " STABLE "} {
		if _, err := normalizeCompareMatchMode(value); err != nil {
			t.Fatalf("expected %q to be accepted: %v", value, err)
		}
	}
	if _, err := normalizeCompareMatchMode("unknown"); err == nil || !strings.Contains(err.Error(), "exact, stable, heuristic, or histogram") {
		t.Fatalf("expected helpful validation error, got %v", err)
	}
}

func TestCompareNodeScopeFiltersSnapshot(t *testing.T) {
	observation := api.Observation{
		Tree: []api.Node{
			{ID: 1, Fingerprint: "button", Role: "button", Name: "Save", Visible: true, Enabled: true, Invokable: true},
			{ID: 2, Fingerprint: "status", Role: "status", Text: "Ready", Visible: true, Enabled: true},
			{ID: 3, Fingerprint: "generic", Role: "generic", Text: "Decorative", Visible: true, Enabled: true},
		},
	}

	current := buildCompareSnapshot(observation, compareSnapshotOptions{NodeScope: compareNodeScopeCurrent})
	if len(current.Nodes) != 3 {
		t.Fatalf("current node scope should preserve all observed nodes: %+v", current.Nodes)
	}

	actionable := buildCompareSnapshot(observation, compareSnapshotOptions{NodeScope: compareNodeScopeActionable})
	if len(actionable.Nodes) != 1 || actionable.Nodes[0].Role != "button" {
		t.Fatalf("actionable node scope should keep only controls: %+v", actionable.Nodes)
	}

	semantic := buildCompareSnapshot(observation, compareSnapshotOptions{NodeScope: compareNodeScopeSemantic})
	roles := []string{}
	for _, node := range semantic.Nodes {
		roles = append(roles, node.Role)
	}
	if !slices.Contains(roles, "button") || !slices.Contains(roles, "status") || slices.Contains(roles, "generic") {
		t.Fatalf("semantic node scope should keep semantic nodes without generic text: %+v", semantic.Nodes)
	}

	all := buildCompareSnapshot(api.Observation{Tree: []api.Node{
		{ID: 1, Fingerprint: "generic", Role: "generic", Text: "Decorative", Visible: true, StructurePath: "html:1>body:1>div:1", TextLength: 10, Descendants: 2},
	}}, compareSnapshotOptions{NodeScope: compareNodeScopeAll})
	if len(all.Nodes) != 1 || all.Nodes[0].StructureKey != "html:1>body:1>div:1" || all.Nodes[0].SubtreeSignature != "generic|text:1-20|desc:1-3" {
		t.Fatalf("all node scope should preserve structural metadata: %+v", all.Nodes)
	}
}

func TestNormalizeCompareNodeScope(t *testing.T) {
	for _, value := range []string{"", "current", "actionable", "semantic", "all", " ALL "} {
		if _, err := normalizeCompareNodeScope(value); err != nil {
			t.Fatalf("expected %q to be accepted: %v", value, err)
		}
	}
	if _, err := normalizeCompareNodeScope("unknown"); err == nil || !strings.Contains(err.Error(), "current, actionable, semantic, or all") {
		t.Fatalf("expected helpful validation error, got %v", err)
	}
}

func TestCompareNodeScopeAllRequiresScopeSelector(t *testing.T) {
	if err := validateCompareNodeScopeSelectors(compareNodeScopeAll, "", "", ""); err == nil || !strings.Contains(err.Error(), "requires --scope-selector") {
		t.Fatalf("expected all scope to require a scope selector, got %v", err)
	}
	if err := validateCompareNodeScopeSelectors(compareNodeScopeAll, "main", "", ""); err != nil {
		t.Fatalf("expected common scope selector to be accepted: %v", err)
	}
	if err := validateCompareNodeScopeSelectors(compareNodeScopeAll, "", "main", "main"); err != nil {
		t.Fatalf("expected side-specific scope selectors to be accepted: %v", err)
	}
}

func TestCompareHistogramUsesStructureAnchors(t *testing.T) {
	oldNodes := []compareSnapshotNode{
		{Fingerprint: "old-a", Role: "div", StructureKey: "html:1>body:1>main:1>section:1>div:1", OriginalIndex: 0},
		{Fingerprint: "old-b", Role: "div", StructureKey: "html:1>body:1>main:1>section:1>div:2", OriginalIndex: 1},
	}
	newNodes := []compareSnapshotNode{
		{Fingerprint: "new-a", Role: "div", StructureKey: "html:1>body:1>main:1>section:1>div:1", OriginalIndex: 0},
		{Fingerprint: "new-b", Role: "div", StructureKey: "html:1>body:1>main:1>section:1>div:2", OriginalIndex: 1},
	}

	result := compareHistogramNodeMatches(oldNodes, newNodes, true)

	if len(result.Matches) != 2 {
		t.Fatalf("expected structure anchors to match nodes: %+v", result)
	}
	for _, match := range result.Matches {
		if match.MatchedBy != "histogram:structure-key" {
			t.Fatalf("expected structure-key match, got %+v", match)
		}
	}
	if result.Debug == nil || len(result.Debug.Anchors) != 2 || result.Debug.Anchors[0].KeyKind != "structure-key" {
		t.Fatalf("expected structure anchors in debug: %+v", result.Debug)
	}
}

func TestCompareManifestMatchModeAndNodeScopeMerge(t *testing.T) {
	base := compareRun{MatchMode: compareMatchModeExact, NodeScope: compareNodeScopeCurrent}
	run := mergeCompareManifestPage(base, compareManifestDefaults{MatchMode: compareMatchModeStable}, compareManifestPage{})
	if run.MatchMode != compareMatchModeStable {
		t.Fatalf("expected defaults match_mode, got %q", run.MatchMode)
	}

	heuristic := compareMatchModeHeuristic
	semantic := compareNodeScopeSemantic
	disabled := false
	run = mergeCompareManifestPage(base, compareManifestDefaults{MatchMode: compareMatchModeStable, NodeScope: compareNodeScopeActionable, MatchingDebug: true}, compareManifestPage{MatchMode: &heuristic, NodeScope: &semantic, MatchingDebug: &disabled})
	if run.MatchMode != compareMatchModeHeuristic {
		t.Fatalf("expected page match_mode override, got %q", run.MatchMode)
	}
	if run.NodeScope != compareNodeScopeSemantic {
		t.Fatalf("expected page node_scope override, got %q", run.NodeScope)
	}
	if run.MatchingDebug {
		t.Fatalf("expected page matching_debug override")
	}
}
