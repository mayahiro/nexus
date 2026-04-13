package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/browsermgr"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/daemon"
	"github.com/mayahiro/nexus/internal/target/browser"
	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

func TestBrowserUninstall(t *testing.T) {
	original := newBrowserManager
	defer func() {
		newBrowserManager = original
	}()

	newBrowserManager = func(config.Paths) browserManager {
		return fakeBrowserManager{}
	}

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"browser", "uninstall", "--name", "chromium"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected browser uninstall exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "chromium") {
		t.Fatalf("unexpected browser uninstall output: %s", stdout.String())
	}
}

func TestDoctorStartsDaemon(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}

	original := startDaemonProcess
	defer func() {
		startDaemonProcess = original
	}()

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	startDaemonProcess = func(config.Paths) error {
		go func() {
			done <- daemon.Run(runCtx, paths, daemon.RunOptions{IdleTimeout: time.Second})
		}()
		return nil
	}

	var stdout bytes.Buffer
	code := Run(context.Background(), []string{"doctor"}, &stdout, &stdout)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d\n%s", code, stdout.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "daemon: started") {
		t.Fatalf("unexpected doctor output: %s", output)
	}
	if !strings.Contains(output, "daemon: stopped") {
		t.Fatalf("unexpected doctor output: %s", output)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop")
	}

	cancel()
}

func TestAutoStartedDaemonPersistsAcrossCommands(t *testing.T) {
	configureXDGTestEnv(t)

	restoreBackend := browser.SetBackendFactory(spec.BackendLightpanda, func() spec.Backend {
		return autoStartLightpandaBackend{}
	})
	defer restoreBackend()

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}

	originalStart := startDaemonProcess
	originalManager := newBrowserManager
	defer func() {
		startDaemonProcess = originalStart
		newBrowserManager = originalManager
	}()

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	startDaemonProcess = func(config.Paths) error {
		go func() {
			done <- daemon.Run(runCtx, paths, daemon.RunOptions{})
		}()
		return nil
	}
	newBrowserManager = func(config.Paths) browserManager {
		return fakeBrowserManager{}
	}

	var openOut bytes.Buffer
	if code := Run(context.Background(), []string{"open", "https://example.com", "--backend", "lightpanda", "--session", "auto"}, &openOut, &openOut); code != 0 {
		t.Fatalf("unexpected open exit code: %d\n%s", code, openOut.String())
	}
	if !strings.Contains(openOut.String(), "attached browser auto (lightpanda) /tmp/lightpanda") {
		t.Fatalf("unexpected open output: %s", openOut.String())
	}

	var sessionsOut bytes.Buffer
	if code := Run(context.Background(), []string{"sessions", "--json"}, &sessionsOut, &sessionsOut); code != 0 {
		t.Fatalf("unexpected sessions exit code: %d\n%s", code, sessionsOut.String())
	}
	if !strings.Contains(sessionsOut.String(), `"id": "auto"`) {
		t.Fatalf("unexpected sessions output: %s", sessionsOut.String())
	}

	var evalOut bytes.Buffer
	if code := Run(context.Background(), []string{"eval", "document.title", "--session", "auto", "--json"}, &evalOut, &evalOut); code != 0 {
		t.Fatalf("unexpected eval exit code: %d\n%s", code, evalOut.String())
	}
	if strings.TrimSpace(evalOut.String()) != `"Example Title"` {
		t.Fatalf("unexpected eval output: %s", evalOut.String())
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop")
	}
}

