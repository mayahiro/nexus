package comparecmd

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/rpc"
)

func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, connectClient func(context.Context) (*rpc.Client, error)) int {
	if isHelpArgs(args) {
		PrintHelp(stdout)
		return 0
	}
	if len(args) > 0 && args[0] == "validate-decisions" {
		return runCompareValidateDecisions(args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "normalize-decisions" {
		return runCompareNormalizeDecisions(args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "materialize-decisions" {
		return runCompareMaterializeDecisionsWithClient(ctx, args[1:], stdout, stderr, connectClient)
	}
	if len(args) > 0 && args[0] == "audit-decisions" {
		return runCompareAuditDecisions(args[1:], stdout, stderr)
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
	matchMode := fs.String("match-mode", defaultCompareMatchMode, "node match mode: exact, stable, heuristic, or histogram")
	nodeScope := fs.String("node-scope", defaultCompareNodeScope, "node scope: current, actionable, semantic, or all")
	matchingDebug := fs.Bool("matching-debug", false, "include matching debug details in json and markdown reports")
	decisionsFile := fs.String("decisions-file", "", "read AI or human pairing decisions from a JSONL file")
	outputDecisionsTemplate := fs.String("output-decisions-template", "", "write a JSONL decisions template from ambiguous and unmatched matching debug nodes")
	outputFindingDecisionsTemplate := fs.String("output-finding-decisions-template", "", "write a JSONL decisions template from current findings")
	manifestPath := fs.String("manifest", "", "compare manifest json")
	continueOnError := fs.Bool("continue-on-error", false, "continue after manifest page error")
	limit := fs.Int("limit", 0, "limit manifest pages")
	waitSelector := fs.String("wait-selector", "", "wait selector before compare")
	scopeSelector := fs.String("scope-selector", "", "restrict compare to a single CSS selector subtree")
	oldScopeSelector := fs.String("old-scope-selector", "", "old side CSS selector subtree")
	newScopeSelector := fs.String("new-scope-selector", "", "new side CSS selector subtree")
	waitFunction := fs.String("wait-function", "", "wait until javascript expression returns true before compare")
	waitNetworkIdle := fs.Bool("wait-network-idle", false, "wait for a short post-load network idle window before compare")
	compareCSS := fs.Bool("compare-css", false, "compare computed css values for matching nodes")
	compareLayout := fs.Bool("compare-layout", false, "compare viewport-relative element bounds for matching nodes")
	waitTimeout := fs.Int("wait-timeout", 10000, "wait timeout in ms")
	asJSON := fs.Bool("json", false, "print as json")
	outputJSON := fs.String("output-json", "", "write compare report json to file")
	outputMD := fs.String("output-md", "", "write compare report markdown to file")
	reviewDir := fs.String("review-dir", "", "write an AI review packet directory")
	var ignoreRegex compareStringValues
	var cssProperty compareStringValues
	var ignoreSelector compareStringValues
	var maskSelector compareStringValues
	fs.Var(&cssProperty, "css-property", "computed css property to compare")
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
		PrintHelp(stderr)
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
	normalizedMatchMode, err := normalizeCompareMatchMode(*matchMode)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	normalizedNodeScope, err := normalizeCompareNodeScope(*nodeScope)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if strings.TrimSpace(*manifestPath) == "" {
		if err := validateCompareNodeScopeSelectors(normalizedNodeScope, *scopeSelector, *oldScopeSelector, *newScopeSelector); err != nil {
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

	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	base := compareRun{
		Backend:                 *backend,
		TargetRef:               *targetRef,
		Viewport:                *viewport,
		MatchMode:               normalizedMatchMode,
		NodeScope:               normalizedNodeScope,
		MatchingDebug:           *matchingDebug || strings.TrimSpace(*outputDecisionsTemplate) != "" || strings.TrimSpace(*reviewDir) != "",
		DecisionsFile:           *decisionsFile,
		OutputDecisionsTemplate: strings.TrimSpace(*outputDecisionsTemplate),
		ReviewDir:               strings.TrimSpace(*reviewDir),
		WaitSelector:            *waitSelector,
		ScopeSelector:           *scopeSelector,
		OldScopeSelector:        *oldScopeSelector,
		NewScopeSelector:        *newScopeSelector,
		WaitFunction:            *waitFunction,
		WaitNetworkIdle:         *waitNetworkIdle,
		CompareCSS:              *compareCSS,
		CompareLayout:           *compareLayout,
		WaitTimeout:             *waitTimeout,
		CSSProperties:           append([]string(nil), cssProperty...),
		IgnoreTextRegex:         append([]string(nil), ignoreRegex...),
		IgnoreSelector:          append([]string(nil), ignoreSelector...),
		MaskSelector:            append([]string(nil), maskSelector...),
	}

	if strings.TrimSpace(*manifestPath) != "" {
		if strings.TrimSpace(*outputDecisionsTemplate) != "" {
			fmt.Fprintln(stderr, "compare can not use --output-decisions-template with --manifest")
			return 1
		}
		if strings.TrimSpace(*outputFindingDecisionsTemplate) != "" {
			fmt.Fprintln(stderr, "compare can not use --output-finding-decisions-template with --manifest")
			return 1
		}
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
	if strings.TrimSpace(*outputDecisionsTemplate) != "" {
		if err := writeCompareDecisionsTemplate(*outputDecisionsTemplate, report.MatchingDebug); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if strings.TrimSpace(*outputFindingDecisionsTemplate) != "" {
		if err := writeCompareFindingDecisionsTemplate(*outputFindingDecisionsTemplate, report); err != nil {
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

func runCompareValidateDecisions(args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		PrintValidateDecisionsHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("compare validate-decisions", flag.ContinueOnError)
	fs.SetOutput(stderr)

	decisionsFile := fs.String("decisions-file", "", "decisions JSONL file to validate")
	compareJSON := fs.String("compare-json", "", "compare report JSON used to validate refs and fingerprints")
	reviewSummary := fs.String("review-summary", "", "review-summary.json used to validate finding cluster decisions")
	asJSON := fs.Bool("json", false, "print validation report as json")

	if err := parseCommandFlags(fs, args, stderr, "compare validate-decisions"); err != nil {
		return 1
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "compare validate-decisions accepts only flags")
		PrintValidateDecisionsHelp(stderr)
		return 1
	}
	if strings.TrimSpace(*decisionsFile) == "" {
		fmt.Fprintln(stderr, "compare validate-decisions requires --decisions-file")
		return 1
	}

	decisions, err := loadCompareDecisions(*decisionsFile)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	var loadedCompareReport *compareReport
	if strings.TrimSpace(*compareJSON) != "" {
		report, err := loadCompareReport(*compareJSON)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		loadedCompareReport = &report
	}
	reviewClusters, err := loadCompareFindingClusters(*reviewSummary)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	report := validateCompareDecisionsWithClusters(decisions, loadedCompareReport, reviewClusters)
	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else {
		printCompareDecisionValidationReport(stdout, report)
	}
	if report.Summary.Errors > 0 {
		return 1
	}
	return 0
}

func runCompareNormalizeDecisions(args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		PrintNormalizeDecisionsHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("compare normalize-decisions", flag.ContinueOnError)
	fs.SetOutput(stderr)

	decisionsFile := fs.String("decisions-file", "", "decisions JSONL file to normalize")
	compareJSON := fs.String("compare-json", "", "compare report JSON used to validate refs, fingerprints, and finding ids")
	reviewSummary := fs.String("review-summary", "", "review-summary.json used to materialize finding cluster decisions")
	output := fs.String("output", "", "write normalized decisions JSONL to file")
	asJSON := fs.Bool("json", false, "print normalization report as json")

	if err := parseCommandFlags(fs, args, stderr, "compare normalize-decisions"); err != nil {
		return 1
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "compare normalize-decisions accepts only flags")
		PrintNormalizeDecisionsHelp(stderr)
		return 1
	}
	if strings.TrimSpace(*decisionsFile) == "" {
		fmt.Fprintln(stderr, "compare normalize-decisions requires --decisions-file")
		return 1
	}

	decisions, err := loadCompareDecisions(*decisionsFile)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	var loadedCompareReport *compareReport
	if strings.TrimSpace(*compareJSON) != "" {
		report, err := loadCompareReport(*compareJSON)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		loadedCompareReport = &report
	}
	reviewClusters, err := loadCompareFindingClusters(*reviewSummary)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	normalized, duplicates := normalizeCompareDecisionsWithClusters(decisions, loadedCompareReport, reviewClusters)
	validation := validateCompareDecisionsWithClusters(normalized, loadedCompareReport, reviewClusters)
	report := compareDecisionNormalizeReport{
		Summary: compareDecisionNormalizeSummary{
			InputDecisions:    len(decisions),
			OutputDecisions:   len(normalized),
			DuplicatesRemoved: duplicates,
			Output:            strings.TrimSpace(*output),
			Errors:            validation.Summary.Errors,
			Warnings:          validation.Summary.Warnings,
			CompareJSONUsed:   validation.Summary.CompareJSONUsed,
			ReviewSummaryUsed: len(reviewClusters) > 0,
		},
		Issues: validation.Issues,
	}

	if report.Summary.Output != "" {
		if err := writeCompareDecisionJSONL(report.Summary.Output, normalized); err != nil {
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
	} else if report.Summary.Output != "" {
		printCompareDecisionNormalizeReport(stdout, report)
	} else {
		if err := printCompareDecisionJSONL(stdout, normalized); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if len(report.Issues) > 0 {
			fmt.Fprintln(stderr)
			printCompareDecisionNormalizeReport(stderr, report)
		}
	}
	if report.Summary.Errors > 0 {
		return 1
	}
	return 0
}

func runCompareMaterializeDecisions(args []string, stdout io.Writer, stderr io.Writer) int {
	return runCompareMaterializeDecisionsWithClient(context.Background(), args, stdout, stderr, nil)
}

func runCompareMaterializeDecisionsWithClient(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, connectClient func(context.Context) (*rpc.Client, error)) int {
	if isHelpArgs(args) {
		PrintMaterializeDecisionsHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("compare materialize-decisions", flag.ContinueOnError)
	fs.SetOutput(stderr)

	decisionsFile := fs.String("decisions-file", "", "decisions JSONL file to materialize")
	compareJSON := fs.String("compare-json", "", "compare report JSON used to resolve locators")
	oldSession := fs.String("old-session", "", "old browser session used to resolve old_selector")
	newSession := fs.String("new-session", "", "new browser session used to resolve new_selector")
	output := fs.String("output", "", "write materialized decisions JSONL to file")
	asJSON := fs.Bool("json", false, "print materialization report as json")

	if err := parseCommandFlags(fs, args, stderr, "compare materialize-decisions"); err != nil {
		return 1
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "compare materialize-decisions accepts only flags")
		PrintMaterializeDecisionsHelp(stderr)
		return 1
	}
	if strings.TrimSpace(*decisionsFile) == "" {
		fmt.Fprintln(stderr, "compare materialize-decisions requires --decisions-file")
		return 1
	}
	if strings.TrimSpace(*compareJSON) == "" {
		fmt.Fprintln(stderr, "compare materialize-decisions requires --compare-json")
		return 1
	}

	decisions, err := loadCompareDecisions(*decisionsFile)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	compareReport, err := loadCompareReport(*compareJSON)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	materialized := decisions
	materializeIssues := []compareDecisionValidationIssue{}
	materializedRefs := 0
	selectorResolver, closeSelectorResolver, err := compareDecisionSelectorResolverForSessions(ctx, connectClient, strings.TrimSpace(*oldSession), strings.TrimSpace(*newSession))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer closeSelectorResolver()
	materialized, selectorIssues, selectorRefs := materializeCompareDecisionSelectors(materialized, compareReport, selectorResolver)
	materializeIssues = append(materializeIssues, selectorIssues...)
	materializedRefs += selectorRefs
	materialized, locatorIssues, locatorRefs := materializeCompareDecisionRefs(materialized, compareReport)
	materializeIssues = append(materializeIssues, locatorIssues...)
	materializedRefs += locatorRefs
	validation := compareDecisionValidationReport{}
	if len(materializeIssues) == 0 {
		validation = validateCompareDecisions(materialized, &compareReport)
	}
	issues := append([]compareDecisionValidationIssue{}, materializeIssues...)
	issues = append(issues, validation.Issues...)
	report := compareDecisionMaterializeReport{
		Summary: compareDecisionMaterializeSummary{
			InputDecisions:   len(decisions),
			OutputDecisions:  len(materialized),
			MaterializedRefs: materializedRefs,
			Output:           strings.TrimSpace(*output),
			CompareJSONUsed:  true,
		},
		Issues: issues,
	}
	for _, issue := range report.Issues {
		if issue.Severity == "error" {
			report.Summary.Errors++
			continue
		}
		report.Summary.Warnings++
	}

	if report.Summary.Errors == 0 && report.Summary.Output != "" {
		if err := writeCompareDecisionJSONL(report.Summary.Output, materialized); err != nil {
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
	} else if report.Summary.Errors > 0 || report.Summary.Output != "" {
		printCompareDecisionMaterializeReport(stdout, report)
	} else {
		if err := printCompareDecisionJSONL(stdout, materialized); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if len(report.Issues) > 0 {
			fmt.Fprintln(stderr)
			printCompareDecisionMaterializeReport(stderr, report)
		}
	}
	if report.Summary.Errors > 0 {
		return 1
	}
	return 0
}

func compareDecisionSelectorResolverForSessions(ctx context.Context, connectClient func(context.Context) (*rpc.Client, error), oldSession string, newSession string) (compareDecisionSelectorResolver, func(), error) {
	if oldSession == "" && newSession == "" {
		return nil, func() {}, nil
	}
	if connectClient == nil {
		return nil, func() {}, errors.New("compare materialize-decisions needs a client to resolve old_selector or new_selector")
	}
	client, err := connectClient(ctx)
	if err != nil {
		return nil, func() {}, err
	}
	cache := map[string]compareDecisionSelectorCacheEntry{}
	resolver := func(oldSide bool, selector string, nodes []compareSnapshotNode) (string, error) {
		side := "new"
		sessionID := newSession
		if oldSide {
			side = "old"
			sessionID = oldSession
		}
		if sessionID == "" {
			return "", fmt.Errorf("%s_selector requires --%s-session", side, side)
		}
		cacheKey := side + "\x00" + selector
		if cached, ok := cache[cacheKey]; ok {
			return cached.Ref, cached.Err
		}
		ref, err := compareMaterializeSelectorRef(ctx, client, sessionID, side, selector, nodes)
		cache[cacheKey] = compareDecisionSelectorCacheEntry{Ref: ref, Err: err}
		return ref, err
	}
	return resolver, func() { client.Close() }, nil
}

type compareDecisionSelectorCacheEntry struct {
	Ref string
	Err error
}

func compareMaterializeSelectorRef(ctx context.Context, client *rpc.Client, sessionID string, side string, selector string, nodes []compareSnapshotNode) (string, error) {
	selector = strings.TrimSpace(selector)
	res, err := client.ObserveSession(ctx, api.ObserveSessionRequest{
		SessionID: sessionID,
		Options: api.ObserveOptions{
			WithTree:      true,
			ScopeSelector: selector,
			NodeScope:     compareNodeScopeAll,
		},
	})
	if err != nil {
		return "", fmt.Errorf("%s selector %q: %w", side, selector, err)
	}
	if len(res.Observation.Tree) == 0 {
		return "", fmt.Errorf("%s selector %q returned no observed nodes", side, selector)
	}
	selected := buildCompareSnapshot(api.Observation{Tree: []api.Node{res.Observation.Tree[0]}}, compareSnapshotOptions{NodeScope: compareNodeScopeAll})
	if len(selected.Nodes) == 0 {
		return "", fmt.Errorf("%s selector %q returned no comparable node", side, selector)
	}
	return compareResolveSelectorMaterializedRef(side, selector, selected.Nodes[0], nodes)
}

func compareResolveSelectorMaterializedRef(side string, selector string, selected compareSnapshotNode, nodes []compareSnapshotNode) (string, error) {
	matches := compareSelectorMaterializeMatches(selected, nodes)
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("%s selector %q matched the live DOM but no compare JSON node", side, strings.TrimSpace(selector))
	case 1:
		ref := strings.TrimSpace(nodes[matches[0]].Ref)
		if ref == "" {
			return "", fmt.Errorf("%s selector %q matched a compare JSON node without ref", side, strings.TrimSpace(selector))
		}
		return ref, nil
	default:
		return "", fmt.Errorf("%s selector %q matched %d compare JSON nodes: %s", side, strings.TrimSpace(selector), len(matches), compareDecisionNodeHints(nodes, matches, 5))
	}
}

func compareSelectorMaterializeMatches(selected compareSnapshotNode, nodes []compareSnapshotNode) []int {
	matchers := []func(compareSnapshotNode, compareSnapshotNode) bool{
		compareSelectorMaterializeStructureMatch,
		compareSelectorMaterializeTestIDMatch,
		compareSelectorMaterializeHrefMatch,
		compareSelectorMaterializeFingerprintMatch,
		compareSelectorMaterializeContentMatch,
	}
	for _, matches := range matchers {
		indices := make([]int, 0, 1)
		for index, node := range nodes {
			if matches(selected, node) {
				indices = append(indices, index)
			}
		}
		if len(indices) > 0 {
			return indices
		}
	}
	return nil
}

func compareSelectorMaterializeStructureMatch(selected compareSnapshotNode, node compareSnapshotNode) bool {
	return selected.StructureKey != "" && node.StructureKey != "" && selected.StructureKey == node.StructureKey
}

func compareSelectorMaterializeTestIDMatch(selected compareSnapshotNode, node compareSnapshotNode) bool {
	return selected.TestID != "" && selected.TestID == node.TestID && compareSelectorMaterializeRoleCompatible(selected, node)
}

func compareSelectorMaterializeHrefMatch(selected compareSnapshotNode, node compareSnapshotNode) bool {
	return selected.Href != "" && selected.Href == node.Href && compareSelectorMaterializeRoleCompatible(selected, node)
}

func compareSelectorMaterializeFingerprintMatch(selected compareSnapshotNode, node compareSnapshotNode) bool {
	return selected.Fingerprint != "" && selected.Fingerprint == node.Fingerprint && compareSelectorMaterializeRoleCompatible(selected, node) && compareSelectorMaterializeLabelCompatible(selected, node)
}

func compareSelectorMaterializeContentMatch(selected compareSnapshotNode, node compareSnapshotNode) bool {
	if !compareSelectorMaterializeRoleCompatible(selected, node) || !compareSelectorMaterializeLabelCompatible(selected, node) {
		return false
	}
	if selected.Label == "" && selected.Name == "" && selected.Text == "" && selected.Value == "" {
		return false
	}
	return selected.Name == node.Name && selected.Text == node.Text && selected.Value == node.Value
}

func compareSelectorMaterializeRoleCompatible(selected compareSnapshotNode, node compareSnapshotNode) bool {
	return selected.Role == "" || node.Role == "" || selected.Role == node.Role
}

func compareSelectorMaterializeLabelCompatible(selected compareSnapshotNode, node compareSnapshotNode) bool {
	return selected.Label == "" || node.Label == "" || selected.Label == node.Label
}

func runCompareAuditDecisions(args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		PrintAuditDecisionsHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("compare audit-decisions", flag.ContinueOnError)
	fs.SetOutput(stderr)

	decisionsFile := fs.String("decisions-file", "", "decisions JSONL file to audit")
	compareJSON := fs.String("compare-json", "", "compare report JSON used to audit decision application")
	asJSON := fs.Bool("json", false, "print audit report as json")

	if err := parseCommandFlags(fs, args, stderr, "compare audit-decisions"); err != nil {
		return 1
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "compare audit-decisions accepts only flags")
		PrintAuditDecisionsHelp(stderr)
		return 1
	}
	if strings.TrimSpace(*decisionsFile) == "" {
		fmt.Fprintln(stderr, "compare audit-decisions requires --decisions-file")
		return 1
	}
	if strings.TrimSpace(*compareJSON) == "" {
		fmt.Fprintln(stderr, "compare audit-decisions requires --compare-json")
		return 1
	}

	decisions, err := loadCompareDecisions(*decisionsFile)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	report, err := loadCompareReport(*compareJSON)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	audit := auditCompareDecisions(decisions, report)
	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(audit); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else {
		printCompareDecisionAuditReport(stdout, audit)
	}
	if audit.Summary.Errors > 0 {
		return 1
	}
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
	matchMode, err := normalizeCompareMatchMode(run.MatchMode)
	if err != nil {
		return compareReport{}, err
	}
	nodeScope, err := normalizeCompareNodeScope(run.NodeScope)
	if err != nil {
		return compareReport{}, err
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
	cssProperties := ResolveCSSProperties(run.CompareCSS, run.CSSProperties)
	oldScopeSelector, newScopeSelector, err := resolveCompareScopeSelectors(run.ScopeSelector, run.OldScopeSelector, run.NewScopeSelector)
	if err != nil {
		return compareReport{}, err
	}
	if err := validateCompareResolvedNodeScopeSelectors(nodeScope, oldScopeSelector, newScopeSelector); err != nil {
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
		if err := waitForCompareDocumentReady(ctx, client, endpoint.prepared.SessionID, run.WaitTimeout); err != nil {
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
	if strings.TrimSpace(run.WaitFunction) != "" {
		for _, prepared := range []preparedCompareSession{oldPrepared, newPrepared} {
			if err := waitForCompareFunction(ctx, client, prepared.SessionID, run.WaitFunction, run.WaitTimeout); err != nil {
				return compareReport{}, err
			}
		}
	}
	if run.WaitNetworkIdle {
		for _, prepared := range []preparedCompareSession{oldPrepared, newPrepared} {
			if err := waitForCompareNetworkIdle(ctx, client, prepared.SessionID, run.WaitTimeout); err != nil {
				return compareReport{}, err
			}
		}
	}

	oldObservation, err := observeScopedCompareSession(ctx, client, oldPrepared.SessionID, cssProperties, oldScopeSelector, nodeScope)
	if err != nil {
		return compareReport{}, fmt.Errorf("old side %w", err)
	}
	newObservation, err := observeScopedCompareSession(ctx, client, newPrepared.SessionID, cssProperties, newScopeSelector, nodeScope)
	if err != nil {
		return compareReport{}, fmt.Errorf("new side %w", err)
	}

	oldSnapshot := buildCompareSnapshot(oldObservation, compareSnapshotOptions{
		IgnoreText:    ignorePatterns,
		IgnoreNode:    ignoreRules,
		MaskNode:      maskRules,
		CSSProperties: cssProperties,
		CompareLayout: run.CompareLayout,
		NodeScope:     nodeScope,
	})
	newSnapshot := buildCompareSnapshot(newObservation, compareSnapshotOptions{
		IgnoreText:    ignorePatterns,
		IgnoreNode:    ignoreRules,
		MaskNode:      maskRules,
		CSSProperties: cssProperties,
		CompareLayout: run.CompareLayout,
		NodeScope:     nodeScope,
	})
	decisions, err := loadCompareDecisions(run.DecisionsFile)
	if err != nil {
		return compareReport{}, err
	}
	decisionMatches, err := compareResolveDecisionMatches(decisions, oldSnapshot.Nodes, newSnapshot.Nodes)
	if err != nil {
		return compareReport{}, err
	}
	decisionEffects, err := compareResolveDecisionEffects(decisions, oldSnapshot.Nodes, newSnapshot.Nodes)
	if err != nil {
		return compareReport{}, err
	}
	findingDecisionEffects, err := compareResolveFindingDecisionEffects(decisions)
	if err != nil {
		return compareReport{}, err
	}

	report := buildCompareReportWithDecisionEffects(
		oldSnapshot,
		newSnapshot,
		compareScopeFromObservations(oldScopeSelector, newScopeSelector, oldObservation, newObservation),
		matchMode,
		run.MatchingDebug,
		decisionMatches,
		decisionEffects,
		findingDecisionEffects,
	)
	if strings.TrimSpace(run.ReviewDir) != "" {
		decisionAudit := compareDecisionAuditForReview(decisions, run.DecisionsFile, report)
		screenshots := captureCompareReviewScreenshots(ctx, client, oldPrepared.SessionID, newPrepared.SessionID)
		if err := writeCompareReviewPacket(strings.TrimSpace(run.ReviewDir), report, screenshots, compareReviewPacketOptions{DecisionAudit: decisionAudit}); err != nil {
			return compareReport{}, err
		}
	}
	return report, nil
}

func compareDecisionAuditForReview(decisions []compareDecision, decisionsFile string, report compareReport) *compareDecisionAuditReport {
	if strings.TrimSpace(decisionsFile) == "" {
		return nil
	}
	audit := auditCompareDecisions(decisions, report)
	return &audit
}

func resolveCompareScopeSelectors(scopeSelector string, oldScopeSelector string, newScopeSelector string) (string, string, error) {
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
		return "", "", errors.New("compare requires --old-scope-selector or --scope-selector when --new-scope-selector is set")
	}
	if oldSelector != "" && newSelector == "" {
		return "", "", errors.New("compare requires --new-scope-selector or --scope-selector when --old-scope-selector is set")
	}
	return oldSelector, newSelector, nil
}

func validateCompareNodeScopeSelectors(nodeScope string, scopeSelector string, oldScopeSelector string, newScopeSelector string) error {
	oldSelector, newSelector, err := resolveCompareScopeSelectors(scopeSelector, oldScopeSelector, newScopeSelector)
	if err != nil {
		return err
	}
	return validateCompareResolvedNodeScopeSelectors(nodeScope, oldSelector, newSelector)
}

func validateCompareResolvedNodeScopeSelectors(nodeScope string, oldScopeSelector string, newScopeSelector string) error {
	if nodeScope != compareNodeScopeAll {
		return nil
	}
	if strings.TrimSpace(oldScopeSelector) != "" && strings.TrimSpace(newScopeSelector) != "" {
		return nil
	}
	return errors.New("--node-scope all requires --scope-selector or both --old-scope-selector and --new-scope-selector")
}

func compareScopeFromObservations(oldSelector string, newSelector string, oldObservation api.Observation, newObservation api.Observation) *compareScope {
	oldSelector = firstNonEmpty(strings.TrimSpace(oldObservation.Meta["scope_selector"]), strings.TrimSpace(oldSelector))
	newSelector = firstNonEmpty(strings.TrimSpace(newObservation.Meta["scope_selector"]), strings.TrimSpace(newSelector))
	if oldSelector == "" && newSelector == "" {
		return nil
	}
	scope := &compareScope{
		Old: compareScopeSide{
			Selector: oldSelector,
			Matched:  true,
			Tag:      strings.TrimSpace(oldObservation.Meta["scope_tag"]),
		},
		New: compareScopeSide{
			Selector: newSelector,
			Matched:  true,
			Tag:      strings.TrimSpace(newObservation.Meta["scope_tag"]),
		},
	}
	if oldSelector == newSelector {
		scope.Selector = oldSelector
	}
	return scope
}
