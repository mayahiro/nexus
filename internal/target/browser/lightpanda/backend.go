package lightpanda

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/target/browser/chromium"
	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

const startupTimeout = 5 * time.Second
const shutdownTimeout = 5 * time.Second
const maxLogEntries = 200
const lightpandaHost = "127.0.0.1"

type Backend struct {
	mu          sync.Mutex
	cmd         *exec.Cmd
	cancel      context.CancelFunc
	waitCh      chan error
	devtoolsURL string
	logs        []api.LogEntry
}

func New() *Backend {
	return &Backend{}
}

func (*Backend) Name() spec.BackendName {
	return spec.BackendLightpanda
}

func (*Backend) Capabilities() spec.Capabilities {
	return spec.Capabilities{
		Observe: true,
	}
}

func (b *Backend) Attach(ctx context.Context, cfg spec.SessionConfig) error {
	if cfg.TargetRef == "" {
		return errors.New("lightpanda executable path is required")
	}
	if _, err := os.Stat(cfg.TargetRef); err != nil {
		return err
	}

	b.mu.Lock()
	if b.cmd != nil {
		b.mu.Unlock()
		return errors.New("lightpanda backend is already attached")
	}
	b.mu.Unlock()

	port, err := reserveTCPPort()
	if err != nil {
		return err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	args := []string{
		"serve",
		"--host", lightpandaHost,
		"--port", strconv.Itoa(port),
		"--timeout", "86400",
	}

	cmd := exec.CommandContext(runCtx, cfg.TargetRef, args...)
	cmd.Env = append(os.Environ(), "LIGHTPANDA_DISABLE_TELEMETRY=true")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return err
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
		close(waitCh)
	}()

	devtoolsURL := fmt.Sprintf("ws://%s:%d", lightpandaHost, port)

	b.mu.Lock()
	b.cmd = cmd
	b.cancel = cancel
	b.waitCh = waitCh
	b.devtoolsURL = devtoolsURL
	b.logs = nil
	b.mu.Unlock()

	go b.captureLogs(stdout)
	go b.captureLogs(stderr)

	if err := waitForTCP(ctx, lightpandaHost, port, waitCh); err != nil {
		b.Detach(context.Background())
		return err
	}

	navigateURL := initialURL(cfg.Options)
	navigateCtx, navigateCancel := context.WithTimeout(ctx, startupTimeout)
	defer navigateCancel()

	if err := chromium.NavigateViaCDP(navigateCtx, devtoolsURL, navigateURL, chromedp.NoModifyURL); err != nil {
		b.Detach(context.Background())
		return err
	}

	return nil
}

func (b *Backend) Detach(_ context.Context) error {
	b.mu.Lock()
	cmd := b.cmd
	cancel := b.cancel
	waitCh := b.waitCh
	b.cmd = nil
	b.cancel = nil
	b.waitCh = nil
	b.devtoolsURL = ""
	b.mu.Unlock()

	if cmd == nil {
		return nil
	}

	cancel()

	timer := time.NewTimer(shutdownTimeout)
	defer timer.Stop()

	select {
	case <-waitCh:
	case <-timer.C:
		if cmd.Process != nil {
			cmd.Process.Signal(syscall.SIGKILL)
		}
		<-waitCh
	}

	return nil
}

func (b *Backend) Observe(ctx context.Context, opts api.ObserveOptions) (*api.Observation, error) {
	b.mu.Lock()
	devtoolsURL := b.devtoolsURL
	b.mu.Unlock()

	if devtoolsURL == "" {
		return nil, errors.New("lightpanda backend is not attached")
	}

	return chromium.ObserveViaCDP(ctx, devtoolsURL, opts, chromedp.NoModifyURL)
}

func (*Backend) Act(context.Context, api.Action) (*api.ActionResult, error) {
	return nil, fmt.Errorf("%w: act", spec.ErrUnsupported)
}

func (*Backend) Screenshot(context.Context, string) error {
	return fmt.Errorf("%w: screenshot", spec.ErrUnsupported)
}

func (*Backend) Logs(context.Context, api.LogOptions) ([]api.LogEntry, error) {
	return nil, fmt.Errorf("%w: logs", spec.ErrUnsupported)
}

func (b *Backend) captureLogs(reader io.Reader) {
	buf := make([]byte, 0, 4096)
	chunk := make([]byte, 1024)

	for {
		n, err := reader.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
			for {
				index := strings.IndexByte(string(buf), '\n')
				if index < 0 {
					break
				}
				line := strings.TrimSpace(string(buf[:index]))
				buf = buf[index+1:]
				if line != "" {
					b.appendLog(line)
				}
			}
		}

		if err != nil {
			if len(buf) > 0 {
				line := strings.TrimSpace(string(buf))
				if line != "" {
					b.appendLog(line)
				}
			}
			return
		}
	}
}

func (b *Backend) appendLog(message string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.logs = append(b.logs, api.LogEntry{
		Time:    time.Now(),
		Level:   "info",
		Message: message,
	})
	if len(b.logs) > maxLogEntries {
		b.logs = append([]api.LogEntry(nil), b.logs[len(b.logs)-maxLogEntries:]...)
	}
}

func reserveTCPPort() (int, error) {
	listener, err := net.Listen("tcp", net.JoinHostPort(lightpandaHost, "0"))
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("failed to resolve tcp port")
	}

	return address.Port, nil
}

func waitForTCP(ctx context.Context, host string, port int, waitCh <-chan error) error {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	deadline := time.Now().Add(startupTimeout)

	for {
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}

		select {
		case waitErr, ok := <-waitCh:
			if !ok || waitErr == nil {
				return errors.New("lightpanda exited before startup completed")
			}
			return waitErr
		default:
		}

		if time.Now().After(deadline) {
			return errors.New("lightpanda startup timed out")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func initialURL(options map[string]string) string {
	if options != nil && options["initial_url"] != "" {
		return options["initial_url"]
	}
	return "about:blank"
}
