package comparecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	nagicli "github.com/mayahiro/nagicli-go"

	"github.com/mayahiro/nexus/internal/rpc"
)

const compareExecutionUsage = "[--backend chromium|lightpanda] [--target-ref <path>] [--viewport <width>x<height>] [--match-mode exact|stable|heuristic|histogram] [--node-scope current|actionable|semantic|all] [--matching-debug] [--decisions-file <jsonl>] [--review-dir <dir>] [--wait-selector <css>] [--scope-selector <css>] [--old-scope-selector <css>] [--new-scope-selector <css>] [--wait-function <js>] [--wait-network-idle] [--wait-timeout <ms>] [--compare-css] [--css-property <name>]... [--compare-layout] [--no-default-ignores] [--ignore-text-regex <regex>]... [--ignore-selector <rule>]... [--mask-selector <rule>]..."
const compareDirectOutputUsage = "[--output-decisions-template <jsonl>] [--output-finding-decisions-template <jsonl>]"
const compareReportOutputUsage = "[--output-json <file>] [--output-md <file>] [--json]"
const compareURLUsage = "<old-url> <new-url> " + compareExecutionUsage + " " + compareDirectOutputUsage + " " + compareReportOutputUsage
const compareEndpointUsage = "(--old-session <id>|--old-url <url>) (--new-session <id>|--new-url <url>) " + compareExecutionUsage + " " + compareDirectOutputUsage + " " + compareReportOutputUsage
const compareManifestUsage = "--manifest <file> " + compareExecutionUsage + " [--continue-on-error] [--limit <n>] " + compareReportOutputUsage
const validateDecisionsUsage = "--decisions-file <jsonl> [--compare-json <file>] [--review-summary <file>] [--old-session <id>] [--new-session <id>] [--strict] [--json]"
const normalizeDecisionsUsage = "--decisions-file <jsonl> [--compare-json <file>] [--review-summary <file>] [--output <jsonl>] [--json]"
const materializeDecisionsUsage = "--decisions-file <jsonl> --compare-json <file> [--old-session <id>] [--new-session <id>] [--output <jsonl>] [--json]"
const repairDecisionsUsage = "--decisions-file <jsonl> --compare-json <file> [--old-session <id>] [--new-session <id>] [--output <jsonl>] [--json]"
const auditDecisionsUsage = "--decisions-file <jsonl> --compare-json <file> [--json]"
const compareValidateDecisionsUsage = "validate-decisions " + validateDecisionsUsage
const compareNormalizeDecisionsUsage = "normalize-decisions " + normalizeDecisionsUsage
const compareMaterializeDecisionsUsage = "materialize-decisions " + materializeDecisionsUsage
const compareRepairDecisionsUsage = "repair-decisions " + repairDecisionsUsage
const compareAuditDecisionsUsage = "audit-decisions " + auditDecisionsUsage

type nagiCompareArguments struct {
	Positionals                    []string
	OldSession                     string
	NewSession                     string
	OldURL                         string
	NewURL                         string
	Backend                        string
	TargetRef                      string
	Viewport                       string
	MatchMode                      string
	NodeScope                      string
	MatchingDebug                  bool
	DecisionsFile                  string
	OutputDecisionsTemplate        string
	OutputFindingDecisionsTemplate string
	ManifestPath                   string
	ContinueOnError                bool
	Limit                          int
	WaitSelector                   string
	ScopeSelector                  string
	OldScopeSelector               string
	NewScopeSelector               string
	WaitFunction                   string
	WaitNetworkIdle                bool
	CompareCSS                     bool
	CompareLayout                  bool
	NoDefaultIgnores               bool
	WaitTimeout                    int
	JSON                           bool
	OutputJSON                     string
	OutputMD                       string
	ReviewDir                      string
	CSSProperty                    []string
	IgnoreTextRegex                []string
	IgnoreSelector                 []string
	MaskSelector                   []string
}

type nagiCompareDecisionArguments struct {
	DecisionsFile string
	CompareJSON   string
	ReviewSummary string
	OldSession    string
	NewSession    string
	Output        string
	Strict        bool
	JSON          bool
}

