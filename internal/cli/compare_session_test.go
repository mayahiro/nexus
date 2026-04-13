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
