package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	nagicli "github.com/mayahiro/nagicli-go"

	"github.com/mayahiro/nexus/internal/api"
)

func runInputInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	node := nagiNodeValue(invocation, "node")
	return runTextAction(ctx, stdout, stderr, textActionOptions{
		SessionID: nagiStringValue(invocation, "session"),
		JSON:      nagiBoolValue(invocation, "json"),
		NodeID:    &node.ID,
		NodeRef:   node.Ref,
		Kind:      "type",
		Text:      nagiStringValue(invocation, "value"),
	})
}

func runFillInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	node := nagiNodeValue(invocation, "node")
	return runTextAction(ctx, stdout, stderr, textActionOptions{
		SessionID: nagiStringValue(invocation, "session"),
		JSON:      nagiBoolValue(invocation, "json"),
		NodeID:    &node.ID,
		NodeRef:   node.Ref,
		Kind:      "fill",
		Text:      nagiStringValue(invocation, "value"),
	})
}

func runSelectInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	sessionID := nagiStringValue(invocation, "session")
	asJSON := nagiBoolValue(invocation, "json")
	node := nagiNodeValue(invocation, "node")
	value := nagiStringValue(invocation, "value")

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: sessionID,
		Action: api.Action{
			Kind:    "select",
			NodeID:  &node.ID,
			NodeRef: node.Ref,
			Text:    value,
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

	if node.Ref == "" && res.Result.Message != "" {
		fmt.Fprintln(stdout, res.Result.Message)
		return 0
	}

	if node.Ref != "" {
		fmt.Fprintf(stdout, "selected %s on %s\n", value, node.Ref)
		return 0
	}

	fmt.Fprintf(stdout, "selected %s on %d\n", value, node.ID)
	return 0
}

func runUploadInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	sessionID := nagiStringValue(invocation, "session")
	asJSON := nagiBoolValue(invocation, "json")
	selector := strings.TrimSpace(nagiStringValue(invocation, "selector"))
	values := nagiRawValues(invocation, "values")
	path := values[len(values)-1]
	uploadPath, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	var node nodeSelector
	if selector == "" {
		nodeID, nodeRef, err := parseNodeSelector(values[0])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		node = nodeSelector{ID: nodeID, Ref: nodeRef}
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	action := api.Action{
		Kind:     "upload",
		NodeRef:  node.Ref,
		Selector: selector,
		Text:     uploadPath,
	}
	if selector == "" {
		action.NodeID = &node.ID
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
		if err := encoder.Encode(res.Result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	if selector != "" {
		fmt.Fprintf(stdout, "uploaded %s to %s\n", path, selector)
		return 0
	}
	if node.Ref == "" && res.Result.Message != "" {
		fmt.Fprintln(stdout, res.Result.Message)
		return 0
	}

	if node.Ref != "" {
		fmt.Fprintf(stdout, "uploaded %s to %s\n", path, node.Ref)
		return 0
	}

	fmt.Fprintf(stdout, "uploaded %s to %d\n", path, node.ID)
	return 0
}

func runTypeInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	return runTextAction(ctx, stdout, stderr, textActionOptions{
		SessionID: nagiStringValue(invocation, "session"),
		JSON:      nagiBoolValue(invocation, "json"),
		Kind:      "type",
		Text:      nagiStringValue(invocation, "value"),
	})
}

type textActionOptions struct {
	SessionID string
	JSON      bool
	NodeID    *int
	NodeRef   string
	Kind      string
	Text      string
}

func runTextAction(ctx context.Context, stdout io.Writer, stderr io.Writer, opts textActionOptions) int {
	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: opts.SessionID,
		Action: api.Action{
			Kind:    opts.Kind,
			NodeID:  opts.NodeID,
			NodeRef: opts.NodeRef,
			Text:    opts.Text,
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

	if opts.JSON {
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

	fmt.Fprintln(stdout, defaultTextActionMessage(opts.Kind))
	return 0
}

func defaultTextActionMessage(kind string) string {
	if kind == "fill" {
		return "filled"
	}
	return "typed"
}
