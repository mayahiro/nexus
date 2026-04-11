package api

import "time"

type Session struct {
	ID         string            `json:"id"`
	TargetType string            `json:"target_type"`
	TargetRef  string            `json:"target_ref,omitempty"`
	Backend    string            `json:"backend,omitempty"`
	Options    map[string]string `json:"options,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	LastUsedAt time.Time         `json:"last_used_at"`
}

type AttachConfig struct {
	SessionID string            `json:"session_id"`
	TargetRef string            `json:"target_ref"`
	Options   map[string]string `json:"options,omitempty"`
}

type AttachSessionRequest struct {
	TargetType string            `json:"target_type"`
	SessionID  string            `json:"session_id"`
	TargetRef  string            `json:"target_ref,omitempty"`
	Backend    string            `json:"backend,omitempty"`
	Options    map[string]string `json:"options,omitempty"`
}

type AttachSessionResponse struct {
	Session Session `json:"session"`
}

type ListSessionsRequest struct{}

type ListSessionsResponse struct {
	Sessions []Session `json:"sessions"`
}

type DetachSessionRequest struct {
	SessionID string `json:"session_id"`
}

type DetachSessionResponse struct {
	Session Session `json:"session"`
}

type StopDaemonRequest struct{}

type StopDaemonResponse struct {
	Stopped bool `json:"stopped"`
}

type ObserveSessionRequest struct {
	SessionID string         `json:"session_id"`
	Options   ObserveOptions `json:"options"`
}

type ObserveSessionResponse struct {
	Observation Observation `json:"observation"`
}

type ActSessionRequest struct {
	SessionID string `json:"session_id"`
	Action    Action `json:"action"`
}

type ActSessionResponse struct {
	Result ActionResult `json:"result"`
}

type ObserveOptions struct {
	WithText       bool `json:"with_text"`
	WithTree       bool `json:"with_tree"`
	WithScreenshot bool `json:"with_screenshot"`
	FullScreenshot bool `json:"full_screenshot"`
}

type LogOptions struct {
	Limit int `json:"limit,omitempty"`
}

type Observation struct {
	SessionID    string            `json:"session_id"`
	TargetType   string            `json:"target_type"`
	URLOrScreen  string            `json:"url_or_screen,omitempty"`
	Title        string            `json:"title,omitempty"`
	Text         string            `json:"text,omitempty"`
	Tree         []Node            `json:"tree,omitempty"`
	Screenshot   string            `json:"screenshot,omitempty"`
	Logs         []LogEntry        `json:"logs,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Meta         map[string]string `json:"meta,omitempty"`
}

type Node struct {
	ID          int               `json:"id"`
	Fingerprint string            `json:"fingerprint,omitempty"`
	Role        string            `json:"role"`
	Name        string            `json:"name,omitempty"`
	Text        string            `json:"text,omitempty"`
	Value       string            `json:"value,omitempty"`
	Bounds      Rect              `json:"bounds,omitempty"`
	Visible     bool              `json:"visible"`
	Enabled     bool              `json:"enabled"`
	Focused     bool              `json:"focused"`
	Editable    bool              `json:"editable"`
	Selectable  bool              `json:"selectable"`
	Invokable   bool              `json:"invokable"`
	Scrollable  bool              `json:"scrollable"`
	Children    []int             `json:"children,omitempty"`
	Attrs       map[string]string `json:"attrs,omitempty"`
}

type Action struct {
	Kind   string            `json:"kind"`
	NodeID *int              `json:"node_id,omitempty"`
	Text   string            `json:"text,omitempty"`
	Dir    string            `json:"dir,omitempty"`
	Keys   []string          `json:"keys,omitempty"`
	Args   map[string]string `json:"args,omitempty"`
}

type ActionResult struct {
	OK         bool              `json:"ok"`
	Message    string            `json:"message,omitempty"`
	Changed    bool              `json:"changed"`
	Screenshot string            `json:"screenshot,omitempty"`
	Value      interface{}       `json:"value,omitempty"`
	Meta       map[string]string `json:"meta,omitempty"`
}

type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}
