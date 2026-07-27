package comparecmd

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/mayahiro/nexus/internal/api"
)

func buildCompareSnapshot(observation api.Observation, options compareSnapshotOptions) compareSnapshot {
	nodes := make([]compareSnapshotNode, 0, len(observation.Tree))
	includeStructure := compareNodeScopeIncludesStructure(options.NodeScope)
	byID := map[int]api.Node{}
	for _, node := range observation.Tree {
		if node.ID > 0 {
			byID[node.ID] = node
		}
	}
	for originalIndex, node := range observation.Tree {
		if matchesCompareDefaultIgnore(node, options) {
			continue
		}
		if matchesCompareSelectorRule(node, options.IgnoreNode) {
			continue
		}

		fingerprint := strings.TrimSpace(node.Fingerprint)
		if fingerprint == "" {
			fingerprint = strings.Join([]string{
				strings.TrimSpace(node.Role),
				strings.TrimSpace(node.Name),
				strings.TrimSpace(node.Text),
				strings.TrimSpace(node.Value),
			}, "|")
		}

		name := normalizeCompareString(node.Name, options.IgnoreText)
		text := normalizeCompareString(node.Text, options.IgnoreText)
		value := normalizeCompareString(node.Value, options.IgnoreText)
		href := normalizeCompareString(node.Attrs["href"], options.IgnoreText)
		testID := normalizeCompareString(firstNonEmpty(node.Attrs["data-testid"], node.Attrs["data-test"]), options.IgnoreText)
		tag := normalizeCompareString(node.Attrs["tag"], options.IgnoreText)
		idAttr := normalizeCompareString(node.Attrs["id"], options.IgnoreText)
		nameAttr := normalizeCompareString(node.Attrs["name"], options.IgnoreText)
		typeAttr := normalizeCompareString(node.Attrs["type"], options.IgnoreText)
		placeholder := normalizeCompareString(node.Attrs["placeholder"], options.IgnoreText)
		ariaLabel := normalizeCompareString(node.Attrs["aria-label"], options.IgnoreText)
		if matchesCompareSelectorRule(node, options.MaskNode) {
			name = ""
			text = ""
			value = ""
			placeholder = ""
			ariaLabel = ""
		}
		css := compareNodeCSS(node, options.CSSProperties)
		bounds := compareNodeBounds(node, options.CompareLayout)
		matchBounds := compareNodeMatchingBounds(node)
		cropBounds := compareNodeCropBounds(node)
		structureKey := ""
		subtreeSignature := ""
		if includeStructure {
			structureKey = compareNodeStructureKey(node)
			subtreeSignature = compareNodeSubtreeSignature(node, strings.TrimSpace(node.Role), byID)
		}

		snapshotNode := compareSnapshotNode{
			Fingerprint:      fingerprint,
			StructureKey:     structureKey,
			SubtreeSignature: subtreeSignature,
			Ref:              strings.TrimSpace(node.Ref),
			Role:             strings.TrimSpace(node.Role),
			Label:            compareNodeLabel(name, text, value, href, testID),
			Name:             name,
			Text:             text,
			Value:            value,
			Href:             href,
			TestID:           testID,
			CSS:              css,
			Bounds:           bounds,
			Visible:          node.Visible,
			Enabled:          node.Enabled,
			Editable:         node.Editable,
			Selectable:       node.Selectable,
			Invokable:        node.Invokable,
			ID:               node.ID,
			Children:         append([]int(nil), node.Children...),
			OriginalIndex:    originalIndex,
			Tag:              tag,
			IDAttr:           idAttr,
			NameAttr:         nameAttr,
			TypeAttr:         typeAttr,
			Placeholder:      placeholder,
			AriaLabel:        ariaLabel,
			MatchBounds:      matchBounds,
			CropBounds:       cropBounds,
		}
		if !compareNodeInScope(snapshotNode, options.NodeScope) {
			continue
		}
		nodes = append(nodes, snapshotNode)
	}

	slices.SortFunc(nodes, func(a, b compareSnapshotNode) int {
		switch {
		case a.Fingerprint < b.Fingerprint:
			return -1
		case a.Fingerprint > b.Fingerprint:
			return 1
		case a.Label < b.Label:
			return -1
		case a.Label > b.Label:
			return 1
		default:
			return 0
		}
	})

	return compareSnapshot{
		SessionID:       observation.SessionID,
		URL:             normalizeCompareString(observation.URLOrScreen, options.IgnoreText),
		Title:           normalizeCompareString(observation.Title, options.IgnoreText),
		Text:            normalizeCompareString(observation.Text, options.IgnoreText),
		Nodes:           nodes,
		ReferenceBounds: compareReferenceBounds(observation, options.CompareLayout),
	}
}

