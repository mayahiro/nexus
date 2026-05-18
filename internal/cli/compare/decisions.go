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
	SchemaVersion  int      `json:"schema_version,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Old            string   `json:"old,omitempty"`
	New            string   `json:"new,omitempty"`
	OldFingerprint string   `json:"old_fingerprint,omitempty"`
	NewFingerprint string   `json:"new_fingerprint,omitempty"`
	FindingID      string   `json:"finding_id,omitempty"`
	Confidence     string   `json:"confidence,omitempty"`
	MatchKind      string   `json:"match_kind,omitempty"`
	Count          int      `json:"count,omitempty"`
	Reason         string   `json:"reason,omitempty"`
	Note           string   `json:"note,omitempty"`
	DecidedBy      string   `json:"decided_by,omitempty"`
	DecidedAt      string   `json:"decided_at,omitempty"`
	Context        string   `json:"context,omitempty"`
	SessionContext string   `json:"session_context,omitempty"`
	Name           string   `json:"name,omitempty"`
	Matches        []string `json:"matches,omitempty"`
	From           string   `json:"from,omitempty"`
	To             string   `json:"to,omitempty"`
	Line           int      `json:"-"`
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

type compareFindingDecisionEffects struct {
	ByID map[string]compareDecisionEffect
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
		decision.MatchKind = normalizeCompareDecisionMatchKind(decision.MatchKind)
		if decision.Confidence != "" && !compareDecisionConfidenceSupported(decision.Confidence) {
			return nil, fmt.Errorf("invalid decisions file %q line %d: unsupported confidence %q", path, lineNumber, decision.Confidence)
		}
		if decision.Count < 0 {
			return nil, fmt.Errorf("invalid decisions file %q line %d: count must be non-negative", path, lineNumber)
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

func writeCompareDecisionJSONL(path string, decisions []compareDecision) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return printCompareDecisionJSONL(file, decisions)
}

func printCompareDecisionJSONL(w io.Writer, decisions []compareDecision) error {
	encoder := json.NewEncoder(w)
	for _, decision := range decisions {
		decision.Line = 0
		if err := encoder.Encode(decision); err != nil {
			return err
		}
	}
	return nil
}

func normalizeCompareDecisions(decisions []compareDecision) ([]compareDecision, int) {
	normalized := make([]compareDecision, 0, len(decisions))
	seen := map[string]struct{}{}
	duplicates := 0
	for _, decision := range decisions {
		decision = normalizeCompareDecision(decision)
		key := compareDecisionDedupeKey(decision)
		if _, ok := seen[key]; ok {
			duplicates++
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, decision)
	}
	return normalized, duplicates
}

func normalizeCompareDecision(decision compareDecision) compareDecision {
	if decision.SchemaVersion == 0 {
		decision.SchemaVersion = 1
	}
	decision.Kind = normalizeCompareDecisionToken(decision.Kind)
	decision.Old = strings.TrimSpace(decision.Old)
	decision.New = strings.TrimSpace(decision.New)
	decision.OldFingerprint = strings.TrimSpace(decision.OldFingerprint)
	decision.NewFingerprint = strings.TrimSpace(decision.NewFingerprint)
	decision.FindingID = strings.TrimSpace(decision.FindingID)
	decision.Confidence = normalizeCompareDecisionToken(decision.Confidence)
	decision.MatchKind = normalizeCompareDecisionMatchKind(decision.MatchKind)
	decision.Reason = strings.TrimSpace(decision.Reason)
	decision.Note = strings.TrimSpace(decision.Note)
	decision.DecidedBy = strings.TrimSpace(decision.DecidedBy)
	decision.DecidedAt = strings.TrimSpace(decision.DecidedAt)
	decision.Context = strings.TrimSpace(decision.Context)
	decision.SessionContext = strings.TrimSpace(decision.SessionContext)
	decision.Name = strings.TrimSpace(decision.Name)
	decision.Matches = normalizeCompareDecisionStrings(decision.Matches)
	decision.From = strings.TrimSpace(decision.From)
	decision.To = strings.TrimSpace(decision.To)
	decision.Line = 0
	return decision
}

func normalizeCompareDecisionStrings(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func compareDecisionDedupeKey(decision compareDecision) string {
	confidence := compareDecisionEffectiveConfidence(decision)
	switch decision.Kind {
	case "pair":
		return strings.Join([]string{decision.Kind, decision.Old, decision.New, decision.OldFingerprint, decision.NewFingerprint, confidence}, "\x1f")
	case "subtree_pair":
		return strings.Join([]string{decision.Kind, decision.Old, decision.New, decision.OldFingerprint, decision.NewFingerprint, confidence, decision.MatchKind, fmt.Sprint(decision.Count)}, "\x1f")
	case "accepted_removed", "regression_removed":
		return strings.Join([]string{decision.Kind, decision.Old, decision.OldFingerprint, confidence}, "\x1f")
	case "accepted_added", "unexpected_added":
		return strings.Join([]string{decision.Kind, decision.New, decision.NewFingerprint, confidence}, "\x1f")
	case "accepted_finding", "regression_finding":
		return strings.Join([]string{decision.Kind, decision.FindingID, confidence}, "\x1f")
	case "pattern":
		return strings.Join(append([]string{decision.Kind, decision.Name}, decision.Matches...), "\x1f")
	case "severity":
		return strings.Join([]string{decision.Kind, decision.FindingID, decision.Name, decision.From, decision.To}, "\x1f")
	default:
		bytes, err := json.Marshal(decision)
		if err != nil {
			return fmt.Sprintf("%+v", decision)
		}
		return string(bytes)
	}
}

func compareDecisionEffectiveConfidence(decision compareDecision) string {
	if decision.Confidence != "" {
		return decision.Confidence
	}
	switch decision.Kind {
	case "accepted_removed", "regression_removed", "accepted_added", "unexpected_added", "accepted_finding", "regression_finding":
		return "high"
	default:
		return ""
	}
}

func auditCompareDecisions(decisions []compareDecision, compareReport compareReport) compareDecisionAuditReport {
	report := compareDecisionAuditReport{
		Summary: compareDecisionAuditSummary{
			TotalDecisions:  len(decisions),
			CompareJSONUsed: true,
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

	normalized := make([]compareDecision, 0, len(decisions))
	for _, decision := range decisions {
		line := decision.Line
		decision = normalizeCompareDecision(decision)
		decision.Line = line
		normalized = append(normalized, decision)
	}

	report.Summary.Conflicts = auditCompareDecisionConflicts(normalized, addIssue)
	for index, decision := range normalized {
		switch decision.Kind {
		case "pair", "subtree_pair":
			auditComparePairLikeDecision(&report, addIssue, decision, index, compareReport)
		case "accepted_removed", "regression_removed":
			auditCompareOldEffectDecision(&report, addIssue, decision, index, compareReport)
		case "accepted_added", "unexpected_added":
			auditCompareNewEffectDecision(&report, addIssue, decision, index, compareReport)
		case "accepted_finding", "regression_finding":
			auditCompareFindingEffectDecision(&report, addIssue, decision, index, compareReport)
		default:
			report.Summary.Pending++
		}
	}
	return report
}

type compareAuditConflictSeen struct {
	Line int
	Key  string
}

func auditCompareDecisionConflicts(decisions []compareDecision, addIssue func(string, compareDecision, int, string, string)) int {
	seen := map[string]compareAuditConflictSeen{}
	conflicts := 0
	for index, decision := range decisions {
		if !compareAuditDecisionCanApply(decision) {
			continue
		}
		dedupeKey := compareDecisionDedupeKey(decision)
		for _, target := range compareAuditDecisionConflictTargets(decision) {
			if previous, ok := seen[target]; ok {
				if previous.Key == dedupeKey {
					continue
				}
				conflicts++
				addIssue("error", decision, index, "decision", fmt.Sprintf("decision target conflicts with line %d", previous.Line))
				continue
			}
			seen[target] = compareAuditConflictSeen{
				Line: compareDecisionLineNumber(decision, index),
				Key:  dedupeKey,
			}
		}
	}
	return conflicts
}

func compareAuditDecisionConflictTargets(decision compareDecision) []string {
	switch decision.Kind {
	case "pair", "subtree_pair":
		targets := make([]string, 0, 2)
		if target := compareAuditDecisionTarget("old_pair", decision.Old, decision.OldFingerprint); target != "" {
			targets = append(targets, target)
		}
		if target := compareAuditDecisionTarget("new_pair", decision.New, decision.NewFingerprint); target != "" {
			targets = append(targets, target)
		}
		return targets
	case "accepted_removed", "regression_removed":
		if target := compareAuditDecisionTarget("old_effect", decision.Old, decision.OldFingerprint); target != "" {
			return []string{target}
		}
		return nil
	case "accepted_added", "unexpected_added":
		if target := compareAuditDecisionTarget("new_effect", decision.New, decision.NewFingerprint); target != "" {
			return []string{target}
		}
		return nil
	case "accepted_finding", "regression_finding":
		if strings.TrimSpace(decision.FindingID) == "" {
			return nil
		}
		return []string{"finding:" + strings.TrimSpace(decision.FindingID)}
	default:
		return nil
	}
}

func compareAuditDecisionTarget(prefix string, ref string, fingerprint string) string {
	ref = strings.TrimSpace(ref)
	fingerprint = strings.TrimSpace(fingerprint)
	if !compareDecisionUnknownValue(ref) {
		return prefix + ":ref:" + ref
	}
	if fingerprint != "" {
		return prefix + ":fingerprint:" + fingerprint
	}
	return ""
}

func compareAuditDecisionCanApply(decision compareDecision) bool {
	switch decision.Kind {
	case "pair", "subtree_pair":
		return decision.Confidence == "high"
	case "accepted_removed", "regression_removed", "accepted_added", "unexpected_added", "accepted_finding", "regression_finding":
		return compareDecisionApplies(decision)
	default:
		return false
	}
}

func auditComparePairLikeDecision(report *compareDecisionAuditReport, addIssue func(string, compareDecision, int, string, string), decision compareDecision, index int, compareReport compareReport) {
	if decision.Confidence != "high" {
		if decision.Confidence == "" {
			addIssue("error", decision, index, "confidence", decision.Kind+" decisions require confidence high, tentative, or unknown")
			return
		}
		report.Summary.Pending++
		return
	}
	if compareReport.MatchingDebug == nil {
		report.Summary.Stale++
		addIssue("warning", decision, index, "matching_debug", "matching_debug is required to verify pair decision application")
		return
	}
	var matches []compareNodeMatch
	var err error
	switch decision.Kind {
	case "pair":
		matches, err = compareBuildPairDecisionMatches(decision, compareReport.Old.Nodes, compareReport.New.Nodes)
	case "subtree_pair":
		matches, err = compareBuildSubtreePairDecisionMatches(decision, compareReport.Old.Nodes, compareReport.New.Nodes)
	}
	if err != nil {
		report.Summary.Stale++
		addIssue("warning", decision, index, decision.Kind, err.Error())
		return
	}
	for _, match := range matches {
		if !compareMatchingDebugHasDecisionMatch(compareReport.MatchingDebug, match) {
			report.Summary.Stale++
			addIssue("warning", decision, index, decision.Kind, "decision match was not observed in matching_debug")
			return
		}
	}
	report.Summary.Applied++
}

func compareMatchingDebugHasDecisionMatch(debug *compareMatchingDebug, match compareNodeMatch) bool {
	if debug == nil {
		return false
	}
	for _, debugMatch := range debug.Matches {
		if debugMatch.Old.Index == match.OldIndex && debugMatch.New.Index == match.NewIndex && strings.TrimSpace(debugMatch.MatchedBy) == match.MatchedBy {
			return true
		}
	}
	return false
}

func auditCompareOldEffectDecision(report *compareDecisionAuditReport, addIssue func(string, compareDecision, int, string, string), decision compareDecision, index int, compareReport compareReport) {
	if !compareDecisionApplies(decision) {
		report.Summary.Pending++
		return
	}
	oldIndex, err := compareResolveDecisionNode(compareReport.Old.Nodes, "old", decision.Old, decision.OldFingerprint)
	if err != nil {
		report.Summary.Stale++
		addIssue("warning", decision, index, "old", err.Error())
		return
	}
	if compareReportHasNodeDecisionFinding(compareReport.Findings, "missing_node", decision.Kind, compareReport.Old.Nodes[oldIndex]) {
		report.Summary.Applied++
		return
	}
	report.Summary.Stale++
	addIssue("warning", decision, index, "old", "decision was not observed on a current missing_node finding")
}

func auditCompareNewEffectDecision(report *compareDecisionAuditReport, addIssue func(string, compareDecision, int, string, string), decision compareDecision, index int, compareReport compareReport) {
	if !compareDecisionApplies(decision) {
		report.Summary.Pending++
		return
	}
	newIndex, err := compareResolveDecisionNode(compareReport.New.Nodes, "new", decision.New, decision.NewFingerprint)
	if err != nil {
		report.Summary.Stale++
		addIssue("warning", decision, index, "new", err.Error())
		return
	}
	if compareReportHasNodeDecisionFinding(compareReport.Findings, "new_node", decision.Kind, compareReport.New.Nodes[newIndex]) {
		report.Summary.Applied++
		return
	}
	report.Summary.Stale++
	addIssue("warning", decision, index, "new", "decision was not observed on a current new_node finding")
}

func compareReportHasNodeDecisionFinding(findings []compareFinding, kind string, decisionKind string, node compareSnapshotNode) bool {
	for _, finding := range findings {
		if strings.TrimSpace(finding.Kind) != kind || strings.TrimSpace(finding.DecisionKind) != decisionKind {
			continue
		}
		if !compareFindingMatchesNode(finding, node) {
			continue
		}
		return true
	}
	return false
}

func compareFindingMatchesNode(finding compareFinding, node compareSnapshotNode) bool {
	if finding.Fingerprint != "" && node.Fingerprint != "" && finding.Fingerprint != node.Fingerprint {
		return false
	}
	if finding.Role != "" && node.Role != "" && finding.Role != node.Role {
		return false
	}
	if finding.Label != "" && node.Label != "" && finding.Label != node.Label {
		return false
	}
	return true
}

func auditCompareFindingEffectDecision(report *compareDecisionAuditReport, addIssue func(string, compareDecision, int, string, string), decision compareDecision, index int, compareReport compareReport) {
	if !compareDecisionApplies(decision) {
		report.Summary.Pending++
		return
	}
	findingID := strings.TrimSpace(decision.FindingID)
	if compareDecisionUnknownValue(findingID) {
		report.Summary.Stale++
		addIssue("warning", decision, index, "finding_id", "finding decision requires finding_id")
		return
	}
	for _, finding := range compareReport.Findings {
		if strings.TrimSpace(finding.FindingID) != findingID {
			continue
		}
		if strings.TrimSpace(finding.DecisionKind) == decision.Kind {
			report.Summary.Applied++
			return
		}
		report.Summary.Stale++
		addIssue("warning", decision, index, "finding_id", "finding exists but decision_kind was not observed")
		return
	}
	report.Summary.Stale++
	addIssue("warning", decision, index, "finding_id", fmt.Sprintf("finding_id %q was not found in compare report", findingID))
}

func compareResolveDecisionMatches(decisions []compareDecision, oldNodes []compareSnapshotNode, newNodes []compareSnapshotNode) ([]compareNodeMatch, error) {
	if len(decisions) == 0 {
		return nil, nil
	}
	matches := make([]compareNodeMatch, 0)
	usedOld := map[int]struct{}{}
	usedNew := map[int]struct{}{}
	for index, decision := range decisions {
		lineNumber := compareDecisionLineNumber(decision, index)
		if decision.Confidence != "high" {
			continue
		}
		var decisionMatches []compareNodeMatch
		var err error
		switch decision.Kind {
		case "pair":
			decisionMatches, err = compareBuildPairDecisionMatches(decision, oldNodes, newNodes)
		case "subtree_pair":
			decisionMatches, err = compareBuildSubtreePairDecisionMatches(decision, oldNodes, newNodes)
		default:
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("decision line %d: %w", lineNumber, err)
		}
		for _, match := range decisionMatches {
			if _, ok := usedOld[match.OldIndex]; ok {
				return nil, fmt.Errorf("decision line %d: old node %q is already paired", lineNumber, oldNodes[match.OldIndex].Ref)
			}
			if _, ok := usedNew[match.NewIndex]; ok {
				return nil, fmt.Errorf("decision line %d: new node %q is already paired", lineNumber, newNodes[match.NewIndex].Ref)
			}
			usedOld[match.OldIndex] = struct{}{}
			usedNew[match.NewIndex] = struct{}{}
			matches = append(matches, match)
		}
	}
	return matches, nil
}

func compareResolvePairDecisionMatches(decisions []compareDecision, oldNodes []compareSnapshotNode, newNodes []compareSnapshotNode) ([]compareNodeMatch, error) {
	return compareResolveDecisionMatches(decisions, oldNodes, newNodes)
}

func compareBuildPairDecisionMatches(decision compareDecision, oldNodes []compareSnapshotNode, newNodes []compareSnapshotNode) ([]compareNodeMatch, error) {
	oldIndex, err := compareResolveDecisionNode(oldNodes, "old", decision.Old, decision.OldFingerprint)
	if err != nil {
		return nil, err
	}
	newIndex, err := compareResolveDecisionNode(newNodes, "new", decision.New, decision.NewFingerprint)
	if err != nil {
		return nil, err
	}
	return []compareNodeMatch{{
		OldIndex:  oldIndex,
		NewIndex:  newIndex,
		MatchedBy: "decision:pair",
		Score:     100,
		Reasons:   []string{"decision"},
	}}, nil
}

func compareBuildSubtreePairDecisionMatches(decision compareDecision, oldNodes []compareSnapshotNode, newNodes []compareSnapshotNode) ([]compareNodeMatch, error) {
	if normalizeCompareDecisionMatchKind(decision.MatchKind) != "ordered_children" {
		return nil, fmt.Errorf("subtree_pair decision requires match_kind ordered_children")
	}
	oldIndex, err := compareResolveDecisionNode(oldNodes, "old", decision.Old, decision.OldFingerprint)
	if err != nil {
		return nil, err
	}
	newIndex, err := compareResolveDecisionNode(newNodes, "new", decision.New, decision.NewFingerprint)
	if err != nil {
		return nil, err
	}
	oldChildren := compareDecisionChildNodeIndices(oldNodes, oldIndex)
	newChildren := compareDecisionChildNodeIndices(newNodes, newIndex)
	childPairs := min(len(oldChildren), len(newChildren))
	if decision.Count > 0 && decision.Count != childPairs {
		return nil, fmt.Errorf("subtree_pair count %d does not match %d ordered child pairs", decision.Count, childPairs)
	}
	matches := []compareNodeMatch{{
		OldIndex:  oldIndex,
		NewIndex:  newIndex,
		MatchedBy: "decision:subtree_pair",
		Score:     100,
		Reasons:   []string{"decision", "subtree-pair"},
	}}
	for i := 0; i < childPairs; i++ {
		matches = append(matches, compareNodeMatch{
			OldIndex:  oldChildren[i],
			NewIndex:  newChildren[i],
			MatchedBy: "decision:subtree_pair",
			Score:     100,
			Reasons:   []string{"decision", "subtree-pair", "ordered-children"},
		})
	}
	return matches, nil
}

func compareDecisionChildNodeIndices(nodes []compareSnapshotNode, rootIndex int) []int {
	if rootIndex < 0 || rootIndex >= len(nodes) {
		return nil
	}
	byID := map[int]int{}
	for index, node := range nodes {
		if node.ID <= 0 {
			continue
		}
		byID[node.ID] = index
	}
	indices := make([]int, 0, len(nodes[rootIndex].Children))
	for _, childID := range nodes[rootIndex].Children {
		childIndex, ok := byID[childID]
		if !ok {
			continue
		}
		indices = append(indices, childIndex)
	}
	compareSortNodeIndicesBySequence(nodes, indices)
	return indices
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

func compareResolveFindingDecisionEffects(decisions []compareDecision) (compareFindingDecisionEffects, error) {
	effects := compareFindingDecisionEffects{
		ByID: map[string]compareDecisionEffect{},
	}
	for index, decision := range decisions {
		lineNumber := compareDecisionLineNumber(decision, index)
		if !compareDecisionApplies(decision) {
			continue
		}
		switch decision.Kind {
		case "accepted_finding", "regression_finding":
			findingID := strings.TrimSpace(decision.FindingID)
			if compareDecisionUnknownValue(findingID) {
				return compareFindingDecisionEffects{}, fmt.Errorf("decision line %d: finding decision requires finding_id", lineNumber)
			}
			if _, ok := effects.ByID[findingID]; ok {
				return compareFindingDecisionEffects{}, fmt.Errorf("decision line %d: finding %q already has a decision effect", lineNumber, findingID)
			}
			effects.ByID[findingID] = compareDecisionEffectFor(decision.Kind)
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
	case "accepted_finding":
		effect.Severity = "info"
		effect.Impact = "accepted_finding"
	case "regression_finding":
		effect.Severity = "critical"
		effect.Impact = "regression_finding"
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
	case "pair", "subtree_pair", "accepted_removed", "accepted_added", "regression_removed", "unexpected_added", "accepted_finding", "regression_finding", "unknown", "pattern", "severity":
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
	usedFindingEffects := map[string]int{}

	for index, decision := range decisions {
		decision.Kind = normalizeCompareDecisionToken(decision.Kind)
		decision.Confidence = normalizeCompareDecisionToken(decision.Confidence)
		decision.MatchKind = normalizeCompareDecisionMatchKind(decision.MatchKind)
		if decision.Kind == "" {
			addIssue("error", decision, index, "kind", "decision kind is required")
			continue
		}
		if !compareDecisionKindSupported(decision.Kind) {
			addIssue("error", decision, index, "kind", fmt.Sprintf("unsupported decision kind %q", decision.Kind))
			continue
		}
		if decision.Confidence != "" && !compareDecisionConfidenceSupported(decision.Confidence) {
			addIssue("error", decision, index, "confidence", fmt.Sprintf("unsupported confidence %q", decision.Confidence))
			continue
		}
		switch decision.Kind {
		case "pair":
			validateComparePairDecision(&report, addIssue, usedOldPairs, usedNewPairs, decision, index, compareReport)
		case "subtree_pair":
			validateCompareSubtreePairDecision(&report, addIssue, usedOldPairs, usedNewPairs, decision, index, compareReport)
		case "accepted_removed", "regression_removed":
			validateCompareOldDecision(&report, addIssue, usedOldEffects, decision, index, compareReport)
		case "accepted_added", "unexpected_added":
			validateCompareNewDecision(&report, addIssue, usedNewEffects, decision, index, compareReport)
		case "accepted_finding", "regression_finding":
			validateCompareFindingDecision(&report, addIssue, usedFindingEffects, decision, index, compareReport)
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

func validateCompareSubtreePairDecision(report *compareDecisionValidationReport, addIssue func(string, compareDecision, int, string, string), usedOld map[int]int, usedNew map[int]int, decision compareDecision, index int, compareReport *compareReport) {
	report.Summary.SubtreePairs++
	switch decision.Confidence {
	case "high", "tentative", "unknown":
	default:
		addIssue("error", decision, index, "confidence", "subtree_pair decisions require confidence high, tentative, or unknown")
	}
	if decision.MatchKind != "ordered_children" {
		addIssue("error", decision, index, "match_kind", "subtree_pair decisions require match_kind ordered_children")
	}
	if decision.Count < 0 {
		addIssue("error", decision, index, "count", "subtree_pair count must be non-negative")
	}
	if compareDecisionUnknownValue(decision.Old) && strings.TrimSpace(decision.OldFingerprint) == "" {
		addIssue("error", decision, index, "old", "subtree_pair decisions require old ref or old_fingerprint")
	}
	if compareDecisionUnknownValue(decision.New) && strings.TrimSpace(decision.NewFingerprint) == "" && decision.Confidence != "unknown" {
		addIssue("error", decision, index, "new", "subtree_pair decisions require new ref, new_fingerprint, or confidence unknown")
	}
	if decision.Confidence == "high" && compareDecisionUnknownValue(decision.New) && strings.TrimSpace(decision.NewFingerprint) == "" {
		addIssue("error", decision, index, "new", "high-confidence subtree_pair decisions require a concrete new ref or new_fingerprint")
	}
	if decision.Confidence == "high" && compareDecisionUnknownValue(decision.Old) && strings.TrimSpace(decision.OldFingerprint) == "" {
		addIssue("error", decision, index, "old", "high-confidence subtree_pair decisions require a concrete old ref or old_fingerprint")
	}
	if compareReport == nil || decision.Confidence != "high" {
		return
	}
	matches, err := compareBuildSubtreePairDecisionMatches(decision, compareReport.Old.Nodes, compareReport.New.Nodes)
	if err != nil {
		addIssue("error", decision, index, "subtree_pair", err.Error())
		return
	}
	lineNumber := compareDecisionLineNumber(decision, index)
	for _, match := range matches {
		if previousLine, ok := usedOld[match.OldIndex]; ok {
			addIssue("error", decision, index, "old", fmt.Sprintf("old node already paired by decision line %d", previousLine))
			continue
		}
		if previousLine, ok := usedNew[match.NewIndex]; ok {
			addIssue("error", decision, index, "new", fmt.Sprintf("new node already paired by decision line %d", previousLine))
			continue
		}
		usedOld[match.OldIndex] = lineNumber
		usedNew[match.NewIndex] = lineNumber
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

func validateCompareFindingDecision(report *compareDecisionValidationReport, addIssue func(string, compareDecision, int, string, string), used map[string]int, decision compareDecision, index int, compareReport *compareReport) {
	if decision.Kind == "accepted_finding" {
		report.Summary.AcceptedFindings++
	}
	if decision.Kind == "regression_finding" {
		report.Summary.RegressionFindings++
	}
	findingID := strings.TrimSpace(decision.FindingID)
	if compareDecisionUnknownValue(findingID) {
		addIssue("error", decision, index, "finding_id", "finding decisions require finding_id")
		return
	}
	if compareReport == nil || !compareDecisionApplies(decision) {
		return
	}
	if !compareReportHasFindingID(compareReport, findingID) {
		addIssue("error", decision, index, "finding_id", fmt.Sprintf("finding_id %q was not found in compare report", findingID))
		return
	}
	if previousLine, ok := used[findingID]; ok {
		addIssue("error", decision, index, "finding_id", fmt.Sprintf("finding already has a decision effect from line %d", previousLine))
		return
	}
	used[findingID] = compareDecisionLineNumber(decision, index)
}

func compareReportHasFindingID(report *compareReport, findingID string) bool {
	for _, finding := range report.Findings {
		if strings.TrimSpace(finding.FindingID) == findingID {
			return true
		}
	}
	return false
}

func writeCompareDecisionsTemplate(path string, debug *compareMatchingDebug) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return printCompareDecisionsTemplate(file, debug)
}

func writeCompareFindingDecisionsTemplate(path string, report compareReport) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return printCompareFindingDecisionsTemplate(file, report)
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

func printCompareFindingDecisionsTemplate(w io.Writer, report compareReport) error {
	encoder := json.NewEncoder(w)
	for _, finding := range report.Findings {
		if !compareFindingNeedsDecisionTemplate(finding) {
			continue
		}
		decision := compareDecision{
			SchemaVersion: 1,
			Kind:          compareFindingDecisionTemplateKind(finding),
			FindingID:     strings.TrimSpace(finding.FindingID),
			Confidence:    "unknown",
			Reason:        "review finding",
			Note:          compareFindingDecisionTemplateNote(finding),
		}
		if err := encoder.Encode(decision); err != nil {
			return err
		}
	}
	return nil
}

func compareFindingNeedsDecisionTemplate(finding compareFinding) bool {
	if strings.TrimSpace(finding.FindingID) == "" {
		return false
	}
	switch strings.TrimSpace(finding.Severity) {
	case "critical", "warning":
		return true
	default:
		return false
	}
}

func compareFindingDecisionTemplateKind(finding compareFinding) string {
	if strings.TrimSpace(finding.Severity) == "critical" {
		return "regression_finding"
	}
	return "accepted_finding"
}

func compareFindingDecisionTemplateNote(finding compareFinding) string {
	values := []string{
		strings.TrimSpace(finding.Kind),
		strings.TrimSpace(finding.Severity),
		strings.TrimSpace(finding.Impact),
		strings.TrimSpace(finding.Locator),
		strings.TrimSpace(finding.Role),
		strings.TrimSpace(finding.Label),
		strings.TrimSpace(finding.Field),
	}
	return strings.Join(compactCompareDecisionTemplateValues(values), " | ")
}

func compactCompareDecisionTemplateValues(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.Join(strings.Fields(value), " ")
		if value == "" {
			continue
		}
		result = append(result, value)
	}
	return result
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

func normalizeCompareDecisionMatchKind(value string) string {
	return strings.ReplaceAll(normalizeCompareDecisionToken(value), "-", "_")
}

func compareDecisionUnknownValue(value string) bool {
	value = normalizeCompareDecisionToken(value)
	return value == "" || value == "?" || value == "unknown" || value == "null"
}
