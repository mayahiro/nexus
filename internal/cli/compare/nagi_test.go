package comparecmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	nagicli "github.com/mayahiro/nagicli-go"

	"github.com/mayahiro/nexus/internal/rpc"
)

const expectedCompareExecutionUsage = "[--backend chromium] [--target-ref <path>] [--viewport <width>x<height>] [--match-mode exact|stable|heuristic|histogram] [--node-scope current|actionable|semantic|all] [--matching-debug] [--decisions-file <jsonl>] [--review-dir <dir>] [--wait-selector <css>] [--scope-selector <css>] [--old-scope-selector <css>] [--new-scope-selector <css>] [--wait-function <js>] [--wait-network-idle] [--wait-timeout <ms>] [--compare-css] [--all-css-properties] [--css-property <name>]... [--compare-layout] [--no-default-ignores] [--ignore-text-regex <regex>]... [--ignore-selector <rule>]... [--mask-selector <rule>]..."
const expectedCompareDirectOutputUsage = "[--output-decisions-template <jsonl>] [--output-finding-decisions-template <jsonl>]"
const expectedCompareReportOutputUsage = "[--output-json <file>] [--output-md <file>] [--json]"
const expectedCompareURLUsage = "<old-url> <new-url> " + expectedCompareExecutionUsage + " " + expectedCompareDirectOutputUsage + " " + expectedCompareReportOutputUsage
const expectedCompareEndpointUsage = "(--old-session <id>|--old-url <url>) (--new-session <id>|--new-url <url>) " + expectedCompareExecutionUsage + " " + expectedCompareDirectOutputUsage + " " + expectedCompareReportOutputUsage
const expectedCompareManifestUsage = "--manifest <file> " + expectedCompareExecutionUsage + " [--continue-on-error] [--limit <n>] " + expectedCompareReportOutputUsage
const expectedValidateDecisionsUsage = "--decisions-file <jsonl> [--compare-json <file>] [--review-summary <file>] [--old-session <id>] [--new-session <id>] [--strict] [--json]"
const expectedNormalizeDecisionsUsage = "--decisions-file <jsonl> [--compare-json <file>] [--review-summary <file>] [--output <jsonl>] [--json]"
const expectedMaterializeDecisionsUsage = "--decisions-file <jsonl> --compare-json <file> [--old-session <id>] [--new-session <id>] [--output <jsonl>] [--json]"
const expectedRepairDecisionsUsage = "--decisions-file <jsonl> --compare-json <file> [--old-session <id>] [--new-session <id>] [--output <jsonl>] [--json]"
const expectedAuditDecisionsUsage = "--decisions-file <jsonl> --compare-json <file> [--json]"

func TestNagiCompareSchema(t *testing.T) {
	root := newNagiCompareRoot()
	if err := root.Validate(); err != nil {
		t.Fatal(err)
	}

	document, err := root.HelpDocument([]string{"nxctl", "compare"})
	if err != nil {
		t.Fatal(err)
	}
	variantIDs := make([]string, 0, len(document.UsageVariants()))
	for _, variant := range document.UsageVariants() {
		variantIDs = append(variantIDs, strings.Join(variant.CommandIDPath, "/")+":"+variant.ID)
	}
	expected := []string{
		"nxctl/compare:url-pair",
		"nxctl/compare:endpoint-pair",
		"nxctl/compare:manifest",
		"nxctl/compare/validate-decisions:default",
		"nxctl/compare/normalize-decisions:default",
		"nxctl/compare/materialize-decisions:default",
		"nxctl/compare/repair-decisions:default",
		"nxctl/compare/audit-decisions:default",
	}
	if !reflect.DeepEqual(variantIDs, expected) {
		t.Fatalf("unexpected usage variants: %#v", variantIDs)
	}
}

