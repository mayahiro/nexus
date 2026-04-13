package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/rpc"
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
	Fingerprint string `json:"fingerprint"`
	Ref         string `json:"ref,omitempty"`
	Role        string `json:"role"`
	Label       string `json:"label,omitempty"`
	Name        string `json:"name,omitempty"`
	Text        string `json:"text,omitempty"`
	Value       string `json:"value,omitempty"`
	Href        string `json:"href,omitempty"`
	TestID      string `json:"testid,omitempty"`
	Visible     bool   `json:"visible"`
	Enabled     bool   `json:"enabled"`
	Editable    bool   `json:"editable"`
	Selectable  bool   `json:"selectable"`
	Invokable   bool   `json:"invokable"`
}

type compareSummary struct {
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

type compareFinding struct {
	Kind        string `json:"kind"`
	Severity    string `json:"severity,omitempty"`
	Impact      string `json:"impact,omitempty"`
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
	Summary  compareSummary   `json:"summary"`
	Findings []compareFinding `json:"findings"`
}

type compareManifest struct {
	Defaults compareManifestDefaults `json:"defaults,omitempty"`
	Pages    []compareManifestPage   `json:"pages,omitempty"`
}

type compareManifestDefaults struct {
	WaitSelector    string   `json:"wait_selector,omitempty"`
	WaitTimeout     *int     `json:"wait_timeout,omitempty"`
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
	WaitSelector    *string  `json:"wait_selector,omitempty"`
	WaitTimeout     *int     `json:"wait_timeout,omitempty"`
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
	WaitTimeout     int
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
	IgnoreText []*regexp.Regexp
	IgnoreNode []compareSelectorRule
	MaskNode   []compareSelectorRule
}

const compareURLReadyTimeout = 10 * time.Second

func runCompare(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printCompareHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	fs.SetOutput(stderr)

	positional := make([]string, 0, 2)
	for len(args) > 0 && len(positional) < 2 && !strings.HasPrefix(args[0], "-") {
		positional = append(positional, args[0])
		args = args[1:]
	}

	oldSession := fs.String("old-session", "", "old session id")
	newSession := fs.String("new-session", "", "new session id")
	oldURL := fs.String("old-url", "", "old url")
	newURL := fs.String("new-url", "", "new url")
	backend := fs.String("backend", "chromium", "browser backend")
	targetRef := fs.String("target-ref", "", "target ref")
	viewport := fs.String("viewport", "", "viewport as WIDTHxHEIGHT")
	manifestPath := fs.String("manifest", "", "compare manifest json")
	continueOnError := fs.Bool("continue-on-error", false, "continue after manifest page error")
	limit := fs.Int("limit", 0, "limit manifest pages")
	waitSelector := fs.String("wait-selector", "", "wait selector before compare")
	waitTimeout := fs.Int("wait-timeout", 10000, "wait timeout in ms")
	asJSON := fs.Bool("json", false, "print as json")
	outputJSON := fs.String("output-json", "", "write compare report json to file")
	outputMD := fs.String("output-md", "", "write compare report markdown to file")
	var ignoreRegex compareStringValues
	var ignoreSelector compareStringValues
	var maskSelector compareStringValues
	fs.Var(&ignoreRegex, "ignore-text-regex", "regex to strip from text before compare")
	fs.Var(&ignoreSelector, "ignore-selector", "node selector to ignore such as @e3, role=button, text=Save")
	fs.Var(&maskSelector, "mask-selector", "node selector to mask such as @e3, role=textbox, testid=user-id")

	if err := parseCommandFlags(fs, args, stderr, "compare"); err != nil {
		return 1
	}

	if strings.TrimSpace(*manifestPath) != "" {
		if len(positional) > 0 || fs.NArg() > 0 || *oldURL != "" || *newURL != "" || *oldSession != "" || *newSession != "" {
			fmt.Fprintln(stderr, "compare can not mix --manifest with urls or session flags")
			fmt.Fprintln(stderr, "hint: nxctl compare --manifest migration-pages.json")
			fmt.Fprintln(stderr, "hint: run `nxctl help compare` for details")
			return 1
		}
	} else if len(positional) == 2 && *oldURL == "" && *newURL == "" && *oldSession == "" && *newSession == "" {
		*oldURL = positional[0]
		*newURL = positional[1]
	} else if fs.NArg() == 2 && *oldURL == "" && *newURL == "" && *oldSession == "" && *newSession == "" {
		*oldURL = fs.Arg(0)
		*newURL = fs.Arg(1)
	} else if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "compare accepts either two urls, two sessions, or --manifest")
		printCompareHelp(stderr)
		return 1
	}

	if *waitTimeout < 0 {
		fmt.Fprintln(stderr, "wait-timeout must be a non-negative integer")
		return 1
	}
	if *limit < 0 {
		fmt.Fprintln(stderr, "limit must be a non-negative integer")
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	base := compareRun{
		Backend:         *backend,
		TargetRef:       *targetRef,
		Viewport:        *viewport,
		WaitSelector:    *waitSelector,
		WaitTimeout:     *waitTimeout,
		IgnoreTextRegex: append([]string(nil), ignoreRegex...),
		IgnoreSelector:  append([]string(nil), ignoreSelector...),
		MaskSelector:    append([]string(nil), maskSelector...),
	}

	if strings.TrimSpace(*manifestPath) != "" {
		manifest, err := loadCompareManifest(*manifestPath)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		report, err := executeCompareManifest(ctx, client, paths, *manifestPath, manifest, base, *continueOnError, *limit)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if strings.TrimSpace(*outputJSON) != "" {
			if err := writeIndentedJSONFile(*outputJSON, report); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		}
		if strings.TrimSpace(*outputMD) != "" {
			if err := writeCompareManifestMarkdown(*outputMD, report); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		}
		if *asJSON {
			encoder := json.NewEncoder(stdout)
			encoder.SetIndent("", "  ")
			if err := encoder.Encode(report); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			return 0
		}
		printCompareManifestReport(stdout, report)
		return 0
	}

	base.OldEndpoint = compareEndpoint{SessionID: strings.TrimSpace(*oldSession), URL: strings.TrimSpace(*oldURL)}
	base.NewEndpoint = compareEndpoint{SessionID: strings.TrimSpace(*newSession), URL: strings.TrimSpace(*newURL)}
	if base.OldEndpoint.SessionID == "" && base.OldEndpoint.URL == "" && base.NewEndpoint.SessionID == "" && base.NewEndpoint.URL == "" {
		fmt.Fprintln(stderr, "compare requires either two urls, two sessions, or --manifest")
		fmt.Fprintln(stderr, "hint: nxctl compare https://old.example.com https://new.example.com")
		fmt.Fprintln(stderr, "hint: run `nxctl help compare` for details")
		return 1
	}

	report, err := executeCompare(ctx, client, paths, base)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if strings.TrimSpace(*outputJSON) != "" {
		if err := writeIndentedJSONFile(*outputJSON, report); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if strings.TrimSpace(*outputMD) != "" {
		if err := writeCompareMarkdown(*outputMD, report); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	printCompareReport(stdout, report)
	return 0
}

func executeCompare(ctx context.Context, client *rpc.Client, paths config.Paths, run compareRun) (compareReport, error) {
	if err := validateCompareEndpoint("old", run.OldEndpoint); err != nil {
		return compareReport{}, err
	}
	if err := validateCompareEndpoint("new", run.NewEndpoint); err != nil {
		return compareReport{}, err
	}
	if run.WaitTimeout < 0 {
		return compareReport{}, errors.New("wait-timeout must be a non-negative integer")
	}

	ignorePatterns, err := compileCompareRegexps(run.IgnoreTextRegex)
	if err != nil {
		return compareReport{}, err
	}
	ignoreRules, err := compileCompareSelectorRules(run.IgnoreSelector)
	if err != nil {
		return compareReport{}, err
	}
	maskRules, err := compileCompareSelectorRules(run.MaskSelector)
	if err != nil {
		return compareReport{}, err
	}

	oldPrepared, newPrepared, err := prepareCompareSessions(ctx, client, paths, run.OldEndpoint, run.NewEndpoint, run.Backend, run.TargetRef, run.Viewport)
	if err != nil {
		return compareReport{}, err
	}
	defer cleanupCompareSession(context.Background(), client, oldPrepared)
	defer cleanupCompareSession(context.Background(), client, newPrepared)

	for _, endpoint := range []struct {
		prepared preparedCompareSession
		source   compareEndpoint
	}{
		{prepared: oldPrepared, source: run.OldEndpoint},
		{prepared: newPrepared, source: run.NewEndpoint},
	} {
		if endpoint.source.URL == "" {
			continue
		}
		if err := waitForCompareURLReady(ctx, client, endpoint.prepared.SessionID); err != nil {
			return compareReport{}, err
		}
	}

	if strings.TrimSpace(run.WaitSelector) != "" {
		for _, prepared := range []preparedCompareSession{oldPrepared, newPrepared} {
			if err := waitForCompareSelector(ctx, client, prepared.SessionID, run.WaitSelector, run.WaitTimeout); err != nil {
				return compareReport{}, err
			}
		}
	}

	oldObservation, err := observeCompareSession(ctx, client, oldPrepared.SessionID)
	if err != nil {
		return compareReport{}, err
	}
	newObservation, err := observeCompareSession(ctx, client, newPrepared.SessionID)
	if err != nil {
		return compareReport{}, err
	}

	return buildCompareReport(
		buildCompareSnapshot(oldObservation, compareSnapshotOptions{
			IgnoreText: ignorePatterns,
			IgnoreNode: ignoreRules,
			MaskNode:   maskRules,
		}),
		buildCompareSnapshot(newObservation, compareSnapshotOptions{
			IgnoreText: ignorePatterns,
			IgnoreNode: ignoreRules,
			MaskNode:   maskRules,
		}),
	), nil
}

func executeCompareManifest(ctx context.Context, client *rpc.Client, paths config.Paths, manifestPath string, manifest compareManifest, base compareRun, continueOnError bool, limit int) (compareManifestReport, error) {
	pages := manifest.Pages
	if len(pages) == 0 {
		return compareManifestReport{}, errors.New("manifest requires at least one page")
	}
	if limit > 0 && limit < len(pages) {
		pages = pages[:limit]
	}

	report := compareManifestReport{
		Manifest: manifestPath,
		Pages:    make([]compareManifestPageReport, 0, len(pages)),
	}

	for i, page := range pages {
		name := compareManifestPageName(page, i)
		run := mergeCompareManifestPage(base, manifest.Defaults, page)
		single, err := executeCompare(ctx, client, paths, run)
		if err != nil {
			if !continueOnError {
				return compareManifestReport{}, fmt.Errorf("manifest %s failed: %w", name, err)
			}
			report.Pages = append(report.Pages, compareManifestPageReport{
				Name:  name,
				Error: err.Error(),
			})
			continue
		}
		report.Pages = append(report.Pages, compareManifestPageReport{
			Name:   name,
			Report: &single,
		})
	}

	report.Summary = summarizeCompareManifest(report.Pages)
	return report, nil
}

func loadCompareManifest(path string) (compareManifest, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return compareManifest{}, err
	}
	var manifest compareManifest
	if err := json.Unmarshal(bytes, &manifest); err != nil {
		return compareManifest{}, fmt.Errorf("invalid compare manifest %q: %w", path, err)
	}
	if len(manifest.Pages) == 0 {
		return compareManifest{}, fmt.Errorf("manifest %q requires at least one page", path)
	}
	return manifest, nil
}

func mergeCompareManifestPage(base compareRun, defaults compareManifestDefaults, page compareManifestPage) compareRun {
	run := compareRun{
		OldEndpoint: compareEndpoint{
			SessionID: strings.TrimSpace(page.OldSession),
			URL:       strings.TrimSpace(page.OldURL),
		},
		NewEndpoint: compareEndpoint{
			SessionID: strings.TrimSpace(page.NewSession),
			URL:       strings.TrimSpace(page.NewURL),
		},
		Backend:         base.Backend,
		TargetRef:       base.TargetRef,
		Viewport:        base.Viewport,
		WaitSelector:    base.WaitSelector,
		WaitTimeout:     base.WaitTimeout,
		IgnoreTextRegex: append([]string(nil), base.IgnoreTextRegex...),
		IgnoreSelector:  append([]string(nil), base.IgnoreSelector...),
		MaskSelector:    append([]string(nil), base.MaskSelector...),
	}

	if defaults.WaitSelector != "" {
		run.WaitSelector = defaults.WaitSelector
	}
	if defaults.WaitTimeout != nil {
		run.WaitTimeout = *defaults.WaitTimeout
	}
	run.IgnoreTextRegex = append(run.IgnoreTextRegex, defaults.IgnoreTextRegex...)
	run.IgnoreSelector = append(run.IgnoreSelector, defaults.IgnoreSelector...)
	run.MaskSelector = append(run.MaskSelector, defaults.MaskSelector...)

	if page.WaitSelector != nil {
		run.WaitSelector = strings.TrimSpace(*page.WaitSelector)
	}
	if page.WaitTimeout != nil {
		run.WaitTimeout = *page.WaitTimeout
	}
	run.IgnoreTextRegex = append(run.IgnoreTextRegex, page.IgnoreTextRegex...)
	run.IgnoreSelector = append(run.IgnoreSelector, page.IgnoreSelector...)
	run.MaskSelector = append(run.MaskSelector, page.MaskSelector...)
	return run
}

func compareManifestPageName(page compareManifestPage, index int) string {
	if strings.TrimSpace(page.Name) != "" {
		return strings.TrimSpace(page.Name)
	}
	return fmt.Sprintf("page[%d]", index)
}

func summarizeCompareManifest(pages []compareManifestPageReport) compareManifestSummary {
	summary := compareManifestSummary{
		TotalPages: len(pages),
	}
	for _, page := range pages {
		if page.Error != "" {
			summary.FailedPages++
			continue
		}
		if page.Report == nil {
			summary.FailedPages++
			continue
		}
		summary.ComparedPages++
		if page.Report.Summary.Same {
			summary.SamePages++
		} else {
			summary.DifferentPages++
		}
		summary.TotalFindings += page.Report.Summary.TotalFindings
		summary.Critical += page.Report.Summary.Critical
		summary.Warning += page.Report.Summary.Warning
		summary.Info += page.Report.Summary.Info
	}
	return summary
}

func writeIndentedJSONFile(path string, value any) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeCompareJSON(path string, report compareReport) error {
	return writeIndentedJSONFile(path, report)
}

func writeCompareMarkdown(path string, report compareReport) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	printCompareMarkdown(file, report)
	return nil
}

