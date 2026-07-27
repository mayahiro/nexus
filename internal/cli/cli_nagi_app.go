package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	nagicli "github.com/mayahiro/nagicli-go"

	comparecmd "github.com/mayahiro/nexus/internal/cli/compare"
)

type nagiCommandRunner func(context.Context, *nagicli.Invocation, io.Writer, io.Writer) int

type nagiViewport struct {
	Width  int
	Height int
}

func newNagiApplication() *nagicli.Command {
	return nagicli.NewCommand("nxctl").
		About("Control managed browser sessions and compare interfaces").
		RequireSubcommand().
		Link("AI usage guide", aiUsageDocURL).
		Link("Migration playbook", migrationPlaybookDocURL).
		Subcommand(newNagiAttachCommand()).
		Subcommand(newNagiBackCommand()).
		Subcommand(newNagiBatchCommand()).
		Subcommand(newNagiBrowserCommand()).
		Subcommand(newNagiClickCommand()).
		Subcommand(comparecmd.NewNagiCommand(connectClient)).
		Subcommand(newNagiCloseCommand()).
		Subcommand(newNagiNodeActionCommand("dblclick", "Double-click one observed node", runDblclickInvocation)).
		Subcommand(newNagiEvalCommand()).
		Subcommand(newNagiFillCommand()).
		Subcommand(newNagiFindCommand()).
		Subcommand(newNagiFlowCommand()).
		Subcommand(newNagiGetCommand()).
		Subcommand(newNagiNodeActionCommand("hover", "Hover one observed node", runHoverInvocation)).
		Subcommand(newNagiInspectCommand()).
		Subcommand(newNagiInputCommand()).
		Subcommand(newNagiKeysCommand()).
		Subcommand(newNagiNavigateCommand()).
		Subcommand(newNagiOpenCommand()).
		Subcommand(newNagiObserveCommand()).
		Subcommand(newNagiNodeActionCommand("rightclick", "Right-click one observed node", runRightclickInvocation)).
		Subcommand(newNagiScrollCommand()).
		Subcommand(newNagiScreenshotCommand(int(screenshotCaptureTimeout.Milliseconds()))).
		Subcommand(newNagiSelectCommand()).
		Subcommand(newNagiSessionsCommand()).
		Subcommand(newNagiStateCommand()).
		Subcommand(newNagiTypeCommand()).
		Subcommand(newNagiUploadCommand()).
		Subcommand(newNagiViewportCommand()).
		Subcommand(newNagiWaitCommand()).
		Subcommand(newNagiDetachCommand()).
		Subcommand(newNagiDaemonCommand()).
		Subcommand(newNagiDoctorCommand())
}

func newNagiAttachCommand() *nagicli.Command {
	return nagicli.NewCommand("attach").
		About("Attach a managed target as a named session").
		RequireSubcommand().
		SubcommandUsage(nagicli.SubcommandUsageExpanded).
		Subcommand(
			nagicli.NewCommand("browser").
				About("Attach a browser session").
				UsageVariant("default", "--session <ID> [--backend chromium|lightpanda] [--url <URL>] [--viewport <WIDTHxHEIGHT>] [--target-ref <PATH>]").
				Option(nagiRequiredValueOption("session", "ID", "Session identifier")).
				Option(nagiChoiceOption("backend", "NAME", "Browser backend", "chromium", "lightpanda").Default("chromium")).
				Option(nagiValueOption("url", "URL", "Initial URL")).
				Option(nagiViewportOption("viewport", "Browser viewport")).
				Option(nagiValueOption("target-ref", "PATH", "Browser executable or target reference")).
				Handle(nagiRunHandler(runAttachBrowserInvocation)),
		)
}

func newNagiBackCommand() *nagicli.Command {
	return nagicli.NewCommand("back").
		About("Navigate one session back").
		UsageVariant("default", "[--session <ID>] [--json]").
		Option(nagiSessionOption()).
		Option(nagiJSONFlag()).
		Handle(nagiRunHandler(runBackInvocation))
}

func newNagiBatchCommand() *nagicli.Command {
	return nagicli.NewCommand("batch").
		About("Run multiple nxctl commands sequentially").
		UsageVariant("default", `--cmd "COMMAND" [--cmd "COMMAND"]... [--keep-going] [--json]`).
		Note("Commands run in order; failures stop the batch unless --keep-going is set").
		Option(
			nagiRequiredValueOption("cmd", "COMMAND", "Command to execute").
				Parser(nagiNonEmptyParser("COMMAND")).
				Repeated(),
		).
		Option(nagicli.Flag("keep-going").Long("keep-going").Help("continue after failed commands")).
		Option(nagiJSONFlag()).
		Handle(nagiRunHandler(runBatchInvocation))
}