func TestNagiCompareUsageVariantContract(t *testing.T) {
	root := newNagiCompareRoot()
	contracts := []struct {
		ID      string
		Command string
		Syntax  string
		Args    []string
	}{
		{
			ID:     "url-pair",
			Syntax: expectedCompareURLUsage,
			Args:   []string{"compare", "https://old.example.com", "https://new.example.com"},
		},
		{
			ID:     "endpoint-pair",
			Syntax: expectedCompareEndpointUsage,
			Args:   []string{"compare", "--old-session", "old", "--new-session", "new"},
		},
		{
			ID:     "manifest",
			Syntax: expectedCompareManifestUsage,
			Args:   []string{"compare", "--manifest", "migration.json", "--backend", "chromium", "--scope-selector", "main", "--css-property", "color", "--limit", "10"},
		},
		{
			ID:      "validate-decisions",
			Command: "validate-decisions",
			Syntax:  "validate-decisions " + expectedValidateDecisionsUsage,
			Args:    []string{"compare", "validate-decisions", "--decisions-file", "decisions.jsonl"},
		},
		{
			ID:      "normalize-decisions",
			Command: "normalize-decisions",
			Syntax:  "normalize-decisions " + expectedNormalizeDecisionsUsage,
			Args:    []string{"compare", "normalize-decisions", "--decisions-file", "decisions.jsonl"},
		},
		{
			ID:      "materialize-decisions",
			Command: "materialize-decisions",
			Syntax:  "materialize-decisions " + expectedMaterializeDecisionsUsage,
			Args:    []string{"compare", "materialize-decisions", "--decisions-file", "decisions.jsonl", "--compare-json", "compare.json"},
		},
		{
			ID:      "repair-decisions",
			Command: "repair-decisions",
			Syntax:  "repair-decisions " + expectedRepairDecisionsUsage,
			Args:    []string{"compare", "repair-decisions", "--decisions-file", "decisions.jsonl", "--compare-json", "compare.json"},
		},
		{
			ID:      "audit-decisions",
			Command: "audit-decisions",
			Syntax:  "audit-decisions " + expectedAuditDecisionsUsage,
			Args:    []string{"compare", "audit-decisions", "--decisions-file", "decisions.jsonl", "--compare-json", "compare.json"},
		},
	}
	for index := range contracts {
		if contracts[index].Command != "" {
			contracts[index].ID = "default"
		}
	}

	document, err := root.HelpDocument([]string{"nxctl", "compare"})
	if err != nil {
		t.Fatal(err)
	}
	variants := document.UsageVariants()
	if len(variants) != len(contracts) {
		t.Fatalf("unexpected compare usage variants: %#v", variants)
	}
	for index, contract := range contracts {
		variant := variants[index]
		if variant.ID != contract.ID {
			t.Fatalf("usage variant differs at %d: variant=%q contract=%q", index, variant.ID, contract.ID)
		}
		if variant.Syntax != contract.Syntax {
			t.Fatalf("usage syntax differs for %q: variant=%q contract=%q", contract.ID, variant.Syntax, contract.Syntax)
		}
		expectedPath := []string{"nxctl", "compare"}
		if contract.Command != "" {
			expectedPath = append(expectedPath, contract.Command)
		}
		if !reflect.DeepEqual(variant.CommandIDPath, expectedPath) {
			t.Fatalf("usage command path differs for %q: variant=%q contract=%q", contract.ID, variant.CommandIDPath, expectedPath)
		}
		if _, err := root.Parse(contract.Args); err != nil {
			t.Fatalf("usage variant %q representative argv was rejected: %v", contract.ID, err)
		}
		if contract.Command == "" {
			continue
		}
		childDocument, err := root.HelpDocument([]string{"nxctl", "compare", contract.Command})
		if err != nil {
			t.Fatal(err)
		}
		childVariants := childDocument.UsageVariants()
		if len(childVariants) != 1 {
			t.Fatalf("unexpected %s variants: %#v", contract.Command, childVariants)
		}
		expectedChildSyntax := strings.TrimPrefix(contract.Syntax, contract.Command+" ")
		if childVariants[0].Syntax != expectedChildSyntax {
			t.Fatalf("child usage differs for %q: variant=%q contract=%q", contract.ID, childVariants[0].Syntax, expectedChildSyntax)
		}
		expectedSyntax := contract.Command + " " + childVariants[0].Syntax
		if variant.Syntax != expectedSyntax {
			t.Fatalf("parent and child usage differ: parent=%q child=%q", variant.Syntax, expectedSyntax)
		}
	}
}