func newNagiCompareRoot() *nagicli.Command {
	return nagicli.NewCommand("nxctl").Subcommand(newNagiCompareCommand())
}

func newNagiCompareCommand() *nagicli.Command {
	return buildNagiCompareCommand(nil, false)
}

func NewNagiCommand(connectClient func(context.Context) (*rpc.Client, error)) *nagicli.Command {
	return buildNagiCompareCommand(connectClient, true)
}

func buildNagiCompareCommand(connectClient func(context.Context) (*rpc.Client, error), withHandlers bool) *nagicli.Command {
	validateDecisions := newNagiValidateDecisionsCommand()
	normalizeDecisions := newNagiNormalizeDecisionsCommand()
	materializeDecisions := newNagiMaterializeDecisionsCommand()
	repairDecisions := newNagiRepairDecisionsCommand()
	auditDecisions := newNagiAuditDecisionsCommand()
	command := nagicli.NewCommand("compare").
		About("Compare browser interfaces and manage matching decisions").
		UsageVariant("url-pair", compareURLUsage).
		UsageVariant("endpoint-pair", compareEndpointUsage).
		UsageVariant("manifest", compareManifestUsage).
		SubcommandUsage(nagicli.SubcommandUsageExpanded).
		Option(nagiCompareValueOption("old-session", "old session id")).
		Option(nagiCompareValueOption("new-session", "new session id")).
		Option(nagiCompareValueOption("old-url", "old url")).
		Option(nagiCompareValueOption("new-url", "new url")).
		Option(nagiCompareValueOption("backend", "browser backend").Default("chromium")).
		Option(nagiCompareValueOption("target-ref", "target ref")).
		Option(nagiCompareValueOption("viewport", "viewport as WIDTHxHEIGHT")).
		Option(nagiCompareValueOption("match-mode", "node match mode").Default(defaultCompareMatchMode)).
		Option(nagiCompareValueOption("node-scope", "node scope").Default(defaultCompareNodeScope)).
		Option(nagicli.Flag("matching-debug").Long("matching-debug").Help("include matching debug details")).
		Option(nagiCompareValueOption("decisions-file", "read pairing decisions from JSONL")).
		Option(nagiCompareValueOption("output-decisions-template", "write a decisions template")).
		Option(nagiCompareValueOption("output-finding-decisions-template", "write a finding decisions template")).
		Option(nagiCompareValueOption("manifest", "compare manifest json")).
		Option(nagicli.Flag("continue-on-error").Long("continue-on-error").Help("continue after manifest page error")).
		Option(
			nagiCompareValueOption("limit", "limit manifest pages").
				Parser(nagiCompareIntParser("N")).
				Default("0"),
		).
		Option(nagiCompareValueOption("wait-selector", "wait selector before compare")).
		Option(nagiCompareValueOption("scope-selector", "common CSS scope")).
		Option(nagiCompareValueOption("old-scope-selector", "old side CSS scope")).
		Option(nagiCompareValueOption("new-scope-selector", "new side CSS scope")).
		Option(nagiCompareValueOption("wait-function", "wait javascript expression")).
		Option(nagicli.Flag("wait-network-idle").Long("wait-network-idle").Help("wait for network idle")).
		Option(nagicli.Flag("compare-css").Long("compare-css").Help("compare computed css values")).
		Option(nagicli.Flag("compare-layout").Long("compare-layout").Help("compare element bounds")).
		Option(nagicli.Flag("no-default-ignores").Long("no-default-ignores").Help("disable default ignored nodes")).
		Option(
			nagiCompareValueOption("wait-timeout", "wait timeout in milliseconds").
				Parser(nagiCompareIntParser("MS")).
				Default("10000"),
		).
		Option(nagicli.Flag("json").Long("json").Help("print as json")).
		Option(nagiCompareValueOption("output-json", "write compare report json")).
		Option(nagiCompareValueOption("output-md", "write compare report markdown")).
		Option(nagiCompareValueOption("review-dir", "write an AI review packet")).
		Option(nagiCompareRepeatedOption("css-property", "computed css property to compare")).
		Option(nagiCompareRepeatedOption("ignore-text-regex", "regex to strip from text")).
		Option(nagiCompareRepeatedOption("ignore-selector", "node selector to ignore")).
		Option(nagiCompareRepeatedOption("mask-selector", "node selector to mask")).
		Argument(nagicli.Positional("urls").Repeated()).
		Validator(validateNagiCompareInvocation).
		Note("Locator rules support @eN and role, name, text, testid, href, or combined role and name terms").
		Note("--matching-debug includes anchors, regions, ambiguous candidates, and unmatched nodes in JSON and Markdown reports").
		Note("--decisions-file applies reviewed pair, subtree, and finding decisions before and after automatic matching").
		Note("--node-scope all requires an explicit common scope or both old and new scopes").
		Note("--scope-selector applies to both sides and old or new scope selectors override it per side").
		Link("Compare guide", aiCompareDocURL).
		Link("Migration playbook", migrationPlaybookDocURL).
		Link("AI usage guide", aiUsageDocURL).
		Subcommand(validateDecisions).
		Subcommand(normalizeDecisions).
		Subcommand(materializeDecisions).
		Subcommand(repairDecisions).
		Subcommand(auditDecisions)
	if withHandlers {
		command.Handle(nagiCompareHandler(connectClient, runNagiCompare))
		validateDecisions.Handle(nagiCompareHandler(connectClient, runNagiCompareValidateDecisions))
		normalizeDecisions.Handle(nagiCompareHandler(connectClient, runNagiCompareNormalizeDecisions))
		materializeDecisions.Handle(nagiCompareHandler(connectClient, runNagiCompareMaterializeDecisions))
		repairDecisions.Handle(nagiCompareHandler(connectClient, runNagiCompareRepairDecisions))
		auditDecisions.Handle(nagiCompareHandler(connectClient, runNagiCompareAuditDecisions))
	}
	return command
}

