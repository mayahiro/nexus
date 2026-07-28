package cli

import (
	"strconv"
	"strings"

	nagicli "github.com/mayahiro/nagicli-go"
)

type nagiClickArguments struct {
	SessionID   string
	JSON        bool
	Refs        string
	Positionals []string
}

type nagiScreenshotArguments struct {
	SessionID string
	Full      bool
	Annotate  bool
	Recover   bool
	Verbose   bool
	Locator   string
	Nth       int
	Timeout   int
	Paths     []string
}

func newNagiCloseRoot() *nagicli.Command {
	return newNagiCommandRoot(newNagiCloseCommand())
}

func newNagiCloseCommand() *nagicli.Command {
	return nagicli.NewCommand("close").
		About("Close one or all sessions").
		UsageVariant("session", "[--session <id>]").
		UsageVariant("all", "--all").
		Option(
			nagicli.ValueOption("session").
				Long("session").
				Parser(nagicli.StringParser()).
				Default("default").
				Help("session id"),
		).
		Option(nagicli.Flag("all").Long("all").Help("close all sessions")).
		OptionGroup(nagicli.AtMostOne("target", "session", "all")).
		Handle(nagiRunHandler(runCloseInvocation))
}

func newNagiClickRoot() *nagicli.Command {
	return newNagiCommandRoot(newNagiClickCommand())
}

func newNagiClickCommand() *nagicli.Command {
	return nagicli.NewCommand("click").
		About("Click one or more observed nodes or coordinates").
		UsageVariant("node", "<index|@eN> [--session <id>] [--json]").
		UsageVariant("refs", "--refs <@eN,@eN,...> [--session <id>] [--json]").
		UsageVariant("coordinates", "<x> <y> [--session <id>] [--json]").
		Option(
			nagicli.ValueOption("session").
				Long("session").
				Parser(nagicli.StringParser()).
				Default("default").
				Help("session id"),
		).
		Option(nagicli.Flag("json").Long("json").Help("print as json")).
		Option(
			nagicli.ValueOption("refs").
				Long("refs").
				Parser(nagicli.StringParser()).
				Help("comma-separated node refs"),
		).
		Argument(nagicli.Positional("targets").Repeated()).
		Validator(validateNagiClickInvocation).
		Handle(nagiRunHandler(runClickInvocation))
}

func newNagiScreenshotRoot(timeout int) *nagicli.Command {
	return newNagiCommandRoot(newNagiScreenshotCommand(timeout))
}

func newNagiScreenshotCommand(timeout int) *nagicli.Command {
	return nagicli.NewCommand("screenshot").
		About("Capture a page or element screenshot").
		UsageVariant("capture", "[path] [--session <id>] [--full] [--annotate] [--recover-target] [--verbose] [--locator <locator>] [--nth <n>] [--timeout <ms>]").
		Option(
			nagicli.ValueOption("session").
				Long("session").
				Parser(nagicli.StringParser()).
				Default("default").
				Help("session id"),
		).
		Option(nagicli.Flag("full").Long("full").Help("capture full page")).
		Option(nagicli.Flag("annotate").Long("annotate").Help("draw node refs on the screenshot")).
		Option(nagicli.Flag("recover-target").Long("recover-target").Help("replace an unresponsive tab and retry, losing transient page state")).
		Option(nagicli.Flag("verbose").Long("verbose").Help("write detailed capture and recovery diagnostics to the daemon output")).
		Option(
			nagicli.ValueOption("locator").
				Long("locator").
				Parser(nagicli.StringParser()).
				Help("capture a single element"),
		).
		Option(
			nagicli.ValueOption("nth").
				Long("nth").
				Parser(nagiIntParser("N")).
				Help("select nth locator match"),
		).
		Option(
			nagicli.ValueOption("timeout").
				Long("timeout").
				Parser(nagiIntParser("MS")).
				Default(strconv.Itoa(timeout)).
				Help("overall screenshot recovery timeout in milliseconds"),
		).
		Argument(nagicli.Positional("paths").Repeated()).
		Validator(validateNagiScreenshotInvocation).
		Handle(nagiRunHandler(runScreenshotInvocation)).
		Note("Locator forms include @eN, role, name, text, label, testid, and href").
		Note("Viewport capture is the default; --full captures the full page within safety limits").
		Note("Each capture attempt is capped at 10000 ms within the overall timeout").
		Note("Paint readiness is best-effort for 1000 ms; capture continues with a warning on fallback").
		Note("A failed capture automatically reattaches to the same target once; --recover-target additionally permits tab replacement").
		Note("--verbose writes capture boundary events to the current daemon output")
}

func newNagiCommandRoot(command *nagicli.Command) *nagicli.Command {
	return nagicli.NewCommand("nxctl").Subcommand(command)
}

func nagiIntParser(metavar string) nagicli.ValueParser {
	return nagicli.CustomParser(metavar, func(raw string) (int, error) {
		return strconv.Atoi(raw)
	})
}

