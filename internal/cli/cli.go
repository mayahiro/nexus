package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/browsermgr"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/daemon"
	"github.com/mayahiro/nexus/internal/rpc"
)

const daemonStartTimeout = 3 * time.Second

var startDaemonProcess = startDaemon
var newBrowserManager = func(paths config.Paths) browserManager {
	return browsermgr.New(paths)
}

type browserManager interface {
	Setup(ctx context.Context) (browsermgr.SetupResult, error)
	Update(ctx context.Context) (browsermgr.SetupResult, error)
	Uninstall(ctx context.Context, names ...string) (browsermgr.UninstallResult, error)
	Status() (browsermgr.Status, error)
	Resolve(name string) (browsermgr.Installation, error)
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 1
	}

	switch args[0] {
	case "attach":
		return runAttach(ctx, args[1:], stdout, stderr)
	case "back":
		return runBack(ctx, args[1:], stdout, stderr)
	case "browser":
		return runBrowser(ctx, args[1:], stdout, stderr)
	case "click":
		return runClick(ctx, args[1:], stdout, stderr)
	case "close":
		return runClose(ctx, args[1:], stdout, stderr)
	case "dblclick":
		return runDblclick(ctx, args[1:], stdout, stderr)
	case "eval":
		return runEval(ctx, args[1:], stdout, stderr)
	case "get":
		return runGet(ctx, args[1:], stdout, stderr)
	case "help":
		return runHelp(args[1:], stdout, stderr)
	case "hover":
		return runHover(ctx, args[1:], stdout, stderr)
	case "input":
		return runInput(ctx, args[1:], stdout, stderr)
	case "keys":
		return runKeys(ctx, args[1:], stdout, stderr)
	case "open":
		return runOpen(ctx, args[1:], stdout, stderr)
	case "observe":
		return runObserve(ctx, args[1:], stdout, stderr)
	case "scroll":
		return runScroll(ctx, args[1:], stdout, stderr)
	case "screenshot":
		return runScreenshot(ctx, args[1:], stdout, stderr)
	case "select":
		return runSelect(ctx, args[1:], stdout, stderr)
	case "sessions":
		return runSessions(ctx, args[1:], stdout, stderr)
	case "state":
		return runState(ctx, args[1:], stdout, stderr)
	case "type":
		return runType(ctx, args[1:], stdout, stderr)
	case "upload":
		return runUpload(ctx, args[1:], stdout, stderr)
	case "wait":
		return runWait(ctx, args[1:], stdout, stderr)
	case "rightclick":
		return runRightclick(ctx, args[1:], stdout, stderr)
	case "detach":
		return runDetach(ctx, args[1:], stdout, stderr)
	case "daemon":
		return runDaemon(ctx, stderr)
	case "doctor":
		return runDoctor(ctx, stdout)
	default:
		printUsage(stderr)
		return 1
	}
}

func runDaemon(ctx context.Context, stderr io.Writer) int {
	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if err := daemon.Run(ctx, paths, daemon.RunOptions{}); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	return 0
}

func runHelp(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}

	if len(args) > 1 {
		fmt.Fprintln(stderr, "help accepts at most one command")
		return 1
	}

	if !printCommandHelp(stdout, args[0]) {
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		return 1
	}

	return 0
}

func runDoctor(ctx context.Context, stdout io.Writer) (exitCode int) {
	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintf(stdout, "config: error (%v)\n", err)
		fmt.Fprintln(stdout, "socket: skipped")
		fmt.Fprintln(stdout, "daemon: skipped")
		fmt.Fprintln(stdout, "protocol: skipped")
		return 1
	}

	fmt.Fprintf(stdout, "config: ok (%s)\n", paths.Config)

	client, started, err := ensureDaemon(ctx, paths)
	if err != nil {
		reportSocketStatus(stdout, paths, err)
		return 1
	}
	defer client.Close()
	if started {
		defer func() {
			stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			if _, err := client.StopDaemon(stopCtx); err != nil {
				fmt.Fprintf(stdout, "daemon: stop error (%v)\n", err)
				if exitCode == 0 {
					exitCode = 1
				}
				return
			}

			fmt.Fprintln(stdout, "daemon: stopped")
		}()
	}

	if started {
		fmt.Fprintf(stdout, "socket: started (%s)\n", paths.Socket)
		fmt.Fprintln(stdout, "daemon: started")
	} else {
		fmt.Fprintf(stdout, "socket: ok (%s)\n", paths.Socket)
		fmt.Fprintln(stdout, "daemon: ok")
	}

	res, err := client.Ping(ctx)
	if err != nil {
		fmt.Fprintf(stdout, "protocol: error (%v)\n", err)
		return 1
	}

	if res.ProtocolVersion != api.ProtocolVersion {
		fmt.Fprintf(stdout, "protocol: mismatch (client=%s daemon=%s)\n", api.ProtocolVersion, res.ProtocolVersion)
		return 1
	}

	fmt.Fprintf(stdout, "protocol: ok (%s)\n", res.ProtocolVersion)
	return 0
}