func writeCompareManifestMarkdown(path string, report compareManifestReport) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	printCompareManifestMarkdown(file, report)
	return nil
}

func validateCompareEndpoint(label string, endpoint compareEndpoint) error {
	switch {
	case endpoint.SessionID == "" && endpoint.URL == "":
		return fmt.Errorf("%s side requires either --%s-session or --%s-url", label, label, label)
	case endpoint.SessionID != "" && endpoint.URL != "":
		return fmt.Errorf("%s side can not use both session and url", label)
	default:
		return nil
	}
}

func compileCompareRegexps(values []string) ([]*regexp.Regexp, error) {
	patterns := make([]*regexp.Regexp, 0, len(values))
	for _, value := range values {
		pattern, err := regexp.Compile(value)
		if err != nil {
			return nil, fmt.Errorf("invalid ignore-text-regex %q: %w", value, err)
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}

func compileCompareSelectorRules(values []string) ([]compareSelectorRule, error) {
	rules := make([]compareSelectorRule, 0, len(values))
	for _, value := range values {
		rule, err := parseCompareSelectorRule(value)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func parseCompareSelectorRule(value string) (compareSelectorRule, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return compareSelectorRule{}, errors.New("compare selector must not be empty")
	}
	if strings.HasPrefix(trimmed, "@e") {
		return compareSelectorRule{All: []compareSelectorTerm{{Kind: "ref", Value: trimmed}}}, nil
	}

	parts := strings.Split(trimmed, "&")
	terms := make([]compareSelectorTerm, 0, len(parts))
	for _, part := range parts {
		term, err := parseCompareSelectorTerm(part, value)
		if err != nil {
			return compareSelectorRule{}, err
		}
		terms = append(terms, term)
	}
	return compareSelectorRule{All: terms}, nil
}

func parseCompareSelectorTerm(value string, rawInput string) (compareSelectorTerm, error) {
	kind, raw, ok := strings.Cut(strings.TrimSpace(value), "=")
	if !ok {
		return compareSelectorTerm{}, fmt.Errorf("invalid compare selector %q: use @eN or role/name/text/testid/href=<value>[&...]", rawInput)
	}
	kind = strings.TrimSpace(kind)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return compareSelectorTerm{}, fmt.Errorf("invalid compare selector %q: value must not be empty", rawInput)
	}
	switch kind {
	case "role", "name", "text", "testid", "href":
		return compareSelectorTerm{Kind: kind, Value: raw}, nil
	default:
		return compareSelectorTerm{}, fmt.Errorf("invalid compare selector %q: supported kinds are role, name, text, testid, href, or @eN", rawInput)
	}
}

func prepareCompareSessions(ctx context.Context, client *rpc.Client, paths config.Paths, oldEndpoint compareEndpoint, newEndpoint compareEndpoint, backend string, targetRef string, viewport string) (preparedCompareSession, preparedCompareSession, error) {
	resolvedTargetRef := strings.TrimSpace(targetRef)
	if resolvedTargetRef == "" && (oldEndpoint.URL != "" || newEndpoint.URL != "") {
		installation, err := newBrowserManager(paths).Resolve(backend)
		if err != nil {
			return preparedCompareSession{}, preparedCompareSession{}, err
		}
		resolvedTargetRef = installation.ExecutablePath
	}

	width, height, err := resolvedViewport(viewport)
	if err != nil {
		return preparedCompareSession{}, preparedCompareSession{}, err
	}

	oldPrepared, err := prepareCompareSession(ctx, client, "old", oldEndpoint, backend, resolvedTargetRef, width, height)
	if err != nil {
		return preparedCompareSession{}, preparedCompareSession{}, err
	}
	newPrepared, err := prepareCompareSession(ctx, client, "new", newEndpoint, backend, resolvedTargetRef, width, height)
	if err != nil {
		cleanupCompareSession(context.Background(), client, oldPrepared)
		return preparedCompareSession{}, preparedCompareSession{}, err
	}

	return oldPrepared, newPrepared, nil
}

func prepareCompareSession(ctx context.Context, client *rpc.Client, label string, endpoint compareEndpoint, backend string, targetRef string, width int, height int) (preparedCompareSession, error) {
	if endpoint.SessionID != "" {
		return preparedCompareSession{SessionID: endpoint.SessionID}, nil
	}

	sessionID := fmt.Sprintf("compare-%s-%s", label, newCompareSessionSuffix())
	res, err := client.AttachSession(ctx, api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  sessionID,
		TargetRef:  targetRef,
		Backend:    backend,
		Options: map[string]string{
			"initial_url":     endpoint.URL,
			"viewport_width":  strconv.Itoa(width),
			"viewport_height": strconv.Itoa(height),
		},
	})
	if err != nil {
		return preparedCompareSession{}, err
	}

	return preparedCompareSession{
		SessionID: res.Session.ID,
		Detach:    true,
	}, nil
}

func cleanupCompareSession(ctx context.Context, client *rpc.Client, prepared preparedCompareSession) {
	if !prepared.Detach || prepared.SessionID == "" {
		return
	}
	detachCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, _ = client.DetachSession(detachCtx, api.DetachSessionRequest{SessionID: prepared.SessionID})
}

func waitForCompareSelector(ctx context.Context, client *rpc.Client, sessionID string, selector string, timeout int) error {
	_, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: sessionID,
		Action: api.Action{
			Kind: "wait",
			Args: map[string]string{
				"target":     "selector",
				"value":      selector,
				"state":      "visible",
				"timeout_ms": strconv.Itoa(timeout),
			},
		},
	})
	return err
}

