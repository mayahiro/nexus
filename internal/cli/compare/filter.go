package comparecmd

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/mayahiro/nexus/internal/api"
)

// normalizeCompareObservation applies exclusions before deriving any comparison
// identity. It never mutates the observation used by the browser's live refs.
func normalizeCompareObservation(observation api.Observation, options compareSnapshotOptions) (api.Observation, []int) {
	source := observation.Tree
	ignored := make([]bool, len(source))
	masked := make([]bool, len(source))
	byID := make(map[int]int, len(source))
	for i, node := range source {
		if node.ID > 0 {
			byID[node.ID] = i
		}
	}
	mark := func(root int, flags []bool) {
		pending := []int{root}
		for len(pending) > 0 {
			i := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if flags[i] {
				continue
			}
			flags[i] = true
			for _, id := range source[i].Children {
				if child, ok := byID[id]; ok {
					pending = append(pending, child)
				}
			}
		}
		if path := strings.TrimSpace(source[root].StructurePath); path != "" {
			for i, node := range source {
				if strings.HasPrefix(strings.TrimSpace(node.StructurePath), path+">") {
					flags[i] = true
				}
			}
		}
	}
	for i, node := range source {
		if matchesCompareDefaultIgnore(node, options) || matchesCompareSelectorRule(node, options.IgnoreNode) {
			mark(i, ignored)
		}
		if matchesCompareSelectorRule(node, options.MaskNode) {
			mark(i, masked)
		}
	}

	// Text is an aggregate without source ranges. Remove full normalized text
	// contributions, longest first; do not remove input values from unrelated
	// prose (for example the checkbox value "on" in "Confirmation").
	var suppressed []string
	for i, node := range source {
		if ignored[i] || masked[i] {
			if text := normalizeCompareString(node.Text, options.IgnoreText); text != "" {
				suppressed = append(suppressed, text)
			}
		}
	}
	slices.SortFunc(suppressed, func(a, b string) int { return len(b) - len(a) })
	removeText := func(value string, terms []string) string {
		value = normalizeCompareString(value, options.IgnoreText)
		for _, term := range terms {
			value = strings.ReplaceAll(value, term, "")
		}
		return normalizeCompareString(value, nil)
	}
	filteredIdentity := len(options.IgnoreText) > 0 || len(options.IgnoreNode) > 0 || len(options.MaskNode) > 0 ||
		(compareNodeScopeIncludesStructure(options.NodeScope) && !options.NoDefaultIgnores)
	observation.Tree = make([]api.Node, 0, len(source))
	originalIndices := make([]int, 0, len(source))
	for i, original := range source {
		if ignored[i] {
			continue
		}
		node := original
		node.Attrs = maps.Clone(original.Attrs)
		node.States = maps.Clone(original.States)
		for key, value := range node.Attrs {
			node.Attrs[key] = normalizeCompareString(value, options.IgnoreText)
		}
		// Limit ancestor redaction to descendants. Identical text on a sibling
		// remains independently comparable even when the page aggregate omits it.
		var terms []string
		pending := append([]int(nil), original.Children...)
		seen := map[int]bool{}
		for len(pending) > 0 {
			id := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			child, ok := byID[id]
			if !ok || seen[id] {
				continue
			}
			seen[id] = true
			if ignored[child] || masked[child] {
				if text := normalizeCompareString(source[child].Text, options.IgnoreText); text != "" {
					terms = append(terms, text)
				}
			}
			pending = append(pending, source[child].Children...)
		}
		slices.SortFunc(terms, func(a, b string) int { return len(b) - len(a) })
		node.Role = normalizeCompareString(node.Role, options.IgnoreText)
		node.Name = removeText(node.Name, terms)
		node.Text = removeText(node.Text, terms)
		node.Value = normalizeCompareString(node.Value, options.IgnoreText)
		if masked[i] {
			node.Name, node.Text, node.Value = "", "", ""
			delete(node.Attrs, "placeholder")
			delete(node.Attrs, "aria-label")
		}
		node.Children = nil
		for _, id := range original.Children {
			if child, ok := byID[id]; ok && !ignored[child] {
				node.Children = append(node.Children, id)
			}
		}
		if filteredIdentity {
			node.Fingerprint = compareFilteredFingerprint(node)
			node.TextLength = len([]rune(node.Text))
			if len(node.Children) != len(original.Children) {
				node.Descendants = len(node.Children)
			}
		}
		observation.Tree = append(observation.Tree, node)
		originalIndices = append(originalIndices, i)
	}
	observation.URLOrScreen = normalizeCompareString(observation.URLOrScreen, options.IgnoreText)
	observation.Title = normalizeCompareString(observation.Title, options.IgnoreText)
	observation.Text = removeText(observation.Text, suppressed)
	return observation, originalIndices
}

func compareFilteredFingerprint(node api.Node) string {
	preview := func(value string) string {
		runes := []rune(value)
		return string(runes[:min(len(runes), 80)])
	}
	parts := []string{node.Attrs["tag"], node.Role, node.Attrs["id"], node.Attrs["name"],
		firstNonEmpty(node.Attrs["data-testid"], node.Attrs["data-test"]), node.Attrs["aria-label"],
		node.Attrs["href"], node.Attrs["placeholder"], preview(node.Name), preview(node.Text)}
	payload, _ := json.Marshal(parts)
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("filtered:v2:%x", sum[:16])
}
