package browser

import (
	"context"
	"errors"
	"testing"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

type testBackend struct {
	name         spec.BackendName
	capabilities spec.Capabilities
	observe      *api.Observation
}

func (b testBackend) Name() spec.BackendName {
	return b.name
}

func (b testBackend) Capabilities() spec.Capabilities {
	return b.capabilities
}

func (b testBackend) Attach(context.Context, spec.SessionConfig) error {
	return nil
}

func (b testBackend) Detach(context.Context) error {
	return nil
}

func (b testBackend) Observe(context.Context, api.ObserveOptions) (*api.Observation, error) {
	return b.observe, nil
}

func (b testBackend) Act(context.Context, api.Action) (*api.ActionResult, error) {
	return &api.ActionResult{OK: true}, nil
}

func (b testBackend) Screenshot(context.Context, string) error {
	return nil
}

func (b testBackend) Logs(context.Context, api.LogOptions) ([]api.LogEntry, error) {
	return nil, nil
}

func TestObserveAddsBackendMetaAndCapabilities(t *testing.T) {
	adapter := NewAdapter(testBackend{
		name: spec.BackendLightpanda,
		capabilities: spec.Capabilities{
			Observe: true,
		},
		observe: &api.Observation{Title: "Example"},
	})

	obs, err := adapter.Observe(context.Background(), api.ObserveOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if obs.TargetType != "browser" {
		t.Fatalf("unexpected target type: %s", obs.TargetType)
	}

	if obs.Meta["browser_backend"] != "lightpanda" {
		t.Fatalf("unexpected backend meta: %v", obs.Meta)
	}

	if len(obs.Capabilities) != 1 || obs.Capabilities[0] != "observe" {
		t.Fatalf("unexpected capabilities: %v", obs.Capabilities)
	}
}

func TestActReturnsUnsupportedForObserveOnlyBackend(t *testing.T) {
	adapter := NewAdapter(testBackend{
		name: spec.BackendLightpanda,
		capabilities: spec.Capabilities{
			Observe: true,
		},
	})

	_, err := adapter.Act(context.Background(), api.Action{Kind: "invoke"})
	if !errors.Is(err, spec.ErrUnsupported) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestObserveReturnsUnsupportedForScreenshotOnObserveOnlyBackend(t *testing.T) {
	adapter := NewAdapter(testBackend{
		name: spec.BackendLightpanda,
		capabilities: spec.Capabilities{
			Observe: true,
		},
	})

	_, err := adapter.Observe(context.Background(), api.ObserveOptions{WithScreenshot: true})
	if !errors.Is(err, spec.ErrUnsupported) {
		t.Fatalf("unexpected error: %v", err)
	}
}
