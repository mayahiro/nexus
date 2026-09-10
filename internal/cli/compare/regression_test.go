package comparecmd

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mayahiro/nexus/internal/api"
)

func TestMatchedNodeAttributesAndStates(t *testing.T) {
	for _, mode := range []string{"exact", "stable", "heuristic", "histogram"} {
		t.Run(mode, func(t *testing.T) {
			old := compareSnapshotNode{ID: 1, Ref: "@e1", Fingerprint: "control", TestID: "control", Role: "link", Href: "/docs", TypeAttr: "text", Placeholder: "Email", States: map[string]string{"checked": "false", "aria-checked": "mixed"}}
			newNode := old
			newNode.Role, newNode.Href, newNode.TypeAttr, newNode.Placeholder = "button", "/billing", "password", "Password"
			newNode.States = map[string]string{"checked": "true", "aria-selected": "false", "aria-checked": "true"}
			report := buildCompareReport(compareSnapshot{Nodes: []compareSnapshotNode{old}}, compareSnapshot{Nodes: []compareSnapshotNode{newNode}}, nil, mode)
			if report.Summary.Same || report.Summary.AttributeChanged != 4 || report.Summary.StateChanged != 3 || report.Summary.MatchedNodes != 1 {
				t.Fatalf("attribute/state changes lost: %+v", report)
			}
			for _, finding := range report.Findings {
				if finding.Field == "aria-selected" && (finding.Old != "<absent>" || finding.New != "false") {
					t.Fatalf("missing and false conflated: %+v", finding)
				}
			}
		})
	}
}

func TestCompareFiltersApplyBeforeMatchingAndOutput(t *testing.T) {
	rules, err := compileCompareSelectorRules([]string{"testid=account"})
	if err != nil {
		t.Fatal(err)
	}
	observation := func(secret string) api.Observation {
		return api.Observation{Text: "Account " + secret + " Confirmation", Tree: []api.Node{
			{ID: 1, Ref: "@e1", Role: "main", Name: "Account " + secret, Text: "Account " + secret, Fingerprint: "main|" + secret, Children: []int{2}, StructurePath: "main:1", Attrs: map[string]string{"tag": "main"}},
			{ID: 2, Ref: "@e2", Role: "span", Name: secret, Text: secret, Fingerprint: "span|" + secret, Children: []int{3}, StructurePath: "main:1>span:1", Attrs: map[string]string{"tag": "span", "data-testid": "account"}},
			{ID: 3, Ref: "@e3", Role: "span", Name: secret, Text: secret, Fingerprint: "span|" + secret, StructurePath: "main:1>span:1>span:1", Attrs: map[string]string{"tag": "span"}},
		}}
	}
	for _, name := range []string{"mask", "ignore", "regex"} {
		t.Run(name, func(t *testing.T) {
			options := compareSnapshotOptions{NodeScope: "all", NoDefaultIgnores: true}
			switch name {
			case "mask":
				options.MaskNode = rules
			case "ignore":
				options.IgnoreNode = rules
			case "regex":
				options.IgnoreText = []*regexp.Regexp{regexp.MustCompile(`account-(old|new)@example\.invalid`)}
			}
			oldRaw, newRaw := observation("account-old@example.invalid"), observation("account-new@example.invalid")
			original, _ := json.Marshal(oldRaw)
			old, newSnapshot := buildCompareSnapshot(oldRaw, options), buildCompareSnapshot(newRaw, options)
			for _, mode := range []string{"exact", "stable", "heuristic", "histogram"} {
				report := buildCompareReportWithDebug(old, newSnapshot, nil, mode, true)
				if !report.Summary.Same {
					t.Fatalf("%s differences survived %s: %+v", mode, name, report.Findings)
				}
				payload, err := json.Marshal(report)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(payload), "account-old@") || strings.Contains(string(payload), "account-new@") {
					t.Fatalf("unfiltered output: %s", payload)
				}
			}
			after, _ := json.Marshal(oldRaw)
			if string(original) != string(after) {
				t.Fatal("filter mutated the source observation")
			}
			if name == "ignore" && (len(old.Nodes) != 1 || len(old.Nodes[0].Children) != 0) {
				t.Fatalf("ignored subtree survived: %+v", old.Nodes)
			}
		})
	}
}

func TestCompareMaskPreservesUnrelatedNodeAndCheckboxValueInProse(t *testing.T) {
	rules, err := compileCompareSelectorRules([]string{"testid=mask"})
	if err != nil {
		t.Fatal(err)
	}
	raw := api.Observation{Text: "Confirmation Private Private", Tree: []api.Node{
		{ID: 1, Role: "checkbox", Name: "Private", Text: "Private", Value: "on", Attrs: map[string]string{"tag": "input", "data-testid": "mask"}},
		{ID: 2, Role: "heading", Name: "Private", Text: "Private", Attrs: map[string]string{"tag": "h1"}},
	}}
	snapshot := buildCompareSnapshot(raw, compareSnapshotOptions{MaskNode: rules, NodeScope: "semantic"})
	if snapshot.Text != "Confirmation" {
		t.Fatalf("unexpected aggregate: %q", snapshot.Text)
	}
	for _, node := range snapshot.Nodes {
		if node.ID == 2 && node.Text != "Private" {
			t.Fatalf("unrelated sibling masked: %+v", node)
		}
	}
}