func buildCompareReport(oldSnapshot compareSnapshot, newSnapshot compareSnapshot, scope *compareScope, matchMode string) compareReport {
	return buildCompareReportWithDebug(oldSnapshot, newSnapshot, scope, matchMode, false)
}

func buildCompareReportWithDebug(oldSnapshot compareSnapshot, newSnapshot compareSnapshot, scope *compareScope, matchMode string, matchingDebug bool) compareReport {
	return buildCompareReportWithDecisionMatches(oldSnapshot, newSnapshot, scope, matchMode, matchingDebug, nil)
}

func buildCompareReportWithDecisionMatches(oldSnapshot compareSnapshot, newSnapshot compareSnapshot, scope *compareScope, matchMode string, matchingDebug bool, decisionMatches []compareNodeMatch) compareReport {
	return buildCompareReportWithDecisions(oldSnapshot, newSnapshot, scope, matchMode, matchingDebug, decisionMatches, compareDecisionEffects{})
}

func buildCompareReportWithDecisions(oldSnapshot compareSnapshot, newSnapshot compareSnapshot, scope *compareScope, matchMode string, matchingDebug bool, decisionMatches []compareNodeMatch, decisionEffects compareDecisionEffects) compareReport {
	return buildCompareReportWithDecisionEffects(oldSnapshot, newSnapshot, scope, matchMode, matchingDebug, decisionMatches, decisionEffects, compareFindingDecisionEffects{})
}

func buildCompareReportWithDecisionEffects(oldSnapshot compareSnapshot, newSnapshot compareSnapshot, scope *compareScope, matchMode string, matchingDebug bool, decisionMatches []compareNodeMatch, decisionEffects compareDecisionEffects, findingDecisionEffects compareFindingDecisionEffects) compareReport {
	report := compareReport{
		Old:   oldSnapshot,
		New:   newSnapshot,
		Scope: scope,
	}

	add := func(finding compareFinding) {
		if strings.TrimSpace(finding.FindingID) == "" {
			finding.FindingID = compareFindingID(finding)
		}
		applyCompareFindingDecisionEffect(&finding, findingDecisionEffects.ByID[finding.FindingID])
		severity, impact := classifyCompareFinding(finding)
		if finding.Severity == "" {
			finding.Severity = severity
		}
		if finding.Impact == "" {
			finding.Impact = impact
		}
		report.Findings = append(report.Findings, finding)
		report.Summary.TotalFindings++
		switch finding.Kind {
		case "title_changed":
			report.Summary.TitleChanged++
		case "text_changed":
			report.Summary.TextChanged++
		case "missing_node":
			report.Summary.MissingNodes++
		case "new_node":
			report.Summary.NewNodes++
		case "state_changed":
			report.Summary.StateChanged++
		case "css_changed":
			report.Summary.CSSChanged++
		case "layout_changed":
			report.Summary.LayoutChanged++
		case "page_text_changed":
			report.Summary.PageTextChanged++
		}
		switch finding.Severity {
		case "critical":
			report.Summary.Critical++
		case "warning":
			report.Summary.Warning++
		default:
			report.Summary.Info++
		}
	}

	if oldSnapshot.Title != newSnapshot.Title {
		add(compareFinding{
			Kind:  "title_changed",
			Field: "title",
			Old:   oldSnapshot.Title,
			New:   newSnapshot.Title,
		})
	}

	if oldSnapshot.Text != newSnapshot.Text {
		add(compareFinding{
			Kind:  "page_text_changed",
			Field: "page_text",
			Old:   summarizeCompareValue(oldSnapshot.Text),
			New:   summarizeCompareValue(newSnapshot.Text),
		})
	}

	matchResult := compareMatchNodesWithDecisionMatches(oldSnapshot.Nodes, newSnapshot.Nodes, matchMode, matchingDebug, decisionMatches)
	if matchingDebug {
		report.MatchingDebug = matchResult.Debug
	}
	report.Summary.AmbiguousMatchesSkipped = matchResult.AmbiguousSkipped
	for _, match := range matchResult.Matches {
		addCompareMatchSummary(&report.Summary, match)
		oldNode := oldSnapshot.Nodes[match.OldIndex]
		newNode := newSnapshot.Nodes[match.NewIndex]
		addCompareMatchedNodeFindings(add, oldSnapshot, newSnapshot, oldNode, newNode, match)
	}
	for _, index := range matchResult.UnmatchedOld {
		node := oldSnapshot.Nodes[index]
		finding := compareFinding{
			Kind:             "missing_node",
			Locator:          compareFindingLocator(&node, nil),
			Fingerprint:      node.Fingerprint,
			StructureKey:     node.StructureKey,
			SubtreeSignature: node.SubtreeSignature,
			Role:             node.Role,
			Label:            node.Label,
		}
		applyCompareDecisionEffect(&finding, decisionEffects.Old[index])
		add(finding)
	}
	for _, index := range matchResult.UnmatchedNew {
		node := newSnapshot.Nodes[index]
		finding := compareFinding{
			Kind:             "new_node",
			Locator:          compareFindingLocator(nil, &node),
			Fingerprint:      node.Fingerprint,
			StructureKey:     node.StructureKey,
			SubtreeSignature: node.SubtreeSignature,
			Role:             node.Role,
			Label:            node.Label,
		}
		applyCompareDecisionEffect(&finding, decisionEffects.New[index])
		add(finding)
	}

	report.Summary.Same = report.Summary.TotalFindings == 0
	return report
}

