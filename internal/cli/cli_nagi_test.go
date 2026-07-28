package cli

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	nagicli "github.com/mayahiro/nagicli-go"
)

func TestNagiCoreSchemas(t *testing.T) {
	tests := []struct {
		name     string
		root     *nagicli.Command
		variants []nagiUsageVariantContract
	}{
		{
			name: "close",
			root: newNagiCloseRoot(),
			variants: []nagiUsageVariantContract{
				{ID: "session", Syntax: "[--session <id>]", Args: []string{"close", "--session", "work"}},
				{ID: "all", Syntax: "--all", Args: []string{"close", "--all"}},
			},
		},
		{
			name: "click",
			root: newNagiClickRoot(),
			variants: []nagiUsageVariantContract{
				{ID: "node", Syntax: "<index|@eN> [--session <id>] [--json]", Args: []string{"click", "@e3"}},
				{ID: "refs", Syntax: "--refs <@eN,@eN,...> [--session <id>] [--json]", Args: []string{"click", "--refs", "@e1,@e2"}},
				{ID: "coordinates", Syntax: "<x> <y> [--session <id>] [--json]", Args: []string{"click", "120", "240"}},
			},
		},
		{
			name: "screenshot",
			root: newNagiScreenshotRoot(30000),
			variants: []nagiUsageVariantContract{
				{
					ID:     "capture",
					Syntax: "[path] [--session <id>] [--full] [--annotate] [--recover-target] [--verbose] [--locator <locator>] [--nth <n>] [--timeout <ms>]",
					Args:   []string{"screenshot", "capture.png", "--recover-target", "--verbose", "--timeout", "45000"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.root.Validate(); err != nil {
				t.Fatal(err)
			}
			assertNagiUsageVariantContract(
				t,
				test.root,
				[]string{"nxctl", test.name},
				test.variants,
			)
		})
	}
}

type nagiUsageVariantContract struct {
	ID     string
	Syntax string
	Args   []string
}

func assertNagiUsageVariantContract(
	t *testing.T,
	root *nagicli.Command,
	path []string,
	contracts []nagiUsageVariantContract,
) {
	t.Helper()
	document, err := root.HelpDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	variants := document.UsageVariants()
	if len(variants) != len(contracts) {
		t.Fatalf("usage variant count differs: variants=%d contracts=%d", len(variants), len(contracts))
	}
	for index, contract := range contracts {
		if variants[index].ID != contract.ID {
			t.Fatalf("usage variant differs at %d: variant=%q contract=%q", index, variants[index].ID, contract.ID)
		}
		if variants[index].Syntax != contract.Syntax {
			t.Fatalf("usage syntax differs for %q: variant=%q contract=%q", contract.ID, variants[index].Syntax, contract.Syntax)
		}
		if _, err := root.Parse(contract.Args); err != nil {
			t.Fatalf("usage variant %q representative argv was rejected: %v", contract.ID, err)
		}
	}
}

func TestNagiCloseOptionGroupUsesCommandLinePresence(t *testing.T) {
	root := newNagiCloseRoot()
	for _, args := range [][]string{
		{"close"},
		{"close", "--all"},
		{"close", "--session", "work"},
	} {
		if _, err := root.Parse(args); err != nil {
			t.Fatalf("unexpected parse error for %#v: %v", args, err)
		}
	}

	_, err := root.Parse([]string{"close", "--all", "--session", "work"})
	var diagnostic *nagicli.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("expected diagnostic, got %v", err)
	}
	if diagnostic.Code() != nagicli.CodeOptionGroup {
		t.Fatalf("unexpected diagnostic code: %s", diagnostic.Code())
	}
}

func TestNagiClickInvocationValidation(t *testing.T) {
	root := newNagiClickRoot()
	for _, args := range [][]string{
		{"click", "@e3"},
		{"click", "120", "240", "--json"},
		{"click", "--refs", "@e1,@e2", "--session", "work"},
	} {
		if _, err := root.Parse(args); err != nil {
			t.Fatalf("unexpected parse error for %#v: %v", args, err)
		}
	}

	for _, args := range [][]string{
		{"click"},
		{"click", "--refs", "@e1", "@e2"},
		{"click", "left", "20"},
	} {
		if _, err := root.Parse(args); err == nil {
			t.Fatalf("expected parse error for %#v", args)
		}
	}
}

func TestNagiScreenshotTypedInvocation(t *testing.T) {
	result, err := newNagiScreenshotRoot(30000).Parse(
		[]string{"screenshot", "capture.png", "--recover-target", "--verbose", "--timeout", "45000"},
	)
	if err != nil {
		t.Fatal(err)
	}
	arguments := nagiScreenshotArgumentsFromInvocation(result.Invocation())
	if arguments.Paths[0] != "capture.png" || !arguments.Recover || !arguments.Verbose {
		t.Fatalf("unexpected screenshot arguments: %#v", arguments)
	}
	if arguments.Timeout != 45000 {
		t.Fatalf("unexpected typed values: %#v", arguments)
	}
	result, err = newNagiScreenshotRoot(30000).Parse(
		[]string{"screenshot", "capture.png", "--locator", "label=Email", "--nth", "2", "--timeout", "45000"},
	)
	if err != nil {
		t.Fatal(err)
	}
	arguments = nagiScreenshotArgumentsFromInvocation(result.Invocation())
	if arguments.Locator != "label=Email" || arguments.Nth != 2 {
		t.Fatalf("unexpected locator arguments: %#v", arguments)
	}
	result, err = newNagiScreenshotRoot(30000).Parse([]string{"screenshot"})
	if err != nil {
		t.Fatal(err)
	}
	timeout, accessErr := nagicli.RequireValueAs[int](result.Invocation(), "timeout")
	if accessErr != nil || timeout != 30000 {
		t.Fatalf("unexpected required typed value: value=%d err=%v", timeout, accessErr)
	}
	if _, accessErr := nagicli.RequireValueAs[string](result.Invocation(), "timeout"); accessErr == nil ||
		accessErr.Kind() != nagicli.ValueTypeMismatch {
		t.Fatalf("expected typed access mismatch, got %v", accessErr)
	}

	for _, args := range [][]string{
		{"--nth", "1"},
		{"--locator", "@e1", "--full"},
		{"--locator", "@e1", "--recover-target"},
		{"--timeout", "0"},
		{"one.png", "two.png"},
	} {
		argv := append([]string{"screenshot"}, args...)
		if _, err := newNagiScreenshotRoot(30000).Parse(argv); err == nil {
			t.Fatalf("expected validation error for %#v", args)
		}
	}
}

func TestNagiStructuredValidatorDiagnostic(t *testing.T) {
	_, err := newNagiScreenshotRoot(30000).Parse([]string{"screenshot", "--nth", "0"})
	var diagnostic *nagicli.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("expected diagnostic, got %v", err)
	}
	targets := diagnostic.Targets()
	if diagnostic.Code() != nagicli.CodeValidation ||
		len(targets) != 1 ||
		targets[0].Kind() != nagicli.TargetOption ||
		targets[0].ValueID() != "nth" {
		t.Fatalf("unexpected diagnostic: %#v", diagnostic)
	}
	expectedPath := []string{"nxctl", "screenshot"}
	if !slices.Equal(targets[0].CommandIDPath(), expectedPath) {
		t.Fatalf("unexpected target path: %#v", targets[0].CommandIDPath())
	}
}

