package comparecmd

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func PrintHelp(w io.Writer) {
	if !printNagiCompareUsage(w) {
		usages := []string{
			compareURLUsage,
			compareEndpointUsage,
			compareManifestUsage,
			compareValidateDecisionsUsage,
			compareNormalizeDecisionsUsage,
			compareMaterializeDecisionsUsage,
			compareRepairDecisionsUsage,
			compareAuditDecisionsUsage,
		}
		for index, usage := range usages {
			prefix := "   or: "
			if index == 0 {
				prefix = "usage: "
			}
			fmt.Fprintln(w, prefix+"nxctl compare "+usage)
		}
	}
	fmt.Fprintln(w, "rules: @eN, role=<value>, name=<value>, text=<value>, testid=<value>, href=<value>, role=<value>&name=<value>")
	fmt.Fprintln(w, "css: --compare-css uses the stable default property allowlist, --css-property overrides it, and --all-css-properties exhaustively compares every computed property")
	fmt.Fprintln(w, "layout: --compare-layout reports significant viewport-relative bounds changes for matching nodes")
	fmt.Fprintln(w, "matching: --match-mode exact preserves fingerprint matching, stable uses unique identity keys, heuristic adds conservative fuzzy matching, histogram experimentally anchors low-occurrence semantic keys before local matching")
	fmt.Fprintln(w, "matching debug: --matching-debug includes anchors, regions, ambiguous candidates, and unmatched nodes in json and markdown reports")
	fmt.Fprintln(w, "decisions: --decisions-file reads JSONL entries and applies high-confidence pair/subtree_pair decisions before automatic matching, plus finding_id decisions after finding generation")
	fmt.Fprintln(w, "decision validation: validate-decisions can preflight session-backed old_selector/new_selector fields with --old-session/--new-session before materialization")
	fmt.Fprintln(w, "decision materialize: materialize-decisions resolves old_locator/new_locator and session-backed old_selector/new_selector fields to current refs before compare")
	fmt.Fprintln(w, "decision repair: repair-decisions updates stale old/new refs from selector, locator, or fingerprint metadata when they still resolve uniquely")
	fmt.Fprintln(w, "decisions template: --output-decisions-template writes editable JSONL stubs with locator and unique selector hints for ambiguous and unmatched old/new nodes")
	fmt.Fprintln(w, "finding decisions template: --output-finding-decisions-template writes editable JSONL stubs for critical and warning findings")
	fmt.Fprintln(w, "review packet: --review-dir writes REVIEW.md, compare.json, compare.md, pair/finding/cluster decision templates, decision audit counts, full-page screenshots, cropped finding screenshots, finding clusters, and review-summary.json")
	fmt.Fprintln(w, "node scope: --node-scope current preserves existing candidates, actionable narrows to controls, semantic includes named/content semantic nodes, all observes every visible element inside an explicit scope")
	fmt.Fprintln(w, "default ignores: --node-scope all suppresses common structural noise such as SVG descendants, hidden or aria-hidden nodes, script/style/link/meta/noscript, and data-nxctl-skip=true unless --no-default-ignores is set")
	fmt.Fprintln(w, "selector rules: --ignore-selector and --mask-selector support @eN, role/name/text/testid/href/tag=<value>, attr:<name>=<value>, and & to combine terms")
	fmt.Fprintln(w, "scope: --scope-selector applies to both sides; --old-scope-selector and --new-scope-selector override it per side")
	fmt.Fprintln(w, "scope selectors accept raw CSS selectors, must match exactly one element on their side, and may use positional selectors such as :nth-child()")
	fmt.Fprintln(w, "scope selector multi-match errors include up to five matched candidate hints")
	fmt.Fprintln(w, "manifest: defaults and pages support backend, viewport, match_mode, node_scope, matching_debug, decisions_file, wait_*, scope_selector, old_scope_selector, new_scope_selector, compare_css, all_css_properties, compare_layout, no_default_ignores, css_property, ignore_selector, and mask_selector; all_css_properties and css_property can not coexist in one object")
	fmt.Fprintln(w, "manifest review packet: --manifest with --review-dir writes root REVIEW.md, manifest summaries, cluster decisions, review-index.md/html, finding clusters, cropped finding previews, and one review packet directory per page")
	fmt.Fprintln(w, "")
	printDocLink(w, "compare guide", aiCompareDocURL)
	printDocLink(w, "migration playbook", migrationPlaybookDocURL)
	printDocLink(w, "ai guide", aiUsageDocURL)
}

