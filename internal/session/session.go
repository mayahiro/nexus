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
	"github.com/mayahiro/nexus/internal/diagnostic"
	"github.com/mayahiro/nexus/internal/target"
	"github.com/mayahiro/nexus/internal/target/browser"
	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

var (
	ErrSessionExists   = errors.New("session already exists")
	ErrSessionNotFound = errors.New("session not found")
)

type Manager struct {
	mu           sync.RWMutex
	sessions     map[string]*entry
	pending      map[string]*pendingAttach
	shuttingDown bool
}

type pendingAttach struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type entry struct {
	session api.Session
	adapter target.Adapter
	opGate  chan struct{}
	closed  bool
}

func NewManager() *Manager {
	return &Manager{
		sessions: map[string]*entry{},
		pending:  map[string]*pendingAttach{},
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
	if m.shuttingDown {
		m.mu.Unlock()
		return api.Session{}, errors.New("session manager is shutting down")
	}
	if m.sessions[req.SessionID] != nil || m.pending[req.SessionID] != nil {
		m.mu.Unlock()
		return api.Session{}, fmt.Errorf("%w: %s", ErrSessionExists, req.SessionID)
	}
	attachCtx, cancel := context.WithCancel(ctx)
	pending := &pendingAttach{cancel: cancel, done: make(chan struct{})}
	m.pending[req.SessionID] = pending
	m.mu.Unlock()
	defer func() {
		cancel()
		m.mu.Lock()
		delete(m.pending, req.SessionID)
		close(pending.done)
		m.mu.Unlock()
	}()

	cfg := api.AttachConfig{
		SessionID: req.SessionID,
		TargetRef: req.TargetRef,
		Options:   cloneOptions(req.Options),
	}
	trace := diagnostic.FromContext(ctx)
	backendStartedAt := time.Now()
	trace.Event("session_backend", "attach_start",
		diagnostic.Value("session", req.SessionID),
		diagnostic.Value("backend", backendName),
	)
	attachErr := adapter.Attach(attachCtx, cfg)
	if attachErr != nil {
		trace.Event("session_backend", "attach_finish",
			diagnostic.Value("session", req.SessionID),
			diagnostic.Value("backend", backendName),
			diagnostic.Value("duration_ms", time.Since(backendStartedAt).Milliseconds()),
			diagnostic.Value("error", attachErr),
		)
	} else {
		trace.Event("session_backend", "attach_finish",
			diagnostic.Value("session", req.SessionID),
			diagnostic.Value("backend", backendName),
			diagnostic.Value("duration_ms", time.Since(backendStartedAt).Milliseconds()),
		)
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

	sessionEntry := &entry{
		session: session,
		adapter: adapter,
		opGate:  make(chan struct{}, 1),
	}
	sessionEntry.opGate <- struct{}{}
	m.mu.Lock()
	if attachErr == nil {
		attachErr = attachCtx.Err()
	}
	if attachErr == nil && m.shuttingDown {
		attachErr = errors.New("session manager is shutting down")
	}
	if attachErr == nil {
		m.sessions[req.SessionID] = sessionEntry
		session.Options = cloneOptions(session.Options)
		m.mu.Unlock()
		return session, nil
	}
	m.mu.Unlock()

	// A canceled startup can still have created a browser. Cleanup must not
	// inherit its canceled context, and failed cleanup remains retryable.
	cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cleanupCancel()
	if cleanupErr := adapter.Detach(cleanupCtx); cleanupErr != nil {
		m.mu.Lock()
		m.sessions[req.SessionID] = sessionEntry
		m.mu.Unlock()
		return api.Session{}, errors.Join(attachErr, fmt.Errorf("cleanup session %s: %w", req.SessionID, cleanupErr))
	}

	return api.Session{}, attachErr
}

func (m *Manager) List() []api.Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]api.Session, 0, len(m.sessions))
	for _, entry := range m.sessions {
		session := entry.session
		session.Options = cloneOptions(session.Options)
		out = append(out, session)
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

	m.mu.RLock()
	sessionEntry, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return api.Session{}, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	if err := sessionEntry.acquire(ctx); err != nil {
		return api.Session{}, err
	}
	defer sessionEntry.release()

	if sessionEntry.closed {
		return api.Session{}, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}
	trace := diagnostic.FromContext(ctx)
	backendStartedAt := time.Now()
	trace.Event("session_backend", "detach_start", diagnostic.Value("session", sessionID))
	if err := sessionEntry.adapter.Detach(ctx); err != nil {
		trace.Event("session_backend", "detach_finish",
			diagnostic.Value("session", sessionID),
			diagnostic.Value("duration_ms", time.Since(backendStartedAt).Milliseconds()),
			diagnostic.Value("error", err),
		)
		return api.Session{}, err
	}
	trace.Event("session_backend", "detach_finish",
		diagnostic.Value("session", sessionID),
		diagnostic.Value("duration_ms", time.Since(backendStartedAt).Milliseconds()),
	)

	m.mu.Lock()
	if current, exists := m.sessions[sessionID]; exists && current == sessionEntry {
		delete(m.sessions, sessionID)
	}
	sessionEntry.closed = true
	m.mu.Unlock()

	return sessionEntry.session, nil
}

