package comparecmd

import (
	"errors"
	"strings"
)

const (
	compareNodeScopeCurrent    = defaultCompareNodeScope
	compareNodeScopeActionable = "actionable"
	compareNodeScopeSemantic   = "semantic"
)

func normalizeCompareNodeScope(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return defaultCompareNodeScope, nil
	}
	switch normalized {
	case compareNodeScopeCurrent, compareNodeScopeActionable, compareNodeScopeSemantic:
		return normalized, nil
	default:
		return "", errors.New("node-scope must be current, actionable, or semantic")
	}
}

func compareNodeInScope(node compareSnapshotNode, scope string) bool {
	normalized, err := normalizeCompareNodeScope(scope)
	if err != nil {
		normalized = defaultCompareNodeScope
	}
	switch normalized {
	case compareNodeScopeActionable:
		return compareNodeActionable(node)
	case compareNodeScopeSemantic:
		return compareNodeSemantic(node)
	default:
		return true
	}
}

func compareNodeActionable(node compareSnapshotNode) bool {
	if node.Editable || node.Selectable || node.Invokable {
		return true
	}
	if strings.TrimSpace(node.Href) != "" {
		return true
	}
	switch compareNodeRole(node) {
	case "button", "link", "textbox", "searchbox", "combobox", "checkbox", "radio", "switch", "tab", "menuitem", "menuitemcheckbox", "menuitemradio", "option", "slider", "spinbutton":
		return true
	default:
		return false
	}
}

func compareNodeSemantic(node compareSnapshotNode) bool {
	if compareNodeActionable(node) {
		return true
	}
	if strings.TrimSpace(node.TestID) != "" || strings.TrimSpace(node.IDAttr) != "" {
		return true
	}
	role := compareNodeRole(node)
	if strings.TrimSpace(node.Name) != "" && compareNodeNamedSemanticRole(role) {
		return true
	}
	if strings.TrimSpace(node.Text) != "" && compareNodeContentSemanticRole(role) {
		return true
	}
	return false
}

func compareNodeNamedSemanticRole(role string) bool {
	switch role {
	case "alert", "article", "banner", "complementary", "contentinfo", "dialog", "figure", "footer", "form", "h1", "h2", "h3", "h4", "h5", "h6", "header", "heading", "img", "main", "nav", "navigation", "region", "search", "section", "status", "table", "tabpanel", "toolbar", "tree":
		return true
	default:
		return false
	}
}

func compareNodeContentSemanticRole(role string) bool {
	switch role {
	case "alert", "article", "blockquote", "caption", "cell", "code", "columnheader", "definition", "figure", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "header", "heading", "img", "list", "listbox", "listitem", "log", "main", "mark", "nav", "note", "paragraph", "region", "row", "rowheader", "section", "status", "table", "term", "time":
		return true
	default:
		return false
	}
}

func compareNodeRole(node compareSnapshotNode) string {
	return strings.ToLower(strings.TrimSpace(node.Role))
}