func TestAttachSessionsDetach(t *testing.T) {
	configureXDGTestEnv(t)
	restoreBackend := browser.SetBackendFactory(spec.BackendLightpanda, func() spec.Backend {
		return fakeLightpandaBackend{}
	})
	defer restoreBackend()

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- daemon.Run(ctx, paths, daemon.RunOptions{IdleTimeout: time.Second})
	}()

	waitForSocket(t, paths.Socket)

	var attachOut bytes.Buffer
	original := newBrowserManager
	defer func() {
		newBrowserManager = original
	}()
	newBrowserManager = func(config.Paths) browserManager {
		return fakeBrowserManager{}
	}

	if code := Run(context.Background(), []string{"attach", "browser", "--session", "web1", "--backend", "lightpanda", "--url", "https://example.com", "--viewport", "1440x900"}, &attachOut, &attachOut); code != 0 {
		t.Fatalf("unexpected attach exit code: %d\n%s", code, attachOut.String())
	}
	if !strings.Contains(attachOut.String(), "attached browser web1 (lightpanda) /tmp/lightpanda") {
		t.Fatalf("unexpected attach output: %s", attachOut.String())
	}

	var sessionsOut bytes.Buffer
	if code := Run(context.Background(), []string{"sessions", "--json"}, &sessionsOut, &sessionsOut); code != 0 {
		t.Fatalf("unexpected sessions exit code: %d\n%s", code, sessionsOut.String())
	}
	if !strings.Contains(sessionsOut.String(), "\"id\": \"web1\"") {
		t.Fatalf("unexpected sessions output: %s", sessionsOut.String())
	}
	if !strings.Contains(sessionsOut.String(), "\"backend\": \"lightpanda\"") {
		t.Fatalf("unexpected sessions output: %s", sessionsOut.String())
	}
	if !strings.Contains(sessionsOut.String(), "/tmp/lightpanda") {
		t.Fatalf("unexpected sessions output: %s", sessionsOut.String())
	}
	if !strings.Contains(sessionsOut.String(), "\"initial_url\": \"https://example.com\"") {
		t.Fatalf("unexpected sessions output: %s", sessionsOut.String())
	}
	if !strings.Contains(sessionsOut.String(), "\"viewport_width\": \"1440\"") {
		t.Fatalf("unexpected sessions output: %s", sessionsOut.String())
	}
	if !strings.Contains(sessionsOut.String(), "\"viewport_height\": \"900\"") {
		t.Fatalf("unexpected sessions output: %s", sessionsOut.String())
	}

	var detachOut bytes.Buffer
	if code := Run(context.Background(), []string{"detach", "--session", "web1"}, &detachOut, &detachOut); code != 0 {
		t.Fatalf("unexpected detach exit code: %d\n%s", code, detachOut.String())
	}
	if !strings.Contains(detachOut.String(), "detached web1") {
		t.Fatalf("unexpected detach output: %s", detachOut.String())
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop")
	}
}

func TestObserveJSON(t *testing.T) {
	configureXDGTestEnv(t)
	restoreBackend := browser.SetBackendFactory(spec.BackendLightpanda, func() spec.Backend {
		return fakeLightpandaBackend{}
	})
	defer restoreBackend()

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- daemon.Run(ctx, paths, daemon.RunOptions{IdleTimeout: time.Second})
	}()

	waitForSocket(t, paths.Socket)

	original := newBrowserManager
	defer func() {
		newBrowserManager = original
	}()
	newBrowserManager = func(config.Paths) browserManager {
		return fakeBrowserManager{}
	}

	var attachOut bytes.Buffer
	if code := Run(context.Background(), []string{"attach", "browser", "--session", "web1", "--backend", "lightpanda"}, &attachOut, &attachOut); code != 0 {
		t.Fatalf("unexpected attach exit code: %d\n%s", code, attachOut.String())
	}

	var observeOut bytes.Buffer
	if code := Run(context.Background(), []string{"observe", "--session", "web1", "--json"}, &observeOut, &observeOut); code != 0 {
		t.Fatalf("unexpected observe exit code: %d\n%s", code, observeOut.String())
	}

	if !strings.Contains(observeOut.String(), "\"session_id\": \"web1\"") {
		t.Fatalf("unexpected observe output: %s", observeOut.String())
	}
	if !strings.Contains(observeOut.String(), "\"target_type\": \"browser\"") {
		t.Fatalf("unexpected observe output: %s", observeOut.String())
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop")
	}
}

