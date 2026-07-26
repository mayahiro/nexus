package cli

import (
	"reflect"
	"testing"

	nagicli "github.com/mayahiro/nagicli-go"
)

func TestNagiApplicationSchema(t *testing.T) {
	application := newNagiApplication()
	if err := application.Validate(); err != nil {
		t.Fatal(err)
	}

	document, err := application.HelpDocument([]string{"nxctl"})
	if err != nil {
		t.Fatal(err)
	}
	commandIDs := make([]string, 0, len(document.Commands()))
	for _, command := range document.Commands() {
		commandIDs = append(commandIDs, command.ID)
	}
	expected := []string{
		"attach",
		"back",
		"batch",
		"browser",
		"click",
		"compare",
		"close",
		"dblclick",
		"eval",
		"fill",
		"find",
		"flow",
		"get",
		"hover",
		"inspect",
		"input",
		"keys",
		"navigate",
		"open",
		"observe",
		"rightclick",
		"scroll",
		"screenshot",
		"select",
		"sessions",
		"state",
		"type",
		"upload",
		"viewport",
		"wait",
		"detach",
		"daemon",
		"doctor",
		"help",
	}
	if !reflect.DeepEqual(commandIDs, expected) {
		t.Fatalf("unexpected root commands: %#v", commandIDs)
	}
}

func TestNagiApplicationRepresentativeInvocations(t *testing.T) {
	application := newNagiApplication()
	tests := []struct {
		name string
		args []string
	}{
		{name: "attach browser", args: []string{"attach", "browser", "--session", "work"}},
		{name: "back", args: []string{"back"}},
		{name: "batch", args: []string{"batch", "--cmd", "help"}},
		{name: "browser setup", args: []string{"browser", "setup"}},
		{name: "browser update", args: []string{"browser", "update"}},
		{name: "browser status", args: []string{"browser", "status"}},
		{name: "browser uninstall", args: []string{"browser", "uninstall", "--name", "chromium"}},
		{name: "click node", args: []string{"click", "@e1"}},
		{name: "compare urls", args: []string{"compare", "https://old.example.com", "https://new.example.com"}},
		{name: "compare sessions", args: []string{"compare", "--old-session", "old", "--new-session", "new"}},
		{name: "compare mixed endpoints", args: []string{"compare", "--old-session", "old", "--new-url", "https://new.example.com"}},
		{name: "compare validate decisions", args: []string{"compare", "validate-decisions", "--decisions-file", "decisions.jsonl"}},
		{name: "compare normalize decisions", args: []string{"compare", "normalize-decisions", "--decisions-file", "decisions.jsonl"}},
		{name: "compare materialize decisions", args: []string{"compare", "materialize-decisions", "--decisions-file", "decisions.jsonl", "--compare-json", "compare.json"}},
		{name: "compare repair decisions", args: []string{"compare", "repair-decisions", "--decisions-file", "decisions.jsonl", "--compare-json", "compare.json"}},
		{name: "compare audit decisions", args: []string{"compare", "audit-decisions", "--decisions-file", "decisions.jsonl", "--compare-json", "compare.json"}},
		{name: "close", args: []string{"close"}},
		{name: "dblclick", args: []string{"dblclick", "@e1"}},
		{name: "eval", args: []string{"eval", "document.title"}},
		{name: "fill", args: []string{"fill", "@e1", "value"}},
		{name: "find role", args: []string{"find", "role", "button", "click", "--name", "Save"}},
		{name: "find text", args: []string{"find", "text", "Welcome", "--all"}},
		{name: "find label", args: []string{"find", "label", "Email", "fill", "person@example.com"}},
		{name: "find testid", args: []string{"find", "testid", "submit", "click"}},
		{name: "find href", args: []string{"find", "href", "/docs", "get", "attributes"}},
		{name: "flow run", args: []string{"flow", "run", "--manifest", "flow.json"}},
		{name: "get title", args: []string{"get", "title"}},
		{name: "get html", args: []string{"get", "html", "--selector", "main"}},
		{name: "get bbox selector", args: []string{"get", "bbox", "--selector", ".hero"}},
		{name: "get node", args: []string{"get", "text", "@e1"}},
		{name: "get refs", args: []string{"get", "attributes", "--refs", "@e1,@e2"}},
		{name: "hover", args: []string{"hover", "@e1"}},
		{name: "inspect locator", args: []string{"inspect", `role button --name "Save"`, "--old-session", "old", "--new-session", "new"}},
		{name: "inspect selector", args: []string{"inspect", "--selector", "main", "--old-session", "old", "--new-session", "new"}},
		{name: "inspect scope selector", args: []string{"inspect", "--scope-selector", "main", "--old-session", "old", "--new-session", "new"}},
		{name: "inspect scopes", args: []string{"inspect", "--old-scope-selector", "#old", "--new-scope-selector", "#new", "--old-session", "old", "--new-session", "new"}},
		{name: "input", args: []string{"input", "@e1", "value"}},
		{name: "keys", args: []string{"keys", "Enter"}},
		{name: "navigate", args: []string{"navigate", "https://example.com"}},
		{name: "open", args: []string{"open", "https://example.com"}},
		{name: "observe", args: []string{"observe", "--session", "work"}},
		{name: "rightclick", args: []string{"rightclick", "@e1"}},
		{name: "scroll", args: []string{"scroll", "down", "--amount", "400"}},
		{name: "screenshot", args: []string{"screenshot", "capture.png"}},
		{name: "select", args: []string{"select", "@e1", "choice"}},
		{name: "sessions", args: []string{"sessions"}},
		{name: "state", args: []string{"state", "--role", "button", "--limit", "20"}},
		{name: "type", args: []string{"type", "value"}},
		{name: "upload", args: []string{"upload", "@e1", "artifact.txt"}},
		{name: "viewport", args: []string{"viewport", "1280x720"}},
		{name: "wait selector", args: []string{"wait", "selector", ".ready", "--state", "visible"}},
		{name: "wait text", args: []string{"wait", "text", "Ready"}},
		{name: "wait url", args: []string{"wait", "url", "/dashboard"}},
		{name: "wait navigation", args: []string{"wait", "navigation"}},
		{name: "wait function", args: []string{"wait", "function", "window.ready"}},
		{name: "detach", args: []string{"detach", "--session", "work"}},
		{name: "daemon", args: []string{"daemon"}},
		{name: "doctor", args: []string{"doctor"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := application.Parse(test.args)
			if err != nil {
				t.Fatalf("representative argv was rejected: args=%#v err=%v", test.args, err)
			}
			if result.Kind() != nagicli.ParseInvocation {
				t.Fatalf("unexpected parse kind for %#v: %v", test.args, result.Kind())
			}
		})
	}
}

