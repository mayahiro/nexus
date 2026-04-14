package cli

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/rpc"
)

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

func TestTypeInputAndFill(t *testing.T) {
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

	stdout.Reset()
	if code := Run(context.Background(), []string{"fill", "3", "hello@example.com"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected fill exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "filled into 3" {
		t.Fatalf("unexpected fill output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"fill", "@e3", "hello@example.com", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected fill --json exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "filled into 3"`) {
		t.Fatalf("unexpected fill --json output: %s", stdout.String())
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
