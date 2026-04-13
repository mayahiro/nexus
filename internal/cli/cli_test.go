package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/browsermgr"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/daemon"
	"github.com/mayahiro/nexus/internal/rpc"
	"github.com/mayahiro/nexus/internal/target/browser"
	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

func TestDoctor(t *testing.T) {
	configureXDGTestEnv(t)

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

	var stdout bytes.Buffer
	code := Run(context.Background(), []string{"doctor"}, &stdout, &stdout)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d\n%s", code, stdout.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "daemon: ok") {
		t.Fatalf("unexpected doctor output: %s", output)
	}
	if !strings.Contains(output, "protocol: ok") {
		t.Fatalf("unexpected doctor output: %s", output)
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

func TestHelp(t *testing.T) {
	var stdout bytes.Buffer

	if code := Run(context.Background(), []string{"help"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected help exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "usage: nxctl <command>") {
		t.Fatalf("unexpected help output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"help", "wait"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected help wait exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `usage: nxctl wait selector`) {
		t.Fatalf("unexpected help wait output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"wait", "--help"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected wait --help exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `wait url "value"`) {
		t.Fatalf("unexpected wait --help output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"help", "find"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected help find exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `usage: nxctl find role <role> click`) {
		t.Fatalf("unexpected help find output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `find testid "value" click|get`) {
		t.Fatalf("unexpected help find output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"help", "batch"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected help batch exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `usage: nxctl batch --cmd "open https://example.com"`) {
		t.Fatalf("unexpected help batch output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"help", "compare"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected help compare exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `usage: nxctl compare <old-url> <new-url>`) {
		t.Fatalf("unexpected help compare output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"wait"}, &stdout, &stdout); code == 0 {
		t.Fatalf("expected wait without args to fail\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `hint: nxctl wait selector ".ready"`) {
		t.Fatalf("unexpected wait missing-args output: %s", stdout.String())
	}
}

func TestSplitBatchCommand(t *testing.T) {
	args, err := splitBatchCommand(`find text "Sign In" --all`)
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"find", "text", "Sign In", "--all"}
	if len(args) != len(expected) {
		t.Fatalf("unexpected arg length: %#v", args)
	}
	for i := range expected {
		if args[i] != expected[i] {
			t.Fatalf("unexpected args: %#v", args)
		}
	}
}

func TestBatch(t *testing.T) {
	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"batch", "--cmd", "help wait", "--cmd", "help find"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected batch exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "==> help wait") {
		t.Fatalf("unexpected batch output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "==> help find") {
		t.Fatalf("unexpected batch output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"batch", "--cmd", "help wait", "--cmd", "unknown"}, &stdout, &stdout); code == 0 {
		t.Fatalf("expected batch failure\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "usage: nxctl <command>") {
		t.Fatalf("unexpected batch failure output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"batch", "--cmd", "help wait", "--cmd", "help find", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected batch json exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"command": "help wait"`) {
		t.Fatalf("unexpected batch json output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"exit_code": 0`) {
		t.Fatalf("unexpected batch json output: %s", stdout.String())
	}
}

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

func TestStateFilters(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, findRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"state", "--role", "button", "--limit", "1"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected state filter exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `[@e1] button "Submit"`) {
		t.Fatalf("unexpected state filter output: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), `Cancel`) || strings.Contains(stdout.String(), `Sign In`) {
		t.Fatalf("unexpected state filter output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"state", "--testid", "submit-primary", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected state json filter exit code: %d\n%s", code, stdout.String())
	}
	var observation api.Observation
	if err := json.Unmarshal(stdout.Bytes(), &observation); err != nil {
		t.Fatalf("unexpected state json filter output: %v\n%s", err, stdout.String())
	}
	if len(observation.Tree) != 1 || observation.Tree[0].Role != "button" {
		t.Fatalf("unexpected filtered tree: %+v", observation.Tree)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestCommandHints(t *testing.T) {
	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"open"}, &stdout, &stdout); code == 0 {
		t.Fatalf("expected open without url to fail\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "hint: nxctl open https://example.com --session work") {
		t.Fatalf("unexpected open hint output: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "usage: nxctl <command>") {
		t.Fatalf("unexpected global usage in open hint output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"click", "--bogus", "3"}, &stdout, &stdout); code == 0 {
		t.Fatalf("expected click parse failure\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "hint: run `nxctl help click` for details") {
		t.Fatalf("unexpected click parse hint output: %s", stdout.String())
	}
}

func TestEval(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, evalRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"eval", "document.title"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected eval exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "Example Title" {
		t.Fatalf("unexpected eval output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"eval", "document.title", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected eval string --json exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != `"Example Title"` {
		t.Fatalf("unexpected eval string --json output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"eval", "[1, 2, 3]", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected eval --json exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "[\n  1,\n  2,\n  3\n]") {
		t.Fatalf("unexpected eval --json output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"eval", "false"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected eval false exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "false" {
		t.Fatalf("unexpected eval false output: %q", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"eval", "0"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected eval zero exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "0" {
		t.Fatalf("unexpected eval zero output: %q", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"eval", `""`}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected eval empty-string exit code: %d\n%s", code, stdout.String())
	}
	if stdout.String() != "\n" {
		t.Fatalf("unexpected eval empty-string output: %q", stdout.String())
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestCompare(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, compareRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	args := []string{
		"compare",
		"--old-session", "old",
		"--new-session", "new",
		"--ignore-text-regex", `20\d\d-\d\d-\d\d`,
		"--json",
	}
	if code := Run(context.Background(), args, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected compare exit code: %d\n%s", code, stdout.String())
	}

	var report compareReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("unexpected compare json: %v\n%s", err, stdout.String())
	}

	if report.Summary.Same {
		t.Fatalf("expected differences, got same report: %s", stdout.String())
	}
	if report.Summary.TotalFindings != 6 {
		t.Fatalf("unexpected finding count: %+v", report.Summary)
	}
	if report.Summary.TitleChanged != 1 || report.Summary.TextChanged != 2 || report.Summary.MissingNodes != 1 || report.Summary.NewNodes != 1 || report.Summary.StateChanged != 1 {
		t.Fatalf("unexpected summary: %+v", report.Summary)
	}
	if report.Summary.Critical != 1 || report.Summary.Warning != 5 || report.Summary.Info != 0 {
		t.Fatalf("unexpected severity summary: %+v", report.Summary)
	}
	if report.Summary.PageTextChanged != 0 {
		t.Fatalf("unexpected page_text_changed summary: %+v", report.Summary)
	}
	if report.Old.SessionID != "old" || report.New.SessionID != "new" {
		t.Fatalf("unexpected report sessions: %+v", report)
	}
	if report.Findings[0].Severity == "" || report.Findings[0].Impact == "" {
		t.Fatalf("expected severity and impact in findings: %+v", report.Findings[0])
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestCompareURLs(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	originalManager := newBrowserManager
	originalSuffix := newCompareSessionSuffix
	defer func() {
		newBrowserManager = originalManager
		newCompareSessionSuffix = originalSuffix
	}()

	newBrowserManager = func(config.Paths) browserManager {
		return fakeBrowserManager{}
	}
	newCompareSessionSuffix = func() string {
		return "same"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	handler := &compareURLRPCHandler{
		observations: map[string]api.Observation{
			"https://old.example.test/dashboard": {
				SessionID:   "old",
				URLOrScreen: "https://old.example.test/dashboard",
				Title:       "Orders",
				Text:        "Orders stable",
			},
			"https://new.example.test/dashboard": {
				SessionID:   "new",
				URLOrScreen: "https://new.example.test/dashboard",
				Title:       "Orders v2",
				Text:        "Orders stable",
			},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, handler, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	args := []string{
		"compare",
		"https://old.example.test/dashboard",
		"https://new.example.test/dashboard",
		"--json",
	}
	if code := Run(context.Background(), args, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected compare url exit code: %d\n%s", code, stdout.String())
	}

	var report compareReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("unexpected compare url json: %v\n%s", err, stdout.String())
	}
	if report.Old.URL != "https://old.example.test/dashboard" || report.New.URL != "https://new.example.test/dashboard" {
		t.Fatalf("unexpected compare url report: %+v", report)
	}
	if report.Summary.TitleChanged != 1 {
		t.Fatalf("unexpected compare url summary: %+v", report.Summary)
	}
	if report.Summary.Warning != 1 {
		t.Fatalf("unexpected compare url severity summary: %+v", report.Summary)
	}
	if len(handler.attachIDs) != 2 {
		t.Fatalf("unexpected attach count: %#v", handler.attachIDs)
	}
	if handler.attachIDs[0] == handler.attachIDs[1] {
		t.Fatalf("compare url used duplicate temp session ids: %#v", handler.attachIDs)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestCompareIgnoreAndMaskSelectors(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, compareRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	args := []string{
		"compare",
		"--old-session", "old",
		"--new-session", "new",
		"--ignore-selector", "@e3",
		"--mask-selector", "@e2",
		"--ignore-text-regex", `20\d\d-\d\d-\d\d`,
		"--json",
	}
	if code := Run(context.Background(), args, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected compare selector exit code: %d\n%s", code, stdout.String())
	}

	var report compareReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("unexpected compare selector json: %v\n%s", err, stdout.String())
	}

	if report.Summary.TotalFindings != 3 {
		t.Fatalf("unexpected compare selector findings: %+v", report.Summary)
	}
	if report.Summary.MissingNodes != 0 || report.Summary.NewNodes != 0 {
		t.Fatalf("unexpected compare selector node summary: %+v", report.Summary)
	}
	if report.Summary.TextChanged != 1 || report.Summary.StateChanged != 1 || report.Summary.TitleChanged != 1 {
		t.Fatalf("unexpected compare selector summary: %+v", report.Summary)
	}
	for _, finding := range report.Findings {
		if finding.Field == "value" {
			t.Fatalf("masked value should not appear in findings: %+v", finding)
		}
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestClick(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, clickRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"click", "3"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected click exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "clicked 3" {
		t.Fatalf("unexpected click output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"click", "120", "240"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected coordinate click exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "clicked 120 240" {
		t.Fatalf("unexpected coordinate click output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"click", "@e3"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected click ref exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "clicked @e3" {
		t.Fatalf("unexpected click ref output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"click", "3", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected click --json exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "clicked 3"`) {
		t.Fatalf("unexpected click --json output: %s", stdout.String())
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestHoverDblclickRightclick(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, mouseRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"hover", "3"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected hover exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "hovered 3" {
		t.Fatalf("unexpected hover output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"dblclick", "3"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected dblclick exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "double-clicked 3" {
		t.Fatalf("unexpected dblclick output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"rightclick", "3", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected rightclick --json exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "right-clicked 3"`) {
		t.Fatalf("unexpected rightclick --json output: %s", stdout.String())
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestTypeAndInput(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, typeRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"type", "hello"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected type exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "typed" {
		t.Fatalf("unexpected type output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"input", "3", "hello"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected input exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "typed into 3" {
		t.Fatalf("unexpected input output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"input", "@e3", "hello"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected input ref exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "typed into 3" {
		t.Fatalf("unexpected input ref output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"input", "3", "hello", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected input --json exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "typed into 3"`) {
		t.Fatalf("unexpected input --json output: %s", stdout.String())
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestKeys(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, keysRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"keys", "Enter"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected keys exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "sent keys Enter" {
		t.Fatalf("unexpected keys output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"keys", "Meta+L", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected keys --json exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "sent keys Meta+L"`) {
		t.Fatalf("unexpected keys --json output: %s", stdout.String())
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestScreenshot(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, screenshotRPCHandler{}, rpc.ServeOptions{})
	}()

	tempDir := t.TempDir()
	t.Chdir(tempDir)

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"screenshot"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected screenshot exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "saved screenshot screenshot.png" {
		t.Fatalf("unexpected screenshot output: %s", stdout.String())
	}

	data, err := os.ReadFile(filepath.Join(tempDir, "screenshot.png"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "pngdata" {
		t.Fatalf("unexpected screenshot data: %q", string(data))
	}

	stdout.Reset()
	customPath := filepath.Join(tempDir, "full.png")
	if code := Run(context.Background(), []string{"screenshot", customPath, "--full"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected full screenshot exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "saved screenshot "+customPath {
		t.Fatalf("unexpected full screenshot output: %s", stdout.String())
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestScreenshotAnnotate(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, annotateScreenshotRPCHandler{}, rpc.ServeOptions{})
	}()

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "annotated.png")

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"screenshot", outputPath, "--annotate"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected screenshot --annotate exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "saved screenshot "+outputPath {
		t.Fatalf("unexpected screenshot --annotate output: %s", stdout.String())
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("expected annotated png: %v", err)
	}

	rgba := image.NewRGBA(img.Bounds())
	drawBounds := img.Bounds()
	for y := drawBounds.Min.Y; y < drawBounds.Max.Y; y++ {
		for x := drawBounds.Min.X; x < drawBounds.Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}

	if !hasNonWhitePixel(rgba) {
		t.Fatalf("expected annotation pixels in output")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestScroll(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, scrollRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"scroll", "down"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected scroll exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "scrolled down" {
		t.Fatalf("unexpected scroll output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"scroll", "up", "--amount", "500", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected scroll --json exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "scrolled up"`) {
		t.Fatalf("unexpected scroll --json output: %s", stdout.String())
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestBack(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, backRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"back"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected back exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "went back" {
		t.Fatalf("unexpected back output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"back", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected back --json exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "went back"`) {
		t.Fatalf("unexpected back --json output: %s", stdout.String())
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestViewport(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, viewportRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"viewport", "1280x720"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected viewport exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "set viewport 1280x720" {
		t.Fatalf("unexpected viewport output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"viewport", "1440x900", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected viewport --json exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "set viewport 1440x900"`) {
		t.Fatalf("unexpected viewport --json output: %s", stdout.String())
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestWait(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, waitRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"wait", "selector", ".ready"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected wait selector exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "waited for selector" {
		t.Fatalf("unexpected wait selector output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"wait", "text", "Done", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected wait text exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "waited for text"`) {
		t.Fatalf("unexpected wait text output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"wait", "url", "/done"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected wait url exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "waited for url" {
		t.Fatalf("unexpected wait url output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"wait", "navigation"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected wait navigation exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "waited for navigation" {
		t.Fatalf("unexpected wait navigation output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"wait", "function", "window.appReady === true", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected wait function exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "waited for function"`) {
		t.Fatalf("unexpected wait function output: %s", stdout.String())
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestGet(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, getRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"get", "title"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected get title exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "Example Title" {
		t.Fatalf("unexpected get title output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"get", "attributes", "@e3", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected get attributes ref exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"href": "/docs"`) {
		t.Fatalf("unexpected get attributes ref output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"get", "attributes", "3", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected get attributes exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"href": "/docs"`) {
		t.Fatalf("unexpected get attributes output: %s", stdout.String())
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestFind(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, findRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"find", "role", "button", "click", "--name", "Submit"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected find role exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "clicked @e1" {
		t.Fatalf("unexpected find role output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"find", "text", "Sign In", "click", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected find text exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"ref": "@e2"`) {
		t.Fatalf("unexpected find text output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "clicked 2"`) {
		t.Fatalf("unexpected find text output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"find", "role", "button", "--all"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected find --all exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `[@e1] button "Submit"`) {
		t.Fatalf("unexpected find --all output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `[@e4] button "Cancel"`) {
		t.Fatalf("unexpected find --all output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"find", "testid", "submit-primary", "--all", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected find testid --all exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"kind": "testid"`) {
		t.Fatalf("unexpected find testid --all output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"command": "testid \"submit-primary\""`) {
		t.Fatalf("unexpected find testid --all output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"find", "role", "link", "get", "attributes", "--name", "Sign In", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected find get exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"ref": "@e2"`) {
		t.Fatalf("unexpected find get output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"href": "/signin"`) {
		t.Fatalf("unexpected find get output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"find", "testid", "submit-primary", "click"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected find testid exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "clicked @e1" {
		t.Fatalf("unexpected find testid output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"find", "href", "/signin", "get", "attributes", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected find href exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"ref": "@e2"`) {
		t.Fatalf("unexpected find href output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"href": "/signin"`) {
		t.Fatalf("unexpected find href output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"find", "label", "Email", "input", "hello@example.com"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected find label exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "typed into @e3" {
		t.Fatalf("unexpected find label output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"find", "role", "button", "click"}, &stdout, &stdout); code == 0 {
		t.Fatalf("expected ambiguous find role to fail\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "multiple matching nodes found") {
		t.Fatalf("unexpected ambiguous find output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "@e1 button") || !strings.Contains(stdout.String(), "@e4 button") {
		t.Fatalf("unexpected ambiguous find output: %s", stdout.String())
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestSelectAndUpload(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	uploadFile := filepath.Join(t.TempDir(), "upload.txt")
	if err := os.WriteFile(uploadFile, []byte("upload"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, selectUploadRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"select", "3", "two"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected select exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "selected two on 3" {
		t.Fatalf("unexpected select output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"select", "@e3", "two"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected select ref exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "selected two on @e3" {
		t.Fatalf("unexpected select ref output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"upload", "4", uploadFile, "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected upload exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "uploaded `) {
		t.Fatalf("unexpected upload output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"upload", "@e4", uploadFile}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected upload ref exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(strings.TrimSpace(stdout.String()), "uploaded "+uploadFile+" to @e4") {
		t.Fatalf("unexpected upload ref output: %s", stdout.String())
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
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

func configureXDGTestEnv(t *testing.T) {
	t.Helper()

	root, err := os.MkdirTemp("/tmp", "nexus-cli-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.RemoveAll(root)
	})

	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(root, "run"))
}

func waitForSocket(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("socket not ready: %s", path)
}

func hasNonWhitePixel(img *image.RGBA) bool {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if r != 0xffff || g != 0xffff || b != 0xffff || a != 0xffff {
				return true
			}
		}
	}
	return false
}

func testPNGBase64() string {
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.White)
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

type fakeBrowserManager struct{}
type fakeLightpandaBackend struct{}
type autoStartLightpandaBackend struct{}

type evalRPCHandler struct{}
type compareRPCHandler struct{}
type compareURLRPCHandler struct {
	mu           sync.Mutex
	attachIDs    []string
	sessionURLs  map[string]string
	observations map[string]api.Observation
	observeCount map[string]int
}
type clickRPCHandler struct{}
type mouseRPCHandler struct{}
type typeRPCHandler struct{}
type keysRPCHandler struct{}
type screenshotRPCHandler struct{}
type annotateScreenshotRPCHandler struct{}
type scrollRPCHandler struct{}
type backRPCHandler struct{}
type viewportRPCHandler struct{}
type waitRPCHandler struct{}
type getRPCHandler struct{}
type findRPCHandler struct{}
type selectUploadRPCHandler struct{}

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

func (evalRPCHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, nil
}

func (evalRPCHandler) AttachSession(context.Context, api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	return api.AttachSessionResponse{}, nil
}

func (evalRPCHandler) ListSessions(context.Context, api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{}, nil
}

func (evalRPCHandler) DetachSession(context.Context, api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{}, nil
}

func (evalRPCHandler) StopDaemon(context.Context, api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (evalRPCHandler) ObserveSession(context.Context, api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	return api.ObserveSessionResponse{}, nil
}

func (evalRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	switch req.Action.Text {
	case "document.title":
		return api.ActSessionResponse{
			Result: api.ActionResult{
				OK:      true,
				Changed: false,
				Value:   "Example Title",
			},
		}, nil
	case "[1, 2, 3]":
		return api.ActSessionResponse{
			Result: api.ActionResult{
				OK:      true,
				Changed: false,
				Value:   []interface{}{1, 2, 3},
			},
		}, nil
	case "false":
		return api.ActSessionResponse{
			Result: api.ActionResult{
				OK:      true,
				Changed: false,
				Value:   false,
			},
		}, nil
	case "0":
		return api.ActSessionResponse{
			Result: api.ActionResult{
				OK:      true,
				Changed: false,
				Value:   0,
			},
		}, nil
	case `""`:
		return api.ActSessionResponse{
			Result: api.ActionResult{
				OK:      true,
				Changed: false,
				Value:   "",
			},
		}, nil
	default:
		return api.ActSessionResponse{}, nil
	}
}

func (compareRPCHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, nil
}

func (compareRPCHandler) AttachSession(context.Context, api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	return api.AttachSessionResponse{}, nil
}

func (compareRPCHandler) ListSessions(context.Context, api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{}, nil
}

func (compareRPCHandler) DetachSession(context.Context, api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{}, nil
}

func (compareRPCHandler) StopDaemon(context.Context, api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (compareRPCHandler) ObserveSession(_ context.Context, req api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	switch req.SessionID {
	case "old":
		return api.ObserveSessionResponse{
			Observation: api.Observation{
				SessionID:   "old",
				URLOrScreen: "https://old.example.test/dashboard",
				Title:       "Orders",
				Text:        "Orders 2026-04-13",
				Tree: []api.Node{
					{ID: 1, Ref: "@e1", Fingerprint: "cta-save", Role: "button", Name: "Save", Visible: true, Enabled: true, Invokable: true},
					{ID: 2, Ref: "@e2", Fingerprint: "email", Role: "textbox", Name: "Email", Value: "old@example.com", Visible: true, Enabled: true, Editable: true},
					{ID: 3, Ref: "@e3", Fingerprint: "legacy-link", Role: "link", Text: "Legacy", Visible: true, Enabled: true, Invokable: true, Attrs: map[string]string{"href": "/legacy"}},
					{ID: 4, Ref: "@e4", Fingerprint: "status", Role: "status", Text: "Ready 2026-04-13", Visible: true, Enabled: true},
				},
			},
		}, nil
	case "new":
		return api.ObserveSessionResponse{
			Observation: api.Observation{
				SessionID:   "new",
				URLOrScreen: "https://new.example.test/dashboard",
				Title:       "Orders v2",
				Text:        "Orders 2026-04-14",
				Tree: []api.Node{
					{ID: 1, Ref: "@e1", Fingerprint: "cta-save", Role: "button", Name: "Submit", Visible: true, Enabled: true, Invokable: true},
					{ID: 2, Ref: "@e2", Fingerprint: "email", Role: "textbox", Name: "Email", Value: "new@example.com", Visible: true, Enabled: false, Editable: true},
					{ID: 3, Ref: "@e3", Fingerprint: "next-link", Role: "link", Text: "Next", Visible: true, Enabled: true, Invokable: true, Attrs: map[string]string{"href": "/next"}},
					{ID: 4, Ref: "@e4", Fingerprint: "status", Role: "status", Text: "Ready 2026-04-14", Visible: true, Enabled: true},
				},
			},
		}, nil
	default:
		return api.ObserveSessionResponse{}, nil
	}
}

func (compareRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if req.Action.Kind != "wait" {
		return api.ActSessionResponse{}, nil
	}
	return api.ActSessionResponse{
		Result: api.ActionResult{
			OK:      true,
			Changed: false,
			Message: "waited",
		},
	}, nil
}

func (h *compareURLRPCHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, nil
}

func (h *compareURLRPCHandler) AttachSession(_ context.Context, req api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, existing := range h.attachIDs {
		if existing == req.SessionID {
			return api.AttachSessionResponse{}, errors.New("duplicate session id")
		}
	}

	h.attachIDs = append(h.attachIDs, req.SessionID)
	if h.sessionURLs == nil {
		h.sessionURLs = map[string]string{}
	}
	h.sessionURLs[req.SessionID] = req.Options["initial_url"]

	return api.AttachSessionResponse{
		Session: api.Session{
			ID:         req.SessionID,
			TargetType: req.TargetType,
			TargetRef:  req.TargetRef,
			Backend:    req.Backend,
			Options:    req.Options,
		},
	}, nil
}

func (h *compareURLRPCHandler) ListSessions(context.Context, api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{}, nil
}

func (h *compareURLRPCHandler) DetachSession(context.Context, api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{}, nil
}

func (h *compareURLRPCHandler) StopDaemon(context.Context, api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (h *compareURLRPCHandler) ObserveSession(_ context.Context, req api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	url := h.sessionURLs[req.SessionID]
	observation, ok := h.observations[url]
	if !ok {
		return api.ObserveSessionResponse{}, errors.New("unknown session")
	}
	if h.observeCount == nil {
		h.observeCount = map[string]int{}
	}
	h.observeCount[req.SessionID]++
	if h.observeCount[req.SessionID] == 1 {
		return api.ObserveSessionResponse{
			Observation: api.Observation{
				SessionID:   req.SessionID,
				URLOrScreen: "about:blank",
			},
		}, nil
	}
	observation.SessionID = req.SessionID
	return api.ObserveSessionResponse{Observation: observation}, nil
}

func (h *compareURLRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if req.Action.Kind != "wait" {
		return api.ActSessionResponse{}, nil
	}
	return api.ActSessionResponse{
		Result: api.ActionResult{
			OK:      true,
			Changed: false,
		},
	}, nil
}

func (clickRPCHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, nil
}

func (clickRPCHandler) AttachSession(context.Context, api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	return api.AttachSessionResponse{}, nil
}

func (clickRPCHandler) ListSessions(context.Context, api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{}, nil
}

func (clickRPCHandler) DetachSession(context.Context, api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{}, nil
}

func (clickRPCHandler) StopDaemon(context.Context, api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (clickRPCHandler) ObserveSession(context.Context, api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	return api.ObserveSessionResponse{}, nil
}

func (clickRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if req.Action.Kind != "invoke" {
		return api.ActSessionResponse{}, nil
	}
	if req.Action.NodeID != nil {
		return api.ActSessionResponse{
			Result: api.ActionResult{
				OK:      true,
				Changed: true,
				Message: "clicked 3",
				Value: map[string]interface{}{
					"id": float64(*req.Action.NodeID),
				},
			},
		}, nil
	}
	if req.Action.Args["x"] == "120" && req.Action.Args["y"] == "240" {
		return api.ActSessionResponse{
			Result: api.ActionResult{
				OK:      true,
				Changed: true,
				Message: "clicked 120 240",
				Value: map[string]interface{}{
					"x": float64(120),
					"y": float64(240),
				},
			},
		}, nil
	}
	return api.ActSessionResponse{}, nil
}

func (mouseRPCHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, nil
}

func (mouseRPCHandler) AttachSession(context.Context, api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	return api.AttachSessionResponse{}, nil
}

func (mouseRPCHandler) ListSessions(context.Context, api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{}, nil
}

func (mouseRPCHandler) DetachSession(context.Context, api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{}, nil
}

func (mouseRPCHandler) StopDaemon(context.Context, api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (mouseRPCHandler) ObserveSession(context.Context, api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	return api.ObserveSessionResponse{}, nil
}

func (mouseRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	switch req.Action.Kind {
	case "hover":
		return api.ActSessionResponse{
			Result: api.ActionResult{OK: true, Changed: true, Message: "hovered 3"},
		}, nil
	case "dblclick":
		return api.ActSessionResponse{
			Result: api.ActionResult{OK: true, Changed: true, Message: "double-clicked 3"},
		}, nil
	case "rightclick":
		return api.ActSessionResponse{
			Result: api.ActionResult{OK: true, Changed: true, Message: "right-clicked 3"},
		}, nil
	default:
		return api.ActSessionResponse{}, nil
	}
}

func (typeRPCHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, nil
}

func (typeRPCHandler) AttachSession(context.Context, api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	return api.AttachSessionResponse{}, nil
}

func (typeRPCHandler) ListSessions(context.Context, api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{}, nil
}

func (typeRPCHandler) DetachSession(context.Context, api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{}, nil
}

func (typeRPCHandler) StopDaemon(context.Context, api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (typeRPCHandler) ObserveSession(context.Context, api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	return api.ObserveSessionResponse{}, nil
}

func (typeRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if req.Action.Kind != "type" || req.Action.Text == "" {
		return api.ActSessionResponse{}, nil
	}

	message := "typed"
	if req.Action.NodeID != nil {
		message = "typed into 3"
	}

	return api.ActSessionResponse{
		Result: api.ActionResult{
			OK:      true,
			Changed: true,
			Message: message,
			Value: map[string]interface{}{
				"text": req.Action.Text,
			},
		},
	}, nil
}

func (keysRPCHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, nil
}

func (keysRPCHandler) AttachSession(context.Context, api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	return api.AttachSessionResponse{}, nil
}

func (keysRPCHandler) ListSessions(context.Context, api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{}, nil
}

func (keysRPCHandler) DetachSession(context.Context, api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{}, nil
}

func (keysRPCHandler) StopDaemon(context.Context, api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (keysRPCHandler) ObserveSession(context.Context, api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	return api.ObserveSessionResponse{}, nil
}

func (keysRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if req.Action.Kind != "key" || len(req.Action.Keys) != 1 {
		return api.ActSessionResponse{}, nil
	}

	return api.ActSessionResponse{
		Result: api.ActionResult{
			OK:      true,
			Changed: true,
			Message: "sent keys " + req.Action.Keys[0],
		},
	}, nil
}

func (screenshotRPCHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, nil
}

func (screenshotRPCHandler) AttachSession(context.Context, api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	return api.AttachSessionResponse{}, nil
}

func (screenshotRPCHandler) ListSessions(context.Context, api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{}, nil
}

func (screenshotRPCHandler) DetachSession(context.Context, api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{}, nil
}

func (screenshotRPCHandler) StopDaemon(context.Context, api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (screenshotRPCHandler) ObserveSession(_ context.Context, req api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	if !req.Options.WithScreenshot {
		return api.ObserveSessionResponse{}, nil
	}
	return api.ObserveSessionResponse{
		Observation: api.Observation{
			Screenshot: base64.StdEncoding.EncodeToString([]byte("pngdata")),
		},
	}, nil
}

func (screenshotRPCHandler) ActSession(context.Context, api.ActSessionRequest) (api.ActSessionResponse, error) {
	return api.ActSessionResponse{}, nil
}

func (annotateScreenshotRPCHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, nil
}

func (annotateScreenshotRPCHandler) AttachSession(context.Context, api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	return api.AttachSessionResponse{}, nil
}

func (annotateScreenshotRPCHandler) ListSessions(context.Context, api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{}, nil
}

func (annotateScreenshotRPCHandler) DetachSession(context.Context, api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{}, nil
}

func (annotateScreenshotRPCHandler) StopDaemon(context.Context, api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (annotateScreenshotRPCHandler) ObserveSession(_ context.Context, req api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	if !req.Options.WithScreenshot {
		return api.ObserveSessionResponse{}, nil
	}
	return api.ObserveSessionResponse{
		Observation: api.Observation{
			Screenshot: testPNGBase64(),
			Tree: []api.Node{
				{
					ID:      1,
					Ref:     "@e1",
					Role:    "button",
					Name:    "Submit",
					Visible: true,
					Enabled: true,
					Bounds:  api.Rect{X: 4, Y: 6, W: 18, H: 12},
				},
			},
		},
	}, nil
}

func (annotateScreenshotRPCHandler) ActSession(context.Context, api.ActSessionRequest) (api.ActSessionResponse, error) {
	return api.ActSessionResponse{}, nil
}

func (scrollRPCHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, nil
}

func (scrollRPCHandler) AttachSession(context.Context, api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	return api.AttachSessionResponse{}, nil
}

func (scrollRPCHandler) ListSessions(context.Context, api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{}, nil
}

func (scrollRPCHandler) DetachSession(context.Context, api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{}, nil
}

func (scrollRPCHandler) StopDaemon(context.Context, api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (scrollRPCHandler) ObserveSession(context.Context, api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	return api.ObserveSessionResponse{}, nil
}

func (scrollRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if req.Action.Kind != "scroll" || (req.Action.Dir != "up" && req.Action.Dir != "down") {
		return api.ActSessionResponse{}, nil
	}

	return api.ActSessionResponse{
		Result: api.ActionResult{
			OK:      true,
			Changed: true,
			Message: "scrolled " + req.Action.Dir,
			Value: map[string]interface{}{
				"dir": req.Action.Dir,
			},
		},
	}, nil
}

func (backRPCHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, nil
}

func (backRPCHandler) AttachSession(context.Context, api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	return api.AttachSessionResponse{}, nil
}

func (backRPCHandler) ListSessions(context.Context, api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{}, nil
}

func (backRPCHandler) DetachSession(context.Context, api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{}, nil
}

func (backRPCHandler) StopDaemon(context.Context, api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (backRPCHandler) ObserveSession(context.Context, api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	return api.ObserveSessionResponse{}, nil
}

func (backRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if req.Action.Kind != "back" {
		return api.ActSessionResponse{}, nil
	}

	return api.ActSessionResponse{
		Result: api.ActionResult{
			OK:      true,
			Changed: true,
			Message: "went back",
		},
	}, nil
}

func (viewportRPCHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, nil
}

func (viewportRPCHandler) AttachSession(context.Context, api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	return api.AttachSessionResponse{}, nil
}

func (viewportRPCHandler) ListSessions(context.Context, api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{}, nil
}

func (viewportRPCHandler) DetachSession(context.Context, api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{}, nil
}

func (viewportRPCHandler) StopDaemon(context.Context, api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (viewportRPCHandler) ObserveSession(context.Context, api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	return api.ObserveSessionResponse{}, nil
}

func (viewportRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if req.Action.Kind != "viewport" {
		return api.ActSessionResponse{}, nil
	}
	if req.Action.Args["width"] == "" || req.Action.Args["height"] == "" {
		return api.ActSessionResponse{}, nil
	}

	return api.ActSessionResponse{
		Result: api.ActionResult{
			OK:      true,
			Changed: true,
			Message: "set viewport " + req.Action.Args["width"] + "x" + req.Action.Args["height"],
			Value: map[string]interface{}{
				"width":  req.Action.Args["width"],
				"height": req.Action.Args["height"],
			},
		},
	}, nil
}

func (waitRPCHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, nil
}

func (waitRPCHandler) AttachSession(context.Context, api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	return api.AttachSessionResponse{}, nil
}

func (waitRPCHandler) ListSessions(context.Context, api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{}, nil
}

func (waitRPCHandler) DetachSession(context.Context, api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{}, nil
}

func (waitRPCHandler) StopDaemon(context.Context, api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (waitRPCHandler) ObserveSession(context.Context, api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	return api.ObserveSessionResponse{}, nil
}

func (waitRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if req.Action.Kind != "wait" || req.Action.Args["target"] == "" {
		return api.ActSessionResponse{}, nil
	}

	return api.ActSessionResponse{
		Result: api.ActionResult{
			OK:      true,
			Changed: false,
			Message: "waited for " + req.Action.Args["target"],
		},
	}, nil
}

func (getRPCHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, nil
}

func (getRPCHandler) AttachSession(context.Context, api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	return api.AttachSessionResponse{}, nil
}

func (getRPCHandler) ListSessions(context.Context, api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{}, nil
}

func (getRPCHandler) DetachSession(context.Context, api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{}, nil
}

func (getRPCHandler) StopDaemon(context.Context, api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (getRPCHandler) ObserveSession(context.Context, api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	return api.ObserveSessionResponse{}, nil
}

func (getRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if req.Action.Kind != "get" {
		return api.ActSessionResponse{}, nil
	}

	switch req.Action.Args["target"] {
	case "title":
		return api.ActSessionResponse{
			Result: api.ActionResult{OK: true, Value: "Example Title"},
		}, nil
	case "attributes":
		return api.ActSessionResponse{
			Result: api.ActionResult{
				OK: true,
				Value: map[string]interface{}{
					"href": "/docs",
				},
			},
		}, nil
	default:
		return api.ActSessionResponse{}, nil
	}
}

func (findRPCHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, nil
}

func (findRPCHandler) AttachSession(context.Context, api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	return api.AttachSessionResponse{}, nil
}

func (findRPCHandler) ListSessions(context.Context, api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{}, nil
}

func (findRPCHandler) DetachSession(context.Context, api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{}, nil
}

func (findRPCHandler) StopDaemon(context.Context, api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (findRPCHandler) ObserveSession(context.Context, api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	return api.ObserveSessionResponse{
		Observation: api.Observation{
			Tree: []api.Node{
				{
					ID:      1,
					Ref:     "@e1",
					Role:    "button",
					Name:    "Submit",
					Visible: true,
					Enabled: true,
					Attrs:   map[string]string{"data-testid": "submit-primary"},
					LocatorHints: []api.LocatorHint{
						{Kind: "role", Value: "button", Name: "Submit", Command: `role button --name "Submit"`},
						{Kind: "text", Value: "Submit", Command: `text "Submit"`},
						{Kind: "testid", Value: "submit-primary", Command: `testid "submit-primary"`},
					},
				},
				{
					ID:      2,
					Ref:     "@e2",
					Role:    "link",
					Text:    "Sign In",
					Visible: true,
					Enabled: true,
					Attrs:   map[string]string{"href": "/signin"},
					LocatorHints: []api.LocatorHint{
						{Kind: "role", Value: "link", Name: "Sign In", Command: `role link --name "Sign In"`},
						{Kind: "text", Value: "Sign In", Command: `text "Sign In"`},
						{Kind: "href", Value: "/signin", Command: `href "/signin"`},
					},
				},
				{
					ID:       3,
					Ref:      "@e3",
					Role:     "textbox",
					Name:     "Email",
					Visible:  true,
					Enabled:  true,
					Editable: true,
					LocatorHints: []api.LocatorHint{
						{Kind: "role", Value: "textbox", Name: "Email", Command: `role textbox --name "Email"`},
						{Kind: "label", Value: "Email", Command: `label "Email"`},
					},
				},
				{ID: 4, Ref: "@e4", Role: "button", Name: "Cancel", Visible: true, Enabled: true},
			},
		},
	}, nil
}

func (findRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	switch req.Action.Kind {
	case "invoke":
		return api.ActSessionResponse{
			Result: api.ActionResult{
				OK:      true,
				Changed: true,
				Message: "clicked " + strconv.Itoa(*req.Action.NodeID),
			},
		}, nil
	case "type":
		return api.ActSessionResponse{
			Result: api.ActionResult{
				OK:      true,
				Changed: true,
				Message: "typed into " + strconv.Itoa(*req.Action.NodeID),
			},
		}, nil
	case "get":
		return api.ActSessionResponse{
			Result: api.ActionResult{
				OK: true,
				Value: map[string]interface{}{
					"href": "/signin",
				},
			},
		}, nil
	default:
		return api.ActSessionResponse{}, nil
	}
}

func (selectUploadRPCHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, nil
}

func (selectUploadRPCHandler) AttachSession(context.Context, api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	return api.AttachSessionResponse{}, nil
}

func (selectUploadRPCHandler) ListSessions(context.Context, api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{}, nil
}

func (selectUploadRPCHandler) DetachSession(context.Context, api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{}, nil
}

func (selectUploadRPCHandler) StopDaemon(context.Context, api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (selectUploadRPCHandler) ObserveSession(context.Context, api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	return api.ObserveSessionResponse{}, nil
}

func (selectUploadRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	switch req.Action.Kind {
	case "select":
		return api.ActSessionResponse{
			Result: api.ActionResult{
				OK:      true,
				Changed: true,
				Message: "selected " + req.Action.Text + " on 3",
			},
		}, nil
	case "upload":
		return api.ActSessionResponse{
			Result: api.ActionResult{
				OK:      true,
				Changed: true,
				Message: "uploaded " + req.Action.Text + " to 4",
			},
		}, nil
	default:
		return api.ActSessionResponse{}, nil
	}
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

type missingBrowserManager struct{}

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
