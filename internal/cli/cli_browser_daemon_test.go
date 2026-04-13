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
