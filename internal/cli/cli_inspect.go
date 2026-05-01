package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/mayahiro/nexus/internal/api"
	comparecmd "github.com/mayahiro/nexus/internal/cli/compare"
)

func runEval(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printEvalHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("eval", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	source := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		source = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "eval"); err != nil {
		return 1
	}

	if source == "" && fs.NArg() == 1 {
		source = fs.Arg(0)
	}

	if source == "" || fs.NArg() > 1 {
		fmt.Fprintln(stderr, "eval requires js code")
		printCommandHint(stderr, "eval", `nxctl eval "document.title" --json`)
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: *sessionID,
		Action: api.Action{
			Kind: "eval",
			Text: source,
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

	if *asJSON {
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

func runGet(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printGetHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	selector := fs.String("selector", "", "selector for html or bbox")
	refs := fs.String("refs", "", "comma-separated node refs")
	target := ""
	arg := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		target = args[0]
		args = args[1:]
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		arg = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "get"); err != nil {
		return 1
	}

	positionals := make([]string, 0, 2)
	if target != "" {
		positionals = append(positionals, target)
	}
	if arg != "" {
		positionals = append(positionals, arg)
	}
	positionals = append(positionals, fs.Args()...)
	if len(positionals) == 0 {
		fmt.Fprintln(stderr, "get requires a target")
		printCommandHint(stderr, "get", "nxctl get title")
		return 1
	}

	target = positionals[0]
	selectorValue := strings.TrimSpace(*selector)
	refsValue := strings.TrimSpace(*refs)
	action := api.Action{
		Kind: "get",
		Args: map[string]string{
			"target": target,
		},
	}

	if refsValue != "" {
		return runGetRefs(ctx, *sessionID, target, selectorValue, positionals, refsValue, *asJSON, stdout, stderr)
	}

	switch target {
	case "title":
		if len(positionals) != 1 {
			fmt.Fprintln(stderr, "get title does not accept an index")
			return 1
		}
		if selectorValue != "" {
			fmt.Fprintln(stderr, "get title does not support --selector")
			return 1
		}
	case "html":
		if len(positionals) != 1 {
			fmt.Fprintln(stderr, "get html does not accept an index")
			return 1
		}
		if selectorValue != "" {
			action.Args["selector"] = selectorValue
		}
	case "text", "value", "attributes":
		if len(positionals) != 2 {
			fmt.Fprintf(stderr, "get %s requires an index\n", target)
			printCommandHint(stderr, "get", "nxctl get attributes @e3")
			return 1
		}
		if selectorValue != "" {
			fmt.Fprintf(stderr, "get %s does not support --selector\n", target)
			return 1
		}
		nodeID, _, err := parseNodeSelector(positionals[1])
		if err != nil {
			fmt.Fprintf(stderr, "get %s requires a positive integer index or @eN ref\n", target)
			return 1
		}
		action.NodeID = &nodeID
	case "bbox":
		if selectorValue != "" {
			if len(positionals) != 1 {
				fmt.Fprintln(stderr, "get bbox with --selector does not accept an index")
				return 1
			}
			action.Args["selector"] = selectorValue
		} else {
			if len(positionals) != 2 {
				fmt.Fprintln(stderr, "get bbox requires an index or --selector")
				printCommandHint(stderr, "get", `nxctl get bbox --selector ".hero"`)
				return 1
			}
			nodeID, _, err := parseNodeSelector(positionals[1])
			if err != nil {
				fmt.Fprintln(stderr, "get bbox requires a positive integer index or @eN ref")
				return 1
			}
			action.NodeID = &nodeID
		}
	default:
		fmt.Fprintln(stderr, "get target must be title, html, text, value, attributes, or bbox")
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: *sessionID,
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

	if *asJSON {
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

func runGetRefs(ctx context.Context, sessionID string, target string, selectorValue string, positionals []string, refsValue string, asJSON bool, stdout io.Writer, stderr io.Writer) int {
	switch target {
	case "text", "value", "attributes", "bbox":
	default:
		fmt.Fprintln(stderr, "get --refs supports text, value, attributes, or bbox")
		return 1
	}
	if len(positionals) != 1 {
		fmt.Fprintf(stderr, "get %s --refs does not accept an index\n", target)
		return 1
	}
	if selectorValue != "" {
		fmt.Fprintf(stderr, "get %s --refs does not support --selector\n", target)
		return 1
	}

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
				Kind:   "get",
				NodeID: &nodeID,
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

func runObserve(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printObserveHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("observe", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	withText := fs.Bool("text", true, "include text")
	withTree := fs.Bool("tree", true, "include tree")
	withScreenshot := fs.Bool("screenshot", false, "include screenshot")

	if err := parseCommandFlags(fs, args, stderr, "observe"); err != nil {
		return 1
	}

	if *sessionID == "" {
		fmt.Fprintln(stderr, "--session is required")
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ObserveSession(ctx, api.ObserveSessionRequest{
		SessionID: *sessionID,
		Options: api.ObserveOptions{
			WithText:       *withText,
			WithTree:       *withTree,
			WithScreenshot: *withScreenshot,
		},
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(res.Observation); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
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

func runState(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printStateHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("state", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	role := fs.String("role", "", "filter by role")
	name := fs.String("name", "", "filter by accessible name")
	text := fs.String("text", "", "filter by text")
	testID := fs.String("testid", "", "filter by data-testid or data-test")
	href := fs.String("href", "", "filter by href")
	limit := fs.Int("limit", 0, "maximum nodes to print")

	if err := parseCommandFlags(fs, args, stderr, "state"); err != nil {
		return 1
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "state does not accept positional arguments")
		printCommandHint(stderr, "state", "nxctl state --role button --limit 20")
		return 1
	}
	if *limit < 0 {
		fmt.Fprintln(stderr, "state limit must be a non-negative integer")
		printCommandHint(stderr, "state", "nxctl state --limit 20")
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ObserveSession(ctx, api.ObserveSessionRequest{
		SessionID: *sessionID,
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
		Role:   *role,
		Name:   *name,
		Text:   *text,
		TestID: *testID,
		Href:   *href,
		Limit:  *limit,
	})

	if *asJSON {
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

type inspectStringValues []string

func (v *inspectStringValues) String() string {
	return strings.Join(*v, ", ")
}

func (v *inspectStringValues) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("inspect value must not be empty")
	}
	*v = append(*v, trimmed)
	return nil
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

func runInspect(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printInspectHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)

	oldSession := fs.String("old-session", "", "old session id")
	newSession := fs.String("new-session", "", "new session id")
	selector := fs.String("selector", "", "raw css selector to inspect")
	scopeSelector := fs.String("scope-selector", "", "raw css selector subtree for locator inspection")
	oldScopeSelector := fs.String("old-scope-selector", "", "old side css selector subtree")
	newScopeSelector := fs.String("new-scope-selector", "", "new side css selector subtree")
	asJSON := fs.Bool("json", false, "print as json")
	withLayoutContext := fs.Bool("layout-context", false, "include ancestor layout context")
	nth := fs.Int("nth", 0, "choose the nth matching node")
	var cssProperty inspectStringValues
	fs.Var(&cssProperty, "css-property", "computed css property to compare")

	locatorValue := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		locatorValue = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "inspect"); err != nil {
		return 1
	}

	positionals := make([]string, 0, 1)
	if locatorValue != "" {
		positionals = append(positionals, locatorValue)
	}
	positionals = append(positionals, fs.Args()...)
	if len(positionals) > 1 {
		fmt.Fprintln(stderr, "inspect requires exactly one locator or selector")
		printCommandHint(stderr, "inspect", `nxctl inspect 'role button --name "Submit"' --old-session old --new-session new`)
		return 1
	}
	hasLocator := len(positionals) == 1
	selectorValue := strings.TrimSpace(*selector)
	scopeSelectorValue := strings.TrimSpace(*scopeSelector)
	oldScopeSelectorValue := strings.TrimSpace(*oldScopeSelector)
	newScopeSelectorValue := strings.TrimSpace(*newScopeSelector)
	if !hasLocator && selectorValue == "" && scopeSelectorValue == "" && oldScopeSelectorValue == "" && newScopeSelectorValue == "" {
		fmt.Fprintln(stderr, "inspect requires exactly one locator or --selector")
		printCommandHint(stderr, "inspect", `nxctl inspect 'role button --name "Submit"' --old-session old --new-session new`)
		return 1
	}
	if strings.TrimSpace(*oldSession) == "" || strings.TrimSpace(*newSession) == "" {
		fmt.Fprintln(stderr, "inspect requires --old-session and --new-session")
		printCommandHint(stderr, "inspect", `nxctl inspect 'role button --name "Submit"' --old-session old --new-session new`)
		return 1
	}
	if hasLocator && selectorValue != "" {
		fmt.Fprintln(stderr, "inspect can not combine a locator with --selector")
		return 1
	}
	if selectorValue != "" && scopeSelectorValue != "" {
		fmt.Fprintln(stderr, "inspect can not combine --selector with --scope-selector")
		return 1
	}
	targetSelectorMode := !hasLocator
	if targetSelectorMode && *nth > 0 {
		fmt.Fprintln(stderr, "inspect --selector does not support --nth")
		return 1
	}
	if isInvalidNthFlag(fs, *nth) {
		fmt.Fprintln(stderr, "inspect --nth must be a positive integer")
		return 1
	}

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
		locator, err = parseInspectLocator(positionals[0])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	cssProperties := comparecmd.ResolveCSSProperties(true, append([]string(nil), cssProperty...))
	layoutProperties := inspectResolveLayoutProperties(*withLayoutContext)
	oldObservation, err := inspectObservation(ctx, client, *oldSession, cssProperties, oldEffectiveScopeSelector, *withLayoutContext, layoutProperties)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	newObservation, err := inspectObservation(ctx, client, *newSession, cssProperties, newEffectiveScopeSelector, *withLayoutContext, layoutProperties)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	selection := nodeSelectionOptions{Nth: *nth}
	oldNode, err := resolveInspectNode(oldObservation.Tree, locator, selection)
	if err != nil {
		fmt.Fprintf(stderr, "old session %s: %v\n", *oldSession, err)
		return 1
	}
	newNode, err := resolveInspectNode(newObservation.Tree, locator, selection)
	if err != nil {
		fmt.Fprintf(stderr, "new session %s: %v\n", *newSession, err)
		return 1
	}

	report := buildInspectReport(locator, cssProperties, layoutProperties, inspectMatch{
		SessionID: *oldSession,
		URL:       oldObservation.URLOrScreen,
		Title:     oldObservation.Title,
		Node:      oldNode,
	}, inspectMatch{
		SessionID: *newSession,
		URL:       newObservation.URLOrScreen,
		Title:     newObservation.Title,
		Node:      newNode,
	}, inspectScopeFromObservations(oldEffectiveScopeSelector, newEffectiveScopeSelector, oldObservation, newObservation))

	if *asJSON {
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
	fs := flag.NewFlagSet("inspect role locator", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	name := fs.String("name", "", "accessible name")
	if err := fs.Parse(normalizeFlagArgs(fs, args)); err != nil {
		return "", err
	}
	if fs.NArg() != 0 {
		return "", fmt.Errorf("unexpected extra arguments")
	}
	return strings.TrimSpace(*name), nil
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
