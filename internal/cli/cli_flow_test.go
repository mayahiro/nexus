package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/rpc"
)

type flowRPCHandler struct {
	noopRPCHandler

	mu             sync.Mutex
	attachRequests []api.AttachSessionRequest
	actRequests    []api.ActSessionRequest
	detachIDs      []string
}

func (h *flowRPCHandler) AttachSession(_ context.Context, req api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	h.mu.Lock()
	h.attachRequests = append(h.attachRequests, req)
	h.mu.Unlock()
	return api.AttachSessionResponse{
		Session: api.Session{
			ID:      req.SessionID,
			Backend: req.Backend,
			Options: req.Options,
		},
	}, nil
}

func (h *flowRPCHandler) ObserveSession(_ context.Context, req api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	sessionID := req.SessionID
	isOld := strings.Contains(sessionID, "old")
	label := "New Dashboard"
	if isOld {
		label = "Old Dashboard"
	}
	return api.ObserveSessionResponse{
		Observation: api.Observation{
			SessionID:   sessionID,
			URLOrScreen: "https://example.com/dashboard",
			Title:       label,
			Text:        label,
			Tree: []api.Node{
				{ID: 1, Ref: "@e1", Role: "textbox", Name: "Email", Visible: true, Enabled: true, Editable: true},
				{ID: 2, Ref: "@e2", Role: "button", Name: "Sign in", Text: "Sign in", Visible: true, Enabled: true, Invokable: true},
				{ID: 3, Ref: "@e3", Role: "heading", Name: label, Text: label, Visible: true, Enabled: true},
			},
		},
	}, nil
}

func (h *flowRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	h.mu.Lock()
	h.actRequests = append(h.actRequests, req)
	h.mu.Unlock()

	switch req.Action.Kind {
	case "wait":
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Message: "waited"}}, nil
	case "fill":
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Changed: true, Message: "filled"}}, nil
	case "invoke":
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Changed: true, Message: "clicked"}}, nil
	case "viewport":
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Changed: true, Message: "set viewport"}}, nil
	default:
		return api.ActSessionResponse{}, nil
	}
}

func (h *flowRPCHandler) DetachSession(_ context.Context, req api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	h.mu.Lock()
	h.detachIDs = append(h.detachIDs, req.SessionID)
	h.mu.Unlock()
	return api.DetachSessionResponse{
		Session: api.Session{ID: req.SessionID},
	}, nil
}

func TestFlowRunManifest(t *testing.T) {
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

	handler := &flowRPCHandler{}
	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, handler, rpc.ServeOptions{})
	}()

	manifestPath := filepath.Join(t.TempDir(), "flow-manifest.json")
	manifest := map[string]any{
		"defaults": map[string]any{
			"backend":    "chromium",
			"target_ref": "/tmp/fake-browser",
		},
		"matrices": map[string]any{
			"desktop": map[string]any{
				"viewport": "1440x900",
			},
			"mobile": map[string]any{
				"viewport": "390x844",
			},
		},
		"scenarios": []map[string]any{
			{
				"name": "login",
				"matrix": []string{
					"desktop",
					"mobile",
				},
				"variables": map[string]any{
					"email": "user@example.com",
				},
				"old": map[string]any{
					"url": "https://old.example.com/login",
				},
				"new": map[string]any{
					"url": "https://new.example.com/login",
				},
				"steps": []map[string]any{
					{
						"action": "wait",
						"target": "selector",
						"value":  "form",
					},
					{
						"action":  "fill",
						"locator": "label=Email",
						"text":    "{{ email }}",
					},
					{
						"action":  "click",
						"locator": "role=button&name=Sign in",
					},
					{
						"action": "compare",
						"name":   "dashboard",
					},
				},
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
		"flow",
		"run",
		"--manifest", manifestPath,
		"--json",
	}
	if code := Run(context.Background(), args, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected flow run exit code: %d\n%s", code, stdout.String())
	}

	var report flowReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("unexpected flow report json: %v\n%s", err, stdout.String())
	}
	if report.Summary.TotalScenarios != 2 || report.Summary.CompletedScenarios != 2 || report.Summary.FailedScenarios != 0 {
		t.Fatalf("unexpected flow summary: %+v", report.Summary)
	}
	if report.Summary.TotalCompares != 2 || report.Summary.DifferentCompares != 2 {
		t.Fatalf("unexpected flow compare summary: %+v", report.Summary)
	}
	if len(report.Scenarios) != 2 {
		t.Fatalf("unexpected flow scenarios: %+v", report.Scenarios)
	}

	handler.mu.Lock()
	defer handler.mu.Unlock()

	if len(handler.attachRequests) != 4 {
		t.Fatalf("expected 4 attach requests, got %#v", handler.attachRequests)
	}

	desktopCount := 0
	mobileCount := 0
	fillCount := 0
	clickCount := 0
	for _, req := range handler.attachRequests {
		switch {
		case req.Options["viewport_width"] == "1440" && req.Options["viewport_height"] == "900":
			desktopCount++
		case req.Options["viewport_width"] == "390" && req.Options["viewport_height"] == "844":
			mobileCount++
		}
	}
	for _, req := range handler.actRequests {
		if req.Action.Kind == "fill" && req.Action.Text == "user@example.com" {
			fillCount++
		}
		if req.Action.Kind == "invoke" {
			clickCount++
		}
	}
	if desktopCount != 2 || mobileCount != 2 {
		t.Fatalf("unexpected viewport attach distribution: desktop=%d mobile=%d requests=%#v", desktopCount, mobileCount, handler.attachRequests)
	}
	if fillCount != 4 {
		t.Fatalf("expected 4 fill actions, got %#v", handler.actRequests)
	}
	if clickCount != 4 {
		t.Fatalf("expected 4 click actions, got %#v", handler.actRequests)
	}
	if len(handler.detachIDs) != 4 {
		t.Fatalf("expected 4 detached sessions, got %#v", handler.detachIDs)
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
