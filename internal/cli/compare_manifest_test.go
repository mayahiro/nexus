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
		},
		"pages": []map[string]any{
			{
				"name":    "dashboard-url",
				"old_url": "https://old.example.test/dashboard",
				"new_url": "https://new.example.test/dashboard",
			},
			{
				"name":            "dashboard-session",
				"old_session":     "old",
				"new_session":     "new",
				"ignore_selector": []string{"role=link"},
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