func TestNagiApplicationUsageVariantContracts(t *testing.T) {
	application := newNagiApplication()
	tests := []struct {
		name      string
		path      []string
		variantID string
		syntax    string
		args      []string
	}{
		{name: "find role action", path: []string{"nxctl", "find", "role"}, variantID: "action", syntax: "<QUERY> <click|input|fill|get> [VALUE] [--name <TEXT>] [--nth <N>] [--session <ID>] [--json]", args: []string{"find", "role", "button", "click", "--name", "Save"}},
		{name: "find role all", path: []string{"nxctl", "find", "role"}, variantID: "all", syntax: "<QUERY> --all [--name <TEXT>] [--session <ID>] [--json]", args: []string{"find", "role", "button", "--all", "--name", "Save"}},
		{name: "find text action", path: []string{"nxctl", "find", "text"}, variantID: "action", syntax: "<QUERY> <click|input|fill|get> [VALUE] [--nth <N>] [--session <ID>] [--json]", args: []string{"find", "text", "Welcome", "get", "text"}},
		{name: "find text all", path: []string{"nxctl", "find", "text"}, variantID: "all", syntax: "<QUERY> --all [--session <ID>] [--json]", args: []string{"find", "text", "Welcome", "--all"}},
		{name: "get title", path: []string{"nxctl", "get"}, variantID: "title", syntax: "title [--session <ID>] [--json]", args: []string{"get", "title"}},
		{name: "get html", path: []string{"nxctl", "get"}, variantID: "html", syntax: "html [--selector <CSS>] [--session <ID>] [--json]", args: []string{"get", "html", "--selector", "main"}},
		{name: "get bbox selector", path: []string{"nxctl", "get"}, variantID: "bbox-selector", syntax: "bbox --selector <CSS> [--session <ID>] [--json]", args: []string{"get", "bbox", "--selector", ".hero"}},
		{name: "get node", path: []string{"nxctl", "get"}, variantID: "node", syntax: "text|value|attributes|bbox <NODE> [--session <ID>] [--json]", args: []string{"get", "text", "@e1"}},
		{name: "get refs", path: []string{"nxctl", "get"}, variantID: "refs", syntax: "text|value|attributes|bbox --refs <NODES> [--session <ID>] [--json]", args: []string{"get", "text", "--refs", "@e1,@e2"}},
		{name: "inspect locator", path: []string{"nxctl", "inspect"}, variantID: "locator", syntax: "<LOCATOR> --old-session <ID> --new-session <ID> [--nth <N>] [--scope-selector <CSS>] [--old-scope-selector <CSS>] [--new-scope-selector <CSS>] [--css-property <NAME>]... [--layout-context] [--json]", args: []string{"inspect", "role button", "--old-session", "old", "--new-session", "new"}},
		{name: "inspect selector", path: []string{"nxctl", "inspect"}, variantID: "selector", syntax: "--selector <CSS> --old-session <ID> --new-session <ID> [--old-scope-selector <CSS>] [--new-scope-selector <CSS>] [--css-property <NAME>]... [--layout-context] [--json]", args: []string{"inspect", "--selector", "main", "--old-session", "old", "--new-session", "new"}},
		{name: "inspect scope selector", path: []string{"nxctl", "inspect"}, variantID: "scope-selector", syntax: "--scope-selector <CSS> --old-session <ID> --new-session <ID> [--old-scope-selector <CSS>] [--new-scope-selector <CSS>] [--css-property <NAME>]... [--layout-context] [--json]", args: []string{"inspect", "--scope-selector", "main", "--old-session", "old", "--new-session", "new"}},
		{name: "inspect scopes", path: []string{"nxctl", "inspect"}, variantID: "scopes", syntax: "--old-scope-selector <CSS> --new-scope-selector <CSS> --old-session <ID> --new-session <ID> [--css-property <NAME>]... [--layout-context] [--json]", args: []string{"inspect", "--old-scope-selector", "#old", "--new-scope-selector", "#new", "--old-session", "old", "--new-session", "new"}},
		{name: "wait selector", path: []string{"nxctl", "wait"}, variantID: "selector", syntax: "selector <CSS> [--state attached|detached|visible|hidden] [--timeout <MS>] [--session <ID>] [--json]", args: []string{"wait", "selector", ".ready"}},
		{name: "wait text", path: []string{"nxctl", "wait"}, variantID: "text", syntax: "text <VALUE> [--timeout <MS>] [--session <ID>] [--json]", args: []string{"wait", "text", "Ready"}},
		{name: "wait url", path: []string{"nxctl", "wait"}, variantID: "url", syntax: "url <VALUE> [--timeout <MS>] [--session <ID>] [--json]", args: []string{"wait", "url", "/dashboard"}},
		{name: "wait navigation", path: []string{"nxctl", "wait"}, variantID: "navigation", syntax: "navigation [--timeout <MS>] [--session <ID>] [--json]", args: []string{"wait", "navigation"}},
		{name: "wait function", path: []string{"nxctl", "wait"}, variantID: "function", syntax: "function <EXPRESSION> [--timeout <MS>] [--session <ID>] [--json]", args: []string{"wait", "function", "window.ready"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document, err := application.HelpDocument(test.path)
			if err != nil {
				t.Fatal(err)
			}
			var found bool
			for _, variant := range document.UsageVariants() {
				if variant.ID != test.variantID {
					continue
				}
				if variant.Syntax != test.syntax {
					t.Fatalf("usage syntax differs: got=%q want=%q", variant.Syntax, test.syntax)
				}
				found = true
				break
			}
			if !found {
				t.Fatalf("usage variant %q was not found", test.variantID)
			}
			if _, err := application.Parse(test.args); err != nil {
				t.Fatalf("usage representative argv was rejected: args=%#v err=%v", test.args, err)
			}
		})
	}
}

