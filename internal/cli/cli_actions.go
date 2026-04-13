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

func runBack(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printBackHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("back", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")

	if err := parseCommandFlags(fs, args, stderr, "back"); err != nil {
		return 1
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "back does not accept positional arguments")
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

	fmt.Fprintln(stdout, "went back")
	return 0
}

func runClick(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printClickHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("click", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	positionals := make([]string, 0, 2)
	for len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionals = append(positionals, args[0])
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "click"); err != nil {
		return 1
	}

	positionals = append(positionals, fs.Args()...)
	if len(positionals) != 1 && len(positionals) != 2 {
		fmt.Fprintln(stderr, "click requires an index or x y coordinates")
		printCommandHint(stderr, "click", "nxctl click @e3 --json")
		return 1
	}

	action := api.Action{Kind: "invoke"}
	fallbackMessage := ""
	useNodeRefMessage := false
	if len(positionals) == 1 {
		nodeID, nodeRef, err := parseNodeSelector(positionals[0])
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
		x, err := strconv.Atoi(positionals[0])
		if err != nil || x < 0 {
			fmt.Fprintln(stderr, "click requires non-negative integer x y coordinates")
			return 1
		}
		y, err := strconv.Atoi(positionals[1])
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

	if !useNodeRefMessage && res.Result.Message != "" {
		fmt.Fprintln(stdout, res.Result.Message)
		return 0
	}

	fmt.Fprintln(stdout, fallbackMessage)
	return 0
}

func runHover(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	return runNodeActionCommand(ctx, "hover", "hovered %d", args, stdout, stderr)
}

func runDblclick(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	return runNodeActionCommand(ctx, "dblclick", "double-clicked %d", args, stdout, stderr)
}

func runRightclick(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	return runNodeActionCommand(ctx, "rightclick", "right-clicked %d", args, stdout, stderr)
}

func runNodeActionCommand(ctx context.Context, command string, fallbackFormat string, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printNodeActionHelp(stdout, command)
		return 0
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	indexArg := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		indexArg = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, command); err != nil {
		return 1
	}

	if indexArg == "" && fs.NArg() == 1 {
		indexArg = fs.Arg(0)
	}

	if indexArg == "" || fs.NArg() > 1 {
		fmt.Fprintf(stderr, "%s requires an index\n", command)
		printCommandHint(stderr, command, fmt.Sprintf("nxctl %s @e3 --json", command))
		return 1
	}

	nodeID, nodeRef, err := parseNodeSelector(indexArg)
	if err != nil {
		fmt.Fprintf(stderr, "%s requires a positive integer index or @eN ref\n", command)
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

	if *asJSON {
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

func runKeys(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printKeysHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("keys", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	keySpec := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		keySpec = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "keys"); err != nil {
		return 1
	}

	if keySpec == "" && fs.NArg() == 1 {
		keySpec = fs.Arg(0)
	}

	if keySpec == "" || fs.NArg() > 1 {
		fmt.Fprintln(stderr, "keys requires a key spec")
		printCommandHint(stderr, "keys", `nxctl keys "Enter" --json`)
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

	fmt.Fprintf(stdout, "sent keys %s\n", keySpec)
	return 0
}
