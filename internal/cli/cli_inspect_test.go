package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/rpc"
)

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

	stdout.Reset()
	if code := Run(context.Background(), []string{"get", "bbox", "--selector", "#hero", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected get bbox selector exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"width": 120`) {
		t.Fatalf("unexpected get bbox selector output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"get", "bbox", "@e3", "--selector", "#hero"}, &stdout, &stdout); code == 0 {
		t.Fatalf("expected get bbox selector with index validation to fail\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "get bbox with --selector does not accept an index") {
		t.Fatalf("unexpected get bbox selector validation output: %s", stdout.String())
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

func TestInspect(t *testing.T) {
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
		done <- rpc.Serve(ctx, listener, inspectRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"inspect", `role button --name "Submit"`, "--old-session", "old", "--new-session", "new"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected inspect exit code: %d\n%s", code, stdout.String())
	}
	output := stdout.String()
	if !strings.Contains(output, `locator: role button --name "Submit"`) {
		t.Fatalf("unexpected inspect output: %s", output)
	}
	if !strings.Contains(output, "color") || !strings.Contains(output, "rgb(0, 0, 0)") || !strings.Contains(output, "rgb(255, 0, 0)") {
		t.Fatalf("unexpected inspect output: %s", output)
	}
	if !strings.Contains(output, "changed") {
		t.Fatalf("unexpected inspect output: %s", output)
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"inspect", `role button`, "--old-session", "old", "--new-session", "new", "--nth", "2", "--css-property", "color"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected inspect nth exit code: %d\n%s", code, stdout.String())
	}
	output = stdout.String()
	if !strings.Contains(output, `old: old @e4 button "Cancel"`) {
		t.Fatalf("unexpected inspect nth output: %s", output)
	}
	if !strings.Contains(output, `new: new @e4 button "Cancel"`) {
		t.Fatalf("unexpected inspect nth output: %s", output)
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"inspect", `role button`, "--old-session", "old", "--new-session", "new", "--nth", "0"}, &stdout, &stdout); code == 0 {
		t.Fatalf("expected inspect nth validation to fail\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "inspect --nth must be a positive integer") {
		t.Fatalf("unexpected inspect nth validation output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"inspect", `role button --name "Submit"`, "--old-session", "old", "--new-session", "new", "--css-property", "color", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected inspect json exit code: %d\n%s", code, stdout.String())
	}
	var report inspectReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("unexpected inspect json: %v\n%s", err, stdout.String())
	}
	if len(report.Properties) != 1 || report.Properties[0].Name != "color" {
		t.Fatalf("unexpected inspect properties: %+v", report.Properties)
	}
	if report.Same {
		t.Fatalf("expected inspect report to differ: %+v", report)
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"inspect", `role button --name "Submit"`, "--old-session", "old", "--new-session", "new", "--layout-context"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected inspect layout context exit code: %d\n%s", code, stdout.String())
	}
	output = stdout.String()
	if !strings.Contains(output, "old layout context:") || !strings.Contains(output, "new layout context:") {
		t.Fatalf("unexpected inspect layout context output: %s", output)
	}
	if !strings.Contains(output, `main.layout`) || !strings.Contains(output, `display="grid"`) || !strings.Contains(output, `gap="16px"`) {
		t.Fatalf("unexpected inspect layout context output: %s", output)
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"inspect", `role button --name "Submit"`, "--old-session", "old", "--new-session", "new", "--layout-context", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected inspect layout context json exit code: %d\n%s", code, stdout.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("unexpected inspect layout context json: %v\n%s", err, stdout.String())
	}
	if len(report.LayoutProperties) == 0 || len(report.Old.Node.LayoutContext) != 1 || report.Old.Node.LayoutContext[0].Selector != "main.layout" {
		t.Fatalf("unexpected inspect layout context json: %+v", report)
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"inspect", "--selector", "aside.filters", "--old-session", "old", "--new-session", "new", "--css-property", "color", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected inspect selector exit code: %d\n%s", code, stdout.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("unexpected inspect selector json: %v\n%s", err, stdout.String())
	}
	if report.Locator.Kind != "selector" || report.Locator.Value != "aside.filters" {
		t.Fatalf("unexpected inspect selector locator: %+v", report.Locator)
	}
	if report.Old.Node.ID != 1 || report.New.Node.ID != 1 {
		t.Fatalf("unexpected inspect selector nodes: %+v", report)
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"inspect", "--selector", "aside.filters", "--old-session", "old", "--new-session", "new", "--nth", "2"}, &stdout, &stdout); code == 0 {
		t.Fatalf("expected inspect selector nth validation to fail\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "inspect --selector does not support --nth") {
		t.Fatalf("unexpected inspect selector nth output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"inspect", `role button`, "--selector", "aside.filters", "--old-session", "old", "--new-session", "new"}, &stdout, &stdout); code == 0 {
		t.Fatalf("expected inspect selector mixed-input validation to fail\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "inspect can not combine a locator with --selector") {
		t.Fatalf("unexpected inspect selector mixed-input output: %s", stdout.String())
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

func TestInspectSideSpecificScopeSelectors(t *testing.T) {
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

	handler := &compareURLRPCHandler{
		sessionObservations: map[string]api.Observation{
			"old": {
				SessionID:   "old",
				URLOrScreen: "https://old.example.test/dashboard",
				Title:       "Inspect",
				Tree: []api.Node{
					{ID: 10, Ref: "@e10", Role: "region", Name: "Filters", Visible: true, Enabled: true, Styles: map[string]string{"color": "rgb(0, 0, 0)"}},
				},
			},
			"new": {
				SessionID:   "new",
				URLOrScreen: "https://new.example.test/dashboard",
				Title:       "Inspect",
				Tree: []api.Node{
					{ID: 20, Ref: "@e20", Role: "region", Name: "Filters", Visible: true, Enabled: true, Styles: map[string]string{"color": "rgb(255, 0, 0)"}},
				},
			},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, handler, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	args := []string{
		"inspect",
		"--old-session", "old",
		"--new-session", "new",
		"--old-scope-selector", "#legacy-filters",
		"--new-scope-selector", "main .filters",
		"--css-property", "color",
		"--json",
	}
	if code := Run(context.Background(), args, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected inspect side-specific scope exit code: %d\n%s", code, stdout.String())
	}

	var report inspectReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("unexpected inspect side-specific scope json: %v\n%s", err, stdout.String())
	}
	if report.Locator.Kind != "selector" || report.Locator.Value != "" {
		t.Fatalf("unexpected inspect selector locator: %+v", report.Locator)
	}
	if report.Scope == nil || report.Scope.Old.Selector != "#legacy-filters" || report.Scope.New.Selector != "main .filters" {
		t.Fatalf("unexpected inspect scope report: %+v", report.Scope)
	}
	if report.Old.Node.ID != 10 || report.New.Node.ID != 20 {
		t.Fatalf("unexpected inspect nodes: %+v", report)
	}
	if got := handler.observeScopes["old"]; len(got) != 1 || got[0] != "#legacy-filters" {
		t.Fatalf("unexpected old observe scopes: %#v", got)
	}
	if got := handler.observeScopes["new"]; len(got) != 1 || got[0] != "main .filters" {
		t.Fatalf("unexpected new observe scopes: %#v", got)
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
	if code := Run(context.Background(), []string{"find", "label", "Email", "fill", "hello@example.com"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected find label fill exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "filled into @e3" {
		t.Fatalf("unexpected find label fill output: %s", stdout.String())
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

	stdout.Reset()
	if code := Run(context.Background(), []string{"find", "role", "button", "click", "--nth", "2"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected find nth exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "clicked @e4" {
		t.Fatalf("unexpected find nth output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"find", "role", "button", "click", "--nth", "0"}, &stdout, &stdout); code == 0 {
		t.Fatalf("expected invalid find nth to fail\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "find role --nth must be a positive integer") {
		t.Fatalf("unexpected invalid find nth output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"find", "role", "button", "--all", "--nth", "2"}, &stdout, &stdout); code == 0 {
		t.Fatalf("expected find --all --nth to fail\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "find role --all does not accept --nth") {
		t.Fatalf("unexpected find --all --nth output: %s", stdout.String())
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
