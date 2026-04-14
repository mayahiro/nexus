package cli

import (
	"context"
	"errors"
	"sync"

	"github.com/mayahiro/nexus/internal/api"
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
	CSSChanged      int  `json:"css_changed"`
	PageTextChanged int  `json:"page_text_changed"`
	Critical        int  `json:"critical"`
	Warning         int  `json:"warning"`
	Info            int  `json:"info"`
}

type compareFindingJSON struct {
	Severity string `json:"severity"`
	Impact   string `json:"impact"`
	Locator  string `json:"locator"`
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

type compareRPCHandler struct{}

type compareURLRPCHandler struct {
	mu                  sync.Mutex
	attachIDs           []string
	attachRequests      map[string]api.AttachSessionRequest
	sessionURLs         map[string]string
	sessionObservations map[string]api.Observation
	observations        map[string]api.Observation
	observeCount        map[string]int
	waitTargets         map[string][]string
	waitValues          map[string][]string
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
					{ID: 1, Ref: "@e1", Fingerprint: "cta-save", Role: "button", Name: "Save", Visible: true, Enabled: true, Invokable: true, Styles: map[string]string{"color": "rgb(0, 0, 0)", "display": "inline-block"}},
					{ID: 2, Ref: "@e2", Fingerprint: "email", Role: "textbox", Name: "Email", Value: "old@example.com", Visible: true, Enabled: true, Editable: true, Styles: map[string]string{"color": "rgb(34, 34, 34)", "pointer-events": "auto"}},
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
					{ID: 1, Ref: "@e1", Fingerprint: "cta-save", Role: "button", Name: "Submit", Visible: true, Enabled: true, Invokable: true, Styles: map[string]string{"color": "rgb(255, 0, 0)", "display": "inline-block"}},
					{ID: 2, Ref: "@e2", Fingerprint: "email", Role: "textbox", Name: "Email", Value: "new@example.com", Visible: true, Enabled: false, Editable: true, Styles: map[string]string{"color": "rgb(34, 34, 34)", "pointer-events": "none"}},
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
	if h.attachRequests == nil {
		h.attachRequests = map[string]api.AttachSessionRequest{}
	}
	h.attachRequests[req.SessionID] = req
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
	h.mu.Lock()
	if h.waitTargets == nil {
		h.waitTargets = map[string][]string{}
	}
	if h.waitValues == nil {
		h.waitValues = map[string][]string{}
	}
	h.waitTargets[req.SessionID] = append(h.waitTargets[req.SessionID], req.Action.Args["target"])
	h.waitValues[req.SessionID] = append(h.waitValues[req.SessionID], req.Action.Args["value"])
	h.mu.Unlock()
	return api.ActSessionResponse{
		Result: api.ActionResult{
			OK:      true,
			Changed: false,
		},
	}, nil
}