func PrintValidateDecisionsHelp(w io.Writer) {
	if !printNagiCompareDecisionUsage(w, "validate-decisions") {
		fmt.Fprintln(w, "usage: nxctl compare "+compareValidateDecisionsUsage)
	}
	fmt.Fprintln(w, "validates compare decision JSONL syntax, supported kinds, schema_version, unknown or unused fields, duplicate high-confidence pairs/subtrees/finding decisions, current refs/fingerprints/finding_ids from --compare-json, finding clusters from --review-summary, and session-backed selector uniqueness when --old-session or --new-session is supplied")
	fmt.Fprintln(w, "--strict turns unknown or kind-unused field warnings into errors")
	fmt.Fprintln(w, "selector preflight requires --compare-json and checks that each old_selector/new_selector matches one live DOM node that maps uniquely to one compare JSON node")
	fmt.Fprintln(w, "")
	printDocLink(w, "compare guide", aiCompareDocURL)
}

func PrintNormalizeDecisionsHelp(w io.Writer) {
	if !printNagiCompareDecisionUsage(w, "normalize-decisions") {
		fmt.Fprintln(w, "usage: nxctl compare "+compareNormalizeDecisionsUsage)
	}
	fmt.Fprintln(w, "normalizes compare decision JSONL tokens, materializes finding-cluster decisions from --compare-json or --review-summary, removes duplicate decisions, and validates current refs/fingerprints/finding_ids")
	fmt.Fprintln(w, "without --output, normalized JSONL is written to stdout unless --json is used")
	fmt.Fprintln(w, "")
	printDocLink(w, "compare guide", aiCompareDocURL)
}

func PrintMaterializeDecisionsHelp(w io.Writer) {
	if !printNagiCompareDecisionUsage(w, "materialize-decisions") {
		fmt.Fprintln(w, "usage: nxctl compare "+compareMaterializeDecisionsUsage)
	}
	fmt.Fprintln(w, "resolves old_locator/new_locator fields against compare JSON nodes and old_selector/new_selector fields through live sessions, then writes concrete old/new refs")
	fmt.Fprintln(w, "locator terms support @eN, role:button, name:Save, label:Save, text:Login, href:/jobs, testid:submit, fingerprint:<value>, and role=button&name=Save")
	fmt.Fprintln(w, "selector materialization requires --old-session for old_selector and --new-session for new_selector; each selector must resolve to one live node that maps to one compare JSON node")
	fmt.Fprintln(w, "--json includes materialized[] entries with line, side, source, value, ref, and matched_by for review")
	fmt.Fprintln(w, "without --output, materialized JSONL is written to stdout unless --json is used")
	fmt.Fprintln(w, "")
	printDocLink(w, "compare guide", aiCompareDocURL)
}

func PrintRepairDecisionsHelp(w io.Writer) {
	if !printNagiCompareDecisionUsage(w, "repair-decisions") {
		fmt.Fprintln(w, "usage: nxctl compare "+compareRepairDecisionsUsage)
	}
	fmt.Fprintln(w, "repairs stale old/new refs in compare decision JSONL when old_selector/new_selector, old_locator/new_locator, or old_fingerprint/new_fingerprint resolves uniquely against the current compare JSON")
	fmt.Fprintln(w, "selector-backed repair uses --old-session for old_selector and --new-session for new_selector; unresolved or ambiguous stale refs are left unchanged and reported as warnings")
	fmt.Fprintln(w, "without --output, repaired JSONL is written to stdout unless --json is used")
	fmt.Fprintln(w, "")
	printDocLink(w, "compare guide", aiCompareDocURL)
}

func PrintAuditDecisionsHelp(w io.Writer) {
	if !printNagiCompareDecisionUsage(w, "audit-decisions") {
		fmt.Fprintln(w, "usage: nxctl compare "+compareAuditDecisionsUsage)
	}
	fmt.Fprintln(w, "audits whether reviewed decisions are applied, pending, stale, or conflicting against a current compare report")
	fmt.Fprintln(w, "with --json, entries[] explains each state with reason, expected/actual values, conflict metadata, and repair hints")
	fmt.Fprintln(w, "pair and subtree application checks require compare json produced with --matching-debug")
	fmt.Fprintln(w, "")
	printDocLink(w, "compare guide", aiCompareDocURL)
}

func resolvedViewport(value string) (int, int, error) {
	if strings.TrimSpace(value) == "" {
		return defaultViewportWidth, defaultViewportHeight, nil
	}
	return parseViewport(value)
}

