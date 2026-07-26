package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	nagicli "github.com/mayahiro/nagicli-go"

	"github.com/mayahiro/nexus/internal/api"
)

func runScrollInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	sessionID := nagiStringValue(invocation, "session")
	asJSON := nagiBoolValue(invocation, "json")
	amount := nagiIntValue(invocation, "amount")
	nodeID := nagiIntValue(invocation, "node")
	direction := nagiStringValue(invocation, "direction")

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	action := api.Action{
		Kind: "scroll",
		Dir:  direction,
	}
	if amount > 0 {
		action.Args = map[string]string{"amount": strconv.Itoa(amount)}
	}
	if nodeID > 0 {
		action.NodeID = &nodeID
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

	if res.Result.Message != "" {
		fmt.Fprintln(stdout, res.Result.Message)
		return 0
	}

	fmt.Fprintf(stdout, "scrolled %s\n", direction)
	return 0
}

func runWaitInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	sessionID := nagiStringValue(invocation, "session")
	asJSON := nagiBoolValue(invocation, "json")
	state := nagiStringValue(invocation, "state")
	timeout := nagiIntValue(invocation, "timeout")
	targetType := nagiStringValue(invocation, "target")
	value := nagiRawValue(invocation, "value")

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	action := api.Action{
		Kind: "wait",
		Args: map[string]string{
			"target":     targetType,
			"value":      value,
			"timeout_ms": strconv.Itoa(timeout),
		},
	}
	if targetType == "selector" {
		action.Args["state"] = state
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

	if res.Result.Message != "" {
		fmt.Fprintln(stdout, res.Result.Message)
		return 0
	}

	fmt.Fprintf(stdout, "waited for %s\n", targetType)
	return 0
}

func runViewportInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	sessionID := nagiStringValue(invocation, "session")
	asJSON := nagiBoolValue(invocation, "json")
	viewport, _ := nagiViewportValue(invocation, "viewport")

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: sessionID,
		Action: api.Action{
			Kind: "viewport",
			Args: map[string]string{
				"width":  strconv.Itoa(viewport.Width),
				"height": strconv.Itoa(viewport.Height),
			},
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

	fmt.Fprintf(stdout, "set viewport %dx%d\n", viewport.Width, viewport.Height)
	return 0
}
