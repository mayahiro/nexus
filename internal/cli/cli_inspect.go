package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	nagicli "github.com/mayahiro/nagicli-go"

	"github.com/mayahiro/nexus/internal/api"
	comparecmd "github.com/mayahiro/nexus/internal/cli/compare"
)

func runEvalInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	sessionID := nagiStringValue(invocation, "session")
	asJSON := nagiBoolValue(invocation, "json")
	source := nagiStringValue(invocation, "source")
	world := nagiStringValue(invocation, "world")

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: sessionID,
		Action: api.Action{
			Kind: "eval",
			Text: source,
			Args: map[string]string{"world": world},
		},
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if !res.Result.OK {
		if res.Result.Message != "" {
			fmt.Fprintln(stderr, res.Result.Message)
		}
		return 1
	}

	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(res.Result.Value); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	if err := printEvalValue(stdout, res.Result.Value); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	return 0
}

func runGetInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	sessionID := nagiStringValue(invocation, "session")
	asJSON := nagiBoolValue(invocation, "json")
	selectorValue := strings.TrimSpace(nagiStringValue(invocation, "selector"))
	refsValue := strings.TrimSpace(nagiStringValue(invocation, "refs"))
	target := nagiStringValue(invocation, "target")
	action := api.Action{
		Kind: "get",
		Args: map[string]string{
			"target": target,
		},
	}

	if refsValue != "" {
		return runGetRefs(ctx, sessionID, target, refsValue, asJSON, stdout, stderr)
	}

	switch target {
	case "title":
	case "html":
		if selectorValue != "" {
			action.Args["selector"] = selectorValue
		}
	case "text", "value", "attributes":
		node := nagiNodeValue(invocation, "node")
		action.NodeID = &node.ID
		action.NodeRef = node.Ref
	case "bbox":
		if selectorValue != "" {
			action.Args["selector"] = selectorValue
		} else {
			node := nagiNodeValue(invocation, "node")
			action.NodeID = &node.ID
			action.NodeRef = node.Ref
		}
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: sessionID,
		Action:    action,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if !res.Result.OK {
		if res.Result.Message != "" {
			fmt.Fprintln(stderr, res.Result.Message)
		}
		return 1
	}

	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(res.Result.Value); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	if err := printEvalValue(stdout, res.Result.Value); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	return 0
}

type refValueResult struct {
	Ref   string      `json:"ref"`
	Value interface{} `json:"value"`
}