func newNagiValidateDecisionsCommand() *nagicli.Command {
	const command = "validate-decisions"
	return nagicli.NewCommand(command).
		About("Validate compare decision records").
		UsageVariant("default", validateDecisionsUsage).
		Option(nagiCompareDecisionValueOption("decisions-file", "decisions JSONL file to validate")).
		Option(nagiCompareDecisionValueOption("compare-json", "compare report JSON")).
		Option(nagiCompareDecisionValueOption("review-summary", "review summary JSON")).
		Option(nagiCompareDecisionValueOption("old-session", "old browser session")).
		Option(nagiCompareDecisionValueOption("new-session", "new browser session")).
		Option(nagiCompareDecisionFlag("strict", "treat warnings as errors")).
		Option(nagiCompareDecisionFlag("json", "print as json")).
		Validator(validateNagiCompareDecisionInvocation(command)).
		Link("Compare guide", aiCompareDocURL)
}

func newNagiNormalizeDecisionsCommand() *nagicli.Command {
	const command = "normalize-decisions"
	return nagicli.NewCommand(command).
		About("Normalize compare decision records").
		UsageVariant("default", normalizeDecisionsUsage).
		Option(nagiCompareDecisionValueOption("decisions-file", "decisions JSONL file to normalize")).
		Option(nagiCompareDecisionValueOption("compare-json", "compare report JSON")).
		Option(nagiCompareDecisionValueOption("review-summary", "review summary JSON")).
		Option(nagiCompareDecisionValueOption("output", "output decisions JSONL")).
		Option(nagiCompareDecisionFlag("json", "print as json")).
		Validator(validateNagiCompareDecisionInvocation(command)).
		Link("Compare guide", aiCompareDocURL)
}

func newNagiMaterializeDecisionsCommand() *nagicli.Command {
	const command = "materialize-decisions"
	return nagicli.NewCommand(command).
		About("Materialize decision selectors as observed refs").
		UsageVariant("default", materializeDecisionsUsage).
		Option(nagiCompareDecisionValueOption("decisions-file", "decisions JSONL file to materialize")).
		Option(nagiCompareDecisionValueOption("compare-json", "compare report JSON")).
		Option(nagiCompareDecisionValueOption("old-session", "old browser session")).
		Option(nagiCompareDecisionValueOption("new-session", "new browser session")).
		Option(nagiCompareDecisionValueOption("output", "output decisions JSONL")).
		Option(nagiCompareDecisionFlag("json", "print as json")).
		Validator(validateNagiCompareDecisionInvocation(command)).
		Link("Compare guide", aiCompareDocURL)
}