func newNagiBrowserCommand() *nagicli.Command {
	return nagicli.NewCommand("browser").
		About("Manage browser installations").
		RequireSubcommand().
		SubcommandUsage(nagicli.SubcommandUsageExpanded).
		Subcommand(
			nagicli.NewCommand("setup").
				About("Install managed browsers").
				Handle(nagiRunHandler(runBrowserSetupInvocation)),
		).
		Subcommand(
			nagicli.NewCommand("update").
				About("Update managed browsers").
				Handle(nagiRunHandler(runBrowserUpdateInvocation)),
		).
		Subcommand(
			nagicli.NewCommand("status").
				About("Show managed browser status").
				Handle(nagiRunHandler(runBrowserStatusInvocation)),
		).
		Subcommand(
			nagicli.NewCommand("uninstall").
				About("Uninstall managed browsers").
				UsageVariant("default", "[--name chromium|lightpanda]").
				Option(nagiChoiceOption("name", "NAME", "Browser name", "chromium", "lightpanda")).
				Handle(nagiRunHandler(runBrowserUninstallInvocation)),
		)
}

func newNagiEvalCommand() *nagicli.Command {
	return nagicli.NewCommand("eval").
		About("Evaluate JavaScript in one session").
		UsageVariant("default", "<SOURCE> [--world main|persistent] [--session <ID>] [--json]").
		Option(nagiSessionOption()).
		Option(nagiJSONFlag()).
		Option(nagiChoiceOption("world", "WORLD", "JavaScript execution world", "main", "persistent").Default("main")).
		Argument(nagiRequiredArgument("source", "SOURCE", "JavaScript source")).
		Handle(nagiRunHandler(runEvalInvocation)).
		Note("Persistent world state survives eval calls until the page navigates; store values on globalThis")
}

func newNagiFillCommand() *nagicli.Command {
	return newNagiNodeTextCommand(
		"fill",
		"Replace the value of one observed node",
		"<NODE> <TEXT> [--session <ID>] [--json]",
		runFillInvocation,
	)
}

func newNagiInputCommand() *nagicli.Command {
	return newNagiNodeTextCommand(
		"input",
		"Type text into one observed node",
		"<NODE> <TEXT> [--session <ID>] [--json]",
		runInputInvocation,
	)
}

func newNagiSelectCommand() *nagicli.Command {
	return newNagiNodeTextCommand(
		"select",
		"Select a value on one observed node",
		"<NODE> <VALUE> [--session <ID>] [--json]",
		runSelectInvocation,
	)
}

func newNagiUploadCommand() *nagicli.Command {
	return nagicli.NewCommand("upload").
		About("Upload a file through a file input").
		UsageVariant("node", "<NODE> <PATH> [--session <ID>] [--json]").
		UsageVariant("selector", "--selector <CSS> <PATH> [--session <ID>] [--json]").
		Option(nagiSessionOption()).
		Option(nagiJSONFlag()).
		Option(nagiValueOption("selector", "CSS", "Select a file input directly, including a hidden input")).
		Argument(nagicli.Positional("values").Parser(nagicli.RawParser()).Repeated().Help("Node and path, or path with --selector")).
		Validator(validateNagiUploadInvocation).
		Handle(nagiRunHandler(runUploadInvocation)).
		Note("--selector must match exactly one input[type=file]")
}

func newNagiNodeTextCommand(name string, about string, usage string, run nagiCommandRunner) *nagicli.Command {
	return nagicli.NewCommand(name).
		About(about).
		UsageVariant("default", usage).
		Option(nagiSessionOption()).
		Option(nagiJSONFlag()).
		Argument(nagiNodeArgument()).
		Argument(nagiRequiredArgument("value", "VALUE", "Text, value, or path")).
		Handle(nagiRunHandler(run))
}

func newNagiFindCommand() *nagicli.Command {
	return nagicli.NewCommand("find").
		About("Find observed nodes and optionally act on one").
		RequireSubcommand().
		SubcommandUsage(nagicli.SubcommandUsageExpanded).
		Subcommand(newNagiFindKindCommand("role", "Find by semantic role", true, runFindRoleInvocation)).
		Subcommand(newNagiFindKindCommand("text", "Find by visible text", false, runFindTextInvocation)).
		Subcommand(newNagiFindKindCommand("label", "Find by form label", false, runFindLabelInvocation)).
		Subcommand(newNagiFindKindCommand("testid", "Find by test identifier", false, runFindTestIDInvocation)).
		Subcommand(newNagiFindKindCommand("href", "Find by link target", false, runFindHrefInvocation)).
		Subcommand(newNagiFindKindCommand("aria-label", "Find by aria-label", false, runFindAriaLabelInvocation)).
		Subcommand(newNagiFindKindCommand("css", "Find by CSS selector", false, runFindCSSInvocation)).
		Note("--within requires a recent @eN ref and evaluates the query inside that container").
		Note("Refs become stale after navigation, URL changes, or stable-identity changes at the referenced selector").
		Link("AI usage guide", aiUsageDocURL)
}

