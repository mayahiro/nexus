package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	server := Server{
		sessions: fakeSessionManager{
			detachErr: errors.New("boom"),
		},
		stop: func() {},
	}

	if _, err := server.DetachSession(context.Background(), api.DetachSessionRequest{
		SessionID: "web1",
	}); err == nil {
		t.Fatal("expected detach error")
	}
}

type fakeSessionManager struct {
	session   api.Session
	detachErr error
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

func (f fakeSessionManager) Observe(context.Context, string, api.ObserveOptions) (api.Observation, error) {
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
