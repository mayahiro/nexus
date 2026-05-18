package comparecmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"
)

const compareDecisionMaxLineBytes = 1024 * 1024
const compareDecisionTemplateMaxUnmatchedNodes = 50

type compareDecisionTemplatePlan struct {
	Ambiguous    []compareMatchingDebugAmbiguousCandidate
	UnmatchedOld []compareMatchingDebugNode
	UnmatchedNew []compareMatchingDebugNode
	Counts       compareDecisionTemplateCounts
}

type compareDecision struct {
	SchemaVersion  int      `json:"schema_version,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Old            string   `json:"old,omitempty"`
	New            string   `json:"new,omitempty"`
	OldLocator     string   `json:"old_locator,omitempty"`
	NewLocator     string   `json:"new_locator,omitempty"`
	OldSelector    string   `json:"old_selector,omitempty"`
	NewSelector    string   `json:"new_selector,omitempty"`
	OldFingerprint string   `json:"old_fingerprint,omitempty"`
	NewFingerprint string   `json:"new_fingerprint,omitempty"`
	FindingID      string   `json:"finding_id,omitempty"`
	ClusterKey     string   `json:"cluster_key,omitempty"`
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

func loadCompareFindingClusters(path string) ([]compareFindingCluster, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var summary struct {
		FindingClusters []compareFindingCluster `json:"finding_clusters,omitempty"`
	}
	if err := json.Unmarshal(bytes, &summary); err != nil {
		return nil, fmt.Errorf("invalid review summary %q: %w", path, err)
	}
	return summary.FindingClusters, nil
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

func normalizeCompareDecisionsWithClusters(decisions []compareDecision, compareReport *compareReport, clusters []compareFindingCluster) ([]compareDecision, int) {
	return normalizeCompareDecisions(materializeCompareClusterDecisions(decisions, compareReport, clusters))
}

func materializeCompareDecisionRefs(decisions []compareDecision, compareReport compareReport) ([]compareDecision, []compareDecisionValidationIssue, []compareDecisionMaterializedRef) {
	materialized := make([]compareDecision, 0, len(decisions))
	issues := []compareDecisionValidationIssue{}
	materializedRefs := []compareDecisionMaterializedRef{}

	addIssue := func(decision compareDecision, index int, field string, message string) {
		issues = append(issues, compareDecisionValidationIssue{
			Severity: "error",
			Line:     compareDecisionLineNumber(decision, index),
			Field:    field,
			Message:  message,
		})
	}

	for index, decision := range decisions {
		line := decision.Line
		decision = normalizeCompareDecision(decision)
		decision.Line = line

		switch decision.Kind {
		case "pair", "subtree_pair":
			next, detail, err := materializeCompareDecisionSide(decision, index, compareReport.Old.Nodes, true)
			if err != nil {
				addIssue(decision, index, "old_locator", err.Error())
			} else if detail != nil {
				decision = next
				materializedRefs = append(materializedRefs, *detail)
			}
			next, detail, err = materializeCompareDecisionSide(decision, index, compareReport.New.Nodes, false)
			if err != nil {
				addIssue(decision, index, "new_locator", err.Error())
			} else if detail != nil {
				decision = next
				materializedRefs = append(materializedRefs, *detail)
			}
		case "accepted_removed", "regression_removed":
			next, detail, err := materializeCompareDecisionSide(decision, index, compareReport.Old.Nodes, true)
			if err != nil {
				addIssue(decision, index, "old_locator", err.Error())
			} else if detail != nil {
				decision = next
				materializedRefs = append(materializedRefs, *detail)
			}
		case "accepted_added", "unexpected_added":
			next, detail, err := materializeCompareDecisionSide(decision, index, compareReport.New.Nodes, false)
			if err != nil {
				addIssue(decision, index, "new_locator", err.Error())
			} else if detail != nil {
				decision = next
				materializedRefs = append(materializedRefs, *detail)
			}
		}
		materialized = append(materialized, decision)
	}

	return materialized, issues, materializedRefs
}

type compareDecisionRefResolution struct {
	Ref       string
	MatchedBy string
}

type compareDecisionSelectorResolver func(oldSide bool, selector string, nodes []compareSnapshotNode) (compareDecisionRefResolution, error)

type compareDecisionSelectorPreflightResult struct {
	Issues           []compareDecisionValidationIssue
	SuccessfulFields map[string]struct{}
	Count            int
}

func materializeCompareDecisionSelectors(decisions []compareDecision, compareReport compareReport, resolver compareDecisionSelectorResolver) ([]compareDecision, []compareDecisionValidationIssue, []compareDecisionMaterializedRef) {
	materialized := make([]compareDecision, 0, len(decisions))
	issues := []compareDecisionValidationIssue{}
	materializedRefs := []compareDecisionMaterializedRef{}

	addIssue := func(decision compareDecision, index int, field string, message string) {
		issues = append(issues, compareDecisionValidationIssue{
			Severity: "error",
			Line:     compareDecisionLineNumber(decision, index),
			Field:    field,
			Message:  message,
		})
	}

	for index, decision := range decisions {
		line := decision.Line
		decision = normalizeCompareDecision(decision)
		decision.Line = line

		switch decision.Kind {
		case "pair", "subtree_pair":
			next, detail, err := materializeCompareDecisionSelectorSide(decision, index, compareReport.Old.Nodes, true, resolver)
			if err != nil {
				addIssue(decision, index, "old_selector", err.Error())
			} else if detail != nil {
				decision = next
				materializedRefs = append(materializedRefs, *detail)
			}
			next, detail, err = materializeCompareDecisionSelectorSide(decision, index, compareReport.New.Nodes, false, resolver)
			if err != nil {
				addIssue(decision, index, "new_selector", err.Error())
			} else if detail != nil {
				decision = next
				materializedRefs = append(materializedRefs, *detail)
			}
		case "accepted_removed", "regression_removed":
			next, detail, err := materializeCompareDecisionSelectorSide(decision, index, compareReport.Old.Nodes, true, resolver)
			if err != nil {
				addIssue(decision, index, "old_selector", err.Error())
			} else if detail != nil {
				decision = next
				materializedRefs = append(materializedRefs, *detail)
			}
		case "accepted_added", "unexpected_added":
			next, detail, err := materializeCompareDecisionSelectorSide(decision, index, compareReport.New.Nodes, false, resolver)
			if err != nil {
				addIssue(decision, index, "new_selector", err.Error())
			} else if detail != nil {
				decision = next
				materializedRefs = append(materializedRefs, *detail)
			}
		}
		materialized = append(materialized, decision)
	}

	return materialized, issues, materializedRefs
}

func materializeCompareDecisionSelectorSide(decision compareDecision, index int, nodes []compareSnapshotNode, oldSide bool, resolver compareDecisionSelectorResolver) (compareDecision, *compareDecisionMaterializedRef, error) {
	ref := decision.New
	selector := decision.NewSelector
	locator := decision.NewLocator
	fingerprint := decision.NewFingerprint
	side := "new"
	if oldSide {
		ref = decision.Old
		selector = decision.OldSelector
		locator = decision.OldLocator
		fingerprint = decision.OldFingerprint
		side = "old"
	}
	if !compareDecisionUnknownValue(ref) || strings.TrimSpace(selector) == "" {
		return decision, nil, nil
	}
	if strings.TrimSpace(locator) != "" || strings.TrimSpace(fingerprint) != "" {
		return decision, nil, nil
	}
	if resolver == nil {
		return decision, nil, fmt.Errorf("%s_selector requires --%s-session", side, side)
	}
	resolution, err := resolver(oldSide, selector, nodes)
	if err != nil {
		return decision, nil, err
	}
	ref = strings.TrimSpace(resolution.Ref)
	if ref == "" {
		return decision, nil, fmt.Errorf("%s selector %q resolved no ref", side, strings.TrimSpace(selector))
	}
	if oldSide {
		decision.Old = ref
	} else {
		decision.New = ref
	}
	return decision, &compareDecisionMaterializedRef{
		Line:      compareDecisionLineNumber(decision, index),
		Side:      side,
		Source:    side + "_selector",
		Value:     strings.TrimSpace(selector),
		Ref:       ref,
		MatchedBy: firstNonEmpty(strings.TrimSpace(resolution.MatchedBy), "selector"),
	}, nil
}

func preflightCompareDecisionSelectors(decisions []compareDecision, compareReport compareReport, resolver compareDecisionSelectorResolver) compareDecisionSelectorPreflightResult {
	result := compareDecisionSelectorPreflightResult{
		SuccessfulFields: map[string]struct{}{},
	}
	addIssue := func(decision compareDecision, index int, field string, message string) {
		result.Issues = append(result.Issues, compareDecisionValidationIssue{
			Severity: "error",
			Line:     compareDecisionLineNumber(decision, index),
			Field:    field,
			Message:  message,
		})
	}

	for index, original := range decisions {
		line := original.Line
		decision := normalizeCompareDecision(original)
		decision.Line = line

		switch decision.Kind {
		case "pair", "subtree_pair":
			preflightCompareDecisionSelectorSide(decision, index, compareReport.Old.Nodes, true, resolver, &result, addIssue)
			preflightCompareDecisionSelectorSide(decision, index, compareReport.New.Nodes, false, resolver, &result, addIssue)
		case "accepted_removed", "regression_removed":
			preflightCompareDecisionSelectorSide(decision, index, compareReport.Old.Nodes, true, resolver, &result, addIssue)
		case "accepted_added", "unexpected_added":
			preflightCompareDecisionSelectorSide(decision, index, compareReport.New.Nodes, false, resolver, &result, addIssue)
		}
	}

	return result
}

func preflightCompareDecisionSelectorSide(decision compareDecision, index int, nodes []compareSnapshotNode, oldSide bool, resolver compareDecisionSelectorResolver, result *compareDecisionSelectorPreflightResult, addIssue func(compareDecision, int, string, string)) {
	selector := decision.NewSelector
	side := "new"
	field := "new_selector"
	if oldSide {
		selector = decision.OldSelector
		side = "old"
		field = "old_selector"
	}
	if strings.TrimSpace(selector) == "" {
		return
	}
	if resolver == nil {
		addIssue(decision, index, field, fmt.Sprintf("%s_selector requires --%s-session", side, side))
		return
	}
	if _, err := resolver(oldSide, selector, nodes); err != nil {
		addIssue(decision, index, field, err.Error())
		return
	}
	result.Count++
	result.SuccessfulFields[compareDecisionValidationIssueKey(compareDecisionLineNumber(decision, index), field)] = struct{}{}
}

func applyCompareDecisionSelectorPreflightReport(report *compareDecisionValidationReport, result compareDecisionSelectorPreflightResult) {
	report.Summary.SelectorPreflightUsed = true
	report.Summary.SelectorPreflighted = result.Count
	if len(result.SuccessfulFields) > 0 {
		filtered := make([]compareDecisionValidationIssue, 0, len(report.Issues))
		for _, issue := range report.Issues {
			if compareDecisionSelectorMaterializeWarning(issue) {
				if _, ok := result.SuccessfulFields[compareDecisionValidationIssueKey(issue.Line, issue.Field)]; ok {
					report.Summary.Warnings--
					continue
				}
			}
			filtered = append(filtered, issue)
		}
		report.Issues = filtered
	}
	for _, issue := range result.Issues {
		report.Issues = append(report.Issues, issue)
		if issue.Severity == "error" {
			report.Summary.Errors++
			continue
		}
		report.Summary.Warnings++
	}
}

func compareDecisionValidationIssueKey(line int, field string) string {
	return strconv.Itoa(line) + "\x00" + field
}

func compareDecisionSelectorMaterializeWarning(issue compareDecisionValidationIssue) bool {
	if issue.Severity != "warning" {
		return false
	}
	if issue.Field != "old_selector" && issue.Field != "new_selector" {
		return false
	}
	return strings.Contains(issue.Message, "selector-only decisions should be materialized")
}

func materializeCompareDecisionSide(decision compareDecision, index int, nodes []compareSnapshotNode, oldSide bool) (compareDecision, *compareDecisionMaterializedRef, error) {
	ref := decision.New
	locator := decision.NewLocator
	side := "new"
	if oldSide {
		ref = decision.Old
		locator = decision.OldLocator
		side = "old"
	}
	if !compareDecisionUnknownValue(ref) || strings.TrimSpace(locator) == "" {
		return decision, nil, nil
	}
	nodeIndex, err := compareResolveDecisionLocator(nodes, side, locator)
	if err != nil {
		return decision, nil, err
	}
	ref = strings.TrimSpace(nodes[nodeIndex].Ref)
	if oldSide {
		decision.Old = ref
	} else {
		decision.New = ref
	}
	return decision, &compareDecisionMaterializedRef{
		Line:      compareDecisionLineNumber(decision, index),
		Side:      side,
		Source:    side + "_locator",
		Value:     strings.TrimSpace(locator),
		Ref:       ref,
		MatchedBy: "locator",
	}, nil
}

func materializeCompareClusterDecisions(decisions []compareDecision, compareReport *compareReport, clusters []compareFindingCluster) []compareDecision {
	if len(decisions) == 0 {
		return nil
	}
	materialized := make([]compareDecision, 0, len(decisions))
	for _, decision := range decisions {
		if !compareDecisionKindIsFindingCluster(decision.Kind) || !compareDecisionApplies(decision) {
			materialized = append(materialized, decision)
			continue
		}
		cluster, ok := compareFindDecisionFindingCluster(compareReport, clusters, decision.ClusterKey)
		if !ok {
			materialized = append(materialized, decision)
			continue
		}
		for _, findingID := range cluster.FindingIDs {
			next := decision
			next.Kind = compareMaterializedFindingClusterKind(decision.Kind)
			next.FindingID = findingID
			next.ClusterKey = ""
			next.Confidence = compareDecisionEffectiveConfidence(decision)
			materialized = append(materialized, next)
		}
	}
	return materialized
}

func compareDecisionKindIsFindingCluster(kind string) bool {
	switch normalizeCompareDecisionToken(kind) {
	case "accepted_finding_cluster", "regression_finding_cluster":
		return true
	default:
		return false
	}
}

func compareMaterializedFindingClusterKind(kind string) string {
	switch normalizeCompareDecisionToken(kind) {
	case "accepted_finding_cluster":
		return "accepted_finding"
	case "regression_finding_cluster":
		return "regression_finding"
	default:
		return normalizeCompareDecisionToken(kind)
	}
}

func compareFindDecisionFindingCluster(report *compareReport, clusters []compareFindingCluster, clusterKey string) (compareFindingCluster, bool) {
	if report != nil {
		if cluster, ok := compareFindFindingCluster(compareFindingClusters(report.Findings, ""), clusterKey); ok {
			return cluster, true
		}
	}
	return compareFindFindingCluster(clusters, clusterKey)
}

func compareFindFindingCluster(clusters []compareFindingCluster, clusterKey string) (compareFindingCluster, bool) {
	if strings.TrimSpace(clusterKey) == "" {
		return compareFindingCluster{}, false
	}
	for _, cluster := range clusters {
		if cluster.Key == clusterKey || strings.TrimSpace(cluster.Key) == strings.TrimSpace(clusterKey) {
			return cluster, true
		}
	}
	return compareFindingCluster{}, false
}

func normalizeCompareDecision(decision compareDecision) compareDecision {
	if decision.SchemaVersion == 0 {
		decision.SchemaVersion = 1
	}
	decision.Kind = normalizeCompareDecisionToken(decision.Kind)
	decision.Old = strings.TrimSpace(decision.Old)
	decision.New = strings.TrimSpace(decision.New)
	decision.OldLocator = strings.TrimSpace(decision.OldLocator)
	decision.NewLocator = strings.TrimSpace(decision.NewLocator)
	decision.OldSelector = strings.TrimSpace(decision.OldSelector)
	decision.NewSelector = strings.TrimSpace(decision.NewSelector)
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
		return strings.Join([]string{decision.Kind, decision.Old, decision.New, decision.OldLocator, decision.NewLocator, decision.OldSelector, decision.NewSelector, decision.OldFingerprint, decision.NewFingerprint, confidence}, "\x1f")
	case "subtree_pair":
		return strings.Join([]string{decision.Kind, decision.Old, decision.New, decision.OldLocator, decision.NewLocator, decision.OldSelector, decision.NewSelector, decision.OldFingerprint, decision.NewFingerprint, confidence, decision.MatchKind, fmt.Sprint(decision.Count)}, "\x1f")
	case "accepted_removed", "regression_removed":
		return strings.Join([]string{decision.Kind, decision.Old, decision.OldLocator, decision.OldSelector, decision.OldFingerprint, confidence}, "\x1f")
	case "accepted_added", "unexpected_added":
		return strings.Join([]string{decision.Kind, decision.New, decision.NewLocator, decision.NewSelector, decision.NewFingerprint, confidence}, "\x1f")
	case "accepted_finding", "regression_finding":
		return strings.Join([]string{decision.Kind, decision.FindingID, confidence}, "\x1f")
	case "accepted_finding_cluster", "regression_finding_cluster":
		return strings.Join([]string{decision.Kind, decision.ClusterKey, confidence}, "\x1f")
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
	case "accepted_removed", "regression_removed", "accepted_added", "unexpected_added", "accepted_finding", "regression_finding", "accepted_finding_cluster", "regression_finding_cluster":
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
	case "accepted_finding_cluster", "regression_finding_cluster":
		if strings.TrimSpace(decision.ClusterKey) == "" {
			return nil
		}
		return []string{"finding_cluster:" + strings.TrimSpace(decision.ClusterKey)}
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
	case "accepted_removed", "regression_removed", "accepted_added", "unexpected_added", "accepted_finding", "regression_finding", "accepted_finding_cluster", "regression_finding_cluster":
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
	if err := compareRejectLocatorOnlyDecisionSide("old", decision.Old, decision.OldFingerprint, decision.OldLocator, decision.OldSelector); err != nil {
		return nil, err
	}
	if err := compareRejectLocatorOnlyDecisionSide("new", decision.New, decision.NewFingerprint, decision.NewLocator, decision.NewSelector); err != nil {
		return nil, err
	}
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
	if err := compareRejectLocatorOnlyDecisionSide("old", decision.Old, decision.OldFingerprint, decision.OldLocator, decision.OldSelector); err != nil {
		return nil, err
	}
	if err := compareRejectLocatorOnlyDecisionSide("new", decision.New, decision.NewFingerprint, decision.NewLocator, decision.NewSelector); err != nil {
		return nil, err
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
			if err := compareRejectLocatorOnlyDecisionSide("old", decision.Old, decision.OldFingerprint, decision.OldLocator, decision.OldSelector); err != nil {
				return compareDecisionEffects{}, fmt.Errorf("decision line %d: %w", lineNumber, err)
			}
			oldIndex, err := compareResolveDecisionNode(oldNodes, "old", decision.Old, decision.OldFingerprint)
			if err != nil {
				return compareDecisionEffects{}, fmt.Errorf("decision line %d: %w", lineNumber, err)
			}
			if _, ok := effects.Old[oldIndex]; ok {
				return compareDecisionEffects{}, fmt.Errorf("decision line %d: old node %q already has a decision effect", lineNumber, decision.Old)
			}
			effects.Old[oldIndex] = compareDecisionEffectFor(decision.Kind)
		case "accepted_added", "unexpected_added":
			if err := compareRejectLocatorOnlyDecisionSide("new", decision.New, decision.NewFingerprint, decision.NewLocator, decision.NewSelector); err != nil {
				return compareDecisionEffects{}, fmt.Errorf("decision line %d: %w", lineNumber, err)
			}
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
		case "accepted_finding_cluster", "regression_finding_cluster":
			return compareFindingDecisionEffects{}, fmt.Errorf("decision line %d: finding cluster decisions must be materialized with compare normalize-decisions --compare-json before compare", lineNumber)
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

func compareResolveDecisionNodeOrLocator(nodes []compareSnapshotNode, side string, ref string, fingerprint string, locator string) (int, error) {
	if !compareDecisionUnknownValue(ref) || strings.TrimSpace(fingerprint) != "" {
		return compareResolveDecisionNode(nodes, side, ref, fingerprint)
	}
	if strings.TrimSpace(locator) != "" {
		return compareResolveDecisionLocator(nodes, side, locator)
	}
	return compareResolveDecisionNode(nodes, side, ref, fingerprint)
}

type compareDecisionLocatorTerm struct {
	Kind  string
	Value string
}

func compareResolveDecisionLocator(nodes []compareSnapshotNode, side string, locator string) (int, error) {
	terms, err := parseCompareDecisionLocator(locator)
	if err != nil {
		return -1, fmt.Errorf("%s locator %q is invalid: %w", side, strings.TrimSpace(locator), err)
	}
	matches := make([]int, 0, 1)
	for index, node := range nodes {
		if matchesCompareDecisionLocatorTerms(node, terms) {
			matches = append(matches, index)
		}
	}
	switch len(matches) {
	case 0:
		return -1, fmt.Errorf("%s locator %q matched no nodes", side, strings.TrimSpace(locator))
	case 1:
		if strings.TrimSpace(nodes[matches[0]].Ref) == "" {
			return -1, fmt.Errorf("%s locator %q matched a node without ref", side, strings.TrimSpace(locator))
		}
		return matches[0], nil
	default:
		return -1, fmt.Errorf("%s locator %q matched %d nodes: %s", side, strings.TrimSpace(locator), len(matches), compareDecisionNodeHints(nodes, matches, 5))
	}
}

func parseCompareDecisionLocator(locator string) ([]compareDecisionLocatorTerm, error) {
	trimmed := strings.TrimSpace(locator)
	if trimmed == "" {
		return nil, fmt.Errorf("locator must not be empty")
	}
	if strings.HasPrefix(trimmed, "@e") && !strings.ContainsAny(trimmed, " \t\r\n:=&") {
		return []compareDecisionLocatorTerm{{Kind: "ref", Value: trimmed}}, nil
	}
	if terms, ok, err := parseCompareDecisionHumanLocator(trimmed); ok || err != nil {
		return terms, err
	}

	parts, err := splitCompareDecisionLocatorParts(trimmed)
	if err != nil {
		return nil, err
	}
	terms := make([]compareDecisionLocatorTerm, 0, len(parts))
	for _, part := range parts {
		term, err := parseCompareDecisionLocatorTerm(part)
		if err != nil {
			return nil, err
		}
		terms = append(terms, term)
	}
	if len(terms) == 0 {
		return nil, fmt.Errorf("locator must include at least one term")
	}
	return terms, nil
}

func parseCompareDecisionHumanLocator(locator string) ([]compareDecisionLocatorTerm, bool, error) {
	if strings.HasPrefix(locator, "role ") {
		roleValue := strings.TrimSpace(strings.TrimPrefix(locator, "role "))
		if roleValue == "" {
			return nil, true, fmt.Errorf("role value must not be empty")
		}
		if role, rawName, ok := strings.Cut(roleValue, " --name "); ok {
			name, err := unquoteCompareDecisionLocatorValue(rawName)
			if err != nil {
				return nil, true, err
			}
			return []compareDecisionLocatorTerm{
				{Kind: "role", Value: strings.TrimSpace(role)},
				{Kind: "name", Value: name},
			}, true, nil
		}
		return []compareDecisionLocatorTerm{{Kind: "role", Value: roleValue}}, true, nil
	}
	for _, kind := range []string{"testid", "href", "label", "text"} {
		prefix := kind + " "
		if !strings.HasPrefix(locator, prefix) {
			continue
		}
		value, err := unquoteCompareDecisionLocatorValue(strings.TrimSpace(strings.TrimPrefix(locator, prefix)))
		if err != nil {
			return nil, true, err
		}
		return []compareDecisionLocatorTerm{{Kind: kind, Value: value}}, true, nil
	}
	return nil, false, nil
}

func splitCompareDecisionLocatorParts(locator string) ([]string, error) {
	if strings.Contains(locator, "&") {
		rawParts := strings.Split(locator, "&")
		parts := make([]string, 0, len(rawParts))
		for _, part := range rawParts {
			part = strings.TrimSpace(part)
			if part != "" {
				parts = append(parts, part)
			}
		}
		return parts, nil
	}
	return splitCompareDecisionLocatorFields(locator)
}

func splitCompareDecisionLocatorFields(locator string) ([]string, error) {
	parts := []string{}
	var builder strings.Builder
	inQuote := false
	escaped := false
	for _, char := range locator {
		if escaped {
			builder.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' && inQuote {
			builder.WriteRune(char)
			escaped = true
			continue
		}
		if char == '"' {
			builder.WriteRune(char)
			inQuote = !inQuote
			continue
		}
		if unicode.IsSpace(char) && !inQuote {
			part := strings.TrimSpace(builder.String())
			if part != "" {
				parts = append(parts, part)
				builder.Reset()
			}
			continue
		}
		builder.WriteRune(char)
	}
	if inQuote {
		return nil, fmt.Errorf("unterminated quoted value")
	}
	part := strings.TrimSpace(builder.String())
	if part != "" {
		parts = append(parts, part)
	}
	return parts, nil
}

func parseCompareDecisionLocatorTerm(value string) (compareDecisionLocatorTerm, error) {
	raw := strings.TrimSpace(value)
	separator := strings.IndexAny(raw, ":=")
	if separator < 0 {
		return compareDecisionLocatorTerm{}, fmt.Errorf("locator term %q must use key:value or key=value", raw)
	}
	kind := normalizeCompareDecisionLocatorKind(raw[:separator])
	if kind == "" {
		return compareDecisionLocatorTerm{}, fmt.Errorf("locator term %q has unsupported key", raw)
	}
	termValue, err := unquoteCompareDecisionLocatorValue(raw[separator+1:])
	if err != nil {
		return compareDecisionLocatorTerm{}, err
	}
	if termValue == "" {
		return compareDecisionLocatorTerm{}, fmt.Errorf("locator term %q value must not be empty", raw)
	}
	return compareDecisionLocatorTerm{Kind: kind, Value: termValue}, nil
}

func normalizeCompareDecisionLocatorKind(kind string) string {
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case "ref":
		return "ref"
	case "role":
		return "role"
	case "label":
		return "label"
	case "name":
		return "name"
	case "text":
		return "text"
	case "href":
		return "href"
	case "testid", "test_id", "data-testid", "data-test":
		return "testid"
	case "fingerprint":
		return "fingerprint"
	case "structure", "structure_key", "structure-key":
		return "structure_key"
	case "subtree", "subtree_signature", "subtree-signature":
		return "subtree_signature"
	case "locator":
		return "locator"
	default:
		return ""
	}
}

func unquoteCompareDecisionLocatorValue(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if len(trimmed) >= 2 && (trimmed[0] == '"' || trimmed[0] == '`') {
		unquoted, err := strconv.Unquote(trimmed)
		if err != nil {
			return "", fmt.Errorf("invalid quoted value %q: %w", trimmed, err)
		}
		return strings.TrimSpace(unquoted), nil
	}
	return trimmed, nil
}

