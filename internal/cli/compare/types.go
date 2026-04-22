package comparecmd

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var newCompareSessionSuffix = func() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}

type compareStringValues []string

func (v *compareStringValues) String() string {
	return strings.Join(*v, ", ")
}

func (v *compareStringValues) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return errors.New("compare value must not be empty")
	}
	*v = append(*v, trimmed)
	return nil
}

type compareEndpoint struct {
	SessionID string
	URL       string
}

type compareSnapshot struct {
	SessionID string                `json:"session_id,omitempty"`
	URL       string                `json:"url,omitempty"`
	Title     string                `json:"title,omitempty"`
	Text      string                `json:"text,omitempty"`
	Nodes     []compareSnapshotNode `json:"nodes,omitempty"`
}

type compareSnapshotNode struct {
	Fingerprint string            `json:"fingerprint"`
	Ref         string            `json:"ref,omitempty"`
	Role        string            `json:"role"`
	Label       string            `json:"label,omitempty"`
	Name        string            `json:"name,omitempty"`
	Text        string            `json:"text,omitempty"`
	Value       string            `json:"value,omitempty"`
	Href        string            `json:"href,omitempty"`
	TestID      string            `json:"testid,omitempty"`
	CSS         map[string]string `json:"css,omitempty"`
	Visible     bool              `json:"visible"`
	Enabled     bool              `json:"enabled"`
	Editable    bool              `json:"editable"`
	Selectable  bool              `json:"selectable"`
	Invokable   bool              `json:"invokable"`
}

type compareSummary struct {
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

type compareScopeSide struct {
	Matched bool   `json:"matched"`
	Tag     string `json:"tag,omitempty"`
}

type compareScope struct {
	Selector string           `json:"selector"`
	Old      compareScopeSide `json:"old"`
	New      compareScopeSide `json:"new"`
}

type compareFinding struct {
	Kind        string `json:"kind"`
	Severity    string `json:"severity,omitempty"`
	Impact      string `json:"impact,omitempty"`
	Locator     string `json:"locator,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Role        string `json:"role,omitempty"`
	Label       string `json:"label,omitempty"`
	Field       string `json:"field,omitempty"`
	Old         string `json:"old,omitempty"`
	New         string `json:"new,omitempty"`
}

type compareReport struct {
	Old      compareSnapshot  `json:"old"`
	New      compareSnapshot  `json:"new"`
	Scope    *compareScope    `json:"scope,omitempty"`
	Summary  compareSummary   `json:"summary"`
	Findings []compareFinding `json:"findings"`
}

type compareManifest struct {
	Defaults compareManifestDefaults `json:"defaults,omitempty"`
	Pages    []compareManifestPage   `json:"pages,omitempty"`
}

type compareManifestDefaults struct {
	Backend         string   `json:"backend,omitempty"`
	Viewport        string   `json:"viewport,omitempty"`
	WaitSelector    string   `json:"wait_selector,omitempty"`
	ScopeSelector   string   `json:"scope_selector,omitempty"`
	WaitFunction    string   `json:"wait_function,omitempty"`
	WaitNetworkIdle bool     `json:"wait_network_idle,omitempty"`
	CompareCSS      bool     `json:"compare_css,omitempty"`
	WaitTimeout     *int     `json:"wait_timeout,omitempty"`
	CSSProperty     []string `json:"css_property,omitempty"`
	IgnoreTextRegex []string `json:"ignore_text_regex,omitempty"`
	IgnoreSelector  []string `json:"ignore_selector,omitempty"`
	MaskSelector    []string `json:"mask_selector,omitempty"`
}

type compareManifestPage struct {
	Name            string   `json:"name,omitempty"`
	OldURL          string   `json:"old_url,omitempty"`
	NewURL          string   `json:"new_url,omitempty"`
	OldSession      string   `json:"old_session,omitempty"`
	NewSession      string   `json:"new_session,omitempty"`
	Backend         *string  `json:"backend,omitempty"`
	Viewport        *string  `json:"viewport,omitempty"`
	WaitSelector    *string  `json:"wait_selector,omitempty"`
	ScopeSelector   *string  `json:"scope_selector,omitempty"`
	WaitFunction    *string  `json:"wait_function,omitempty"`
	WaitNetworkIdle *bool    `json:"wait_network_idle,omitempty"`
	CompareCSS      *bool    `json:"compare_css,omitempty"`
	WaitTimeout     *int     `json:"wait_timeout,omitempty"`
	CSSProperty     []string `json:"css_property,omitempty"`
	IgnoreTextRegex []string `json:"ignore_text_regex,omitempty"`
	IgnoreSelector  []string `json:"ignore_selector,omitempty"`
	MaskSelector    []string `json:"mask_selector,omitempty"`
}

type compareManifestPageReport struct {
	Name   string         `json:"name"`
	Error  string         `json:"error,omitempty"`
	Report *compareReport `json:"report,omitempty"`
}

type compareManifestSummary struct {
	TotalPages     int `json:"total_pages"`
	ComparedPages  int `json:"compared_pages"`
	FailedPages    int `json:"failed_pages"`
	SamePages      int `json:"same_pages"`
	DifferentPages int `json:"different_pages"`
	TotalFindings  int `json:"total_findings"`
	Critical       int `json:"critical"`
	Warning        int `json:"warning"`
	Info           int `json:"info"`
}

type compareManifestReport struct {
	Manifest string                      `json:"manifest,omitempty"`
	Summary  compareManifestSummary      `json:"summary"`
	Pages    []compareManifestPageReport `json:"pages"`
}

type compareRun struct {
	OldEndpoint     compareEndpoint
	NewEndpoint     compareEndpoint
	Backend         string
	TargetRef       string
	Viewport        string
	WaitSelector    string
	ScopeSelector   string
	WaitFunction    string
	WaitNetworkIdle bool
	CompareCSS      bool
	WaitTimeout     int
	CSSProperties   []string
	IgnoreTextRegex []string
	IgnoreSelector  []string
	MaskSelector    []string
}

type preparedCompareSession struct {
	SessionID string
	Detach    bool
}

type compareSelectorRule struct {
	All []compareSelectorTerm
}

type compareSelectorTerm struct {
	Kind  string
	Value string
}

type compareSnapshotOptions struct {
	IgnoreText    []*regexp.Regexp
	IgnoreNode    []compareSelectorRule
	MaskNode      []compareSelectorRule
	CSSProperties []string
}

const compareURLReadyTimeout = 10 * time.Second
const compareNetworkIdleWindow = 500 * time.Millisecond
const defaultViewportWidth = 1920
const defaultViewportHeight = 1080

var DefaultCSSProperties = []string{
	"color",
	"background-color",
	"font-size",
	"font-weight",
	"line-height",
	"display",
	"visibility",
	"opacity",
	"pointer-events",
}
