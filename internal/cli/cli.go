package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	nagicli "github.com/mayahiro/nagicli-go"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/browsermgr"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/daemon"
)

const daemonStartTimeout = 3 * time.Second
const daemonHandshakeTimeout = 2 * time.Second
const daemonStopTimeout = 8 * time.Second
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
	currentDirectory, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	commandContext := nagicli.NewContextWithCancellation(
		strings.NewReader(""),
		stdout,
		stderr,
		nil,
		currentDirectory,
		ctx,
	)
	policy := nagicli.DefaultRuntimePolicy().WithExitCodePolicy(
		nagicli.DefaultExitCodePolicy().
			WithStatus(nagicli.CategoryUsage, nagicli.StatusFailure),
	)
	outcome, err := newNagiApplication().RunWithPolicy(commandContext, args, policy)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return int(outcome.Status())
}

func runDaemon(ctx context.Context, verbose bool, stderr io.Writer) int {
	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if err := daemon.Run(ctx, paths, daemon.RunOptions{Verbose: verbose}); err != nil {
		fmt.Fprintln(stderr, err)
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
	if res.DaemonVersion != api.DaemonVersion {
		fmt.Fprintf(stdout, "daemon build: mismatch (client=%s daemon=%s)\n", api.DaemonVersion, res.DaemonVersion)
		return 1
	}

	fmt.Fprintf(stdout, "protocol: ok (%s)\n", res.ProtocolVersion)
	fmt.Fprintf(stdout, "daemon build: ok (%s)\n", res.DaemonVersion)
	return 0
}

type batchStepResult struct {
	Command  string   `json:"command"`
	Args     []string `json:"args"`
	ExitCode int      `json:"exit_code"`
	Stdout   string   `json:"stdout,omitempty"`
	Stderr   string   `json:"stderr,omitempty"`
}

func runBatchInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	commands := nagiStringValues(invocation, "cmd")
	asJSON := nagiBoolValue(invocation, "json")
	keepGoing := nagiBoolValue(invocation, "keep-going")

	results := make([]batchStepResult, 0, len(commands))
	firstFailure := 0
	for _, raw := range commands {
		var stepStdout bytes.Buffer
		var stepStderr bytes.Buffer
		argv, err := splitBatchCommand(raw)
		exitCode := 0
		if err != nil {
			fmt.Fprintln(&stepStderr, err)
			exitCode = 1
		} else if len(argv) == 0 {
			fmt.Fprintln(&stepStderr, "batch command must not be empty")
			exitCode = 1
		} else {
			exitCode = Run(ctx, argv, &stepStdout, &stepStderr)
		}

		results = append(results, batchStepResult{
			Command:  raw,
			Args:     argv,
			ExitCode: exitCode,
			Stdout:   stepStdout.String(),
			Stderr:   stepStderr.String(),
		})
		if exitCode != 0 && firstFailure == 0 {
			firstFailure = exitCode
		}

		if asJSON {
			if exitCode != 0 && !keepGoing {
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
			if keepGoing {
				fmt.Fprintf(stderr, "batch step failed: %s\n", raw)
				continue
			}
			fmt.Fprintf(stderr, "batch stopped at: %s\n", raw)
			return exitCode
		}
	}

	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(results); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return firstFailure
	}

	return firstFailure
}

func runDaemonInvocation(ctx context.Context, invocation *nagicli.Invocation, _ io.Writer, stderr io.Writer) int {
	return runDaemon(ctx, nagiBoolValue(invocation, "verbose"), stderr)
}

func runDoctorInvocation(ctx context.Context, _ *nagicli.Invocation, stdout io.Writer, _ io.Writer) int {
	return runDoctor(ctx, stdout)
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