func newNagiFindKindCommand(name string, about string, withName bool, run nagiCommandRunner) *nagicli.Command {
	actionUsage := "<QUERY> <click|input|fill|get> [VALUE] [--within <@eN>] [--nth <N>] [--session <ID>] [--json]"
	allUsage := "<QUERY> --all [--within <@eN>] [--session <ID>] [--json]"
	if withName {
		actionUsage = "<QUERY> <click|input|fill|get> [VALUE] [--name <TEXT>] [--within <@eN>] [--nth <N>] [--session <ID>] [--json]"
		allUsage = "<QUERY> --all [--name <TEXT>] [--within <@eN>] [--session <ID>] [--json]"
	}
	command := nagicli.NewCommand(name).
		About(about).
		UsageVariant("action", actionUsage).
		UsageVariant("all", allUsage).
		Option(nagiSessionOption()).
		Option(nagiJSONFlag()).
		Option(nagicli.Flag("all").Long("all").Help("List all matching nodes")).
		Option(nagiIntOption("nth", "N", "Choose the nth matching node")).
		Option(nagiValueOption("within", "@eN", "Limit the search to a previously observed node")).
		Argument(nagiRequiredArgument("query", "QUERY", "Locator query or CSS selector")).
		Argument(nagicli.Positional("action").Parser(nagicli.RawParser()).Help("Action to execute")).
		Argument(nagicli.Positional("action-value").Parser(nagicli.RawParser()).Help("Action value")).
		Validator(validateNagiFindInvocation).
		Handle(nagiRunHandler(run))
	if withName {
		command.Option(nagiValueOption("name", "TEXT", "Accessible name"))
	}
	return command
}

func newNagiFlowCommand() *nagicli.Command {
	return nagicli.NewCommand("flow").
		About("Run browser comparison workflows").
		RequireSubcommand().
		SubcommandUsage(nagicli.SubcommandUsageExpanded).
		Link("Flow guide", aiFlowDocURL).
		Subcommand(
			nagicli.NewCommand("run").
				About("Run a flow manifest").
				UsageVariant("default", "--manifest <FILE> [--scenario <NAME>] [--matrix <NAME>] [--continue-on-error] [--output-json <FILE>] [--json]").
				Option(nagiRequiredValueOption("manifest", "FILE", "Flow manifest JSON")).
				Option(nagiValueOption("scenario", "NAME", "Scenario name")).
				Option(nagiValueOption("matrix", "NAME", "Matrix name")).
				Option(nagicli.Flag("continue-on-error").Long("continue-on-error").Help("Continue after scenario failure")).
				Option(nagiValueOption("output-json", "FILE", "Write flow report JSON")).
				Option(nagiJSONFlag()).
				Handle(nagiRunHandler(runFlowRunInvocation)),
		)
}

func newNagiGetCommand() *nagicli.Command {
	return nagicli.NewCommand("get").
		About("Read values from one browser session").
		UsageVariant("title", "title [--session <ID>] [--json]").
		UsageVariant("html", "html [--selector <CSS>] [--session <ID>] [--json]").
		UsageVariant("bbox-selector", "bbox --selector <CSS> [--session <ID>] [--json]").
		UsageVariant("node", "text|value|attributes|bbox <NODE> [--session <ID>] [--json]").
		UsageVariant("refs", "text|value|attributes|bbox --refs <NODES> [--session <ID>] [--json]").
		Option(nagiSessionOption()).
		Option(nagiJSONFlag()).
		Option(nagiValueOption("selector", "CSS", "CSS selector for html or bbox")).
		Option(nagiValueOption("refs", "NODES", "Comma-separated node refs")).
		Argument(nagiRequiredArgument("target", "TARGET", "title, html, text, value, attributes, or bbox")).
		Argument(nagicli.Positional("node").Parser(nagiNodeSelectorParser()).Help("Observed node")).
		Validator(validateNagiGetInvocation).
		Handle(nagiRunHandler(runGetInvocation))
}

func newNagiInspectCommand() *nagicli.Command {
	return nagicli.NewCommand("inspect").
		About("Compare one node across two sessions").
		UsageVariant("locator", "<LOCATOR> --old-session <ID> --new-session <ID> [--nth <N>] [--scope-selector <CSS>] [--old-scope-selector <CSS>] [--new-scope-selector <CSS>] [--css-property <NAME>]... [--layout-context] [--json]").
		UsageVariant("selector", "--selector <CSS> --old-session <ID> --new-session <ID> [--old-scope-selector <CSS>] [--new-scope-selector <CSS>] [--css-property <NAME>]... [--layout-context] [--json]").
		UsageVariant("scope-selector", "--scope-selector <CSS> --old-session <ID> --new-session <ID> [--old-scope-selector <CSS>] [--new-scope-selector <CSS>] [--css-property <NAME>]... [--layout-context] [--json]").
		UsageVariant("scopes", "--old-scope-selector <CSS> --new-scope-selector <CSS> --old-session <ID> --new-session <ID> [--css-property <NAME>]... [--layout-context] [--json]").
		Option(nagiRequiredValueOption("old-session", "ID", "Old session identifier")).
		Option(nagiRequiredValueOption("new-session", "ID", "New session identifier")).
		Option(nagiValueOption("selector", "CSS", "Raw CSS selector to inspect")).
		Option(nagiValueOption("scope-selector", "CSS", "Common CSS scope")).
		Option(nagiValueOption("old-scope-selector", "CSS", "Old-side CSS scope")).
		Option(nagiValueOption("new-scope-selector", "CSS", "New-side CSS scope")).
		Option(nagiRepeatedValueOption("css-property", "NAME", "Computed CSS property")).
		Option(nagiIntOption("nth", "N", "Choose the nth matching node")).
		Option(nagicli.Flag("layout-context").Long("layout-context").Help("Include ancestor layout context")).
		Option(nagiJSONFlag()).
		Argument(nagicli.Positional("locator").Parser(nagiInspectLocatorParser()).Help("Node locator")).
		Validator(validateNagiInspectInvocation).
		Handle(nagiRunHandler(runInspectInvocation)).
		Note("Locator forms include @eN, role, text, label, testid, and href").
		Note("A side-specific scope needs a common scope fallback or the other side-specific scope").
		Link("Compare guide", aiCompareDocURL)
}

