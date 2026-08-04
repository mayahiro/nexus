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
	if !strings.Contains(stdout.String(), "Usage:\n  nxctl [OPTIONS] <COMMAND>") {
		t.Fatalf("unexpected help output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Commands:\n") || !strings.Contains(stdout.String(), "compare     Compare browser interfaces") {
		t.Fatalf("unexpected command list: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), aiUsageDocURL) {
		t.Fatalf("unexpected help output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"--help"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected --help exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Usage:\n  nxctl [OPTIONS] <COMMAND>") {
		t.Fatalf("unexpected --help output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), migrationPlaybookDocURL) {
		t.Fatalf("unexpected --help output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"-h"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected -h exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Commands:") {
		t.Fatalf("unexpected -h output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"help", "wait"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected help wait exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `nxctl wait selector <CSS>`) {
		t.Fatalf("unexpected help wait output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), aiCompareDocURL) {
		t.Fatalf("unexpected help wait output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"wait", "--help"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected wait --help exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `nxctl wait url <VALUE>`) {
		t.Fatalf("unexpected wait --help output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"help", "find"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected help find exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `nxctl find role <QUERY> <click|input|fill|get> [VALUE] [--name <TEXT>] [--within <@eN>] [--nth <N>]`) {
		t.Fatalf("unexpected help find output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `nxctl find testid <QUERY> <click|input|fill|get>`) {
		t.Fatalf("unexpected help find output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"help", "get"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected help get exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `nxctl get bbox --selector <CSS>`) {
		t.Fatalf("unexpected help get output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"help", "inspect"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected help inspect exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `nxctl inspect <LOCATOR> --session <ID>`) {
		t.Fatalf("unexpected help inspect output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `nxctl inspect <LOCATOR> --old-session <ID> --new-session <ID>`) {
		t.Fatalf("unexpected help inspect output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `--nth <N>`) {
		t.Fatalf("unexpected help inspect output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `--layout-context`) {
		t.Fatalf("unexpected help inspect output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `--no-style-sources`) {
		t.Fatalf("unexpected help inspect output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `nxctl inspect --selector <CSS> --old-session <ID> --new-session <ID>`) {
		t.Fatalf("unexpected help inspect output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), aiInspectDocURL) {
		t.Fatalf("unexpected help inspect output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"help", "batch"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected help batch exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `nxctl batch --cmd "COMMAND" [--cmd "COMMAND"]... [--keep-going] [--json]`) {
		t.Fatalf("unexpected help batch output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Commands run in order; failures stop the batch unless --keep-going is set") {
		t.Fatalf("unexpected help batch output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"help", "screenshot"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected help screenshot exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Each capture attempt is capped at 10000 ms within the overall timeout") {
		t.Fatalf("unexpected help screenshot output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"help", "observe"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected help observe exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Each capture attempt is capped at 10000 ms within the overall timeout") {
		t.Fatalf("unexpected help observe output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"help", "compare"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected help compare exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `nxctl compare <old-url> <new-url>`) {
		t.Fatalf("unexpected help compare output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `nxctl compare --manifest <file>`) {
		t.Fatalf("unexpected help compare output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `--compare-css`) ||
		!strings.Contains(stdout.String(), `--all-css-properties`) ||
		!strings.Contains(stdout.String(), `--css-property <name>`) {
		t.Fatalf("unexpected help compare output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `--compare-layout`) {
		t.Fatalf("unexpected help compare output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `--no-default-ignores`) {
		t.Fatalf("unexpected help compare output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `--scope-selector <css>`) {
		t.Fatalf("unexpected help compare output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), aiCompareDocURL) {
		t.Fatalf("unexpected help compare output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), migrationPlaybookDocURL) {
		t.Fatalf("unexpected help compare output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"help", "flow"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected help flow exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), aiFlowDocURL) {
		t.Fatalf("unexpected help flow output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"help", "navigate"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected help navigate exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `nxctl navigate <URL>`) {
		t.Fatalf("unexpected help navigate output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"wait"}, &stdout, &stdout); code == 0 {
		t.Fatalf("expected wait without args to fail\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `error[missing-required]: required argument "target" is missing`) ||
		!strings.Contains(stdout.String(), `usage: nxctl wait [OPTIONS] <TARGET> [VALUE]`) {
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
	if !strings.Contains(stdout.String(), "error[unknown-command]: unknown command 'unknown'") ||
		!strings.Contains(stdout.String(), "usage: nxctl [OPTIONS] <COMMAND>") {
		t.Fatalf("unexpected batch failure output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(
		context.Background(),
		[]string{"batch", "--cmd", "help wait", "--cmd", "open", "--cmd", "help inspect"},
		&stdout,
		&stdout,
	); code == 0 {
		t.Fatalf("expected batch fail-fast failure\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "batch stopped at: open") {
		t.Fatalf("expected batch stop message: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), `nxctl inspect <LOCATOR>`) {
		t.Fatalf("batch continued after failure: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(
		context.Background(),
		[]string{"batch", "--keep-going", "--cmd", "help wait", "--cmd", "open", "--cmd", "help inspect"},
		&stdout,
		&stdout,
	); code == 0 {
		t.Fatalf("expected keep-going batch failure\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "batch step failed: open") {
		t.Fatalf("expected keep-going failure message: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `nxctl inspect <LOCATOR>`) {
		t.Fatalf("batch did not continue after failure: %s", stdout.String())
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

	stdout.Reset()
	if code := Run(
		context.Background(),
		[]string{"batch", "--cmd", "help wait", "--cmd", "open", "--cmd", "help inspect", "--json"},
		&stdout,
		&stdout,
	); code == 0 {
		t.Fatalf("expected batch json fail-fast failure\n%s", stdout.String())
	}
	if strings.Count(stdout.String(), `"command":`) != 2 {
		t.Fatalf("unexpected batch json result count: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), `"command": "help inspect"`) {
		t.Fatalf("batch json continued after failure: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(
		context.Background(),
		[]string{"batch", "--keep-going", "--cmd", "help wait", "--cmd", "open", "--cmd", "help inspect", "--json"},
		&stdout,
		&stdout,
	); code == 0 {
		t.Fatalf("expected keep-going batch json failure\n%s", stdout.String())
	}
	if strings.Count(stdout.String(), `"command":`) != 3 ||
		!strings.Contains(stdout.String(), `"command": "help inspect"`) {
		t.Fatalf("unexpected keep-going batch json output: %s", stdout.String())
	}
}

func TestBatchKeepGoingWithoutDaemon(t *testing.T) {
	var output bytes.Buffer
	code := Run(
		context.Background(),
		[]string{"batch", "--keep-going", "--cmd", "unknown", "--cmd", "help wait"},
		&output,
		&output,
	)
	if code == 0 {
		t.Fatalf("expected failed batch status\n%s", output.String())
	}
	if !strings.Contains(output.String(), "batch step failed: unknown") ||
		!strings.Contains(output.String(), "==> help wait") {
		t.Fatalf("unexpected keep-going output: %s", output.String())
	}

	output.Reset()
	code = Run(
		context.Background(),
		[]string{"batch", "--keep-going", "--cmd", `"unterminated`, "--cmd", "help wait"},
		&output,
		&output,
	)
	if code == 0 {
		t.Fatalf("expected malformed command failure\n%s", output.String())
	}
	if !strings.Contains(output.String(), "unterminated quote in batch command") ||
		!strings.Contains(output.String(), "==> help wait") {
		t.Fatalf("malformed command stopped keep-going batch: %s", output.String())
	}
}

func TestDirectRefLocatorDoesNotReobserve(t *testing.T) {
	node, directRef, err := directRefNode("@e42", nodeSelectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !directRef || node.ID != 42 || node.Ref != "@e42" {
		t.Fatalf("unexpected direct ref node: %+v direct=%t", node, directRef)
	}

	node, err = resolveFlowLocator(context.Background(), nil, "unused", "@e42", nodeSelectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if node.ID != 42 || node.Ref != "@e42" {
		t.Fatalf("unexpected direct flow ref: %+v", node)
	}

	if _, directRef, err := directRefNode("@e0", nodeSelectionOptions{}); !directRef || err == nil {
		t.Fatalf("expected invalid direct ref error, direct=%t err=%v", directRef, err)
	}
	if _, directRef, err := directRefNode("@e42", nodeSelectionOptions{Nth: 2}); !directRef || err == nil {
		t.Fatalf("expected direct ref nth error, direct=%t err=%v", directRef, err)
	}
}

func TestParseScreenshotRectAcceptsGetBBoxShape(t *testing.T) {
	rect, err := parseRectValue(map[string]interface{}{
		"x":      float64(1),
		"y":      float64(2),
		"width":  float64(30),
		"height": float64(40),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rect.X != 1 || rect.Y != 2 || rect.W != 30 || rect.H != 40 {
		t.Fatalf("unexpected screenshot rect: %+v", rect)
	}
}

func TestCommandDiagnostics(t *testing.T) {
	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"open"}, &stdout, &stdout); code == 0 {
		t.Fatalf("expected open without url to fail\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `error[missing-required]: required argument "url" is missing`) ||
		!strings.Contains(stdout.String(), `usage: nxctl open [OPTIONS] <URL>`) {
		t.Fatalf("unexpected open diagnostic output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"navigate"}, &stdout, &stdout); code == 0 {
		t.Fatalf("expected navigate without url to fail\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `error[missing-required]: required argument "url" is missing`) ||
		!strings.Contains(stdout.String(), `usage: nxctl navigate [OPTIONS] <URL>`) {
		t.Fatalf("unexpected navigate diagnostic output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"click", "--bogus", "3"}, &stdout, &stdout); code == 0 {
		t.Fatalf("expected click parse failure\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `error[unknown-option]: unknown option '--bogus'`) ||
		!strings.Contains(stdout.String(), `usage: nxctl click [OPTIONS] [TARGETS]...`) {
		t.Fatalf("unexpected click parse diagnostic output: %s", stdout.String())
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
