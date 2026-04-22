package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/rpc"
)

func TestCompareManifest(t *testing.T) {
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
				URLOrScreen: "https://old.example.test/dashboard",
				Title:       "Orders",
				Text:        "Orders 2026-04-13",
				Tree: []api.Node{
					{ID: 1, Ref: "@e1", Fingerprint: "cta-save", Role: "button", Name: "Save", Visible: true, Enabled: true, Invokable: true},
					{ID: 2, Ref: "@e2", Fingerprint: "email", Role: "textbox", Name: "Email", Value: "old@example.com", Visible: true, Enabled: true, Editable: true},
				},
			},
			"https://new.example.test/dashboard": {
				URLOrScreen: "https://new.example.test/dashboard",
				Title:       "Orders v2",
				Text:        "Orders 2026-04-14",
				Tree: []api.Node{
					{ID: 1, Ref: "@e1", Fingerprint: "cta-save", Role: "button", Name: "Submit", Visible: true, Enabled: true, Invokable: true},
					{ID: 2, Ref: "@e2", Fingerprint: "email", Role: "textbox", Name: "Email", Value: "new@example.com", Visible: true, Enabled: false, Editable: true},
				},
			},
		},
		sessionObservations: map[string]api.Observation{
			"old": {
				SessionID:   "old",
				URLOrScreen: "https://old.example.test/session",
				Title:       "Orders",
				Text:        "Orders 2026-04-13",
				Tree: []api.Node{
					{ID: 1, Ref: "@e1", Fingerprint: "cta-save", Role: "button", Name: "Save", Visible: true, Enabled: true, Invokable: true},
					{ID: 2, Ref: "@e2", Fingerprint: "email", Role: "textbox", Name: "Email", Value: "old@example.com", Visible: true, Enabled: true, Editable: true},
					{ID: 3, Ref: "@e3", Fingerprint: "legacy-link", Role: "link", Text: "Legacy", Visible: true, Enabled: true, Invokable: true},
				},
			},
			"new": {
				SessionID:   "new",
				URLOrScreen: "https://new.example.test/session",
				Title:       "Orders v2",
				Text:        "Orders 2026-04-14",
				Tree: []api.Node{
					{ID: 1, Ref: "@e1", Fingerprint: "cta-save", Role: "button", Name: "Submit", Visible: true, Enabled: true, Invokable: true},
					{ID: 2, Ref: "@e2", Fingerprint: "email", Role: "textbox", Name: "Email", Value: "new@example.com", Visible: true, Enabled: false, Editable: true},
					{ID: 3, Ref: "@e3", Fingerprint: "next-link", Role: "link", Text: "Next", Visible: true, Enabled: true, Invokable: true},
				},
			},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, handler, rpc.ServeOptions{})
	}()

	manifestPath := filepath.Join(t.TempDir(), "compare-manifest.json")
	manifest := map[string]any{
		"defaults": map[string]any{
			"ignore_text_regex": []string{`20\d\d-\d\d-\d\d`},
			"mask_selector":     []string{"role=textbox&name=Email"},
			"scope_selector":    "main",
			"wait_function":     `window.appReady === true`,
			"wait_network_idle": true,
		},
		"pages": []map[string]any{
			{
				"name":           "dashboard-url",
				"old_url":        "https://old.example.test/dashboard",
				"new_url":        "https://new.example.test/dashboard",
				"scope_selector": "aside.filters",
			},
			{
				"name":              "dashboard-session",
				"old_session":       "old",
				"new_session":       "new",
				"ignore_selector":   []string{"role=link"},
				"wait_network_idle": false,
			},
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	args := []string{
		"compare",
		"--manifest", manifestPath,
		"--target-ref", "/tmp/fake-chromium",
		"--json",
	}
	if code := Run(context.Background(), args, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected compare manifest exit code: %d\n%s", code, stdout.String())
	}

	var report compareManifestReportJSON
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("unexpected compare manifest json: %v\n%s", err, stdout.String())
	}
	if report.Summary.TotalPages != 2 || report.Summary.ComparedPages != 2 || report.Summary.FailedPages != 0 {
		t.Fatalf("unexpected compare manifest summary: %+v", report.Summary)
	}
	if report.Summary.TotalFindings != 6 || report.Summary.Critical != 2 || report.Summary.Warning != 4 {
		t.Fatalf("unexpected compare manifest findings: %+v", report.Summary)
	}
	if len(report.Pages) != 2 {
		t.Fatalf("unexpected compare manifest pages: %+v", report.Pages)
	}
	if report.Pages[0].Name != "dashboard-url" || report.Pages[0].Report == nil {
		t.Fatalf("unexpected first manifest page: %+v", report.Pages[0])
	}
	if report.Pages[1].Name != "dashboard-session" || report.Pages[1].Report == nil {
		t.Fatalf("unexpected second manifest page: %+v", report.Pages[1])
	}
	for _, sessionID := range handler.attachIDs {
		if len(handler.waitValues[sessionID]) != 3 {
			t.Fatalf("expected url manifest page waits for %s, got %#v", sessionID, handler.waitValues[sessionID])
		}
	}
	if len(handler.waitValues["old"]) != 1 || handler.waitValues["old"][0] != `window.appReady === true` {
		t.Fatalf("expected merged wait_function on old session page, got %#v", handler.waitValues["old"])
	}
	if len(handler.waitValues["new"]) != 1 || handler.waitValues["new"][0] != `window.appReady === true` {
		t.Fatalf("expected merged wait_function on new session page, got %#v", handler.waitValues["new"])
	}
	for _, sessionID := range handler.attachIDs {
		scopes := handler.observeScopes[sessionID]
		if len(scopes) == 0 || scopes[len(scopes)-1] != "aside.filters" {
			t.Fatalf("expected page scope selector on %s, got %#v", sessionID, scopes)
		}
	}
	if len(handler.observeScopes["old"]) == 0 || handler.observeScopes["old"][0] != "main" {
		t.Fatalf("expected default scope selector on old session page, got %#v", handler.observeScopes["old"])
	}
	if len(handler.observeScopes["new"]) == 0 || handler.observeScopes["new"][0] != "main" {
		t.Fatalf("expected default scope selector on new session page, got %#v", handler.observeScopes["new"])
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

func TestCompareManifestAppliesBackendAndViewportOverrides(t *testing.T) {
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
			"https://old.example.test/defaults": {
				URLOrScreen: "https://old.example.test/defaults",
				Title:       "Defaults",
				Text:        "Defaults",
				Tree: []api.Node{
					{ID: 1, Ref: "@e1", Fingerprint: "title", Role: "heading", Name: "Defaults", Visible: true, Enabled: true},
				},
			},
			"https://new.example.test/defaults": {
				URLOrScreen: "https://new.example.test/defaults",
				Title:       "Defaults",
				Text:        "Defaults",
				Tree: []api.Node{
					{ID: 1, Ref: "@e1", Fingerprint: "title", Role: "heading", Name: "Defaults", Visible: true, Enabled: true},
				},
			},
			"https://old.example.test/override": {
				URLOrScreen: "https://old.example.test/override",
				Title:       "Override",
				Text:        "Override",
				Tree: []api.Node{
					{ID: 1, Ref: "@e1", Fingerprint: "title", Role: "heading", Name: "Override", Visible: true, Enabled: true},
				},
			},
			"https://new.example.test/override": {
				URLOrScreen: "https://new.example.test/override",
				Title:       "Override",
				Text:        "Override",
				Tree: []api.Node{
					{ID: 1, Ref: "@e1", Fingerprint: "title", Role: "heading", Name: "Override", Visible: true, Enabled: true},
				},
			},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, handler, rpc.ServeOptions{})
	}()

	manifestPath := filepath.Join(t.TempDir(), "compare-manifest.json")
	manifest := map[string]any{
		"defaults": map[string]any{
			"backend":  "chromium",
			"viewport": "1440x900",
		},
		"pages": []map[string]any{
			{
				"name":    "defaults",
				"old_url": "https://old.example.test/defaults",
				"new_url": "https://new.example.test/defaults",
			},
			{
				"name":     "override",
				"old_url":  "https://old.example.test/override",
				"new_url":  "https://new.example.test/override",
				"backend":  "lightpanda",
				"viewport": "1280x720",
			},
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	args := []string{
		"compare",
		"--manifest", manifestPath,
		"--target-ref", "/tmp/fake-browser",
		"--json",
	}
	if code := Run(context.Background(), args, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected compare manifest exit code: %d\n%s", code, stdout.String())
	}

	countAttaches := func(url string, backend string, width string, height string) int {
		handler.mu.Lock()
		defer handler.mu.Unlock()

		total := 0
		for _, req := range handler.attachRequests {
			if req.Options["initial_url"] != url {
				continue
			}
			if req.Backend != backend {
				continue
			}
			if req.Options["viewport_width"] != width || req.Options["viewport_height"] != height {
				continue
			}
			total++
		}
		return total
	}

	if countAttaches("https://old.example.test/defaults", "chromium", "1440", "900") != 1 {
		t.Fatalf("expected defaults old url attach request to use chromium 1440x900, got %#v", handler.attachRequests)
	}
	if countAttaches("https://new.example.test/defaults", "chromium", "1440", "900") != 1 {
		t.Fatalf("expected defaults new url attach request to use chromium 1440x900, got %#v", handler.attachRequests)
	}
	if countAttaches("https://old.example.test/override", "lightpanda", "1280", "720") != 1 {
		t.Fatalf("expected override old url attach request to use lightpanda 1280x720, got %#v", handler.attachRequests)
	}
	if countAttaches("https://new.example.test/override", "lightpanda", "1280", "720") != 1 {
		t.Fatalf("expected override new url attach request to use lightpanda 1280x720, got %#v", handler.attachRequests)
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

func TestCompareManifestCSSOverrides(t *testing.T) {
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
			"https://old.example.test/colors": {
				URLOrScreen: "https://old.example.test/colors",
				Title:       "Styles",
				Text:        "Styles",
				Tree: []api.Node{
					{ID: 1, Ref: "@e1", Fingerprint: "cta-save", Role: "button", Name: "Save", Visible: true, Enabled: true, Invokable: true, Styles: map[string]string{"color": "rgb(0, 0, 0)", "display": "inline-block"}},
				},
			},
			"https://new.example.test/colors": {
				URLOrScreen: "https://new.example.test/colors",
				Title:       "Styles",
				Text:        "Styles",
				Tree: []api.Node{
					{ID: 1, Ref: "@e1", Fingerprint: "cta-save", Role: "button", Name: "Save", Visible: true, Enabled: true, Invokable: true, Styles: map[string]string{"color": "rgb(255, 0, 0)", "display": "block"}},
				},
			},
			"https://old.example.test/disabled": {
				URLOrScreen: "https://old.example.test/disabled",
				Title:       "Disabled",
				Text:        "Disabled",
				Tree: []api.Node{
					{ID: 1, Ref: "@e1", Fingerprint: "cta-save", Role: "button", Name: "Save", Visible: true, Enabled: true, Invokable: true, Styles: map[string]string{"color": "rgb(0, 0, 0)"}},
				},
			},
			"https://new.example.test/disabled": {
				URLOrScreen: "https://new.example.test/disabled",
				Title:       "Disabled",
				Text:        "Disabled",
				Tree: []api.Node{
					{ID: 1, Ref: "@e1", Fingerprint: "cta-save", Role: "button", Name: "Save", Visible: true, Enabled: true, Invokable: true, Styles: map[string]string{"color": "rgb(255, 0, 0)"}},
				},
			},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, handler, rpc.ServeOptions{})
	}()

	manifestPath := filepath.Join(t.TempDir(), "compare-manifest.json")
	manifest := map[string]any{
		"defaults": map[string]any{
			"compare_css":  true,
			"css_property": []string{"display"},
		},
		"pages": []map[string]any{
			{
				"name":         "color-only",
				"old_url":      "https://old.example.test/colors",
				"new_url":      "https://new.example.test/colors",
				"css_property": []string{"color"},
			},
			{
				"name":        "css-disabled",
				"old_url":     "https://old.example.test/disabled",
				"new_url":     "https://new.example.test/disabled",
				"compare_css": false,
			},
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	args := []string{
		"compare",
		"--manifest", manifestPath,
		"--target-ref", "/tmp/fake-chromium",
		"--json",
	}
	if code := Run(context.Background(), args, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected compare manifest css exit code: %d\n%s", code, stdout.String())
	}

	var report compareManifestReportJSON
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("unexpected compare manifest css json: %v\n%s", err, stdout.String())
	}
	if len(report.Pages) != 2 || report.Pages[0].Report == nil || report.Pages[1].Report == nil {
		t.Fatalf("unexpected compare manifest css pages: %+v", report.Pages)
	}
	if report.Pages[0].Report.Summary.CSSChanged != 1 {
		t.Fatalf("unexpected css override summary: %+v", report.Pages[0].Report.Summary)
	}
	if report.Pages[1].Report.Summary.CSSChanged != 0 || !report.Pages[1].Report.Summary.Same {
		t.Fatalf("unexpected css disabled summary: %+v", report.Pages[1].Report.Summary)
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
