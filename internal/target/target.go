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
}

// StyleInspector is implemented by target adapters that can inspect computed
// styles and their authored declarations for one observed node.
type StyleInspector interface {
	InspectStyles(ctx context.Context, req api.InspectStylesRequest) (*api.StyleInspection, error)
}