func matchesCompareDecisionLocatorTerms(node compareSnapshotNode, terms []compareDecisionLocatorTerm) bool {
	for _, term := range terms {
		if !matchesCompareDecisionLocatorTerm(node, term) {
			return false
		}
	}
	return true
}

func matchesCompareDecisionLocatorTerm(node compareSnapshotNode, term compareDecisionLocatorTerm) bool {
	switch term.Kind {
	case "ref":
		return strings.TrimSpace(node.Ref) == term.Value
	case "role":
		return normalizeFindValue(node.Role) == normalizeFindValue(term.Value)
	case "label":
		return compareSelectorContains(node.Label, term.Value) || compareSelectorContains(node.Name, term.Value)
	case "name":
		return compareSelectorContains(node.Name, term.Value)
	case "text":
		return compareSelectorContains(node.Text, term.Value)
	case "href":
		return compareSelectorContains(node.Href, term.Value)
	case "testid":
		return compareSelectorContains(node.TestID, term.Value)
	case "fingerprint":
		return strings.TrimSpace(node.Fingerprint) == strings.TrimSpace(term.Value)
	case "structure_key":
		return strings.TrimSpace(node.StructureKey) == strings.TrimSpace(term.Value)
	case "subtree_signature":
		return strings.TrimSpace(node.SubtreeSignature) == strings.TrimSpace(term.Value)
	case "locator":
		return compareNodeLocator(node) == term.Value
	default:
		return false
	}
}

