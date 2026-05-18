package comparecmd

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mayahiro/nexus/internal/api"
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
	SessionID       string                `json:"session_id,omitempty"`
	URL             string                `json:"url,omitempty"`
	Title           string                `json:"title,omitempty"`
	Text            string                `json:"text,omitempty"`
	Nodes           []compareSnapshotNode `json:"nodes,omitempty"`
	ReferenceBounds *api.Rect             `json:"-"`
}

type compareSnapshotNode struct {
	Fingerprint      string            `json:"fingerprint"`
	StructureKey     string            `json:"structure_key,omitempty"`
	SubtreeSignature string            `json:"subtree_signature,omitempty"`
	Ref              string            `json:"ref,omitempty"`
	Role             string            `json:"role"`
	Label            string            `json:"label,omitempty"`
	Name             string            `json:"name,omitempty"`
	Text             string            `json:"text,omitempty"`
	Value            string            `json:"value,omitempty"`
	Href             string            `json:"href,omitempty"`
	TestID           string            `json:"testid,omitempty"`
	CSS              map[string]string `json:"css,omitempty"`
	Bounds           *api.Rect         `json:"bounds,omitempty"`
	Visible          bool              `json:"visible"`
	Enabled          bool              `json:"enabled"`
	Editable         bool              `json:"editable"`
	Selectable       bool              `json:"selectable"`
	Invokable        bool              `json:"invokable"`
	ID               int               `json:"id,omitempty"`
	Children         []int             `json:"children,omitempty"`
	OriginalIndex    int               `json:"-"`
	Tag              string            `json:"-"`
	IDAttr           string            `json:"-"`
	NameAttr         string            `json:"-"`
	TypeAttr         string            `json:"-"`
	Placeholder      string            `json:"-"`
	AriaLabel        string            `json:"-"`
	MatchBounds      *api.Rect         `json:"-"`
	CropBounds       *api.Rect         `json:"-"`
}

type compareSummary struct {
	Same                    bool `json:"same"`
	TotalFindings           int  `json:"total_findings"`
	TitleChanged            int  `json:"title_changed"`
	TextChanged             int  `json:"text_changed"`
	MissingNodes            int  `json:"missing_nodes"`
	NewNodes                int  `json:"new_nodes"`
	StateChanged            int  `json:"state_changed"`
	CSSChanged              int  `json:"css_changed"`
	LayoutChanged           int  `json:"layout_changed"`
	PageTextChanged         int  `json:"page_text_changed"`
	MatchedNodes            int  `json:"matched_nodes,omitempty"`
	ExactMatches            int  `json:"exact_matches,omitempty"`
	StableMatches           int  `json:"stable_matches,omitempty"`
	HeuristicMatches        int  `json:"heuristic_matches,omitempty"`
	HistogramMatches        int  `json:"histogram_matches,omitempty"`
	DecisionMatches         int  `json:"decision_matches,omitempty"`
	AmbiguousMatchesSkipped int  `json:"ambiguous_matches_skipped,omitempty"`
	Critical                int  `json:"critical"`
	Warning                 int  `json:"warning"`
	Info                    int  `json:"info"`
}

type compareScopeSide struct {
	Selector string `json:"selector,omitempty"`
	Matched  bool   `json:"matched"`
	Tag      string `json:"tag,omitempty"`
}

type compareScope struct {
	Selector string           `json:"selector,omitempty"`
	Old      compareScopeSide `json:"old"`
	New      compareScopeSide `json:"new"`
}

type compareFinding struct {
	Kind             string   `json:"kind"`
	FindingID        string   `json:"finding_id,omitempty"`
	Severity         string   `json:"severity,omitempty"`
	Impact           string   `json:"impact,omitempty"`
	DecisionKind     string   `json:"decision_kind,omitempty"`
	Locator          string   `json:"locator,omitempty"`
	Fingerprint      string   `json:"fingerprint,omitempty"`
	StructureKey     string   `json:"structure_key,omitempty"`
	SubtreeSignature string   `json:"subtree_signature,omitempty"`
	Role             string   `json:"role,omitempty"`
	Label            string   `json:"label,omitempty"`
	Field            string   `json:"field,omitempty"`
	Old              string   `json:"old,omitempty"`
	New              string   `json:"new,omitempty"`
	MatchedBy        string   `json:"matched_by,omitempty"`
	MatchScore       int      `json:"match_score,omitempty"`
	MatchReasons     []string `json:"match_reasons,omitempty"`
}

