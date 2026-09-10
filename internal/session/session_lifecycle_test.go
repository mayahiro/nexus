package session

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/target/browser"
	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

type lifecycleBackend struct {
	fakeSessionBackend
	attach func(context.Context, spec.SessionConfig) error
	detach func(context.Context) error
}

func (b *lifecycleBackend) Attach(ctx context.Context, cfg spec.SessionConfig) error {
	if b.attach != nil {
		return b.attach(ctx, cfg)
	}
	return nil
}

func (b *lifecycleBackend) Detach(ctx context.Context) error {
	if b.detach != nil {
		return b.detach(ctx)
	}
	return nil
}

func lifecycleRequest(id string) api.AttachSessionRequest {
	return api.AttachSessionRequest{SessionID: id, TargetType: "browser", Backend: string(testBackendName)}
}

func TestShutdownAttemptsAllSessionsAndRetainsFailures(t *testing.T) {
	var calls atomic.Int32
	var fail atomic.Bool
	fail.Store(true)
	cleanupErr := errors.New("profile cleanup failed")
	restore := browser.SetBackendFactory(testBackendName, func() spec.Backend {
		return &lifecycleBackend{detach: func(context.Context) error {
			calls.Add(1)
			if fail.Load() {
				return cleanupErr
			}
			return nil
		}}
	})
	defer restore()
	m := NewManager()
	for _, id := range []string{"one", "two"} {
		if _, err := m.Attach(context.Background(), lifecycleRequest(id)); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Shutdown(context.Background()); !errors.Is(err, cleanupErr) {
		t.Fatalf("cleanup error lost: %v", err)
	}
	if calls.Load() != 2 || len(m.List()) != 2 {
		t.Fatalf("incomplete cleanup: calls=%d sessions=%v", calls.Load(), m.List())
	}
	fail.Store(false)
	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 4 || len(m.List()) != 0 {
		t.Fatal("failed cleanup could not be retried")
	}
}

func TestShutdownCanceledGateRetainsSession(t *testing.T) {
	restore := browser.SetBackendFactory(testBackendName, func() spec.Backend { return fakeSessionBackend{} })
	defer restore()
	m := NewManager()
	if _, err := m.Attach(context.Background(), lifecycleRequest("busy")); err != nil {
		t.Fatal(err)
	}
	e := m.sessions["busy"]
	if err := e.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.Shutdown(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	e.release()
	if len(m.List()) != 1 {
		t.Fatal("busy session forgotten")
	}
	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAttachDoesNotBlockOtherSessionsAndReservesID(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	restore := browser.SetBackendFactory(testBackendName, func() spec.Backend {
		return &lifecycleBackend{attach: func(ctx context.Context, cfg spec.SessionConfig) error {
			if cfg.SessionID != "slow" {
				return nil
			}
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}}
	})
	defer restore()
	m := NewManager()
	if _, err := m.Attach(context.Background(), lifecycleRequest("ready")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	attached := make(chan error, 1)
	go func() { _, err := m.Attach(ctx, lifecycleRequest("slow")); attached <- err }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("startup never entered")
	}
	done := make(chan error, 1)
	go func() {
		if len(m.List()) != 1 {
			done <- errors.New("pending session listed")
			return
		}
		_, err := m.Observe(ctx, "ready", api.ObserveOptions{})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("other session blocked on Attach")
	}
	if _, err := m.Attach(ctx, lifecycleRequest("slow")); !errors.Is(err, ErrSessionExists) {
		t.Fatalf("pending ID was not reserved: %v", err)
	}
	close(release)
	if err := <-attached; err != nil {
		t.Fatal(err)
	}
	if err := m.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestShutdownCancelsPendingAttachAndCleansSuccessfulStartup(t *testing.T) {
	entered := make(chan struct{})
	var detached atomic.Int32
	restore := browser.SetBackendFactory(testBackendName, func() spec.Backend {
		return &lifecycleBackend{
			attach: func(ctx context.Context, _ spec.SessionConfig) error { close(entered); <-ctx.Done(); return nil },
			detach: func(ctx context.Context) error { detached.Add(1); return ctx.Err() },
		}
	})
	defer restore()
	m := NewManager()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	attached := make(chan error, 1)
	go func() { _, err := m.Attach(ctx, lifecycleRequest("pending")); attached <- err }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("startup never entered")
	}
	if err := m.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-attached; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup registered: %v", err)
	}
	if detached.Load() != 1 || len(m.List()) != 0 || len(m.pending) != 0 {
		t.Fatal("pending startup leaked")
	}
}

func TestSessionOptionsReturnedToCallerAreIndependent(t *testing.T) {
	restore := browser.SetBackendFactory(testBackendName, func() spec.Backend { return fakeSessionBackend{} })
	defer restore()
	m := NewManager()
	req := lifecycleRequest("options")
	req.Options = map[string]string{"viewport_width": "1920"}
	attached, err := m.Attach(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	attached.Options["viewport_width"] = "1"
	listed := m.List()
	listed[0].Options["viewport_width"] = "2"
	if m.List()[0].Options["viewport_width"] != "1920" {
		t.Fatal("caller mutated internal options")
	}
	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
