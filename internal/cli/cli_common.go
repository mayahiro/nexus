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
	"github.com/mayahiro/nexus/internal/daemon"
	"github.com/mayahiro/nexus/internal/rpc"
)

func ensureDaemon(ctx context.Context, paths config.Paths) (*rpc.Client, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

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
		client.Close()
	}

	startupLock, err := acquireDaemonStartupLock(ctx, daemonStartupLockPath(paths.PID))
	if err != nil {
		return nil, false, err
	}
	defer releaseDaemonStartupLock(startupLock)

	return ensureDaemonLocked(ctx, paths)
}

func ensureDaemonLocked(ctx context.Context, paths config.Paths) (*rpc.Client, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

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
		if err := replaceIncompatibleDaemon(ctx, client, paths); err != nil {
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
		if err := replaceIncompatibleDaemon(ctx, client, paths); err != nil {
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

func replaceIncompatibleDaemon(ctx context.Context, client *rpc.Client, paths config.Paths) error {
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
		_, socketErr := os.Stat(paths.Socket)
		socketRemoved := errors.Is(socketErr, os.ErrNotExist)
		if socketErr != nil && !socketRemoved {
			return fmt.Errorf("check incompatible daemon socket: %w", socketErr)
		}
		lockReleased, err := daemonProcessLockReleased(paths.PID)
		if err != nil {
			return err
		}
		if socketRemoved && lockReleased {
			return nil
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

func daemonStartupLockPath(pidPath string) string {
	extension := filepath.Ext(pidPath)
	name := strings.TrimSuffix(filepath.Base(pidPath), extension)
	return filepath.Join(filepath.Dir(pidPath), name+".start.lock")
}

func acquireDaemonStartupLock(ctx context.Context, path string) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			lock.Close()
			return nil, err
		}
		err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
				lock.Close()
				return nil, ctxErr
			}
			return lock, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			lock.Close()
			return nil, fmt.Errorf("lock daemon startup file %s: %w", path, err)
		}
		select {
		case <-ctx.Done():
			lock.Close()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func releaseDaemonStartupLock(lock *os.File) {
	if lock == nil {
		return
	}
	syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	lock.Close()
}

func daemonProcessLockReleased(path string) (bool, error) {
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return false, err
	}
	defer lock.Close()

	err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("probe daemon pid lock %s: %w", path, err)
	}
	syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return true, nil
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

	logFile, err := os.CreateTemp(filepath.Dir(paths.Log), ".nxd-starting-*.log")
	if err != nil {
		return err
	}
	defer logFile.Close()
	logPath := logFile.Name()
	removeLog := true
	defer func() {
		if removeLog {
			os.Remove(logPath)
		}
	}()
	if err := logFile.Chmod(0o644); err != nil {
		return err
	}

	executable, err := findDaemonExecutable()
	if err != nil {
		return err
	}

	cmd := exec.Command(executable)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.Env = daemonProcessEnvironment(paths.Log)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return err
	}

	processLog := daemon.ProcessLogPath(paths.Log, cmd.Process.Pid)
	if err := os.Rename(logPath, processLog); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return fmt.Errorf("name daemon log for pid %d: %w", cmd.Process.Pid, err)
	}
	removeLog = false

	return cmd.Process.Release()
}

func daemonProcessEnvironment(logBase string) []string {
	prefix := daemon.ProcessLogBaseEnv + "="
	environment := make([]string, 0, len(os.Environ())+1)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, prefix) {
			continue
		}
		environment = append(environment, value)
	}
	return append(environment, prefix+logBase)
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