func observeCompareSession(ctx context.Context, client *rpc.Client, sessionID string) (api.Observation, error) {
	res, err := client.ObserveSession(ctx, api.ObserveSessionRequest{
		SessionID: sessionID,
		Options: api.ObserveOptions{
			WithText: true,
			WithTree: true,
		},
	})
	if err != nil {
		return api.Observation{}, err
	}
	return res.Observation, nil
}

func waitForCompareURLReady(ctx context.Context, client *rpc.Client, sessionID string) error {
	waitCtx, cancel := context.WithTimeout(ctx, compareURLReadyTimeout)
	defer cancel()

	for {
		observation, err := observeCompareSession(waitCtx, client, sessionID)
		if err != nil {
			return err
		}
		currentURL := strings.TrimSpace(observation.URLOrScreen)
		if currentURL != "" && currentURL != "about:blank" {
			return nil
		}

		select {
		case <-waitCtx.Done():
			return fmt.Errorf("session %s stayed on about:blank", sessionID)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func buildCompareSnapshot(observation api.Observation, options compareSnapshotOptions) compareSnapshot {
	nodes := make([]compareSnapshotNode, 0, len(observation.Tree))
	for _, node := range observation.Tree {
		if matchesCompareSelectorRule(node, options.IgnoreNode) {
			continue
		}

		fingerprint := strings.TrimSpace(node.Fingerprint)
		if fingerprint == "" {
			fingerprint = strings.Join([]string{
				strings.TrimSpace(node.Role),
				strings.TrimSpace(node.Name),
				strings.TrimSpace(node.Text),
				strings.TrimSpace(node.Value),
			}, "|")
		}

		name := normalizeCompareString(node.Name, options.IgnoreText)
		text := normalizeCompareString(node.Text, options.IgnoreText)
		value := normalizeCompareString(node.Value, options.IgnoreText)
		href := normalizeCompareString(node.Attrs["href"], options.IgnoreText)
		testID := normalizeCompareString(firstNonEmpty(node.Attrs["data-testid"], node.Attrs["data-test"]), options.IgnoreText)
		if matchesCompareSelectorRule(node, options.MaskNode) {
			name = ""
			text = ""
			value = ""
		}

		nodes = append(nodes, compareSnapshotNode{
			Fingerprint: fingerprint,
			Ref:         strings.TrimSpace(node.Ref),
			Role:        strings.TrimSpace(node.Role),
			Label:       compareNodeLabel(name, text, value, href, testID),
			Name:        name,
			Text:        text,
			Value:       value,
			Href:        href,
			TestID:      testID,
			Visible:     node.Visible,
			Enabled:     node.Enabled,
			Editable:    node.Editable,
			Selectable:  node.Selectable,
			Invokable:   node.Invokable,
		})
	}

	slices.SortFunc(nodes, func(a, b compareSnapshotNode) int {
		switch {
		case a.Fingerprint < b.Fingerprint:
			return -1
		case a.Fingerprint > b.Fingerprint:
			return 1
		case a.Label < b.Label:
			return -1
		case a.Label > b.Label:
			return 1
		default:
			return 0
		}
	})

	return compareSnapshot{
		SessionID: observation.SessionID,
		URL:       normalizeCompareString(observation.URLOrScreen, options.IgnoreText),
		Title:     normalizeCompareString(observation.Title, options.IgnoreText),
		Text:      normalizeCompareString(observation.Text, options.IgnoreText),
		Nodes:     nodes,
	}
}

func matchesCompareSelectorRule(node api.Node, rules []compareSelectorRule) bool {
	for _, rule := range rules {
		matched := true
		for _, term := range rule.All {
			switch term.Kind {
			case "ref":
				if strings.TrimSpace(node.Ref) != term.Value {
					matched = false
				}
			case "role":
				if normalizeFindValue(node.Role) != normalizeFindValue(term.Value) {
					matched = false
				}
			case "name":
				if !compareSelectorContains(node.Name, term.Value) {
					matched = false
				}
			case "text":
				if !compareSelectorContains(node.Text, term.Value) {
					matched = false
				}
			case "testid":
				if !compareSelectorContains(firstNonEmpty(node.Attrs["data-testid"], node.Attrs["data-test"]), term.Value) {
					matched = false
				}
			case "href":
				if !compareSelectorContains(node.Attrs["href"], term.Value) {
					matched = false
				}
			}
			if !matched {
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func compareSelectorContains(value string, needle string) bool {
	return strings.Contains(normalizeFindValue(value), normalizeFindValue(needle))
}

func normalizeCompareString(value string, ignore []*regexp.Regexp) string {
	normalized := value
	for _, pattern := range ignore {
		normalized = pattern.ReplaceAllString(normalized, "")
	}
	return strings.Join(strings.Fields(strings.TrimSpace(normalized)), " ")
}

func compareNodeLabel(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func buildCompareReport(oldSnapshot compareSnapshot, newSnapshot compareSnapshot) compareReport {
	report := compareReport{
		Old: oldSnapshot,
		New: newSnapshot,
	}

	add := func(finding compareFinding) {
		finding.Severity, finding.Impact = classifyCompareFinding(finding)
		report.Findings = append(report.Findings, finding)
		report.Summary.TotalFindings++
		switch finding.Kind {
		case "title_changed":
			report.Summary.TitleChanged++
		case "text_changed":
			report.Summary.TextChanged++
		case "missing_node":
			report.Summary.MissingNodes++
		case "new_node":
			report.Summary.NewNodes++
		case "state_changed":
			report.Summary.StateChanged++
		case "page_text_changed":
			report.Summary.PageTextChanged++
		}
		switch finding.Severity {
		case "critical":
			report.Summary.Critical++
		case "warning":
			report.Summary.Warning++
		default:
			report.Summary.Info++
		}
	}

	if oldSnapshot.Title != newSnapshot.Title {
		add(compareFinding{
			Kind:  "title_changed",
			Field: "title",
			Old:   oldSnapshot.Title,
			New:   newSnapshot.Title,
		})
	}

	if oldSnapshot.Text != newSnapshot.Text {
		add(compareFinding{
			Kind:  "page_text_changed",
			Field: "page_text",
			Old:   summarizeCompareValue(oldSnapshot.Text),
			New:   summarizeCompareValue(newSnapshot.Text),
		})
	}

	oldGroups := groupCompareNodes(oldSnapshot.Nodes)
	newGroups := groupCompareNodes(newSnapshot.Nodes)
	keys := make([]string, 0, len(oldGroups)+len(newGroups))
	seen := map[string]struct{}{}
	for key := range oldGroups {
		keys = append(keys, key)
		seen[key] = struct{}{}
	}
	for key := range newGroups {
		if _, ok := seen[key]; ok {
			continue
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)

	for _, key := range keys {
		oldNodes := oldGroups[key]
		newNodes := newGroups[key]
		maxLen := len(oldNodes)
		if len(newNodes) > maxLen {
			maxLen = len(newNodes)
		}
		for i := 0; i < maxLen; i++ {
			switch {
			case i >= len(oldNodes):
				node := newNodes[i]
				add(compareFinding{
					Kind:        "new_node",
					Fingerprint: node.Fingerprint,
					Role:        node.Role,
					Label:       node.Label,
				})
			case i >= len(newNodes):
				node := oldNodes[i]
				add(compareFinding{
					Kind:        "missing_node",
					Fingerprint: node.Fingerprint,
					Role:        node.Role,
					Label:       node.Label,
				})
			default:
				oldNode := oldNodes[i]
				newNode := newNodes[i]
				if oldNode.Name != newNode.Name {
					add(compareFinding{
						Kind:        "text_changed",
						Fingerprint: oldNode.Fingerprint,
						Role:        oldNode.Role,
						Label:       firstNonEmpty(oldNode.Label, newNode.Label),
						Field:       "name",
						Old:         oldNode.Name,
						New:         newNode.Name,
					})
				}
				if oldNode.Text != newNode.Text {
					add(compareFinding{
						Kind:        "text_changed",
						Fingerprint: oldNode.Fingerprint,
						Role:        oldNode.Role,
						Label:       firstNonEmpty(oldNode.Label, newNode.Label),
						Field:       "text",
						Old:         summarizeCompareValue(oldNode.Text),
						New:         summarizeCompareValue(newNode.Text),
					})
				}
				if oldNode.Value != newNode.Value {
					add(compareFinding{
						Kind:        "text_changed",
						Fingerprint: oldNode.Fingerprint,
						Role:        oldNode.Role,
						Label:       firstNonEmpty(oldNode.Label, newNode.Label),
						Field:       "value",
						Old:         oldNode.Value,
						New:         newNode.Value,
					})
				}
				oldState := compareNodeState(oldNode)
				newState := compareNodeState(newNode)
				if oldState != newState {
					add(compareFinding{
						Kind:        "state_changed",
						Fingerprint: oldNode.Fingerprint,
						Role:        oldNode.Role,
						Label:       firstNonEmpty(oldNode.Label, newNode.Label),
						Field:       "state",
						Old:         oldState,
						New:         newState,
					})
				}
			}
		}
	}

	report.Summary.Same = report.Summary.TotalFindings == 0
	return report
}

func groupCompareNodes(nodes []compareSnapshotNode) map[string][]compareSnapshotNode {
	grouped := make(map[string][]compareSnapshotNode, len(nodes))
	for _, node := range nodes {
		grouped[node.Fingerprint] = append(grouped[node.Fingerprint], node)
	}

	for key := range grouped {
		slices.SortFunc(grouped[key], func(a, b compareSnapshotNode) int {
			aKey := compareNodeSortKey(a)
			bKey := compareNodeSortKey(b)
			switch {
			case aKey < bKey:
				return -1
			case aKey > bKey:
				return 1
			default:
				return 0
			}
		})
	}

	return grouped
}

func compareNodeSortKey(node compareSnapshotNode) string {
	return strings.Join([]string{
		node.Role,
		node.Label,
		node.Name,
		node.Text,
		node.Value,
		node.Href,
		node.TestID,
		compareNodeState(node),
	}, "|")
}

func compareNodeState(node compareSnapshotNode) string {
	return strings.Join([]string{
		strconv.FormatBool(node.Visible),
		strconv.FormatBool(node.Enabled),
		strconv.FormatBool(node.Editable),
		strconv.FormatBool(node.Selectable),
		strconv.FormatBool(node.Invokable),
	}, "/")
}

func summarizeCompareValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= 120 {
		return trimmed
	}
	return trimmed[:117] + "..."
}

func classifyCompareFinding(finding compareFinding) (string, string) {
	switch finding.Kind {
	case "title_changed":
		return "warning", "page_title_changed"
	case "page_text_changed":
		return "info", "content_changed"
	case "new_node":
		return "warning", "new_content"
	case "missing_node":
		switch {
		case finding.Role == "button":
			return "critical", "primary_action_missing"
		case finding.Role == "link":
			return "warning", "navigation_changed"
		case finding.Role == "textbox" || finding.Role == "combobox":
			return "critical", "form_input_changed"
		default:
			return "warning", "content_changed"
		}
	case "state_changed":
		if finding.Field == "state" && strings.Contains(finding.Old, "true/true") && strings.Contains(finding.New, "true/false") {
			if finding.Role == "textbox" || finding.Role == "combobox" {
				return "critical", "form_input_disabled"
			}
			if finding.Role == "button" {
				return "critical", "primary_action_missing"
			}
		}
		return "warning", "content_changed"
	case "text_changed":
		if finding.Role == "textbox" || finding.Role == "combobox" {
			return "warning", "form_input_changed"
		}
		if finding.Role == "link" {
			return "warning", "navigation_changed"
		}
		return "warning", "content_changed"
	default:
		return "info", "content_changed"
	}
}

func printCompareReport(w io.Writer, report compareReport) {
	fmt.Fprintf(w, "old: %s", firstNonEmpty(report.Old.URL, report.Old.SessionID))
	if report.Old.Title != "" {
		fmt.Fprintf(w, " (%s)", report.Old.Title)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "new: %s", firstNonEmpty(report.New.URL, report.New.SessionID))
	if report.New.Title != "" {
		fmt.Fprintf(w, " (%s)", report.New.Title)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "summary: %d findings\n", report.Summary.TotalFindings)
	if report.Summary.Same {
		fmt.Fprintln(w, "no significant differences")
		return
	}
	fmt.Fprintf(w, "critical: %d\n", report.Summary.Critical)
	fmt.Fprintf(w, "warning: %d\n", report.Summary.Warning)
	fmt.Fprintf(w, "info: %d\n", report.Summary.Info)

	if report.Summary.TitleChanged > 0 {
		fmt.Fprintf(w, "title_changed: %d\n", report.Summary.TitleChanged)
	}
	if report.Summary.PageTextChanged > 0 {
		fmt.Fprintf(w, "page_text_changed: %d\n", report.Summary.PageTextChanged)
	}
	if report.Summary.TextChanged > 0 {
		fmt.Fprintf(w, "text_changed: %d\n", report.Summary.TextChanged)
	}
	if report.Summary.MissingNodes > 0 {
		fmt.Fprintf(w, "missing_node: %d\n", report.Summary.MissingNodes)
	}
	if report.Summary.NewNodes > 0 {
		fmt.Fprintf(w, "new_node: %d\n", report.Summary.NewNodes)
	}
	if report.Summary.StateChanged > 0 {
		fmt.Fprintf(w, "state_changed: %d\n", report.Summary.StateChanged)
	}

	fmt.Fprintln(w)
	for _, finding := range report.Findings {
		switch finding.Kind {
		case "title_changed":
			fmt.Fprintf(w, "[%s] [title_changed] %s: %q -> %q\n", finding.Severity, finding.Impact, finding.Old, finding.New)
		case "page_text_changed":
			fmt.Fprintf(w, "[%s] [page_text_changed] %s: %q -> %q\n", finding.Severity, finding.Impact, finding.Old, finding.New)
		case "missing_node":
			fmt.Fprintf(w, "[%s] [missing_node] %s %s %q\n", finding.Severity, finding.Impact, finding.Role, finding.Label)
		case "new_node":
			fmt.Fprintf(w, "[%s] [new_node] %s %s %q\n", finding.Severity, finding.Impact, finding.Role, finding.Label)
		case "text_changed":
			fmt.Fprintf(w, "[%s] [text_changed] %s %s %q %s: %q -> %q\n", finding.Severity, finding.Impact, finding.Role, finding.Label, finding.Field, finding.Old, finding.New)
		case "state_changed":
			fmt.Fprintf(w, "[%s] [state_changed] %s %s %q: %s -> %s\n", finding.Severity, finding.Impact, finding.Role, finding.Label, finding.Old, finding.New)
		}
	}
}

func printCompareManifestReport(w io.Writer, report compareManifestReport) {
	fmt.Fprintf(w, "manifest: %s\n", report.Manifest)
	fmt.Fprintf(w, "pages: %d total, %d compared, %d failed\n", report.Summary.TotalPages, report.Summary.ComparedPages, report.Summary.FailedPages)
	fmt.Fprintf(w, "same: %d\n", report.Summary.SamePages)
	fmt.Fprintf(w, "different: %d\n", report.Summary.DifferentPages)
	fmt.Fprintf(w, "summary: %d findings\n", report.Summary.TotalFindings)
	fmt.Fprintf(w, "critical: %d\n", report.Summary.Critical)
	fmt.Fprintf(w, "warning: %d\n", report.Summary.Warning)
	fmt.Fprintf(w, "info: %d\n", report.Summary.Info)
	if report.Summary.TotalFindings == 0 && report.Summary.FailedPages == 0 {
		fmt.Fprintln(w, "no significant differences")
	}
	for _, page := range report.Pages {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "[%s]\n", page.Name)
		if page.Error != "" {
			fmt.Fprintf(w, "error: %s\n", page.Error)
			continue
		}
		if page.Report != nil {
			printCompareReport(w, *page.Report)
		}
	}
}

func printCompareMarkdown(w io.Writer, report compareReport) {
	fmt.Fprintln(w, "# Compare Report")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- Old: `%s`\n", firstNonEmpty(report.Old.URL, report.Old.SessionID))
	fmt.Fprintf(w, "- New: `%s`\n", firstNonEmpty(report.New.URL, report.New.SessionID))
	if report.Old.Title != "" || report.New.Title != "" {
		fmt.Fprintf(w, "- Titles: `%s` -> `%s`\n", report.Old.Title, report.New.Title)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Summary")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- Total findings: %d\n", report.Summary.TotalFindings)
	fmt.Fprintf(w, "- Critical: %d\n", report.Summary.Critical)
	fmt.Fprintf(w, "- Warning: %d\n", report.Summary.Warning)
	fmt.Fprintf(w, "- Info: %d\n", report.Summary.Info)
	if report.Summary.Same {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "No significant differences.")
		return
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Findings")
	fmt.Fprintln(w)
	for _, finding := range report.Findings {
		switch finding.Kind {
		case "title_changed":
			fmt.Fprintf(w, "- [%s] `%s`: `%s` -> `%s`\n", finding.Severity, finding.Impact, finding.Old, finding.New)
		case "page_text_changed":
			fmt.Fprintf(w, "- [%s] `%s`: `%s` -> `%s`\n", finding.Severity, finding.Impact, finding.Old, finding.New)
		case "missing_node", "new_node":
			fmt.Fprintf(w, "- [%s] `%s`: `%s` `%s`\n", finding.Severity, finding.Impact, finding.Role, finding.Label)
		case "text_changed":
			fmt.Fprintf(w, "- [%s] `%s`: `%s` `%s` `%s` `%s` -> `%s`\n", finding.Severity, finding.Impact, finding.Role, finding.Label, finding.Field, finding.Old, finding.New)
		case "state_changed":
			fmt.Fprintf(w, "- [%s] `%s`: `%s` `%s` `%s` -> `%s`\n", finding.Severity, finding.Impact, finding.Role, finding.Label, finding.Old, finding.New)
		}
	}
}

func printCompareManifestMarkdown(w io.Writer, report compareManifestReport) {
	fmt.Fprintln(w, "# Compare Manifest Report")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- Manifest: `%s`\n", report.Manifest)
	fmt.Fprintf(w, "- Pages: %d total / %d compared / %d failed\n", report.Summary.TotalPages, report.Summary.ComparedPages, report.Summary.FailedPages)
	fmt.Fprintf(w, "- Same: %d\n", report.Summary.SamePages)
	fmt.Fprintf(w, "- Different: %d\n", report.Summary.DifferentPages)
	fmt.Fprintf(w, "- Total findings: %d\n", report.Summary.TotalFindings)
	fmt.Fprintf(w, "- Critical: %d\n", report.Summary.Critical)
	fmt.Fprintf(w, "- Warning: %d\n", report.Summary.Warning)
	fmt.Fprintf(w, "- Info: %d\n", report.Summary.Info)
	for _, page := range report.Pages {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "## %s\n", page.Name)
		fmt.Fprintln(w)
		if page.Error != "" {
			fmt.Fprintf(w, "Error: %s\n", page.Error)
			continue
		}
		if page.Report == nil {
			continue
		}
		fmt.Fprintf(w, "- Old: `%s`\n", firstNonEmpty(page.Report.Old.URL, page.Report.Old.SessionID))
		fmt.Fprintf(w, "- New: `%s`\n", firstNonEmpty(page.Report.New.URL, page.Report.New.SessionID))
		fmt.Fprintf(w, "- Findings: %d\n", page.Report.Summary.TotalFindings)
		fmt.Fprintf(w, "- Critical: %d\n", page.Report.Summary.Critical)
		fmt.Fprintf(w, "- Warning: %d\n", page.Report.Summary.Warning)
		fmt.Fprintf(w, "- Info: %d\n", page.Report.Summary.Info)
		if page.Report.Summary.Same {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "No significant differences.")
			continue
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, "### Findings")
		fmt.Fprintln(w)
		for _, finding := range page.Report.Findings {
			switch finding.Kind {
			case "title_changed":
				fmt.Fprintf(w, "- [%s] `%s`: `%s` -> `%s`\n", finding.Severity, finding.Impact, finding.Old, finding.New)
			case "page_text_changed":
				fmt.Fprintf(w, "- [%s] `%s`: `%s` -> `%s`\n", finding.Severity, finding.Impact, finding.Old, finding.New)
			case "missing_node", "new_node":
				fmt.Fprintf(w, "- [%s] `%s`: `%s` `%s`\n", finding.Severity, finding.Impact, finding.Role, finding.Label)
			case "text_changed":
				fmt.Fprintf(w, "- [%s] `%s`: `%s` `%s` `%s` `%s` -> `%s`\n", finding.Severity, finding.Impact, finding.Role, finding.Label, finding.Field, finding.Old, finding.New)
			case "state_changed":
				fmt.Fprintf(w, "- [%s] `%s`: `%s` `%s` `%s` -> `%s`\n", finding.Severity, finding.Impact, finding.Role, finding.Label, finding.Old, finding.New)
			}
		}
	}
}