func parseViewport(value string) (int, int, error) {
	normalized := strings.TrimSpace(strings.ToLower(value))
	parts := strings.Split(normalized, "x")
	if len(parts) != 2 {
		return 0, 0, errors.New("viewport must be WIDTHxHEIGHT")
	}

	width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || width <= 0 {
		return 0, 0, errors.New("viewport width must be a positive integer")
	}
	height, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || height <= 0 {
		return 0, 0, errors.New("viewport height must be a positive integer")
	}

	return width, height, nil
}

func compareFindingLocator(oldNode *compareSnapshotNode, newNode *compareSnapshotNode) string {
	switch {
	case oldNode == nil && newNode == nil:
		return ""
	case oldNode == nil:
		return compareNodeLocator(*newNode)
	case newNode == nil:
		return compareNodeLocator(*oldNode)
	default:
		return compareSharedNodeLocator(*oldNode, *newNode)
	}
}

func compareSharedNodeLocator(oldNode compareSnapshotNode, newNode compareSnapshotNode) string {
	if testID := compareSharedValue(oldNode.TestID, newNode.TestID); testID != "" {
		return compareQuotedLocator("testid", testID)
	}
	if href := compareSharedValue(oldNode.Href, newNode.Href); href != "" {
		return compareQuotedLocator("href", href)
	}
	if label := compareSharedLabel(oldNode, newNode); label != "" {
		return compareQuotedLocator("label", label)
	}
	if role := compareSharedRoleNameLocator(oldNode, newNode); role != "" {
		return role
	}
	if text := compareSharedValue(oldNode.Text, newNode.Text); text != "" {
		return compareQuotedLocator("text", text)
	}
	return ""
}

func compareNodeLocator(node compareSnapshotNode) string {
	if node.TestID != "" {
		return compareQuotedLocator("testid", node.TestID)
	}
	if node.Href != "" {
		return compareQuotedLocator("href", node.Href)
	}
	if label := compareNodeLabelLocator(node); label != "" {
		return compareQuotedLocator("label", label)
	}
	if role := compareNodeRoleNameLocator(node); role != "" {
		return role
	}
	if node.Text != "" {
		return compareQuotedLocator("text", node.Text)
	}
	return ""
}

type compareNodeSelectorCandidate struct {
	Selector string
	Matches  func(compareSnapshotNode) bool
}

func compareNodeSelector(node compareSnapshotNode, nodes []compareSnapshotNode) string {
	for _, candidate := range compareNodeSelectorCandidates(node) {
		if strings.TrimSpace(candidate.Selector) == "" || candidate.Matches == nil {
			continue
		}
		matches := 0
		for _, other := range nodes {
			if candidate.Matches(other) {
				matches++
			}
		}
		if matches == 1 {
			return candidate.Selector
		}
	}
	return ""
}

func compareNodeSelectorCandidates(node compareSnapshotNode) []compareNodeSelectorCandidate {
	tag := compareCSSSelectorTag(node.Tag)
	if tag == "" {
		return nil
	}

	candidates := []compareNodeSelectorCandidate{}
	sameTag := func(other compareSnapshotNode) bool {
		return compareCSSSelectorTag(other.Tag) == tag
	}
	addAttr := func(attr string, value string, matches func(compareSnapshotNode) bool) {
		value = strings.TrimSpace(value)
		if value == "" || matches == nil {
			return
		}
		selector := tag + "[" + attr + "=" + compareCSSString(value) + "]"
		candidates = append(candidates, compareNodeSelectorCandidate{Selector: selector, Matches: matches})
	}

	addAttr("id", node.IDAttr, func(other compareSnapshotNode) bool {
		return sameTag(other) && strings.TrimSpace(other.IDAttr) == strings.TrimSpace(node.IDAttr)
	})
	if testID := strings.TrimSpace(node.TestID); testID != "" {
		selector := tag + "[data-testid=" + compareCSSString(testID) + "]," + tag + "[data-test=" + compareCSSString(testID) + "]"
		candidates = append(candidates, compareNodeSelectorCandidate{
			Selector: selector,
			Matches: func(other compareSnapshotNode) bool {
				return sameTag(other) && strings.TrimSpace(other.TestID) == testID
			},
		})
	}
	if name := strings.TrimSpace(node.NameAttr); name != "" {
		selector := tag + "[name=" + compareCSSString(name) + "]"
		typeAttr := strings.TrimSpace(node.TypeAttr)
		if typeAttr != "" {
			selector += "[type=" + compareCSSString(typeAttr) + "]"
		}
		candidates = append(candidates, compareNodeSelectorCandidate{
			Selector: selector,
			Matches: func(other compareSnapshotNode) bool {
				if !sameTag(other) || strings.TrimSpace(other.NameAttr) != name {
					return false
				}
				return typeAttr == "" || strings.TrimSpace(other.TypeAttr) == typeAttr
			},
		})
	}
	addAttr("href", node.Href, func(other compareSnapshotNode) bool {
		return sameTag(other) && strings.TrimSpace(other.Href) == strings.TrimSpace(node.Href)
	})
	addAttr("aria-label", node.AriaLabel, func(other compareSnapshotNode) bool {
		return sameTag(other) && strings.TrimSpace(other.AriaLabel) == strings.TrimSpace(node.AriaLabel)
	})
	if selector := compareStructureKeySelector(node.StructureKey); selector != "" {
		structureKey := strings.TrimSpace(node.StructureKey)
		candidates = append(candidates, compareNodeSelectorCandidate{
			Selector: selector,
			Matches: func(other compareSnapshotNode) bool {
				return strings.TrimSpace(other.StructureKey) == structureKey
			},
		})
	}
	return candidates
}