func newNagiRepairDecisionsCommand() *nagicli.Command {
	const command = "repair-decisions"
	return nagicli.NewCommand(command).
		About("Repair stale refs in compare decision records").
		UsageVariant("default", repairDecisionsUsage).
		Option(nagiCompareDecisionValueOption("decisions-file", "decisions JSONL file to repair")).
		Option(nagiCompareDecisionValueOption("compare-json", "compare report JSON")).
		Option(nagiCompareDecisionValueOption("old-session", "old browser session")).
		Option(nagiCompareDecisionValueOption("new-session", "new browser session")).
		Option(nagiCompareDecisionValueOption("output", "output decisions JSONL")).
		Option(nagiCompareDecisionFlag("json", "print as json")).
		Validator(validateNagiCompareDecisionInvocation(command)).
		Link("Compare guide", aiCompareDocURL)
}

func newNagiAuditDecisionsCommand() *nagicli.Command {
	const command = "audit-decisions"
	return nagicli.NewCommand(command).
		About("Audit compare decisions against one report").
		UsageVariant("default", auditDecisionsUsage).
		Option(nagiCompareDecisionValueOption("decisions-file", "decisions JSONL file to audit")).
		Option(nagiCompareDecisionValueOption("compare-json", "compare report JSON")).
		Option(nagiCompareDecisionFlag("json", "print as json")).
		Validator(validateNagiCompareDecisionInvocation(command)).
		Link("Compare guide", aiCompareDocURL)
}

func nagiCompareValueOption(name string, help string) *nagicli.OptionSpec {
	return nagicli.ValueOption(name).
		Long(name).
		Parser(nagicli.CustomParser(nagiCompareMetavar(name), func(raw string) (string, error) {
			return raw, nil
		})).
		Help(help)
}

func nagiCompareRepeatedOption(name string, help string) *nagicli.OptionSpec {
	return nagiCompareValueOption(name, help).
		Parser(
			nagicli.CustomParser(nagiCompareMetavar(name), func(raw string) (string, error) {
				value := strings.TrimSpace(raw)
				if value == "" {
					return "", errors.New("compare value must not be empty")
				}
				return value, nil
			}),
		).
		Repeated()
}

func nagiCompareDecisionValueOption(name string, help string) *nagicli.OptionSpec {
	return nagicli.ValueOption(name).
		Long(name).
		Parser(nagicli.CustomParser(nagiCompareMetavar(name), func(raw string) (string, error) {
			return raw, nil
		})).
		Help(help)
}

func nagiCompareMetavar(name string) string {
	switch name {
	case "old-session", "new-session":
		return "ID"
	case "old-url", "new-url":
		return "URL"
	case "backend":
		return "NAME"
	case "target-ref":
		return "PATH"
	case "viewport":
		return "WIDTHxHEIGHT"
	case "match-mode":
		return "MODE"
	case "node-scope":
		return "SCOPE"
	case "manifest", "decisions-file", "output-decisions-template", "output-finding-decisions-template",
		"compare-json", "review-summary", "output-json", "output-md", "output":
		return "FILE"
	case "review-dir":
		return "DIR"
	case "wait-selector", "scope-selector", "old-scope-selector", "new-scope-selector":
		return "CSS"
	case "wait-function":
		return "EXPRESSION"
	case "css-property":
		return "NAME"
	case "ignore-text-regex":
		return "REGEX"
	case "ignore-selector", "mask-selector":
		return "RULE"
	default:
		return "VALUE"
	}
}

func nagiCompareDecisionFlag(name string, help string) *nagicli.OptionSpec {
	return nagicli.Flag(name).Long(name).Help(help)
}

func nagiCompareIntParser(metavar string) nagicli.ValueParser {
	return nagicli.CustomParser(metavar, func(raw string) (int, error) {
		return strconv.Atoi(raw)
	})
}

