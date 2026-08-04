package api

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

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

const (
	// StyleSourcesStatusComplete means the supported matched declaration data
	// and its available source metadata were collected.
	StyleSourcesStatusComplete = "complete"
	// StyleSourcesStatusPartial means declaration data was collected but some
	// stylesheet source metadata was unavailable.
	StyleSourcesStatusPartial = "partial"
	// StyleSourcesStatusUnavailable means matched declaration collection failed.
	StyleSourcesStatusUnavailable = "unavailable"
	// StyleSourcesStatusDisabled means matched declaration collection was skipped.
	StyleSourcesStatusDisabled = "disabled"
)

// InspectStylesRequest identifies one recently observed node and the computed
// CSS properties to inspect in its current document generation.
type InspectStylesRequest struct {
	SessionID     string   `json:"session_id"`
	NodeRef       string   `json:"node_ref"`
	CSSProperties []string `json:"css_properties"`
}

// InspectStylesResponse contains one targeted style inspection.
type InspectStylesResponse struct {
	Inspection StyleInspection `json:"inspection"`
}

// StyleInspection contains computed values and best-effort authored
// declarations for one node.
type StyleInspection struct {
	Computed           map[string]string         `json:"computed"`
	StyleSourcesStatus string                    `json:"style_sources_status"`
	StyleSourcesError  string                    `json:"style_sources_error,omitempty"`
	Properties         []StylePropertyInspection `json:"properties"`
}

// StylePropertyInspection groups authored declarations by requested computed
// property without claiming which declaration wins the cascade.
type StylePropertyInspection struct {
	Name         string             `json:"name"`
	Declarations []StyleDeclaration `json:"declarations"`
}

// StyleDeclaration describes one authored declaration reported by Chromium as
// directly matched, inline, attribute-derived, shorthand-derived, or inherited.
type StyleDeclaration struct {
	Property          string   `json:"property"`
	Value             string   `json:"value"`
	ResolvedValue     string   `json:"resolved_value,omitempty"`
	Text              string   `json:"text,omitempty"`
	Selector          string   `json:"selector,omitempty"`
	MatchingSelectors []string `json:"matching_selectors,omitempty"`
	Origin            string   `json:"origin,omitempty"`
	Relation          string   `json:"relation"`
	Important         bool     `json:"important,omitempty"`
	Disabled          bool     `json:"disabled,omitempty"`
	Implicit          bool     `json:"implicit,omitempty"`
	Inline            bool     `json:"inline,omitempty"`
	Attribute         bool     `json:"attribute,omitempty"`
	Inherited         bool     `json:"inherited,omitempty"`
	AncestorDepth     int      `json:"ancestor_depth,omitempty"`
	SourceURL         string   `json:"source_url,omitempty"`
	SourceMapURL      string   `json:"source_map_url,omitempty"`
	Line              int      `json:"line,omitempty"`
	Column            int      `json:"column,omitempty"`
}

type ActSessionRequest struct {
	SessionID string `json:"session_id"`
	Action    Action `json:"action"`
}

type ActSessionResponse struct {
	Result ActionResult `json:"result"`
}

type ObserveOptions struct {
	WithText          bool     `json:"with_text"`
	WithTree          bool     `json:"with_tree"`
	WithScreenshot    bool     `json:"with_screenshot"`
	FullScreenshot    bool     `json:"full_screenshot"`
	RecoverScreenshot bool     `json:"recover_screenshot,omitempty"`
	WithLayoutContext bool     `json:"with_layout_context,omitempty"`
	CSSProperties     []string `json:"css_properties,omitempty"`
	LayoutProperties  []string `json:"layout_properties,omitempty"`
	ScopeSelector     string   `json:"scope_selector,omitempty"`
	MatchSelector     string   `json:"match_selector,omitempty"`
	WithinRef         string   `json:"within_ref,omitempty"`
	ExcludeScopeRoot  bool     `json:"exclude_scope_root,omitempty"`
	NodeScope         string   `json:"node_scope,omitempty"`
	TimeoutMS         int      `json:"timeout_ms,omitempty"`
	Verbose           bool     `json:"verbose,omitempty"`
}

