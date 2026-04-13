package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mayahiro/nexus/internal/api"
)

func runScroll(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printScrollHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("scroll", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	amount := fs.Int("amount", 0, "scroll amount in pixels")
	nodeID := fs.Int("node", 0, "node id")
	dirArg := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		dirArg = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "scroll"); err != nil {
		return 1
	}

	if dirArg == "" && fs.NArg() == 1 {
		dirArg = fs.Arg(0)
	}

	if dirArg == "" || fs.NArg() > 1 {
		fmt.Fprintln(stderr, "scroll requires a direction")
		printCommandHint(stderr, "scroll", "nxctl scroll down --amount 500")
		return 1
	}
	if dirArg != "up" && dirArg != "down" {
		fmt.Fprintln(stderr, "scroll direction must be up or down")
		return 1
	}
	if *amount < 0 {
		fmt.Fprintln(stderr, "scroll amount must be a non-negative integer")
		return 1
	}
	if *nodeID < 0 {
		fmt.Fprintln(stderr, "scroll node must be a positive integer")
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	action := api.Action{
		Kind: "scroll",
		Dir:  dirArg,
	}
	if *amount > 0 {
		action.Args = map[string]string{"amount": strconv.Itoa(*amount)}
	}
	if *nodeID > 0 {
		action.NodeID = nodeID
	}

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

	fmt.Fprintf(stdout, "scrolled %s\n", dirArg)
	return 0
}

func runWait(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printWaitHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("wait", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	state := fs.String("state", "visible", "wait state for selector")
	timeout := fs.Int("timeout", 30000, "wait timeout in milliseconds")
	targetType := ""
	value := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		targetType = args[0]
		args = args[1:]
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		value = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "wait"); err != nil {
		return 1
	}

	positionals := make([]string, 0, 2)
	if targetType != "" {
		positionals = append(positionals, targetType)
	}
	if value != "" {
		positionals = append(positionals, value)
	}
	positionals = append(positionals, fs.Args()...)

	if len(positionals) == 0 {
		fmt.Fprintln(stderr, "wait requires a target and value")
		printCommandHint(stderr, "wait", `nxctl wait selector ".ready"`)
		return 1
	}
	targetType = positionals[0]
	switch targetType {
	case "navigation":
		if len(positionals) != 1 {
			fmt.Fprintln(stderr, "wait navigation does not accept a value")
			printCommandHint(stderr, "wait", "nxctl wait navigation")
			return 1
		}
	case "selector", "text", "url", "function":
		if len(positionals) != 2 {
			fmt.Fprintln(stderr, "wait requires a target and value")
			printCommandHint(stderr, "wait", `nxctl wait selector ".ready"`)
			return 1
		}
		value = positionals[1]
	default:
		fmt.Fprintln(stderr, "wait target must be selector, text, url, navigation, or function")
		return 1
	}
	if *timeout < 0 {
		fmt.Fprintln(stderr, "wait timeout must be a non-negative integer")
		return 1
	}
	if targetType != "selector" && *state != "visible" {
		fmt.Fprintf(stderr, "wait %s does not support --state\n", targetType)
		return 1
	}

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
			"timeout_ms": strconv.Itoa(*timeout),
		},
	}
	if targetType == "selector" {
		action.Args["state"] = *state
	}

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

func runViewport(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printViewportHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("viewport", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	value := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		value = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "viewport"); err != nil {
		return 1
	}
	if value == "" && fs.NArg() == 1 {
		value = fs.Arg(0)
	}
	if value == "" || fs.NArg() > 1 {
		fmt.Fprintln(stderr, "viewport requires WIDTHxHEIGHT")
		printCommandHint(stderr, "viewport", "nxctl viewport 1280x720")
		return 1
	}

	width, height, err := parseViewport(value)
	if err != nil {
		fmt.Fprintln(stderr, err)
		printCommandHint(stderr, "viewport", "nxctl viewport 1280x720")
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
			Kind: "viewport",
			Args: map[string]string{
				"width":  strconv.Itoa(width),
				"height": strconv.Itoa(height),
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

	if *asJSON {
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

	fmt.Fprintf(stdout, "set viewport %dx%d\n", width, height)
	return 0
}
