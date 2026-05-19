package comparecmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
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

func TestRunCompareValidateDecisionsSelectorPreflightRequiresCompareJSON(t *testing.T) {
	dir := t.TempDir()
	decisionsPath := filepath.Join(dir, "selector-decisions.jsonl")
	if err := os.WriteFile(decisionsPath, []byte(`{"kind":"pair","old_selector":"#legacy","new":"@e2","confidence":"high"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	code := runCompareValidateDecisions([]string{"--decisions-file", decisionsPath, "--old-session", "old"}, &stdout, &stdout)
	if code == 0 {
		t.Fatalf("expected selector preflight to require compare json:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "selector preflight requires --compare-json") {
		t.Fatalf("expected compare-json requirement, got:\n%s", stdout.String())
	}
}

func TestPreflightCompareDecisionSelectorsFiltersMaterializeWarning(t *testing.T) {
	report := compareReport{
		Old: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Ref: "@e1", Role: "button", Name: "Save", StructureKey: "body>main>button:nth(1)"},
			},
		},
		New: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Ref: "@e2", Role: "button", Name: "Save", StructureKey: "body>main>button:nth(1)"},
			},
		},
	}
	decisions := []compareDecision{
		{Kind: "pair", Old: "?", OldSelector: "#legacy-save", New: "@e2", Confidence: "high", Line: 7},
	}
	validation := validateCompareDecisions(decisions, &report)
	if validation.Summary.Warnings != 1 {
		t.Fatalf("expected selector materialize warning before preflight: %+v", validation)
	}
	resolver := func(oldSide bool, selector string, nodes []compareSnapshotNode) (compareDecisionRefResolution, error) {
		if !oldSide || selector != "#legacy-save" {
			t.Fatalf("unexpected selector resolution request old=%v selector=%q", oldSide, selector)
		}
		return compareDecisionRefResolution{Ref: "@e1", MatchedBy: "structure_key"}, nil
	}

	preflight := preflightCompareDecisionSelectors(decisions, report, resolver)
	if len(preflight.Issues) != 0 || preflight.Count != 1 {
		t.Fatalf("unexpected preflight result: %+v", preflight)
	}
	applyCompareDecisionSelectorPreflightReport(&validation, preflight)
	if validation.Summary.Errors != 0 || validation.Summary.Warnings != 0 || validation.Summary.SelectorPreflighted != 1 || !validation.Summary.SelectorPreflightUsed {
		t.Fatalf("unexpected validation after preflight: %+v", validation)
	}
}

func TestPreflightCompareDecisionSelectorsChecksNonHighSelectors(t *testing.T) {
	report := compareReport{
		New: compareSnapshot{
			Nodes: []compareSnapshotNode{{Ref: "@e2", Role: "link", Name: "Jobs"}},
		},
	}
	decisions := []compareDecision{
		{Kind: "accepted_added", NewSelector: "main a.jobs", Confidence: "unknown", Line: 3},
	}
	resolver := func(oldSide bool, selector string, nodes []compareSnapshotNode) (compareDecisionRefResolution, error) {
		if oldSide || selector != "main a.jobs" {
			t.Fatalf("unexpected selector resolution request old=%v selector=%q", oldSide, selector)
		}
		return compareDecisionRefResolution{Ref: "@e2", MatchedBy: "content"}, nil
	}

	preflight := preflightCompareDecisionSelectors(decisions, report, resolver)
	if len(preflight.Issues) != 0 || preflight.Count != 1 {
		t.Fatalf("unexpected preflight result: %+v", preflight)
	}
}

func TestRunCompareValidateDecisionsUsesReviewSummaryClusters(t *testing.T) {
	dir := t.TempDir()
	decisionsPath := filepath.Join(dir, "cluster-decisions.jsonl")
	summaryPath := filepath.Join(dir, "review-summary.json")
	findings := []compareFinding{
		{Kind: "layout_changed", FindingID: "layout_changed:a", Severity: "warning", Impact: "layout_changed", Field: "bounds"},
		{Kind: "layout_changed", FindingID: "layout_changed:b", Severity: "warning", Impact: "layout_changed", Field: "bounds"},
	}
	clusters := compareFindingClusters(findings, "dashboard")
	input := fmt.Sprintf(`{"kind":"accepted_finding_cluster","cluster_key":%q,"confidence":"high","reason":"ok"}`+"\n", clusters[0].Key)
	if err := os.WriteFile(decisionsPath, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeIndentedJSONFile(summaryPath, compareReviewSummary{FindingClusters: clusters}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	code := runCompareValidateDecisions([]string{"--decisions-file", decisionsPath, "--review-summary", summaryPath, "--json"}, &stdout, &stdout)
	if code != 0 {
		t.Fatalf("expected validation to pass: %d\n%s", code, stdout.String())
	}
	var report compareDecisionValidationReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("expected validation json: %v\n%s", err, stdout.String())
	}
	if report.Summary.Errors != 0 || !report.Summary.ReviewSummaryUsed || report.Summary.AcceptedFindings != 1 {
		t.Fatalf("unexpected validation report: %+v", report)
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

func TestRunCompareNormalizeDecisionsMaterializesFindingCluster(t *testing.T) {
	dir := t.TempDir()
	decisionsPath := filepath.Join(dir, "decisions.jsonl")
	comparePath := filepath.Join(dir, "compare.json")
	outputPath := filepath.Join(dir, "decisions.normalized.jsonl")
	findings := []compareFinding{
		{Kind: "layout_changed", FindingID: "layout_changed:a", Severity: "warning", Impact: "layout_changed", Field: "bounds"},
		{Kind: "layout_changed", FindingID: "layout_changed:b", Severity: "warning", Impact: "layout_changed", Field: "bounds"},
	}
	clusterKey := compareFindingClusterKey(findings[0])
	input := fmt.Sprintf(`{"kind":"accepted_finding_cluster","cluster_key":%q,"confidence":"high","reason":"ok"}`+"\n", clusterKey)
	if err := os.WriteFile(decisionsPath, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeIndentedJSONFile(comparePath, compareReport{Findings: findings}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	code := runCompareNormalizeDecisions([]string{"--decisions-file", decisionsPath, "--compare-json", comparePath, "--output", outputPath, "--json"}, &stdout, &stdout)
	if code != 0 {
		t.Fatalf("expected normalize to pass: %d\n%s", code, stdout.String())
	}
	var report compareDecisionNormalizeReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("expected normalize json: %v\n%s", err, stdout.String())
	}
	if report.Summary.InputDecisions != 1 || report.Summary.OutputDecisions != 2 || report.Summary.Errors != 0 || !report.Summary.CompareJSONUsed {
		t.Fatalf("unexpected normalize summary: %+v", report.Summary)
	}
	outputBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(outputBytes)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two materialized decisions, got %q", string(outputBytes))
	}
	for i, line := range lines {
		var decision compareDecision
		if err := json.Unmarshal([]byte(line), &decision); err != nil {
			t.Fatalf("expected materialized jsonl: %v\n%s", err, line)
		}
		if decision.Kind != "accepted_finding" || decision.FindingID != findings[i].FindingID || decision.ClusterKey != "" || decision.Confidence != "high" {
			t.Fatalf("unexpected materialized decision: %+v", decision)
		}
	}
}

func TestRunCompareNormalizeDecisionsMaterializesFindingClusterFromReviewSummary(t *testing.T) {
	dir := t.TempDir()
	decisionsPath := filepath.Join(dir, "decisions.jsonl")
	summaryPath := filepath.Join(dir, "review-summary.json")
	outputPath := filepath.Join(dir, "decisions.normalized.jsonl")
	findings := []compareFinding{
		{Kind: "layout_changed", FindingID: "layout_changed:a", Severity: "warning", Impact: "layout_changed", Field: "bounds"},
		{Kind: "layout_changed", FindingID: "layout_changed:b", Severity: "warning", Impact: "layout_changed", Field: "bounds"},
	}
	clusters := compareFindingClusters(findings, "dashboard")
	input := fmt.Sprintf(`{"kind":"regression_finding_cluster","cluster_key":%q,"confidence":"high","reason":"regression"}`+"\n", clusters[0].Key)
	if err := os.WriteFile(decisionsPath, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeIndentedJSONFile(summaryPath, compareReviewSummary{FindingClusters: clusters}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	code := runCompareNormalizeDecisions([]string{"--decisions-file", decisionsPath, "--review-summary", summaryPath, "--output", outputPath, "--json"}, &stdout, &stdout)
	if code != 0 {
		t.Fatalf("expected normalize to pass: %d\n%s", code, stdout.String())
	}
	var report compareDecisionNormalizeReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("expected normalize json: %v\n%s", err, stdout.String())
	}
	if report.Summary.OutputDecisions != 2 || !report.Summary.ReviewSummaryUsed || report.Summary.CompareJSONUsed {
		t.Fatalf("unexpected normalize summary: %+v", report.Summary)
	}
	outputBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(outputBytes), `"kind":"regression_finding"`) || strings.Contains(string(outputBytes), "regression_finding_cluster") {
		t.Fatalf("expected materialized regression finding decisions:\n%s", string(outputBytes))
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

func TestRunCompareMaterializeDecisionsWritesRefs(t *testing.T) {
	dir := t.TempDir()
	decisionsPath := filepath.Join(dir, "decisions.jsonl")
	comparePath := filepath.Join(dir, "compare.json")
	outputPath := filepath.Join(dir, "decisions.materialized.jsonl")
	input := strings.Join([]string{
		`{"kind":"pair","old_locator":"role:button label:\"Save changes\"","new_locator":"href:/jobs","confidence":"high","reason":"same CTA"}`,
		`{"kind":"accepted_removed","old_locator":"text \"Legacy only\"","old_selector":"#legacy","reason":"intentional removal"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(decisionsPath, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeIndentedJSONFile(comparePath, compareReport{
		Old: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "old-save", Ref: "@e1", Role: "button", Label: "Save changes", Name: "Save changes"},
				{Fingerprint: "old-legacy", Ref: "@e3", Role: "link", Text: "Legacy only"},
			},
		},
		New: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "new-jobs", Ref: "@e2", Role: "link", Label: "Jobs", Name: "Jobs", Href: "/jobs"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	code := runCompareMaterializeDecisions([]string{"--decisions-file", decisionsPath, "--compare-json", comparePath, "--output", outputPath, "--json"}, &stdout, &stdout)
	if code != 0 {
		t.Fatalf("expected materialize to pass: %d\n%s", code, stdout.String())
	}
	var report compareDecisionMaterializeReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("expected materialize json: %v\n%s", err, stdout.String())
	}
	if report.Summary.InputDecisions != 2 || report.Summary.OutputDecisions != 2 || report.Summary.MaterializedRefs != 3 || report.Summary.Errors != 0 || !report.Summary.CompareJSONUsed {
		t.Fatalf("unexpected materialize summary: %+v", report.Summary)
	}
	if len(report.Materialized) != 3 {
		t.Fatalf("expected materialized detail entries: %+v", report.Materialized)
	}
	if report.Materialized[0].Line != 1 || report.Materialized[0].Side != "old" || report.Materialized[0].Source != "old_locator" || report.Materialized[0].Ref != "@e1" || report.Materialized[0].MatchedBy != "locator" {
		t.Fatalf("unexpected first materialized detail: %+v", report.Materialized[0])
	}
	outputBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(outputBytes)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two materialized decisions, got %q", string(outputBytes))
	}
	var first compareDecision
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("expected first materialized decision: %v\n%s", err, lines[0])
	}
	if first.Old != "@e1" || first.New != "@e2" || first.OldLocator == "" || first.NewLocator == "" {
		t.Fatalf("unexpected first materialized decision: %+v", first)
	}
	var second compareDecision
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("expected second materialized decision: %v\n%s", err, lines[1])
	}
	if second.Kind != "accepted_removed" || second.Old != "@e3" || second.OldLocator == "" || second.OldSelector != "#legacy" {
		t.Fatalf("unexpected second materialized decision: %+v", second)
	}
}