func newNagiKeysCommand() *nagicli.Command {
	return nagicli.NewCommand("keys").
		About("Send a key sequence to one session").
		UsageVariant("default", "<KEYS> [--session <ID>] [--json]").
		Option(nagiSessionOption()).
		Option(nagiJSONFlag()).
		Argument(nagiRequiredArgument("keys", "KEYS", "Key specification")).
		Handle(nagiRunHandler(runKeysInvocation))
}

func newNagiNavigateCommand() *nagicli.Command {
	return nagicli.NewCommand("navigate").
		About("Navigate an attached browser session").
		UsageVariant("default", "<URL> [--session <ID>] [--json]").
		Option(nagiSessionOption()).
		Option(nagiJSONFlag()).
		Argument(nagiRequiredArgument("url", "URL", "Destination URL")).
		Handle(nagiRunHandler(runNavigateInvocation))
}

func newNagiNodeActionCommand(name string, about string, run nagiCommandRunner) *nagicli.Command {
	return nagicli.NewCommand(name).
		About(about).
		UsageVariant("default", "<NODE> [--session <ID>] [--json]").
		Option(nagiSessionOption()).
		Option(nagiJSONFlag()).
		Argument(nagiNodeArgument()).
		Handle(nagiRunHandler(run))
}

func newNagiObserveCommand() *nagicli.Command {
	return nagicli.NewCommand("observe").
		About("Observe one attached session").
		UsageVariant("default", "--session <ID> [--json] [--text] [--tree] [--screenshot] [--full] [--recover-target] [--timeout <MS>]").
		Option(nagiRequiredValueOption("session", "ID", "Session identifier")).
		Option(nagiJSONFlag()).
		Option(nagicli.Flag("text").Long("text").Help("Include page text")).
		Option(nagicli.Flag("tree").Long("tree").Help("Include the observed node tree")).
		Option(nagicli.Flag("screenshot").Long("screenshot").Help("Include a screenshot")).
		Option(nagicli.Flag("full").Long("full").Help("Capture a full-page screenshot")).
		Option(nagicli.Flag("recover-target").Long("recover-target").Help("Replace an unresponsive tab and retry, losing transient page state")).
		Option(nagiIntOption("timeout", "MS", "Overall screenshot recovery timeout in milliseconds").Default("30000")).
		Validator(validateNagiObserveInvocation).
		Handle(nagiRunHandler(runObserveInvocation)).
		Note("Each capture attempt is capped at 10000 ms within the overall timeout")
}

func newNagiOpenCommand() *nagicli.Command {
	return nagicli.NewCommand("open").
		About("Open a URL in a managed browser session").
		UsageVariant("default", "<URL> [--session <ID>] [--backend chromium|lightpanda] [--viewport <WIDTHxHEIGHT>] [--target-ref <PATH>]").
		Option(nagiSessionOption()).
		Option(nagiChoiceOption("backend", "NAME", "Browser backend", "chromium", "lightpanda").Default("chromium")).
		Option(nagiViewportOption("viewport", "Browser viewport")).
		Option(nagiValueOption("target-ref", "PATH", "Browser executable or target reference")).
		Argument(nagiRequiredArgument("url", "URL", "Initial URL")).
		Handle(nagiRunHandler(runOpenInvocation))
}

func newNagiScrollCommand() *nagicli.Command {
	return nagicli.NewCommand("scroll").
		About("Scroll a page or observed node").
		UsageVariant("default", "up|down [--session <ID>] [--node <INDEX>] [--amount <PX>] [--json]").
		Option(nagiSessionOption()).
		Option(nagiJSONFlag()).
		Option(nagiIntOption("node", "INDEX", "Observed node index").Default("0")).
		Option(nagiIntOption("amount", "PX", "Scroll amount in pixels").Default("0")).
		Argument(
			nagicli.Positional("direction").
				Parser(nagiChoiceParser("up|down", "up", "down")).
				Required().
				Help("Scroll direction"),
		).
		Validator(validateNagiScrollInvocation).
		Handle(nagiRunHandler(runScrollInvocation))
}

func newNagiSessionsCommand() *nagicli.Command {
	return nagicli.NewCommand("sessions").
		About("List attached sessions").
		UsageVariant("default", "[--json]").
		Option(nagiJSONFlag()).
		Handle(nagiRunHandler(runSessionsInvocation))
}

