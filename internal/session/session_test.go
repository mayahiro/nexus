package session

import (
	"context"
	"errors"
	"testing"

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
