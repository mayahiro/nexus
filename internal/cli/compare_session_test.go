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

	var report compareReportJSON
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
	if report.Findings[1].Locator != "" {
		t.Fatalf("expected no shared locator for renamed button: %+v", report.Findings[1])
	}
	if report.Findings[3].Locator != `label "Email"` {
		t.Fatalf("expected label locator for email findings: %+v", report.Findings[3])
	}
	if report.Findings[4].Locator != `href "/legacy"` || report.Findings[5].Locator != `href "/next"` {
		t.Fatalf("expected href locators for link findings: %+v", report.Findings)
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
		"--target-ref", "/tmp/fake-chromium",
		"--json",
	}
	if code := Run(context.Background(), args, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected compare url exit code: %d\n%s", code, stdout.String())
	}

	var report compareReportJSON
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
	for _, sessionID := range handler.attachIDs {
		targets := handler.waitTargets[sessionID]
		if len(targets) != 1 || targets[0] != "function" {
			t.Fatalf("expected document-ready wait for %s, got %#v", sessionID, targets)
		}
		values := handler.waitValues[sessionID]
		if len(values) != 1 || values[0] != `document.readyState === "complete"` {
			t.Fatalf("expected document-ready expression for %s, got %#v", sessionID, values)
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

func TestCompareCSSProperties(t *testing.T) {
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
		"--css-property", "color",
		"--css-property", "pointer-events",
		"--json",
	}
	if code := Run(context.Background(), args, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected compare css exit code: %d\n%s", code, stdout.String())
	}

	var report compareReportJSON
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("unexpected compare css json: %v\n%s", err, stdout.String())
	}
	if report.Summary.CSSChanged != 2 {
		t.Fatalf("unexpected css_changed summary: %+v", report.Summary)
	}
	if report.Summary.Warning != 7 || report.Summary.Info != 2 {
		t.Fatalf("unexpected severity summary: %+v", report.Summary)
	}

	fields := map[string]bool{}
	for _, finding := range report.Findings {
		if finding.Field == "color" || finding.Field == "pointer-events" {
			fields[finding.Field] = true
		}
	}
	if !fields["color"] || !fields["pointer-events"] {
		t.Fatalf("expected css findings for color and pointer-events: %+v", report.Findings)
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

func TestCompareURLWaitOptions(t *testing.T) {
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
		observations: map[string]api.Observation{
			"https://old.example.test/orders": {
				URLOrScreen: "https://old.example.test/orders",
				Title:       "Orders",
				Text:        "Orders stable",
			},
			"https://new.example.test/orders": {
				URLOrScreen: "https://new.example.test/orders",
				Title:       "Orders",
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
		"https://old.example.test/orders",
		"https://new.example.test/orders",
		"--target-ref", "/tmp/fake-chromium",
		"--wait-function", `window.appReady === true`,
		"--wait-network-idle",
		"--json",
	}
	if code := Run(context.Background(), args, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected compare wait-options exit code: %d\n%s", code, stdout.String())
	}

	for _, sessionID := range handler.attachIDs {
		targets := handler.waitTargets[sessionID]
		if len(targets) != 3 {
			t.Fatalf("expected three wait steps for %s, got %#v", sessionID, targets)
		}
		if targets[0] != "function" || targets[1] != "function" || targets[2] != "function" {
			t.Fatalf("unexpected wait targets for %s: %#v", sessionID, targets)
		}

		values := handler.waitValues[sessionID]
		if len(values) != 3 {
			t.Fatalf("expected three wait values for %s, got %#v", sessionID, values)
		}
		if values[0] != `document.readyState === "complete"` {
			t.Fatalf("unexpected document-ready wait for %s: %#v", sessionID, values)
		}
		if values[1] != `window.appReady === true` {
			t.Fatalf("unexpected custom wait-function for %s: %#v", sessionID, values)
		}
		if !strings.Contains(values[2], `performance.getEntriesByType("resource")`) {
			t.Fatalf("unexpected network-idle expression for %s: %#v", sessionID, values)
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

func TestCompareScopeSelector(t *testing.T) {
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
		observations: map[string]api.Observation{
			"https://old.example.test/products": {
				URLOrScreen: "https://old.example.test/products",
				Title:       "Products",
				Text:        "Filters",
				Tree: []api.Node{
					{ID: 1, Ref: "@e1", Fingerprint: "filters", Role: "aside", Name: "Filters", Visible: true, Enabled: true},
				},
			},
			"https://new.example.test/products": {
				URLOrScreen: "https://new.example.test/products",
				Title:       "Products",
				Text:        "Filters",
				Tree: []api.Node{
					{ID: 1, Ref: "@e1", Fingerprint: "filters", Role: "aside", Name: "Filters", Visible: true, Enabled: true},
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
		"compare",
		"https://old.example.test/products",
		"https://new.example.test/products",
		"--target-ref", "/tmp/fake-chromium",
		"--scope-selector", "aside.filters",
		"--json",
	}
	if code := Run(context.Background(), args, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected compare scope exit code: %d\n%s", code, stdout.String())
	}

	var report compareReportJSON
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("unexpected compare scope json: %v\n%s", err, stdout.String())
	}
	if report.Scope == nil {
		t.Fatalf("expected scope in compare report: %+v", report)
	}
	if report.Scope.Selector != "aside.filters" {
		t.Fatalf("unexpected scope selector: %+v", report.Scope)
	}
	if !report.Scope.Old.Matched || report.Scope.Old.Tag != "aside" {
		t.Fatalf("unexpected old scope: %+v", report.Scope)
	}
	if !report.Scope.New.Matched || report.Scope.New.Tag != "aside" {
		t.Fatalf("unexpected new scope: %+v", report.Scope)
	}
	for _, sessionID := range handler.attachIDs {
		scopes := handler.observeScopes[sessionID]
		if len(scopes) == 0 || scopes[len(scopes)-1] != "aside.filters" {
			t.Fatalf("expected scope selector to reach observe for %s, got %#v", sessionID, scopes)
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

func TestCompareScopeSelectorError(t *testing.T) {
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
		observations: map[string]api.Observation{
			"https://old.example.test/products": {
				URLOrScreen: "https://old.example.test/products",
				Title:       "Products",
				Text:        "Products",
			},
			"https://new.example.test/products": {
				URLOrScreen: "https://new.example.test/products",
				Title:       "Products",
				Text:        "Products",
			},
		},
		scopeErrors: map[string]string{
			"aside.filters": "scope selector matched 0 nodes: aside.filters",
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, handler, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	args := []string{
		"compare",
		"https://old.example.test/products",
		"https://new.example.test/products",
		"--target-ref", "/tmp/fake-chromium",
		"--scope-selector", "aside.filters",
	}
	if code := Run(context.Background(), args, &stdout, &stdout); code == 0 {
		t.Fatalf("expected compare scope error\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "old side scope selector matched 0 nodes: aside.filters") {
		t.Fatalf("unexpected compare scope error: %s", stdout.String())
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
		"--ignore-selector", "role=link",
		"--mask-selector", "role=textbox&name=Email",
		"--ignore-text-regex", `20\d\d-\d\d-\d\d`,
		"--json",
	}
	if code := Run(context.Background(), args, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected compare selector exit code: %d\n%s", code, stdout.String())
	}

	var report compareReportJSON
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

func TestCompareReportOutputs(t *testing.T) {
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

	jsonPath := filepath.Join(t.TempDir(), "compare.json")
	mdPath := filepath.Join(t.TempDir(), "compare.md")

	var stdout bytes.Buffer
	args := []string{
		"compare",
		"--old-session", "old",
		"--new-session", "new",
		"--ignore-text-regex", `20\d\d-\d\d-\d\d`,
		"--output-json", jsonPath,
		"--output-md", mdPath,
	}
	if code := Run(context.Background(), args, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected compare report exit code: %d\n%s", code, stdout.String())
	}

	jsonBytes, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var report compareReportJSON
	if err := json.Unmarshal(jsonBytes, &report); err != nil {
		t.Fatalf("unexpected compare output json: %v\n%s", err, string(jsonBytes))
	}
	if report.Summary.TotalFindings != 6 {
		t.Fatalf("unexpected compare output summary: %+v", report.Summary)
	}

	mdBytes, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatal(err)
	}
	md := string(mdBytes)
	if !strings.Contains(md, "# Compare Report") {
		t.Fatalf("unexpected markdown output: %s", md)
	}
	if !strings.Contains(md, "## Findings") {
		t.Fatalf("unexpected markdown output: %s", md)
	}
	if !strings.Contains(md, "locator `label \"Email\"`") {
		t.Fatalf("expected locator in markdown output: %s", md)
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