func runBrowser(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printBrowserHelp(stdout)
		return 0
	}
	if len(args) == 0 {
		printBrowserHelp(stderr)
		return 1
	}

	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	manager := newBrowserManager(paths)

	switch args[0] {
	case "setup":
		result, err := manager.Setup(ctx)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		printBrowserResults(stdout, result)
		return 0
	case "update":
		result, err := manager.Update(ctx)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		printBrowserResults(stdout, result)
		return 0
	case "status":
		status, err := manager.Status()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		printBrowserStatus(stdout, status)
		return 0
	case "uninstall":
		fs := flag.NewFlagSet("browser uninstall", flag.ContinueOnError)
		fs.SetOutput(stderr)
		name := fs.String("name", "", "browser name")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		names := []string{}
		if *name != "" {
			names = append(names, *name)
		}
		result, err := manager.Uninstall(ctx, names...)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		printBrowserResults(stdout, browsermgr.SetupResult{Browsers: result.Browsers})
		return 0
	default:
		if args[0] == "help" {
			printBrowserHelp(stdout)
			return 0
		}
		fmt.Fprintf(stderr, "unknown browser subcommand: %s\n", args[0])
		printBrowserHelp(stderr)
		return 1
	}
}

func runBack(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printBackHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("back", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")

	if err := fs.Parse(args); err != nil {
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

func runAttach(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printAttachHelp(stdout)
		return 0
	}
	if len(args) == 0 {
		printAttachHelp(stderr)
		return 1
	}

	switch args[0] {
	case "browser":
		return runAttachBrowser(ctx, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown target type: %s\n", args[0])
		printAttachHelp(stderr)
		return 1
	}
}

func runOpen(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printOpenHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("open", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	backend := fs.String("backend", "chromium", "browser backend")
	targetRef := fs.String("target-ref", "", "target ref")
	urlArg := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		urlArg = args[0]
		args = args[1:]
	}

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if urlArg == "" && fs.NArg() == 1 {
		urlArg = fs.Arg(0)
	}

	if urlArg == "" || fs.NArg() > 1 {
		fmt.Fprintln(stderr, "open requires a url")
		printOpenHelp(stderr)
		return 1
	}

	openArgs := []string{
		"--session", *sessionID,
		"--backend", *backend,
		"--url", urlArg,
	}
	if *targetRef != "" {
		openArgs = append(openArgs, "--target-ref", *targetRef)
	}

	return runAttachBrowser(ctx, openArgs, stdout, stderr)
}

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

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if source == "" && fs.NArg() == 1 {
		source = fs.Arg(0)
	}

	if source == "" || fs.NArg() > 1 {
		fmt.Fprintln(stderr, "eval requires js code")
		printEvalHelp(stderr)
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

	if err := fs.Parse(args); err != nil {
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
		printGetHelp(stderr)
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
			printGetHelp(stderr)
			return 1
		}
		if *selector != "" {
			fmt.Fprintf(stderr, "get %s does not support --selector\n", target)
			return 1
		}
		nodeID, err := strconv.Atoi(positionals[1])
		if err != nil || nodeID <= 0 {
			fmt.Fprintf(stderr, "get %s requires a positive integer index\n", target)
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

	if err := fs.Parse(args); err != nil {
		return 1
	}

	positionals = append(positionals, fs.Args()...)
	if len(positionals) != 1 && len(positionals) != 2 {
		fmt.Fprintln(stderr, "click requires an index or x y coordinates")
		printClickHelp(stderr)
		return 1
	}

	action := api.Action{Kind: "invoke"}
	fallbackMessage := ""
	if len(positionals) == 1 {
		nodeID, err := strconv.Atoi(positionals[0])
		if err != nil || nodeID <= 0 {
			fmt.Fprintln(stderr, "click requires a positive integer index")
			return 1
		}
		action.NodeID = &nodeID
		fallbackMessage = fmt.Sprintf("clicked %d", nodeID)
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

	if res.Result.Message != "" {
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

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if indexArg == "" && fs.NArg() == 1 {
		indexArg = fs.Arg(0)
	}

	if indexArg == "" || fs.NArg() > 1 {
		fmt.Fprintf(stderr, "%s requires an index\n", command)
		printNodeActionHelp(stderr, command)
		return 1
	}

	nodeID, err := strconv.Atoi(indexArg)
	if err != nil || nodeID <= 0 {
		fmt.Fprintf(stderr, "%s requires a positive integer index\n", command)
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

	if res.Result.Message != "" {
		fmt.Fprintln(stdout, res.Result.Message)
		return 0
	}

	fmt.Fprintf(stdout, fallbackFormat+"\n", nodeID)
	return 0
}

func runClose(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printCloseHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("close", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	closeAll := fs.Bool("all", false, "close all sessions")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	if *closeAll {
		listed, err := client.ListSessions(ctx)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if len(listed.Sessions) == 0 {
			if _, err := client.StopDaemon(ctx); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			fmt.Fprintln(stdout, "closed all sessions")
			return 0
		}
		for _, session := range listed.Sessions {
			if _, err := client.DetachSession(ctx, api.DetachSessionRequest{SessionID: session.ID}); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		}
		fmt.Fprintln(stdout, "closed all sessions")
		return 0
	}

	res, err := client.DetachSession(ctx, api.DetachSessionRequest{SessionID: *sessionID})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "closed %s\n", res.Session.ID)
	return 0
}

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

	if err := fs.Parse(args); err != nil {
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
		printInputHelp(stderr)
		return 1
	}

	indexArg = positionals[0]
	textArg = positionals[1]

	nodeID, err := strconv.Atoi(indexArg)
	if err != nil || nodeID <= 0 {
		fmt.Fprintln(stderr, "input requires a positive integer index")
		return 1
	}

	return runTypeAction(ctx, stdout, stderr, typeActionOptions{
		SessionID: *sessionID,
		JSON:      *asJSON,
		NodeID:    &nodeID,
		Text:      textArg,
	})
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

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if keySpec == "" && fs.NArg() == 1 {
		keySpec = fs.Arg(0)
	}

	if keySpec == "" || fs.NArg() > 1 {
		fmt.Fprintln(stderr, "keys requires a key spec")
		printKeysHelp(stderr)
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

func runScreenshot(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printScreenshotHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("screenshot", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	full := fs.Bool("full", false, "capture full page")
	pathArg := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		pathArg = args[0]
		args = args[1:]
	}

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if pathArg == "" && fs.NArg() == 1 {
		pathArg = fs.Arg(0)
	}
	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "screenshot accepts at most one path")
		return 1
	}
	if pathArg == "" {
		pathArg = "screenshot.png"
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
			WithScreenshot: true,
			FullScreenshot: *full,
		},
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if res.Observation.Screenshot == "" {
		fmt.Fprintln(stderr, "empty screenshot")
		return 1
	}

	data, err := base64.StdEncoding.DecodeString(res.Observation.Screenshot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := os.WriteFile(pathArg, data, 0o644); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "saved screenshot %s\n", pathArg)
	return 0
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

	if err := fs.Parse(args); err != nil {
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
		printSelectHelp(stderr)
		return 1
	}

	nodeID, err := strconv.Atoi(positionals[0])
	if err != nil || nodeID <= 0 {
		fmt.Fprintln(stderr, "select requires a positive integer index")
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

	if res.Result.Message != "" {
		fmt.Fprintln(stdout, res.Result.Message)
		return 0
	}

	fmt.Fprintf(stdout, "selected %s on %d\n", positionals[1], nodeID)
	return 0
}

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

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if dirArg == "" && fs.NArg() == 1 {
		dirArg = fs.Arg(0)
	}

	if dirArg == "" || fs.NArg() > 1 {
		fmt.Fprintln(stderr, "scroll requires a direction")
		printScrollHelp(stderr)
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

	if err := fs.Parse(args); err != nil {
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
		printUploadHelp(stderr)
		return 1
	}

	nodeID, err := strconv.Atoi(positionals[0])
	if err != nil || nodeID <= 0 {
		fmt.Fprintln(stderr, "upload requires a positive integer index")
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

	if res.Result.Message != "" {
		fmt.Fprintln(stdout, res.Result.Message)
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

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if textArg == "" && fs.NArg() == 1 {
		textArg = fs.Arg(0)
	}

	if textArg == "" || fs.NArg() > 1 {
		fmt.Fprintln(stderr, "type requires text")
		printTypeHelp(stderr)
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

	if err := fs.Parse(args); err != nil {
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

	if len(positionals) != 2 {
		fmt.Fprintln(stderr, "wait requires a target and value")
		printWaitHelp(stderr)
		return 1
	}

	targetType = positionals[0]
	value = positionals[1]
	if targetType != "selector" && targetType != "text" && targetType != "url" {
		fmt.Fprintln(stderr, "wait target must be selector, text, or url")
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

func runAttachBrowser(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printAttachBrowserHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("attach browser", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "", "session id")
	backend := fs.String("backend", "chromium", "browser backend")
	targetRef := fs.String("target-ref", "", "target ref")
	initialURL := fs.String("url", "", "initial url")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *sessionID == "" {
		fmt.Fprintln(stderr, "--session is required")
		printAttachBrowserHelp(stderr)
		return 1
	}

	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	resolvedTargetRef := *targetRef
	if resolvedTargetRef == "" {
		installation, err := newBrowserManager(paths).Resolve(*backend)
		if err != nil {
			if errors.Is(err, browsermgr.ErrBrowserNotInstalled) {
				fmt.Fprintf(stderr, "%s is not installed. run `nxctl browser setup` first\n", *backend)
				return 1
			}
			fmt.Fprintln(stderr, err)
			return 1
		}
		resolvedTargetRef = installation.ExecutablePath
	}

	options := map[string]string{}
	if *initialURL != "" {
		options["initial_url"] = *initialURL
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.AttachSession(ctx, api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  *sessionID,
		TargetRef:  resolvedTargetRef,
		Backend:    *backend,
		Options:    options,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "attached %s %s (%s) %s\n", res.Session.TargetType, res.Session.ID, res.Session.Backend, res.Session.TargetRef)
	return 0
}

func runSessions(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printSessionsHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("sessions", flag.ContinueOnError)
	fs.SetOutput(stderr)

	asJSON := fs.Bool("json", false, "print as json")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ListSessions(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(res.Sessions); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	if len(res.Sessions) == 0 {
		fmt.Fprintln(stdout, "no sessions")
		return 0
	}

	for _, session := range res.Sessions {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", session.ID, session.TargetType, session.Backend, session.TargetRef)
	}

	return 0
}

func runDetach(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printDetachHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("detach", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "", "session id")
	if err := fs.Parse(args); err != nil {
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

	res, err := client.DetachSession(ctx, api.DetachSessionRequest{
		SessionID: *sessionID,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "detached %s\n", res.Session.ID)
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

	if err := fs.Parse(args); err != nil {
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

	if err := fs.Parse(args); err != nil {
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
		fmt.Fprintf(stdout, "[%d] %s", node.ID, node.Role)
		if node.Name != "" {
			fmt.Fprintf(stdout, " %q", node.Name)
		} else if node.Text != "" {
			fmt.Fprintf(stdout, " %q", node.Text)
		}
		fmt.Fprintln(stdout)
	}

	return 0
}

func ensureDaemon(ctx context.Context, paths config.Paths) (*rpc.Client, bool, error) {
	client, err := rpc.Dial(ctx, paths.Socket)
	if err == nil {
		return client, false, nil
	}

	if err := startDaemonProcess(paths); err != nil {
		return nil, false, err
	}

	client, err = waitForDaemon(ctx, paths.Socket)
	if err != nil {
		return nil, true, err
	}

	return client, true, nil
}

func connectClient(ctx context.Context) (*rpc.Client, error) {
	paths, err := config.DefaultPaths()
	if err != nil {
		return nil, err
	}

	client, _, err := ensureDaemon(ctx, paths)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func waitForDaemon(ctx context.Context, socket string) (*rpc.Client, error) {
	deadline := time.Now().Add(daemonStartTimeout)

	for {
		client, err := rpc.Dial(ctx, socket)
		if err == nil {
			return client, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func startDaemon(paths config.Paths) error {
	if err := os.MkdirAll(filepath.Dir(paths.Log), 0o755); err != nil {
		return err
	}

	logFile, err := os.OpenFile(paths.Log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()

	executable, err := findDaemonExecutable()
	if err != nil {
		return err
	}

	cmd := exec.Command(executable)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		return err
	}

	return cmd.Process.Release()
}

func findDaemonExecutable() (string, error) {
	if path, err := exec.LookPath("nxd"); err == nil {
		return path, nil
	}

	current, err := os.Executable()
	if err != nil {
		return "", err
	}

	candidate := filepath.Join(filepath.Dir(current), "nxd")
	if _, err := os.Stat(candidate); err != nil {
		return "", err
	}

	return candidate, nil
}

func reportSocketStatus(stdout io.Writer, paths config.Paths, dialErr error) {
	socketStatus := "ok"
	if _, err := os.Stat(paths.Socket); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			socketStatus = "missing"
		} else {
			socketStatus = fmt.Sprintf("error (%v)", err)
		}
	}

	fmt.Fprintf(stdout, "socket: %s (%s)\n", socketStatus, paths.Socket)
	fmt.Fprintf(stdout, "daemon: error (%v)\n", dialErr)
	fmt.Fprintln(stdout, "protocol: skipped")
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl <command>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "commands:")
	fmt.Fprintln(w, "  attach")
	fmt.Fprintln(w, "  back")
	fmt.Fprintln(w, "  browser")
	fmt.Fprintln(w, "  click")
	fmt.Fprintln(w, "  close")
	fmt.Fprintln(w, "  help")
	fmt.Fprintln(w, "  eval")
	fmt.Fprintln(w, "  dblclick")
	fmt.Fprintln(w, "  get")
	fmt.Fprintln(w, "  hover")
	fmt.Fprintln(w, "  input")
	fmt.Fprintln(w, "  keys")
	fmt.Fprintln(w, "  open")
	fmt.Fprintln(w, "  observe")
	fmt.Fprintln(w, "  rightclick")
	fmt.Fprintln(w, "  scroll")
	fmt.Fprintln(w, "  screenshot")
	fmt.Fprintln(w, "  select")
	fmt.Fprintln(w, "  sessions")
	fmt.Fprintln(w, "  state")
	fmt.Fprintln(w, "  type")
	fmt.Fprintln(w, "  upload")
	fmt.Fprintln(w, "  wait")
	fmt.Fprintln(w, "  detach")
	fmt.Fprintln(w, "  daemon")
	fmt.Fprintln(w, "  doctor")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "run `nxctl help <command>` for command-specific usage")
}

func isHelpArgs(args []string) bool {
	return len(args) == 1 && (args[0] == "-h" || args[0] == "--help")
}

func printCommandHelp(w io.Writer, command string) bool {
	switch command {
	case "attach":
		printAttachHelp(w)
	case "back":
		printBackHelp(w)
	case "browser":
		printBrowserHelp(w)
	case "click":
		printClickHelp(w)
	case "close":
		printCloseHelp(w)
	case "dblclick":
		printNodeActionHelp(w, "dblclick")
	case "eval":
		printEvalHelp(w)
	case "get":
		printGetHelp(w)
	case "hover":
		printNodeActionHelp(w, "hover")
	case "input":
		printInputHelp(w)
	case "keys":
		printKeysHelp(w)
	case "observe":
		printObserveHelp(w)
	case "open":
		printOpenHelp(w)
	case "rightclick":
		printNodeActionHelp(w, "rightclick")
	case "scroll":
		printScrollHelp(w)
	case "screenshot":
		printScreenshotHelp(w)
	case "select":
		printSelectHelp(w)
	case "sessions":
		printSessionsHelp(w)
	case "state":
		printStateHelp(w)
	case "type":
		printTypeHelp(w)
	case "upload":
		printUploadHelp(w)
	case "wait":
		printWaitHelp(w)
	case "detach":
		printDetachHelp(w)
	case "daemon":
		printDaemonHelp(w)
	case "doctor":
		printDoctorHelp(w)
	default:
		return false
	}
	return true
}

func printAttachHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl attach browser --session <id> --backend <name> [--url <url>] [--target-ref <path>]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "targets:")
	fmt.Fprintln(w, "  browser")
}

func printAttachBrowserHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl attach browser --session <id> --backend chromium|lightpanda [--url <url>] [--target-ref <path>]")
}

func printBackHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl back [--session <id>] [--json]")
}

func printBrowserHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl browser <setup|update|status|uninstall>")
	fmt.Fprintln(w, "   or: nxctl browser uninstall [--name chromium|lightpanda]")
}

func printClickHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl click <index> [--session <id>] [--json]")
	fmt.Fprintln(w, "   or: nxctl click <x> <y> [--session <id>] [--json]")
}

func printCloseHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl close [--session <id>]")
	fmt.Fprintln(w, "   or: nxctl close --all")
}

func printDaemonHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl daemon")
}

func printDoctorHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl doctor")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "doctor starts nxd temporarily if needed and stops it after the check")
}

func printEvalHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl eval "js code" [--session <id>] [--json]`)
}

func printGetHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl get title [--session <id>] [--json]")
	fmt.Fprintln(w, "   or: nxctl get html [--selector <css>] [--session <id>] [--json]")
	fmt.Fprintln(w, "   or: nxctl get text|value|attributes|bbox <index> [--session <id>] [--json]")
}

func printInputHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl input <index> "text" [--session <id>] [--json]`)
}

func printKeysHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl keys "Enter" [--session <id>] [--json]`)
}

func printNodeActionHelp(w io.Writer, command string) {
	switch command {
	case "hover":
		fmt.Fprintln(w, "usage: nxctl hover <index> [--session <id>] [--json]")
	case "dblclick":
		fmt.Fprintln(w, "usage: nxctl dblclick <index> [--session <id>] [--json]")
	case "rightclick":
		fmt.Fprintln(w, "usage: nxctl rightclick <index> [--session <id>] [--json]")
	}
}

func printObserveHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl observe [--session <id>] [--json] [--text] [--tree] [--screenshot] [--full]")
}

func printOpenHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl open <url> [--session <id>] [--backend chromium|lightpanda] [--json]")
}

func printScrollHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl scroll up|down [--session <id>] [--node <index>] [--amount <px>] [--json]")
}

func printScreenshotHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl screenshot [path] [--session <id>] [--full]")
}

func printSelectHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl select <index> "value" [--session <id>] [--json]`)
}

func printSessionsHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl sessions [--json]")
}

func printStateHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl state [--session <id>] [--json] [--text] [--tree] [--screenshot] [--full]")
}

func printTypeHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl type "text" [--session <id>] [--json]`)
}

func printUploadHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl upload <index> <path> [--session <id>] [--json]")
}

func printWaitHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl wait selector "<css>" [--state attached|detached|visible|hidden] [--timeout <ms>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl wait text "value" [--timeout <ms>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl wait url "value" [--timeout <ms>] [--session <id>] [--json]`)
}

func printDetachHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl detach --session <id>")
}

func printBrowserResults(w io.Writer, result browsermgr.SetupResult) {
	for _, browser := range result.Browsers {
		status := "unchanged"
		if browser.Changed {
			status = "updated"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", browser.Name, browser.Version, status, browser.ExecutablePath)
	}
}

func printBrowserStatus(w io.Writer, status browsermgr.Status) {
	for _, browser := range status.Browsers {
		state := "not_installed"
		if browser.Installed {
			state = "installed"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", browser.Name, browser.Version, state, browser.ExecutablePath)
	}
}

func printEvalValue(w io.Writer, value interface{}) error {
	switch value := value.(type) {
	case nil:
		_, err := fmt.Fprintln(w, "null")
		return err
	case string:
		_, err := fmt.Fprintln(w, value)
		return err
	default:
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}
}