func applyCompareDecisionEffect(finding *compareFinding, effect compareDecisionEffect) {
	if effect.Kind == "" {
		return
	}
	finding.DecisionKind = effect.Kind
	finding.MatchedBy = effect.MatchedBy
	finding.MatchReasons = append([]string(nil), effect.Reasons...)
	if effect.Severity != "" {
		finding.Severity = effect.Severity
	}
	if effect.Impact != "" {
		finding.Impact = effect.Impact
	}
}

func applyCompareFindingDecisionEffect(finding *compareFinding, effect compareDecisionEffect) {
	if effect.Kind == "" {
		return
	}
	finding.DecisionKind = effect.Kind
	if finding.MatchedBy == "" {
		finding.MatchedBy = effect.MatchedBy
	}
	finding.MatchReasons = appendCompareDecisionReasons(finding.MatchReasons, effect.Reasons)
	if effect.Severity != "" {
		finding.Severity = effect.Severity
	}
	if effect.Impact != "" {
		finding.Impact = effect.Impact
	}
}

func appendCompareDecisionReasons(current []string, reasons []string) []string {
	result := append([]string(nil), current...)
	for _, reason := range reasons {
		reason = strings.TrimSpace(reason)
		if reason == "" {
			continue
		}
		if slices.Contains(result, reason) {
			continue
		}
		result = append(result, reason)
	}
	return result
}

func compareFindingID(finding compareFinding) string {
	kind := compareFindingIDKind(finding.Kind)
	parts := []string{
		strings.TrimSpace(finding.Kind),
		strings.TrimSpace(finding.Locator),
		strings.TrimSpace(finding.Fingerprint),
		strings.TrimSpace(finding.StructureKey),
		strings.TrimSpace(finding.SubtreeSignature),
		strings.TrimSpace(finding.Role),
		strings.TrimSpace(finding.Label),
		strings.TrimSpace(finding.Field),
		strings.TrimSpace(finding.Old),
		strings.TrimSpace(finding.New),
	}
	sum := sha1.Sum([]byte(strings.Join(parts, "\x1f")))
	hash := fmt.Sprintf("%x", sum)
	if len(hash) > 12 {
		hash = hash[:12]
	}
	return kind + ":" + hash
}

func compareFindingIDKind(kind string) string {
	kind = normalizeCompareDecisionToken(kind)
	if kind == "" {
		return "finding"
	}
	var builder strings.Builder
	for _, r := range kind {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '_' || r == '-':
			builder.WriteByte('_')
		}
	}
	if builder.Len() == 0 {
		return "finding"
	}
	return builder.String()
}

