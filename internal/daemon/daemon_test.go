package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/config"
)

func TestRunStopsAfterIdleTimeout(t *testing.T) {
	paths := configureDaemonTestEnv(t)

	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), paths, RunOptions{IdleTimeout: 300 * time.Millisecond})
	}()

	waitForSocket(t, paths.Socket, done)

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop after idle timeout")
	}

	if _, err := os.Stat(paths.Socket); !os.IsNotExist(err) {
		t.Fatalf("socket still exists: %s", paths.Socket)
	}
}

func TestRunRejectsSecondDaemonAfterSocketPathIsRemoved(t *testing.T) {
	paths := configureDaemonTestEnv(t)
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- Run(runCtx, paths, RunOptions{})
	}()
	waitForSocket(t, paths.Socket, done)

	pidData, err := os.ReadFile(paths.PID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(pidData)) != strconv.Itoa(os.Getpid()) {
		t.Fatalf("unexpected daemon pid: %q", pidData)
	}
	if err := os.Remove(paths.Socket); err != nil {
		t.Fatal(err)
	}

	err = Run(context.Background(), paths, RunOptions{})
	if err == nil || !strings.Contains(err.Error(), "daemon already running with pid") {
		t.Fatalf("unexpected second daemon error: %v", err)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first daemon did not stop")
	}
}

func TestProcessLogPathIncludesPID(t *testing.T) {
	path := ProcessLogPath("/tmp/nexus/nxd.log", 1234)
	if path != "/tmp/nexus/nxd.1234.log" {
		t.Fatalf("unexpected process log path: %s", path)
	}
}

func TestProcessLockRejectsConcurrentOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nxd.pid")
	first, err := acquireProcessLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseProcessLock(first)

	second, err := acquireProcessLock(path)
	if second != nil {
		releaseProcessLock(second)
	}
	if err == nil || !strings.Contains(err.Error(), "daemon already running with pid") {
		t.Fatalf("unexpected concurrent lock result: lock=%v err=%v", second, err)
	}
}

func TestServerDetachSessionDoesNotStopDaemon(t *testing.T) {
	stopped := false
	server := Server{
		sessions: fakeSessionManager{
			session: api.Session{ID: "web1"},
		},
		stop: func() {
			stopped = true
		},
	}

	res, err := server.DetachSession(context.Background(), api.DetachSessionRequest{
		SessionID: "web1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Session.ID != "web1" {
		t.Fatalf("unexpected detached session: %+v", res.Session)
	}
	if stopped {
		t.Fatal("daemon stopped after detach")
	}
}

func TestServerDetachSessionReturnsError(t *testing.T) {
	var entries []string
	server := Server{
		sessions: fakeSessionManager{
			detachErr: errors.New("boom"),
		},
		stop:   func() {},
		logger: func(entry string) { entries = append(entries, entry) },
	}

	if _, err := server.DetachSession(context.Background(), api.DetachSessionRequest{
		SessionID: "web1",
	}); err == nil {
		t.Fatal("expected detach error")
	}
	joined := strings.Join(entries, "\n")
	for _, expected := range []string{
		`request="detach_session" event="failure"`,
		`daemon_version=`,
		`go_version=`,
		`os=`,
		`os_version=`,
		`arch=`,
		`stage="daemon_request" event="start"`,
		`error="boom"`,
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("failure diagnostic does not contain %q:\n%s", expected, joined)
		}
	}
}

func TestServerNonVerboseSuccessDoesNotLogRequest(t *testing.T) {
	var entries []string
	server := Server{
		sessions: fakeSessionManager{},
		logger:   func(entry string) { entries = append(entries, entry) },
	}

	if _, err := server.ListSessions(context.Background(), api.ListSessionsRequest{}); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("successful non-verbose request emitted diagnostics: %v", entries)
	}
}

func TestServerVerboseSuccessLogsRequestStages(t *testing.T) {
	var entries []string
	server := Server{
		sessions: fakeSessionManager{},
		verbose:  true,
		logger:   func(entry string) { entries = append(entries, entry) },
	}

	if _, err := server.ListSessions(context.Background(), api.ListSessionsRequest{}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(entries, "\n")
	for _, expected := range []string{
		`stage="environment" event="snapshot"`,
		`stage="daemon_request" event="start"`,
		`stage="daemon_request" event="finish"`,
		`event="complete" outcome="success"`,
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("verbose diagnostic does not contain %q:\n%s", expected, joined)
		}
	}
}