func TestOpenAndState(t *testing.T) {
	configureXDGTestEnv(t)
	restoreBackend := browser.SetBackendFactory(spec.BackendLightpanda, func() spec.Backend {
		return fakeLightpandaBackend{}
	})
	defer restoreBackend()

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- daemon.Run(ctx, paths, daemon.RunOptions{IdleTimeout: time.Second})
	}()

	waitForSocket(t, paths.Socket)

	original := newBrowserManager
	defer func() {
		newBrowserManager = original
	}()
	newBrowserManager = func(config.Paths) browserManager {
		return fakeBrowserManager{}
	}

	var openOut bytes.Buffer
	if code := Run(context.Background(), []string{"open", "https://example.com", "--backend", "lightpanda", "--viewport", "1280x720"}, &openOut, &openOut); code != 0 {
		t.Fatalf("unexpected open exit code: %d\n%s", code, openOut.String())
	}
	if !strings.Contains(openOut.String(), "attached browser default (lightpanda) /tmp/lightpanda") {
		t.Fatalf("unexpected open output: %s", openOut.String())
	}

	var sessionsOut bytes.Buffer
	if code := Run(context.Background(), []string{"sessions", "--json"}, &sessionsOut, &sessionsOut); code != 0 {
		t.Fatalf("unexpected sessions exit code: %d\n%s", code, sessionsOut.String())
	}
	if !strings.Contains(sessionsOut.String(), "\"viewport_width\": \"1280\"") {
		t.Fatalf("unexpected sessions output: %s", sessionsOut.String())
	}
	if !strings.Contains(sessionsOut.String(), "\"viewport_height\": \"720\"") {
		t.Fatalf("unexpected sessions output: %s", sessionsOut.String())
	}

	var stateOut bytes.Buffer
	if code := Run(context.Background(), []string{"state"}, &stateOut, &stateOut); code != 0 {
		t.Fatalf("unexpected state exit code: %d\n%s", code, stateOut.String())
	}
	if !strings.Contains(stateOut.String(), "URL:") {
		t.Fatalf("unexpected state output: %s", stateOut.String())
	}
	if !strings.Contains(stateOut.String(), "Title:") {
		t.Fatalf("unexpected state output: %s", stateOut.String())
	}
	if !strings.Contains(stateOut.String(), "[@e1] link \"Docs\"") {
		t.Fatalf("unexpected state output: %s", stateOut.String())
	}
	if !strings.Contains(stateOut.String(), `find: role link --name "Docs"`) {
		t.Fatalf("unexpected state output: %s", stateOut.String())
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop")
	}
}

func TestOpenFlagsFirst(t *testing.T) {
	configureXDGTestEnv(t)
	restoreBackend := browser.SetBackendFactory(spec.BackendLightpanda, func() spec.Backend {
		return fakeLightpandaBackend{}
	})
	defer restoreBackend()

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- daemon.Run(ctx, paths, daemon.RunOptions{IdleTimeout: time.Second})
	}()

	waitForSocket(t, paths.Socket)

	original := newBrowserManager
	defer func() {
		newBrowserManager = original
	}()
	newBrowserManager = func(config.Paths) browserManager {
		return fakeBrowserManager{}
	}

	var openOut bytes.Buffer
	args := []string{"open", "--backend", "lightpanda", "--session", "flags-first", "https://example.com"}
	if code := Run(context.Background(), args, &openOut, &openOut); code != 0 {
		t.Fatalf("unexpected open flags-first exit code: %d\n%s", code, openOut.String())
	}
	if !strings.Contains(openOut.String(), "attached browser flags-first (lightpanda) /tmp/lightpanda") {
		t.Fatalf("unexpected open flags-first output: %s", openOut.String())
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop")
	}
}