func validateNagiCompareInvocation(invocation *nagicli.Invocation) *nagicli.Diagnostic {
	if len(invocation.CommandPath()) != 2 {
		return nil
	}
	arguments := nagiCompareArgumentsFromInvocation(invocation)
	if arguments.WaitTimeout < 0 {
		return nagiCompareValidationDiagnostic("wait-timeout must be a non-negative integer").
			WithTarget(nagicli.OptionTarget("wait-timeout"))
	}
	if arguments.Limit < 0 {
		return nagiCompareValidationDiagnostic("limit must be a non-negative integer").
			WithTarget(nagicli.OptionTarget("limit"))
	}
	switch strings.ToLower(strings.TrimSpace(arguments.Backend)) {
	case "chromium", "lightpanda":
	default:
		return nagiCompareValidationDiagnostic("backend must be chromium or lightpanda").
			WithTarget(nagicli.OptionTarget("backend"))
	}
	if strings.TrimSpace(arguments.Viewport) != "" {
		if _, _, err := resolvedViewport(arguments.Viewport); err != nil {
			return nagiCompareValidationDiagnostic(err.Error()).
				WithTarget(nagicli.OptionTarget("viewport"))
		}
	}
	if _, err := normalizeCompareMatchMode(arguments.MatchMode); err != nil {
		return nagiCompareValidationDiagnostic(err.Error()).
			WithTarget(nagicli.OptionTarget("match-mode"))
	}
	nodeScope, err := normalizeCompareNodeScope(arguments.NodeScope)
	if err != nil {
		return nagiCompareValidationDiagnostic(err.Error()).
			WithTarget(nagicli.OptionTarget("node-scope"))
	}
	hasEndpoints := arguments.OldSession != "" || arguments.NewSession != "" || arguments.OldURL != "" || arguments.NewURL != ""
	if strings.TrimSpace(arguments.ManifestPath) != "" {
		if len(arguments.Positionals) != 0 || hasEndpoints {
			return nagiCompareValidationDiagnostic("compare can not mix --manifest with urls or endpoint flags").
				WithTarget(nagicli.OptionTarget("manifest")).
				WithTarget(nagicli.ArgumentTarget("urls"))
		}
		if strings.TrimSpace(arguments.OutputDecisionsTemplate) != "" {
			return nagiCompareValidationDiagnostic("compare can not use --output-decisions-template with --manifest").
				WithTarget(nagicli.OptionTarget("output-decisions-template")).
				WithTarget(nagicli.OptionTarget("manifest"))
		}
		if strings.TrimSpace(arguments.OutputFindingDecisionsTemplate) != "" {
			return nagiCompareValidationDiagnostic("compare can not use --output-finding-decisions-template with --manifest").
				WithTarget(nagicli.OptionTarget("output-finding-decisions-template")).
				WithTarget(nagicli.OptionTarget("manifest"))
		}
		return nil
	}
	if invocation.Supplied("continue-on-error") {
		return nagiCompareValidationDiagnostic("compare --continue-on-error requires --manifest").
			WithTarget(nagicli.OptionTarget("continue-on-error")).
			WithTarget(nagicli.OptionTarget("manifest"))
	}
	if invocation.Supplied("limit") {
		return nagiCompareValidationDiagnostic("compare --limit requires --manifest").
			WithTarget(nagicli.OptionTarget("limit")).
			WithTarget(nagicli.OptionTarget("manifest"))
	}
	if len(arguments.Positionals) == 2 && !hasEndpoints {
		if err := validateCompareNodeScopeSelectors(nodeScope, arguments.ScopeSelector, arguments.OldScopeSelector, arguments.NewScopeSelector); err != nil {
			return nagiCompareValidationDiagnostic(err.Error()).
				WithTarget(nagicli.OptionTarget("node-scope")).
				WithTarget(nagicli.OptionTarget("scope-selector"))
		}
		return nil
	}
	if len(arguments.Positionals) != 0 {
		return nagiCompareValidationDiagnostic("compare accepts either two positional urls, two configured endpoints, or --manifest").
			WithTarget(nagicli.ArgumentTarget("urls"))
	}
	if !hasEndpoints {
		return nagiCompareValidationDiagnostic("compare requires either two positional urls, two configured endpoints, or --manifest")
	}
	if err := validateCompareEndpoint("old", compareEndpoint{
		SessionID: strings.TrimSpace(arguments.OldSession),
		URL:       strings.TrimSpace(arguments.OldURL),
	}); err != nil {
		return nagiCompareValidationDiagnostic(err.Error()).
			WithTarget(nagicli.OptionTarget("old-session")).
			WithTarget(nagicli.OptionTarget("old-url"))
	}
	if err := validateCompareEndpoint("new", compareEndpoint{
		SessionID: strings.TrimSpace(arguments.NewSession),
		URL:       strings.TrimSpace(arguments.NewURL),
	}); err != nil {
		return nagiCompareValidationDiagnostic(err.Error()).
			WithTarget(nagicli.OptionTarget("new-session")).
			WithTarget(nagicli.OptionTarget("new-url"))
	}
	if err := validateCompareNodeScopeSelectors(nodeScope, arguments.ScopeSelector, arguments.OldScopeSelector, arguments.NewScopeSelector); err != nil {
		return nagiCompareValidationDiagnostic(err.Error()).
			WithTarget(nagicli.OptionTarget("node-scope")).
			WithTarget(nagicli.OptionTarget("scope-selector"))
	}
	return nil
}