type compareDecisionValidationSummary struct {
	TotalDecisions     int  `json:"total_decisions"`
	HighPairs          int  `json:"high_pairs"`
	TentativePairs     int  `json:"tentative_pairs"`
	UnknownPairs       int  `json:"unknown_pairs"`
	SubtreePairs       int  `json:"subtree_pairs"`
	AcceptedRemoved    int  `json:"accepted_removed"`
	AcceptedAdded      int  `json:"accepted_added"`
	AcceptedFindings   int  `json:"accepted_findings"`
	RegressionFindings int  `json:"regression_findings"`
	Errors             int  `json:"errors"`
	Warnings           int  `json:"warnings"`
	CompareJSONUsed    bool `json:"compare_json_used"`
}

type compareDecisionValidationIssue struct {
	Severity string `json:"severity"`
	Line     int    `json:"line,omitempty"`
	Field    string `json:"field,omitempty"`
	Message  string `json:"message"`
}

type compareDecisionValidationReport struct {
	Summary compareDecisionValidationSummary `json:"summary"`
	Issues  []compareDecisionValidationIssue `json:"issues,omitempty"`
}

type compareDecisionNormalizeSummary struct {
	InputDecisions    int    `json:"input_decisions"`
	OutputDecisions   int    `json:"output_decisions"`
	DuplicatesRemoved int    `json:"duplicates_removed"`
	Output            string `json:"output,omitempty"`
	Errors            int    `json:"errors"`
	Warnings          int    `json:"warnings"`
	CompareJSONUsed   bool   `json:"compare_json_used"`
	ReviewSummaryUsed bool   `json:"review_summary_used,omitempty"`
}

type compareDecisionNormalizeReport struct {
	Summary compareDecisionNormalizeSummary  `json:"summary"`
	Issues  []compareDecisionValidationIssue `json:"issues,omitempty"`
}

type compareDecisionAuditSummary struct {
	TotalDecisions  int  `json:"total_decisions"`
	Applied         int  `json:"applied"`
	Pending         int  `json:"pending"`
	Stale           int  `json:"stale"`
	Conflicts       int  `json:"conflicts"`
	Errors          int  `json:"errors"`
	Warnings        int  `json:"warnings"`
	CompareJSONUsed bool `json:"compare_json_used"`
}

type compareDecisionAuditReport struct {
	Summary compareDecisionAuditSummary      `json:"summary"`
	Issues  []compareDecisionValidationIssue `json:"issues,omitempty"`
}

type compareReviewFiles struct {
	CompareJSON              string `json:"compare_json"`
	CompareMarkdown          string `json:"compare_markdown"`
	PairDecisionsTemplate    string `json:"pair_decisions_template"`
	FindingDecisionsTemplate string `json:"finding_decisions_template"`
	OldScreenshot            string `json:"old_screenshot,omitempty"`
	NewScreenshot            string `json:"new_screenshot,omitempty"`
	FindingScreenshotsDir    string `json:"finding_screenshots_dir,omitempty"`
	ReviewSummary            string `json:"review_summary"`
}