func TestServerVerboseCoversEveryDaemonRequest(t *testing.T) {
	var entries []string
	server := Server{
		sessions: fakeSessionManager{session: api.Session{ID: "web1"}},
		verbose:  true,
		logger:   func(entry string) { entries = append(entries, entry) },
	}
	ctx := context.Background()

	if _, err := server.Ping(ctx, api.PingRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.AttachSession(ctx, api.AttachSessionRequest{SessionID: "web1", TargetType: "browser"}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.ListSessions(ctx, api.ListSessionsRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.DetachSession(ctx, api.DetachSessionRequest{SessionID: "web1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.ObserveSession(ctx, api.ObserveSessionRequest{SessionID: "web1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.ActSession(ctx, api.ActSessionRequest{SessionID: "web1", Action: api.Action{Kind: "click"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.StopDaemon(ctx, api.StopDaemonRequest{}); err != nil {
		t.Fatal(err)
	}
	if err := server.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(entries, "\n")
	for _, request := range []string{
		"ping",
		"attach_session",
		"list_sessions",
		"detach_session",
		"observe_session",
		"act_session",
		"stop_daemon",
		"shutdown",
	} {
		if !strings.Contains(joined, `request="`+request+`" event="complete" outcome="success"`) {
			t.Fatalf("verbose diagnostics do not cover %s:\n%s", request, joined)
		}
	}
}

func TestServerObserveSessionTimesOutScreenshots(t *testing.T) {
	previousTimeout := screenshotObserveTimeout
	screenshotObserveTimeout = 20 * time.Millisecond
	t.Cleanup(func() {
		screenshotObserveTimeout = previousTimeout
	})

	server := Server{
		sessions: fakeSessionManager{
			observe: func(ctx context.Context, _ string, _ api.ObserveOptions) (api.Observation, error) {
				<-ctx.Done()
				return api.Observation{}, ctx.Err()
			},
		},
		stop: func() {},
	}

	_, err := server.ObserveSession(context.Background(), api.ObserveSessionRequest{
		SessionID: "web1",
		Options:   api.ObserveOptions{WithScreenshot: true},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected screenshot observe deadline, got: %v", err)
	}
}

func TestServerVerbosePropagatesToObserveOptions(t *testing.T) {
	var observedOptions api.ObserveOptions
	server := Server{
		sessions: fakeSessionManager{
			observe: func(_ context.Context, _ string, opts api.ObserveOptions) (api.Observation, error) {
				observedOptions = opts
				return api.Observation{}, nil
			},
		},
		verbose: true,
	}

	if _, err := server.ObserveSession(context.Background(), api.ObserveSessionRequest{
		SessionID: "web1",
	}); err != nil {
		t.Fatal(err)
	}
	if !observedOptions.Verbose {
		t.Fatal("daemon verbose mode was not propagated to observe options")
	}
}

type fakeSessionManager struct {
	session   api.Session
	detachErr error
	observe   func(context.Context, string, api.ObserveOptions) (api.Observation, error)
}

func (f fakeSessionManager) Attach(context.Context, api.AttachSessionRequest) (api.Session, error) {
	return api.Session{}, nil
}

func (f fakeSessionManager) List() []api.Session {
	return nil
}

func (f fakeSessionManager) Detach(context.Context, string) (api.Session, error) {
	if f.detachErr != nil {
		return api.Session{}, f.detachErr
	}
	return f.session, nil
}

func (f fakeSessionManager) Observe(ctx context.Context, sessionID string, opts api.ObserveOptions) (api.Observation, error) {
	if f.observe != nil {
		return f.observe(ctx, sessionID, opts)
	}
	return api.Observation{}, nil
}

func (f fakeSessionManager) Act(context.Context, string, api.Action) (api.ActionResult, error) {
	return api.ActionResult{}, nil
}

func (f fakeSessionManager) Shutdown(context.Context) error {
	return nil
}

func configureDaemonTestEnv(t *testing.T) config.Paths {
	t.Helper()

	root, err := os.MkdirTemp("/tmp", "nexus-daemon-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.RemoveAll(root)
	})

	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(root, "run"))

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}

	return paths
}

func waitForSocket(t *testing.T, path string, done <-chan error) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("daemon stopped before socket was ready: %v", err)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("socket not ready: %s", path)
}