func addCompareMatchSummary(summary *compareSummary, match compareNodeMatch) {
	summary.MatchedNodes++
	switch {
	case strings.HasPrefix(match.MatchedBy, "decision:"):
		summary.DecisionMatches++
	case strings.HasPrefix(match.MatchedBy, "stable:"):
		summary.StableMatches++
	case match.MatchedBy == "heuristic":
		summary.HeuristicMatches++
	case strings.HasPrefix(match.MatchedBy, "histogram:"):
		summary.HistogramMatches++
	default:
		summary.ExactMatches++
	}
}

func addCompareMatchedNodeFindings(add func(compareFinding), oldSnapshot compareSnapshot, newSnapshot compareSnapshot, oldNode compareSnapshotNode, newNode compareSnapshotNode, match compareNodeMatch) {
	if compareMatchSuppressesFindings(match) {
		return
	}
	locator := compareFindingLocator(&oldNode, &newNode)
	if oldNode.Name != newNode.Name {
		add(compareFindingWithMatch(compareFinding{
			Kind:        "text_changed",
			Locator:     locator,
			Fingerprint: oldNode.Fingerprint,
			Role:        oldNode.Role,
			Label:       firstNonEmpty(oldNode.Label, newNode.Label),
			Field:       "name",
			Old:         oldNode.Name,
			New:         newNode.Name,
		}, match))
	}
	if oldNode.Text != newNode.Text {
		add(compareFindingWithMatch(compareFinding{
			Kind:        "text_changed",
			Locator:     locator,
			Fingerprint: oldNode.Fingerprint,
			Role:        oldNode.Role,
			Label:       firstNonEmpty(oldNode.Label, newNode.Label),
			Field:       "text",
			Old:         summarizeCompareValue(oldNode.Text),
			New:         summarizeCompareValue(newNode.Text),
		}, match))
	}
	if oldNode.Value != newNode.Value {
		add(compareFindingWithMatch(compareFinding{
			Kind:        "text_changed",
			Locator:     locator,
			Fingerprint: oldNode.Fingerprint,
			Role:        oldNode.Role,
			Label:       firstNonEmpty(oldNode.Label, newNode.Label),
			Field:       "value",
			Old:         oldNode.Value,
			New:         newNode.Value,
		}, match))
	}
	oldState := compareNodeState(oldNode)
	newState := compareNodeState(newNode)
	if oldState != newState {
		add(compareFindingWithMatch(compareFinding{
			Kind:        "state_changed",
			Locator:     locator,
			Fingerprint: oldNode.Fingerprint,
			Role:        oldNode.Role,
			Label:       firstNonEmpty(oldNode.Label, newNode.Label),
			Field:       "state",
			Old:         oldState,
			New:         newState,
		}, match))
	}
	for _, property := range sortedCompareCSSPropertyKeys(oldNode.CSS, newNode.CSS) {
		oldValue := strings.TrimSpace(oldNode.CSS[property])
		newValue := strings.TrimSpace(newNode.CSS[property])
		if oldValue == "" && newValue == "" {
			continue
		}
		if oldValue == newValue {
			continue
		}
		add(compareFindingWithMatch(compareFinding{
			Kind:        "css_changed",
			Locator:     locator,
			Fingerprint: oldNode.Fingerprint,
			Role:        oldNode.Role,
			Label:       firstNonEmpty(oldNode.Label, newNode.Label),
			Field:       property,
			Old:         oldValue,
			New:         newValue,
		}, match))
	}
	if compareNodeLayoutChanged(oldNode, newNode) {
		severity := "info"
		if compareNodeLayoutWarning(oldNode, newNode) {
			severity = "warning"
		}
		add(compareFindingWithMatch(compareFinding{
			Kind:        "layout_changed",
			Severity:    severity,
			Impact:      "layout_changed",
			Locator:     locator,
			Fingerprint: oldNode.Fingerprint,
			Role:        oldNode.Role,
			Label:       firstNonEmpty(oldNode.Label, newNode.Label),
			Field:       "bounds",
			Old:         compareLayoutValue(oldNode, oldSnapshot.ReferenceBounds),
			New:         compareLayoutValue(newNode, newSnapshot.ReferenceBounds),
		}, match))
	}
}