func validateNagiCompareDecisionInvocation(command string) nagicli.InvocationValidator {
	return func(invocation *nagicli.Invocation) *nagicli.Diagnostic {
		arguments := nagiCompareDecisionArgumentsFromInvocation(invocation)
		if strings.TrimSpace(arguments.DecisionsFile) == "" {
			return nagiCompareValidationDiagnostic(fmt.Sprintf("compare %s requires --decisions-file", command)).
				WithTarget(nagicli.OptionTarget("decisions-file"))
		}
		switch command {
		case "validate-decisions":
			selectorPreflight := strings.TrimSpace(arguments.OldSession) != "" || strings.TrimSpace(arguments.NewSession) != ""
			if selectorPreflight && strings.TrimSpace(arguments.CompareJSON) == "" {
				return nagiCompareValidationDiagnostic("compare validate-decisions selector preflight requires --compare-json").
					WithTarget(nagicli.OptionTarget("compare-json"))
			}
		case "materialize-decisions", "repair-decisions", "audit-decisions":
			if strings.TrimSpace(arguments.CompareJSON) == "" {
				return nagiCompareValidationDiagnostic(fmt.Sprintf("compare %s requires --compare-json", command)).
					WithTarget(nagicli.OptionTarget("compare-json"))
			}
		}
		return nil
	}
}

func nagiCompareValidationDiagnostic(message string) *nagicli.Diagnostic {
	return nagicli.NewDiagnostic(nagicli.CodeValidation, message)
}

type nagiCompareRunner func(
	context.Context,
	*nagicli.Invocation,
	io.Writer,
	io.Writer,
	func(context.Context) (*rpc.Client, error),
) int

func nagiCompareHandler(connectClient func(context.Context) (*rpc.Client, error), run nagiCompareRunner) nagicli.Handler {
	return func(commandContext *nagicli.Context, invocation *nagicli.Invocation) (nagicli.Outcome, error) {
		code := run(
			commandContext.Cancellation(),
			invocation,
			commandContext.Stdout(),
			commandContext.Stderr(),
			connectClient,
		)
		return nagicli.NewOutcome(nagicli.ExitStatus(code)), nil
	}
}

func runNagiCompare(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer, connectClient func(context.Context) (*rpc.Client, error)) int {
	return runCompareWithArguments(ctx, nagiCompareArgumentsFromInvocation(invocation), stdout, stderr, connectClient)
}

func runNagiCompareValidateDecisions(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer, connectClient func(context.Context) (*rpc.Client, error)) int {
	return runCompareValidateDecisionsWithArguments(ctx, nagiCompareDecisionArgumentsFromInvocation(invocation), stdout, stderr, connectClient)
}