func TestMaskPreservesSemanticMembershipAndOriginalOrder(t *testing.T) {
	mask, err := compileCompareSelectorRules([]string{"role=heading"})
	if err != nil {
		t.Fatal(err)
	}
	ignore, err := compileCompareSelectorRules([]string{"@e1"})
	if err != nil {
		t.Fatal(err)
	}
	observation := api.Observation{Tree: []api.Node{
		{ID: 1, Ref: "@e1", Role: "button", Name: "Ignored"},
		{ID: 2, Ref: "@e2", Role: "heading", Name: "Private", Text: "Private"},
	}}
	snapshot := buildCompareSnapshot(observation, compareSnapshotOptions{NodeScope: "semantic", MaskNode: mask, IgnoreNode: ignore})
	if len(snapshot.Nodes) != 1 || snapshot.Nodes[0].OriginalIndex != 1 || snapshot.Nodes[0].Name != "" || snapshot.Nodes[0].Text != "" {
		t.Fatalf("mask changed scope membership or order: %+v", snapshot.Nodes)
	}
}

func TestHistogramDecisionsPreserveOneToOneMatching(t *testing.T) {
	nodes := func(order string) []compareSnapshotNode {
		var result []compareSnapshotNode
		for i, name := range strings.Fields(order) {
			node := compareSnapshotNode{ID: i + 1, Ref: fmt.Sprintf("@e%d", i+1), OriginalIndex: i, Fingerprint: name, Role: "button", Name: name}
			if name != "x" {
				node.TestID = name
			}
			result = append(result, node)
		}
		return result
	}
	old := nodes("x x A x B x C")
	newNodes := nodes("x B x A x x C")
	rng := rand.New(rand.NewPCG(1, 2))
	for iteration := 0; iteration < 100; iteration++ {
		decision := compareNodeMatch{OldIndex: 2, NewIndex: 3, MatchedBy: "decision:pair"}
		if iteration > 0 {
			rng.Shuffle(len(newNodes), func(i, j int) { newNodes[i], newNodes[j] = newNodes[j], newNodes[i] })
			for i := range newNodes {
				newNodes[i].OriginalIndex, newNodes[i].ID = i, i+1
			}
			decision.NewIndex = rng.IntN(len(newNodes))
		}
		for _, mode := range []string{"exact", "stable", "heuristic", "histogram"} {
			result := compareMatchNodesWithDecisionMatches(old, newNodes, mode, true, []compareNodeMatch{decision})
			oldUse, newUse := make([]int, len(old)), make([]int, len(newNodes))
			foundDecision := false
			for _, match := range result.Matches {
				oldUse[match.OldIndex]++
				newUse[match.NewIndex]++
				foundDecision = foundDecision || (match.OldIndex == decision.OldIndex && match.NewIndex == decision.NewIndex && match.MatchedBy == decision.MatchedBy)
			}
			for _, i := range result.UnmatchedOld {
				oldUse[i]++
			}
			for _, i := range result.UnmatchedNew {
				newUse[i]++
			}
			if !foundDecision {
				t.Fatalf("%s discarded manual pair", mode)
			}
			for _, counts := range [][]int{oldUse, newUse} {
				for _, count := range counts {
					if count != 1 {
						t.Fatalf("%s iteration %d is not a partition: %v / %v", mode, iteration, oldUse, newUse)
					}
				}
			}
		}
	}
}

func TestFindingIDsDistinguishOccurrencesFullValuesAndPages(t *testing.T) {
	old := compareSnapshot{URL: "https://example.invalid/orders", Nodes: []compareSnapshotNode{
		{ID: 1, Ref: "@e1", Fingerprint: "delete", Role: "button", Label: "Delete"},
		{ID: 2, Ref: "@e2", Fingerprint: "delete", Role: "button", Label: "Delete"},
	}}
	report := buildCompareReport(old, compareSnapshot{}, nil, "exact")
	if report.FindingIDVersion != 2 || len(report.Findings) != 2 || report.Findings[0].FindingID == report.Findings[1].FindingID {
		t.Fatalf("duplicate finding identities: %+v", report)
	}
	effects := compareFindingDecisionEffects{ByID: map[string]compareDecisionEffect{report.Findings[0].FindingID: compareDecisionEffectFor("accepted_finding")}}
	approved := buildCompareReportWithDecisionEffects(old, compareSnapshot{}, nil, "exact", false, nil, compareDecisionEffects{}, effects)
	if approved.Findings[0].DecisionKind != "accepted_finding" || approved.Findings[1].DecisionKind != "" {
		t.Fatalf("approval leaked to another node: %+v", approved.Findings)
	}
	old.URL += "/other"
	other := buildCompareReport(old, compareSnapshot{}, nil, "exact")
	if report.Findings[0].FindingID == other.Findings[0].FindingID {
		t.Fatal("finding ID reused on another page")
	}
	prefix := strings.Repeat("文", 130)
	a := buildCompareReport(compareSnapshot{Text: prefix + "old"}, compareSnapshot{Text: prefix + "first"}, nil, "exact")
	b := buildCompareReport(compareSnapshot{Text: prefix + "old"}, compareSnapshot{Text: prefix + "second"}, nil, "exact")
	if a.Findings[0].FindingID == b.Findings[0].FindingID || a.Findings[0].Old == a.Findings[0].New || a.Findings[0].New != prefix+"first" {
		t.Fatal("full finding values not preserved")
	}
	if !utf8.ValidString(summarizeCompareValue(prefix)) {
		t.Fatal("preview splits UTF-8")
	}
}