func compareMatchSuppressesFindings(match compareNodeMatch) bool {
	return strings.TrimSpace(match.MatchedBy) == "decision:opaque_subtree"
}

func compareFindingWithMatch(finding compareFinding, match compareNodeMatch) compareFinding {
	if strings.TrimSpace(match.MatchedBy) == "" {
		return finding
	}
	finding.MatchedBy = match.MatchedBy
	finding.MatchScore = match.Score
	finding.MatchReasons = append([]string(nil), match.Reasons...)
	return finding
}

func groupCompareNodes(nodes []compareSnapshotNode) map[string][]compareSnapshotNode {
	grouped := make(map[string][]compareSnapshotNode, len(nodes))
	for _, node := range nodes {
		grouped[node.Fingerprint] = append(grouped[node.Fingerprint], node)
	}

	for key := range grouped {
		slices.SortFunc(grouped[key], func(a, b compareSnapshotNode) int {
			aKey := compareNodeSortKey(a)
			bKey := compareNodeSortKey(b)
			switch {
			case aKey < bKey:
				return -1
			case aKey > bKey:
				return 1
			default:
				return 0
			}
		})
	}

	return grouped
}

func compareNodeSortKey(node compareSnapshotNode) string {
	return strings.Join([]string{
		node.Role,
		node.Label,
		node.Name,
		node.Text,
		node.Value,
		node.Href,
		node.TestID,
		compareNodeState(node),
	}, "|")
}

func compareNodeState(node compareSnapshotNode) string {
	return strings.Join([]string{
		strconv.FormatBool(node.Visible),
		strconv.FormatBool(node.Enabled),
		strconv.FormatBool(node.Editable),
		strconv.FormatBool(node.Selectable),
		strconv.FormatBool(node.Invokable),
	}, "/")
}

func compareNodeScopeIncludesStructure(scope string) bool {
	normalized, err := normalizeCompareNodeScope(scope)
	return err == nil && normalized == compareNodeScopeAll
}

func compareNodeStructureKey(node api.Node) string {
	return strings.TrimSpace(node.StructurePath)
}

func compareNodeSubtreeSignature(node api.Node, role string, byID map[int]api.Node) string {
	textLength := node.TextLength
	if textLength <= 0 {
		textLength = len([]rune(strings.Join(strings.Fields(firstNonEmpty(node.Text, node.Name, node.Value)), " ")))
	}
	descendants := node.Descendants
	if descendants <= 0 {
		descendants = len(node.Children)
	}
	role = strings.TrimSpace(role)
	if role == "" {
		role = strings.TrimSpace(node.Attrs["tag"])
	}
	if role == "" {
		return ""
	}
	firstChildRole := compareFirstChildRole(node, byID)
	return strings.Join([]string{
		role,
		"text:" + compareTextLengthBucket(textLength),
		"desc:" + compareDescendantCountBucket(descendants),
		"children:" + compareDescendantCountBucket(len(node.Children)),
		"first:" + firstChildRole,
		"w:" + compareDimensionBucket(node.Bounds.W),
	}, "|")
}

func matchesCompareDefaultIgnore(node api.Node, options compareSnapshotOptions) bool {
	if options.NoDefaultIgnores {
		return false
	}
	normalized, err := normalizeCompareNodeScope(options.NodeScope)
	if err != nil || normalized != compareNodeScopeAll {
		return false
	}
	tag := strings.ToLower(strings.TrimSpace(node.Attrs["tag"]))
	switch tag {
	case "script", "style", "link", "meta", "noscript":
		return true
	}
	if tag != "" && tag != "svg" && strings.Contains(strings.ToLower(strings.TrimSpace(node.StructurePath)), ">svg:") {
		return true
	}
	if _, ok := node.Attrs["hidden"]; ok {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(node.Attrs["aria-hidden"]), "true") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(node.Attrs["data-nxctl-skip"]), "true") {
		return true
	}
	return false
}

