package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
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
}

type compareFinding struct {
	Kind        string `json:"kind"`
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

type preparedCompareSession struct {
	SessionID string
	Detach    bool
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
	waitSelector := fs.String("wait-selector", "", "wait selector before compare")
	waitTimeout := fs.Int("wait-timeout", 10000, "wait timeout in ms")
	asJSON := fs.Bool("json", false, "print as json")
	var ignoreRegex compareStringValues
	fs.Var(&ignoreRegex, "ignore-text-regex", "regex to strip from text before compare")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if len(positional) == 2 && *oldURL == "" && *newURL == "" && *oldSession == "" && *newSession == "" {
		*oldURL = positional[0]
		*newURL = positional[1]
	} else if fs.NArg() == 2 && *oldURL == "" && *newURL == "" && *oldSession == "" && *newSession == "" {
		*oldURL = fs.Arg(0)
		*newURL = fs.Arg(1)
	} else if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "compare accepts either two urls or explicit --old/--new flags")
		printCompareHelp(stderr)
		return 1
	}

	oldEndpoint := compareEndpoint{SessionID: strings.TrimSpace(*oldSession), URL: strings.TrimSpace(*oldURL)}
	newEndpoint := compareEndpoint{SessionID: strings.TrimSpace(*newSession), URL: strings.TrimSpace(*newURL)}
	if err := validateCompareEndpoint("old", oldEndpoint); err != nil {
		fmt.Fprintln(stderr, err)
		printCompareHelp(stderr)
		return 1
	}
	if err := validateCompareEndpoint("new", newEndpoint); err != nil {
		fmt.Fprintln(stderr, err)
		printCompareHelp(stderr)
		return 1
	}
	if *waitTimeout < 0 {
		fmt.Fprintln(stderr, "wait-timeout must be a non-negative integer")
		return 1
	}

	ignorePatterns, err := compileCompareRegexps(ignoreRegex)
	if err != nil {
		fmt.Fprintln(stderr, err)
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

	oldPrepared, newPrepared, err := prepareCompareSessions(ctx, client, paths, oldEndpoint, newEndpoint, *backend, *targetRef, *viewport)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer cleanupCompareSession(context.Background(), client, oldPrepared)
	defer cleanupCompareSession(context.Background(), client, newPrepared)

	for _, endpoint := range []struct {
		prepared preparedCompareSession
		source   compareEndpoint
	}{
		{prepared: oldPrepared, source: oldEndpoint},
		{prepared: newPrepared, source: newEndpoint},
	} {
		if endpoint.source.URL == "" {
			continue
		}
		if err := waitForCompareURLReady(ctx, client, endpoint.prepared.SessionID); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	if strings.TrimSpace(*waitSelector) != "" {
		for _, prepared := range []preparedCompareSession{oldPrepared, newPrepared} {
			if err := waitForCompareSelector(ctx, client, prepared.SessionID, *waitSelector, *waitTimeout); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		}
	}

	oldObservation, err := observeCompareSession(ctx, client, oldPrepared.SessionID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	newObservation, err := observeCompareSession(ctx, client, newPrepared.SessionID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	report := buildCompareReport(
		buildCompareSnapshot(oldObservation, ignorePatterns),
		buildCompareSnapshot(newObservation, ignorePatterns),
	)

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

func buildCompareSnapshot(observation api.Observation, ignore []*regexp.Regexp) compareSnapshot {
	nodes := make([]compareSnapshotNode, 0, len(observation.Tree))
	for _, node := range observation.Tree {
		fingerprint := strings.TrimSpace(node.Fingerprint)
		if fingerprint == "" {
			fingerprint = strings.Join([]string{
				strings.TrimSpace(node.Role),
				strings.TrimSpace(node.Name),
				strings.TrimSpace(node.Text),
				strings.TrimSpace(node.Value),
			}, "|")
		}

		name := normalizeCompareString(node.Name, ignore)
		text := normalizeCompareString(node.Text, ignore)
		value := normalizeCompareString(node.Value, ignore)
		href := normalizeCompareString(node.Attrs["href"], ignore)
		testID := normalizeCompareString(firstNonEmpty(node.Attrs["data-testid"], node.Attrs["data-test"]), ignore)

		nodes = append(nodes, compareSnapshotNode{
			Fingerprint: fingerprint,
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
		URL:       normalizeCompareString(observation.URLOrScreen, ignore),
		Title:     normalizeCompareString(observation.Title, ignore),
		Text:      normalizeCompareString(observation.Text, ignore),
		Nodes:     nodes,
	}
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
			fmt.Fprintf(w, "[title_changed] %q -> %q\n", finding.Old, finding.New)
		case "page_text_changed":
			fmt.Fprintf(w, "[page_text_changed] %q -> %q\n", finding.Old, finding.New)
		case "missing_node":
			fmt.Fprintf(w, "[missing_node] %s %q\n", finding.Role, finding.Label)
		case "new_node":
			fmt.Fprintf(w, "[new_node] %s %q\n", finding.Role, finding.Label)
		case "text_changed":
			fmt.Fprintf(w, "[text_changed] %s %q %s: %q -> %q\n", finding.Role, finding.Label, finding.Field, finding.Old, finding.New)
		case "state_changed":
			fmt.Fprintf(w, "[state_changed] %s %q: %s -> %s\n", finding.Role, finding.Label, finding.Old, finding.New)
		}
	}
}