func TestNagiRuntimeCompatibilityPolicy(t *testing.T) {
	command := nagicli.NewCommand("probe").
		UsageVariant("value", "<VALUE>").
		Argument(nagicli.Positional("value").Required()).
		Handle(func(context *nagicli.Context, invocation *nagicli.Invocation) (nagicli.Outcome, error) {
			value, _ := invocation.RawValue("value")
			if _, err := fmt.Fprintln(context.Stdout(), value); err != nil {
				return nagicli.Outcome{}, err
			}
			return nagicli.Success(), nil
		})
	policy := nagicli.DefaultRuntimePolicy().
		WithExitCodePolicy(
			nagicli.DefaultExitCodePolicy().
				WithStatus(nagicli.CategoryUsage, nagicli.StatusFailure),
		).
		WithHelpRenderer(nagiRuntimeUsageRenderer{}).
		WithDiagnosticRenderer(nagiRuntimeDiagnosticRenderer{})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	context := nagicli.NewContext(strings.NewReader(""), &stdout, &stderr, nil, "")
	result, err := command.Parse([]string{"--help"})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := command.RunParsedWithPolicy(context, result, policy)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status() != nagicli.StatusSuccess || stdout.String() != "usage: probe <VALUE>\n" {
		t.Fatalf("unexpected runtime help: status=%d stdout=%q", outcome.Status(), stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	_, err = command.Parse(nil)
	var diagnostic *nagicli.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("expected diagnostic, got %v", err)
	}
	if policy.StatusForDiagnostic(diagnostic) != nagicli.StatusFailure {
		t.Fatalf("unexpected usage status: %d", policy.StatusForDiagnostic(diagnostic))
	}
	rendered := policy.RenderDiagnostic(diagnostic)
	if !strings.Contains(rendered, `required argument "value" is missing`) ||
		!strings.Contains(rendered, "hint: run `probe --help` for details") {
		t.Fatalf("unexpected runtime diagnostic: %s", rendered)
	}

	stdout.Reset()
	stderr.Reset()
	result, err = command.Parse([]string{"accepted"})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err = command.RunInvocationWithPolicy(context, result.Invocation(), policy)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status() != nagicli.StatusSuccess || stdout.String() != "accepted\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected runtime handler result: status=%d stdout=%q stderr=%q", outcome.Status(), stdout.String(), stderr.String())
	}
}

type nagiRuntimeUsageRenderer struct{}

func (nagiRuntimeUsageRenderer) RenderHelp(document nagicli.HelpDocument) string {
	var output strings.Builder
	for index, usage := range document.Usage() {
		prefix := "   or: "
		if index == 0 {
			prefix = "usage: "
		}
		output.WriteString(prefix)
		output.WriteString(usage)
		output.WriteByte('\n')
	}
	return output.String()
}

type nagiRuntimeDiagnosticRenderer struct{}

func (nagiRuntimeDiagnosticRenderer) RenderDiagnostic(diagnostic *nagicli.Diagnostic) string {
	command := strings.Join(diagnostic.CommandPath(), " ")
	return fmt.Sprintf("%s\nhint: run `%s --help` for details\n", diagnostic.Message(), command)
}

var _ nagicli.HelpRenderer = nagiRuntimeUsageRenderer{}
var _ nagicli.DiagnosticRenderer = nagiRuntimeDiagnosticRenderer{}