func newNagiStateCommand() *nagicli.Command {
	return nagicli.NewCommand("state").
		About("Show an AI-readable page state").
		UsageVariant("default", "[--session <ID>] [--role <ROLE>] [--name <TEXT>] [--text <TEXT>] [--testid <VALUE>] [--href <VALUE>] [--limit <N>] [--json]").
		Option(nagiSessionOption()).
		Option(nagiValueOption("role", "ROLE", "Filter by semantic role")).
		Option(nagiValueOption("name", "TEXT", "Filter by accessible name")).
		Option(nagiValueOption("text", "TEXT", "Filter by text")).
		Option(nagiValueOption("testid", "VALUE", "Filter by test identifier")).
		Option(nagiValueOption("href", "VALUE", "Filter by href")).
		Option(nagiIntOption("limit", "N", "Maximum nodes to print").Default("0")).
		Option(nagiJSONFlag()).
		Validator(validateNagiStateInvocation).
		Handle(nagiRunHandler(runStateInvocation))
}

func newNagiTypeCommand() *nagicli.Command {
	return nagicli.NewCommand("type").
		About("Type text into the active page").
		UsageVariant("default", "<TEXT> [--session <ID>] [--json]").
		Option(nagiSessionOption()).
		Option(nagiJSONFlag()).
		Argument(nagiRequiredArgument("value", "TEXT", "Text to type")).
		Handle(nagiRunHandler(runTypeInvocation))
}

func newNagiViewportCommand() *nagicli.Command {
	return nagicli.NewCommand("viewport").
		About("Set the browser viewport").
		UsageVariant("default", "<WIDTHxHEIGHT> [--session <ID>] [--json]").
		Option(nagiSessionOption()).
		Option(nagiJSONFlag()).
		Argument(
			nagicli.Positional("viewport").
				Parser(nagiViewportParser()).
				Required().
				Help("Viewport dimensions"),
		).
		Handle(nagiRunHandler(runViewportInvocation))
}

func newNagiWaitCommand() *nagicli.Command {
	return nagicli.NewCommand("wait").
		About("Wait for a browser condition").
		UsageVariant("selector", "selector <CSS> [--state attached|detached|visible|hidden] [--timeout <MS>] [--session <ID>] [--json]").
		UsageVariant("text", "text <VALUE> [--timeout <MS>] [--session <ID>] [--json]").
		UsageVariant("url", "url <VALUE> [--timeout <MS>] [--session <ID>] [--json]").
		UsageVariant("navigation", "navigation [--timeout <MS>] [--session <ID>] [--json]").
		UsageVariant("hydrated", "hydrated [--timeout <MS>] [--session <ID>] [--json]").
		UsageVariant("function", "function <EXPRESSION> [--timeout <MS>] [--session <ID>] [--json]").
		Option(nagiSessionOption()).
		Option(nagiJSONFlag()).
		Option(nagiChoiceOption("state", "STATE", "Selector state", "attached", "detached", "visible", "hidden").Default("visible")).
		Option(nagiIntOption("timeout", "MS", "Wait timeout in milliseconds").Default("30000")).
		Argument(
			nagicli.Positional("target").
				Parser(nagiChoiceParser("TARGET", "selector", "text", "url", "navigation", "hydrated", "function")).
				Required().
				Help("Wait target"),
		).
		Argument(nagicli.Positional("value").Parser(nagicli.RawParser()).Help("Wait value")).
		Validator(validateNagiWaitInvocation).
		Handle(nagiRunHandler(runWaitInvocation)).
		Note("hydrated waits for DOMContentLoaded, animation frames, and a DOM mutation quiet window; it is not a React-internal signal").
		Link("Compare guide", aiCompareDocURL)
}

func newNagiDetachCommand() *nagicli.Command {
	return nagicli.NewCommand("detach").
		About("Detach one session without stopping the daemon").
		UsageVariant("default", "--session <ID>").
		Option(nagiRequiredValueOption("session", "ID", "Session identifier")).
		Handle(nagiRunHandler(runDetachInvocation))
}

func newNagiDaemonCommand() *nagicli.Command {
	return nagicli.NewCommand("daemon").
		About("Run the Nexus daemon").
		Handle(nagiRunHandler(runDaemonInvocation))
}

func newNagiDoctorCommand() *nagicli.Command {
	return nagicli.NewCommand("doctor").
		About("Check configuration, daemon, and protocol status").
		Handle(nagiRunHandler(runDoctorInvocation)).
		Note("Starts nxd temporarily when needed and stops it after the check")
}

func nagiRunHandler(run nagiCommandRunner) nagicli.Handler {
	return func(commandContext *nagicli.Context, invocation *nagicli.Invocation) (nagicli.Outcome, error) {
		code := run(
			commandContext.Cancellation(),
			invocation,
			commandContext.Stdout(),
			commandContext.Stderr(),
		)
		return nagicli.NewOutcome(nagicli.ExitStatus(code)), nil
	}
}

func nagiSessionOption() *nagicli.OptionSpec {
	return nagiValueOption("session", "ID", "Session identifier").Default("default")
}

