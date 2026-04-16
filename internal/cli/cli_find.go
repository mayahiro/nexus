package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/mayahiro/nexus/internal/api"
)

func runFind(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printFindHelp(stdout)
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "find requires a selector kind such as role, text, label, testid, or href")
		printCommandHint(stderr, "find", `nxctl find role button --name "Submit"`)
		return 1
	}

	switch args[0] {
	case "role":
		return runFindRole(ctx, args[1:], stdout, stderr)
	case "text":
		return runFindText(ctx, args[1:], stdout, stderr)
	case "label":
		return runFindLabel(ctx, args[1:], stdout, stderr)
	case "testid":
		return runFindAttr(ctx, "testid", []string{"data-testid", "data-test"}, args[1:], stdout, stderr)
	case "href":
		return runFindAttr(ctx, "href", []string{"href"}, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "find target must be role, text, label, testid, or href\n")
		printCommandHint(stderr, "find", `nxctl find role button --name "Submit"`)
		return 1
	}
}

func runFindRole(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("find role", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	matchAll := fs.Bool("all", false, "list all matching nodes")
	nth := fs.Int("nth", 0, "choose the nth matching node")
	name := fs.String("name", "", "accessible name")
	role := ""
	actionName := ""
	actionValue := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		role = args[0]
		args = args[1:]
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		actionName = args[0]
		args = args[1:]
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		actionValue = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "find"); err != nil {
		return 1
	}

	if role == "" || (!*matchAll && actionName == "") {
		fmt.Fprintln(stderr, "find role requires <role> <click|input|fill|get> or --all")
		printCommandHint(stderr, "find", `nxctl find role button click --name "Submit"`)
		return 1
	}
	if *matchAll && actionName != "" {
		fmt.Fprintln(stderr, "find role --all does not accept an action")
		return 1
	}
	if isInvalidNthFlag(fs, *nth) {
		fmt.Fprintln(stderr, "find role --nth must be a positive integer")
		return 1
	}
	if *matchAll && *nth > 0 {
		fmt.Fprintln(stderr, "find role --all does not accept --nth")
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	observation, err := observeTreeForFind(ctx, client, *sessionID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	nodes := selectNodes(observation.Tree, func(node api.Node) bool {
		if !strings.EqualFold(strings.TrimSpace(node.Role), strings.TrimSpace(role)) {
			return false
		}
		if strings.TrimSpace(*name) == "" {
			return true
		}
		return nodeMatches(node, *name)
	})
	if *matchAll {
		return renderFindMatches(nodes, *asJSON, stdout, stderr)
	}
	node, err := chooseNode(nodes, *name, nodeSelectionOptions{Nth: *nth})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	return executeFoundAction(ctx, client, *sessionID, node, actionName, actionValue, *asJSON, stdout, stderr)
}

func runFindText(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("find text", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	matchAll := fs.Bool("all", false, "list all matching nodes")
	nth := fs.Int("nth", 0, "choose the nth matching node")
	textValue := ""
	actionName := ""
	actionValue := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		textValue = args[0]
		args = args[1:]
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		actionName = args[0]
		args = args[1:]
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		actionValue = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "find"); err != nil {
		return 1
	}

	if textValue == "" || (!*matchAll && actionName == "") {
		fmt.Fprintln(stderr, "find text requires <text> <click|fill|get> or --all")
		printCommandHint(stderr, "find", `nxctl find text "Sign In" click`)
		return 1
	}
	if *matchAll && actionName != "" {
		fmt.Fprintln(stderr, "find text --all does not accept an action")
		return 1
	}
	if isInvalidNthFlag(fs, *nth) {
		fmt.Fprintln(stderr, "find text --nth must be a positive integer")
		return 1
	}
	if *matchAll && *nth > 0 {
		fmt.Fprintln(stderr, "find text --all does not accept --nth")
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	observation, err := observeTreeForFind(ctx, client, *sessionID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	nodes := selectNodes(observation.Tree, func(node api.Node) bool {
		return nodeMatches(node, textValue)
	})
	if *matchAll {
		return renderFindMatches(nodes, *asJSON, stdout, stderr)
	}
	node, err := chooseNode(nodes, textValue, nodeSelectionOptions{Nth: *nth})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	return executeFoundAction(ctx, client, *sessionID, node, actionName, actionValue, *asJSON, stdout, stderr)
}

func runFindLabel(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("find label", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	matchAll := fs.Bool("all", false, "list all matching nodes")
	nth := fs.Int("nth", 0, "choose the nth matching node")
	label := ""
	actionName := ""
	actionValue := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		label = args[0]
		args = args[1:]
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		actionName = args[0]
		args = args[1:]
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		actionValue = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "find"); err != nil {
		return 1
	}

	if label == "" || (!*matchAll && actionName == "") {
		fmt.Fprintln(stderr, `find label requires "label" input|fill "text", get <target>, or --all`)
		printCommandHint(stderr, "find", `nxctl find label "Email" fill "hello@example.com"`)
		return 1
	}
	if *matchAll && actionName != "" {
		fmt.Fprintln(stderr, "find label --all does not accept an action")
		return 1
	}
	if isInvalidNthFlag(fs, *nth) {
		fmt.Fprintln(stderr, "find label --nth must be a positive integer")
		return 1
	}
	if *matchAll && *nth > 0 {
		fmt.Fprintln(stderr, "find label --all does not accept --nth")
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	observation, err := observeTreeForFind(ctx, client, *sessionID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	nodes := selectNodes(observation.Tree, func(node api.Node) bool {
		if !node.Editable && !node.Selectable && !strings.EqualFold(node.Role, "textbox") && !strings.EqualFold(node.Role, "combobox") {
			return false
		}
		return nodeMatches(node, label)
	})
	if *matchAll {
		return renderFindMatches(nodes, *asJSON, stdout, stderr)
	}
	node, err := chooseNode(nodes, label, nodeSelectionOptions{Nth: *nth})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	return executeFoundAction(ctx, client, *sessionID, node, actionName, actionValue, *asJSON, stdout, stderr)
}

func runFindAttr(ctx context.Context, kind string, attrs []string, args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("find "+kind, flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	matchAll := fs.Bool("all", false, "list all matching nodes")
	nth := fs.Int("nth", 0, "choose the nth matching node")
	attrValue := ""
	actionName := ""
	actionValue := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		attrValue = args[0]
		args = args[1:]
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		actionName = args[0]
		args = args[1:]
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		actionValue = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "find"); err != nil {
		return 1
	}

	if attrValue == "" || (!*matchAll && actionName == "") {
		fmt.Fprintf(stderr, "find %s requires <value> <click|fill|get> or --all\n", kind)
		printCommandHint(stderr, "find", fmt.Sprintf(`nxctl find %s "value" --all`, kind))
		return 1
	}
	if *matchAll && actionName != "" {
		fmt.Fprintf(stderr, "find %s --all does not accept an action\n", kind)
		return 1
	}
	if isInvalidNthFlag(fs, *nth) {
		fmt.Fprintf(stderr, "find %s --nth must be a positive integer\n", kind)
		return 1
	}
	if *matchAll && *nth > 0 {
		fmt.Fprintf(stderr, "find %s --all does not accept --nth\n", kind)
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	observation, err := observeTreeForFind(ctx, client, *sessionID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	nodes := selectNodes(observation.Tree, func(node api.Node) bool {
		needle := normalizeFindValue(attrValue)
		for _, attr := range attrs {
			if strings.Contains(normalizeFindValue(node.Attrs[attr]), needle) {
				return true
			}
		}
		return false
	})
	if *matchAll {
		return renderFindMatches(nodes, *asJSON, stdout, stderr)
	}
	node, err := chooseNode(nodes, attrValue, nodeSelectionOptions{Nth: *nth})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	return executeFoundAction(ctx, client, *sessionID, node, actionName, actionValue, *asJSON, stdout, stderr)
}
