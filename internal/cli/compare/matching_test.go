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

func TestNormalizeCompareMatchMode(t *testing.T) {
	for _, value := range []string{"", "exact", "stable", "heuristic", " STABLE "} {
		if _, err := normalizeCompareMatchMode(value); err != nil {
			t.Fatalf("expected %q to be accepted: %v", value, err)
		}
	}
	if _, err := normalizeCompareMatchMode("unknown"); err == nil || !strings.Contains(err.Error(), "exact, stable, or heuristic") {
		t.Fatalf("expected helpful validation error, got %v", err)
	}
}

func TestCompareManifestMatchModeMerge(t *testing.T) {
	base := compareRun{MatchMode: compareMatchModeExact}
	run := mergeCompareManifestPage(base, compareManifestDefaults{MatchMode: compareMatchModeStable}, compareManifestPage{})
	if run.MatchMode != compareMatchModeStable {
		t.Fatalf("expected defaults match_mode, got %q", run.MatchMode)
	}

	heuristic := compareMatchModeHeuristic
	run = mergeCompareManifestPage(base, compareManifestDefaults{MatchMode: compareMatchModeStable}, compareManifestPage{MatchMode: &heuristic})
	if run.MatchMode != compareMatchModeHeuristic {
		t.Fatalf("expected page match_mode override, got %q", run.MatchMode)
	}
}