type compareReviewSummary struct {
	Old                     string                  `json:"old,omitempty"`
	New                     string                  `json:"new,omitempty"`
	Scope                   string                  `json:"scope,omitempty"`
	Same                    bool                    `json:"same"`
	TotalFindings           int                     `json:"total_findings"`
	CriticalFindings        int                     `json:"critical_findings"`
	WarningFindings         int                     `json:"warning_findings"`
	InfoFindings            int                     `json:"info_findings"`
	MatchedNodes            int                     `json:"matched_nodes,omitempty"`
	AmbiguousMatchesSkipped int                     `json:"ambiguous_matches_skipped,omitempty"`
	AmbiguousCandidates     int                     `json:"ambiguous_candidates,omitempty"`
	UnmatchedOld            int                     `json:"unmatched_old,omitempty"`
	UnmatchedNew            int                     `json:"unmatched_new,omitempty"`
	Files                   compareReviewFiles      `json:"files"`
	FindingClusters         []compareFindingCluster `json:"finding_clusters,omitempty"`
	ScreenshotWarnings      []string                `json:"screenshot_warnings,omitempty"`
	CropWarnings            []string                `json:"crop_warnings,omitempty"`
	NextCommands            []string                `json:"next_commands,omitempty"`
}

type compareFindingCluster struct {
	Key              string   `json:"key"`
	Count            int      `json:"count"`
	Severity         string   `json:"severity,omitempty"`
	Kind             string   `json:"kind,omitempty"`
	Impact           string   `json:"impact,omitempty"`
	DecisionKind     string   `json:"decision_kind,omitempty"`
	Field            string   `json:"field,omitempty"`
	Role             string   `json:"role,omitempty"`
	Label            string   `json:"label,omitempty"`
	Old              string   `json:"old,omitempty"`
	New              string   `json:"new,omitempty"`
	ExampleFindingID string   `json:"example_finding_id,omitempty"`
	FindingIDs       []string `json:"finding_ids,omitempty"`
	MoreFindingIDs   int      `json:"more_finding_ids,omitempty"`
	Pages            []string `json:"pages,omitempty"`
}

type compareManifestReviewPageDirectory struct {
	Name             string `json:"name"`
	Directory        string `json:"directory,omitempty"`
	Priority         string `json:"priority,omitempty"`
	TotalFindings    int    `json:"total_findings,omitempty"`
	CriticalFindings int    `json:"critical_findings,omitempty"`
	WarningFindings  int    `json:"warning_findings,omitempty"`
	InfoFindings     int    `json:"info_findings,omitempty"`
	OldScreenshot    string `json:"old_screenshot,omitempty"`
	NewScreenshot    string `json:"new_screenshot,omitempty"`
	Error            string `json:"error,omitempty"`
}

type compareManifestReviewFiles struct {
	ManifestJSON     string                               `json:"manifest_json"`
	ManifestMarkdown string                               `json:"manifest_markdown"`
	ReviewIndex      string                               `json:"review_index"`
	ReviewIndexHTML  string                               `json:"review_index_html"`
	ReviewSummary    string                               `json:"review_summary"`
	PageDirectories  []compareManifestReviewPageDirectory `json:"page_directories,omitempty"`
}

type compareManifestReviewSummary struct {
	Manifest         string                     `json:"manifest,omitempty"`
	TotalPages       int                        `json:"total_pages"`
	ComparedPages    int                        `json:"compared_pages"`
	FailedPages      int                        `json:"failed_pages"`
	SamePages        int                        `json:"same_pages"`
	DifferentPages   int                        `json:"different_pages"`
	TotalFindings    int                        `json:"total_findings"`
	CriticalFindings int                        `json:"critical_findings"`
	WarningFindings  int                        `json:"warning_findings"`
	InfoFindings     int                        `json:"info_findings"`
	Files            compareManifestReviewFiles `json:"files"`
	FindingClusters  []compareFindingCluster    `json:"finding_clusters,omitempty"`
}

type compareMatchingDebug struct {
	Mode                    string                                   `json:"mode"`
	OldNodes                int                                      `json:"old_nodes"`
	NewNodes                int                                      `json:"new_nodes"`
	MatchedNodes            int                                      `json:"matched_nodes"`
	AmbiguousMatchesSkipped int                                      `json:"ambiguous_matches_skipped,omitempty"`
	Matches                 []compareMatchingDebugMatch              `json:"matches,omitempty"`
	Anchors                 []compareMatchingDebugAnchor             `json:"anchors,omitempty"`
	Regions                 []compareMatchingDebugRegion             `json:"regions,omitempty"`
	AmbiguousCandidates     []compareMatchingDebugAmbiguousCandidate `json:"ambiguous_candidates,omitempty"`
	UnmatchedOld            []compareMatchingDebugNode               `json:"unmatched_old,omitempty"`
	UnmatchedNew            []compareMatchingDebugNode               `json:"unmatched_new,omitempty"`
}

