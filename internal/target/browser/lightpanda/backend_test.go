package lightpanda

import (
	"context"
	"testing"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

func TestCapabilities(t *testing.T) {
	backend := New()

	if backend.Name() != spec.BackendLightpanda {
		t.Fatalf("unexpected backend name: %s", backend.Name())
	}

	capabilities := backend.Capabilities()
	if !capabilities.Observe {
		t.Fatal("expected observe capability")
	}
	if capabilities.Act || capabilities.Screenshot || capabilities.Logs {
		t.Fatalf("unexpected capabilities: %+v", capabilities)
	}
}

func TestInitialURL(t *testing.T) {
	if initialURL(nil) != "about:blank" {
		t.Fatalf("unexpected default initial url: %s", initialURL(nil))
	}

	options := map[string]string{"initial_url": "https://example.com"}
	if initialURL(options) != "https://example.com" {
		t.Fatalf("unexpected initial url: %s", initialURL(options))
	}
}

func TestUnsupportedOperations(t *testing.T) {
	backend := New()

	if _, err := backend.Act(context.Background(), api.Action{Kind: "invoke"}); err == nil {
		t.Fatal("expected act to be unsupported")
	}
	if err := backend.Screenshot(context.Background(), "out.png"); err == nil {
		t.Fatal("expected screenshot to be unsupported")
	}
	if _, err := backend.Logs(context.Background(), api.LogOptions{}); err == nil {
		t.Fatal("expected logs to be unsupported")
	}
}

func TestReserveTCPPort(t *testing.T) {
	port, err := reserveTCPPort()
	if err != nil {
		t.Fatal(err)
	}
	if port <= 0 {
		t.Fatalf("unexpected port: %d", port)
	}
}