func compareFirstChildRole(node api.Node, byID map[int]api.Node) string {
	for _, childID := range node.Children {
		child, ok := byID[childID]
		if !ok {
			continue
		}
		role := strings.TrimSpace(child.Role)
		if role == "" {
			role = strings.TrimSpace(child.Attrs["tag"])
		}
		if role != "" {
			return role
		}
	}
	return "none"
}

func compareTextLengthBucket(value int) string {
	switch {
	case value <= 0:
		return "0"
	case value <= 20:
		return "1-20"
	case value <= 80:
		return "21-80"
	case value <= 240:
		return "81-240"
	default:
		return "241+"
	}
}

func compareDescendantCountBucket(value int) string {
	switch {
	case value <= 0:
		return "0"
	case value <= 3:
		return "1-3"
	case value <= 10:
		return "4-10"
	case value <= 30:
		return "11-30"
	default:
		return "31+"
	}
}

func compareDimensionBucket(value int) string {
	switch {
	case value <= 0:
		return "0"
	case value <= 80:
		return "1-80"
	case value <= 320:
		return "81-320"
	case value <= 768:
		return "321-768"
	case value <= 1280:
		return "769-1280"
	default:
		return "1281+"
	}
}

func compareNodeCSS(node api.Node, properties []string) map[string]string {
	if len(properties) == 0 {
		return nil
	}

	if len(properties) == 1 && properties[0] == allCSSPropertiesMarker {
		values := make(map[string]string, len(node.Styles))
		for property, value := range node.Styles {
			values[property] = strings.TrimSpace(value)
		}
		return values
	}

	values := make(map[string]string, len(properties))
	for _, property := range properties {
		values[property] = strings.TrimSpace(node.Styles[property])
	}
	return values
}

func compareNodeBounds(node api.Node, enabled bool) *api.Rect {
	if !enabled || !compareRectValid(node.Bounds) {
		return nil
	}
	bounds := node.Bounds
	return &bounds
}

func compareNodeMatchingBounds(node api.Node) *api.Rect {
	if !compareRectValid(node.Bounds) {
		return nil
	}
	bounds := node.Bounds
	return &bounds
}

func compareNodeCropBounds(node api.Node) *api.Rect {
	if node.DocumentBounds != nil && compareRectValid(*node.DocumentBounds) {
		bounds := *node.DocumentBounds
		return &bounds
	}
	return compareNodeMatchingBounds(node)
}

func compareReferenceBounds(observation api.Observation, enabled bool) *api.Rect {
	if !enabled {
		return nil
	}
	width, widthOK := compareMetaInt(observation.Meta, "viewport_width")
	height, heightOK := compareMetaInt(observation.Meta, "viewport_height")
	if !widthOK || !heightOK || width <= 0 || height <= 0 {
		return nil
	}
	return &api.Rect{W: width, H: height}
}