func nagiClickArgumentsFromInvocation(invocation *nagicli.Invocation) nagiClickArguments {
	sessionID, _ := invocation.RawValue("session")
	asJSON, _ := invocation.Flag("json")
	refs, _ := invocation.RawValue("refs")
	return nagiClickArguments{
		SessionID:   sessionID,
		JSON:        asJSON,
		Refs:        refs,
		Positionals: nagiRawValues(invocation, "targets"),
	}
}

func validateNagiClickInvocation(invocation *nagicli.Invocation) *nagicli.Diagnostic {
	arguments := nagiClickArgumentsFromInvocation(invocation)
	refs := strings.TrimSpace(arguments.Refs)
	if refs != "" {
		if len(arguments.Positionals) != 0 {
			return nagiValidationDiagnostic("click --refs does not accept positional arguments").
				WithTarget(nagicli.OptionTarget("refs")).
				WithTarget(nagicli.ArgumentTarget("targets"))
		}
		if _, err := parseNodeSelectorList(refs); err != nil {
			return nagiValidationDiagnostic("click --refs requires comma-separated positive integer indexes or @eN refs").
				WithTarget(nagicli.OptionTarget("refs"))
		}
		return nil
	}
	if len(arguments.Positionals) != 1 && len(arguments.Positionals) != 2 {
		return nagiValidationDiagnostic("click requires an index or x y coordinates").
			WithTarget(nagicli.ArgumentTarget("targets"))
	}
	if len(arguments.Positionals) == 1 {
		if _, _, err := parseNodeSelector(arguments.Positionals[0]); err != nil {
			return nagiValidationDiagnostic("click requires a positive integer index or @eN ref").
				WithTarget(nagicli.ArgumentTarget("targets"))
		}
		return nil
	}
	for _, value := range arguments.Positionals {
		coordinate, err := strconv.Atoi(value)
		if err != nil || coordinate < 0 {
			return nagiValidationDiagnostic("click requires non-negative integer x y coordinates").
				WithTarget(nagicli.ArgumentTarget("targets"))
		}
	}
	return nil
}

func nagiScreenshotArgumentsFromInvocation(invocation *nagicli.Invocation) nagiScreenshotArguments {
	sessionID, _ := invocation.RawValue("session")
	full, _ := invocation.Flag("full")
	annotate, _ := invocation.Flag("annotate")
	recoverTarget, _ := invocation.Flag("recover-target")
	verbose, _ := invocation.Flag("verbose")
	locator, _ := invocation.RawValue("locator")
	nth, _ := nagicli.ValueAs[int](invocation, "nth")
	timeout, _ := nagicli.ValueAs[int](invocation, "timeout")
	return nagiScreenshotArguments{
		SessionID: sessionID,
		Full:      full,
		Annotate:  annotate,
		Recover:   recoverTarget,
		Verbose:   verbose,
		Locator:   locator,
		Nth:       nth,
		Timeout:   timeout,
		Paths:     nagiRawValues(invocation, "paths"),
	}
}

func validateNagiScreenshotInvocation(invocation *nagicli.Invocation) *nagicli.Diagnostic {
	arguments := nagiScreenshotArgumentsFromInvocation(invocation)
	if len(arguments.Paths) > 1 {
		return nagiValidationDiagnostic("screenshot accepts at most one path").
			WithTarget(nagicli.ArgumentTarget("paths"))
	}
	if invocation.Supplied("nth") && arguments.Nth <= 0 {
		return nagiValidationDiagnostic("--nth must be a positive integer").
			WithTarget(nagicli.OptionTarget("nth"))
	}
	if arguments.Nth > 0 && strings.TrimSpace(arguments.Locator) == "" {
		return nagiValidationDiagnostic("--nth requires --locator").
			WithTarget(nagicli.OptionTarget("nth")).
			WithTarget(nagicli.OptionTarget("locator"))
	}
	if strings.TrimSpace(arguments.Locator) != "" && arguments.Full {
		return nagiValidationDiagnostic("--full is not supported with --locator").
			WithTarget(nagicli.OptionTarget("full")).
			WithTarget(nagicli.OptionTarget("locator"))
	}
	if strings.TrimSpace(arguments.Locator) != "" && arguments.Recover {
		return nagiValidationDiagnostic("--recover-target is not supported with --locator because target replacement invalidates the crop").
			WithTarget(nagicli.OptionTarget("recover-target")).
			WithTarget(nagicli.OptionTarget("locator"))
	}
	if arguments.Timeout <= 0 {
		return nagiValidationDiagnostic("--timeout must be a positive integer").
			WithTarget(nagicli.OptionTarget("timeout"))
	}
	return nil
}

func nagiValidationDiagnostic(message string) *nagicli.Diagnostic {
	return nagicli.NewDiagnostic(nagicli.CodeValidation, message)
}

func nagiRawValues(invocation *nagicli.Invocation, id string) []string {
	values := invocation.ParsedValues(id)
	raw := make([]string, 0, len(values))
	for _, value := range values {
		raw = append(raw, value.Raw())
	}
	return raw
}
