package comparecmd

import (
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
}

func TestNormalizeCompareNodeScope(t *testing.T) {
	for _, value := range []string{"", "current", "actionable", "semantic", " SEMANTIC "} {
		if _, err := normalizeCompareNodeScope(value); err != nil {
			t.Fatalf("expected %q to be accepted: %v", value, err)
		}
	}
	if _, err := normalizeCompareNodeScope("unknown"); err == nil || !strings.Contains(err.Error(), "current, actionable, or semantic") {
		t.Fatalf("expected helpful validation error, got %v", err)
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