func runGetRefs(ctx context.Context, sessionID string, target string, refsValue string, asJSON bool, stdout io.Writer, stderr io.Writer) int {
	nodes, err := parseNodeSelectorList(refsValue)
	if err != nil {
		fmt.Fprintln(stderr, "get --refs requires comma-separated positive integer indexes or @eN refs")
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	results := make([]refValueResult, 0, len(nodes))
	for _, node := range nodes {
		nodeID := node.ID
		res, err := client.ActSession(ctx, api.ActSessionRequest{
			SessionID: sessionID,
			Action: api.Action{
				Kind:    "get",
				NodeID:  &nodeID,
				NodeRef: node.Ref,
				Args: map[string]string{
					"target": target,
				},
			},
		})
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if !res.Result.OK {
			if res.Result.Message != "" {
				fmt.Fprintln(stderr, res.Result.Message)
			}
			return 1
		}
		results = append(results, refValueResult{
			Ref:   node.Ref,
			Value: res.Result.Value,
		})
	}

	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(results); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	for _, result := range results {
		if err := printRefValue(stdout, result.Ref, result.Value); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	return 0
}

func printRefValue(w io.Writer, ref string, value interface{}) error {
	switch value := value.(type) {
	case nil:
		_, err := fmt.Fprintf(w, "%s\tnull\n", ref)
		return err
	case string:
		_, err := fmt.Fprintf(w, "%s\t%s\n", ref, value)
		return err
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "%s\t%s\n", ref, data)
		return err
	}
}

func runObserveInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	sessionID := nagiStringValue(invocation, "session")
	asJSON := nagiBoolValue(invocation, "json")
	withScreenshot := nagiBoolValue(invocation, "screenshot")
	fullScreenshot := nagiBoolValue(invocation, "full")
	recoverScreenshot := nagiBoolValue(invocation, "recover-target")
	verbose := nagiBoolValue(invocation, "verbose")
	timeoutMS := nagiIntValue(invocation, "timeout")

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	observeCtx := ctx
	cancel := func() {}
	if withScreenshot || fullScreenshot {
		observeCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	}
	defer cancel()

	res, err := client.ObserveSession(observeCtx, api.ObserveSessionRequest{
		SessionID: sessionID,
		Options: api.ObserveOptions{
			WithText:          true,
			WithTree:          true,
			WithScreenshot:    withScreenshot || fullScreenshot,
			FullScreenshot:    fullScreenshot,
			RecoverScreenshot: recoverScreenshot,
			TimeoutMS:         timeoutMS,
			Verbose:           verbose,
		},
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(res.Observation); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	if warning := strings.TrimSpace(res.Observation.Meta["screenshot_recovery_warning"]); warning != "" {
		fmt.Fprintf(stderr, "warning: %s\n", warning)
	}

	fmt.Fprintf(stdout, "session: %s\n", res.Observation.SessionID)
	fmt.Fprintf(stdout, "target: %s\n", res.Observation.TargetType)
	fmt.Fprintf(stdout, "url: %s\n", res.Observation.URLOrScreen)
	fmt.Fprintf(stdout, "title: %s\n", res.Observation.Title)
	if res.Observation.Text != "" {
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, res.Observation.Text)
	}
	return 0
}

func runStateInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	sessionID := nagiStringValue(invocation, "session")
	asJSON := nagiBoolValue(invocation, "json")
	role := nagiStringValue(invocation, "role")
	name := nagiStringValue(invocation, "name")
	textValue := nagiStringValue(invocation, "text")
	testID := nagiStringValue(invocation, "testid")
	href := nagiStringValue(invocation, "href")
	limit := nagiIntValue(invocation, "limit")

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ObserveSession(ctx, api.ObserveSessionRequest{
		SessionID: sessionID,
		Options: api.ObserveOptions{
			WithText:       true,
			WithTree:       true,
			WithScreenshot: false,
		},
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	res.Observation.Tree = filterStateNodes(res.Observation.Tree, stateFilterOptions{
		Role:   role,
		Name:   name,
		Text:   textValue,
		TestID: testID,
		Href:   href,
		Limit:  limit,
	})

	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(res.Observation); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(stdout, "URL: %s\n", res.Observation.URLOrScreen)
	fmt.Fprintf(stdout, "Title: %s\n", res.Observation.Title)
	if len(res.Observation.Tree) == 0 {
		return 0
	}

	fmt.Fprintln(stdout, "")
	for _, node := range res.Observation.Tree {
		printNode(stdout, node)
	}

	return 0
}

type inspectLocator struct {
	Raw   string `json:"raw"`
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
	Role  string `json:"role,omitempty"`
	Name  string `json:"name,omitempty"`
	Ref   string `json:"ref,omitempty"`
}

type inspectMatch struct {
	SessionID string   `json:"session_id"`
	URL       string   `json:"url,omitempty"`
	Title     string   `json:"title,omitempty"`
	Node      api.Node `json:"node"`
}

type inspectScopeSide struct {
	Selector string `json:"selector,omitempty"`
	Tag      string `json:"tag,omitempty"`
}

type inspectScope struct {
	Selector string           `json:"selector,omitempty"`
	Old      inspectScopeSide `json:"old"`
	New      inspectScopeSide `json:"new"`
}

type inspectPropertyReport struct {
	Name string `json:"name"`
	Old  string `json:"old,omitempty"`
	New  string `json:"new,omitempty"`
	Same bool   `json:"same"`
}

type inspectReport struct {
	Locator          inspectLocator          `json:"locator"`
	Scope            *inspectScope           `json:"scope,omitempty"`
	CSSProperties    []string                `json:"css_properties"`
	LayoutProperties []string                `json:"layout_properties,omitempty"`
	Old              inspectMatch            `json:"old"`
	New              inspectMatch            `json:"new"`
	Properties       []inspectPropertyReport `json:"properties"`
	Same             bool                    `json:"same"`
}

var inspectDefaultLayoutProperties = []string{
	"display",
	"position",
	"top",
	"right",
	"bottom",
	"left",
	"box-sizing",
	"width",
	"height",
	"min-width",
	"max-width",
	"min-height",
	"max-height",
	"margin-top",
	"margin-right",
	"margin-bottom",
	"margin-left",
	"padding-top",
	"padding-right",
	"padding-bottom",
	"padding-left",
	"overflow-x",
	"overflow-y",
	"flex-direction",
	"flex-wrap",
	"justify-content",
	"align-items",
	"align-content",
	"gap",
	"row-gap",
	"column-gap",
	"grid-template-columns",
	"grid-template-rows",
	"grid-auto-flow",
	"contain",
	"container-type",
	"transform",
}

func runInspectInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	oldSession := nagiStringValue(invocation, "old-session")
	newSession := nagiStringValue(invocation, "new-session")
	locatorValue := strings.TrimSpace(nagiRawValue(invocation, "locator"))
	selectorValue := strings.TrimSpace(nagiStringValue(invocation, "selector"))
	scopeSelectorValue := strings.TrimSpace(nagiStringValue(invocation, "scope-selector"))
	oldScopeSelectorValue := strings.TrimSpace(nagiStringValue(invocation, "old-scope-selector"))
	newScopeSelectorValue := strings.TrimSpace(nagiStringValue(invocation, "new-scope-selector"))
	asJSON := nagiBoolValue(invocation, "json")
	withLayoutContext := nagiBoolValue(invocation, "layout-context")
	nth := nagiIntValue(invocation, "nth")
	cssProperty := nagiStringValues(invocation, "css-property")

	hasLocator := locatorValue != ""
	targetSelectorMode := !hasLocator

	locator := inspectLocator{}
	commonScopeSelector := scopeSelectorValue
	if targetSelectorMode {
		commonScopeSelector = firstNonEmpty(selectorValue, scopeSelectorValue)
	}
	oldEffectiveScopeSelector, newEffectiveScopeSelector, err := resolveInspectScopeSelectors(commonScopeSelector, oldScopeSelectorValue, newScopeSelectorValue)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if targetSelectorMode {
		locator = inspectSelectorLocator(oldEffectiveScopeSelector, newEffectiveScopeSelector)
	} else {
		locator, _ = nagicli.ValueAs[inspectLocator](invocation, "locator")
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	cssProperties := comparecmd.ResolveCSSProperties(true, append([]string(nil), cssProperty...))
	layoutProperties := inspectResolveLayoutProperties(withLayoutContext)
	oldObservation, err := inspectObservation(ctx, client, oldSession, cssProperties, oldEffectiveScopeSelector, withLayoutContext, layoutProperties)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	newObservation, err := inspectObservation(ctx, client, newSession, cssProperties, newEffectiveScopeSelector, withLayoutContext, layoutProperties)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	selection := nodeSelectionOptions{Nth: nth}
	oldNode, err := resolveInspectNode(oldObservation.Tree, locator, selection)
	if err != nil {
		fmt.Fprintf(stderr, "old session %s: %v\n", oldSession, err)
		return 1
	}
	newNode, err := resolveInspectNode(newObservation.Tree, locator, selection)
	if err != nil {
		fmt.Fprintf(stderr, "new session %s: %v\n", newSession, err)
		return 1
	}

	report := buildInspectReport(locator, cssProperties, layoutProperties, inspectMatch{
		SessionID: oldSession,
		URL:       oldObservation.URLOrScreen,
		Title:     oldObservation.Title,
		Node:      oldNode,
	}, inspectMatch{
		SessionID: newSession,
		URL:       newObservation.URLOrScreen,
		Title:     newObservation.Title,
		Node:      newNode,
	}, inspectScopeFromObservations(oldEffectiveScopeSelector, newEffectiveScopeSelector, oldObservation, newObservation))

	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	printInspectReport(stdout, report)
	return 0
}

func resolveInspectScopeSelectors(scopeSelector string, oldScopeSelector string, newScopeSelector string) (string, string, error) {
	common := strings.TrimSpace(scopeSelector)
	oldSelector := strings.TrimSpace(oldScopeSelector)
	newSelector := strings.TrimSpace(newScopeSelector)
	if oldSelector == "" {
		oldSelector = common
	}
	if newSelector == "" {
		newSelector = common
	}
	if oldSelector == "" && newSelector != "" {
		return "", "", fmt.Errorf("inspect requires --old-scope-selector, --scope-selector, or --selector when --new-scope-selector is set")
	}
	if oldSelector != "" && newSelector == "" {
		return "", "", fmt.Errorf("inspect requires --new-scope-selector, --scope-selector, or --selector when --old-scope-selector is set")
	}
	return oldSelector, newSelector, nil
}

func inspectSelectorLocator(oldSelector string, newSelector string) inspectLocator {
	if oldSelector == newSelector {
		return inspectLocator{Raw: "selector: " + oldSelector, Kind: "selector", Value: oldSelector}
	}
	return inspectLocator{Raw: "selector: old=" + oldSelector + " new=" + newSelector, Kind: "selector"}
}

func inspectScopeFromObservations(oldSelector string, newSelector string, oldObservation api.Observation, newObservation api.Observation) *inspectScope {
	oldSelector = firstNonEmpty(strings.TrimSpace(oldObservation.Meta["scope_selector"]), strings.TrimSpace(oldSelector))
	newSelector = firstNonEmpty(strings.TrimSpace(newObservation.Meta["scope_selector"]), strings.TrimSpace(newSelector))
	if oldSelector == "" && newSelector == "" {
		return nil
	}
	scope := &inspectScope{
		Old: inspectScopeSide{
			Selector: oldSelector,
			Tag:      strings.TrimSpace(oldObservation.Meta["scope_tag"]),
		},
		New: inspectScopeSide{
			Selector: newSelector,
			Tag:      strings.TrimSpace(newObservation.Meta["scope_tag"]),
		},
	}
	if oldSelector == newSelector {
		scope.Selector = oldSelector
	}
	return scope
}

func inspectObservation(ctx context.Context, client clientObserver, sessionID string, cssProperties []string, scopeSelector string, withLayoutContext bool, layoutProperties []string) (api.Observation, error) {
	res, err := client.ObserveSession(ctx, api.ObserveSessionRequest{
		SessionID: sessionID,
		Options: api.ObserveOptions{
			WithTree:          true,
			WithLayoutContext: withLayoutContext,
			CSSProperties:     append([]string(nil), cssProperties...),
			LayoutProperties:  append([]string(nil), layoutProperties...),
			ScopeSelector:     strings.TrimSpace(scopeSelector),
		},
	})
	if err != nil {
		return api.Observation{}, err
	}
	return res.Observation, nil
}

func inspectResolveLayoutProperties(enabled bool) []string {
	if !enabled {
		return nil
	}
	return append([]string(nil), inspectDefaultLayoutProperties...)
}

type clientObserver interface {
	ObserveSession(context.Context, api.ObserveSessionRequest) (api.ObserveSessionResponse, error)
}

func parseInspectLocator(value string) (inspectLocator, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return inspectLocator{}, fmt.Errorf("inspect locator must not be empty")
	}
	if strings.HasPrefix(trimmed, "@e") {
		if _, _, err := parseNodeSelector(trimmed); err != nil {
			return inspectLocator{}, fmt.Errorf("invalid inspect locator %q", value)
		}
		return inspectLocator{Raw: trimmed, Kind: "ref", Ref: trimmed}, nil
	}

	args, err := splitBatchCommand(trimmed)
	if err != nil {
		return inspectLocator{}, fmt.Errorf("invalid inspect locator %q: %w", value, err)
	}
	if len(args) == 0 {
		return inspectLocator{}, fmt.Errorf("inspect locator must not be empty")
	}

	switch args[0] {
	case "role":
		if len(args) < 2 {
			return inspectLocator{}, fmt.Errorf(`invalid inspect locator %q: role locator requires "role <role> [--name <text>]"`, value)
		}
		role := strings.TrimSpace(args[1])
		name, err := parseInspectRoleName(args[2:])
		if err != nil {
			return inspectLocator{}, fmt.Errorf("invalid inspect locator %q: %w", value, err)
		}
		return inspectLocator{Raw: trimmed, Kind: "role", Role: role, Name: name}, nil
	case "text", "label", "testid", "href":
		if len(args) != 2 {
			return inspectLocator{}, fmt.Errorf("invalid inspect locator %q", value)
		}
		return inspectLocator{Raw: trimmed, Kind: args[0], Value: strings.TrimSpace(args[1])}, nil
	default:
		return inspectLocator{}, fmt.Errorf("inspect locator must be @eN, role ..., text ..., label ..., testid ..., or href ...")
	}
}

func parseInspectRoleName(args []string) (string, error) {
	command := nagicli.NewCommand("role").
		UsageVariant("default", "[--name <TEXT>]").
		Option(nagiValueOption("name", "TEXT", "Accessible name"))
	result, err := command.Parse(args)
	if err != nil {
		return "", err
	}
	if result.Kind() != nagicli.ParseInvocation {
		return "", fmt.Errorf("invalid role locator")
	}
	return strings.TrimSpace(nagiStringValue(result.Invocation(), "name")), nil
}

func resolveInspectNode(nodes []api.Node, locator inspectLocator, selection nodeSelectionOptions) (api.Node, error) {
	switch locator.Kind {
	case "ref":
		nodeID, _, err := parseNodeSelector(locator.Ref)
		if err != nil {
			return api.Node{}, err
		}
		for _, node := range nodes {
			if node.ID == nodeID || strings.TrimSpace(node.Ref) == locator.Ref {
				return node, nil
			}
		}
		return api.Node{}, fmt.Errorf("matching node not found")
	case "role":
		matches := selectNodes(nodes, func(node api.Node) bool {
			if !strings.EqualFold(strings.TrimSpace(node.Role), locator.Role) {
				return false
			}
			if locator.Name == "" {
				return true
			}
			return nodeMatches(node, locator.Name)
		})
		return chooseNode(matches, inspectFirstNonEmpty(locator.Name, locator.Role), selection)
	case "text":
		matches := selectNodes(nodes, func(node api.Node) bool {
			return nodeMatches(node, locator.Value)
		})
		return chooseNode(matches, locator.Value, selection)
	case "label":
		matches := selectNodes(nodes, func(node api.Node) bool {
			if !node.Editable && !node.Selectable && !strings.EqualFold(node.Role, "textbox") && !strings.EqualFold(node.Role, "combobox") {
				return false
			}
			return nodeMatches(node, locator.Value)
		})
		return chooseNode(matches, locator.Value, selection)
	case "testid":
		matches := selectNodes(nodes, func(node api.Node) bool {
			return nodeMatches(api.Node{
				Name:  inspectFirstNonEmpty(node.Attrs["data-testid"], node.Attrs["data-test"]),
				Attrs: node.Attrs,
			}, locator.Value)
		})
		return chooseNode(matches, locator.Value, selection)
	case "href":
		matches := selectNodes(nodes, func(node api.Node) bool {
			return nodeMatches(api.Node{Name: node.Attrs["href"], Attrs: node.Attrs}, locator.Value)
		})
		return chooseNode(matches, locator.Value, selection)
	case "selector":
		if len(nodes) == 0 {
			return api.Node{}, fmt.Errorf("matching node not found")
		}
		return nodes[0], nil
	default:
		return api.Node{}, fmt.Errorf("unsupported inspect locator")
	}
}

func buildInspectReport(locator inspectLocator, cssProperties []string, layoutProperties []string, oldMatch inspectMatch, newMatch inspectMatch, scope *inspectScope) inspectReport {
	properties := make([]inspectPropertyReport, 0, len(cssProperties))
	same := true
	for _, property := range cssProperties {
		oldValue := strings.TrimSpace(oldMatch.Node.Styles[property])
		newValue := strings.TrimSpace(newMatch.Node.Styles[property])
		entry := inspectPropertyReport{
			Name: property,
			Old:  oldValue,
			New:  newValue,
			Same: oldValue == newValue,
		}
		if !entry.Same {
			same = false
		}
		properties = append(properties, entry)
	}
	return inspectReport{
		Locator:          locator,
		Scope:            scope,
		CSSProperties:    append([]string(nil), cssProperties...),
		LayoutProperties: append([]string(nil), layoutProperties...),
		Old:              oldMatch,
		New:              newMatch,
		Properties:       properties,
		Same:             same,
	}
}

func printInspectReport(w io.Writer, report inspectReport) {
	fmt.Fprintf(w, "locator: %s\n", report.Locator.Raw)
	if report.Scope != nil {
		fmt.Fprintf(w, "scope: %s\n", inspectScopeLabel(report.Scope))
	}
	fmt.Fprintf(w, "old: %s %s\n", report.Old.SessionID, inspectNodeSummary(report.Old.Node))
	fmt.Fprintf(w, "new: %s %s\n", report.New.SessionID, inspectNodeSummary(report.New.Node))
	fmt.Fprintf(w, "same: %t\n", report.Same)
	if len(report.Properties) > 0 {
		fmt.Fprintln(w, "")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "property\told\tnew\tstatus")
		for _, property := range report.Properties {
			status := "same"
			if !property.Same {
				status = "changed"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", property.Name, property.Old, property.New, status)
		}
		tw.Flush()
	}

	if len(report.LayoutProperties) > 0 {
		fmt.Fprintln(w, "")
		printInspectLayoutContext(w, "old layout context", report.Old.Node.LayoutContext)
		printInspectLayoutContext(w, "new layout context", report.New.Node.LayoutContext)
	}
}

func inspectScopeLabel(scope *inspectScope) string {
	if scope == nil {
		return ""
	}
	if strings.TrimSpace(scope.Selector) != "" {
		return strings.TrimSpace(scope.Selector)
	}
	return "old=" + strings.TrimSpace(scope.Old.Selector) + " new=" + strings.TrimSpace(scope.New.Selector)
}

func printInspectLayoutContext(w io.Writer, title string, nodes []api.LayoutContextNode) {
	fmt.Fprintf(w, "%s:\n", title)
	if len(nodes) == 0 {
		fmt.Fprintln(w, "  unavailable")
		return
	}
	for index, node := range nodes {
		label := inspectLayoutContextLabel(node)
		styles := inspectFormatLayoutStyles(node.Styles)
		if styles == "" {
			fmt.Fprintf(w, "  %d. %s\n", index+1, label)
			continue
		}
		fmt.Fprintf(w, "  %d. %s %s\n", index+1, label, styles)
	}
}

func inspectLayoutContextLabel(node api.LayoutContextNode) string {
	if strings.TrimSpace(node.Selector) != "" {
		return strings.TrimSpace(node.Selector)
	}
	if node.Attrs != nil && strings.TrimSpace(node.Attrs["tag"]) != "" {
		return strings.TrimSpace(node.Attrs["tag"])
	}
	if strings.TrimSpace(node.Role) != "" {
		return strings.TrimSpace(node.Role)
	}
	return "ancestor"
}

func inspectFormatLayoutStyles(styles map[string]string) string {
	if len(styles) == 0 {
		return ""
	}

	values := make([]string, 0, len(inspectDefaultLayoutProperties))
	for _, property := range inspectDefaultLayoutProperties {
		value := strings.TrimSpace(styles[property])
		if !inspectShouldPrintLayoutStyle(property, value, styles) {
			continue
		}
		values = append(values, property+"="+strconv.Quote(value))
	}
	return strings.Join(values, " ")
}

func inspectShouldPrintLayoutStyle(property string, value string, styles map[string]string) bool {
	if value == "" {
		return false
	}
	switch property {
	case "display", "position", "width", "height":
		return true
	case "top", "right", "bottom", "left":
		return value != "auto"
	case "box-sizing":
		return value != "content-box"
	case "min-width", "min-height":
		return value != "0px"
	case "max-width", "max-height":
		return value != "none"
	case "margin-top", "margin-right", "margin-bottom", "margin-left", "padding-top", "padding-right", "padding-bottom", "padding-left":
		return value != "0px"
	case "overflow-x", "overflow-y":
		return value != "visible"
	case "flex-direction", "flex-wrap", "justify-content", "align-items", "align-content":
		return strings.Contains(styles["display"], "flex")
	case "gap", "row-gap", "column-gap":
		return value != "normal" && value != "0px"
	case "grid-template-columns", "grid-template-rows", "grid-auto-flow":
		return strings.Contains(styles["display"], "grid")
	case "contain":
		return value != "none"
	case "container-type":
		return value != "normal"
	case "transform":
		return value != "none"
	default:
		return true
	}
}

func inspectNodeSummary(node api.Node) string {
	label := displayNodeRef(node)
	text := inspectFirstNonEmpty(node.Name, node.Text, node.Value)
	if text == "" {
		return fmt.Sprintf("%s %s", label, node.Role)
	}
	return fmt.Sprintf("%s %s %q", label, node.Role, text)
}

func inspectFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

type stateFilterOptions struct {
	Role   string
	Name   string
	Text   string
	TestID string
	Href   string
	Limit  int
}

func filterStateNodes(nodes []api.Node, options stateFilterOptions) []api.Node {
	filtered := make([]api.Node, 0, len(nodes))
	role := normalizeFindValue(options.Role)
	name := normalizeFindValue(options.Name)
	text := normalizeFindValue(options.Text)
	testID := normalizeFindValue(options.TestID)
	href := normalizeFindValue(options.Href)

	for _, node := range nodes {
		if role != "" && normalizeFindValue(node.Role) != role {
			continue
		}
		if name != "" && !matchStateField(node.Name, name) {
			continue
		}
		if text != "" && !matchStateField(node.Text, text) {
			continue
		}
		if testID != "" && !matchStateField(stateNodeTestID(node), testID) {
			continue
		}
		if href != "" && !matchStateField(node.Attrs["href"], href) {
			continue
		}
		filtered = append(filtered, node)
	}

	if options.Limit > 0 && len(filtered) > options.Limit {
		return filtered[:options.Limit]
	}
	return filtered
}

func matchStateField(value string, needle string) bool {
	normalized := normalizeFindValue(value)
	if needle == "" {
		return true
	}
	return normalized != "" && strings.Contains(normalized, needle)
}

func stateNodeTestID(node api.Node) string {
	if node.Attrs["data-testid"] != "" {
		return node.Attrs["data-testid"]
	}
	return node.Attrs["data-test"]
}

func printNode(w io.Writer, node api.Node) {
	label := node.Ref
	if label == "" {
		label = fmt.Sprintf("%d", node.ID)
	}
	fmt.Fprintf(w, "[%s] %s", label, node.Role)
	if node.Name != "" {
		fmt.Fprintf(w, " %q", node.Name)
	} else if node.Text != "" {
		fmt.Fprintf(w, " %q", node.Text)
	}
	fmt.Fprintln(w)
	if len(node.LocatorHints) > 0 {
		commands := make([]string, 0, len(node.LocatorHints))
		for _, hint := range node.LocatorHints {
			commands = append(commands, hint.Command)
		}
		fmt.Fprintf(w, "  find: %s\n", strings.Join(commands, " | "))
	}
}
