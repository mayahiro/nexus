package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/mayahiro/nexus/internal/api"
)

func runEval(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printEvalHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("eval", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	source := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		source = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "eval"); err != nil {
		return 1
	}

	if source == "" && fs.NArg() == 1 {
		source = fs.Arg(0)
	}

	if source == "" || fs.NArg() > 1 {
		fmt.Fprintln(stderr, "eval requires js code")
		printCommandHint(stderr, "eval", `nxctl eval "document.title" --json`)
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: *sessionID,
		Action: api.Action{
			Kind: "eval",
			Text: source,
		},
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if !res.Result.OK {
		if res.Result.Message != "" {
			fmt.Fprintln(stderr, res.Result.Message)
		}
		return 1
	}

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(res.Result.Value); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	if err := printEvalValue(stdout, res.Result.Value); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	return 0
}

func runGet(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printGetHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	selector := fs.String("selector", "", "selector for html")
	target := ""
	arg := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		target = args[0]
		args = args[1:]
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		arg = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "get"); err != nil {
		return 1
	}

	positionals := make([]string, 0, 2)
	if target != "" {
		positionals = append(positionals, target)
	}
	if arg != "" {
		positionals = append(positionals, arg)
	}
	positionals = append(positionals, fs.Args()...)
	if len(positionals) == 0 {
		fmt.Fprintln(stderr, "get requires a target")
		printCommandHint(stderr, "get", "nxctl get title")
		return 1
	}

	target = positionals[0]
	action := api.Action{
		Kind: "get",
		Args: map[string]string{
			"target": target,
		},
	}

	switch target {
	case "title":
		if len(positionals) != 1 {
			fmt.Fprintln(stderr, "get title does not accept an index")
			return 1
		}
		if *selector != "" {
			fmt.Fprintln(stderr, "get title does not support --selector")
			return 1
		}
	case "html":
		if len(positionals) != 1 {
			fmt.Fprintln(stderr, "get html does not accept an index")
			return 1
		}
		if *selector != "" {
			action.Args["selector"] = *selector
		}
	case "text", "value", "attributes", "bbox":
		if len(positionals) != 2 {
			fmt.Fprintf(stderr, "get %s requires an index\n", target)
			printCommandHint(stderr, "get", "nxctl get attributes @e3")
			return 1
		}
		if *selector != "" {
			fmt.Fprintf(stderr, "get %s does not support --selector\n", target)
			return 1
		}
		nodeID, _, err := parseNodeSelector(positionals[1])
		if err != nil {
			fmt.Fprintf(stderr, "get %s requires a positive integer index or @eN ref\n", target)
			return 1
		}
		action.NodeID = &nodeID
	default:
		fmt.Fprintln(stderr, "get target must be title, html, text, value, attributes, or bbox")
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: *sessionID,
		Action:    action,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if !res.Result.OK {
		if res.Result.Message != "" {
			fmt.Fprintln(stderr, res.Result.Message)
		}
		return 1
	}

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(res.Result.Value); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	if err := printEvalValue(stdout, res.Result.Value); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	return 0
}

func runObserve(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printObserveHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("observe", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	withText := fs.Bool("text", true, "include text")
	withTree := fs.Bool("tree", true, "include tree")
	withScreenshot := fs.Bool("screenshot", false, "include screenshot")

	if err := parseCommandFlags(fs, args, stderr, "observe"); err != nil {
		return 1
	}

	if *sessionID == "" {
		fmt.Fprintln(stderr, "--session is required")
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ObserveSession(ctx, api.ObserveSessionRequest{
		SessionID: *sessionID,
		Options: api.ObserveOptions{
			WithText:       *withText,
			WithTree:       *withTree,
			WithScreenshot: *withScreenshot,
		},
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(res.Observation); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(stdout, "session: %s\n", res.Observation.SessionID)
	fmt.Fprintf(stdout, "target: %s\n", res.Observation.TargetType)
	fmt.Fprintf(stdout, "url: %s\n", res.Observation.URLOrScreen)
	fmt.Fprintf(stdout, "title: %s\n", res.Observation.Title)
	if res.Observation.Text != "" {
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, res.Observation.Text)
	}
	return 0
}

func runState(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printStateHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("state", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	role := fs.String("role", "", "filter by role")
	name := fs.String("name", "", "filter by accessible name")
	text := fs.String("text", "", "filter by text")
	testID := fs.String("testid", "", "filter by data-testid or data-test")
	href := fs.String("href", "", "filter by href")
	limit := fs.Int("limit", 0, "maximum nodes to print")

	if err := parseCommandFlags(fs, args, stderr, "state"); err != nil {
		return 1
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "state does not accept positional arguments")
		printCommandHint(stderr, "state", "nxctl state --role button --limit 20")
		return 1
	}
	if *limit < 0 {
		fmt.Fprintln(stderr, "state limit must be a non-negative integer")
		printCommandHint(stderr, "state", "nxctl state --limit 20")
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ObserveSession(ctx, api.ObserveSessionRequest{
		SessionID: *sessionID,
		Options: api.ObserveOptions{
			WithText:       true,
			WithTree:       true,
			WithScreenshot: false,
		},
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	res.Observation.Tree = filterStateNodes(res.Observation.Tree, stateFilterOptions{
		Role:   *role,
		Name:   *name,
		Text:   *text,
		TestID: *testID,
		Href:   *href,
		Limit:  *limit,
	})

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(res.Observation); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(stdout, "URL: %s\n", res.Observation.URLOrScreen)
	fmt.Fprintf(stdout, "Title: %s\n", res.Observation.Title)
	if len(res.Observation.Tree) == 0 {
		return 0
	}

	fmt.Fprintln(stdout, "")
	for _, node := range res.Observation.Tree {
		printNode(stdout, node)
	}

	return 0
}

type stateFilterOptions struct {
	Role   string
	Name   string
	Text   string
	TestID string
	Href   string
	Limit  int
}

func filterStateNodes(nodes []api.Node, options stateFilterOptions) []api.Node {
	filtered := make([]api.Node, 0, len(nodes))
	role := normalizeFindValue(options.Role)
	name := normalizeFindValue(options.Name)
	text := normalizeFindValue(options.Text)
	testID := normalizeFindValue(options.TestID)
	href := normalizeFindValue(options.Href)

	for _, node := range nodes {
		if role != "" && normalizeFindValue(node.Role) != role {
			continue
		}
		if name != "" && !matchStateField(node.Name, name) {
			continue
		}
		if text != "" && !matchStateField(node.Text, text) {
			continue
		}
		if testID != "" && !matchStateField(stateNodeTestID(node), testID) {
			continue
		}
		if href != "" && !matchStateField(node.Attrs["href"], href) {
			continue
		}
		filtered = append(filtered, node)
	}

	if options.Limit > 0 && len(filtered) > options.Limit {
		return filtered[:options.Limit]
	}
	return filtered
}

func matchStateField(value string, needle string) bool {
	normalized := normalizeFindValue(value)
	if needle == "" {
		return true
	}
	return normalized != "" && strings.Contains(normalized, needle)
}

func stateNodeTestID(node api.Node) string {
	if node.Attrs["data-testid"] != "" {
		return node.Attrs["data-testid"]
	}
	return node.Attrs["data-test"]
}

func printNode(w io.Writer, node api.Node) {
	label := node.Ref
	if label == "" {
		label = fmt.Sprintf("%d", node.ID)
	}
	fmt.Fprintf(w, "[%s] %s", label, node.Role)
	if node.Name != "" {
		fmt.Fprintf(w, " %q", node.Name)
	} else if node.Text != "" {
		fmt.Fprintf(w, " %q", node.Text)
	}
	fmt.Fprintln(w)
	if len(node.LocatorHints) > 0 {
		commands := make([]string, 0, len(node.LocatorHints))
		for _, hint := range node.LocatorHints {
			commands = append(commands, hint.Command)
		}
		fmt.Fprintf(w, "  find: %s\n", strings.Join(commands, " | "))
	}
}
