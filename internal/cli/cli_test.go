package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/daemon"
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
	if !strings.Contains(stdout.String(), `find role <role> fill "text"`) {
		t.Fatalf("unexpected help find output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `find testid "value" click|fill|get`) {
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
	if !strings.Contains(stdout.String(), `nxctl compare --manifest <file>`) {
		t.Fatalf("unexpected help compare output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `--compare-css`) || !strings.Contains(stdout.String(), `--css-property <name>`) {
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
