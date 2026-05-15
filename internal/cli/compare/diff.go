package comparecmd

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/mayahiro/nexus/internal/api"
)

func buildCompareSnapshot(observation api.Observation, options compareSnapshotOptions) compareSnapshot {
	nodes := make([]compareSnapshotNode, 0, len(observation.Tree))
	for originalIndex, node := range observation.Tree {
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

		snapshotNode := compareSnapshotNode{
			Fingerprint:   fingerprint,
			Ref:           strings.TrimSpace(node.Ref),
			Role:          strings.TrimSpace(node.Role),
			Label:         compareNodeLabel(name, text, value, href, testID),
			Name:          name,
			Text:          text,
			Value:         value,
			Href:          href,
			TestID:        testID,
			CSS:           css,
			Bounds:        bounds,
			Visible:       node.Visible,
			Enabled:       node.Enabled,
			Editable:      node.Editable,
			Selectable:    node.Selectable,
			Invokable:     node.Invokable,
			OriginalIndex: originalIndex,
			Tag:           tag,
			IDAttr:        idAttr,
			NameAttr:      nameAttr,
			TypeAttr:      typeAttr,
			Placeholder:   placeholder,
			AriaLabel:     ariaLabel,
			MatchBounds:   matchBounds,
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
	report := compareReport{
		Old:   oldSnapshot,
		New:   newSnapshot,
		Scope: scope,
	}

	add := func(finding compareFinding) {
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

	matchResult := compareMatchNodes(oldSnapshot.Nodes, newSnapshot.Nodes, matchMode)
	report.Summary.AmbiguousMatchesSkipped = matchResult.AmbiguousSkipped
	for _, match := range matchResult.Matches {
		addCompareMatchSummary(&report.Summary, match)
		oldNode := oldSnapshot.Nodes[match.OldIndex]
		newNode := newSnapshot.Nodes[match.NewIndex]
		addCompareMatchedNodeFindings(add, oldSnapshot, newSnapshot, oldNode, newNode, match)
	}
	for _, index := range matchResult.UnmatchedOld {
		node := oldSnapshot.Nodes[index]
		add(compareFinding{
			Kind:        "missing_node",
			Locator:     compareFindingLocator(&node, nil),
			Fingerprint: node.Fingerprint,
			Role:        node.Role,
			Label:       node.Label,
		})
	}
	for _, index := range matchResult.UnmatchedNew {
		node := newSnapshot.Nodes[index]
		add(compareFinding{
			Kind:        "new_node",
			Locator:     compareFindingLocator(nil, &node),
			Fingerprint: node.Fingerprint,
			Role:        node.Role,
			Label:       node.Label,
		})
	}

	report.Summary.Same = report.Summary.TotalFindings == 0
	return report
}

func addCompareMatchSummary(summary *compareSummary, match compareNodeMatch) {
	summary.MatchedNodes++
	switch {
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

func compareNodeCSS(node api.Node, properties []string) map[string]string {
	if len(properties) == 0 {
		return nil
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
	if len(requested) == 0 && !compareCSS {
		return nil
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
		return nil
	}
	return values
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
