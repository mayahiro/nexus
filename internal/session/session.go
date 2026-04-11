package session

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/target"
	"github.com/mayahiro/nexus/internal/target/browser"
	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

var (
	ErrSessionExists   = errors.New("session already exists")
	ErrSessionNotFound = errors.New("session not found")
)

type Manager struct {
	mu       sync.RWMutex
	sessions map[string]entry
}

type entry struct {
	session api.Session
	adapter target.Adapter
}

func NewManager() *Manager {
	return &Manager{
		sessions: map[string]entry{},
	}
}

func (m *Manager) Attach(ctx context.Context, req api.AttachSessionRequest) (api.Session, error) {
	if req.SessionID == "" {
		return api.Session{}, errors.New("session_id is required")
	}
	if req.TargetType == "" {
		return api.Session{}, errors.New("target_type is required")
	}

	adapter, backendName, err := newAdapter(req)
	if err != nil {
		return api.Session{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[req.SessionID]; exists {
		return api.Session{}, fmt.Errorf("%w: %s", ErrSessionExists, req.SessionID)
	}

	cfg := api.AttachConfig{
		SessionID: req.SessionID,
		TargetRef: req.TargetRef,
		Options:   req.Options,
	}
	if err := adapter.Attach(ctx, cfg); err != nil {
		return api.Session{}, err
	}

	now := time.Now()
	session := api.Session{
		ID:         req.SessionID,
		TargetType: req.TargetType,
		TargetRef:  req.TargetRef,
		Backend:    backendName,
		Options:    cloneOptions(req.Options),
		CreatedAt:  now,
		LastUsedAt: now,
	}

	m.sessions[req.SessionID] = entry{
		session: session,
		adapter: adapter,
	}

	return session, nil
}

func (m *Manager) List() []api.Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]api.Session, 0, len(m.sessions))
	for _, entry := range m.sessions {
		out = append(out, entry.session)
	}

	slices.SortFunc(out, func(a, b api.Session) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})

	return out
}

func (m *Manager) Detach(ctx context.Context, sessionID string) (api.Session, error) {
	if sessionID == "" {
		return api.Session{}, errors.New("session_id is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.sessions[sessionID]
	if !ok {
		return api.Session{}, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	if err := entry.adapter.Detach(ctx); err != nil {
		return api.Session{}, err
	}

	delete(m.sessions, sessionID)
	return entry.session, nil
}

func (m *Manager) Observe(ctx context.Context, sessionID string, opts api.ObserveOptions) (api.Observation, error) {
	if sessionID == "" {
		return api.Observation{}, errors.New("session_id is required")
	}

	m.mu.Lock()
	entry, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return api.Observation{}, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}
	entry.session.LastUsedAt = time.Now()
	m.sessions[sessionID] = entry
	m.mu.Unlock()

	observation, err := entry.adapter.Observe(ctx, opts)
	if err != nil {
		return api.Observation{}, err
	}
	if observation == nil {
		return api.Observation{}, errors.New("empty observation")
	}

	observation.SessionID = entry.session.ID
	return *observation, nil
}

func (m *Manager) Act(ctx context.Context, sessionID string, action api.Action) (api.ActionResult, error) {
	if sessionID == "" {
		return api.ActionResult{}, errors.New("session_id is required")
	}

	m.mu.Lock()
	entry, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return api.ActionResult{}, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}
	entry.session.LastUsedAt = time.Now()
	m.sessions[sessionID] = entry
	m.mu.Unlock()

	result, err := entry.adapter.Act(ctx, action)
	if err != nil {
		return api.ActionResult{}, err
	}
	if result == nil {
		return api.ActionResult{}, errors.New("empty action result")
	}
	if result.OK {
		m.applyActionOptions(sessionID, action)
	}

	return *result, nil
}

func (m *Manager) applyActionOptions(sessionID string, action api.Action) {
	if action.Kind != "viewport" || action.Args == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.sessions[sessionID]
	if !ok {
		return
	}
	if entry.session.Options == nil {
		entry.session.Options = map[string]string{}
	}
	if width := strings.TrimSpace(action.Args["width"]); width != "" {
		entry.session.Options["viewport_width"] = width
	}
	if height := strings.TrimSpace(action.Args["height"]); height != "" {
		entry.session.Options["viewport_height"] = height
	}
	m.sessions[sessionID] = entry
}

func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	entries := make([]entry, 0, len(m.sessions))
	for _, entry := range m.sessions {
		entries = append(entries, entry)
	}
	m.sessions = map[string]entry{}
	m.mu.Unlock()

	for _, entry := range entries {
		if err := entry.adapter.Detach(ctx); err != nil {
			return err
		}
	}

	return nil
}

func newAdapter(req api.AttachSessionRequest) (target.Adapter, string, error) {
	switch req.TargetType {
	case "browser":
		backendName := spec.BackendChromium
		if req.Backend != "" {
			backendName = spec.BackendName(req.Backend)
		}

		backend, err := browser.NewBackend(backendName)
		if err != nil {
			return nil, "", err
		}

		return browser.NewAdapter(backend), string(backend.Name()), nil
	default:
		return nil, "", fmt.Errorf("unknown target type: %s", req.TargetType)
	}
}

func cloneOptions(options map[string]string) map[string]string {
	if len(options) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(options))
	for key, value := range options {
		cloned[key] = value
	}
	return cloned
}