func compareDecisionNodeHints(nodes []compareSnapshotNode, indices []int, limit int) string {
	if limit <= 0 || limit > len(indices) {
		limit = len(indices)
	}
	hints := make([]string, 0, limit)
	for _, index := range indices[:limit] {
		hints = append(hints, compareDecisionNodeHint(nodes[index]))
	}
	if len(indices) > limit {
		hints = append(hints, fmt.Sprintf("+%d more", len(indices)-limit))
	}
	return strings.Join(hints, ", ")
}

func compareDecisionNodeHint(node compareSnapshotNode) string {
	parts := []string{}
	if strings.TrimSpace(node.Ref) != "" {
		parts = append(parts, strings.TrimSpace(node.Ref))
	}
	if locator := compareNodeLocator(node); locator != "" {
		parts = append(parts, locator)
	} else if node.Role != "" {
		parts = append(parts, "role="+node.Role)
	}
	if node.Label != "" && node.Label != node.Name {
		parts = append(parts, "label="+compareShortDecisionHint(node.Label))
	}
	if node.Fingerprint != "" {
		parts = append(parts, "fingerprint="+compareShortDecisionHint(node.Fingerprint))
	}
	if len(parts) == 0 {
		return "<unlabeled>"
	}
	return strings.Join(parts, " ")
}