func TestNagiApplicationRejectsInvalidForms(t *testing.T) {
	application := newNagiApplication()
	tests := []struct {
		name string
		args []string
	}{
		{name: "empty attach session", args: []string{"attach", "browser", "--session", " "}},
		{name: "unknown attach backend", args: []string{"attach", "browser", "--session", "work", "--backend", "webkit"}},
		{name: "empty batch command", args: []string{"batch", "--cmd", " "}},
		{name: "unknown browser name", args: []string{"browser", "uninstall", "--name", "webkit"}},
		{name: "empty eval source", args: []string{"eval", ""}},
		{name: "empty find query", args: []string{"find", "role", "", "click"}},
		{name: "invalid get refs", args: []string{"get", "text", "--refs", "@e0"}},
		{name: "invalid inspect locator", args: []string{"inspect", "role", "--old-session", "old", "--new-session", "new"}},
		{name: "one-sided inspect scope", args: []string{"inspect", "--old-scope-selector", "#old", "--old-session", "old", "--new-session", "new"}},
		{name: "empty observe session", args: []string{"observe", "--session", " "}},
		{name: "unknown open backend", args: []string{"open", "https://example.com", "--backend", "webkit"}},
		{name: "wait value missing", args: []string{"wait", "text"}},
		{name: "wait state on text", args: []string{"wait", "text", "Ready", "--state", "visible"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := application.Parse(test.args); err == nil {
				t.Fatalf("expected parse rejection for %#v", test.args)
			}
		})
	}
}
