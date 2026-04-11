package session

import (
	"context"
	"errors"
	"testing"

	"github.com/mayahiro/nexus/internal/api"
)

func TestAttachListDetach(t *testing.T) {
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
