package cli

import (
	"context"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/browsermgr"
	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

type fakeBrowserManager struct{}
type fakeChromiumBackend struct{}
type autoStartChromiumBackend struct{}
type missingBrowserManager struct{}

func (fakeChromiumBackend) Name() spec.BackendName {
	return spec.BackendChromium
}

func (fakeChromiumBackend) Capabilities() spec.Capabilities {
	return spec.Capabilities{Observe: true}
}

func (fakeChromiumBackend) Attach(context.Context, spec.SessionConfig) error {
	return nil
}

func (fakeChromiumBackend) Detach(context.Context) error {
	return nil
}

func (fakeChromiumBackend) Observe(context.Context, api.ObserveOptions) (*api.Observation, error) {
	return &api.Observation{
		URLOrScreen: "https://example.com",
		Title:       "Example",
		Text:        "Example text",
		Tree: []api.Node{
			{
				ID:      1,
				Ref:     "@e1",
				Role:    "link",
				Name:    "Docs",
				Visible: true,
				Enabled: true,
				LocatorHints: []api.LocatorHint{
					{Kind: "role", Value: "link", Name: "Docs", Command: `role link --name "Docs"`},
					{Kind: "text", Value: "Docs", Command: `text "Docs"`},
					{Kind: "href", Value: "/docs", Command: `href "/docs"`},
				},
			},
		},
	}, nil
}

func (fakeChromiumBackend) Act(context.Context, api.Action) (*api.ActionResult, error) {
	return nil, nil
}

func (autoStartChromiumBackend) Name() spec.BackendName {
	return spec.BackendChromium
}

func (autoStartChromiumBackend) Capabilities() spec.Capabilities {
	return spec.Capabilities{Observe: true, Act: true}
}

func (autoStartChromiumBackend) Attach(context.Context, spec.SessionConfig) error {
	return nil
}

func (autoStartChromiumBackend) Detach(context.Context) error {
	return nil
}

func (autoStartChromiumBackend) Observe(context.Context, api.ObserveOptions) (*api.Observation, error) {
	return &api.Observation{
		URLOrScreen: "https://example.com",
		Title:       "Example Title",
		Text:        "Example text",
	}, nil
}

func (autoStartChromiumBackend) Act(_ context.Context, action api.Action) (*api.ActionResult, error) {
	switch action.Kind {
	case "eval":
		if action.Text == "document.title" {
			return &api.ActionResult{OK: true, Value: "Example Title"}, nil
		}
	case "get":
		if action.Args["target"] == "title" {
			return &api.ActionResult{OK: true, Value: "Example Title"}, nil
		}
	}
	return &api.ActionResult{OK: true}, nil
}

func (fakeBrowserManager) Setup(context.Context) (browsermgr.SetupResult, error) {
	return browsermgr.SetupResult{
		Browsers: []browsermgr.InstallResult{
			{Name: "chromium", Version: "1.0.0", Changed: true, ExecutablePath: "/tmp/chromium"},
		},
	}, nil
}

func (fakeBrowserManager) Update(context.Context) (browsermgr.SetupResult, error) {
	return fakeBrowserManager{}.Setup(context.Background())
}

func (fakeBrowserManager) Uninstall(context.Context, ...string) (browsermgr.UninstallResult, error) {
	return browsermgr.UninstallResult{
		Browsers: []browsermgr.InstallResult{
			{Name: "chromium", Version: "1.0.0", ExecutablePath: "/tmp/chromium", Changed: true},
		},
	}, nil
}

func (fakeBrowserManager) Status() (browsermgr.Status, error) {
	return browsermgr.Status{
		Browsers: []browsermgr.Installation{
			{Name: "chromium", Version: "1.0.0", Installed: true, ExecutablePath: "/tmp/chromium"},
		},
	}, nil
}

func (fakeBrowserManager) Resolve(name string) (browsermgr.Installation, error) {
	switch name {
	case "chromium":
		return browsermgr.Installation{Name: name, Version: "1.0.0", Installed: true, ExecutablePath: "/tmp/chromium"}, nil
	default:
		return browsermgr.Installation{}, browsermgr.ErrUnknownBrowser
	}
}

func (missingBrowserManager) Setup(context.Context) (browsermgr.SetupResult, error) {
	return browsermgr.SetupResult{}, nil
}

func (missingBrowserManager) Update(context.Context) (browsermgr.SetupResult, error) {
	return browsermgr.SetupResult{}, nil
}

func (missingBrowserManager) Uninstall(context.Context, ...string) (browsermgr.UninstallResult, error) {
	return browsermgr.UninstallResult{}, nil
}

func (missingBrowserManager) Status() (browsermgr.Status, error) {
	return browsermgr.Status{}, nil
}

func (missingBrowserManager) Resolve(string) (browsermgr.Installation, error) {
	return browsermgr.Installation{}, browsermgr.ErrBrowserNotInstalled
}
