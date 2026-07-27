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

func TestAttachListDetach(t *testing.T) {
	restoreBackend := browser.SetBackendFactory(spec.BackendLightpanda, func() spec.Backend {
		return fakeLightpandaBackend{}
	})
	defer restoreBackend()

	manager := NewManager()

	first, err := manager.Attach(context.Background(), api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  "web2",
		Backend:    "lightpanda",
	})
	if err != nil {
		t.Fatal(err)
	}

	second, err := manager.Attach(context.Background(), api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  "web1",
		Backend:    "lightpanda",
	})
	if err != nil {
		t.Fatal(err)
	}

	sessions := manager.List()
	if len(sessions) != 2 {
		t.Fatalf("unexpected session count: %d", len(sessions))
	}

	if sessions[0].ID != "web1" || sessions[1].ID != "web2" {
		t.Fatalf("unexpected session order: %+v", sessions)
	}

	if first.Backend != "lightpanda" || second.Backend != "lightpanda" {
		t.Fatalf("unexpected backend values: %+v %+v", first, second)
	}

	detached, err := manager.Detach(context.Background(), "web1")
	if err != nil {
		t.Fatal(err)
	}

	if detached.ID != "web1" {
		t.Fatalf("unexpected detached session: %+v", detached)
	}
}

func TestAttachDuplicateSession(t *testing.T) {
	restoreBackend := browser.SetBackendFactory(spec.BackendLightpanda, func() spec.Backend {
		return fakeLightpandaBackend{}
	})
	defer restoreBackend()

	manager := NewManager()

	_, err := manager.Attach(context.Background(), api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  "web1",
		Backend:    "lightpanda",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = manager.Attach(context.Background(), api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  "web1",
		Backend:    "lightpanda",
	})
	if !errors.Is(err, ErrSessionExists) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDetachMissingSession(t *testing.T) {
	manager := NewManager()

	_, err := manager.Detach(context.Background(), "missing")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestObserveSession(t *testing.T) {
	restoreBackend := browser.SetBackendFactory(spec.BackendLightpanda, func() spec.Backend {
		return fakeLightpandaBackend{}
	})
	defer restoreBackend()

	manager := NewManager()

	_, err := manager.Attach(context.Background(), api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  "web1",
		Backend:    "lightpanda",
		TargetRef:  "/tmp/lightpanda",
	})
	if err != nil {
		t.Fatal(err)
	}

	observation, err := manager.Observe(context.Background(), "web1", api.ObserveOptions{
		WithText: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if observation.SessionID != "web1" {
		t.Fatalf("unexpected observation: %+v", observation)
	}
	if observation.TargetType != "browser" {
		t.Fatalf("unexpected observation: %+v", observation)
	}
}

func TestActSessionUnsupported(t *testing.T) {
	restoreBackend := browser.SetBackendFactory(spec.BackendLightpanda, func() spec.Backend {
		return fakeLightpandaBackend{}
	})
	defer restoreBackend()

	manager := NewManager()

	_, err := manager.Attach(context.Background(), api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  "web1",
		Backend:    "lightpanda",
		TargetRef:  "/tmp/lightpanda",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = manager.Act(context.Background(), "web1", api.Action{
		Kind: "eval",
		Text: "document.title",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOperationsForSameSessionAreSerialized(t *testing.T) {
	backend := &serialLightpandaBackend{
		entered: make(chan string, 2),
		release: make(chan struct{}, 2),
	}
	restoreBackend := browser.SetBackendFactory(spec.BackendLightpanda, func() spec.Backend {
		return backend
	})
	defer restoreBackend()

	manager := NewManager()
	if _, err := manager.Attach(context.Background(), api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  "web1",
		Backend:    "lightpanda",
	}); err != nil {
		t.Fatal(err)
	}

	observeDone := make(chan error, 1)
	go func() {
		_, err := manager.Observe(context.Background(), "web1", api.ObserveOptions{WithText: true})
		observeDone <- err
	}()

	if operation := <-backend.entered; operation != "observe" {
		t.Fatalf("unexpected first operation: %s", operation)
	}

	actDone := make(chan error, 1)
	go func() {
		_, err := manager.Act(context.Background(), "web1", api.Action{Kind: "eval"})
		actDone <- err
	}()

	select {
	case operation := <-backend.entered:
		t.Fatalf("operation ran concurrently: %s", operation)
	case <-time.After(50 * time.Millisecond):
	}

	backend.release <- struct{}{}
	if err := <-observeDone; err != nil {
		t.Fatal(err)
	}

	if operation := <-backend.entered; operation != "act" {
		t.Fatalf("unexpected second operation: %s", operation)
	}
	backend.release <- struct{}{}
	if err := <-actDone; err != nil {
		t.Fatal(err)
	}

	if maximum := backend.maximum.Load(); maximum != 1 {
		t.Fatalf("unexpected concurrent operation count: %d", maximum)
	}
}

func TestOperationQueueHonorsContextCancellation(t *testing.T) {
	backend := &serialLightpandaBackend{
		entered: make(chan string, 2),
		release: make(chan struct{}, 2),
	}
	restoreBackend := browser.SetBackendFactory(spec.BackendLightpanda, func() spec.Backend {
		return backend
	})
	defer restoreBackend()

	manager := NewManager()
	if _, err := manager.Attach(context.Background(), api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  "web1",
		Backend:    "lightpanda",
	}); err != nil {
		t.Fatal(err)
	}

	observeDone := make(chan error, 1)
	go func() {
		_, err := manager.Observe(context.Background(), "web1", api.ObserveOptions{WithText: true})
		observeDone <- err
	}()
	if operation := <-backend.entered; operation != "observe" {
		t.Fatalf("unexpected first operation: %s", operation)
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := manager.Act(waitCtx, "web1", api.Action{Kind: "eval"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected queued operation deadline, got %v", err)
	}
	select {
	case operation := <-backend.entered:
		t.Fatalf("canceled operation reached backend: %s", operation)
	default:
	}

	backend.release <- struct{}{}
	if err := <-observeDone; err != nil {
		t.Fatal(err)
	}
}

type fakeLightpandaBackend struct{}

func (fakeLightpandaBackend) Name() spec.BackendName {
	return spec.BackendLightpanda
}

func (fakeLightpandaBackend) Capabilities() spec.Capabilities {
	return spec.Capabilities{Observe: true}
}

func (fakeLightpandaBackend) Attach(context.Context, spec.SessionConfig) error {
	return nil
}

func (fakeLightpandaBackend) Detach(context.Context) error {
	return nil
}

func (fakeLightpandaBackend) Observe(context.Context, api.ObserveOptions) (*api.Observation, error) {
	return &api.Observation{
		URLOrScreen: "https://example.com",
		Title:       "Example",
		Text:        "Example text",
	}, nil
}

func (fakeLightpandaBackend) Act(context.Context, api.Action) (*api.ActionResult, error) {
	return nil, errors.New("unsupported operation")
}

func (fakeLightpandaBackend) Screenshot(context.Context, string) error {
	return nil
}

func (fakeLightpandaBackend) Logs(context.Context, api.LogOptions) ([]api.LogEntry, error) {
	return nil, nil
}

type serialLightpandaBackend struct {
	active  atomic.Int32
	maximum atomic.Int32
	entered chan string
	release chan struct{}
}

func (*serialLightpandaBackend) Name() spec.BackendName {
	return spec.BackendLightpanda
}

func (*serialLightpandaBackend) Capabilities() spec.Capabilities {
	return spec.Capabilities{Observe: true, Act: true}
}

func (*serialLightpandaBackend) Attach(context.Context, spec.SessionConfig) error {
	return nil
}

func (*serialLightpandaBackend) Detach(context.Context) error {
	return nil
}

func (b *serialLightpandaBackend) Observe(context.Context, api.ObserveOptions) (*api.Observation, error) {
	b.begin("observe")
	defer b.end()
	return &api.Observation{}, nil
}

func (b *serialLightpandaBackend) Act(context.Context, api.Action) (*api.ActionResult, error) {
	b.begin("act")
	defer b.end()
	return &api.ActionResult{OK: true}, nil
}

func (*serialLightpandaBackend) Screenshot(context.Context, string) error {
	return nil
}

func (*serialLightpandaBackend) Logs(context.Context, api.LogOptions) ([]api.LogEntry, error) {
	return nil, nil
}

func (b *serialLightpandaBackend) begin(operation string) {
	active := b.active.Add(1)
	for {
		maximum := b.maximum.Load()
		if active <= maximum || b.maximum.CompareAndSwap(maximum, active) {
			break
		}
	}
	b.entered <- operation
	<-b.release
}

func (b *serialLightpandaBackend) end() {
	b.active.Add(-1)
}
