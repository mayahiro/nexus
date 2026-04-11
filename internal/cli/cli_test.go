package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"net"
	"os"
	"path/filepath"
	"strings"
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
	if code := Run(context.Background(), []string{"wait"}, &stdout, &stdout); code == 0 {
		t.Fatalf("expected wait without args to fail\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `usage: nxctl wait selector`) {
		t.Fatalf("unexpected wait missing-args output: %s", stdout.String())
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
	if code := Run(context.Background(), []string{"eval", "[1, 2, 3]", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected eval --json exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "[\n  1,\n  2,\n  3\n]") {
		t.Fatalf("unexpected eval --json output: %s", stdout.String())
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
	if code := Run(context.Background(), []string{"upload", "4", uploadFile, "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected upload exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "uploaded `) {
		t.Fatalf("unexpected upload output: %s", stdout.String())
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

type fakeBrowserManager struct{}
type fakeLightpandaBackend struct{}

type evalRPCHandler struct{}
type clickRPCHandler struct{}
type mouseRPCHandler struct{}
type typeRPCHandler struct{}
type keysRPCHandler struct{}
type screenshotRPCHandler struct{}
type scrollRPCHandler struct{}
type backRPCHandler struct{}
type viewportRPCHandler struct{}
type waitRPCHandler struct{}
type getRPCHandler struct{}
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
	default:
		return api.ActSessionResponse{}, nil
	}
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