func compareMetaInt(meta map[string]string, key string) (int, bool) {
	value, ok := meta[key]
	if !ok {
		return 0, false
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func compareNodeLayoutChanged(oldNode compareSnapshotNode, newNode compareSnapshotNode) bool {
	if oldNode.Bounds == nil || newNode.Bounds == nil {
		return false
	}
	return compareRectDelta(*oldNode.Bounds, *newNode.Bounds) >= compareLayoutThreshold
}

func compareNodeLayoutWarning(oldNode compareSnapshotNode, newNode compareSnapshotNode) bool {
	if oldNode.Bounds == nil || newNode.Bounds == nil {
		return false
	}
	if !compareNodeInteractive(oldNode) && !compareNodeInteractive(newNode) {
		return false
	}
	return compareRectDelta(*oldNode.Bounds, *newNode.Bounds) >= compareLayoutWarningThreshold
}

func compareNodeInteractive(node compareSnapshotNode) bool {
	if node.Editable || node.Selectable || node.Invokable {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(node.Role)) {
	case "button", "link", "textbox", "combobox", "checkbox", "radio", "tab":
		return true
	default:
		return false
	}
}

func compareRectDelta(oldRect api.Rect, newRect api.Rect) int {
	return max(
		compareAbs(oldRect.X-newRect.X),
		compareAbs(oldRect.Y-newRect.Y),
		compareAbs(oldRect.W-newRect.W),
		compareAbs(oldRect.H-newRect.H),
	)
}

func compareAbs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func compareRectValid(rect api.Rect) bool {
	return rect.W > 0 && rect.H > 0
}

func compareLayoutValue(node compareSnapshotNode, reference *api.Rect) string {
	if node.Bounds == nil {
		return ""
	}
	position := comparePlacement(*node.Bounds, reference)
	if position == "" {
		return compareRectValue(*node.Bounds)
	}
	return position + " " + compareRectValue(*node.Bounds)
}

func compareRectValue(rect api.Rect) string {
	return fmt.Sprintf("%d,%d %dx%d", rect.X, rect.Y, rect.W, rect.H)
}

func comparePlacement(rect api.Rect, reference *api.Rect) string {
	if reference == nil || reference.W <= 0 || reference.H <= 0 {
		return ""
	}
	centerX := rect.X + rect.W/2
	centerY := rect.Y + rect.H/2
	x := centerX - reference.X
	y := centerY - reference.Y
	return compareHorizontalPlacement(x, reference.W) + "/" + compareVerticalPlacement(y, reference.H)
}

func compareHorizontalPlacement(center int, width int) string {
	switch {
	case center*3 < width:
		return "left"
	case center*3 > width*2:
		return "right"
	default:
		return "center"
	}
}

func compareVerticalPlacement(center int, height int) string {
	switch {
	case center*3 < height:
		return "top"
	case center*3 > height*2:
		return "bottom"
	default:
		return "middle"
	}
}

func sortedCompareCSSPropertyKeys(left map[string]string, right map[string]string) []string {
	keys := make([]string, 0, len(left)+len(right))
	seen := map[string]struct{}{}
	for _, current := range []map[string]string{left, right} {
		for key := range current {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

func ResolveCSSProperties(compareCSS bool, requested []string) []string {
	properties, _ := ResolveCSSPropertiesMode(compareCSS, false, requested)
	return properties
}

func ResolveCSSPropertiesMode(compareCSS bool, all bool, requested []string) ([]string, error) {
	if all && len(requested) > 0 {
		return nil, errors.New("all css properties can not be combined with explicit css properties")
	}
	if all {
		return []string{allCSSPropertiesMarker}, nil
	}
	if len(requested) == 0 && !compareCSS {
		return nil, nil
	}

	source := requested
	if len(source) == 0 {
		source = DefaultCSSProperties
	}

	values := make([]string, 0, len(source))
	seen := map[string]struct{}{}
	for _, property := range source {
		trimmed := strings.TrimSpace(property)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		values = append(values, trimmed)
	}
	if len(values) == 0 {
		return nil, nil
	}
	return values, nil
}

func summarizeCompareValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= 120 {
		return trimmed
	}
	return trimmed[:117] + "..."
}

func classifyCompareFinding(finding compareFinding) (string, string) {
	switch finding.Kind {
	case "title_changed":
		return "warning", "page_title_changed"
	case "page_text_changed":
		return "info", "content_changed"
	case "new_node":
		return "warning", "new_content"
	case "missing_node":
		switch {
		case finding.Role == "button":
			return "critical", "primary_action_missing"
		case finding.Role == "link":
			return "warning", "navigation_changed"
		case finding.Role == "textbox" || finding.Role == "combobox":
			return "critical", "form_input_changed"
		default:
			return "warning", "content_changed"
		}
	case "state_changed":
		if finding.Field == "state" && strings.Contains(finding.Old, "true/true") && strings.Contains(finding.New, "true/false") {
			if finding.Role == "textbox" || finding.Role == "combobox" {
				return "critical", "form_input_disabled"
			}
			if finding.Role == "button" {
				return "critical", "primary_action_missing"
			}
		}
		return "warning", "content_changed"
	case "css_changed":
		switch finding.Field {
		case "display", "visibility", "opacity", "pointer-events":
			return "warning", "content_changed"
		default:
			return "info", "content_changed"
		}
	case "layout_changed":
		return "info", "layout_changed"
	case "text_changed":
		if finding.Role == "textbox" || finding.Role == "combobox" {
			return "warning", "form_input_changed"
		}
		if finding.Role == "link" {
			return "warning", "navigation_changed"
		}
		return "warning", "content_changed"
	default:
		return "info", "content_changed"
	}
}
