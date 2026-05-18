package comparecmd

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func PrintHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl compare <old-url> <new-url> [--backend chromium|lightpanda] [--viewport <width>x<height>] [--match-mode exact|stable|heuristic|histogram] [--node-scope current|actionable|semantic|all] [--matching-debug] [--decisions-file <jsonl>] [--output-decisions-template <jsonl>] [--output-finding-decisions-template <jsonl>] [--review-dir <dir>] [--wait-selector <css>] [--scope-selector <css>] [--old-scope-selector <css>] [--new-scope-selector <css>] [--wait-function <js>] [--wait-network-idle] [--wait-timeout <ms>] [--compare-css] [--css-property <name>]... [--compare-layout] [--ignore-text-regex <regex>]... [--ignore-selector <rule>]... [--mask-selector <rule>]... [--output-json <file>] [--output-md <file>] [--json]")
	fmt.Fprintln(w, "   or: nxctl compare --old-session <id> --new-session <id> [--match-mode exact|stable|heuristic|histogram] [--node-scope current|actionable|semantic|all] [--matching-debug] [--decisions-file <jsonl>] [--output-decisions-template <jsonl>] [--output-finding-decisions-template <jsonl>] [--review-dir <dir>] [--wait-selector <css>] [--scope-selector <css>] [--old-scope-selector <css>] [--new-scope-selector <css>] [--wait-function <js>] [--wait-network-idle] [--wait-timeout <ms>] [--compare-css] [--css-property <name>]... [--compare-layout] [--ignore-text-regex <regex>]... [--ignore-selector <rule>]... [--mask-selector <rule>]... [--output-json <file>] [--output-md <file>] [--json]")
	fmt.Fprintln(w, "   or: nxctl compare --manifest <file> [--matching-debug] [--decisions-file <jsonl>] [--review-dir <dir>] [--continue-on-error] [--limit <n>] [--output-json <file>] [--output-md <file>] [--json]")
	fmt.Fprintln(w, "   or: nxctl compare validate-decisions --decisions-file <jsonl> [--compare-json <file>] [--review-summary <file>] [--old-session <id>] [--new-session <id>] [--json]")
	fmt.Fprintln(w, "   or: nxctl compare normalize-decisions --decisions-file <jsonl> [--compare-json <file>] [--review-summary <file>] [--output <jsonl>] [--json]")
	fmt.Fprintln(w, "   or: nxctl compare materialize-decisions --decisions-file <jsonl> --compare-json <file> [--old-session <id>] [--new-session <id>] [--output <jsonl>] [--json]")
	fmt.Fprintln(w, "   or: nxctl compare audit-decisions --decisions-file <jsonl> --compare-json <file> [--json]")
	fmt.Fprintln(w, "rules: @eN, role=<value>, name=<value>, text=<value>, testid=<value>, href=<value>, role=<value>&name=<value>")
	fmt.Fprintln(w, "css: --compare-css uses the default property allowlist, --css-property overrides it with explicit properties")
	fmt.Fprintln(w, "layout: --compare-layout reports significant viewport-relative bounds changes for matching nodes")
	fmt.Fprintln(w, "matching: --match-mode exact preserves fingerprint matching, stable uses unique identity keys, heuristic adds conservative fuzzy matching, histogram experimentally anchors low-occurrence semantic keys before local matching")
	fmt.Fprintln(w, "matching debug: --matching-debug includes anchors, regions, ambiguous candidates, and unmatched nodes in json and markdown reports")
	fmt.Fprintln(w, "decisions: --decisions-file reads JSONL entries and applies high-confidence pair/subtree_pair decisions before automatic matching, plus finding_id decisions after finding generation")
	fmt.Fprintln(w, "decision validation: validate-decisions can preflight session-backed old_selector/new_selector fields with --old-session/--new-session before materialization")
	fmt.Fprintln(w, "decision materialize: materialize-decisions resolves old_locator/new_locator and session-backed old_selector/new_selector fields to current refs before compare")
	fmt.Fprintln(w, "decisions template: --output-decisions-template writes editable JSONL stubs with locator and unique selector hints for ambiguous and unmatched old/new nodes")
	fmt.Fprintln(w, "finding decisions template: --output-finding-decisions-template writes editable JSONL stubs for critical and warning findings")
	fmt.Fprintln(w, "review packet: --review-dir writes REVIEW.md, compare.json, compare.md, pair/finding/cluster decision templates, decision audit counts, full-page screenshots, cropped finding screenshots, finding clusters, and review-summary.json")
	fmt.Fprintln(w, "node scope: --node-scope current preserves existing candidates, actionable narrows to controls, semantic includes named/content semantic nodes, all observes every visible element inside an explicit scope")
	fmt.Fprintln(w, "scope: --scope-selector applies to both sides; --old-scope-selector and --new-scope-selector override it per side")
	fmt.Fprintln(w, "scope selectors accept raw CSS selectors, must match exactly one element on their side, and may use positional selectors such as :nth-child()")
	fmt.Fprintln(w, "scope selector multi-match errors include up to five matched candidate hints")
	fmt.Fprintln(w, "manifest: defaults and pages support backend, viewport, match_mode, node_scope, matching_debug, decisions_file, wait_*, scope_selector, old_scope_selector, new_scope_selector, compare_css, compare_layout, css_property, ignore_selector, and mask_selector")
	fmt.Fprintln(w, "manifest review packet: --manifest with --review-dir writes root REVIEW.md, manifest summaries, cluster decisions, review-index.md/html, finding clusters, cropped finding previews, and one review packet directory per page")
	fmt.Fprintln(w, "")
	printDocLink(w, "compare guide", aiCompareDocURL)
	printDocLink(w, "migration playbook", migrationPlaybookDocURL)
	printDocLink(w, "ai guide", aiUsageDocURL)
}

func PrintValidateDecisionsHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl compare validate-decisions --decisions-file <jsonl> [--compare-json <file>] [--review-summary <file>] [--old-session <id>] [--new-session <id>] [--json]")
	fmt.Fprintln(w, "validates compare decision JSONL syntax, supported kinds, duplicate high-confidence pairs/subtrees/finding decisions, current refs/fingerprints/finding_ids from --compare-json, finding clusters from --review-summary, and session-backed selector uniqueness when --old-session or --new-session is supplied")
	fmt.Fprintln(w, "selector preflight requires --compare-json and checks that each old_selector/new_selector matches one live DOM node that maps uniquely to one compare JSON node")
	fmt.Fprintln(w, "")
	printDocLink(w, "compare guide", aiCompareDocURL)
}

func PrintNormalizeDecisionsHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl compare normalize-decisions --decisions-file <jsonl> [--compare-json <file>] [--review-summary <file>] [--output <jsonl>] [--json]")
	fmt.Fprintln(w, "normalizes compare decision JSONL tokens, materializes finding-cluster decisions from --compare-json or --review-summary, removes duplicate decisions, and validates current refs/fingerprints/finding_ids")
	fmt.Fprintln(w, "without --output, normalized JSONL is written to stdout unless --json is used")
	fmt.Fprintln(w, "")
	printDocLink(w, "compare guide", aiCompareDocURL)
}

func PrintMaterializeDecisionsHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl compare materialize-decisions --decisions-file <jsonl> --compare-json <file> [--old-session <id>] [--new-session <id>] [--output <jsonl>] [--json]")
	fmt.Fprintln(w, "resolves old_locator/new_locator fields against compare JSON nodes and old_selector/new_selector fields through live sessions, then writes concrete old/new refs")
	fmt.Fprintln(w, "locator terms support @eN, role:button, name:Save, label:Save, text:Login, href:/jobs, testid:submit, fingerprint:<value>, and role=button&name=Save")
	fmt.Fprintln(w, "selector materialization requires --old-session for old_selector and --new-session for new_selector; each selector must resolve to one live node that maps to one compare JSON node")
	fmt.Fprintln(w, "--json includes materialized[] entries with line, side, source, value, ref, and matched_by for review")
	fmt.Fprintln(w, "without --output, materialized JSONL is written to stdout unless --json is used")
	fmt.Fprintln(w, "")
	printDocLink(w, "compare guide", aiCompareDocURL)
}

func PrintAuditDecisionsHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl compare audit-decisions --decisions-file <jsonl> --compare-json <file> [--json]")
	fmt.Fprintln(w, "audits whether reviewed decisions are applied, pending, stale, or conflicting against a current compare report")
	fmt.Fprintln(w, "pair and subtree application checks require compare json produced with --matching-debug")
	fmt.Fprintln(w, "")
	printDocLink(w, "compare guide", aiCompareDocURL)
}

func isHelpArgs(args []string) bool {
	return len(args) == 1 && (args[0] == "-h" || args[0] == "--help")
}

func parseCommandFlags(fs *flag.FlagSet, args []string, stderr io.Writer, command string) error {
	normalized := normalizeFlagArgs(fs, args)
	output := fs.Output()
	var buf bytes.Buffer
	fs.SetOutput(&buf)
	defer fs.SetOutput(output)

	if err := fs.Parse(normalized); err != nil {
		message := strings.TrimSpace(buf.String())
		if message != "" {
			fmt.Fprintln(stderr, message)
		}
		fmt.Fprintf(stderr, "hint: run `nxctl help %s` for details\n", command)
		return err
	}

	return nil
}

func normalizeFlagArgs(fs *flag.FlagSet, args []string) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}

		name, hasValue := parseFlagToken(arg)
		flags = append(flags, arg)
		if hasValue {
			continue
		}

		defined := fs.Lookup(name)
		if defined == nil || isBoolFlag(defined) {
			continue
		}
		if i+1 >= len(args) {
			continue
		}

		flags = append(flags, args[i+1])
		i++
	}

	return append(flags, positionals...)
}

func parseFlagToken(arg string) (string, bool) {
	trimmed := strings.TrimLeft(arg, "-")
	if trimmed == "" {
		return "", false
	}
	if index := strings.IndexByte(trimmed, '='); index >= 0 {
		return trimmed[:index], true
	}
	return trimmed, false
}

func isBoolFlag(def *flag.Flag) bool {
	if def == nil {
		return false
	}
	getter, ok := def.Value.(flag.Getter)
	if !ok {
		return false
	}
	_, ok = getter.Get().(bool)
	return ok
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