func compareStructureKeySelector(structureKey string) string {
	parts := strings.Split(strings.TrimSpace(structureKey), ">")
	if len(parts) == 0 {
		return ""
	}
	selectorParts := make([]string, 0, len(parts))
	for index, part := range parts {
		tag, ordinal, ok := parseCompareStructureKeyPart(part)
		if !ok {
			return ""
		}
		selector := tag
		if index > 0 && ordinal > 0 {
			selector += fmt.Sprintf(":nth-of-type(%d)", ordinal)
		}
		selectorParts = append(selectorParts, selector)
	}
	return strings.Join(selectorParts, " > ")
}

func parseCompareStructureKeyPart(part string) (string, int, bool) {
	tag, rawOrdinal, ok := strings.Cut(strings.TrimSpace(part), ":")
	if !ok {
		return "", 0, false
	}
	tag = compareCSSSelectorTag(tag)
	if tag == "" {
		return "", 0, false
	}
	ordinal, err := strconv.Atoi(strings.TrimSpace(rawOrdinal))
	if err != nil || ordinal <= 0 {
		return "", 0, false
	}
	return tag, ordinal, true
}

func compareCSSSelectorTag(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return ""
	}
	return value
}

func compareCSSString(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	builder.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\\', '"':
			builder.WriteByte('\\')
			builder.WriteRune(r)
		case '\n', '\r', '\f':
			builder.WriteByte(' ')
		default:
			builder.WriteRune(r)
		}
	}
	builder.WriteByte('"')
	return builder.String()
}

func compareSharedValue(oldValue string, newValue string) string {
	if oldValue == "" || oldValue != newValue {
		return ""
	}
	return oldValue
}

func compareSharedLabel(oldNode compareSnapshotNode, newNode compareSnapshotNode) string {
	if !compareSupportsLabelLocator(oldNode) || !compareSupportsLabelLocator(newNode) {
		return ""
	}
	return compareSharedValue(oldNode.Name, newNode.Name)
}

func compareNodeLabelLocator(node compareSnapshotNode) string {
	if !compareSupportsLabelLocator(node) {
		return ""
	}
	return node.Name
}

func compareSupportsLabelLocator(node compareSnapshotNode) bool {
	if node.Editable || node.Selectable {
		return true
	}
	return strings.EqualFold(node.Role, "textbox") || strings.EqualFold(node.Role, "combobox")
}

func compareSharedRoleNameLocator(oldNode compareSnapshotNode, newNode compareSnapshotNode) string {
	if !strings.EqualFold(oldNode.Role, newNode.Role) {
		return ""
	}
	if oldNode.Name == "" || oldNode.Name != newNode.Name {
		return ""
	}
	return fmt.Sprintf("role %s --name %s", oldNode.Role, strconv.Quote(oldNode.Name))
}

func compareNodeRoleNameLocator(node compareSnapshotNode) string {
	if node.Role == "" || node.Name == "" {
		return ""
	}
	return fmt.Sprintf("role %s --name %s", node.Role, strconv.Quote(node.Name))
}

func compareQuotedLocator(kind string, value string) string {
	if value == "" {
		return ""
	}
	return kind + " " + strconv.Quote(value)
}