type Observation struct {
	SessionID      string            `json:"session_id"`
	TargetType     string            `json:"target_type"`
	URLOrScreen    string            `json:"url_or_screen,omitempty"`
	Title          string            `json:"title,omitempty"`
	Text           string            `json:"text,omitempty"`
	Tree           []Node            `json:"tree,omitempty"`
	Screenshot     string            `json:"screenshot,omitempty"`
	Capabilities   []string          `json:"capabilities,omitempty"`
	Meta           map[string]string `json:"meta,omitempty"`
	ScreenshotData []byte            `json:"-"`
}

func (o Observation) MarshalJSON() ([]byte, error) {
	type observation Observation

	value := observation(o)
	if value.Screenshot == "" && len(o.ScreenshotData) > 0 {
		value.Screenshot = base64.StdEncoding.EncodeToString(o.ScreenshotData)
	}
	return json.Marshal(value)
}

func (o Observation) ScreenshotBytes() ([]byte, error) {
	if len(o.ScreenshotData) > 0 {
		return o.ScreenshotData, nil
	}
	if o.Screenshot == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(o.Screenshot)
}

type Node struct {
	ID             int                 `json:"id"`
	Ref            string              `json:"ref,omitempty"`
	Fingerprint    string              `json:"fingerprint,omitempty"`
	LocatorHints   []LocatorHint       `json:"locator_hints,omitempty"`
	StructurePath  string              `json:"structure_path,omitempty"`
	Selector       string              `json:"selector,omitempty"`
	TextLength     int                 `json:"text_length,omitempty"`
	Descendants    int                 `json:"descendants,omitempty"`
	Role           string              `json:"role"`
	Name           string              `json:"name,omitempty"`
	Text           string              `json:"text,omitempty"`
	Value          string              `json:"value,omitempty"`
	Styles         map[string]string   `json:"styles,omitempty"`
	LayoutContext  []LayoutContextNode `json:"layout_context,omitempty"`
	Bounds         Rect                `json:"bounds,omitempty"`
	DocumentBounds *Rect               `json:"document_bounds,omitempty"`
	Visible        bool                `json:"visible"`
	Enabled        bool                `json:"enabled"`
	Focused        bool                `json:"focused"`
	Editable       bool                `json:"editable"`
	Selectable     bool                `json:"selectable"`
	Invokable      bool                `json:"invokable"`
	Scrollable     bool                `json:"scrollable"`
	Children       []int               `json:"children,omitempty"`
	Attrs          map[string]string   `json:"attrs,omitempty"`
}

type LayoutContextNode struct {
	Selector   string            `json:"selector,omitempty"`
	Role       string            `json:"role,omitempty"`
	Name       string            `json:"name,omitempty"`
	Styles     map[string]string `json:"styles,omitempty"`
	Bounds     Rect              `json:"bounds,omitempty"`
	Scrollable bool              `json:"scrollable,omitempty"`
	Attrs      map[string]string `json:"attrs,omitempty"`
}

type LocatorHint struct {
	Kind    string `json:"kind"`
	Value   string `json:"value"`
	Name    string `json:"name,omitempty"`
	Command string `json:"command"`
}

type Action struct {
	Kind     string            `json:"kind"`
	NodeID   *int              `json:"node_id,omitempty"`
	NodeRef  string            `json:"node_ref,omitempty"`
	Selector string            `json:"selector,omitempty"`
	Text     string            `json:"text,omitempty"`
	Dir      string            `json:"dir,omitempty"`
	Keys     []string          `json:"keys,omitempty"`
	Args     map[string]string `json:"args,omitempty"`
}

type ActionResult struct {
	OK         bool              `json:"ok"`
	Message    string            `json:"message,omitempty"`
	Changed    bool              `json:"changed"`
	Screenshot string            `json:"screenshot,omitempty"`
	Value      interface{}       `json:"value"`
	Meta       map[string]string `json:"meta,omitempty"`
}

type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}
