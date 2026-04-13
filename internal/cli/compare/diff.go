package comparecmd

import (
	"slices"
	"strconv"
	"strings"

	"github.com/mayahiro/nexus/internal/api"
)

func buildCompareSnapshot(observation api.Observation, options compareSnapshotOptions) compareSnapshot {
	nodes := make([]compareSnapshotNode, 0, len(observation.Tree))
	for _, node := range observation.Tree {
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
		if matchesCompareSelectorRule(node, options.MaskNode) {
			name = ""
			text = ""
			value = ""
		}

		nodes = append(nodes, compareSnapshotNode{
			Fingerprint: fingerprint,
			Ref:         strings.TrimSpace(node.Ref),
			Role:        strings.TrimSpace(node.Role),
			Label:       compareNodeLabel(name, text, value, href, testID),
			Name:        name,
			Text:        text,
			Value:       value,
			Href:        href,
			TestID:      testID,
			Visible:     node.Visible,
			Enabled:     node.Enabled,
			Editable:    node.Editable,
			Selectable:  node.Selectable,
			Invokable:   node.Invokable,
		})
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
		SessionID: observation.SessionID,
		URL:       normalizeCompareString(observation.URLOrScreen, options.IgnoreText),
		Title:     normalizeCompareString(observation.Title, options.IgnoreText),
		Text:      normalizeCompareString(observation.Text, options.IgnoreText),
		Nodes:     nodes,
	}
}

func buildCompareReport(oldSnapshot compareSnapshot, newSnapshot compareSnapshot) compareReport {
	report := compareReport{
		Old: oldSnapshot,
		New: newSnapshot,
	}

	add := func(finding compareFinding) {
		finding.Severity, finding.Impact = classifyCompareFinding(finding)
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

	oldGroups := groupCompareNodes(oldSnapshot.Nodes)
	newGroups := groupCompareNodes(newSnapshot.Nodes)
	keys := make([]string, 0, len(oldGroups)+len(newGroups))
	seen := map[string]struct{}{}
	for key := range oldGroups {
		keys = append(keys, key)
		seen[key] = struct{}{}
	}
	for key := range newGroups {
		if _, ok := seen[key]; ok {
			continue
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)

	for _, key := range keys {
		oldNodes := oldGroups[key]
		newNodes := newGroups[key]
		maxLen := len(oldNodes)
		if len(newNodes) > maxLen {
			maxLen = len(newNodes)
		}
		for i := 0; i < maxLen; i++ {
			switch {
			case i >= len(oldNodes):
				node := newNodes[i]
				add(compareFinding{
					Kind:        "new_node",
					Fingerprint: node.Fingerprint,
					Role:        node.Role,
					Label:       node.Label,
				})
			case i >= len(newNodes):
				node := oldNodes[i]
				add(compareFinding{
					Kind:        "missing_node",
					Fingerprint: node.Fingerprint,
					Role:        node.Role,
					Label:       node.Label,
				})
			default:
				oldNode := oldNodes[i]
				newNode := newNodes[i]
				if oldNode.Name != newNode.Name {
					add(compareFinding{
						Kind:        "text_changed",
						Fingerprint: oldNode.Fingerprint,
						Role:        oldNode.Role,
						Label:       firstNonEmpty(oldNode.Label, newNode.Label),
						Field:       "name",
						Old:         oldNode.Name,
						New:         newNode.Name,
					})
				}
				if oldNode.Text != newNode.Text {
					add(compareFinding{
						Kind:        "text_changed",
						Fingerprint: oldNode.Fingerprint,
						Role:        oldNode.Role,
						Label:       firstNonEmpty(oldNode.Label, newNode.Label),
						Field:       "text",
						Old:         summarizeCompareValue(oldNode.Text),
						New:         summarizeCompareValue(newNode.Text),
					})
				}
				if oldNode.Value != newNode.Value {
					add(compareFinding{
						Kind:        "text_changed",
						Fingerprint: oldNode.Fingerprint,
						Role:        oldNode.Role,
						Label:       firstNonEmpty(oldNode.Label, newNode.Label),
						Field:       "value",
						Old:         oldNode.Value,
						New:         newNode.Value,
					})
				}
				oldState := compareNodeState(oldNode)
				newState := compareNodeState(newNode)
				if oldState != newState {
					add(compareFinding{
						Kind:        "state_changed",
						Fingerprint: oldNode.Fingerprint,
						Role:        oldNode.Role,
						Label:       firstNonEmpty(oldNode.Label, newNode.Label),
						Field:       "state",
						Old:         oldState,
						New:         newState,
					})
				}
			}
		}
	}

	report.Summary.Same = report.Summary.TotalFindings == 0
	return report
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
