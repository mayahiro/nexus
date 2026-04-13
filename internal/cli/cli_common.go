package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/rpc"
)

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

func isHelpArgs(args []string) bool {
	return len(args) == 1 && (args[0] == "-h" || args[0] == "--help")
}

func parseCommandFlags(fs *flag.FlagSet, args []string, stderr io.Writer, command string) error {
	normalized := normalizeFlagArgs(fs, args)
	output := fs.Output()
	var buf bytes.Buffer
	fs.SetOutput(&buf)
	defer fs.SetOutput(output)

	if err := fs.Parse(normalized); err != nil {
		message := strings.TrimSpace(buf.String())
		if message != "" {
			fmt.Fprintln(stderr, message)
		}
		fmt.Fprintf(stderr, "hint: run `nxctl help %s` for details\n", command)
		return err
	}

	return nil
}

func normalizeFlagArgs(fs *flag.FlagSet, args []string) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}

		name, hasValue := parseFlagToken(arg)
		flags = append(flags, arg)
		if hasValue {
			continue
		}

		defined := fs.Lookup(name)
		if defined == nil || isBoolFlag(defined) {
			continue
		}
		if i+1 >= len(args) {
			continue
		}

		flags = append(flags, args[i+1])
		i++
	}

	return append(flags, positionals...)
}

func parseFlagToken(arg string) (string, bool) {
	trimmed := strings.TrimLeft(arg, "-")
	if trimmed == "" {
		return "", false
	}
	if index := strings.IndexByte(trimmed, '='); index >= 0 {
		return trimmed[:index], true
	}
	return trimmed, false
}

func isBoolFlag(def *flag.Flag) bool {
	if def == nil {
		return false
	}
	getter, ok := def.Value.(flag.Getter)
	if !ok {
		return false
	}
	_, ok = getter.Get().(bool)
	return ok
}

func printCommandHint(stderr io.Writer, command string, example string) {
	if strings.TrimSpace(example) != "" {
		fmt.Fprintf(stderr, "hint: %s\n", example)
	}
	fmt.Fprintf(stderr, "hint: run `nxctl help %s` for details\n", command)
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