func TestCloseStopsDaemon(t *testing.T) {
	configureXDGTestEnv(t)
	restoreBackend := browser.SetBackendFactory(spec.BackendLightpanda, func() spec.Backend {
		return fakeLightpandaBackend{}
	})
	defer restoreBackend()

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- daemon.Run(ctx, paths, daemon.RunOptions{})
	}()

	waitForSocket(t, paths.Socket)

	original := newBrowserManager
	defer func() {
		newBrowserManager = original
	}()
	newBrowserManager = func(config.Paths) browserManager {
		return fakeBrowserManager{}
	}

	var attachOut bytes.Buffer
	if code := Run(context.Background(), []string{"open", "https://example.com", "--backend", "lightpanda"}, &attachOut, &attachOut); code != 0 {
		t.Fatalf("unexpected open exit code: %d\n%s", code, attachOut.String())
	}

	var closeOut bytes.Buffer
	if code := Run(context.Background(), []string{"close"}, &closeOut, &closeOut); code != 0 {
		t.Fatalf("unexpected close exit code: %d\n%s", code, closeOut.String())
	}
	if strings.TrimSpace(closeOut.String()) != "closed default" {
		t.Fatalf("unexpected close output: %s", closeOut.String())
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop")
	}
}

func TestCloseAllStopsDaemon(t *testing.T) {
	configureXDGTestEnv(t)
	restoreBackend := browser.SetBackendFactory(spec.BackendLightpanda, func() spec.Backend {
		return fakeLightpandaBackend{}
	})
	defer restoreBackend()

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- daemon.Run(ctx, paths, daemon.RunOptions{})
	}()

	waitForSocket(t, paths.Socket)

	original := newBrowserManager
	defer func() {
		newBrowserManager = original
	}()
	newBrowserManager = func(config.Paths) browserManager {
		return fakeBrowserManager{}
	}

	var out bytes.Buffer
	if code := Run(context.Background(), []string{"attach", "browser", "--session", "web1", "--backend", "lightpanda"}, &out, &out); code != 0 {
		t.Fatalf("unexpected attach exit code: %d\n%s", code, out.String())
	}

	out.Reset()
	if code := Run(context.Background(), []string{"attach", "browser", "--session", "web2", "--backend", "lightpanda"}, &out, &out); code != 0 {
		t.Fatalf("unexpected attach exit code: %d\n%s", code, out.String())
	}

	out.Reset()
	if code := Run(context.Background(), []string{"close", "--all"}, &out, &out); code != 0 {
		t.Fatalf("unexpected close --all exit code: %d\n%s", code, out.String())
	}
	if strings.TrimSpace(out.String()) != "closed all sessions" {
		t.Fatalf("unexpected close --all output: %s", out.String())
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop")
	}
}

func TestAttachBrowserRequiresSetupWhenManagedBrowserMissing(t *testing.T) {
	configureXDGTestEnv(t)

	original := newBrowserManager
	defer func() {
		newBrowserManager = original
	}()
	newBrowserManager = func(config.Paths) browserManager {
		return missingBrowserManager{}
	}

	var output bytes.Buffer
	code := Run(context.Background(), []string{"attach", "browser", "--session", "web1"}, &output, &output)
	if code == 0 {
		t.Fatalf("expected failure: %s", output.String())
	}
	if !strings.Contains(output.String(), "chromium is not installed. run `nxctl browser setup` first") {
		t.Fatalf("unexpected output: %s", output.String())
	}
}