func TestSnapshotDOMOrderSurvivesJSON(t *testing.T) {
	nodes := []compareSnapshotNode{
		{ID: 3, Ref: "@e3", Fingerprint: "a", OriginalIndex: 2},
		{ID: 1, Ref: "@e1", Fingerprint: "root", Children: []int{2, 3}, OriginalIndex: 0},
		{ID: 2, Ref: "@e2", Fingerprint: "z", OriginalIndex: 1},
	}
	for _, legacy := range []bool{false, true} {
		payload, err := json.Marshal(nodes)
		if err != nil {
			t.Fatal(err)
		}
		if legacy {
			payload = regexp.MustCompile(`,"original_index":\d+`).ReplaceAll(payload, nil)
		}
		var restored []compareSnapshotNode
		if err := json.Unmarshal(payload, &restored); err != nil {
			t.Fatal(err)
		}
		indices := []int{0, 1, 2}
		compareSortNodeIndicesBySequence(restored, indices)
		if !reflect.DeepEqual(indices, []int{1, 2, 0}) {
			t.Fatalf("DOM order lost (legacy=%v): %v", legacy, indices)
		}
		if !slices.Equal(restored[1].Children, []int{2, 3}) {
			t.Fatal("child sequence lost")
		}
	}
}

func TestSubtreeDecisionAuditSurvivesJSON(t *testing.T) {
	old := compareSnapshot{Nodes: []compareSnapshotNode{
		{ID: 1, Ref: "@e1", Fingerprint: "root", Role: "list", Children: []int{2, 3}},
		{ID: 2, Ref: "@e2", Fingerprint: "z-old-first", Role: "listitem", Name: "First", OriginalIndex: 1},
		{ID: 3, Ref: "@e3", Fingerprint: "a-old-second", Role: "listitem", Name: "Second", OriginalIndex: 2},
	}}
	newSnapshot := compareSnapshot{Nodes: append([]compareSnapshotNode(nil), old.Nodes...)}
	newSnapshot.Nodes[1].Fingerprint, newSnapshot.Nodes[2].Fingerprint = "a-new-first", "z-new-second"
	for _, snapshot := range []*compareSnapshot{&old, &newSnapshot} {
		slices.SortFunc(snapshot.Nodes, func(a, b compareSnapshotNode) int { return strings.Compare(a.Fingerprint, b.Fingerprint) })
	}
	decisions := []compareDecision{{Kind: "subtree_pair", Old: "@e1", New: "@e1", Confidence: "high", MatchKind: "ordered_children", Count: 2}}
	matches, err := compareResolveDecisionMatches(decisions, old.Nodes, newSnapshot.Nodes)
	if err != nil {
		t.Fatal(err)
	}
	report := buildCompareReportWithDecisionMatches(old, newSnapshot, nil, "exact", true, matches)
	before := auditCompareDecisions(decisions, report)
	if before.Summary.Applied != 1 {
		t.Fatalf("initial audit failed: %+v", before)
	}
	for _, legacy := range []bool{false, true} {
		payload, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		if legacy {
			payload = regexp.MustCompile(`,"original_index":\d+`).ReplaceAll(payload, nil)
		}
		var restored compareReport
		if err := json.Unmarshal(payload, &restored); err != nil {
			t.Fatal(err)
		}
		after := auditCompareDecisions(decisions, restored)
		if after.Summary.Applied != 1 || after.Summary.Stale != 0 {
			t.Fatalf("audit changed (legacy=%v): %+v", legacy, after)
		}
	}
}

func TestFindingRefSelectsTheCorrectRepeatedNode(t *testing.T) {
	nodes := []compareSnapshotNode{
		{Ref: "@e1", Fingerprint: "delete", Role: "button", Label: "Delete", CropBounds: &api.Rect{X: 10, Y: 10, W: 20, H: 20}},
		{Ref: "@e2", Fingerprint: "delete", Role: "button", Label: "Delete", CropBounds: &api.Rect{X: 80, Y: 10, W: 20, H: 20}},
	}
	finding := compareFinding{Kind: "missing_node", OldRef: "@e2", Fingerprint: "delete", Role: "button", Label: "Delete"}
	if compareFindingMatchesNode(finding, nodes[0]) || !compareFindingMatchesNode(finding, nodes[1]) {
		t.Fatal("audit matched the wrong occurrence")
	}
	if rect := compareReviewFindingCropRect(nodes, finding, finding.OldRef); rect == nil || rect.X != 80 {
		t.Fatalf("wrong crop: %+v", rect)
	}
}
