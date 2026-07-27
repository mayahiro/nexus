package comparecmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	nagicli "github.com/mayahiro/nagicli-go"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/rpc"
)

func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, connectClient func(context.Context) (*rpc.Client, error)) int {
	commandContext := nagicli.NewContextWithCancellation(
		strings.NewReader(""),
		stdout,
		stderr,
		nil,
		"",
		ctx,
	)
	application := nagicli.NewCommand("nxctl").
		RequireSubcommand().
		Subcommand(NewNagiCommand(connectClient))
	argv := make([]string, 0, len(args)+1)
	argv = append(argv, "compare")
	argv = append(argv, args...)
	policy := nagicli.DefaultRuntimePolicy().WithExitCodePolicy(
		nagicli.DefaultExitCodePolicy().
			WithStatus(nagicli.CategoryUsage, nagicli.StatusFailure),
	)
	outcome, err := application.RunWithPolicy(commandContext, argv, policy)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return int(outcome.Status())
}

func runCompareWithArguments(ctx context.Context, parsed nagiCompareArguments, stdout io.Writer, stderr io.Writer, connectClient func(context.Context) (*rpc.Client, error)) int {
	positional := parsed.Positionals
	if len(positional) == 2 {
		parsed.OldURL = positional[0]
		parsed.NewURL = positional[1]
	}

	if parsed.WaitTimeout < 0 {
		fmt.Fprintln(stderr, "wait-timeout must be a non-negative integer")
		return 1
	}
	if parsed.Limit < 0 {
		fmt.Fprintln(stderr, "limit must be a non-negative integer")
		return 1
	}
	normalizedMatchMode, err := normalizeCompareMatchMode(parsed.MatchMode)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	normalizedNodeScope, err := normalizeCompareNodeScope(parsed.NodeScope)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if strings.TrimSpace(parsed.ManifestPath) == "" {
		if err := validateCompareNodeScopeSelectors(normalizedNodeScope, parsed.ScopeSelector, parsed.OldScopeSelector, parsed.NewScopeSelector); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if connectClient == nil {
		fmt.Fprintln(stderr, "compare requires a client connector")
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
		Backend:                 parsed.Backend,
		TargetRef:               parsed.TargetRef,
		Viewport:                parsed.Viewport,
		MatchMode:               normalizedMatchMode,
		NodeScope:               normalizedNodeScope,
		MatchingDebug:           parsed.MatchingDebug || strings.TrimSpace(parsed.OutputDecisionsTemplate) != "" || strings.TrimSpace(parsed.ReviewDir) != "",
		DecisionsFile:           parsed.DecisionsFile,
		OutputDecisionsTemplate: strings.TrimSpace(parsed.OutputDecisionsTemplate),
		ReviewDir:               strings.TrimSpace(parsed.ReviewDir),
		WaitSelector:            parsed.WaitSelector,
		ScopeSelector:           parsed.ScopeSelector,
		OldScopeSelector:        parsed.OldScopeSelector,
		NewScopeSelector:        parsed.NewScopeSelector,
		WaitFunction:            parsed.WaitFunction,
		WaitNetworkIdle:         parsed.WaitNetworkIdle,
		CompareCSS:              parsed.CompareCSS,
		AllCSSProperties:        parsed.AllCSSProperties,
		CompareLayout:           parsed.CompareLayout,
		NoDefaultIgnores:        parsed.NoDefaultIgnores,
		WaitTimeout:             parsed.WaitTimeout,
		CSSProperties:           append([]string(nil), parsed.CSSProperty...),
		IgnoreTextRegex:         append([]string(nil), parsed.IgnoreTextRegex...),
		IgnoreSelector:          append([]string(nil), parsed.IgnoreSelector...),
		MaskSelector:            append([]string(nil), parsed.MaskSelector...),
	}

	if strings.TrimSpace(parsed.ManifestPath) != "" {
		if strings.TrimSpace(parsed.OutputDecisionsTemplate) != "" {
			fmt.Fprintln(stderr, "compare can not use --output-decisions-template with --manifest")
			return 1
		}
		if strings.TrimSpace(parsed.OutputFindingDecisionsTemplate) != "" {
			fmt.Fprintln(stderr, "compare can not use --output-finding-decisions-template with --manifest")
			return 1
		}
		manifest, err := loadCompareManifest(parsed.ManifestPath)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		report, err := executeCompareManifest(ctx, client, paths, parsed.ManifestPath, manifest, base, parsed.ContinueOnError, parsed.Limit)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if strings.TrimSpace(parsed.OutputJSON) != "" {
			if err := writeIndentedJSONFile(parsed.OutputJSON, report); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		}
		if strings.TrimSpace(parsed.OutputMD) != "" {
			if err := writeCompareManifestMarkdown(parsed.OutputMD, report); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		}
		if parsed.JSON {
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

	base.OldEndpoint = compareEndpoint{SessionID: strings.TrimSpace(parsed.OldSession), URL: strings.TrimSpace(parsed.OldURL)}
	base.NewEndpoint = compareEndpoint{SessionID: strings.TrimSpace(parsed.NewSession), URL: strings.TrimSpace(parsed.NewURL)}

	report, err := executeCompare(ctx, client, paths, base)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if strings.TrimSpace(parsed.OutputJSON) != "" {
		if err := writeIndentedJSONFile(parsed.OutputJSON, report); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if strings.TrimSpace(parsed.OutputMD) != "" {
		if err := writeCompareMarkdown(parsed.OutputMD, report); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if strings.TrimSpace(parsed.OutputDecisionsTemplate) != "" {
		if err := writeCompareDecisionsTemplate(parsed.OutputDecisionsTemplate, report.MatchingDebug); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if strings.TrimSpace(parsed.OutputFindingDecisionsTemplate) != "" {
		if err := writeCompareFindingDecisionsTemplate(parsed.OutputFindingDecisionsTemplate, report); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if parsed.JSON {
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
	return runCompareValidateDecisionsWithClient(context.Background(), args, stdout, stderr, nil)
}

func runCompareValidateDecisionsWithClient(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, connectClient func(context.Context) (*rpc.Client, error)) int {
	parsed, code, done := parseNagiCompareDecisionArguments(
		"validate-decisions",
		args,
		stdout,
		stderr,
		PrintValidateDecisionsHelp,
	)
	if done {
		return code
	}
	return runCompareValidateDecisionsWithArguments(ctx, parsed, stdout, stderr, connectClient)
}

func runCompareValidateDecisionsWithArguments(ctx context.Context, parsed nagiCompareDecisionArguments, stdout io.Writer, stderr io.Writer, connectClient func(context.Context) (*rpc.Client, error)) int {
	selectorPreflightRequested := strings.TrimSpace(parsed.OldSession) != "" || strings.TrimSpace(parsed.NewSession) != ""

	decisions, err := loadCompareDecisions(parsed.DecisionsFile)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	var loadedCompareReport *compareReport
	if strings.TrimSpace(parsed.CompareJSON) != "" {
		report, err := loadCompareReport(parsed.CompareJSON)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		loadedCompareReport = &report
	}
	reviewClusters, err := loadCompareFindingClusters(parsed.ReviewSummary)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	report := validateCompareDecisionsWithOptions(decisions, loadedCompareReport, reviewClusters, compareDecisionValidationOptions{Strict: parsed.Strict})
	if selectorPreflightRequested {
		selectorResolver, closeSelectorResolver, err := compareDecisionSelectorResolverForSessions(ctx, connectClient, strings.TrimSpace(parsed.OldSession), strings.TrimSpace(parsed.NewSession))
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		defer closeSelectorResolver()
		selectorPreflight := preflightCompareDecisionSelectors(decisions, *loadedCompareReport, selectorResolver)
		applyCompareDecisionSelectorPreflightReport(&report, selectorPreflight)
	}
	if parsed.JSON {
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
	parsed, code, done := parseNagiCompareDecisionArguments(
		"normalize-decisions",
		args,
		stdout,
		stderr,
		PrintNormalizeDecisionsHelp,
	)
	if done {
		return code
	}
	return runCompareNormalizeDecisionsWithArguments(parsed, stdout, stderr)
}

func runCompareNormalizeDecisionsWithArguments(parsed nagiCompareDecisionArguments, stdout io.Writer, stderr io.Writer) int {
	decisions, err := loadCompareDecisions(parsed.DecisionsFile)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	var loadedCompareReport *compareReport
	if strings.TrimSpace(parsed.CompareJSON) != "" {
		report, err := loadCompareReport(parsed.CompareJSON)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		loadedCompareReport = &report
	}
	reviewClusters, err := loadCompareFindingClusters(parsed.ReviewSummary)
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
			Output:            strings.TrimSpace(parsed.Output),
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

	if parsed.JSON {
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
	parsed, code, done := parseNagiCompareDecisionArguments(
		"materialize-decisions",
		args,
		stdout,
		stderr,
		PrintMaterializeDecisionsHelp,
	)
	if done {
		return code
	}
	return runCompareMaterializeDecisionsWithArguments(ctx, parsed, stdout, stderr, connectClient)
}

func runCompareMaterializeDecisionsWithArguments(ctx context.Context, parsed nagiCompareDecisionArguments, stdout io.Writer, stderr io.Writer, connectClient func(context.Context) (*rpc.Client, error)) int {
	decisions, err := loadCompareDecisions(parsed.DecisionsFile)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	compareReport, err := loadCompareReport(parsed.CompareJSON)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	materialized := decisions
	materializeIssues := []compareDecisionValidationIssue{}
	materializedRefs := []compareDecisionMaterializedRef{}
	selectorResolver, closeSelectorResolver, err := compareDecisionSelectorResolverForSessions(ctx, connectClient, strings.TrimSpace(parsed.OldSession), strings.TrimSpace(parsed.NewSession))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer closeSelectorResolver()
	materialized, selectorIssues, selectorRefs := materializeCompareDecisionSelectors(materialized, compareReport, selectorResolver)
	materializeIssues = append(materializeIssues, selectorIssues...)
	materializedRefs = append(materializedRefs, selectorRefs...)
	materialized, locatorIssues, locatorRefs := materializeCompareDecisionRefs(materialized, compareReport)
	materializeIssues = append(materializeIssues, locatorIssues...)
	materializedRefs = append(materializedRefs, locatorRefs...)
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
			MaterializedRefs: len(materializedRefs),
			Output:           strings.TrimSpace(parsed.Output),
			CompareJSONUsed:  true,
		},
		Materialized: materializedRefs,
		Issues:       issues,
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

	if parsed.JSON {
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

func runCompareRepairDecisions(args []string, stdout io.Writer, stderr io.Writer) int {
	return runCompareRepairDecisionsWithClient(context.Background(), args, stdout, stderr, nil)
}

func runCompareRepairDecisionsWithClient(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, connectClient func(context.Context) (*rpc.Client, error)) int {
	parsed, code, done := parseNagiCompareDecisionArguments(
		"repair-decisions",
		args,
		stdout,
		stderr,
		PrintRepairDecisionsHelp,
	)
	if done {
		return code
	}
	return runCompareRepairDecisionsWithArguments(ctx, parsed, stdout, stderr, connectClient)
}

func runCompareRepairDecisionsWithArguments(ctx context.Context, parsed nagiCompareDecisionArguments, stdout io.Writer, stderr io.Writer, connectClient func(context.Context) (*rpc.Client, error)) int {
	decisions, err := loadCompareDecisions(parsed.DecisionsFile)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	compareReport, err := loadCompareReport(parsed.CompareJSON)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	selectorResolver, closeSelectorResolver, err := compareDecisionSelectorResolverForSessions(ctx, connectClient, strings.TrimSpace(parsed.OldSession), strings.TrimSpace(parsed.NewSession))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer closeSelectorResolver()
	repaired, issues, repairedRefs := repairCompareDecisionRefs(decisions, compareReport, selectorResolver)
	report := compareDecisionRepairReport{
		Summary: compareDecisionRepairSummary{
			InputDecisions:  len(decisions),
			OutputDecisions: len(repaired),
			RepairedRefs:    len(repairedRefs),
			Output:          strings.TrimSpace(parsed.Output),
			CompareJSONUsed: true,
		},
		Repaired: repairedRefs,
		Issues:   issues,
	}
	for _, issue := range report.Issues {
		if issue.Severity == "error" {
			report.Summary.Errors++
			continue
		}
		report.Summary.Warnings++
		if strings.TrimSpace(issue.Field) == "old" || strings.TrimSpace(issue.Field) == "new" {
			report.Summary.UnrepairedRefs++
		}
	}

	if report.Summary.Output != "" {
		if err := writeCompareDecisionJSONL(report.Summary.Output, repaired); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	if parsed.JSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else if report.Summary.Output != "" {
		printCompareDecisionRepairReport(stdout, report)
	} else {
		if err := printCompareDecisionJSONL(stdout, repaired); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if len(report.Issues) > 0 {
			fmt.Fprintln(stderr)
			printCompareDecisionRepairReport(stderr, report)
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
		return nil, func() {}, errors.New("compare selector resolution needs a client to resolve old_selector or new_selector")
	}
	client, err := connectClient(ctx)
	if err != nil {
		return nil, func() {}, err
	}
	cache := map[string]compareDecisionSelectorCacheEntry{}
	resolver := func(oldSide bool, selector string, nodes []compareSnapshotNode) (compareDecisionRefResolution, error) {
		side := "new"
		sessionID := newSession
		if oldSide {
			side = "old"
			sessionID = oldSession
		}
		if sessionID == "" {
			return compareDecisionRefResolution{}, fmt.Errorf("%s_selector requires --%s-session", side, side)
		}
		cacheKey := side + "\x00" + selector
		if cached, ok := cache[cacheKey]; ok {
			return cached.Resolution, cached.Err
		}
		resolution, err := compareMaterializeSelectorRef(ctx, client, sessionID, side, selector, nodes)
		cache[cacheKey] = compareDecisionSelectorCacheEntry{Resolution: resolution, Err: err}
		return resolution, err
	}
	return resolver, func() { client.Close() }, nil
}

type compareDecisionSelectorCacheEntry struct {
	Resolution compareDecisionRefResolution
	Err        error
}

func compareMaterializeSelectorRef(ctx context.Context, client *rpc.Client, sessionID string, side string, selector string, nodes []compareSnapshotNode) (compareDecisionRefResolution, error) {
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
		return compareDecisionRefResolution{}, fmt.Errorf("%s selector %q: %w", side, selector, err)
	}
	if len(res.Observation.Tree) == 0 {
		return compareDecisionRefResolution{}, fmt.Errorf("%s selector %q returned no observed nodes", side, selector)
	}
	selected := buildCompareSnapshot(api.Observation{Tree: []api.Node{res.Observation.Tree[0]}}, compareSnapshotOptions{NodeScope: compareNodeScopeAll})
	if len(selected.Nodes) == 0 {
		return compareDecisionRefResolution{}, fmt.Errorf("%s selector %q returned no comparable node", side, selector)
	}
	return compareResolveSelectorMaterializedRef(side, selector, selected.Nodes[0], nodes)
}

func compareResolveSelectorMaterializedRef(side string, selector string, selected compareSnapshotNode, nodes []compareSnapshotNode) (compareDecisionRefResolution, error) {
	matches, matchedBy := compareSelectorMaterializeMatches(selected, nodes)
	switch len(matches) {
	case 0:
		return compareDecisionRefResolution{}, fmt.Errorf("%s selector %q matched the live DOM but no compare JSON node", side, strings.TrimSpace(selector))
	case 1:
		ref := strings.TrimSpace(nodes[matches[0]].Ref)
		if ref == "" {
			return compareDecisionRefResolution{}, fmt.Errorf("%s selector %q matched a compare JSON node without ref", side, strings.TrimSpace(selector))
		}
		return compareDecisionRefResolution{
			Ref:       ref,
			MatchedBy: matchedBy,
			LiveNode:  compareDecisionNodeSummaryForNode(selected),
		}, nil
	default:
		return compareDecisionRefResolution{}, fmt.Errorf("%s selector %q matched %d compare JSON nodes: %s", side, strings.TrimSpace(selector), len(matches), compareDecisionNodeHints(nodes, matches, 5))
	}
}

func compareSelectorMaterializeMatches(selected compareSnapshotNode, nodes []compareSnapshotNode) ([]int, string) {
	matchers := []struct {
		MatchedBy string
		Matches   func(compareSnapshotNode, compareSnapshotNode) bool
	}{
		{MatchedBy: "structure_key", Matches: compareSelectorMaterializeStructureMatch},
		{MatchedBy: "testid", Matches: compareSelectorMaterializeTestIDMatch},
		{MatchedBy: "href", Matches: compareSelectorMaterializeHrefMatch},
		{MatchedBy: "fingerprint", Matches: compareSelectorMaterializeFingerprintMatch},
		{MatchedBy: "content", Matches: compareSelectorMaterializeContentMatch},
	}
	for _, matcher := range matchers {
		indices := make([]int, 0, 1)
		for index, node := range nodes {
			if matcher.Matches(selected, node) {
				indices = append(indices, index)
			}
		}
		if len(indices) > 0 {
			return indices, matcher.MatchedBy
		}
	}
	return nil, ""
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
	parsed, code, done := parseNagiCompareDecisionArguments(
		"audit-decisions",
		args,
		stdout,
		stderr,
		PrintAuditDecisionsHelp,
	)
	if done {
		return code
	}
	return runCompareAuditDecisionsWithArguments(parsed, stdout, stderr)
}

func runCompareAuditDecisionsWithArguments(parsed nagiCompareDecisionArguments, stdout io.Writer, stderr io.Writer) int {
	decisions, err := loadCompareDecisions(parsed.DecisionsFile)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	report, err := loadCompareReport(parsed.CompareJSON)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	audit := auditCompareDecisions(decisions, report)
	if parsed.JSON {
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
	cssProperties, err := ResolveCSSPropertiesMode(run.CompareCSS, run.AllCSSProperties, run.CSSProperties)
	if err != nil {
		return compareReport{}, err
	}
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
		IgnoreText:       ignorePatterns,
		IgnoreNode:       ignoreRules,
		MaskNode:         maskRules,
		CSSProperties:    cssProperties,
		CompareLayout:    run.CompareLayout,
		NoDefaultIgnores: run.NoDefaultIgnores,
		NodeScope:        nodeScope,
	})
	newSnapshot := buildCompareSnapshot(newObservation, compareSnapshotOptions{
		IgnoreText:       ignorePatterns,
		IgnoreNode:       ignoreRules,
		MaskNode:         maskRules,
		CSSProperties:    cssProperties,
		CompareLayout:    run.CompareLayout,
		NoDefaultIgnores: run.NoDefaultIgnores,
		NodeScope:        nodeScope,
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