func runNagiCompareNormalizeDecisions(_ context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer, _ func(context.Context) (*rpc.Client, error)) int {
	return runCompareNormalizeDecisionsWithArguments(nagiCompareDecisionArgumentsFromInvocation(invocation), stdout, stderr)
}

func runNagiCompareMaterializeDecisions(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer, connectClient func(context.Context) (*rpc.Client, error)) int {
	return runCompareMaterializeDecisionsWithArguments(ctx, nagiCompareDecisionArgumentsFromInvocation(invocation), stdout, stderr, connectClient)
}

func runNagiCompareRepairDecisions(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer, connectClient func(context.Context) (*rpc.Client, error)) int {
	return runCompareRepairDecisionsWithArguments(ctx, nagiCompareDecisionArgumentsFromInvocation(invocation), stdout, stderr, connectClient)
}

func runNagiCompareAuditDecisions(_ context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer, _ func(context.Context) (*rpc.Client, error)) int {
	return runCompareAuditDecisionsWithArguments(nagiCompareDecisionArgumentsFromInvocation(invocation), stdout, stderr)
}

func parseNagiCompareDecisionArguments(
	command string,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	printHelp func(io.Writer),
) (nagiCompareDecisionArguments, int, bool) {
	argv := make([]string, 0, len(args)+2)
	argv = append(argv, "compare", command)
	argv = append(argv, args...)
	result, err := newNagiCompareRoot().Parse(argv)
	if err != nil {
		var diagnostic *nagicli.Diagnostic
		if errors.As(err, &diagnostic) && diagnostic.Code() == nagicli.CodeUnexpectedArgument {
			fmt.Fprintf(stderr, "compare %s accepts only flags\n", command)
			printHelp(stderr)
			return nagiCompareDecisionArguments{}, 1, true
		}
		message := err.Error()
		if diagnostic != nil {
			message = diagnostic.Message()
		}
		fmt.Fprintln(stderr, message)
		fmt.Fprintf(stderr, "hint: run `nxctl compare %s --help` for details\n", command)
		return nagiCompareDecisionArguments{}, 1, true
	}
	if result.Kind() == nagicli.ParseHelp {
		printHelp(stdout)
		return nagiCompareDecisionArguments{}, 0, true
	}
	if result.Kind() != nagicli.ParseInvocation {
		fmt.Fprintln(stderr, "unexpected Nagi parse result")
		return nagiCompareDecisionArguments{}, 1, true
	}
	return nagiCompareDecisionArgumentsFromInvocation(result.Invocation()), 0, false
}

func nagiCompareArgumentsFromInvocation(invocation *nagicli.Invocation) nagiCompareArguments {
	return nagiCompareArguments{
		Positionals:                    nagiCompareRawValues(invocation, "urls"),
		OldSession:                     nagiCompareRawValue(invocation, "old-session"),
		NewSession:                     nagiCompareRawValue(invocation, "new-session"),
		OldURL:                         nagiCompareRawValue(invocation, "old-url"),
		NewURL:                         nagiCompareRawValue(invocation, "new-url"),
		Backend:                        nagiCompareRawValue(invocation, "backend"),
		TargetRef:                      nagiCompareRawValue(invocation, "target-ref"),
		Viewport:                       nagiCompareRawValue(invocation, "viewport"),
		MatchMode:                      nagiCompareRawValue(invocation, "match-mode"),
		NodeScope:                      nagiCompareRawValue(invocation, "node-scope"),
		MatchingDebug:                  nagiCompareFlag(invocation, "matching-debug"),
		DecisionsFile:                  nagiCompareRawValue(invocation, "decisions-file"),
		OutputDecisionsTemplate:        nagiCompareRawValue(invocation, "output-decisions-template"),
		OutputFindingDecisionsTemplate: nagiCompareRawValue(invocation, "output-finding-decisions-template"),
		ManifestPath:                   nagiCompareRawValue(invocation, "manifest"),
		ContinueOnError:                nagiCompareFlag(invocation, "continue-on-error"),
		Limit:                          nagiCompareIntValue(invocation, "limit"),
		WaitSelector:                   nagiCompareRawValue(invocation, "wait-selector"),
		ScopeSelector:                  nagiCompareRawValue(invocation, "scope-selector"),
		OldScopeSelector:               nagiCompareRawValue(invocation, "old-scope-selector"),
		NewScopeSelector:               nagiCompareRawValue(invocation, "new-scope-selector"),
		WaitFunction:                   nagiCompareRawValue(invocation, "wait-function"),
		WaitNetworkIdle:                nagiCompareFlag(invocation, "wait-network-idle"),
		CompareCSS:                     nagiCompareFlag(invocation, "compare-css"),
		CompareLayout:                  nagiCompareFlag(invocation, "compare-layout"),
		NoDefaultIgnores:               nagiCompareFlag(invocation, "no-default-ignores"),
		WaitTimeout:                    nagiCompareIntValue(invocation, "wait-timeout"),
		JSON:                           nagiCompareFlag(invocation, "json"),
		OutputJSON:                     nagiCompareRawValue(invocation, "output-json"),
		OutputMD:                       nagiCompareRawValue(invocation, "output-md"),
		ReviewDir:                      nagiCompareRawValue(invocation, "review-dir"),
		CSSProperty:                    nagiCompareStringValues(invocation, "css-property"),
		IgnoreTextRegex:                nagiCompareStringValues(invocation, "ignore-text-regex"),
		IgnoreSelector:                 nagiCompareStringValues(invocation, "ignore-selector"),
		MaskSelector:                   nagiCompareStringValues(invocation, "mask-selector"),
	}
}

