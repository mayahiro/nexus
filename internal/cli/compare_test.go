package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

type compareReportJSON struct {
	Old      compareSnapshotJSON  `json:"old"`
	New      compareSnapshotJSON  `json:"new"`
	Summary  compareSummaryJSON   `json:"summary"`
	Findings []compareFindingJSON `json:"findings"`
}

type compareSnapshotJSON struct {
	SessionID string `json:"session_id"`
	URL       string `json:"url"`
	Title     string `json:"title"`
}

type compareSummaryJSON struct {
	Same            bool `json:"same"`
	TotalFindings   int  `json:"total_findings"`
	TitleChanged    int  `json:"title_changed"`
	TextChanged     int  `json:"text_changed"`
	MissingNodes    int  `json:"missing_nodes"`
	NewNodes        int  `json:"new_nodes"`
	StateChanged    int  `json:"state_changed"`
	PageTextChanged int  `json:"page_text_changed"`
	Critical        int  `json:"critical"`
	Warning         int  `json:"warning"`
	Info            int  `json:"info"`
}

type compareFindingJSON struct {
	Severity string `json:"severity"`
	Impact   string `json:"impact"`
	Field    string `json:"field"`
}

type compareManifestReportJSON struct {
	Summary compareManifestSummaryJSON `json:"summary"`
	Pages   []compareManifestPageJSON  `json:"pages"`
}

type compareManifestSummaryJSON struct {
	TotalPages    int `json:"total_pages"`
	ComparedPages int `json:"compared_pages"`
	FailedPages   int `json:"failed_pages"`
	TotalFindings int `json:"total_findings"`
	Critical      int `json:"critical"`
	Warning       int `json:"warning"`
}

type compareManifestPageJSON struct {
	Name   string             `json:"name"`
	Report *compareReportJSON `json:"report"`
}

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

type compareRPCHandler struct{}

type compareURLRPCHandler struct {
	mu                  sync.Mutex
	attachIDs           []string
	sessionURLs         map[string]string
	sessionObservations map[string]api.Observation
	observations        map[string]api.Observation
	observeCount        map[string]int
}

func (compareRPCHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, nil
}

func (compareRPCHandler) AttachSession(context.Context, api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	return api.AttachSessionResponse{}, nil
}

func (compareRPCHandler) ListSessions(context.Context, api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{}, nil
}

func (compareRPCHandler) DetachSession(context.Context, api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{}, nil
}

func (compareRPCHandler) StopDaemon(context.Context, api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (compareRPCHandler) ObserveSession(_ context.Context, req api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	switch req.SessionID {
	case "old":
		return api.ObserveSessionResponse{
			Observation: api.Observation{
				SessionID:   "old",
				URLOrScreen: "https://old.example.test/dashboard",
				Title:       "Orders",
				Text:        "Orders 2026-04-13",
				Tree: []api.Node{
					{ID: 1, Ref: "@e1", Fingerprint: "cta-save", Role: "button", Name: "Save", Visible: true, Enabled: true, Invokable: true},
					{ID: 2, Ref: "@e2", Fingerprint: "email", Role: "textbox", Name: "Email", Value: "old@example.com", Visible: true, Enabled: true, Editable: true},
					{ID: 3, Ref: "@e3", Fingerprint: "legacy-link", Role: "link", Text: "Legacy", Visible: true, Enabled: true, Invokable: true, Attrs: map[string]string{"href": "/legacy"}},
					{ID: 4, Ref: "@e4", Fingerprint: "status", Role: "status", Text: "Ready 2026-04-13", Visible: true, Enabled: true},
				},
			},
		}, nil
	case "new":
		return api.ObserveSessionResponse{
			Observation: api.Observation{
				SessionID:   "new",
				URLOrScreen: "https://new.example.test/dashboard",
				Title:       "Orders v2",
				Text:        "Orders 2026-04-14",
				Tree: []api.Node{
					{ID: 1, Ref: "@e1", Fingerprint: "cta-save", Role: "button", Name: "Submit", Visible: true, Enabled: true, Invokable: true},
					{ID: 2, Ref: "@e2", Fingerprint: "email", Role: "textbox", Name: "Email", Value: "new@example.com", Visible: true, Enabled: false, Editable: true},
					{ID: 3, Ref: "@e3", Fingerprint: "next-link", Role: "link", Text: "Next", Visible: true, Enabled: true, Invokable: true, Attrs: map[string]string{"href": "/next"}},
					{ID: 4, Ref: "@e4", Fingerprint: "status", Role: "status", Text: "Ready 2026-04-14", Visible: true, Enabled: true},
				},
			},
		}, nil
	default:
		return api.ObserveSessionResponse{}, nil
	}
}

func (compareRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if req.Action.Kind != "wait" {
		return api.ActSessionResponse{}, nil
	}
	return api.ActSessionResponse{
		Result: api.ActionResult{
			OK:      true,
			Changed: false,
			Message: "waited",
		},
	}, nil
}

func (h *compareURLRPCHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, nil
}

func (h *compareURLRPCHandler) AttachSession(_ context.Context, req api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, existing := range h.attachIDs {
		if existing == req.SessionID {
			return api.AttachSessionResponse{}, errors.New("duplicate session id")
		}
	}

	h.attachIDs = append(h.attachIDs, req.SessionID)
	if h.sessionURLs == nil {
		h.sessionURLs = map[string]string{}
	}
	h.sessionURLs[req.SessionID] = req.Options["initial_url"]

	return api.AttachSessionResponse{
		Session: api.Session{
			ID:         req.SessionID,
			TargetType: req.TargetType,
			TargetRef:  req.TargetRef,
			Backend:    req.Backend,
			Options:    req.Options,
		},
	}, nil
}

func (h *compareURLRPCHandler) ListSessions(context.Context, api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{}, nil
}

func (h *compareURLRPCHandler) DetachSession(context.Context, api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{}, nil
}

func (h *compareURLRPCHandler) StopDaemon(context.Context, api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (h *compareURLRPCHandler) ObserveSession(_ context.Context, req api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if observation, ok := h.sessionObservations[req.SessionID]; ok {
		observation.SessionID = req.SessionID
		return api.ObserveSessionResponse{Observation: observation}, nil
	}

	url := h.sessionURLs[req.SessionID]
	observation, ok := h.observations[url]
	if !ok {
		return api.ObserveSessionResponse{}, errors.New("unknown session")
	}
	if h.observeCount == nil {
		h.observeCount = map[string]int{}
	}
	h.observeCount[req.SessionID]++
	if h.observeCount[req.SessionID] == 1 {
		return api.ObserveSessionResponse{
			Observation: api.Observation{
				SessionID:   req.SessionID,
				URLOrScreen: "about:blank",
			},
		}, nil
	}
	observation.SessionID = req.SessionID
	return api.ObserveSessionResponse{Observation: observation}, nil
}

func (h *compareURLRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if req.Action.Kind != "wait" {
		return api.ActSessionResponse{}, nil
	}
	return api.ActSessionResponse{
		Result: api.ActionResult{
			OK:      true,
			Changed: false,
		},
	}, nil
}
