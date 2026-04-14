package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/browsermgr"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/daemon"
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
	case "fill":
		return runFill(ctx, args[1:], stdout, stderr)
	case "find":
		return runFind(ctx, args[1:], stdout, stderr)
	case "get":
		return runGet(ctx, args[1:], stdout, stderr)
	case "help":
		return runHelp(args[1:], stdout, stderr)
	case "hover":
		return runHover(ctx, args[1:], stdout, stderr)
	case "inspect":
		return runInspect(ctx, args[1:], stdout, stderr)
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

	if err := parseCommandFlags(fs, args, stderr, "batch"); err != nil {
		return 1
	}
	if len(commands) == 0 {
		fmt.Fprintln(stderr, "batch requires at least one --cmd")
		printCommandHint(stderr, "batch", `nxctl batch --cmd "state" --cmd "find role button --all"`)
		return 1
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "batch does not accept positional arguments")
		printCommandHint(stderr, "batch", `nxctl batch --cmd "state"`)
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
