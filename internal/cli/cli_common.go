package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/rpc"
)

func ensureDaemon(ctx context.Context, paths config.Paths) (*rpc.Client, bool, error) {
	client, err := rpc.Dial(ctx, paths.Socket)
	if err == nil {
		compatible, compatibilityErr := checkDaemonCompatibility(ctx, client)
		if compatibilityErr != nil {
			client.Close()
			return nil, false, compatibilityErr
		}
		if compatible {
			return client, false, nil
		}
		if err := replaceIncompatibleDaemon(ctx, client, paths.Socket); err != nil {
			return nil, false, err
		}
	}

	if err := startDaemonProcess(paths); err != nil {
		return nil, false, err
	}

	client, err = waitForDaemon(ctx, paths.Socket)
	if err != nil {
		return nil, true, err
	}
	compatible, compatibilityErr := checkDaemonCompatibility(ctx, client)
	if compatibilityErr != nil {
		client.Close()
		return nil, true, compatibilityErr
	}
	if !compatible {
		if err := replaceIncompatibleDaemon(ctx, client, paths.Socket); err != nil {
			return nil, true, err
		}
		return nil, true, errors.New("started nxd is incompatible with this nxctl build")
	}

	return client, true, nil
}

func checkDaemonCompatibility(ctx context.Context, client *rpc.Client) (bool, error) {
	pingCtx, cancel := context.WithTimeout(ctx, daemonHandshakeTimeout)
	defer cancel()

	response, err := client.Ping(pingCtx)
	if err != nil {
		return false, fmt.Errorf("daemon handshake failed: %w", err)
	}
	return daemonCompatible(response), nil
}

func daemonCompatible(response api.PingResponse) bool {
	return response.ProtocolVersion == api.ProtocolVersion && response.DaemonVersion == api.DaemonVersion
}

func replaceIncompatibleDaemon(ctx context.Context, client *rpc.Client, socket string) error {
	stopCtx, cancel := context.WithTimeout(ctx, daemonStopTimeout)
	defer cancel()

	if _, err := client.StopDaemon(stopCtx); err != nil {
		client.Close()
		return fmt.Errorf("stop incompatible daemon: %w", err)
	}
	if err := client.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("close incompatible daemon connection: %w", err)
	}

	deadline := time.Now().Add(daemonStopTimeout)
	for {
		probe, err := rpc.Dial(stopCtx, socket)
		if err != nil {
			if _, statErr := os.Stat(socket); errors.Is(statErr, os.ErrNotExist) {
				return nil
			}
		} else {
			probe.Close()
		}

		if time.Now().After(deadline) {
			return errors.New("incompatible daemon did not stop")
		}
		select {
		case <-stopCtx.Done():
			return fmt.Errorf("wait for incompatible daemon to stop: %w", stopCtx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
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
	current, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(current), "nxd")
		if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return candidate, nil
		}
	}

	if path, lookupErr := exec.LookPath("nxd"); lookupErr == nil {
		return path, nil
	}

	if err != nil {
		return "", err
	}
	return "", errors.New("nxd executable not found beside nxctl or on PATH")
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