func TestRunCompareRepairDecisionsWritesRefs(t *testing.T) {
	dir := t.TempDir()
	decisionsPath := filepath.Join(dir, "decisions.jsonl")
	comparePath := filepath.Join(dir, "compare.json")
	outputPath := filepath.Join(dir, "decisions.repaired.jsonl")
	input := strings.Join([]string{
		`{"kind":"pair","old":"@e99","old_fingerprint":"old-save","new":"@e98","new_locator":"href:/jobs","confidence":"high","reason":"same CTA"}`,
		`{"kind":"accepted_removed","old":"@e3","old_fingerprint":"old-legacy","reason":"current ref should stay"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(decisionsPath, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeIndentedJSONFile(comparePath, compareReport{
		Old: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "old-save", Ref: "@e1", Role: "button", Label: "Save changes", Name: "Save changes"},
				{Fingerprint: "old-legacy", Ref: "@e3", Role: "link", Text: "Legacy only"},
			},
		},
		New: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "new-jobs", Ref: "@e2", Role: "link", Label: "Jobs", Name: "Jobs", Href: "/jobs"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	code := runCompareRepairDecisions([]string{"--decisions-file", decisionsPath, "--compare-json", comparePath, "--output", outputPath, "--json"}, &stdout, &stdout)
	if code != 0 {
		t.Fatalf("expected repair to pass: %d\n%s", code, stdout.String())
	}
	var report compareDecisionRepairReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("expected repair json: %v\n%s", err, stdout.String())
	}
	if report.Summary.InputDecisions != 2 || report.Summary.OutputDecisions != 2 || report.Summary.RepairedRefs != 2 || report.Summary.UnrepairedRefs != 0 || report.Summary.Warnings != 0 || !report.Summary.CompareJSONUsed {
		t.Fatalf("unexpected repair summary: %+v", report.Summary)
	}
	if len(report.Repaired) != 2 {
		t.Fatalf("expected two repaired refs: %+v", report.Repaired)
	}
	if report.Repaired[0].Side != "old" || report.Repaired[0].Source != "old_fingerprint" || report.Repaired[0].OldRef != "@e99" || report.Repaired[0].NewRef != "@e1" || report.Repaired[0].MatchedBy != "fingerprint" {
		t.Fatalf("unexpected first repair detail: %+v", report.Repaired[0])
	}
	if report.Repaired[1].Side != "new" || report.Repaired[1].Source != "new_locator" || report.Repaired[1].OldRef != "@e98" || report.Repaired[1].NewRef != "@e2" || report.Repaired[1].MatchedBy != "locator" {
		t.Fatalf("unexpected second repair detail: %+v", report.Repaired[1])
	}
	outputBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(outputBytes)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two repaired decisions, got %q", string(outputBytes))
	}
	var first compareDecision
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("expected first repaired decision: %v\n%s", err, lines[0])
	}
	if first.Old != "@e1" || first.New != "@e2" || first.OldFingerprint != "old-save" || first.NewLocator == "" {
		t.Fatalf("unexpected first repaired decision: %+v", first)
	}
	var second compareDecision
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("expected second repaired decision: %v\n%s", err, lines[1])
	}
	if second.Old != "@e3" {
		t.Fatalf("expected current ref to stay unchanged: %+v", second)
	}
}

func TestRunCompareRepairDecisionsReportsUnrepairedRef(t *testing.T) {
	dir := t.TempDir()
	decisionsPath := filepath.Join(dir, "decisions.jsonl")
	comparePath := filepath.Join(dir, "compare.json")
	if err := os.WriteFile(decisionsPath, []byte(`{"kind":"accepted_removed","old":"@e9","reason":"stale without metadata"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeIndentedJSONFile(comparePath, compareReport{
		Old: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "old-current", Ref: "@e1", Role: "link", Text: "Current"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	code := runCompareRepairDecisions([]string{"--decisions-file", decisionsPath, "--compare-json", comparePath, "--json"}, &stdout, &stdout)
	if code != 0 {
		t.Fatalf("expected unresolved repair to warn without failing: %d\n%s", code, stdout.String())
	}
	var report compareDecisionRepairReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("expected repair json: %v\n%s", err, stdout.String())
	}
	if report.Summary.RepairedRefs != 0 || report.Summary.UnrepairedRefs != 1 || report.Summary.Warnings != 1 || len(report.Issues) != 1 || report.Issues[0].Field != "old" {
		t.Fatalf("unexpected unrepaired report: %+v", report)
	}
}

func TestMaterializeCompareDecisionSelectorsWritesRefs(t *testing.T) {
	decisions := []compareDecision{
		{Kind: "pair", OldSelector: "#save", NewSelector: "a.jobs", Confidence: "high", Reason: "same CTA"},
	}
	report := compareReport{
		Old: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "old-save", Ref: "@e1", Role: "button", Label: "Save", Name: "Save", TestID: "save"},
			},
		},
		New: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "new-jobs", Ref: "@e2", Role: "link", Label: "Jobs", Name: "Jobs", Href: "/jobs"},
			},
		},
	}
	resolver := func(oldSide bool, selector string, nodes []compareSnapshotNode) (compareDecisionRefResolution, error) {
		if oldSide {
			return compareResolveSelectorMaterializedRef("old", selector, compareSnapshotNode{Role: "button", Label: "Save", Name: "Save", TestID: "save"}, nodes)
		}
		return compareResolveSelectorMaterializedRef("new", selector, compareSnapshotNode{Role: "link", Label: "Jobs", Name: "Jobs", Href: "/jobs"}, nodes)
	}

	materialized, issues, refs := materializeCompareDecisionSelectors(decisions, report, resolver)
	if len(issues) != 0 {
		t.Fatalf("expected selector materialization without issues: %+v", issues)
	}
	if len(refs) != 2 || len(materialized) != 1 || materialized[0].Old != "@e1" || materialized[0].New != "@e2" {
		t.Fatalf("unexpected materialized decisions: refs=%+v decisions=%+v", refs, materialized)
	}
	if refs[0].Source != "old_selector" || refs[0].MatchedBy != "testid" || refs[1].Source != "new_selector" || refs[1].MatchedBy != "href" {
		t.Fatalf("unexpected selector materialization details: %+v", refs)
	}
	if materialized[0].OldSelector != "#save" || materialized[0].NewSelector != "a.jobs" {
		t.Fatalf("expected selectors to remain as audit metadata: %+v", materialized[0])
	}
}