func (m *Manager) Observe(ctx context.Context, sessionID string, opts api.ObserveOptions) (api.Observation, error) {
	if sessionID == "" {
		return api.Observation{}, errors.New("session_id is required")
	}

	m.mu.RLock()
	sessionEntry, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return api.Observation{}, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	if err := sessionEntry.acquire(ctx); err != nil {
		return api.Observation{}, err
	}
	defer sessionEntry.release()

	if sessionEntry.closed {
		return api.Observation{}, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}
	m.touch(sessionID, sessionEntry)

	trace := diagnostic.FromContext(ctx)
	backendStartedAt := time.Now()
	trace.Event("session_backend", "observe_start", diagnostic.Value("session", sessionID))
	observation, err := sessionEntry.adapter.Observe(ctx, opts)
	if err != nil {
		trace.Event("session_backend", "observe_finish",
			diagnostic.Value("session", sessionID),
			diagnostic.Value("duration_ms", time.Since(backendStartedAt).Milliseconds()),
			diagnostic.Value("error", err),
		)
		return api.Observation{}, err
	}
	trace.Event("session_backend", "observe_finish",
		diagnostic.Value("session", sessionID),
		diagnostic.Value("duration_ms", time.Since(backendStartedAt).Milliseconds()),
	)
	if observation == nil {
		return api.Observation{}, errors.New("empty observation")
	}

	observation.SessionID = sessionEntry.session.ID
	return *observation, nil
}

// InspectStyles performs one targeted style inspection under the same
// per-session operation gate used by observe and act operations.
func (m *Manager) InspectStyles(ctx context.Context, req api.InspectStylesRequest) (api.StyleInspection, error) {
	if req.SessionID == "" {
		return api.StyleInspection{}, errors.New("session_id is required")
	}
	if strings.TrimSpace(req.NodeRef) == "" {
		return api.StyleInspection{}, errors.New("node_ref is required")
	}

	m.mu.RLock()
	sessionEntry, ok := m.sessions[req.SessionID]
	m.mu.RUnlock()
	if !ok {
		return api.StyleInspection{}, fmt.Errorf("%w: %s", ErrSessionNotFound, req.SessionID)
	}

	if err := sessionEntry.acquire(ctx); err != nil {
		return api.StyleInspection{}, err
	}
	defer sessionEntry.release()

	if sessionEntry.closed {
		return api.StyleInspection{}, fmt.Errorf("%w: %s", ErrSessionNotFound, req.SessionID)
	}
	m.touch(req.SessionID, sessionEntry)

	inspector, ok := sessionEntry.adapter.(target.StyleInspector)
	if !ok {
		return api.StyleInspection{}, fmt.Errorf("%w: style-inspection", spec.ErrUnsupported)
	}

	trace := diagnostic.FromContext(ctx)
	backendStartedAt := time.Now()
	trace.Event("session_backend", "inspect_styles_start", diagnostic.Value("session", req.SessionID))
	inspection, err := inspector.InspectStyles(ctx, req)
	if err != nil {
		trace.Event("session_backend", "inspect_styles_finish",
			diagnostic.Value("session", req.SessionID),
			diagnostic.Value("duration_ms", time.Since(backendStartedAt).Milliseconds()),
			diagnostic.Value("error", err),
		)
		return api.StyleInspection{}, err
	}
	trace.Event("session_backend", "inspect_styles_finish",
		diagnostic.Value("session", req.SessionID),
		diagnostic.Value("duration_ms", time.Since(backendStartedAt).Milliseconds()),
	)
	if inspection == nil {
		return api.StyleInspection{}, errors.New("empty style inspection")
	}
	return *inspection, nil
}

