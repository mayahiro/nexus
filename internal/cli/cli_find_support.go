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

type nodeSelectionOptions struct {
	Nth int
}

func chooseNode(matches []api.Node, query string, options nodeSelectionOptions) (api.Node, error) {
	if len(matches) == 0 {
		return api.Node{}, errors.New("matching node not found")
	}
	if options.Nth > 0 {
		if options.Nth <= len(matches) {
			return matches[options.Nth-1], nil
		}
		return api.Node{}, nthNodeError(matches, options.Nth)
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
	return fmt.Errorf("multiple matching nodes found: %s. narrow the query, use --nth <n>, or use @eN from `nxctl state`", formatNodeCandidates(nodes))
}

func nthNodeError(nodes []api.Node, nth int) error {
	return fmt.Errorf("match %d not found: %d candidate nodes available: %s", nth, len(nodes), formatNodeCandidates(nodes))
}

func formatNodeCandidates(nodes []api.Node) string {
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
	return strings.Join(parts, ", ")
}

func isInvalidNthFlag(fs interface{ Visit(func(*flag.Flag)) }, nth int) bool {
	if nth > 0 {
		return false
	}
	invalid := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "nth" {
			invalid = true
		}
	})
	return invalid
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
	case "fill":
		if actionValue == "" {
			fmt.Fprintln(stderr, `fill action requires "text"`)
			return 1
		}
		action = api.Action{Kind: "fill", NodeID: &node.ID, Text: actionValue}
		fallbackMessage = fmt.Sprintf("filled into %s", displayNodeRef(node))
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

type nodeSelector struct {
	ID  int
	Ref string
}

func parseNodeSelectorList(value string) ([]nodeSelector, error) {
	parts := strings.Split(value, ",")
	nodes := make([]nodeSelector, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		nodeID, nodeRef, err := parseNodeSelector(part)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[nodeID]; ok {
			return nil, errors.New("duplicate node ref")
		}
		seen[nodeID] = struct{}{}
		if nodeRef == "" {
			nodeRef = formatNodeRef(nodeID)
		}
		nodes = append(nodes, nodeSelector{ID: nodeID, Ref: nodeRef})
	}
	if len(nodes) == 0 {
		return nil, errors.New("node refs are required")
	}
	return nodes, nil
}

func formatNodeRef(id int) string {
	return fmt.Sprintf("@e%d", id)
}
