package browser

import (
	"context"
	"fmt"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

type Adapter struct {
	backend spec.Backend
}

func NewAdapter(backend spec.Backend) *Adapter {
	return &Adapter{backend: backend}
}

func (a *Adapter) Attach(ctx context.Context, cfg api.AttachConfig) error {
	return a.backend.Attach(ctx, spec.SessionConfig{
		SessionID: cfg.SessionID,
		TargetRef: cfg.TargetRef,
		Options:   cfg.Options,
	})
}

func (a *Adapter) Detach(ctx context.Context) error {
	return a.backend.Detach(ctx)
}

func (a *Adapter) Observe(ctx context.Context, opts api.ObserveOptions) (*api.Observation, error) {
	capabilities := a.backend.Capabilities()
	if !capabilities.Observe {
		return nil, fmt.Errorf("%w: observe", spec.ErrUnsupported)
	}
	if opts.WithScreenshot && !capabilities.Screenshot {
		return nil, fmt.Errorf("%w: screenshot", spec.ErrUnsupported)
	}

	obs, err := a.backend.Observe(ctx, opts)
	if err != nil {
		return nil, err
	}

	if obs != nil {
		obs.TargetType = "browser"
		obs.Capabilities = spec.CapabilityList(capabilities)
		if obs.Meta == nil {
			obs.Meta = map[string]string{}
		}
		obs.Meta["browser_backend"] = string(a.backend.Name())
	}

	return obs, nil
}

func (a *Adapter) Act(ctx context.Context, action api.Action) (*api.ActionResult, error) {
	if !a.backend.Capabilities().Act {
		return nil, fmt.Errorf("%w: act", spec.ErrUnsupported)
	}

	return a.backend.Act(ctx, action)
}

func (a *Adapter) Screenshot(ctx context.Context, path string) error {
	if !a.backend.Capabilities().Screenshot {
		return fmt.Errorf("%w: screenshot", spec.ErrUnsupported)
	}

	return a.backend.Screenshot(ctx, path)
}

func (a *Adapter) Logs(ctx context.Context, opts api.LogOptions) ([]api.LogEntry, error) {
	if !a.backend.Capabilities().Logs {
		return nil, fmt.Errorf("%w: logs", spec.ErrUnsupported)
	}

	return a.backend.Logs(ctx, opts)
}
