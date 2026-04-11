package target

import (
	"context"

	"github.com/mayahiro/nexus/internal/api"
)

type Adapter interface {
	Attach(ctx context.Context, cfg api.AttachConfig) error
	Detach(ctx context.Context) error
	Observe(ctx context.Context, opts api.ObserveOptions) (*api.Observation, error)
	Act(ctx context.Context, action api.Action) (*api.ActionResult, error)
	Screenshot(ctx context.Context, path string) error
	Logs(ctx context.Context, opts api.LogOptions) ([]api.LogEntry, error)
}
