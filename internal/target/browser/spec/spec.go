package spec

import (
	"context"
	"errors"

	"github.com/mayahiro/nexus/internal/api"
)

type BackendName string

const (
	BackendChromium   BackendName = "chromium"
	BackendLightpanda BackendName = "lightpanda"
)

var ErrUnsupported = errors.New("unsupported operation")

type Capabilities struct {
	Observe    bool
	Act        bool
	Screenshot bool
	Logs       bool
}

type SessionConfig struct {
	SessionID string
	TargetRef string
	Options   map[string]string
}

type Backend interface {
	Name() BackendName
	Capabilities() Capabilities
	Attach(ctx context.Context, cfg SessionConfig) error
	Detach(ctx context.Context) error
	Observe(ctx context.Context, opts api.ObserveOptions) (*api.Observation, error)
	Act(ctx context.Context, action api.Action) (*api.ActionResult, error)
	Screenshot(ctx context.Context, path string) error
	Logs(ctx context.Context, opts api.LogOptions) ([]api.LogEntry, error)
}

func CapabilityList(c Capabilities) []string {
	var out []string
	if c.Observe {
		out = append(out, "observe")
	}
	if c.Act {
		out = append(out, "act")
	}
	if c.Screenshot {
		out = append(out, "screenshot")
	}
	if c.Logs {
		out = append(out, "logs")
	}
	return out
}
