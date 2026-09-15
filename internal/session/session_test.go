package session

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/diagnostic"
	"github.com/mayahiro/nexus/internal/target/browser"
	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

const testBackendName spec.BackendName = "test"

func TestSessionGateCancellationIsTraced(t *testing.T) {
	var entries []string
	trace := diagnostic.New("act_session", true, func(entry string) {
		entries = append(entries, entry)
	})
	ctx, cancel := context.WithCancel(diagnostic.WithTrace(context.Background(), trace))
	cancel()

	sessionEntry := &entry{opGate: make(chan struct{}, 1)}
	err := sessionEntry.acquire(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled session gate, got %v", err)
	}
	trace.Finish(err)

	joined := strings.Join(entries, "\n")
	for _, expected := range []string{
		`stage="session_gate" event="wait"`,
		`stage="session_gate" event="canceled"`,
		`error="context canceled"`,
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("session gate diagnostic does not contain %q:\n%s", expected, joined)
		}
	}
}

func TestAttachListDetach(t *testing.T) {
	restoreBackend := browser.SetBackendFactory(testBackendName, func() spec.Backend {
		return fakeSessionBackend{}
	})
	defer restoreBackend()

	manager := NewManager()

	first, err := manager.Attach(context.Background(), api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  "web2",
		Backend:    "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	second, err := manager.Attach(context.Background(), api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  "web1",
		Backend:    "test",
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

	if first.Backend != "test" || second.Backend != "test" {
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
	restoreBackend := browser.SetBackendFactory(testBackendName, func() spec.Backend {
		return fakeSessionBackend{}
	})
	defer restoreBackend()

	manager := NewManager()

	_, err := manager.Attach(context.Background(), api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  "web1",
		Backend:    "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = manager.Attach(context.Background(), api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  "web1",
		Backend:    "test",
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
	restoreBackend := browser.SetBackendFactory(testBackendName, func() spec.Backend {
		return fakeSessionBackend{}
	})
	defer restoreBackend()

	manager := NewManager()

	_, err := manager.Attach(context.Background(), api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  "web1",
		Backend:    "test",
		TargetRef:  "/tmp/test",
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

func TestInspectStyles(t *testing.T) {
	backend := &styleSessionBackend{requests: make(chan api.InspectStylesRequest, 1)}
	restoreBackend := browser.SetBackendFactory(testBackendName, func() spec.Backend {
		return backend
	})
	defer restoreBackend()

	manager := NewManager()
	_, err := manager.Attach(context.Background(), api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  "web1",
		Backend:    "test",
		TargetRef:  "/tmp/test",
	})
	if err != nil {
		t.Fatal(err)
	}

	request := api.InspectStylesRequest{
		SessionID:     "web1",
		NodeRef:       "@e1",
		CSSProperties: []string{"width"},
	}
	inspection, err := manager.InspectStyles(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Computed["width"] != "154px" || inspection.StyleSourcesStatus != api.StyleSourcesStatusComplete {
		t.Fatalf("unexpected style inspection: %+v", inspection)
	}
	if got := <-backend.requests; got.NodeRef != "@e1" || got.SessionID != "web1" {
		t.Fatalf("unexpected backend style inspection request: %+v", got)
	}
}

func TestActSessionUnsupported(t *testing.T) {
	restoreBackend := browser.SetBackendFactory(testBackendName, func() spec.Backend {
		return fakeSessionBackend{}
	})
	defer restoreBackend()

	manager := NewManager()

	_, err := manager.Attach(context.Background(), api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  "web1",
		Backend:    "test",
		TargetRef:  "/tmp/test",
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
	backend := &serialSessionBackend{
		entered: make(chan string, 2),
		release: make(chan struct{}, 2),
	}
	restoreBackend := browser.SetBackendFactory(testBackendName, func() spec.Backend {
		return backend
	})
	defer restoreBackend()

	manager := NewManager()
	if _, err := manager.Attach(context.Background(), api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  "web1",
		Backend:    "test",
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
	backend := &serialSessionBackend{
		entered: make(chan string, 2),
		release: make(chan struct{}, 2),
	}
	restoreBackend := browser.SetBackendFactory(testBackendName, func() spec.Backend {
		return backend
	})
	defer restoreBackend()

	manager := NewManager()
	if _, err := manager.Attach(context.Background(), api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  "web1",
		Backend:    "test",
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

type fakeSessionBackend struct{}

func (fakeSessionBackend) Name() spec.BackendName {
	return testBackendName
}

func (fakeSessionBackend) Capabilities() spec.Capabilities {
	return spec.Capabilities{Observe: true}
}

func (fakeSessionBackend) Attach(context.Context, spec.SessionConfig) error {
	return nil
}

func (fakeSessionBackend) Detach(context.Context) error {
	return nil
}

func (fakeSessionBackend) Observe(context.Context, api.ObserveOptions) (*api.Observation, error) {
	return &api.Observation{
		URLOrScreen: "https://example.com",
		Title:       "Example",
		Text:        "Example text",
	}, nil
}

func (fakeSessionBackend) Act(context.Context, api.Action) (*api.ActionResult, error) {
	return nil, errors.New("unsupported operation")
}

type styleSessionBackend struct {
	fakeSessionBackend
	requests chan api.InspectStylesRequest
}

func (*styleSessionBackend) Capabilities() spec.Capabilities {
	return spec.Capabilities{Observe: true, StyleInspection: true}
}

func (b *styleSessionBackend) InspectStyles(_ context.Context, req api.InspectStylesRequest) (*api.StyleInspection, error) {
	b.requests <- req
	return &api.StyleInspection{
		Computed:           map[string]string{"width": "154px"},
		StyleSourcesStatus: api.StyleSourcesStatusComplete,
	}, nil
}

type serialSessionBackend struct {
	active  atomic.Int32
	maximum atomic.Int32
	entered chan string
	release chan struct{}
}

func (*serialSessionBackend) Name() spec.BackendName {
	return testBackendName
}

func (*serialSessionBackend) Capabilities() spec.Capabilities {
	return spec.Capabilities{Observe: true, Act: true}
}

func (*serialSessionBackend) Attach(context.Context, spec.SessionConfig) error {
	return nil
}

func (*serialSessionBackend) Detach(context.Context) error {
	return nil
}

func (b *serialSessionBackend) Observe(context.Context, api.ObserveOptions) (*api.Observation, error) {
	b.begin("observe")
	defer b.end()
	return &api.Observation{}, nil
}

func (b *serialSessionBackend) Act(context.Context, api.Action) (*api.ActionResult, error) {
	b.begin("act")
	defer b.end()
	return &api.ActionResult{OK: true}, nil
}

func (b *serialSessionBackend) begin(operation string) {
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

func (b *serialSessionBackend) end() {
	b.active.Add(-1)
}