func nagiJSONFlag() *nagicli.OptionSpec {
	return nagicli.Flag("json").Long("json").Help("Print JSON")
}

func nagiValueOption(id string, metavar string, help string) *nagicli.OptionSpec {
	return nagicli.ValueOption(id).
		Long(id).
		Parser(nagicli.CustomParser(metavar, func(raw string) (string, error) {
			return raw, nil
		})).
		Help(help)
}

func nagiRequiredValueOption(id string, metavar string, help string) *nagicli.OptionSpec {
	return nagiValueOption(id, metavar, help).
		Parser(nagicli.CustomParser(metavar, func(raw string) (string, error) {
			if strings.TrimSpace(raw) == "" {
				return "", fmt.Errorf("must not be empty")
			}
			return raw, nil
		})).
		Required()
}

func nagiRepeatedValueOption(id string, metavar string, help string) *nagicli.OptionSpec {
	return nagiValueOption(id, metavar, help).
		Parser(nagiNonEmptyParser(metavar)).
		Repeated()
}

func nagiIntOption(id string, metavar string, help string) *nagicli.OptionSpec {
	return nagicli.ValueOption(id).
		Long(id).
		Parser(nagiIntParser(metavar)).
		Help(help)
}

func nagiChoiceOption(id string, metavar string, help string, choices ...string) *nagicli.OptionSpec {
	return nagicli.ValueOption(id).
		Long(id).
		Parser(nagiChoiceParser(metavar, choices...)).
		Help(help)
}

func nagiViewportOption(id string, help string) *nagicli.OptionSpec {
	return nagicli.ValueOption(id).
		Long(id).
		Parser(nagiViewportParser()).
		Help(help)
}

func nagiRequiredArgument(id string, metavar string, help string) *nagicli.Argument {
	return nagicli.Positional(id).
		Parser(nagicli.CustomParser(metavar, func(raw string) (string, error) {
			if raw == "" {
				return "", fmt.Errorf("must not be empty")
			}
			return raw, nil
		})).
		Required().
		Help(help)
}

func nagiNodeArgument() *nagicli.Argument {
	return nagicli.Positional("node").
		Parser(nagiNodeSelectorParser()).
		Required().
		Help("Observed node index or @eN ref")
}

func nagiNodeSelectorParser() nagicli.ValueParser {
	return nagicli.CustomParser("NODE", func(raw string) (nodeSelector, error) {
		id, ref, err := parseNodeSelector(raw)
		if err != nil {
			return nodeSelector{}, err
		}
		return nodeSelector{ID: id, Ref: ref}, nil
	})
}

func nagiViewportParser() nagicli.ValueParser {
	return nagicli.CustomParser("WIDTHxHEIGHT", func(raw string) (nagiViewport, error) {
		width, height, err := parseViewport(raw)
		if err != nil {
			return nagiViewport{}, err
		}
		return nagiViewport{Width: width, Height: height}, nil
	})
}

func nagiInspectLocatorParser() nagicli.ValueParser {
	return nagicli.CustomParser("LOCATOR", func(raw string) (inspectLocator, error) {
		return parseInspectLocator(raw)
	})
}

func nagiChoiceParser(metavar string, choices ...string) nagicli.ValueParser {
	return nagicli.CustomParser(metavar, func(raw string) (string, error) {
		for _, choice := range choices {
			if raw == choice {
				return raw, nil
			}
		}
		return "", fmt.Errorf("must be one of %s", strings.Join(choices, ", "))
	})
}

func nagiNonEmptyParser(metavar string) nagicli.ValueParser {
	return nagicli.CustomParser(metavar, func(raw string) (string, error) {
		value := strings.TrimSpace(raw)
		if value == "" {
			return "", fmt.Errorf("must not be empty")
		}
		return value, nil
	})
}

func nagiDiagnostic(message string, targets ...nagicli.DiagnosticTarget) *nagicli.Diagnostic {
	diagnostic := nagicli.NewDiagnostic(nagicli.CodeValidation, message)
	for _, target := range targets {
		diagnostic.WithTarget(target)
	}
	return diagnostic
}

func nagiRawValue(invocation *nagicli.Invocation, id string) string {
	value, _ := invocation.RawValue(id)
	return value
}

func nagiStringValue(invocation *nagicli.Invocation, id string) string {
	value, _ := nagicli.ValueAs[string](invocation, id)
	return value
}

func nagiIntValue(invocation *nagicli.Invocation, id string) int {
	value, _ := nagicli.ValueAs[int](invocation, id)
	return value
}

func nagiBoolValue(invocation *nagicli.Invocation, id string) bool {
	value, _ := invocation.Flag(id)
	return value
}

func nagiNodeValue(invocation *nagicli.Invocation, id string) nodeSelector {
	value, _ := nagicli.ValueAs[nodeSelector](invocation, id)
	return value
}

func nagiViewportValue(invocation *nagicli.Invocation, id string) (nagiViewport, bool) {
	return nagicli.ValueAs[nagiViewport](invocation, id)
}

