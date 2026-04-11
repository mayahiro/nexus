package browser

import (
	"testing"

	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

func TestNewBackend(t *testing.T) {
	backends := []struct {
		name spec.BackendName
	}{
		{name: spec.BackendChromium},
		{name: spec.BackendLightpanda},
	}

	for _, tt := range backends {
		backend, err := NewBackend(tt.name)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", tt.name, err)
		}
		if backend.Name() != tt.name {
			t.Fatalf("unexpected backend name: %s", backend.Name())
		}
	}
}
