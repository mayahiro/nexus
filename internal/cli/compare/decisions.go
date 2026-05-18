package comparecmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
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
	Note           string `json:"note,omitempty"`
	DecidedBy      string `json:"decided_by,omitempty"`
	DecidedAt      string `json:"decided_at,omitempty"`
	Context        string `json:"context,omitempty"`
	Line           int    `json:"-"`
}

type compareDecisionEffect struct {
	Kind      string
	MatchedBy string
	Impact    string
	Severity  string
	Reasons   []string
}

type compareDecisionEffects struct {
	Old map[int]compareDecisionEffect
	New map[int]compareDecisionEffect
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
		if decision.Confidence != "" && !compareDecisionConfidenceSupported(decision.Confidence) {
			return nil, fmt.Errorf("invalid decisions file %q line %d: unsupported confidence %q", path, lineNumber, decision.Confidence)
		}
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

func loadCompareReport(path string) (compareReport, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return compareReport{}, nil
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return compareReport{}, err
	}
	var report compareReport
	if err := json.Unmarshal(bytes, &report); err != nil {
		return compareReport{}, fmt.Errorf("invalid compare json %q: %w", path, err)
	}
	return report, nil
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

func compareResolveDecisionEffects(decisions []compareDecision, oldNodes []compareSnapshotNode, newNodes []compareSnapshotNode) (compareDecisionEffects, error) {
	effects := compareDecisionEffects{
		Old: map[int]compareDecisionEffect{},
		New: map[int]compareDecisionEffect{},
	}
	for index, decision := range decisions {
		lineNumber := compareDecisionLineNumber(decision, index)
		if !compareDecisionApplies(decision) {
			continue
		}
		switch decision.Kind {
		case "accepted_removed", "regression_removed":
			oldIndex, err := compareResolveDecisionNode(oldNodes, "old", decision.Old, decision.OldFingerprint)
			if err != nil {
				return compareDecisionEffects{}, fmt.Errorf("decision line %d: %w", lineNumber, err)
			}
			if _, ok := effects.Old[oldIndex]; ok {
				return compareDecisionEffects{}, fmt.Errorf("decision line %d: old node %q already has a decision effect", lineNumber, decision.Old)
			}
			effects.Old[oldIndex] = compareDecisionEffectFor(decision.Kind)
		case "accepted_added", "unexpected_added":
			newIndex, err := compareResolveDecisionNode(newNodes, "new", decision.New, decision.NewFingerprint)
			if err != nil {
				return compareDecisionEffects{}, fmt.Errorf("decision line %d: %w", lineNumber, err)
			}
			if _, ok := effects.New[newIndex]; ok {
				return compareDecisionEffects{}, fmt.Errorf("decision line %d: new node %q already has a decision effect", lineNumber, decision.New)
			}
			effects.New[newIndex] = compareDecisionEffectFor(decision.Kind)
		}
	}
	return effects, nil
}

func compareDecisionEffectFor(kind string) compareDecisionEffect {
	effect := compareDecisionEffect{
		Kind:      kind,
		MatchedBy: "decision:" + kind,
		Reasons:   []string{"decision"},
	}
	switch kind {
	case "accepted_removed":
		effect.Severity = "info"
		effect.Impact = "accepted_removed"
	case "accepted_added":
		effect.Severity = "info"
		effect.Impact = "accepted_added"
	case "regression_removed":
		effect.Severity = "critical"
		effect.Impact = "regression_removed"
	case "unexpected_added":
		effect.Severity = "warning"
		effect.Impact = "unexpected_added"
	}
	return effect
}

func compareDecisionApplies(decision compareDecision) bool {
	switch decision.Confidence {
	case "", "high":
		return true
	default:
		return false
	}
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

func compareDecisionConfidenceSupported(confidence string) bool {
	switch confidence {
	case "high", "tentative", "unknown":
		return true
	default:
		return false
	}
}

func validateCompareDecisions(decisions []compareDecision, compareReport *compareReport) compareDecisionValidationReport {
	report := compareDecisionValidationReport{
		Summary: compareDecisionValidationSummary{
			TotalDecisions:  len(decisions),
			CompareJSONUsed: compareReport != nil,
		},
	}
	addIssue := func(severity string, decision compareDecision, index int, field string, message string) {
		report.Issues = append(report.Issues, compareDecisionValidationIssue{
			Severity: severity,
			Line:     compareDecisionLineNumber(decision, index),
			Field:    field,
			Message:  message,
		})
		if severity == "error" {
			report.Summary.Errors++
			return
		}
		report.Summary.Warnings++
	}

	usedOldPairs := map[int]int{}
	usedNewPairs := map[int]int{}
	usedOldEffects := map[int]int{}
	usedNewEffects := map[int]int{}

	for index, decision := range decisions {
		switch decision.Kind {
		case "pair":
			validateComparePairDecision(&report, addIssue, usedOldPairs, usedNewPairs, decision, index, compareReport)
		case "accepted_removed", "regression_removed", "removed":
			validateCompareOldDecision(&report, addIssue, usedOldEffects, decision, index, compareReport)
		case "accepted_added", "unexpected_added", "added":
			validateCompareNewDecision(&report, addIssue, usedNewEffects, decision, index, compareReport)
		case "pattern":
			if strings.TrimSpace(decision.Context) == "" && strings.TrimSpace(decision.Note) == "" && strings.TrimSpace(decision.Reason) == "" {
				addIssue("warning", decision, index, "reason", "pattern decisions should include reason, note, or context")
			}
		case "severity":
			if strings.TrimSpace(decision.Reason) == "" {
				addIssue("warning", decision, index, "reason", "severity decisions should include a reason")
			}
		}
	}
	return report
}

func validateComparePairDecision(report *compareDecisionValidationReport, addIssue func(string, compareDecision, int, string, string), usedOld map[int]int, usedNew map[int]int, decision compareDecision, index int, compareReport *compareReport) {
	switch decision.Confidence {
	case "high":
		report.Summary.HighPairs++
	case "tentative":
		report.Summary.TentativePairs++
	case "unknown":
		report.Summary.UnknownPairs++
	default:
		addIssue("error", decision, index, "confidence", "pair decisions require confidence high, tentative, or unknown")
	}
	if compareDecisionUnknownValue(decision.Old) && strings.TrimSpace(decision.OldFingerprint) == "" {
		addIssue("error", decision, index, "old", "pair decisions require old ref or old_fingerprint")
	}
	if compareDecisionUnknownValue(decision.New) && strings.TrimSpace(decision.NewFingerprint) == "" && decision.Confidence != "unknown" {
		addIssue("error", decision, index, "new", "pair decisions require new ref, new_fingerprint, or confidence unknown")
	}
	if decision.Confidence == "high" && compareDecisionUnknownValue(decision.New) && strings.TrimSpace(decision.NewFingerprint) == "" {
		addIssue("error", decision, index, "new", "high-confidence pair decisions require a concrete new ref or new_fingerprint")
	}
	if decision.Confidence == "high" && compareDecisionUnknownValue(decision.Old) && strings.TrimSpace(decision.OldFingerprint) == "" {
		addIssue("error", decision, index, "old", "high-confidence pair decisions require a concrete old ref or old_fingerprint")
	}
	if compareReport == nil || decision.Confidence != "high" {
		return
	}
	oldIndex, err := compareResolveDecisionNode(compareReport.Old.Nodes, "old", decision.Old, decision.OldFingerprint)
	if err != nil {
		addIssue("error", decision, index, "old", err.Error())
	} else if previousLine, ok := usedOld[oldIndex]; ok {
		addIssue("error", decision, index, "old", fmt.Sprintf("old node already paired by decision line %d", previousLine))
	} else {
		usedOld[oldIndex] = compareDecisionLineNumber(decision, index)
	}
	newIndex, err := compareResolveDecisionNode(compareReport.New.Nodes, "new", decision.New, decision.NewFingerprint)
	if err != nil {
		addIssue("error", decision, index, "new", err.Error())
	} else if previousLine, ok := usedNew[newIndex]; ok {
		addIssue("error", decision, index, "new", fmt.Sprintf("new node already paired by decision line %d", previousLine))
	} else {
		usedNew[newIndex] = compareDecisionLineNumber(decision, index)
	}
}

func validateCompareOldDecision(report *compareDecisionValidationReport, addIssue func(string, compareDecision, int, string, string), used map[int]int, decision compareDecision, index int, compareReport *compareReport) {
	if decision.Kind == "accepted_removed" {
		report.Summary.AcceptedRemoved++
	}
	if compareDecisionUnknownValue(decision.Old) && strings.TrimSpace(decision.OldFingerprint) == "" {
		addIssue("error", decision, index, "old", "removed decisions require old ref or old_fingerprint")
		return
	}
	if compareReport == nil || !compareDecisionApplies(decision) {
		return
	}
	oldIndex, err := compareResolveDecisionNode(compareReport.Old.Nodes, "old", decision.Old, decision.OldFingerprint)
	if err != nil {
		addIssue("error", decision, index, "old", err.Error())
		return
	}
	if previousLine, ok := used[oldIndex]; ok {
		addIssue("error", decision, index, "old", fmt.Sprintf("old node already has a decision effect from line %d", previousLine))
		return
	}
	used[oldIndex] = compareDecisionLineNumber(decision, index)
}

func validateCompareNewDecision(report *compareDecisionValidationReport, addIssue func(string, compareDecision, int, string, string), used map[int]int, decision compareDecision, index int, compareReport *compareReport) {
	if decision.Kind == "accepted_added" {
		report.Summary.AcceptedAdded++
	}
	if compareDecisionUnknownValue(decision.New) && strings.TrimSpace(decision.NewFingerprint) == "" {
		addIssue("error", decision, index, "new", "added decisions require new ref or new_fingerprint")
		return
	}
	if compareReport == nil || !compareDecisionApplies(decision) {
		return
	}
	newIndex, err := compareResolveDecisionNode(compareReport.New.Nodes, "new", decision.New, decision.NewFingerprint)
	if err != nil {
		addIssue("error", decision, index, "new", err.Error())
		return
	}
	if previousLine, ok := used[newIndex]; ok {
		addIssue("error", decision, index, "new", fmt.Sprintf("new node already has a decision effect from line %d", previousLine))
		return
	}
	used[newIndex] = compareDecisionLineNumber(decision, index)
}

func writeCompareDecisionsTemplate(path string, debug *compareMatchingDebug) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return printCompareDecisionsTemplate(file, debug)
}

func printCompareDecisionsTemplate(w io.Writer, debug *compareMatchingDebug) error {
	if debug == nil {
		return nil
	}
	encoder := json.NewEncoder(w)
	for _, candidate := range debug.AmbiguousCandidates {
		decision := compareDecision{
			SchemaVersion:  1,
			Kind:           "pair",
			Old:            candidate.Old.Ref,
			OldFingerprint: candidate.Old.Fingerprint,
			New:            "?",
			Confidence:     "unknown",
			Reason:         compareDecisionTemplateReason(candidate),
			Note:           compareDecisionTemplateNote(candidate),
		}
		if decision.Old == "" {
			decision.Old = "?"
		}
		if err := encoder.Encode(decision); err != nil {
			return err
		}
	}
	return nil
}

func compareDecisionTemplateReason(candidate compareMatchingDebugAmbiguousCandidate) string {
	reason := strings.TrimSpace(candidate.ReasonSkipped)
	if reason == "" {
		return "choose the matching new candidate"
	}
	return reason
}

func compareDecisionTemplateNote(candidate compareMatchingDebugAmbiguousCandidate) string {
	refs := make([]string, 0, len(candidate.NewCandidates))
	for _, option := range candidate.NewCandidates {
		if strings.TrimSpace(option.Node.Ref) == "" {
			continue
		}
		refs = append(refs, strings.TrimSpace(option.Node.Ref))
	}
	if len(refs) == 0 {
		return ""
	}
	return "candidate new refs: " + strings.Join(refs, ", ")
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