type compareMatchingDebugNode struct {
	Index            int       `json:"index"`
	OriginalIndex    int       `json:"original_index"`
	Ref              string    `json:"ref,omitempty"`
	Locator          string    `json:"locator,omitempty"`
	Role             string    `json:"role,omitempty"`
	Label            string    `json:"label,omitempty"`
	Name             string    `json:"name,omitempty"`
	Text             string    `json:"text,omitempty"`
	Href             string    `json:"href,omitempty"`
	TestID           string    `json:"testid,omitempty"`
	AriaLabel        string    `json:"aria_label,omitempty"`
	Fingerprint      string    `json:"fingerprint,omitempty"`
	StructureKey     string    `json:"structure_key,omitempty"`
	SubtreeSignature string    `json:"subtree_signature,omitempty"`
	Bounds           *api.Rect `json:"bounds,omitempty"`
}

type compareMatchingDebugAnchor struct {
	Old       compareMatchingDebugNode `json:"old"`
	New       compareMatchingDebugNode `json:"new"`
	KeyKind   string                   `json:"key_kind"`
	KeyValue  string                   `json:"key_value"`
	MatchedBy string                   `json:"matched_by"`
	Reasons   []string                 `json:"reasons,omitempty"`
}

type compareMatchingDebugMatch struct {
	Old       compareMatchingDebugNode `json:"old"`
	New       compareMatchingDebugNode `json:"new"`
	MatchedBy string                   `json:"matched_by,omitempty"`
	Score     int                      `json:"score,omitempty"`
	Reasons   []string                 `json:"reasons,omitempty"`
}

type compareMatchingDebugRegion struct {
	Index                 int `json:"index"`
	OldStartOriginalIndex int `json:"old_start_original_index"`
	OldEndOriginalIndex   int `json:"old_end_original_index"`
	OldNodeCount          int `json:"old_node_count"`
	NewStartOriginalIndex int `json:"new_start_original_index"`
	NewEndOriginalIndex   int `json:"new_end_original_index"`
	NewNodeCount          int `json:"new_node_count"`
	ExactMatches          int `json:"exact_matches,omitempty"`
	HeuristicMatches      int `json:"heuristic_matches,omitempty"`
	AmbiguousSkipped      int `json:"ambiguous_skipped,omitempty"`
}

type compareMatchingDebugAmbiguousCandidate struct {
	Old           compareMatchingDebugNode              `json:"old"`
	NewCandidates []compareMatchingDebugCandidateOption `json:"new_candidates,omitempty"`
	Source        string                                `json:"source,omitempty"`
	KeyKind       string                                `json:"key_kind,omitempty"`
	KeyValue      string                                `json:"key_value,omitempty"`
	ReasonSkipped string                                `json:"reason_skipped,omitempty"`
}

type compareMatchingDebugCandidateOption struct {
	Node          compareMatchingDebugNode `json:"node"`
	Score         int                      `json:"score,omitempty"`
	Reasons       []string                 `json:"reasons,omitempty"`
	SharedKeys    []string                 `json:"shared_keys,omitempty"`
	DifferingKeys []string                 `json:"differing_keys,omitempty"`
}

type compareReport struct {
	Old           compareSnapshot       `json:"old"`
	New           compareSnapshot       `json:"new"`
	Scope         *compareScope         `json:"scope,omitempty"`
	Summary       compareSummary        `json:"summary"`
	Findings      []compareFinding      `json:"findings"`
	MatchingDebug *compareMatchingDebug `json:"matching_debug,omitempty"`
}

type compareManifest struct {
	Defaults compareManifestDefaults `json:"defaults,omitempty"`
	Pages    []compareManifestPage   `json:"pages,omitempty"`
}

