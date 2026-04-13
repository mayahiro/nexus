package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/browsermgr"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/daemon"
	"github.com/mayahiro/nexus/internal/rpc"
)

const daemonStartTimeout = 3 * time.Second
const defaultViewportWidth = 1920
const defaultViewportHeight = 1080

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
	case "batch":
		return runBatch(ctx, args[1:], stdout, stderr)
	case "browser":
		return runBrowser(ctx, args[1:], stdout, stderr)
	case "click":
		return runClick(ctx, args[1:], stdout, stderr)
	case "compare":
		return runCompare(ctx, args[1:], stdout, stderr)
	case "close":
		return runClose(ctx, args[1:], stdout, stderr)
	case "dblclick":
		return runDblclick(ctx, args[1:], stdout, stderr)
	case "eval":
		return runEval(ctx, args[1:], stdout, stderr)
	case "find":
		return runFind(ctx, args[1:], stdout, stderr)
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
	case "viewport":
		return runViewport(ctx, args[1:], stdout, stderr)
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

type batchCommands []string

func (b *batchCommands) String() string {
	return strings.Join(*b, ", ")
}

func (b *batchCommands) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return errors.New("batch command must not be empty")
	}
	*b = append(*b, trimmed)
	return nil
}

type batchStepResult struct {
	Command  string   `json:"command"`
	Args     []string `json:"args"`
	ExitCode int      `json:"exit_code"`
	Stdout   string   `json:"stdout,omitempty"`
	Stderr   string   `json:"stderr,omitempty"`
}