func TestNagiCompareCommandLocalValueScopes(t *testing.T) {
	root := newNagiCompareRoot()
	result, err := root.Parse([]string{
		"compare",
		"--decisions-file", "parent.jsonl",
		"validate-decisions",
		"--decisions-file", "child.jsonl",
	})
	if err != nil {
		t.Fatal(err)
	}
	invocation := result.Invocation()
	if value, _ := invocation.RawValue("decisions-file"); value != "child.jsonl" {
		t.Fatalf("unexpected leaf value: %q", value)
	}
	parent, ok := invocation.Scope("nxctl", "compare")
	if !ok {
		t.Fatal("compare scope was not found")
	}
	if value, _ := parent.RawValue("decisions-file"); value != "parent.jsonl" {
		t.Fatalf("unexpected parent value: %q", value)
	}
}

func TestNagiCompareStructuredValidatorDiagnostic(t *testing.T) {
	_, err := newNagiCompareRoot().Parse([]string{"compare", "validate-decisions"})
	var diagnostic *nagicli.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("expected diagnostic, got %v", err)
	}
	targets := diagnostic.Targets()
	if diagnostic.Code() != nagicli.CodeValidation ||
		len(targets) != 1 ||
		targets[0].Kind() != nagicli.TargetOption ||
		targets[0].ValueID() != "decisions-file" {
		t.Fatalf("unexpected diagnostic: %#v", diagnostic)
	}
	expectedPath := []string{"nxctl", "compare", "validate-decisions"}
	if !reflect.DeepEqual(targets[0].CommandIDPath(), expectedPath) {
		t.Fatalf("unexpected target path: %#v", targets[0].CommandIDPath())
	}
}

func TestNagiCompareValidatorsRejectUnrepresentedForms(t *testing.T) {
	root := newNagiCompareRoot()
	for _, args := range [][]string{
		{"compare"},
		{"compare", "https://old.example.com"},
		{"compare", "--manifest", "migration.json", "--old-session", "old"},
		{"compare", "--old-session", "old"},
		{"compare", "https://old.example.com", "https://new.example.com", "--continue-on-error"},
		{"compare", "https://old.example.com", "https://new.example.com", "--limit", "1"},
		{"compare", "https://old.example.com", "https://new.example.com", "--wait-timeout", "-1"},
		{"compare", "https://old.example.com", "https://new.example.com", "--backend", "lightpanda"},
		{"compare", "https://old.example.com", "https://new.example.com", "--match-mode", "unknown"},
		{"compare", "https://old.example.com", "https://new.example.com", "--node-scope", "unknown"},
		{"compare", "https://old.example.com", "https://new.example.com", "--node-scope", "all"},
		{"compare", "--manifest", "migration.json", "--output-decisions-template", "decisions.jsonl"},
		{"compare", "--manifest", "migration.json", "--output-finding-decisions-template", "findings.jsonl"},
		{"compare", "validate-decisions"},
		{"compare", "validate-decisions", "--decisions-file", "decisions.jsonl", "--old-session", "old"},
		{"compare", "materialize-decisions", "--decisions-file", "decisions.jsonl"},
		{"compare", "repair-decisions", "--decisions-file", "decisions.jsonl"},
		{"compare", "audit-decisions", "--decisions-file", "decisions.jsonl"},
	} {
		if _, err := root.Parse(args); err == nil {
			t.Fatalf("expected parse error for %#v", args)
		}
	}
}

