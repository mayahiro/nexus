package browser

import (
	"testing"

	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

func TestNewBackend(t *testing.T) {
	backend, err := NewBackend(spec.BackendChromium)
	if err != nil {
		t.Fatal(err)
	}
	if backend.Name() != spec.BackendChromium {
		t.Fatalf("unexpected backend name: %s", backend.Name())
	}
}

func TestNewBackendRejectsRemovedLightpandaBackend(t *testing.T) {
	if _, err := NewBackend(spec.BackendName("lightpanda")); err == nil {
		t.Fatal("expected removed backend to be rejected")
	}
}