func runBatch(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printBatchHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("batch", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var commands batchCommands
	asJSON := fs.Bool("json", false, "print as json")
	fs.Var(&commands, "cmd", "subcommand to execute")

	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(commands) == 0 {
		fmt.Fprintln(stderr, "batch requires at least one --cmd")
		printBatchHelp(stderr)
		return 1
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "batch does not accept positional arguments")
		printBatchHelp(stderr)
		return 1
	}

	results := make([]batchStepResult, 0, len(commands))
	for _, raw := range commands {
		argv, err := splitBatchCommand(raw)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if len(argv) == 0 {
			fmt.Fprintln(stderr, "batch command must not be empty")
			return 1
		}

		var stepStdout bytes.Buffer
		var stepStderr bytes.Buffer
		exitCode := Run(ctx, argv, &stepStdout, &stepStderr)
		results = append(results, batchStepResult{
			Command:  raw,
			Args:     argv,
			ExitCode: exitCode,
			Stdout:   stepStdout.String(),
			Stderr:   stepStderr.String(),
		})

		if *asJSON {
			if exitCode != 0 {
				break
			}
			continue
		}

		fmt.Fprintf(stdout, "==> %s\n", raw)
		if stepStdout.Len() > 0 {
			io.Copy(stdout, &stepStdout)
		}
		if stepStderr.Len() > 0 {
			io.Copy(stderr, &stepStderr)
		}
		if exitCode != 0 {
			fmt.Fprintf(stderr, "batch stopped at: %s\n", raw)
			return exitCode
		}
	}

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(results); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if len(results) > 0 && results[len(results)-1].ExitCode != 0 {
			return results[len(results)-1].ExitCode
		}
		return 0
	}

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
	viewport := fs.String("viewport", "", "viewport as WIDTHxHEIGHT")
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
	if *viewport != "" {
		openArgs = append(openArgs, "--viewport", *viewport)
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

func runFind(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printFindHelp(stdout)
		return 0
	}
	if len(args) == 0 {
		printFindHelp(stderr)
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
		printFindHelp(stderr)
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

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if role == "" || (!*matchAll && actionName == "") {
		fmt.Fprintln(stderr, "find role requires <role> <click|input|get> or --all")
		printFindHelp(stderr)
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

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if textValue == "" || (!*matchAll && actionName == "") {
		fmt.Fprintln(stderr, "find text requires <text> <click|get> or --all")
		printFindHelp(stderr)
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

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if label == "" || (!*matchAll && actionName == "") {
		fmt.Fprintln(stderr, `find label requires "label" input "text", get <target>, or --all`)
		printFindHelp(stderr)
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

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if attrValue == "" || (!*matchAll && actionName == "") {
		fmt.Fprintf(stderr, "find %s requires <value> <click|get> or --all\n", kind)
		printFindHelp(stderr)
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
	annotate := fs.Bool("annotate", false, "draw node refs on top of the screenshot")
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
			WithTree:       *annotate,
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
	if *annotate {
		data, err = annotateScreenshot(data, res.Observation.Tree)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
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

	if len(positionals) == 0 {
		fmt.Fprintln(stderr, "wait requires a target and value")
		printWaitHelp(stderr)
		return 1
	}
	targetType = positionals[0]
	switch targetType {
	case "navigation":
		if len(positionals) != 1 {
			fmt.Fprintln(stderr, "wait navigation does not accept a value")
			printWaitHelp(stderr)
			return 1
		}
	case "selector", "text", "url", "function":
		if len(positionals) != 2 {
			fmt.Fprintln(stderr, "wait requires a target and value")
			printWaitHelp(stderr)
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
	viewport := fs.String("viewport", "", "viewport as WIDTHxHEIGHT")

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
	width, height, err := resolvedViewport(*viewport)
	if err != nil {
		fmt.Fprintln(stderr, err)
		printAttachBrowserHelp(stderr)
		return 1
	}
	options["viewport_width"] = strconv.Itoa(width)
	options["viewport_height"] = strconv.Itoa(height)

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

	if err := fs.Parse(args); err != nil {
		return 1
	}
	if value == "" && fs.NArg() == 1 {
		value = fs.Arg(0)
	}
	if value == "" || fs.NArg() > 1 {
		fmt.Fprintln(stderr, "viewport requires WIDTHxHEIGHT")
		printViewportHelp(stderr)
		return 1
	}

	width, height, err := parseViewport(value)
	if err != nil {
		fmt.Fprintln(stderr, err)
		printViewportHelp(stderr)
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
		printNode(stdout, node)
	}

	return 0
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

func annotateScreenshot(data []byte, nodes []api.Node) ([]byte, error) {
	source, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode screenshot: %w", err)
	}

	bounds := source.Bounds()
	canvas := image.NewRGBA(bounds)
	draw.Draw(canvas, bounds, source, bounds.Min, draw.Src)

	palette := []color.RGBA{
		{R: 255, G: 59, B: 48, A: 255},
		{R: 0, G: 122, B: 255, A: 255},
		{R: 52, G: 199, B: 89, A: 255},
		{R: 255, G: 149, B: 0, A: 255},
		{R: 175, G: 82, B: 222, A: 255},
	}

	for _, node := range nodes {
		if node.Bounds.W <= 0 || node.Bounds.H <= 0 {
			continue
		}

		rect := image.Rect(
			node.Bounds.X,
			node.Bounds.Y,
			node.Bounds.X+node.Bounds.W,
			node.Bounds.Y+node.Bounds.H,
		).Intersect(bounds)
		if rect.Dx() <= 0 || rect.Dy() <= 0 {
			continue
		}

		boxColor := palette[(node.ID-1)%len(palette)]
		drawOutline(canvas, rect, boxColor, 2)

		label := node.Ref
		if label == "" {
			label = fmt.Sprintf("%d", node.ID)
		}
		drawNodeLabel(canvas, rect, label, boxColor)
	}

	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		return nil, fmt.Errorf("encode screenshot: %w", err)
	}
	return output.Bytes(), nil
}

func drawOutline(img *image.RGBA, rect image.Rectangle, stroke color.RGBA, thickness int) {
	for offset := 0; offset < thickness; offset++ {
		top := image.Rect(rect.Min.X, rect.Min.Y+offset, rect.Max.X, rect.Min.Y+offset+1)
		bottom := image.Rect(rect.Min.X, rect.Max.Y-offset-1, rect.Max.X, rect.Max.Y-offset)
		left := image.Rect(rect.Min.X+offset, rect.Min.Y, rect.Min.X+offset+1, rect.Max.Y)
		right := image.Rect(rect.Max.X-offset-1, rect.Min.Y, rect.Max.X-offset, rect.Max.Y)
		fillRect(img, top, stroke)
		fillRect(img, bottom, stroke)
		fillRect(img, left, stroke)
		fillRect(img, right, stroke)
	}
}

func fillRect(img *image.RGBA, rect image.Rectangle, fill color.RGBA) {
	rect = rect.Intersect(img.Bounds())
	if rect.Dx() <= 0 || rect.Dy() <= 0 {
		return
	}
	draw.Draw(img, rect, &image.Uniform{C: fill}, image.Point{}, draw.Src)
}

func drawNodeLabel(img *image.RGBA, rect image.Rectangle, label string, accent color.RGBA) {
	face := basicfont.Face7x13
	textWidth := font.MeasureString(face, label).Round()
	labelHeight := face.Metrics().Height.Round() + 4
	labelRect := image.Rect(rect.Min.X, rect.Min.Y-labelHeight, rect.Min.X+textWidth+6, rect.Min.Y)
	if labelRect.Min.Y < img.Bounds().Min.Y {
		labelRect = image.Rect(rect.Min.X, rect.Min.Y, rect.Min.X+textWidth+6, rect.Min.Y+labelHeight)
	}
	labelRect = labelRect.Intersect(img.Bounds())
	if labelRect.Dx() <= 0 || labelRect.Dy() <= 0 {
		return
	}

	fillRect(img, labelRect, accent)
	drawer := font.Drawer{
		Dst:  img,
		Src:  image.White,
		Face: face,
		Dot: fixed.Point26_6{
			X: fixed.I(labelRect.Min.X + 3),
			Y: fixed.I(labelRect.Min.Y + face.Metrics().Ascent.Round() + 2),
		},
	}
	drawer.DrawString(label)
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
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

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
	fmt.Fprintln(w, "  batch")
	fmt.Fprintln(w, "  browser")
	fmt.Fprintln(w, "  click")
	fmt.Fprintln(w, "  compare")
	fmt.Fprintln(w, "  close")
	fmt.Fprintln(w, "  help")
	fmt.Fprintln(w, "  eval")
	fmt.Fprintln(w, "  dblclick")
	fmt.Fprintln(w, "  find")
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
	fmt.Fprintln(w, "  viewport")
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
	case "batch":
		printBatchHelp(w)
	case "browser":
		printBrowserHelp(w)
	case "click":
		printClickHelp(w)
	case "compare":
		printCompareHelp(w)
	case "close":
		printCloseHelp(w)
	case "dblclick":
		printNodeActionHelp(w, "dblclick")
	case "eval":
		printEvalHelp(w)
	case "find":
		printFindHelp(w)
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
	case "viewport":
		printViewportHelp(w)
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
	fmt.Fprintln(w, "usage: nxctl attach browser --session <id> --backend <name> [--url <url>] [--viewport <width>x<height>] [--target-ref <path>]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "targets:")
	fmt.Fprintln(w, "  browser")
}

func printAttachBrowserHelp(w io.Writer) {
	fmt.Fprintf(w, "usage: nxctl attach browser --session <id> --backend chromium|lightpanda [--url <url>] [--viewport <width>x<height>] [--target-ref <path>]\n")
	fmt.Fprintf(w, "default viewport: %dx%d\n", defaultViewportWidth, defaultViewportHeight)
}

func printBackHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl back [--session <id>] [--json]")
}

func printBatchHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl batch --cmd "open https://example.com" --cmd "state" [--json]`)
	fmt.Fprintln(w, `   or: nxctl batch --cmd "find role button --all" --cmd "screenshot annotated.png --annotate" [--json]`)
}

func printBrowserHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl browser <setup|update|status|uninstall>")
	fmt.Fprintln(w, "   or: nxctl browser uninstall [--name chromium|lightpanda]")
}

func printClickHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl click <index|@eN> [--session <id>] [--json]")
	fmt.Fprintln(w, "   or: nxctl click <x> <y> [--session <id>] [--json]")
}

func printCompareHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl compare <old-url> <new-url> [--backend chromium|lightpanda] [--viewport <width>x<height>] [--wait-selector <css>] [--wait-timeout <ms>] [--ignore-text-regex <regex>]... [--json]")
	fmt.Fprintln(w, "   or: nxctl compare --old-session <id> --new-session <id> [--ignore-text-regex <regex>]... [--json]")
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

func printFindHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl find role <role> click [--name <text>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find role <role> input "text" [--name <text>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find role <role> get text|value|attributes|bbox [--name <text>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find role <role> --all [--name <text>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find text "text" click [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find text "text" get text|value|attributes|bbox [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find text "text" --all [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find label "label" input "text" [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find label "label" get value|attributes|bbox [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find label "label" --all [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find testid "value" click|get ... [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find testid "value" --all [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find href "value" click|get ... [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find href "value" --all [--session <id>] [--json]`)
}

func printGetHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl get title [--session <id>] [--json]")
	fmt.Fprintln(w, "   or: nxctl get html [--selector <css>] [--session <id>] [--json]")
	fmt.Fprintln(w, "   or: nxctl get text|value|attributes|bbox <index|@eN> [--session <id>] [--json]")
}

func printInputHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl input <index|@eN> "text" [--session <id>] [--json]`)
}

func printKeysHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl keys "Enter" [--session <id>] [--json]`)
}

func printNodeActionHelp(w io.Writer, command string) {
	switch command {
	case "hover":
		fmt.Fprintln(w, "usage: nxctl hover <index|@eN> [--session <id>] [--json]")
	case "dblclick":
		fmt.Fprintln(w, "usage: nxctl dblclick <index|@eN> [--session <id>] [--json]")
	case "rightclick":
		fmt.Fprintln(w, "usage: nxctl rightclick <index|@eN> [--session <id>] [--json]")
	}
}

func printObserveHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl observe [--session <id>] [--json] [--text] [--tree] [--screenshot] [--full]")
}

func printOpenHelp(w io.Writer) {
	fmt.Fprintf(w, "usage: nxctl open <url> [--session <id>] [--backend chromium|lightpanda] [--viewport <width>x<height>] [--json]\n")
	fmt.Fprintf(w, "default viewport: %dx%d\n", defaultViewportWidth, defaultViewportHeight)
}

func printScrollHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl scroll up|down [--session <id>] [--node <index>] [--amount <px>] [--json]")
}

func printScreenshotHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl screenshot [path] [--session <id>] [--full] [--annotate]")
}

func printSelectHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl select <index|@eN> "value" [--session <id>] [--json]`)
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
	fmt.Fprintln(w, "usage: nxctl upload <index|@eN> <path> [--session <id>] [--json]")
}

func printViewportHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl viewport <width>x<height> [--session <id>] [--json]")
	fmt.Fprintf(w, "default viewport: %dx%d\n", defaultViewportWidth, defaultViewportHeight)
}

func printWaitHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl wait selector "<css>" [--state attached|detached|visible|hidden] [--timeout <ms>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl wait text "value" [--timeout <ms>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl wait url "value" [--timeout <ms>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl wait navigation [--timeout <ms>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl wait function "js expr" [--timeout <ms>] [--session <id>] [--json]`)
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

func resolvedViewport(value string) (int, int, error) {
	if strings.TrimSpace(value) == "" {
		return defaultViewportWidth, defaultViewportHeight, nil
	}
	return parseViewport(value)
}

func parseViewport(value string) (int, int, error) {
	normalized := strings.TrimSpace(strings.ToLower(value))
	parts := strings.Split(normalized, "x")
	if len(parts) != 2 {
		return 0, 0, errors.New("viewport must be WIDTHxHEIGHT")
	}

	width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || width <= 0 {
		return 0, 0, errors.New("viewport width must be a positive integer")
	}
	height, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || height <= 0 {
		return 0, 0, errors.New("viewport height must be a positive integer")
	}

	return width, height, nil
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

func splitBatchCommand(value string) ([]string, error) {
	args := []string{}
	var current strings.Builder
	var quote rune
	escaped := false

	flush := func() {
		if current.Len() == 0 {
			return
		}
		args = append(args, current.String())
		current.Reset()
	}

	for _, r := range value {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
		case r == ' ' || r == '\t' || r == '\n':
			flush()
		default:
			current.WriteRune(r)
		}
	}

	if escaped {
		return nil, errors.New("unterminated escape in batch command")
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote in batch command")
	}

	flush()
	return args, nil
}