func compareShortDecisionHint(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if len(value) <= 80 {
		return strconv.Quote(value)
	}
	return strconv.Quote(value[:77] + "...")
}

func compareDecisionKindSupported(kind string) bool {
	switch kind {
	case "pair", "subtree_pair", "accepted_removed", "accepted_added", "regression_removed", "unexpected_added", "accepted_finding", "regression_finding", "accepted_finding_cluster", "regression_finding_cluster", "unknown", "pattern", "severity":
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
	return validateCompareDecisionsWithClusters(decisions, compareReport, nil)
}

func validateCompareDecisionsWithClusters(decisions []compareDecision, compareReport *compareReport, clusters []compareFindingCluster) compareDecisionValidationReport {
	report := compareDecisionValidationReport{
		Summary: compareDecisionValidationSummary{
			TotalDecisions:    len(decisions),
			CompareJSONUsed:   compareReport != nil,
			ReviewSummaryUsed: len(clusters) > 0,
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
		case "accepted_finding_cluster", "regression_finding_cluster":
			validateCompareFindingClusterDecision(&report, addIssue, usedFindingEffects, decision, index, compareReport, clusters)
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
	if compareDecisionUnknownValue(decision.Old) && strings.TrimSpace(decision.OldFingerprint) == "" && strings.TrimSpace(decision.OldLocator) == "" && strings.TrimSpace(decision.OldSelector) == "" {
		addIssue("error", decision, index, "old", "pair decisions require old ref, old_fingerprint, old_locator, or old_selector")
	}
	if compareDecisionUnknownValue(decision.New) && strings.TrimSpace(decision.NewFingerprint) == "" && strings.TrimSpace(decision.NewLocator) == "" && strings.TrimSpace(decision.NewSelector) == "" && decision.Confidence != "unknown" {
		addIssue("error", decision, index, "new", "pair decisions require new ref, new_fingerprint, new_locator, new_selector, or confidence unknown")
	}
	if decision.Confidence == "high" && compareDecisionUnknownValue(decision.New) && strings.TrimSpace(decision.NewFingerprint) == "" && strings.TrimSpace(decision.NewLocator) == "" && strings.TrimSpace(decision.NewSelector) == "" {
		addIssue("error", decision, index, "new", "high-confidence pair decisions require a concrete new ref, new_fingerprint, new_locator, or new_selector")
	}
	if decision.Confidence == "high" && compareDecisionUnknownValue(decision.Old) && strings.TrimSpace(decision.OldFingerprint) == "" && strings.TrimSpace(decision.OldLocator) == "" && strings.TrimSpace(decision.OldSelector) == "" {
		addIssue("error", decision, index, "old", "high-confidence pair decisions require a concrete old ref, old_fingerprint, old_locator, or old_selector")
	}
	if compareReport == nil || decision.Confidence != "high" {
		if decision.Confidence == "high" {
			validateCompareLocatorMaterializeHint(addIssue, decision, index)
		}
		return
	}
	if compareDecisionHasOnlySelector(decision.Old, decision.OldFingerprint, decision.OldLocator, decision.OldSelector) || compareDecisionHasOnlySelector(decision.New, decision.NewFingerprint, decision.NewLocator, decision.NewSelector) {
		validateCompareLocatorMaterializeHint(addIssue, decision, index)
		return
	}
	oldIndex, err := compareResolveDecisionNodeOrLocator(compareReport.Old.Nodes, "old", decision.Old, decision.OldFingerprint, decision.OldLocator)
	if err != nil {
		addIssue("error", decision, index, "old", err.Error())
	} else if previousLine, ok := usedOld[oldIndex]; ok {
		addIssue("error", decision, index, "old", fmt.Sprintf("old node already paired by decision line %d", previousLine))
	} else {
		usedOld[oldIndex] = compareDecisionLineNumber(decision, index)
	}
	newIndex, err := compareResolveDecisionNodeOrLocator(compareReport.New.Nodes, "new", decision.New, decision.NewFingerprint, decision.NewLocator)
	if err != nil {
		addIssue("error", decision, index, "new", err.Error())
	} else if previousLine, ok := usedNew[newIndex]; ok {
		addIssue("error", decision, index, "new", fmt.Sprintf("new node already paired by decision line %d", previousLine))
	} else {
		usedNew[newIndex] = compareDecisionLineNumber(decision, index)
	}
	validateCompareLocatorMaterializeHint(addIssue, decision, index)
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
	if compareDecisionUnknownValue(decision.Old) && strings.TrimSpace(decision.OldFingerprint) == "" && strings.TrimSpace(decision.OldLocator) == "" && strings.TrimSpace(decision.OldSelector) == "" {
		addIssue("error", decision, index, "old", "subtree_pair decisions require old ref, old_fingerprint, old_locator, or old_selector")
	}
	if compareDecisionUnknownValue(decision.New) && strings.TrimSpace(decision.NewFingerprint) == "" && strings.TrimSpace(decision.NewLocator) == "" && strings.TrimSpace(decision.NewSelector) == "" && decision.Confidence != "unknown" {
		addIssue("error", decision, index, "new", "subtree_pair decisions require new ref, new_fingerprint, new_locator, new_selector, or confidence unknown")
	}
	if decision.Confidence == "high" && compareDecisionUnknownValue(decision.New) && strings.TrimSpace(decision.NewFingerprint) == "" && strings.TrimSpace(decision.NewLocator) == "" && strings.TrimSpace(decision.NewSelector) == "" {
		addIssue("error", decision, index, "new", "high-confidence subtree_pair decisions require a concrete new ref, new_fingerprint, new_locator, or new_selector")
	}
	if decision.Confidence == "high" && compareDecisionUnknownValue(decision.Old) && strings.TrimSpace(decision.OldFingerprint) == "" && strings.TrimSpace(decision.OldLocator) == "" && strings.TrimSpace(decision.OldSelector) == "" {
		addIssue("error", decision, index, "old", "high-confidence subtree_pair decisions require a concrete old ref, old_fingerprint, old_locator, or old_selector")
	}
	if compareReport == nil || decision.Confidence != "high" {
		if decision.Confidence == "high" {
			validateCompareLocatorMaterializeHint(addIssue, decision, index)
		}
		return
	}
	if compareDecisionHasOnlySelector(decision.Old, decision.OldFingerprint, decision.OldLocator, decision.OldSelector) || compareDecisionHasOnlySelector(decision.New, decision.NewFingerprint, decision.NewLocator, decision.NewSelector) {
		validateCompareLocatorMaterializeHint(addIssue, decision, index)
		return
	}
	resolvedDecision, err := compareDecisionWithResolvedLocators(decision, compareReport.Old.Nodes, compareReport.New.Nodes)
	if err != nil {
		addIssue("error", decision, index, "subtree_pair", err.Error())
		return
	}
	matches, err := compareBuildSubtreePairDecisionMatches(resolvedDecision, compareReport.Old.Nodes, compareReport.New.Nodes)
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
	validateCompareLocatorMaterializeHint(addIssue, decision, index)
}

func validateCompareOldDecision(report *compareDecisionValidationReport, addIssue func(string, compareDecision, int, string, string), used map[int]int, decision compareDecision, index int, compareReport *compareReport) {
	if decision.Kind == "accepted_removed" {
		report.Summary.AcceptedRemoved++
	}
	if compareDecisionUnknownValue(decision.Old) && strings.TrimSpace(decision.OldFingerprint) == "" && strings.TrimSpace(decision.OldLocator) == "" && strings.TrimSpace(decision.OldSelector) == "" {
		addIssue("error", decision, index, "old", "removed decisions require old ref, old_fingerprint, old_locator, or old_selector")
		return
	}
	if compareReport == nil || !compareDecisionApplies(decision) {
		if compareDecisionApplies(decision) {
			validateCompareLocatorMaterializeHint(addIssue, decision, index)
		}
		return
	}
	if compareDecisionHasOnlySelector(decision.Old, decision.OldFingerprint, decision.OldLocator, decision.OldSelector) {
		validateCompareLocatorMaterializeHint(addIssue, decision, index)
		return
	}
	oldIndex, err := compareResolveDecisionNodeOrLocator(compareReport.Old.Nodes, "old", decision.Old, decision.OldFingerprint, decision.OldLocator)
	if err != nil {
		addIssue("error", decision, index, "old", err.Error())
		return
	}
	if previousLine, ok := used[oldIndex]; ok {
		addIssue("error", decision, index, "old", fmt.Sprintf("old node already has a decision effect from line %d", previousLine))
		return
	}
	used[oldIndex] = compareDecisionLineNumber(decision, index)
	validateCompareLocatorMaterializeHint(addIssue, decision, index)
}

func validateCompareNewDecision(report *compareDecisionValidationReport, addIssue func(string, compareDecision, int, string, string), used map[int]int, decision compareDecision, index int, compareReport *compareReport) {
	if decision.Kind == "accepted_added" {
		report.Summary.AcceptedAdded++
	}
	if compareDecisionUnknownValue(decision.New) && strings.TrimSpace(decision.NewFingerprint) == "" && strings.TrimSpace(decision.NewLocator) == "" && strings.TrimSpace(decision.NewSelector) == "" {
		addIssue("error", decision, index, "new", "added decisions require new ref, new_fingerprint, new_locator, or new_selector")
		return
	}
	if compareReport == nil || !compareDecisionApplies(decision) {
		if compareDecisionApplies(decision) {
			validateCompareLocatorMaterializeHint(addIssue, decision, index)
		}
		return
	}
	if compareDecisionHasOnlySelector(decision.New, decision.NewFingerprint, decision.NewLocator, decision.NewSelector) {
		validateCompareLocatorMaterializeHint(addIssue, decision, index)
		return
	}
	newIndex, err := compareResolveDecisionNodeOrLocator(compareReport.New.Nodes, "new", decision.New, decision.NewFingerprint, decision.NewLocator)
	if err != nil {
		addIssue("error", decision, index, "new", err.Error())
		return
	}
	if previousLine, ok := used[newIndex]; ok {
		addIssue("error", decision, index, "new", fmt.Sprintf("new node already has a decision effect from line %d", previousLine))
		return
	}
	used[newIndex] = compareDecisionLineNumber(decision, index)
	validateCompareLocatorMaterializeHint(addIssue, decision, index)
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

func validateCompareFindingClusterDecision(report *compareDecisionValidationReport, addIssue func(string, compareDecision, int, string, string), used map[string]int, decision compareDecision, index int, compareReport *compareReport, clusters []compareFindingCluster) {
	if decision.Kind == "accepted_finding_cluster" {
		report.Summary.AcceptedFindings++
	}
	if decision.Kind == "regression_finding_cluster" {
		report.Summary.RegressionFindings++
	}
	if strings.TrimSpace(decision.ClusterKey) == "" {
		addIssue("error", decision, index, "cluster_key", "finding cluster decisions require cluster_key")
		return
	}
	if (compareReport == nil && len(clusters) == 0) || !compareDecisionApplies(decision) {
		return
	}
	cluster, ok := compareFindDecisionFindingCluster(compareReport, clusters, decision.ClusterKey)
	if !ok {
		addIssue("error", decision, index, "cluster_key", "cluster_key was not found in compare report finding clusters")
		return
	}
	for _, findingID := range cluster.FindingIDs {
		if previousLine, ok := used[findingID]; ok {
			addIssue("error", decision, index, "cluster_key", fmt.Sprintf("finding %q already has a decision effect from line %d", findingID, previousLine))
			continue
		}
		used[findingID] = compareDecisionLineNumber(decision, index)
	}
}

func validateCompareLocatorMaterializeHint(addIssue func(string, compareDecision, int, string, string), decision compareDecision, index int) {
	if compareDecisionHasOnlyLocator(decision.Old, decision.OldFingerprint, decision.OldLocator) {
		addIssue("warning", decision, index, "old_locator", "locator-only decisions should be materialized with compare materialize-decisions before compare")
	}
	if compareDecisionHasOnlyLocator(decision.New, decision.NewFingerprint, decision.NewLocator) {
		addIssue("warning", decision, index, "new_locator", "locator-only decisions should be materialized with compare materialize-decisions before compare")
	}
	if compareDecisionHasOnlySelector(decision.Old, decision.OldFingerprint, decision.OldLocator, decision.OldSelector) {
		addIssue("warning", decision, index, "old_selector", "selector-only decisions should be materialized with compare materialize-decisions --old-session before compare")
	}
	if compareDecisionHasOnlySelector(decision.New, decision.NewFingerprint, decision.NewLocator, decision.NewSelector) {
		addIssue("warning", decision, index, "new_selector", "selector-only decisions should be materialized with compare materialize-decisions --new-session before compare")
	}
}

func compareDecisionHasOnlyLocator(ref string, fingerprint string, locator string) bool {
	return compareDecisionUnknownValue(ref) && strings.TrimSpace(fingerprint) == "" && strings.TrimSpace(locator) != ""
}

func compareDecisionHasOnlySelector(ref string, fingerprint string, locator string, selector string) bool {
	return compareDecisionUnknownValue(ref) && strings.TrimSpace(fingerprint) == "" && strings.TrimSpace(locator) == "" && strings.TrimSpace(selector) != ""
}

func compareRejectLocatorOnlyDecisionSide(side string, ref string, fingerprint string, locator string, selector string) error {
	if !compareDecisionHasOnlyLocator(ref, fingerprint, locator) {
		if !compareDecisionHasOnlySelector(ref, fingerprint, locator, selector) {
			return nil
		}
		return fmt.Errorf("%s selector-only decision must be materialized with compare materialize-decisions before compare", side)
	}
	return fmt.Errorf("%s locator-only decision must be materialized with compare materialize-decisions before compare", side)
}

func compareDecisionWithResolvedLocators(decision compareDecision, oldNodes []compareSnapshotNode, newNodes []compareSnapshotNode) (compareDecision, error) {
	next, _, err := materializeCompareDecisionSide(decision, 0, oldNodes, true)
	if err != nil {
		return decision, err
	}
	decision = next
	next, _, err = materializeCompareDecisionSide(decision, 0, newNodes, false)
	if err != nil {
		return decision, err
	}
	return next, nil
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

func writeCompareFindingClusterDecisionsTemplate(path string, clusters []compareFindingCluster) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return printCompareFindingClusterDecisionsTemplate(file, clusters)
}

func printCompareDecisionsTemplate(w io.Writer, debug *compareMatchingDebug) error {
	if debug == nil {
		return nil
	}
	plan := buildCompareDecisionTemplatePlan(debug)
	encoder := json.NewEncoder(w)
	for _, candidate := range plan.Ambiguous {
		decision := compareDecision{
			SchemaVersion:  1,
			Kind:           "pair",
			Old:            candidate.Old.Ref,
			OldLocator:     candidate.Old.Locator,
			OldSelector:    candidate.Old.Selector,
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
	if err := printCompareUnmatchedOldDecisionsTemplate(encoder, plan); err != nil {
		return err
	}
	if err := printCompareUnmatchedNewDecisionsTemplate(encoder, plan); err != nil {
		return err
	}
	return nil
}

func compareDecisionTemplateCountsForDebug(debug *compareMatchingDebug) *compareDecisionTemplateCounts {
	if debug == nil {
		return nil
	}
	plan := buildCompareDecisionTemplatePlan(debug)
	return &plan.Counts
}

func buildCompareDecisionTemplatePlan(debug *compareMatchingDebug) compareDecisionTemplatePlan {
	plan := compareDecisionTemplatePlan{}
	if debug == nil {
		return plan
	}

	oldKeys := map[string]struct{}{}
	newKeys := map[string]struct{}{}
	for _, candidate := range debug.AmbiguousCandidates {
		oldKey := compareDecisionTemplateNodeKey(candidate.Old)
		if oldKey != "" {
			if _, ok := oldKeys[oldKey]; ok {
				plan.Counts.SkippedDuplicateOld++
				continue
			}
			oldKeys[oldKey] = struct{}{}
		}
		plan.Ambiguous = append(plan.Ambiguous, candidate)
		for _, option := range candidate.NewCandidates {
			newKey := compareDecisionTemplateNodeKey(option.Node)
			if newKey != "" {
				newKeys[newKey] = struct{}{}
			}
		}
	}
	plan.Counts.Ambiguous = len(plan.Ambiguous)

	filteredOld := make([]compareMatchingDebugNode, 0, len(debug.UnmatchedOld))
	seenOld := map[string]struct{}{}
	for _, node := range debug.UnmatchedOld {
		key := compareDecisionTemplateNodeKey(node)
		if key != "" {
			if _, ok := oldKeys[key]; ok {
				plan.Counts.SkippedDuplicateOld++
				continue
			}
			if _, ok := seenOld[key]; ok {
				plan.Counts.SkippedDuplicateOld++
				continue
			}
			seenOld[key] = struct{}{}
		}
		filteredOld = append(filteredOld, node)
	}
	plan.UnmatchedOld, plan.Counts.TruncatedOld = compareDecisionTemplateCapNodes(filteredOld)
	plan.Counts.UnmatchedOld = len(plan.UnmatchedOld)

	filteredNew := make([]compareMatchingDebugNode, 0, len(debug.UnmatchedNew))
	seenNew := map[string]struct{}{}
	for _, node := range debug.UnmatchedNew {
		key := compareDecisionTemplateNodeKey(node)
		if key != "" {
			if _, ok := newKeys[key]; ok {
				plan.Counts.SkippedDuplicateNew++
				continue
			}
			if _, ok := seenNew[key]; ok {
				plan.Counts.SkippedDuplicateNew++
				continue
			}
			seenNew[key] = struct{}{}
		}
		filteredNew = append(filteredNew, node)
	}
	plan.UnmatchedNew, plan.Counts.TruncatedNew = compareDecisionTemplateCapNodes(filteredNew)
	plan.Counts.UnmatchedNew = len(plan.UnmatchedNew)
	return plan
}

func compareDecisionTemplateNodeKey(node compareMatchingDebugNode) string {
	if value := strings.TrimSpace(node.Ref); value != "" {
		return "ref:" + value
	}
	if value := strings.TrimSpace(node.Fingerprint); value != "" {
		return "fingerprint:" + value
	}
	if value := strings.TrimSpace(node.Locator); value != "" {
		return "locator:" + value
	}
	if value := strings.TrimSpace(node.Selector); value != "" {
		return "selector:" + value
	}
	values := []string{
		fmt.Sprint(node.OriginalIndex),
		strings.TrimSpace(node.Role),
		strings.TrimSpace(node.Label),
		strings.TrimSpace(node.Name),
		strings.TrimSpace(node.Text),
		strings.TrimSpace(node.Href),
		strings.TrimSpace(node.TestID),
	}
	key := strings.Join(compactCompareDecisionTemplateValues(values), "|")
	if key == "" {
		return ""
	}
	return "node:" + key
}

func compareDecisionTemplateCapNodes(nodes []compareMatchingDebugNode) ([]compareMatchingDebugNode, int) {
	if len(nodes) <= compareDecisionTemplateMaxUnmatchedNodes {
		return nodes, 0
	}
	return nodes[:compareDecisionTemplateMaxUnmatchedNodes], len(nodes) - compareDecisionTemplateMaxUnmatchedNodes
}

func printCompareUnmatchedOldDecisionsTemplate(encoder *json.Encoder, plan compareDecisionTemplatePlan) error {
	for _, node := range plan.UnmatchedOld {
		decision := compareDecision{
			SchemaVersion:  1,
			Kind:           "pair",
			Old:            firstNonEmpty(node.Ref, "?"),
			OldLocator:     strings.TrimSpace(node.Locator),
			OldSelector:    strings.TrimSpace(node.Selector),
			OldFingerprint: strings.TrimSpace(node.Fingerprint),
			New:            "?",
			Confidence:     "unknown",
			Reason:         "review unmatched old node",
			Note:           compareUnmatchedDecisionTemplateNote("unmatched old", node, "choose new/new_locator/new_selector, change kind to accepted_removed, or leave unknown"),
		}
		if err := encoder.Encode(decision); err != nil {
			return err
		}
	}
	return printCompareDecisionTemplateTruncation(encoder, "unmatched_old", plan.Counts.UnmatchedOld, plan.Counts.TruncatedOld)
}

func printCompareUnmatchedNewDecisionsTemplate(encoder *json.Encoder, plan compareDecisionTemplatePlan) error {
	for _, node := range plan.UnmatchedNew {
		decision := compareDecision{
			SchemaVersion:  1,
			Kind:           "accepted_added",
			New:            firstNonEmpty(node.Ref, "?"),
			NewLocator:     strings.TrimSpace(node.Locator),
			NewSelector:    strings.TrimSpace(node.Selector),
			NewFingerprint: strings.TrimSpace(node.Fingerprint),
			Confidence:     "unknown",
			Reason:         "review unmatched new node",
			Note:           compareUnmatchedDecisionTemplateNote("unmatched new", node, "confirm accepted_added, change kind to pair with old/old_locator/old_selector, or leave unknown"),
		}
		if err := encoder.Encode(decision); err != nil {
			return err
		}
	}
	return printCompareDecisionTemplateTruncation(encoder, "unmatched_new", plan.Counts.UnmatchedNew, plan.Counts.TruncatedNew)
}

func printCompareDecisionTemplateTruncation(encoder *json.Encoder, kind string, emitted int, truncated int) error {
	if truncated <= 0 {
		return nil
	}
	total := emitted + truncated
	decision := compareDecision{
		SchemaVersion: 1,
		Kind:          "unknown",
		Confidence:    "unknown",
		Reason:        kind + " template truncated",
		Note:          fmt.Sprintf("emitted %d of %d nodes; inspect matching_debug.%s for the remaining nodes", emitted, total, kind),
	}
	return encoder.Encode(decision)
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

func printCompareFindingClusterDecisionsTemplate(w io.Writer, clusters []compareFindingCluster) error {
	encoder := json.NewEncoder(w)
	for _, cluster := range clusters {
		if strings.TrimSpace(cluster.Key) == "" {
			continue
		}
		decision := compareDecision{
			SchemaVersion: 1,
			Kind:          "accepted_finding_cluster",
			ClusterKey:    strings.TrimSpace(cluster.Key),
			Confidence:    "unknown",
			Reason:        "review repeated finding cluster",
			Note:          compareFindingClusterDecisionTemplateNote(cluster),
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

func compareFindingClusterDecisionTemplateNote(cluster compareFindingCluster) string {
	values := []string{
		fmt.Sprintf("%d similar", cluster.Count),
		strings.TrimSpace(cluster.Severity),
		strings.TrimSpace(cluster.Kind),
		strings.TrimSpace(cluster.Impact),
		strings.TrimSpace(compareFindingClusterTarget(cluster)),
		strings.TrimSpace(cluster.Field),
		strings.TrimSpace(strings.Join(cluster.Pages, ",")),
		strings.TrimSpace(cluster.ExampleFindingID),
	}
	return strings.Join(compactCompareDecisionTemplateValues(values), " | ")
}

func compareUnmatchedDecisionTemplateNote(prefix string, node compareMatchingDebugNode, action string) string {
	values := []string{
		prefix,
		action,
		compareMatchingDebugNodeTemplateNote(node),
	}
	return strings.Join(compactCompareDecisionTemplateValues(values), " | ")
}

func compareMatchingDebugNodeTemplateNote(node compareMatchingDebugNode) string {
	values := []string{}
	if ref := strings.TrimSpace(node.Ref); ref != "" {
		values = append(values, "ref="+ref)
	}
	if locator := strings.TrimSpace(node.Locator); locator != "" {
		values = append(values, "locator="+strconv.Quote(locator))
	}
	if selector := strings.TrimSpace(node.Selector); selector != "" {
		values = append(values, "selector="+strconv.Quote(selector))
	}
	if role := strings.TrimSpace(node.Role); role != "" {
		values = append(values, "role="+role)
	}
	if label := strings.TrimSpace(firstNonEmpty(node.Label, node.Name, node.Text)); label != "" {
		values = append(values, "label="+strconv.Quote(label))
	}
	if href := strings.TrimSpace(node.Href); href != "" {
		values = append(values, "href="+strconv.Quote(href))
	}
	if testID := strings.TrimSpace(node.TestID); testID != "" {
		values = append(values, "testid="+strconv.Quote(testID))
	}
	if fingerprint := strings.TrimSpace(node.Fingerprint); fingerprint != "" {
		values = append(values, "fingerprint="+strconv.Quote(fingerprint))
	}
	return strings.Join(values, " ")
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
	candidates := make([]string, 0, len(candidate.NewCandidates))
	for _, option := range candidate.NewCandidates {
		note := compareDecisionTemplateCandidateNote(option)
		if note == "" {
			continue
		}
		candidates = append(candidates, note)
	}
	if len(candidates) == 0 {
		return ""
	}
	return "candidate new nodes: " + strings.Join(candidates, "; ")
}

func compareDecisionTemplateCandidateNote(option compareMatchingDebugCandidateOption) string {
	values := []string{}
	if ref := strings.TrimSpace(option.Node.Ref); ref != "" {
		values = append(values, ref)
	}
	if locator := strings.TrimSpace(option.Node.Locator); locator != "" {
		values = append(values, "locator="+strconv.Quote(locator))
	}
	if selector := strings.TrimSpace(option.Node.Selector); selector != "" {
		values = append(values, "selector="+strconv.Quote(selector))
	}
	if option.Score > 0 {
		values = append(values, fmt.Sprintf("score=%d", option.Score))
	}
	if len(option.SharedKeys) > 0 {
		values = append(values, "shared="+strings.Join(compactCompareDecisionTemplateValues(option.SharedKeys), ","))
	}
	if len(option.DifferingKeys) > 0 {
		values = append(values, "diff="+strings.Join(compactCompareDecisionTemplateValues(option.DifferingKeys), ","))
	}
	if len(values) == 0 {
		return ""
	}
	return strings.Join(values, " ")
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