func (m *Manager) Act(ctx context.Context, sessionID string, action api.Action) (api.ActionResult, error) {
	if sessionID == "" {
		return api.ActionResult{}, errors.New("session_id is required")
	}

	m.mu.RLock()
	sessionEntry, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return api.ActionResult{}, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	if err := sessionEntry.acquire(ctx); err != nil {
		return api.ActionResult{}, err
	}
	defer sessionEntry.release()

	if sessionEntry.closed {
		return api.ActionResult{}, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}
	m.touch(sessionID, sessionEntry)

	trace := diagnostic.FromContext(ctx)
	backendStartedAt := time.Now()
	trace.Event("session_backend", "act_start",
		diagnostic.Value("session", sessionID),
		diagnostic.Value("action", action.Kind),
	)
	result, err := sessionEntry.adapter.Act(ctx, action)
	if err != nil {
		trace.Event("session_backend", "act_finish",
			diagnostic.Value("session", sessionID),
			diagnostic.Value("action", action.Kind),
			diagnostic.Value("duration_ms", time.Since(backendStartedAt).Milliseconds()),
			diagnostic.Value("error", err),
		)
		return api.ActionResult{}, err
	}
	trace.Event("session_backend", "act_finish",
		diagnostic.Value("session", sessionID),
		diagnostic.Value("action", action.Kind),
		diagnostic.Value("duration_ms", time.Since(backendStartedAt).Milliseconds()),
	)
	if result == nil {
		return api.ActionResult{}, errors.New("empty action result")
	}
	if result.OK {
		m.applyActionOptions(sessionID, action)
	}

	return *result, nil
}

func (m *Manager) touch(sessionID string, sessionEntry *entry) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if current, ok := m.sessions[sessionID]; ok && current == sessionEntry {
		sessionEntry.session.LastUsedAt = time.Now()
	}
}

func (e *entry) acquire(ctx context.Context) error {
	trace := diagnostic.FromContext(ctx)
	startedAt := time.Now()
	trace.Event("session_gate", "wait")
	select {
	case <-ctx.Done():
		trace.Event("session_gate", "canceled",
			diagnostic.Value("wait_ms", time.Since(startedAt).Milliseconds()),
			diagnostic.Value("error", ctx.Err()),
		)
		return ctx.Err()
	case <-e.opGate:
		if err := ctx.Err(); err != nil {
			e.release()
			trace.Event("session_gate", "canceled", diagnostic.Value("error", err))
			return err
		}
		trace.Event("session_gate", "acquired",
			diagnostic.Value("wait_ms", time.Since(startedAt).Milliseconds()),
		)
		return nil
	}
}

func (e *entry) release() {
	e.opGate <- struct{}{}
}

func (m *Manager) applyActionOptions(sessionID string, action api.Action) {
	if action.Kind != "viewport" || action.Args == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sessionEntry, ok := m.sessions[sessionID]
	if !ok {
		return
	}
	if sessionEntry.session.Options == nil {
		sessionEntry.session.Options = map[string]string{}
	}
	if width := strings.TrimSpace(action.Args["width"]); width != "" {
		sessionEntry.session.Options["viewport_width"] = width
	}
	if height := strings.TrimSpace(action.Args["height"]); height != "" {
		sessionEntry.session.Options["viewport_height"] = height
	}
}

// Shutdown cancels pending startups and attempts to detach every active session.
// Sessions whose cleanup fails remain registered so a later call can retry.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	m.shuttingDown = true
	pending := make([]*pendingAttach, 0, len(m.pending))
	for _, startup := range m.pending {
		pending = append(pending, startup)
	}
	m.mu.Unlock()

	for _, startup := range pending {
		startup.cancel()
	}
	var errs []error
	for _, startup := range pending {
		select {
		case <-startup.done:
		case <-ctx.Done():
			errs = append(errs, ctx.Err())
		}
	}

	sessions := m.List()
	results := make(chan error, len(sessions))
	for _, session := range sessions {
		go func() {
			_, err := m.Detach(ctx, session.ID)
			if errors.Is(err, ErrSessionNotFound) {
				err = nil // Another detach already completed cleanup.
			}
			if err != nil {
				err = fmt.Errorf("detach session %s: %w", session.ID, err)
			}
			results <- err
		}()
	}
	for range sessions {
		if err := <-results; err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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
