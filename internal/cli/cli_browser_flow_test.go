package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

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