type compareManifestDefaults struct {
	Backend          string   `json:"backend,omitempty"`
	Viewport         string   `json:"viewport,omitempty"`
	MatchMode        string   `json:"match_mode,omitempty"`
	NodeScope        string   `json:"node_scope,omitempty"`
	MatchingDebug    bool     `json:"matching_debug,omitempty"`
	DecisionsFile    string   `json:"decisions_file,omitempty"`
	WaitSelector     string   `json:"wait_selector,omitempty"`
	ScopeSelector    string   `json:"scope_selector,omitempty"`
	OldScopeSelector string   `json:"old_scope_selector,omitempty"`
	NewScopeSelector string   `json:"new_scope_selector,omitempty"`
	WaitFunction     string   `json:"wait_function,omitempty"`
	WaitNetworkIdle  bool     `json:"wait_network_idle,omitempty"`
	CompareCSS       bool     `json:"compare_css,omitempty"`
	CompareLayout    bool     `json:"compare_layout,omitempty"`
	WaitTimeout      *int     `json:"wait_timeout,omitempty"`
	CSSProperty      []string `json:"css_property,omitempty"`
	IgnoreTextRegex  []string `json:"ignore_text_regex,omitempty"`
	IgnoreSelector   []string `json:"ignore_selector,omitempty"`
	MaskSelector     []string `json:"mask_selector,omitempty"`
}

type compareManifestPage struct {
	Name             string   `json:"name,omitempty"`
	OldURL           string   `json:"old_url,omitempty"`
	NewURL           string   `json:"new_url,omitempty"`
	OldSession       string   `json:"old_session,omitempty"`
	NewSession       string   `json:"new_session,omitempty"`
	Backend          *string  `json:"backend,omitempty"`
	Viewport         *string  `json:"viewport,omitempty"`
	MatchMode        *string  `json:"match_mode,omitempty"`
	NodeScope        *string  `json:"node_scope,omitempty"`
	MatchingDebug    *bool    `json:"matching_debug,omitempty"`
	DecisionsFile    *string  `json:"decisions_file,omitempty"`
	WaitSelector     *string  `json:"wait_selector,omitempty"`
	ScopeSelector    *string  `json:"scope_selector,omitempty"`
	OldScopeSelector *string  `json:"old_scope_selector,omitempty"`
	NewScopeSelector *string  `json:"new_scope_selector,omitempty"`
	WaitFunction     *string  `json:"wait_function,omitempty"`
	WaitNetworkIdle  *bool    `json:"wait_network_idle,omitempty"`
	CompareCSS       *bool    `json:"compare_css,omitempty"`
	CompareLayout    *bool    `json:"compare_layout,omitempty"`
	WaitTimeout      *int     `json:"wait_timeout,omitempty"`
	CSSProperty      []string `json:"css_property,omitempty"`
	IgnoreTextRegex  []string `json:"ignore_text_regex,omitempty"`
	IgnoreSelector   []string `json:"ignore_selector,omitempty"`
	MaskSelector     []string `json:"mask_selector,omitempty"`
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
	OldEndpoint             compareEndpoint
	NewEndpoint             compareEndpoint
	Backend                 string
	TargetRef               string
	Viewport                string
	MatchMode               string
	NodeScope               string
	MatchingDebug           bool
	DecisionsFile           string
	OutputDecisionsTemplate string
	ReviewDir               string
	WaitSelector            string
	ScopeSelector           string
	OldScopeSelector        string
	NewScopeSelector        string
	WaitFunction            string
	WaitNetworkIdle         bool
	CompareCSS              bool
	CompareLayout           bool
	WaitTimeout             int
	CSSProperties           []string
	IgnoreTextRegex         []string
	IgnoreSelector          []string
	MaskSelector            []string
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
	CompareLayout bool
	NodeScope     string
}

const compareURLReadyTimeout = 10 * time.Second
const compareNetworkIdleWindow = 500 * time.Millisecond
const defaultViewportWidth = 1920
const defaultViewportHeight = 1080
const defaultCompareMatchMode = "exact"
const defaultCompareNodeScope = "current"
const compareLayoutThreshold = 12
const compareLayoutWarningThreshold = 48

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