func TestBrowserCommands(t *testing.T) {
	configureXDGTestEnv(t)

	original := newBrowserManager
	defer func() {
		newBrowserManager = original
	}()

	newBrowserManager = func(config.Paths) browserManager {
		return fakeBrowserManager{}
	}

	var setupOut bytes.Buffer
	if code := Run(context.Background(), []string{"browser", "setup"}, &setupOut, &setupOut); code != 0 {
		t.Fatalf("unexpected setup exit code: %d\n%s", code, setupOut.String())
	}
	if !strings.Contains(setupOut.String(), "chromium\t1.0.0\tupdated") {
		t.Fatalf("unexpected setup output: %s", setupOut.String())
	}

	var statusOut bytes.Buffer
	if code := Run(context.Background(), []string{"browser", "status"}, &statusOut, &statusOut); code != 0 {
		t.Fatalf("unexpected status exit code: %d\n%s", code, statusOut.String())
	}
	if !strings.Contains(statusOut.String(), "lightpanda\tv0.1.0\tinstalled") {
		t.Fatalf("unexpected status output: %s", statusOut.String())
	}
}

type fakeBrowserManager struct{}
type fakeLightpandaBackend struct{}
type autoStartLightpandaBackend struct{}
type missingBrowserManager struct{}

func (fakeLightpandaBackend) Name() spec.BackendName {
	return spec.BackendLightpanda
}

func (fakeLightpandaBackend) Capabilities() spec.Capabilities {
	return spec.Capabilities{Observe: true}
}

func (fakeLightpandaBackend) Attach(context.Context, spec.SessionConfig) error {
	return nil
}

func (fakeLightpandaBackend) Detach(context.Context) error {
	return nil
}

func (fakeLightpandaBackend) Observe(context.Context, api.ObserveOptions) (*api.Observation, error) {
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

func (fakeLightpandaBackend) Act(context.Context, api.Action) (*api.ActionResult, error) {
	return nil, nil
}

func (fakeLightpandaBackend) Screenshot(context.Context, string) error {
	return nil
}

func (fakeLightpandaBackend) Logs(context.Context, api.LogOptions) ([]api.LogEntry, error) {
	return nil, nil
}

func (autoStartLightpandaBackend) Name() spec.BackendName {
	return spec.BackendLightpanda
}

func (autoStartLightpandaBackend) Capabilities() spec.Capabilities {
	return spec.Capabilities{Observe: true, Act: true}
}

func (autoStartLightpandaBackend) Attach(context.Context, spec.SessionConfig) error {
	return nil
}

func (autoStartLightpandaBackend) Detach(context.Context) error {
	return nil
}

func (autoStartLightpandaBackend) Observe(context.Context, api.ObserveOptions) (*api.Observation, error) {
	return &api.Observation{
		URLOrScreen: "https://example.com",
		Title:       "Example Title",
		Text:        "Example text",
	}, nil
}

func (autoStartLightpandaBackend) Act(_ context.Context, action api.Action) (*api.ActionResult, error) {
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

func (autoStartLightpandaBackend) Screenshot(context.Context, string) error {
	return nil
}

func (autoStartLightpandaBackend) Logs(context.Context, api.LogOptions) ([]api.LogEntry, error) {
	return nil, nil
}

func (fakeBrowserManager) Setup(context.Context) (browsermgr.SetupResult, error) {
	return browsermgr.SetupResult{
		Browsers: []browsermgr.InstallResult{
			{Name: "chromium", Version: "1.0.0", Changed: true, ExecutablePath: "/tmp/chromium"},
			{Name: "lightpanda", Version: "v0.1.0", Changed: false, ExecutablePath: "/tmp/lightpanda"},
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
			{Name: "lightpanda", Version: "v0.1.0", Installed: true, ExecutablePath: "/tmp/lightpanda"},
		},
	}, nil
}

func (fakeBrowserManager) Resolve(name string) (browsermgr.Installation, error) {
	switch name {
	case "chromium":
		return browsermgr.Installation{Name: name, Version: "1.0.0", Installed: true, ExecutablePath: "/tmp/chromium"}, nil
	case "lightpanda":
		return browsermgr.Installation{Name: name, Version: "v0.1.0", Installed: true, ExecutablePath: "/tmp/lightpanda"}, nil
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
