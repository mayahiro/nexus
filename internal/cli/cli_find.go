package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/rpc"
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
		fmt.Fprintln(stderr, "find role requires <role> <click|input|get> or --all")
		printCommandHint(stderr, "find", `nxctl find role button click --name "Submit"`)
		return 1
	}
	if *matchAll && actionName != "" {
		fmt.Fprintln(stderr, "find role --all does not accept an action")
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
	node, err := chooseNode(nodes, *name)
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
	textValue := ""
	actionName := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		textValue = args[0]
		args = args[1:]
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		actionName = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "find"); err != nil {
		return 1
	}

	if textValue == "" || (!*matchAll && actionName == "") {
		fmt.Fprintln(stderr, "find text requires <text> <click|get> or --all")
		printCommandHint(stderr, "find", `nxctl find text "Sign In" click`)
		return 1
	}
	if *matchAll && actionName != "" {
		fmt.Fprintln(stderr, "find text --all does not accept an action")
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
	node, err := chooseNode(nodes, textValue)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	return executeFoundAction(ctx, client, *sessionID, node, actionName, "", *asJSON, stdout, stderr)
}

func runFindLabel(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("find label", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	matchAll := fs.Bool("all", false, "list all matching nodes")
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
		fmt.Fprintln(stderr, `find label requires "label" input "text", get <target>, or --all`)
		printCommandHint(stderr, "find", `nxctl find label "Email" input "hello@example.com"`)
		return 1
	}
	if *matchAll && actionName != "" {
		fmt.Fprintln(stderr, "find label --all does not accept an action")
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
	node, err := chooseNode(nodes, label)
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
		fmt.Fprintf(stderr, "find %s requires <value> <click|get> or --all\n", kind)
		printCommandHint(stderr, "find", fmt.Sprintf(`nxctl find %s "value" --all`, kind))
		return 1
	}
	if *matchAll && actionName != "" {
		fmt.Fprintf(stderr, "find %s --all does not accept an action\n", kind)
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
	node, err := chooseNode(nodes, attrValue)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	return executeFoundAction(ctx, client, *sessionID, node, actionName, actionValue, *asJSON, stdout, stderr)
}

func observeTreeForFind(ctx context.Context, client *rpc.Client, sessionID string) (api.Observation, error) {
	res, err := client.ObserveSession(ctx, api.ObserveSessionRequest{
		SessionID: sessionID,
		Options: api.ObserveOptions{
			WithTree: true,
			WithText: true,
		},
	})
	if err != nil {
		return api.Observation{}, err
	}
	return res.Observation, nil
}

func selectNodes(nodes []api.Node, match func(api.Node) bool) []api.Node {
	matches := make([]api.Node, 0, len(nodes))
	for _, node := range nodes {
		if match(node) {
			matches = append(matches, node)
		}
	}
	return matches
}

func chooseNode(matches []api.Node, query string) (api.Node, error) {
	if len(matches) == 0 {
		return api.Node{}, errors.New("matching node not found")
	}
	if len(matches) == 1 {
		return matches[0], nil
	}

	needle := normalizeFindValue(query)
	if needle == "" {
		return api.Node{}, ambiguousNodeError(matches)
	}

	bestScore := 0
	bestMatches := make([]api.Node, 0, len(matches))
	for _, node := range matches {
		score := nodeMatchScore(node, needle)
		switch {
		case score > bestScore:
			bestScore = score
			bestMatches = []api.Node{node}
		case score == bestScore:
			bestMatches = append(bestMatches, node)
		}
	}

	if bestScore > 0 && len(bestMatches) == 1 {
		return bestMatches[0], nil
	}
	if bestScore > 0 {
		return api.Node{}, ambiguousNodeError(bestMatches)
	}

	return api.Node{}, ambiguousNodeError(matches)
}

func nodeMatches(node api.Node, value string) bool {
	return nodeMatchScore(node, normalizeFindValue(value)) > 0
}

func normalizeFindValue(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func nodeMatchScore(node api.Node, needle string) int {
	if needle == "" {
		return 0
	}

	best := 0
	for _, candidate := range nodeMatchCandidates(node) {
		normalized := normalizeFindValue(candidate)
		switch {
		case normalized == "":
		case normalized == needle:
			return 3
		case strings.HasPrefix(normalized, needle):
			if best < 2 {
				best = 2
			}
		case strings.Contains(normalized, needle):
			if best < 1 {
				best = 1
			}
		}
	}
	return best
}

func nodeMatchCandidates(node api.Node) []string {
	return []string{
		node.Name,
		node.Text,
		node.Value,
		node.Attrs["aria-label"],
		node.Attrs["placeholder"],
		node.Attrs["name"],
		node.Attrs["data-testid"],
		node.Attrs["data-test"],
		node.Attrs["href"],
	}
}

func ambiguousNodeError(nodes []api.Node) error {
	parts := make([]string, 0, len(nodes))
	for i, node := range nodes {
		if i == 5 {
			parts = append(parts, "...")
			break
		}

		label := displayNodeRef(node)
		text := node.Name
		if text == "" {
			text = node.Text
		}
		if text != "" {
			parts = append(parts, fmt.Sprintf("%s %s %q", label, node.Role, text))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %s", label, node.Role))
	}
	return fmt.Errorf("multiple matching nodes found: %s. narrow the query or use @eN from `nxctl state`", strings.Join(parts, ", "))
}

func executeFoundAction(ctx context.Context, client *rpc.Client, sessionID string, node api.Node, actionName string, actionValue string, asJSON bool, stdout io.Writer, stderr io.Writer) int {
	action := api.Action{}
	fallbackMessage := ""

	switch actionName {
	case "click":
		if actionValue != "" {
			fmt.Fprintln(stderr, "click action does not accept an extra value")
			return 1
		}
		action = api.Action{Kind: "invoke", NodeID: &node.ID}
		fallbackMessage = fmt.Sprintf("clicked %s", displayNodeRef(node))
	case "input":
		if actionValue == "" {
			fmt.Fprintln(stderr, `input action requires "text"`)
			return 1
		}
		action = api.Action{Kind: "type", NodeID: &node.ID, Text: actionValue}
		fallbackMessage = fmt.Sprintf("typed into %s", displayNodeRef(node))
	case "get":
		if !isFindGetTarget(actionValue) {
			fmt.Fprintln(stderr, "get action requires text, value, attributes, or bbox")
			return 1
		}
		action = api.Action{
			Kind:   "get",
			NodeID: &node.ID,
			Args: map[string]string{
				"target": actionValue,
			},
		}
	default:
		fmt.Fprintf(stderr, "unsupported find action: %s\n", actionName)
		return 1
	}

	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: sessionID,
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

	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(map[string]interface{}{
			"match":  node,
			"result": res.Result,
		}); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	if action.Kind == "get" {
		if err := printEvalValue(stdout, res.Result.Value); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	fmt.Fprintln(stdout, fallbackMessage)
	return 0
}

func isFindGetTarget(value string) bool {
	switch value {
	case "text", "value", "attributes", "bbox":
		return true
	default:
		return false
	}
}

func displayNodeRef(node api.Node) string {
	if node.Ref != "" {
		return node.Ref
	}
	return fmt.Sprintf("%d", node.ID)
}

func renderFindMatches(nodes []api.Node, asJSON bool, stdout io.Writer, stderr io.Writer) int {
	if len(nodes) == 0 {
		fmt.Fprintln(stderr, "matching node not found")
		return 1
	}
	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(nodes); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	for _, node := range nodes {
		printNode(stdout, node)
	}
	return 0
}

func parseNodeSelector(value string) (int, string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, "", errors.New("node selector is required")
	}
	if strings.HasPrefix(trimmed, "@e") {
		nodeID, err := strconv.Atoi(strings.TrimPrefix(trimmed, "@e"))
		if err != nil || nodeID <= 0 {
			return 0, "", errors.New("invalid node ref")
		}
		return nodeID, formatNodeRef(nodeID), nil
	}

	nodeID, err := strconv.Atoi(trimmed)
	if err != nil || nodeID <= 0 {
		return 0, "", errors.New("invalid node index")
	}
	return nodeID, "", nil
}

func formatNodeRef(id int) string {
	return fmt.Sprintf("@e%d", id)
}