func nagiStringValues(invocation *nagicli.Invocation, id string) []string {
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

func validateNagiFindInvocation(invocation *nagicli.Invocation) *nagicli.Diagnostic {
	matchAll := nagiBoolValue(invocation, "all")
	action := nagiRawValue(invocation, "action")
	actionValue := nagiRawValue(invocation, "action-value")
	nth := nagiIntValue(invocation, "nth")
	within := strings.TrimSpace(nagiStringValue(invocation, "within"))
	if within != "" {
		_, ref, err := parseNodeSelector(within)
		if err != nil || ref == "" {
			return nagiDiagnostic("find --within requires an @eN ref", nagicli.OptionTarget("within"))
		}
	}
	if matchAll && action != "" {
		return nagiDiagnostic(
			"find --all does not accept an action",
			nagicli.OptionTarget("all"),
			nagicli.ArgumentTarget("action"),
		)
	}
	if !matchAll && action == "" {
		return nagiDiagnostic(
			"find requires an action or --all",
			nagicli.ArgumentTarget("action"),
			nagicli.OptionTarget("all"),
		)
	}
	if invocation.Supplied("nth") && nth <= 0 {
		return nagiDiagnostic("find --nth must be a positive integer", nagicli.OptionTarget("nth"))
	}
	if matchAll && nth > 0 {
		return nagiDiagnostic(
			"find --all does not accept --nth",
			nagicli.OptionTarget("all"),
			nagicli.OptionTarget("nth"),
		)
	}
	switch action {
	case "", "click":
		if actionValue != "" {
			return nagiDiagnostic("click action does not accept a value", nagicli.ArgumentTarget("action-value"))
		}
	case "input", "fill":
		if actionValue == "" {
			return nagiDiagnostic(action+" action requires text", nagicli.ArgumentTarget("action-value"))
		}
	case "get":
		if !isFindGetTarget(actionValue) {
			return nagiDiagnostic(
				"get action requires text, value, attributes, or bbox",
				nagicli.ArgumentTarget("action-value"),
			)
		}
	default:
		return nagiDiagnostic("unsupported find action: "+action, nagicli.ArgumentTarget("action"))
	}
	return nil
}

func validateNagiGetInvocation(invocation *nagicli.Invocation) *nagicli.Diagnostic {
	target := nagiRawValue(invocation, "target")
	_, hasNode := invocation.RawValue("node")
	selector := strings.TrimSpace(nagiRawValue(invocation, "selector"))
	refs := strings.TrimSpace(nagiRawValue(invocation, "refs"))
	if refs != "" {
		if _, err := parseNodeSelectorList(refs); err != nil {
			return nagiDiagnostic(
				"get --refs requires comma-separated positive integer indexes or @eN refs",
				nagicli.OptionTarget("refs"),
			)
		}
	}
	switch target {
	case "title":
		if hasNode || selector != "" || refs != "" {
			return nagiDiagnostic("get title does not accept a node, --selector, or --refs", nagicli.ArgumentTarget("target"))
		}
	case "html":
		if hasNode || refs != "" {
			return nagiDiagnostic("get html does not accept a node or --refs", nagicli.ArgumentTarget("target"))
		}
	case "text", "value", "attributes":
		if selector != "" {
			return nagiDiagnostic("get "+target+" does not support --selector", nagicli.OptionTarget("selector"))
		}
		if refs == "" && !hasNode {
			return nagiDiagnostic("get "+target+" requires a node or --refs", nagicli.ArgumentTarget("node"))
		}
		if refs != "" && hasNode {
			return nagiDiagnostic("get "+target+" can not combine a node with --refs", nagicli.OptionTarget("refs"), nagicli.ArgumentTarget("node"))
		}
	case "bbox":
		count := 0
		if selector != "" {
			count++
		}
		if refs != "" {
			count++
		}
		if hasNode {
			count++
		}
		if count != 1 {
			return nagiDiagnostic("get bbox requires exactly one node, --selector, or --refs", nagicli.ArgumentTarget("node"))
		}
	default:
		return nagiDiagnostic(
			"get target must be title, html, text, value, attributes, or bbox",
			nagicli.ArgumentTarget("target"),
		)
	}
	return nil
}

func validateNagiUploadInvocation(invocation *nagicli.Invocation) *nagicli.Diagnostic {
	values := nagiRawValues(invocation, "values")
	selector := strings.TrimSpace(nagiStringValue(invocation, "selector"))
	if selector != "" {
		if len(values) != 1 {
			return nagiDiagnostic(
				"upload --selector requires exactly one path",
				nagicli.OptionTarget("selector"),
				nagicli.ArgumentTarget("values"),
			)
		}
		if values[0] == "" {
			return nagiDiagnostic("upload path must not be empty", nagicli.ArgumentTarget("values"))
		}
		return nil
	}
	if len(values) != 2 {
		return nagiDiagnostic(
			"upload requires a node and path, or --selector and path",
			nagicli.ArgumentTarget("values"),
			nagicli.OptionTarget("selector"),
		)
	}
	if values[1] == "" {
		return nagiDiagnostic("upload path must not be empty", nagicli.ArgumentTarget("values"))
	}
	if _, _, err := parseNodeSelector(values[0]); err != nil {
		return nagiDiagnostic(
			"upload requires a positive integer index or @eN ref",
			nagicli.ArgumentTarget("values"),
		)
	}
	return nil
}

func validateNagiInspectInvocation(invocation *nagicli.Invocation) *nagicli.Diagnostic {
	locator := strings.TrimSpace(nagiRawValue(invocation, "locator"))
	selector := strings.TrimSpace(nagiRawValue(invocation, "selector"))
	scope := strings.TrimSpace(nagiRawValue(invocation, "scope-selector"))
	oldScope := strings.TrimSpace(nagiRawValue(invocation, "old-scope-selector"))
	newScope := strings.TrimSpace(nagiRawValue(invocation, "new-scope-selector"))
	nth := nagiIntValue(invocation, "nth")
	if locator == "" && selector == "" && scope == "" && oldScope == "" && newScope == "" {
		return nagiDiagnostic("inspect requires a locator or selector scope", nagicli.ArgumentTarget("locator"))
	}
	if locator != "" && selector != "" {
		return nagiDiagnostic(
			"inspect can not combine a locator with --selector",
			nagicli.ArgumentTarget("locator"),
			nagicli.OptionTarget("selector"),
		)
	}
	if selector != "" && scope != "" {
		return nagiDiagnostic(
			"inspect can not combine --selector with --scope-selector",
			nagicli.OptionTarget("selector"),
			nagicli.OptionTarget("scope-selector"),
		)
	}
	if invocation.Supplied("nth") && nth <= 0 {
		return nagiDiagnostic("inspect --nth must be a positive integer", nagicli.OptionTarget("nth"))
	}
	if locator == "" && nth > 0 {
		return nagiDiagnostic("inspect selector mode does not support --nth", nagicli.OptionTarget("nth"))
	}
	commonScope := scope
	if locator == "" {
		commonScope = firstNonEmpty(selector, scope)
	}
	if _, _, err := resolveInspectScopeSelectors(commonScope, oldScope, newScope); err != nil {
		return nagiDiagnostic(
			err.Error(),
			nagicli.OptionTarget("scope-selector"),
			nagicli.OptionTarget("old-scope-selector"),
			nagicli.OptionTarget("new-scope-selector"),
		)
	}
	return nil
}

func validateNagiScrollInvocation(invocation *nagicli.Invocation) *nagicli.Diagnostic {
	if nagiIntValue(invocation, "amount") < 0 {
		return nagiDiagnostic("scroll amount must be a non-negative integer", nagicli.OptionTarget("amount"))
	}
	if nagiIntValue(invocation, "node") < 0 {
		return nagiDiagnostic("scroll node must be a non-negative integer", nagicli.OptionTarget("node"))
	}
	return nil
}

func validateNagiStateInvocation(invocation *nagicli.Invocation) *nagicli.Diagnostic {
	if nagiIntValue(invocation, "limit") < 0 {
		return nagiDiagnostic("state limit must be a non-negative integer", nagicli.OptionTarget("limit"))
	}
	return nil
}

func validateNagiObserveInvocation(invocation *nagicli.Invocation) *nagicli.Diagnostic {
	withScreenshot := nagiBoolValue(invocation, "screenshot") || nagiBoolValue(invocation, "full")
	if nagiBoolValue(invocation, "recover-target") &&
		!withScreenshot {
		return nagiDiagnostic(
			"observe --recover-target requires --screenshot or --full",
			nagicli.OptionTarget("recover-target"),
			nagicli.OptionTarget("screenshot"),
			nagicli.OptionTarget("full"),
		)
	}
	if invocation.Supplied("timeout") && !withScreenshot {
		return nagiDiagnostic(
			"observe --timeout requires --screenshot or --full",
			nagicli.OptionTarget("timeout"),
			nagicli.OptionTarget("screenshot"),
			nagicli.OptionTarget("full"),
		)
	}
	if nagiIntValue(invocation, "timeout") <= 0 {
		return nagiDiagnostic(
			"observe --timeout must be a positive integer",
			nagicli.OptionTarget("timeout"),
		)
	}
	return nil
}

func validateNagiWaitInvocation(invocation *nagicli.Invocation) *nagicli.Diagnostic {
	target := nagiStringValue(invocation, "target")
	value := nagiRawValue(invocation, "value")
	if (target == "navigation" || target == "hydrated") && value != "" {
		return nagiDiagnostic("wait "+target+" does not accept a value", nagicli.ArgumentTarget("value"))
	}
	if target != "navigation" && target != "hydrated" && value == "" {
		return nagiDiagnostic("wait "+target+" requires a value", nagicli.ArgumentTarget("value"))
	}
	if nagiIntValue(invocation, "timeout") < 0 {
		return nagiDiagnostic("wait timeout must be a non-negative integer", nagicli.OptionTarget("timeout"))
	}
	if target != "selector" && invocation.Supplied("state") {
		return nagiDiagnostic("wait "+target+" does not support --state", nagicli.OptionTarget("state"))
	}
	return nil
}
