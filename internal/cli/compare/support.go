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
	fmt.Fprintln(w, "usage: nxctl compare <old-url> <new-url> [--backend chromium|lightpanda] [--viewport <width>x<height>] [--wait-selector <css>] [--scope-selector <css>] [--old-scope-selector <css>] [--new-scope-selector <css>] [--wait-function <js>] [--wait-network-idle] [--wait-timeout <ms>] [--compare-css] [--css-property <name>]... [--compare-layout] [--ignore-text-regex <regex>]... [--ignore-selector <rule>]... [--mask-selector <rule>]... [--output-json <file>] [--output-md <file>] [--json]")
	fmt.Fprintln(w, "   or: nxctl compare --old-session <id> --new-session <id> [--wait-selector <css>] [--scope-selector <css>] [--old-scope-selector <css>] [--new-scope-selector <css>] [--wait-function <js>] [--wait-network-idle] [--wait-timeout <ms>] [--compare-css] [--css-property <name>]... [--compare-layout] [--ignore-text-regex <regex>]... [--ignore-selector <rule>]... [--mask-selector <rule>]... [--output-json <file>] [--output-md <file>] [--json]")
	fmt.Fprintln(w, "   or: nxctl compare --manifest <file> [--continue-on-error] [--limit <n>] [--output-json <file>] [--output-md <file>] [--json]")
	fmt.Fprintln(w, "rules: @eN, role=<value>, name=<value>, text=<value>, testid=<value>, href=<value>, role=<value>&name=<value>")
	fmt.Fprintln(w, "css: --compare-css uses the default property allowlist, --css-property overrides it with explicit properties")
	fmt.Fprintln(w, "layout: --compare-layout reports significant viewport-relative bounds changes for matching nodes")
	fmt.Fprintln(w, "scope: --scope-selector applies to both sides; --old-scope-selector and --new-scope-selector override it per side")
	fmt.Fprintln(w, "scope selectors accept raw CSS selectors, must match exactly one element on their side, and may use positional selectors such as :nth-child()")
	fmt.Fprintln(w, "manifest: defaults and pages support backend, viewport, wait_*, scope_selector, old_scope_selector, new_scope_selector, compare_css, compare_layout, css_property, ignore_selector, and mask_selector")
	fmt.Fprintln(w, "")
	printDocLink(w, "compare guide", aiCompareDocURL)
	printDocLink(w, "migration playbook", migrationPlaybookDocURL)
	printDocLink(w, "ai guide", aiUsageDocURL)
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
