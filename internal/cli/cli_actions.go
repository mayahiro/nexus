package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	nagicli "github.com/mayahiro/nagicli-go"

	"github.com/mayahiro/nexus/internal/api"
)

func runBackInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	sessionID := nagiStringValue(invocation, "session")
	asJSON := nagiBoolValue(invocation, "json")

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: sessionID,
		Action: api.Action{
			Kind: "back",
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

	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(res.Result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	if res.Result.Message != "" {
		fmt.Fprintln(stdout, res.Result.Message)
		return 0
	}

	fmt.Fprintln(stdout, "went back")
	return 0
}

func runClickInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	arguments := nagiClickArgumentsFromInvocation(invocation)
	refsValue := strings.TrimSpace(arguments.Refs)
	if refsValue != "" {
		nodes, err := parseNodeSelectorList(refsValue)
		if err != nil {
			fmt.Fprintln(stderr, "click --refs requires comma-separated positive integer indexes or @eN refs")
			return 1
		}
		return runClickRefs(ctx, arguments.SessionID, nodes, arguments.JSON, stdout, stderr)
	}

	action := api.Action{Kind: "invoke"}
	fallbackMessage := ""
	useNodeRefMessage := false
	if len(arguments.Positionals) == 1 {
		nodeID, nodeRef, err := parseNodeSelector(arguments.Positionals[0])
		if err != nil {
			fmt.Fprintln(stderr, "click requires a positive integer index or @eN ref")
			return 1
		}
		action.NodeID = &nodeID
		if nodeRef != "" {
			fallbackMessage = fmt.Sprintf("clicked %s", nodeRef)
			useNodeRefMessage = true
		} else {
			fallbackMessage = fmt.Sprintf("clicked %d", nodeID)
		}
	} else {
		x, err := strconv.Atoi(arguments.Positionals[0])
		if err != nil || x < 0 {
			fmt.Fprintln(stderr, "click requires non-negative integer x y coordinates")
			return 1
		}
		y, err := strconv.Atoi(arguments.Positionals[1])
		if err != nil || y < 0 {
			fmt.Fprintln(stderr, "click requires non-negative integer x y coordinates")
			return 1
		}
		action.Args = map[string]string{
			"x": strconv.Itoa(x),
			"y": strconv.Itoa(y),
		}
		fallbackMessage = fmt.Sprintf("clicked %d %d", x, y)
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: arguments.SessionID,
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

	if arguments.JSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(res.Result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	if !useNodeRefMessage && res.Result.Message != "" {
		fmt.Fprintln(stdout, res.Result.Message)
		return 0
	}

	fmt.Fprintln(stdout, fallbackMessage)
	return 0
}

type refActionResult struct {
	Ref     string            `json:"ref"`
	OK      bool              `json:"ok"`
	Message string            `json:"message,omitempty"`
	Changed bool              `json:"changed"`
	Value   interface{}       `json:"value"`
	Meta    map[string]string `json:"meta,omitempty"`
}

func runClickRefs(ctx context.Context, sessionID string, nodes []nodeSelector, asJSON bool, stdout io.Writer, stderr io.Writer) int {
	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	results := make([]refActionResult, 0, len(nodes))
	for _, node := range nodes {
		nodeID := node.ID
		res, err := client.ActSession(ctx, api.ActSessionRequest{
			SessionID: sessionID,
			Action: api.Action{
				Kind:   "invoke",
				NodeID: &nodeID,
			},
		})
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		result := refActionResult{
			Ref:     node.Ref,
			OK:      res.Result.OK,
			Message: res.Result.Message,
			Changed: res.Result.Changed,
			Value:   res.Result.Value,
			Meta:    res.Result.Meta,
		}
		results = append(results, result)
		if !res.Result.OK {
			if res.Result.Message != "" {
				fmt.Fprintln(stderr, res.Result.Message)
			}
			return 1
		}
	}

	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(results); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	for _, result := range results {
		fmt.Fprintf(stdout, "clicked %s\n", result.Ref)
	}
	return 0
}

func runHoverInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	return runNodeActionInvocation(ctx, "hover", "hovered %d", invocation, stdout, stderr)
}

func runDblclickInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	return runNodeActionInvocation(ctx, "dblclick", "double-clicked %d", invocation, stdout, stderr)
}

func runRightclickInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	return runNodeActionInvocation(ctx, "rightclick", "right-clicked %d", invocation, stdout, stderr)
}

func runNodeActionInvocation(ctx context.Context, command string, fallbackFormat string, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	sessionID := nagiStringValue(invocation, "session")
	asJSON := nagiBoolValue(invocation, "json")
	node := nagiNodeValue(invocation, "node")
	nodeID := node.ID
	nodeRef := node.Ref

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: sessionID,
		Action: api.Action{
			Kind:   command,
			NodeID: &nodeID,
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

	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(res.Result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	if nodeRef == "" && res.Result.Message != "" {
		fmt.Fprintln(stdout, res.Result.Message)
		return 0
	}

	if nodeRef != "" {
		fmt.Fprintf(stdout, strings.ReplaceAll(fallbackFormat, "%d", "%s")+"\n", nodeRef)
		return 0
	}

	fmt.Fprintf(stdout, fallbackFormat+"\n", nodeID)
	return 0
}

func runKeysInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	sessionID := nagiStringValue(invocation, "session")
	asJSON := nagiBoolValue(invocation, "json")
	keySpec := nagiStringValue(invocation, "keys")

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: sessionID,
		Action: api.Action{
			Kind: "key",
			Keys: []string{keySpec},
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

	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(res.Result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	if res.Result.Message != "" {
		fmt.Fprintln(stdout, res.Result.Message)
		return 0
	}

	fmt.Fprintf(stdout, "sent keys %s\n", keySpec)
	return 0
}