func nagiCompareDecisionArgumentsFromInvocation(invocation *nagicli.Invocation) nagiCompareDecisionArguments {
	return nagiCompareDecisionArguments{
		DecisionsFile: nagiCompareRawValue(invocation, "decisions-file"),
		CompareJSON:   nagiCompareRawValue(invocation, "compare-json"),
		ReviewSummary: nagiCompareRawValue(invocation, "review-summary"),
		OldSession:    nagiCompareRawValue(invocation, "old-session"),
		NewSession:    nagiCompareRawValue(invocation, "new-session"),
		Output:        nagiCompareRawValue(invocation, "output"),
		Strict:        nagiCompareFlag(invocation, "strict"),
		JSON:          nagiCompareFlag(invocation, "json"),
	}
}

func nagiCompareRawValue(invocation *nagicli.Invocation, id string) string {
	value, _ := invocation.RawValue(id)
	return value
}

func nagiCompareIntValue(invocation *nagicli.Invocation, id string) int {
	value, _ := nagicli.ValueAs[int](invocation, id)
	return value
}

func nagiCompareFlag(invocation *nagicli.Invocation, id string) bool {
	value, _ := invocation.Flag(id)
	return value
}

func nagiCompareRawValues(invocation *nagicli.Invocation, id string) []string {
	values := invocation.ParsedValues(id)
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.Raw())
	}
	return result
}

func nagiCompareStringValues(invocation *nagicli.Invocation, id string) []string {
	values := invocation.ParsedValues(id)
	result := make([]string, 0, len(values))
	for _, value := range values {
		typed, ok := value.Typed().(string)
		if ok {
			result = append(result, typed)
		}
	}
	return result
}

func printNagiCompareUsage(w io.Writer) bool {
	document, err := newNagiCompareRoot().HelpDocument([]string{"nxctl", "compare"})
	if err != nil {
		return false
	}
	index := 0
	for _, variant := range document.UsageVariants() {
		prefix := "   or: "
		if index == 0 {
			prefix = "usage: "
		}
		fmt.Fprintln(w, prefix+variant.CommandLine)
		index++
	}
	return index > 0
}

func printNagiCompareDecisionUsage(w io.Writer, command string) bool {
	document, err := newNagiCompareRoot().HelpDocument([]string{"nxctl", "compare", command})
	if err != nil {
		return false
	}
	for index, variant := range document.UsageVariants() {
		prefix := "   or: "
		if index == 0 {
			prefix = "usage: "
		}
		fmt.Fprintln(w, prefix+variant.CommandLine)
	}
	return true
}
