package comparecmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const compareDecisionMaxLineBytes = 1024 * 1024

type compareDecision struct {
	SchemaVersion  int    `json:"schema_version,omitempty"`
	Kind           string `json:"kind,omitempty"`
	Old            string `json:"old,omitempty"`
	New            string `json:"new,omitempty"`
	OldFingerprint string `json:"old_fingerprint,omitempty"`
	NewFingerprint string `json:"new_fingerprint,omitempty"`
	Confidence     string `json:"confidence,omitempty"`
	Reason         string `json:"reason,omitempty"`
	DecidedBy      string `json:"decided_by,omitempty"`
	DecidedAt      string `json:"decided_at,omitempty"`
	Context        string `json:"context,omitempty"`
	Line           int    `json:"-"`
}

func loadCompareDecisions(path string) ([]compareDecision, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), compareDecisionMaxLineBytes)

	decisions := make([]compareDecision, 0)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var decision compareDecision
		if err := json.Unmarshal([]byte(line), &decision); err != nil {
			return nil, fmt.Errorf("invalid decisions file %q line %d: %w", path, lineNumber, err)
		}
		decision.Kind = normalizeCompareDecisionToken(decision.Kind)
		decision.Confidence = normalizeCompareDecisionToken(decision.Confidence)
		if decision.Kind == "" {
			return nil, fmt.Errorf("invalid decisions file %q line %d: kind is required", path, lineNumber)
		}
		if !compareDecisionKindSupported(decision.Kind) {
			return nil, fmt.Errorf("invalid decisions file %q line %d: unsupported kind %q", path, lineNumber, decision.Kind)
		}
		decision.Line = lineNumber
		decisions = append(decisions, decision)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("invalid decisions file %q: %w", path, err)
	}
	return decisions, nil
}

func compareResolvePairDecisionMatches(decisions []compareDecision, oldNodes []compareSnapshotNode, newNodes []compareSnapshotNode) ([]compareNodeMatch, error) {
	if len(decisions) == 0 {
		return nil, nil
	}
	matches := make([]compareNodeMatch, 0)
	usedOld := map[int]struct{}{}
	usedNew := map[int]struct{}{}
	for index, decision := range decisions {
		lineNumber := compareDecisionLineNumber(decision, index)
		if decision.Kind != "pair" || decision.Confidence != "high" {
			continue
		}
		oldIndex, err := compareResolveDecisionNode(oldNodes, "old", decision.Old, decision.OldFingerprint)
		if err != nil {
			return nil, fmt.Errorf("decision line %d: %w", lineNumber, err)
		}
		newIndex, err := compareResolveDecisionNode(newNodes, "new", decision.New, decision.NewFingerprint)
		if err != nil {
			return nil, fmt.Errorf("decision line %d: %w", lineNumber, err)
		}
		if _, ok := usedOld[oldIndex]; ok {
			return nil, fmt.Errorf("decision line %d: old node %q is already paired", lineNumber, decision.Old)
		}
		if _, ok := usedNew[newIndex]; ok {
			return nil, fmt.Errorf("decision line %d: new node %q is already paired", lineNumber, decision.New)
		}
		usedOld[oldIndex] = struct{}{}
		usedNew[newIndex] = struct{}{}
		matches = append(matches, compareNodeMatch{
			OldIndex:  oldIndex,
			NewIndex:  newIndex,
			MatchedBy: "decision:pair",
			Score:     100,
			Reasons:   []string{"decision"},
		})
	}
	return matches, nil
}

func compareResolveDecisionNode(nodes []compareSnapshotNode, side string, ref string, fingerprint string) (int, error) {
	ref = strings.TrimSpace(ref)
	fingerprint = strings.TrimSpace(fingerprint)
	if compareDecisionUnknownValue(ref) {
		ref = ""
	}
	if compareDecisionUnknownValue(fingerprint) {
		fingerprint = ""
	}
	if ref == "" && fingerprint == "" {
		return -1, fmt.Errorf("%s decision requires ref or fingerprint", side)
	}
	if ref != "" {
		for index, node := range nodes {
			if strings.TrimSpace(node.Ref) != ref {
				continue
			}
			if fingerprint != "" && strings.TrimSpace(node.Fingerprint) != fingerprint {
				return -1, fmt.Errorf("%s decision %q fingerprint mismatch", side, ref)
			}
			return index, nil
		}
	}
	if fingerprint == "" {
		return -1, fmt.Errorf("%s decision ref %q was not found", side, ref)
	}
	matches := make([]int, 0, 1)
	for index, node := range nodes {
		if strings.TrimSpace(node.Fingerprint) == fingerprint {
			matches = append(matches, index)
		}
	}
	switch len(matches) {
	case 0:
		if ref != "" {
			return -1, fmt.Errorf("%s decision ref %q and fingerprint were not found", side, ref)
		}
		return -1, fmt.Errorf("%s decision fingerprint was not found", side)
	case 1:
		return matches[0], nil
	default:
		return -1, fmt.Errorf("%s decision fingerprint matched %d nodes; include a current ref", side, len(matches))
	}
}

func compareDecisionKindSupported(kind string) bool {
	switch kind {
	case "pair", "removed", "added", "accepted_removed", "accepted_added", "regression_removed", "unexpected_added", "unknown", "pattern", "severity":
		return true
	default:
		return false
	}
}

func compareDecisionLineNumber(decision compareDecision, index int) int {
	if decision.Line > 0 {
		return decision.Line
	}
	return index + 1
}

func normalizeCompareDecisionToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func compareDecisionUnknownValue(value string) bool {
	value = normalizeCompareDecisionToken(value)
	return value == "" || value == "?" || value == "unknown" || value == "null"
}
