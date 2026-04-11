package lightpanda

import (
	"context"
	"fmt"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

type Backend struct{}

func New() Backend {
	return Backend{}
}

func (Backend) Name() spec.BackendName {
	return spec.BackendLightpanda
}

func (Backend) Capabilities() spec.Capabilities {
	return spec.Capabilities{
		Observe: true,
	}
}

func (Backend) Attach(context.Context, spec.SessionConfig) error {
	return nil
}

func (Backend) Detach(context.Context) error {
	return nil
}

func (Backend) Observe(context.Context, api.ObserveOptions) (*api.Observation, error) {
	return &api.Observation{}, nil
}

func (Backend) Act(context.Context, api.Action) (*api.ActionResult, error) {
	return nil, fmt.Errorf("%w: act", spec.ErrUnsupported)
}

func (Backend) Screenshot(context.Context, string) error {
	return fmt.Errorf("%w: screenshot", spec.ErrUnsupported)
}

func (Backend) Logs(context.Context, api.LogOptions) ([]api.LogEntry, error) {
	return nil, fmt.Errorf("%w: logs", spec.ErrUnsupported)
}
