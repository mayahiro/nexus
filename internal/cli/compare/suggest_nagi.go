package comparecmd

import (
	"context"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"

	nagicli "github.com/mayahiro/nagicli-go"

	"github.com/mayahiro/nexus/internal/rpc"
)

const suggestDecisionsUsage = "--compare-json <file> [--output <jsonl>] [--output-json <file>] [--model <id>] [--min-probability <number>] [--min-margin <number>] [--limit <n>] [--max-candidates <n>] [--timeout <ms>] [--promote] [--dry-run] [--json]"

func newNagiSuggestDecisionsCommand() *nagicli.Command {
	probability := func(name, help, value string) *nagicli.OptionSpec {
		return nagiCompareValueOption(name, help).Parser(nagicli.CustomParser("NUMBER", func(raw string) (float64, error) { return strconv.ParseFloat(raw, 64) })).Default(value)
	}
	return nagicli.NewCommand("suggest-decisions").
		About("Suggest element pairs by sending bounded compare context to Jev").
		UsageVariant("default", suggestDecisionsUsage).
		Option(nagiCompareValueOption("compare-json", "single-page compare JSON with matching debug")).
		Option(nagiCompareValueOption("output", "new decisions JSONL file; required unless --dry-run")).
		Option(nagiCompareValueOption("output-json", "new review JSON file; defaults to <output>.review.json")).
		Option(nagiCompareValueOption("model", "Jev model ID or alias").Parser(nagicli.CustomParser("ID", func(raw string) (string, error) { return raw, nil })).Default(defaultIdentityModel)).
		Option(probability("min-probability", "minimum choice and same-element probabilities for promotion", "0.95")).
		Option(probability("min-margin", "minimum choice probability margin for promotion", "0.20")).
		Option(nagiCompareValueOption("limit", "maximum old nodes to consider (1-200)").Parser(nagiCompareIntParser("N")).Default("20")).
		Option(nagiCompareValueOption("max-candidates", "maximum new candidates per old node (1-20)").Parser(nagiCompareIntParser("N")).Default("5")).
		Option(nagiCompareValueOption("timeout", "total remote evaluation timeout in milliseconds (1-300000)").Parser(nagiCompareIntParser("MS")).Default("30000")).
		Option(nagiCompareDecisionFlag("promote", "emit high-confidence pairs that pass probability thresholds")).
		Option(nagiCompareDecisionFlag("dry-run", "print exact request payloads as JSON without sending or writing decisions")).
		Option(nagiCompareDecisionFlag("json", "print the review report as JSON")).
		Validator(func(invocation *nagicli.Invocation) *nagicli.Diagnostic {
			if err := validateIdentitySuggestionArguments(identitySuggestionArgumentsFromInvocation(invocation)); err != nil {
				return nagiCompareValidationDiagnostic(err.Error())
			}
			return nil
		}).
		Note("Remote evaluation uses TYPESAFE_API_KEY and the official TypeSafe endpoint; invoking this command opts in to sending page context").
		Note("Suggestions remain tentative unless --promote is supplied; thresholds are experimental and are not calibrated on your pages").
		Note("Only unmatched nodes are considered; conflicting proposals stay unknown and existing output files are never overwritten").
		Note("Dry-run needs no API key and includes outgoing page text; inspect it before sharing or sending").
		Link("Identity suggestions", aiCompareIdentityDocURL).
		Link("Compare guide", aiCompareDocURL)
}

func identitySuggestionArgumentsFromInvocation(invocation *nagicli.Invocation) identitySuggestionArguments {
	minProbability, _ := nagicli.ValueAs[float64](invocation, "min-probability")
	minMargin, _ := nagicli.ValueAs[float64](invocation, "min-margin")
	return identitySuggestionArguments{
		CompareJSON: strings.TrimSpace(nagiCompareRawValue(invocation, "compare-json")),
		Output:      strings.TrimSpace(nagiCompareRawValue(invocation, "output")), OutputJSON: strings.TrimSpace(nagiCompareRawValue(invocation, "output-json")),
		Model: strings.TrimSpace(nagiCompareRawValue(invocation, "model")), MinProbability: minProbability, MinMargin: minMargin,
		Limit: nagiCompareIntValue(invocation, "limit"), MaxCandidates: nagiCompareIntValue(invocation, "max-candidates"), Timeout: nagiCompareIntValue(invocation, "timeout"),
		Promote: nagiCompareFlag(invocation, "promote"), DryRun: nagiCompareFlag(invocation, "dry-run"), JSON: nagiCompareFlag(invocation, "json"),
	}
}

func validateIdentitySuggestionArguments(args identitySuggestionArguments) error {
	switch {
	case args.CompareJSON == "":
		return errors.New("compare suggest-decisions requires --compare-json")
	case args.Output == "" && !args.DryRun:
		return errors.New("compare suggest-decisions requires --output unless --dry-run is supplied")
	case !identityValidModel(args.Model):
		return errors.New("model must be a Jev model ID or alias")
	case math.IsNaN(args.MinProbability) || math.IsInf(args.MinProbability, 0) || args.MinProbability <= 0.5 || args.MinProbability > 1:
		return errors.New("min-probability must be greater than 0.5 and at most 1")
	case math.IsNaN(args.MinMargin) || math.IsInf(args.MinMargin, 0) || args.MinMargin < 0 || args.MinMargin > 1:
		return errors.New("min-margin must be between 0 and 1")
	case args.Limit < 1 || args.Limit > 200:
		return errors.New("limit must be between 1 and 200")
	case args.MaxCandidates < 1 || args.MaxCandidates > 20:
		return errors.New("max-candidates must be between 1 and 20")
	case args.Timeout < 1 || args.Timeout > 300000:
		return errors.New("timeout must be between 1 and 300000 milliseconds")
	}
	return nil
}

func runNagiCompareSuggestDecisions(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer, _ func(context.Context) (*rpc.Client, error)) int {
	return runIdentitySuggestions(ctx, identitySuggestionArgumentsFromInvocation(invocation), stdout, stderr, nil)
}
