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

func runInput(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printInputHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("input", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	indexArg := ""
	textArg := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		indexArg = args[0]
		args = args[1:]
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		textArg = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "input"); err != nil {
		return 1
	}

	positionals := make([]string, 0, 2)
	if indexArg != "" {
		positionals = append(positionals, indexArg)
	}
	if textArg != "" {
		positionals = append(positionals, textArg)
	}
	positionals = append(positionals, fs.Args()...)

	if len(positionals) != 2 {
		fmt.Fprintln(stderr, "input requires an index and text")
		printCommandHint(stderr, "input", `nxctl input @e3 "hello@example.com" --json`)
		return 1
	}

	indexArg = positionals[0]
	textArg = positionals[1]

	nodeID, _, err := parseNodeSelector(indexArg)
	if err != nil {
		fmt.Fprintln(stderr, "input requires a positive integer index or @eN ref")
		return 1
	}

	return runTypeAction(ctx, stdout, stderr, typeActionOptions{
		SessionID: *sessionID,
		JSON:      *asJSON,
		NodeID:    &nodeID,
		Text:      textArg,
	})
}

func runSelect(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printSelectHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("select", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	indexArg := ""
	valueArg := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		indexArg = args[0]
		args = args[1:]
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		valueArg = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "select"); err != nil {
		return 1
	}

	positionals := make([]string, 0, 2)
	if indexArg != "" {
		positionals = append(positionals, indexArg)
	}
	if valueArg != "" {
		positionals = append(positionals, valueArg)
	}
	positionals = append(positionals, fs.Args()...)
	if len(positionals) != 2 {
		fmt.Fprintln(stderr, "select requires an index and value")
		printCommandHint(stderr, "select", `nxctl select @e3 "two" --json`)
		return 1
	}

	nodeID, nodeRef, err := parseNodeSelector(positionals[0])
	if err != nil {
		fmt.Fprintln(stderr, "select requires a positive integer index or @eN ref")
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
			Kind:   "select",
			NodeID: &nodeID,
			Text:   positionals[1],
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
		fmt.Fprintf(stdout, "selected %s on %s\n", positionals[1], nodeRef)
		return 0
	}

	fmt.Fprintf(stdout, "selected %s on %d\n", positionals[1], nodeID)
	return 0
}

func runUpload(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printUploadHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("upload", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	indexArg := ""
	pathArg := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		indexArg = args[0]
		args = args[1:]
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		pathArg = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "upload"); err != nil {
		return 1
	}

	positionals := make([]string, 0, 2)
	if indexArg != "" {
		positionals = append(positionals, indexArg)
	}
	if pathArg != "" {
		positionals = append(positionals, pathArg)
	}
	positionals = append(positionals, fs.Args()...)
	if len(positionals) != 2 {
		fmt.Fprintln(stderr, "upload requires an index and path")
		printCommandHint(stderr, "upload", "nxctl upload @e4 /path/to/file")
		return 1
	}

	nodeID, nodeRef, err := parseNodeSelector(positionals[0])
	if err != nil {
		fmt.Fprintln(stderr, "upload requires a positive integer index or @eN ref")
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
			Kind:   "upload",
			NodeID: &nodeID,
			Text:   positionals[1],
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
		fmt.Fprintf(stdout, "uploaded %s to %s\n", positionals[1], nodeRef)
		return 0
	}

	fmt.Fprintf(stdout, "uploaded %s to %d\n", positionals[1], nodeID)
	return 0
}

func runType(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printTypeHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("type", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	textArg := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		textArg = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "type"); err != nil {
		return 1
	}

	if textArg == "" && fs.NArg() == 1 {
		textArg = fs.Arg(0)
	}

	if textArg == "" || fs.NArg() > 1 {
		fmt.Fprintln(stderr, "type requires text")
		printCommandHint(stderr, "type", `nxctl type "hello" --json`)
		return 1
	}

	return runTypeAction(ctx, stdout, stderr, typeActionOptions{
		SessionID: *sessionID,
		JSON:      *asJSON,
		Text:      textArg,
	})
}

type typeActionOptions struct {
	SessionID string
	JSON      bool
	NodeID    *int
	Text      string
}

func runTypeAction(ctx context.Context, stdout io.Writer, stderr io.Writer, opts typeActionOptions) int {
	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: opts.SessionID,
		Action: api.Action{
			Kind:   "type",
			NodeID: opts.NodeID,
			Text:   opts.Text,
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

	fmt.Fprintln(stdout, "typed")
	return 0
}