func TestRunCompareMaterializeDecisionsRequiresSessionForSelector(t *testing.T) {
	dir := t.TempDir()
	decisionsPath := filepath.Join(dir, "decisions.jsonl")
	comparePath := filepath.Join(dir, "compare.json")
	if err := os.WriteFile(decisionsPath, []byte(`{"kind":"accepted_removed","old_selector":"#legacy","confidence":"high"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeIndentedJSONFile(comparePath, compareReport{
		Old: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "old-legacy", Ref: "@e1", Role: "link", Label: "Legacy", Name: "Legacy"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	code := runCompareMaterializeDecisions([]string{"--decisions-file", decisionsPath, "--compare-json", comparePath, "--json"}, &stdout, &stdout)
	if code == 0 {
		t.Fatalf("expected selector without session to fail:\n%s", stdout.String())
	}
	var report compareDecisionMaterializeReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("expected materialize json: %v\n%s", err, stdout.String())
	}
	if report.Summary.Errors != 1 || report.Issues[0].Field != "old_selector" || !strings.Contains(report.Issues[0].Message, "--old-session") {
		t.Fatalf("expected old_selector session issue: %+v", report)
	}
}

func TestValidateCompareDecisionsWarnsForSelectorOnly(t *testing.T) {
	decisions := []compareDecision{
		{Kind: "pair", OldSelector: "#save", New: "@e2", Confidence: "high"},
	}
	report := compareReport{
		New: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "new-save", Ref: "@e2", Role: "button", Label: "Save", Name: "Save"},
			},
		},
	}

	validation := validateCompareDecisions(decisions, &report)
	if validation.Summary.Errors != 0 || validation.Summary.Warnings != 1 || len(validation.Issues) != 1 {
		t.Fatalf("expected one selector materialize warning: %+v", validation)
	}
	if validation.Issues[0].Field != "old_selector" || !strings.Contains(validation.Issues[0].Message, "materialize-decisions") {
		t.Fatalf("unexpected selector warning: %+v", validation.Issues[0])
	}
}

func TestRunCompareMaterializeDecisionsRejectsAmbiguousLocator(t *testing.T) {
	dir := t.TempDir()
	decisionsPath := filepath.Join(dir, "decisions.jsonl")
	comparePath := filepath.Join(dir, "compare.json")
	if err := os.WriteFile(decisionsPath, []byte(`{"kind":"pair","old_locator":"role:button","new":"@e9","confidence":"high"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeIndentedJSONFile(comparePath, compareReport{
		Old: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "old-a", Ref: "@e1", Role: "button", Label: "Save", Name: "Save"},
				{Fingerprint: "old-b", Ref: "@e2", Role: "button", Label: "Cancel", Name: "Cancel"},
			},
		},
		New: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "new-save", Ref: "@e9", Role: "button", Label: "Save", Name: "Save"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	code := runCompareMaterializeDecisions([]string{"--decisions-file", decisionsPath, "--compare-json", comparePath, "--json"}, &stdout, &stdout)
	if code == 0 {
		t.Fatalf("expected ambiguous locator to fail:\n%s", stdout.String())
	}
	var report compareDecisionMaterializeReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("expected materialize json: %v\n%s", err, stdout.String())
	}
	if report.Summary.Errors != 1 || report.Issues[0].Field != "old_locator" || !strings.Contains(report.Issues[0].Message, "matched 2 nodes") || !strings.Contains(report.Issues[0].Message, "@e1") {
		t.Fatalf("expected ambiguous locator issue: %+v", report)
	}
}

func TestRunCompareAuditDecisionsReportsAppliedPendingStaleAndConflicts(t *testing.T) {
	dir := t.TempDir()
	decisionsPath := filepath.Join(dir, "decisions.jsonl")
	comparePath := filepath.Join(dir, "compare.json")
	input := strings.Join([]string{
		`{"kind":"pair","old":"@e1","new":"@e2","confidence":"high"}`,
		`{"kind":"pair","old":"@e3","new":"?","confidence":"unknown"}`,
		`{"kind":"accepted_finding","finding_id":"text_changed:abc123","confidence":"high"}`,
		`{"kind":"regression_finding","finding_id":"text_changed:abc123","confidence":"high"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(decisionsPath, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeIndentedJSONFile(comparePath, compareReport{
		Old: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "old-save", Ref: "@e1", Role: "button", Label: "Save"},
				{Fingerprint: "old-cancel", Ref: "@e3", Role: "button", Label: "Cancel"},
			},
		},
		New: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "new-save", Ref: "@e2", Role: "button", Label: "Save"},
			},
		},
		Findings: []compareFinding{
			{Kind: "text_changed", FindingID: "text_changed:abc123", DecisionKind: "accepted_finding"},
		},
		MatchingDebug: &compareMatchingDebug{
			Matches: []compareMatchingDebugMatch{
				{
					Old:       compareMatchingDebugNode{Index: 0, Ref: "@e1"},
					New:       compareMatchingDebugNode{Index: 0, Ref: "@e2"},
					MatchedBy: "decision:pair",
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	code := runCompareAuditDecisions([]string{"--decisions-file", decisionsPath, "--compare-json", comparePath, "--json"}, &stdout, &stdout)
	if code == 0 {
		t.Fatalf("expected conflicting decisions to fail:\n%s", stdout.String())
	}
	var report compareDecisionAuditReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("expected audit json: %v\n%s", err, stdout.String())
	}
	if report.Summary.TotalDecisions != 4 || report.Summary.Applied != 2 || report.Summary.Pending != 1 || report.Summary.Stale != 1 || report.Summary.Conflicts != 1 {
		t.Fatalf("unexpected audit summary: %+v", report.Summary)
	}
	if report.Summary.Errors != 1 || report.Summary.Warnings != 1 || !report.Summary.CompareJSONUsed {
		t.Fatalf("unexpected audit issue counts: %+v issues=%+v", report.Summary, report.Issues)
	}
}

func TestRunCompareAuditDecisionsRequiresCompareJSON(t *testing.T) {
	dir := t.TempDir()
	decisionsPath := filepath.Join(dir, "decisions.jsonl")
	if err := os.WriteFile(decisionsPath, []byte(`{"kind":"pair","old":"@e1","new":"@e2","confidence":"high"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	code := runCompareAuditDecisions([]string{"--decisions-file", decisionsPath}, &stdout, &stdout)
	if code == 0 {
		t.Fatalf("expected audit to require compare-json:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "requires --compare-json") {
		t.Fatalf("expected compare-json error, got:\n%s", stdout.String())
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

func TestCompareNodeSelectorPrefersUniqueStableSelectors(t *testing.T) {
	nodes := []compareSnapshotNode{
		{Ref: "@e1", Tag: "button", IDAttr: "save:primary"},
		{Ref: "@e2", Tag: "button", TestID: "cancel"},
		{Ref: "@e3", Tag: "button", TestID: "cancel"},
		{Ref: "@e4", Tag: "a", Href: "/jobs"},
		{Ref: "@e5", Tag: "div", StructureKey: "html:1>body:1>main:1>section:2>div:3"},
	}

	if got := compareNodeSelector(nodes[0], nodes); got != `button[id="save:primary"]` {
		t.Fatalf("expected id selector, got %q", got)
	}
	if got := compareNodeSelector(nodes[1], nodes); got != "" {
		t.Fatalf("expected duplicate testid selector to be skipped, got %q", got)
	}
	if got := compareNodeSelector(nodes[3], nodes); got != `a[href="/jobs"]` {
		t.Fatalf("expected href selector, got %q", got)
	}
	if got := compareNodeSelector(nodes[4], nodes); got != `html > body:nth-of-type(1) > main:nth-of-type(1) > section:nth-of-type(2) > div:nth-of-type(3)` {
		t.Fatalf("expected structure selector, got %q", got)
	}
}

func TestCompareDecisionsTemplateWritesAmbiguousCandidateStubs(t *testing.T) {
	debug := &compareMatchingDebug{
		AmbiguousCandidates: []compareMatchingDebugAmbiguousCandidate{
			{
				Old: compareMatchingDebugNode{
					Ref:         "@e1",
					Locator:     `role button --name "Save"`,
					Selector:    `button[id="save"]`,
					Fingerprint: "old-save",
				},
				ReasonSkipped: "candidate margin below threshold",
				NewCandidates: []compareMatchingDebugCandidateOption{
					{
						Node:       compareMatchingDebugNode{Ref: "@e2", Locator: `role button --name "Save"`, Selector: `button[data-testid="save"],button[data-test="save"]`},
						Score:      85,
						SharedKeys: []string{"role", "name"},
					},
					{
						Node:          compareMatchingDebugNode{Ref: "@e3", Locator: `role button --name "Cancel"`},
						Score:         65,
						DifferingKeys: []string{"bbox"},
					},
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
	if decision.OldLocator != `role button --name "Save"` {
		t.Fatalf("expected old locator in template: %+v", decision)
	}
	if decision.OldSelector != `button[id="save"]` {
		t.Fatalf("expected old selector in template: %+v", decision)
	}
	if !strings.Contains(decision.Note, "@e2") || !strings.Contains(decision.Note, `locator="role button --name \"Save\""`) || !strings.Contains(decision.Note, `selector="button[data-testid=\"save\"],button[data-test=\"save\"]"`) || !strings.Contains(decision.Note, "score=85") || !strings.Contains(decision.Note, "shared=role,name") {
		t.Fatalf("expected candidate refs, locators, selectors, and scores in note: %+v", decision)
	}
}

func TestCompareDecisionsTemplateWritesUnmatchedStubs(t *testing.T) {
	debug := &compareMatchingDebug{
		UnmatchedOld: []compareMatchingDebugNode{
			{Ref: "@e10", Locator: `role link --name "Legacy"`, Selector: `a[id="legacy"]`, Fingerprint: "old-legacy", Role: "link", Label: "Legacy"},
		},
		UnmatchedNew: []compareMatchingDebugNode{
			{Ref: "@e88", Locator: `testid "skip-link"`, Selector: `a[data-testid="skip-link"],a[data-test="skip-link"]`, Fingerprint: "new-skip", Role: "link", Label: "Skip to content", TestID: "skip-link"},
		},
	}

	var buffer bytes.Buffer
	if err := printCompareDecisionsTemplate(&buffer, debug); err != nil {
		t.Fatalf("expected template to render: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected unmatched old/new stubs, got %d lines:\n%s", len(lines), buffer.String())
	}
	var oldDecision compareDecision
	if err := json.Unmarshal([]byte(lines[0]), &oldDecision); err != nil {
		t.Fatalf("expected old stub jsonl: %v\n%s", err, lines[0])
	}
	if oldDecision.Kind != "pair" || oldDecision.Old != "@e10" || oldDecision.New != "?" || oldDecision.OldLocator == "" || oldDecision.OldSelector == "" || !strings.Contains(oldDecision.Note, "accepted_removed") || !strings.Contains(oldDecision.Note, "new_selector") {
		t.Fatalf("unexpected unmatched old stub: %+v", oldDecision)
	}
	var newDecision compareDecision
	if err := json.Unmarshal([]byte(lines[1]), &newDecision); err != nil {
		t.Fatalf("expected new stub jsonl: %v\n%s", err, lines[1])
	}
	if newDecision.Kind != "accepted_added" || newDecision.New != "@e88" || newDecision.NewLocator == "" || newDecision.NewSelector == "" || !strings.Contains(newDecision.Note, "old/old_locator/old_selector") {
		t.Fatalf("unexpected unmatched new stub: %+v", newDecision)
	}
}

func TestCompareDecisionsTemplateDeduplicatesAmbiguousAndUnmatchedNodes(t *testing.T) {
	debug := &compareMatchingDebug{
		AmbiguousCandidates: []compareMatchingDebugAmbiguousCandidate{
			{
				Old: compareMatchingDebugNode{Ref: "@e1", Locator: `role button --name "Save"`},
				NewCandidates: []compareMatchingDebugCandidateOption{
					{Node: compareMatchingDebugNode{Ref: "@e2", Locator: `role button --name "Save"`}},
				},
			},
		},
		UnmatchedOld: []compareMatchingDebugNode{
			{Ref: "@e1", Locator: `role button --name "Save"`},
			{Ref: "@e3", Locator: `role link --name "Legacy"`},
			{Ref: "@e3", Locator: `role link --name "Legacy"`},
		},
		UnmatchedNew: []compareMatchingDebugNode{
			{Ref: "@e2", Locator: `role button --name "Save"`},
			{Ref: "@e4", Locator: `role link --name "Skip"`},
			{Ref: "@e4", Locator: `role link --name "Skip"`},
		},
	}

	plan := buildCompareDecisionTemplatePlan(debug)
	if plan.Counts.Ambiguous != 1 || plan.Counts.UnmatchedOld != 1 || plan.Counts.UnmatchedNew != 1 || plan.Counts.SkippedDuplicateOld != 2 || plan.Counts.SkippedDuplicateNew != 2 {
		t.Fatalf("unexpected template counts: %+v", plan.Counts)
	}

	var buffer bytes.Buffer
	if err := printCompareDecisionsTemplate(&buffer, debug); err != nil {
		t.Fatalf("expected template to render: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected deduplicated ambiguous/unmatched stubs, got %d lines:\n%s", len(lines), buffer.String())
	}
	if strings.Count(buffer.String(), "@e1") != 1 || strings.Count(buffer.String(), "@e2") != 1 {
		t.Fatalf("expected ambiguous duplicate nodes to appear once:\n%s", buffer.String())
	}
}

func TestCompareDecisionsTemplateCapsUnmatchedStubs(t *testing.T) {
	nodes := make([]compareMatchingDebugNode, 0, compareDecisionTemplateMaxUnmatchedNodes+1)
	for i := 0; i < compareDecisionTemplateMaxUnmatchedNodes+1; i++ {
		nodes = append(nodes, compareMatchingDebugNode{Ref: fmt.Sprintf("@e%d", i+1), Role: "link", Label: fmt.Sprintf("Link %d", i+1)})
	}

	var buffer bytes.Buffer
	if err := printCompareDecisionsTemplate(&buffer, &compareMatchingDebug{UnmatchedOld: nodes}); err != nil {
		t.Fatalf("expected template to render: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	if len(lines) != compareDecisionTemplateMaxUnmatchedNodes+1 {
		t.Fatalf("expected capped unmatched stubs plus summary, got %d lines", len(lines))
	}
	var summary compareDecision
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &summary); err != nil {
		t.Fatalf("expected truncation summary jsonl: %v\n%s", err, lines[len(lines)-1])
	}
	if summary.Kind != "unknown" || !strings.Contains(summary.Note, "emitted 50 of 51") {
		t.Fatalf("unexpected truncation summary: %+v", summary)
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

func TestCompareReviewPacketWritesReviewFiles(t *testing.T) {
	dir := t.TempDir()
	report := compareReport{
		Old: compareSnapshot{URL: "https://old.example.test"},
		New: compareSnapshot{URL: "https://new.example.test"},
		Summary: compareSummary{
			TotalFindings:           3,
			Critical:                1,
			Warning:                 2,
			MatchedNodes:            3,
			AmbiguousMatchesSkipped: 1,
		},
		Findings: []compareFinding{
			{Kind: "missing_node", FindingID: "missing_node:aaa111", Severity: "critical", Role: "button", Label: "Submit"},
			{Kind: "layout_changed", FindingID: "layout_changed:bbb222", Severity: "warning", Field: "bounds"},
			{Kind: "layout_changed", FindingID: "layout_changed:ccc333", Severity: "warning", Field: "bounds"},
		},
		MatchingDebug: &compareMatchingDebug{
			AmbiguousCandidates: []compareMatchingDebugAmbiguousCandidate{
				{
					Old: compareMatchingDebugNode{Ref: "@e1", Fingerprint: "old-button"},
					NewCandidates: []compareMatchingDebugCandidateOption{
						{Node: compareMatchingDebugNode{Ref: "@e2"}},
					},
				},
			},
			UnmatchedOld: []compareMatchingDebugNode{{Ref: "@e3"}},
			UnmatchedNew: []compareMatchingDebugNode{{Ref: "@e4"}},
		},
	}

	screenshots := compareReviewScreenshots{
		Old:      []byte("old screenshot bytes"),
		New:      []byte("new screenshot bytes"),
		Warnings: []string{"new screenshot: test warning"},
	}
	audit := compareDecisionAuditReport{
		Summary: compareDecisionAuditSummary{
			TotalDecisions:  4,
			Applied:         2,
			Pending:         1,
			Stale:           1,
			Conflicts:       1,
			Warnings:        1,
			CompareJSONUsed: true,
		},
	}
	if err := writeCompareReviewPacket(dir, report, screenshots, compareReviewPacketOptions{DecisionAudit: &audit}); err != nil {
		t.Fatalf("expected review packet to write: %v", err)
	}
	for _, name := range []string{
		compareReviewFileReview,
		compareReviewFileJSON,
		compareReviewFileMarkdown,
		compareReviewFilePairDecisionsTemplate,
		compareReviewFileFindingDecisionsTemplate,
		compareReviewFileClusterDecisionsTemplate,
		compareReviewFileOldScreenshot,
		compareReviewFileNewScreenshot,
		compareReviewFileSummary,
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("expected %s to exist: %v", name, err)
		}
	}
	var summary compareReviewSummary
	bytes, err := os.ReadFile(filepath.Join(dir, compareReviewFileSummary))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(bytes, &summary); err != nil {
		t.Fatalf("expected review summary json: %v\n%s", err, string(bytes))
	}
	if summary.TotalFindings != 3 || summary.CriticalFindings != 1 || summary.WarningFindings != 2 || summary.AmbiguousCandidates != 1 || summary.UnmatchedOld != 1 || summary.UnmatchedNew != 1 {
		t.Fatalf("unexpected review summary: %+v", summary)
	}
	if summary.Files.ReviewMarkdown != filepath.Join(dir, compareReviewFileReview) || summary.Files.CompareJSON != filepath.Join(dir, compareReviewFileJSON) || len(summary.NextCommands) == 0 {
		t.Fatalf("expected review files and next commands: %+v", summary)
	}
	if summary.PairDecisionTemplate == nil || summary.PairDecisionTemplate.Ambiguous != 1 || summary.PairDecisionTemplate.UnmatchedOld != 1 || summary.PairDecisionTemplate.UnmatchedNew != 1 {
		t.Fatalf("expected pair decision template counts: %+v", summary.PairDecisionTemplate)
	}
	if !slices.ContainsFunc(summary.NextCommands, func(command string) bool {
		return strings.Contains(command, "compare materialize-decisions") && strings.Contains(command, "pair-decisions.materialized.jsonl")
	}) {
		t.Fatalf("expected materialize command in review summary: %+v", summary.NextCommands)
	}
	if summary.Files.OldScreenshot != filepath.Join(dir, compareReviewFileOldScreenshot) || summary.Files.NewScreenshot != filepath.Join(dir, compareReviewFileNewScreenshot) {
		t.Fatalf("expected screenshot file references: %+v", summary.Files)
	}
	if summary.Files.ClusterDecisionsTemplate != filepath.Join(dir, compareReviewFileClusterDecisionsTemplate) || len(summary.FindingClusters) != 1 {
		t.Fatalf("expected cluster decision template summary: %+v", summary)
	}
	if summary.DecisionAudit == nil || summary.DecisionAudit.Applied != 2 || summary.DecisionAudit.Pending != 1 || summary.DecisionAudit.Stale != 1 || summary.DecisionAudit.Conflicts != 1 {
		t.Fatalf("expected decision audit summary: %+v", summary.DecisionAudit)
	}
	if len(summary.ScreenshotWarnings) != 1 {
		t.Fatalf("expected screenshot warning in review summary: %+v", summary)
	}
	oldScreenshot, err := os.ReadFile(filepath.Join(dir, compareReviewFileOldScreenshot))
	if err != nil {
		t.Fatal(err)
	}
	if string(oldScreenshot) != "old screenshot bytes" {
		t.Fatalf("unexpected old screenshot bytes: %q", string(oldScreenshot))
	}
	reviewGuide, err := os.ReadFile(filepath.Join(dir, compareReviewFileReview))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(reviewGuide), "Nexus Compare Review") || !strings.Contains(string(reviewGuide), "pair-decisions.todo.jsonl") || !strings.Contains(string(reviewGuide), "cluster-decisions.todo.jsonl") || !strings.Contains(string(reviewGuide), "Decision audit: 4 decisions, 2 applied, 3 unresolved") || !strings.Contains(string(reviewGuide), "materialize-decisions") {
		t.Fatalf("unexpected review guide:\n%s", string(reviewGuide))
	}
	clusterTemplate, err := os.ReadFile(filepath.Join(dir, compareReviewFileClusterDecisionsTemplate))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(clusterTemplate), `"kind":"accepted_finding_cluster"`) || !strings.Contains(string(clusterTemplate), `"confidence":"unknown"`) || !strings.Contains(string(clusterTemplate), "2 similar") {
		t.Fatalf("unexpected cluster decision template:\n%s", string(clusterTemplate))
	}
}

func TestCompareReviewPacketWritesFindingCrops(t *testing.T) {
	dir := t.TempDir()
	oldBounds := api.Rect{X: 10, Y: 12, W: 20, H: 10}
	newBounds := api.Rect{X: 30, Y: 18, W: 24, H: 12}
	oldCropBounds := api.Rect{X: 11, Y: 52, W: 20, H: 10}
	newCropBounds := api.Rect{X: 31, Y: 58, W: 24, H: 12}
	report := compareReport{
		Old: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "cta", Ref: "@e1", Role: "button", Label: "Submit", MatchBounds: &oldBounds, CropBounds: &oldCropBounds},
			},
		},
		New: compareSnapshot{
			Nodes: []compareSnapshotNode{
				{Fingerprint: "cta", Ref: "@e1", Role: "button", Label: "Submit", MatchBounds: &newBounds, CropBounds: &newCropBounds},
			},
		},
		Summary: compareSummary{TotalFindings: 1, Warning: 1},
		Findings: []compareFinding{
			{Kind: "layout_changed", FindingID: "layout_changed:abc123", Severity: "warning", Fingerprint: "cta", Role: "button", Label: "Submit"},
		},
	}

	if err := writeCompareReviewPacket(dir, report, compareReviewScreenshots{Old: testCompareReviewPNG(t, 80, 80), New: testCompareReviewPNG(t, 90, 90)}, compareReviewPacketOptions{}); err != nil {
		t.Fatalf("expected review packet to write: %v", err)
	}
	for _, side := range []struct {
		name string
		w    int
		h    int
		x    int
		y    int
	}{
		{name: "old", w: 20, h: 10, x: oldCropBounds.X, y: oldCropBounds.Y},
		{name: "new", w: 24, h: 12, x: newCropBounds.X, y: newCropBounds.Y},
	} {
		path := filepath.Join(dir, compareReviewFileFindingsDir, compareReviewFindingCropFileName("layout_changed:abc123", side.name))
		file, err := os.Open(path)
		if err != nil {
			t.Fatalf("expected %s crop to exist: %v", side.name, err)
		}
		img, err := png.Decode(file)
		file.Close()
		if err != nil {
			t.Fatalf("expected %s crop png: %v", side.name, err)
		}
		if img.Bounds().Dx() != side.w || img.Bounds().Dy() != side.h {
			t.Fatalf("unexpected %s crop size: %dx%d", side.name, img.Bounds().Dx(), img.Bounds().Dy())
		}
		pixel := color.RGBAModel.Convert(img.At(0, 0)).(color.RGBA)
		if pixel.R != uint8(side.x%255) || pixel.G != uint8(side.y%255) {
			t.Fatalf("expected %s crop to use crop bounds, got first pixel %+v", side.name, pixel)
		}
	}

	var summary compareReviewSummary
	bytes, err := os.ReadFile(filepath.Join(dir, compareReviewFileSummary))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(bytes, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Files.FindingScreenshotsDir != filepath.Join(dir, compareReviewFileFindingsDir) || len(summary.CropWarnings) != 0 {
		t.Fatalf("unexpected crop summary: %+v", summary)
	}
}

func TestCompareFindingClustersGroupsRepeatedFindings(t *testing.T) {
	clusters := compareFindingClusters([]compareFinding{
		{Kind: "layout_changed", FindingID: "layout_changed:a", Severity: "warning", Impact: "layout_changed", Field: "bounds", Role: "button", Label: "Save", Old: "10,10 20x20", New: "20,10 20x20"},
		{Kind: "layout_changed", FindingID: "layout_changed:b", Severity: "warning", Impact: "layout_changed", Field: "bounds", Role: "button", Label: "Save", Old: "30,10 20x20", New: "40,10 20x20"},
		{Kind: "missing_node", FindingID: "missing_node:c", Severity: "critical", Impact: "missing_primary_action", Role: "button", Label: "Delete"},
		{Kind: "missing_node", FindingID: "missing_node:d", Severity: "critical", Impact: "missing_primary_action", Role: "button", Label: "Delete"},
		{Kind: "css_changed", FindingID: "css_changed:e", Severity: "info", Impact: "css_changed", Field: "color", Old: "red", New: "blue"},
	}, "dashboard")

	if len(clusters) != 2 {
		t.Fatalf("expected repeated critical and warning clusters, got %+v", clusters)
	}
	if clusters[0].Severity != "critical" || clusters[0].Count != 2 || clusters[0].ExampleFindingID != "missing_node:c" || len(clusters[0].FindingIDs) != 2 || len(clusters[0].Pages) != 1 {
		t.Fatalf("unexpected critical cluster: %+v", clusters[0])
	}
	if clusters[1].Severity != "warning" || clusters[1].Count != 2 || clusters[1].Old != "" || clusters[1].New != "" {
		t.Fatalf("expected layout cluster to ignore per-node bounds values: %+v", clusters[1])
	}
}

func testCompareReviewPNG(t *testing.T, width int, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 180, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestCompareManifestReviewPacketWritesManifestFiles(t *testing.T) {
	dir := t.TempDir()
	report := compareManifestReport{
		Manifest: "manifest.json",
		Summary: compareManifestSummary{
			TotalPages:     2,
			ComparedPages:  1,
			FailedPages:    1,
			DifferentPages: 1,
			TotalFindings:  5,
			Critical:       1,
			Warning:        3,
		},
		Pages: []compareManifestPageReport{
			{
				Name: "dashboard",
				Report: &compareReport{
					Summary: compareSummary{TotalFindings: 5, Critical: 1, Warning: 3, Info: 1},
					Findings: []compareFinding{
						{Kind: "missing_node", FindingID: "missing_node:aaa111", Severity: "critical", Impact: "missing_primary_action", Locator: "role=button", Role: "button", Label: "Submit"},
						{Kind: "layout_changed", FindingID: "layout_changed:bbb222", Severity: "warning", Impact: "layout_changed", Field: "bounds"},
						{Kind: "css_changed", FindingID: "css_changed:ccc333", Severity: "info", Impact: "css_changed", Field: "color"},
						{Kind: "text_changed", FindingID: "text_changed:ddd444", Severity: "warning", Impact: "text_changed", Role: "heading", Label: "Title"},
						{Kind: "layout_changed", FindingID: "layout_changed:eee555", Severity: "warning", Impact: "layout_changed", Field: "bounds"},
					},
				},
			},
			{Name: "settings", Error: "failed"},
		},
	}
	pageDirectories := []compareManifestReviewPageDirectory{
		{
			Name:             "dashboard",
			Directory:        filepath.Join(dir, "001-dashboard"),
			Priority:         "critical",
			TotalFindings:    5,
			CriticalFindings: 1,
			WarningFindings:  3,
			PairDecisionTemplate: &compareDecisionTemplateCounts{
				Ambiguous:           2,
				UnmatchedOld:        3,
				UnmatchedNew:        4,
				SkippedDuplicateOld: 1,
			},
			DecisionAudit: &compareDecisionAuditSummary{
				TotalDecisions:  4,
				Applied:         2,
				Pending:         1,
				Stale:           1,
				Conflicts:       1,
				Warnings:        1,
				CompareJSONUsed: true,
			},
			OldScreenshot: filepath.Join(dir, "001-dashboard", compareReviewFileOldScreenshot),
			NewScreenshot: filepath.Join(dir, "001-dashboard", compareReviewFileNewScreenshot),
		},
		{Name: "settings", Directory: filepath.Join(dir, "002-settings"), Error: "failed"},
	}
	cropDir := filepath.Join(dir, "001-dashboard", compareReviewFileFindingsDir)
	if err := os.MkdirAll(cropDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cropDir, compareReviewFindingCropFileName("missing_node:aaa111", "old")), testCompareReviewPNG(t, 20, 10), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeCompareManifestReviewPacket(dir, report, pageDirectories); err != nil {
		t.Fatalf("expected manifest review packet to write: %v", err)
	}
	for _, name := range []string{
		compareReviewFileReview,
		compareReviewFileManifestJSON,
		compareReviewFileManifestMarkdown,
		compareReviewFileIndex,
		compareReviewFileIndexHTML,
		compareReviewFileSummary,
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("expected %s to exist: %v", name, err)
		}
	}
	var summary compareManifestReviewSummary
	bytes, err := os.ReadFile(filepath.Join(dir, compareReviewFileSummary))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(bytes, &summary); err != nil {
		t.Fatalf("expected manifest review summary json: %v\n%s", err, string(bytes))
	}
	if summary.TotalPages != 2 || summary.ComparedPages != 1 || summary.FailedPages != 1 || summary.CriticalFindings != 1 || summary.WarningFindings != 3 || len(summary.FindingClusters) != 1 {
		t.Fatalf("unexpected manifest review summary: %+v", summary)
	}
	if len(summary.Files.PageDirectories) != 2 || summary.Files.PageDirectories[1].Error != "failed" {
		t.Fatalf("expected page directory summary: %+v", summary.Files.PageDirectories)
	}
	if summary.PairDecisionTemplate == nil || summary.PairDecisionTemplate.Ambiguous != 2 || summary.PairDecisionTemplate.UnmatchedOld != 3 || summary.PairDecisionTemplate.UnmatchedNew != 4 || summary.PairDecisionTemplate.SkippedDuplicateOld != 1 {
		t.Fatalf("expected aggregate pair decision counts: %+v", summary.PairDecisionTemplate)
	}
	if summary.DecisionAudit == nil || summary.DecisionAudit.TotalDecisions != 4 || summary.DecisionAudit.Applied != 2 || summary.DecisionAudit.Pending != 1 || summary.DecisionAudit.Stale != 1 || summary.DecisionAudit.Conflicts != 1 {
		t.Fatalf("expected aggregate decision audit: %+v", summary.DecisionAudit)
	}
	if summary.Files.PageDirectories[0].PairDecisionTemplate == nil || summary.Files.PageDirectories[0].PairDecisionTemplate.UnmatchedNew != 4 {
		t.Fatalf("expected page pair decision counts: %+v", summary.Files.PageDirectories[0])
	}
	if summary.Files.PageDirectories[0].DecisionAudit == nil || summary.Files.PageDirectories[0].DecisionAudit.Warnings != 1 {
		t.Fatalf("expected page decision audit summary: %+v", summary.Files.PageDirectories[0])
	}
	if summary.Files.ReviewMarkdown != filepath.Join(dir, compareReviewFileReview) {
		t.Fatalf("expected root review guide in summary: %+v", summary.Files)
	}
	if summary.Files.ReviewIndex != filepath.Join(dir, compareReviewFileIndex) {
		t.Fatalf("expected review index in summary: %+v", summary.Files)
	}
	if summary.Files.ClusterDecisionsTemplate != filepath.Join(dir, compareReviewFileClusterDecisionsTemplate) {
		t.Fatalf("expected cluster decision template in summary: %+v", summary.Files)
	}
	guideBytes, err := os.ReadFile(filepath.Join(dir, compareReviewFileReview))
	if err != nil {
		t.Fatal(err)
	}
	guide := string(guideBytes)
	if !strings.Contains(guide, "Nexus Manifest Review") || !strings.Contains(guide, "Pair decision workload: 9 total") || !strings.Contains(guide, "Decision audit: 4 decisions, 2 applied, 3 unresolved") || !strings.Contains(guide, "cluster-decisions.todo.jsonl") || !strings.Contains(guide, "validate-decisions") || !strings.Contains(guide, "review-index.html") || !strings.Contains(guide, "001-dashboard/REVIEW.md") {
		t.Fatalf("unexpected manifest review guide:\n%s", guide)
	}
	clusterTemplate, err := os.ReadFile(filepath.Join(dir, compareReviewFileClusterDecisionsTemplate))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(clusterTemplate), `"kind":"accepted_finding_cluster"`) || !strings.Contains(string(clusterTemplate), `"confidence":"unknown"`) || !strings.Contains(string(clusterTemplate), "2 similar") {
		t.Fatalf("unexpected manifest cluster decision template:\n%s", string(clusterTemplate))
	}
	if summary.Files.ReviewIndexHTML != filepath.Join(dir, compareReviewFileIndexHTML) {
		t.Fatalf("expected html review index in summary: %+v", summary.Files)
	}
	indexBytes, err := os.ReadFile(filepath.Join(dir, compareReviewFileIndex))
	if err != nil {
		t.Fatal(err)
	}
	index := string(indexBytes)
	if !strings.Contains(index, "Compare Review Index") || !strings.Contains(index, "Pair decisions") || !strings.Contains(index, "Decision audit") || !strings.Contains(index, "3 unresolved") || !strings.Contains(index, "9 total") || !strings.Contains(index, "[REVIEW.md](REVIEW.md)") || !strings.Contains(index, "cluster-decisions.todo.jsonl") || !strings.Contains(index, "[md](001-dashboard/compare.md)") || !strings.Contains(index, "failed") {
		t.Fatalf("unexpected review index:\n%s", index)
	}
	htmlBytes, err := os.ReadFile(filepath.Join(dir, compareReviewFileIndexHTML))
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlBytes)
	if !strings.Contains(html, "<title>Compare Review Index</title>") || !strings.Contains(html, `src="001-dashboard/old.png"`) || !strings.Contains(html, `src="001-dashboard/findings/missing_node-aaa111-old.png"`) || !strings.Contains(html, "cluster decisions") || !strings.Contains(html, "Repeated finding clusters") || !strings.Contains(html, "2 similar") || !strings.Contains(html, "missing_node:aaa111") || !strings.Contains(html, "accepted_finding") || !strings.Contains(html, "regression_finding") || strings.Contains(html, "css_changed:ccc333") {
		t.Fatalf("unexpected html review index:\n%s", html)
	}
}

func TestCompareManifestReviewHTMLFindingsLimitsCriticalAndWarning(t *testing.T) {
	report := &compareReport{
		Findings: []compareFinding{
			{FindingID: "critical-1", Severity: "critical"},
			{FindingID: "warning-1", Severity: "warning"},
			{FindingID: "info-1", Severity: "info"},
			{FindingID: "critical-2", Severity: "critical"},
			{FindingID: "warning-2", Severity: "warning"},
			{FindingID: "critical-3", Severity: "critical"},
			{FindingID: "warning-3", Severity: "warning"},
		},
	}

	previews := compareManifestReviewHTMLFindings("", compareManifestReviewPageDirectory{}, report)
	if len(previews) != 5 {
		t.Fatalf("expected first five critical/warning previews, got %+v", previews)
	}
	if previews[0].FindingID != "critical-1" || previews[1].FindingID != "warning-1" || previews[2].FindingID != "critical-2" {
		t.Fatalf("expected info finding to be skipped while preserving order: %+v", previews)
	}
	if overflow := compareManifestReviewHTMLFindingOverflow(report); overflow != 1 {
		t.Fatalf("expected one hidden critical/warning finding, got %d", overflow)
	}
}

func TestCompareManifestReviewFindingDecisionJSONL(t *testing.T) {
	line := compareManifestReviewFindingDecisionJSONL("accepted_finding", " missing_node:aaa111 ")
	expected := `{"kind":"accepted_finding","finding_id":"missing_node:aaa111","confidence":"high","reason":""}`
	if line != expected {
		t.Fatalf("unexpected decision jsonl:\n%s", line)
	}
	if line := compareManifestReviewFindingDecisionJSONL("accepted_finding", " "); line != "" {
		t.Fatalf("expected empty finding id to produce no decision stub, got %q", line)
	}
}

func TestCompareManifestReviewDirNameSanitizesPageName(t *testing.T) {
	if got := compareManifestReviewDirName("admin/settings page", 1); got != "002-admin-settings-page" {
		t.Fatalf("unexpected sanitized review dir name: %q", got)
	}
	if got := compareManifestReviewDirName("../", 0); got != "001-page-001" {
		t.Fatalf("unexpected fallback review dir name: %q", got)
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