func TestNagiCompareDirectInvocation(t *testing.T) {
	result, err := newNagiCompareRoot().Parse([]string{
		"compare",
		"https://old.example.com",
		"--css-property", " color ",
		"https://new.example.com",
		"--css-property", "display",
		"--wait-timeout", "15000",
		"--json",
	})
	if err != nil {
		t.Fatal(err)
	}
	arguments := nagiCompareArgumentsFromInvocation(result.Invocation())
	if !reflect.DeepEqual(arguments.Positionals, []string{
		"https://old.example.com",
		"https://new.example.com",
	}) {
		t.Fatalf("unexpected urls: %#v", arguments.Positionals)
	}
	if !reflect.DeepEqual(arguments.CSSProperty, []string{"color", "display"}) {
		t.Fatalf("unexpected css properties: %#v", arguments.CSSProperty)
	}
	if arguments.Backend != "chromium" || arguments.WaitTimeout != 15000 || !arguments.JSON {
		t.Fatalf("unexpected compare arguments: %#v", arguments)
	}
}

func TestNagiCompareRejectsAllAndExplicitCSSProperties(t *testing.T) {
	_, err := newNagiCompareRoot().Parse([]string{
		"compare",
		"https://old.example.com",
		"https://new.example.com",
		"--all-css-properties",
		"--css-property", "color",
	})
	if err == nil {
		t.Fatal("expected all and explicit css properties to be rejected")
	}
}

func TestNagiCompareEndpointUsageForms(t *testing.T) {
	root := newNagiCompareRoot()
	for _, args := range [][]string{
		{"compare", "--old-session", "old", "--new-session", "new"},
		{"compare", "--old-url", "https://old.example.com", "--new-url", "https://new.example.com"},
		{"compare", "--old-session", "old", "--new-url", "https://new.example.com"},
		{"compare", "--old-url", "https://old.example.com", "--new-session", "new"},
	} {
		if _, err := root.Parse(args); err != nil {
			t.Fatalf("endpoint usage was rejected: args=%#v err=%v", args, err)
		}
	}
}

func TestNagiCompareDecisionHandler(t *testing.T) {
	decisionsFile := filepath.Join(t.TempDir(), "decisions.jsonl")
	if err := os.WriteFile(decisionsFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	connectCalls := 0
	connect := func(context.Context) (*rpc.Client, error) {
		connectCalls++
		return nil, errors.New("unexpected client connection")
	}
	root := nagicli.NewCommand("nxctl").
		RequireSubcommand().
		Subcommand(NewNagiCommand(connect))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runtime := nagicli.NewContext(strings.NewReader(""), &stdout, &stderr, nil, "")
	outcome, err := root.Run(runtime, []string{
		"compare",
		"normalize-decisions",
		"--decisions-file",
		decisionsFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status() != nagicli.StatusSuccess {
		t.Fatalf("unexpected status: %d stderr=%s", outcome.Status(), stderr.String())
	}
	if connectCalls != 0 {
		t.Fatalf("normalize-decisions unexpectedly connected %d times", connectCalls)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("unexpected handler output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestNagiCompareOptionalSubcommand(t *testing.T) {
	result, err := newNagiCompareRoot().Parse([]string{
		"compare",
		"validate-decisions",
		"--decisions-file",
		"decisions.jsonl",
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"nxctl", "compare", "validate-decisions"}
	if !reflect.DeepEqual(result.Invocation().CommandPath(), expected) {
		t.Fatalf("unexpected command path: %#v", result.Invocation().CommandPath())
	}

	var help strings.Builder
	PrintHelp(&help)
	if strings.Contains(help.String(), "nxctl compare [OPTIONS] <COMMAND>") {
		t.Fatalf("unexpected generic subcommand usage: %s", help.String())
	}
	if !strings.Contains(help.String(), "nxctl compare validate-decisions") {
		t.Fatalf("missing explicit subcommand usage: %s", help.String())
	}
}
