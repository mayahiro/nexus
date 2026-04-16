package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/mayahiro/nexus/internal/browsermgr"
)

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl <command>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "commands:")
	fmt.Fprintln(w, "  attach")
	fmt.Fprintln(w, "  back")
	fmt.Fprintln(w, "  batch")
	fmt.Fprintln(w, "  browser")
	fmt.Fprintln(w, "  click")
	fmt.Fprintln(w, "  compare")
	fmt.Fprintln(w, "  close")
	fmt.Fprintln(w, "  help")
	fmt.Fprintln(w, "  eval")
	fmt.Fprintln(w, "  dblclick")
	fmt.Fprintln(w, "  fill")
	fmt.Fprintln(w, "  find")
	fmt.Fprintln(w, "  flow")
	fmt.Fprintln(w, "  get")
	fmt.Fprintln(w, "  hover")
	fmt.Fprintln(w, "  inspect")
	fmt.Fprintln(w, "  input")
	fmt.Fprintln(w, "  keys")
	fmt.Fprintln(w, "  navigate")
	fmt.Fprintln(w, "  open")
	fmt.Fprintln(w, "  observe")
	fmt.Fprintln(w, "  rightclick")
	fmt.Fprintln(w, "  scroll")
	fmt.Fprintln(w, "  screenshot")
	fmt.Fprintln(w, "  select")
	fmt.Fprintln(w, "  sessions")
	fmt.Fprintln(w, "  state")
	fmt.Fprintln(w, "  type")
	fmt.Fprintln(w, "  upload")
	fmt.Fprintln(w, "  viewport")
	fmt.Fprintln(w, "  wait")
	fmt.Fprintln(w, "  detach")
	fmt.Fprintln(w, "  daemon")
	fmt.Fprintln(w, "  doctor")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "run `nxctl help <command>` for command-specific usage")
	fmt.Fprintln(w, "")
	printDocLink(w, "ai guide", aiUsageDocURL)
	printDocLink(w, "migration playbook", migrationPlaybookDocURL)
}

func printCommandHelp(w io.Writer, command string) bool {
	switch command {
	case "attach":
		printAttachHelp(w)
	case "back":
		printBackHelp(w)
	case "batch":
		printBatchHelp(w)
	case "browser":
		printBrowserHelp(w)
	case "click":
		printClickHelp(w)
	case "compare":
		printCompareHelp(w)
	case "close":
		printCloseHelp(w)
	case "dblclick":
		printNodeActionHelp(w, "dblclick")
	case "eval":
		printEvalHelp(w)
	case "fill":
		printFillHelp(w)
	case "find":
		printFindHelp(w)
	case "flow":
		printFlowHelp(w)
	case "get":
		printGetHelp(w)
	case "hover":
		printNodeActionHelp(w, "hover")
	case "inspect":
		printInspectHelp(w)
	case "input":
		printInputHelp(w)
	case "keys":
		printKeysHelp(w)
	case "navigate":
		printNavigateHelp(w)
	case "observe":
		printObserveHelp(w)
	case "open":
		printOpenHelp(w)
	case "rightclick":
		printNodeActionHelp(w, "rightclick")
	case "scroll":
		printScrollHelp(w)
	case "screenshot":
		printScreenshotHelp(w)
	case "select":
		printSelectHelp(w)
	case "sessions":
		printSessionsHelp(w)
	case "state":
		printStateHelp(w)
	case "type":
		printTypeHelp(w)
	case "upload":
		printUploadHelp(w)
	case "viewport":
		printViewportHelp(w)
	case "wait":
		printWaitHelp(w)
	case "detach":
		printDetachHelp(w)
	case "daemon":
		printDaemonHelp(w)
	case "doctor":
		printDoctorHelp(w)
	default:
		return false
	}
	return true
}

func printAttachHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl attach browser --session <id> --backend <name> [--url <url>] [--viewport <width>x<height>] [--target-ref <path>]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "targets:")
	fmt.Fprintln(w, "  browser")
}

func printAttachBrowserHelp(w io.Writer) {
	fmt.Fprintf(w, "usage: nxctl attach browser --session <id> --backend chromium|lightpanda [--url <url>] [--viewport <width>x<height>] [--target-ref <path>]\n")
	fmt.Fprintf(w, "default viewport: %dx%d\n", defaultViewportWidth, defaultViewportHeight)
}

func printBackHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl back [--session <id>] [--json]")
}

func printBatchHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl batch --cmd "open https://example.com" --cmd "state" [--json]`)
	fmt.Fprintln(w, `   or: nxctl batch --cmd "find role button --all" --cmd "screenshot annotated.png --annotate" [--json]`)
}

func printBrowserHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl browser <setup|update|status|uninstall>")
	fmt.Fprintln(w, "   or: nxctl browser uninstall [--name chromium|lightpanda]")
}

func printClickHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl click <index|@eN> [--session <id>] [--json]")
	fmt.Fprintln(w, "   or: nxctl click <x> <y> [--session <id>] [--json]")
}

func printCloseHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl close [--session <id>]")
	fmt.Fprintln(w, "   or: nxctl close --all")
}

func printDaemonHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl daemon")
}

func printDoctorHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl doctor")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "doctor starts nxd temporarily if needed and stops it after the check")
}

func printEvalHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl eval "js code" [--session <id>] [--json]`)
}

func printFindHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl find role <role> click [--name <text>] [--nth <n>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find role <role> input "text" [--name <text>] [--nth <n>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find role <role> fill "text" [--name <text>] [--nth <n>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find role <role> get text|value|attributes|bbox [--name <text>] [--nth <n>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find role <role> --all [--name <text>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find text "text" click [--nth <n>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find text "text" fill "text" [--nth <n>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find text "text" get text|value|attributes|bbox [--nth <n>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find text "text" --all [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find label "label" input "text" [--nth <n>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find label "label" fill "text" [--nth <n>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find label "label" get value|attributes|bbox [--nth <n>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find label "label" --all [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find testid "value" click|fill|get ... [--nth <n>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find testid "value" --all [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find href "value" click|fill|get ... [--nth <n>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl find href "value" --all [--session <id>] [--json]`)
}

func printGetHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl get title [--session <id>] [--json]")
	fmt.Fprintln(w, "   or: nxctl get html [--selector <css>] [--session <id>] [--json]")
	fmt.Fprintln(w, "   or: nxctl get text|value|attributes|bbox <index|@eN> [--session <id>] [--json]")
}

func printInputHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl input <index|@eN> "text" [--session <id>] [--json]`)
}

func printInspectHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl inspect '<locator>' --old-session <id> --new-session <id> [--nth <n>] [--css-property <name>]... [--json]`)
	fmt.Fprintln(w, `locator: @eN, role <role> [--name <text>], text <text>, label <text>, testid <value>, or href <value>`)
	fmt.Fprintln(w, `examples: nxctl inspect 'role button --name "Submit"' --old-session old --new-session new`)
	fmt.Fprintln(w, `          nxctl inspect 'role button' --old-session old --new-session new --nth 2 --css-property color`)
}

func printFillHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl fill <index|@eN> "text" [--session <id>] [--json]`)
}

func printKeysHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl keys "Enter" [--session <id>] [--json]`)
}

func printNavigateHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl navigate <url> [--session <id>] [--json]")
}

func printNodeActionHelp(w io.Writer, command string) {
	switch command {
	case "hover":
		fmt.Fprintln(w, "usage: nxctl hover <index|@eN> [--session <id>] [--json]")
	case "dblclick":
		fmt.Fprintln(w, "usage: nxctl dblclick <index|@eN> [--session <id>] [--json]")
	case "rightclick":
		fmt.Fprintln(w, "usage: nxctl rightclick <index|@eN> [--session <id>] [--json]")
	}
}

func printObserveHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl observe [--session <id>] [--json] [--text] [--tree] [--screenshot] [--full]")
}

func printOpenHelp(w io.Writer) {
	fmt.Fprintf(w, "usage: nxctl open <url> [--session <id>] [--backend chromium|lightpanda] [--viewport <width>x<height>] [--json]\n")
	fmt.Fprintf(w, "default viewport: %dx%d\n", defaultViewportWidth, defaultViewportHeight)
}

func printScrollHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl scroll up|down [--session <id>] [--node <index>] [--amount <px>] [--json]")
}

func printScreenshotHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl screenshot [path] [--session <id>] [--full] [--annotate]")
}

func printSelectHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl select <index|@eN> "value" [--session <id>] [--json]`)
}

func printSessionsHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl sessions [--json]")
}

func printStateHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl state [--session <id>] [--role <role>] [--name <text>] [--text <text>] [--testid <value>] [--href <value>] [--limit <n>] [--json]")
}

func printTypeHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl type "text" [--session <id>] [--json]`)
}

func printUploadHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl upload <index|@eN> <path> [--session <id>] [--json]")
}

func printViewportHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl viewport <width>x<height> [--session <id>] [--json]")
	fmt.Fprintf(w, "default viewport: %dx%d\n", defaultViewportWidth, defaultViewportHeight)
}

func printWaitHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: nxctl wait selector "<css>" [--state attached|detached|visible|hidden] [--timeout <ms>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl wait text "value" [--timeout <ms>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl wait url "value" [--timeout <ms>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl wait navigation [--timeout <ms>] [--session <id>] [--json]`)
	fmt.Fprintln(w, `   or: nxctl wait function "js expr" [--timeout <ms>] [--session <id>] [--json]`)
	fmt.Fprintln(w, "")
	printDocLink(w, "compare guide", aiCompareDocURL)
	printDocLink(w, "ai guide", aiUsageDocURL)
}

func printDetachHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl detach --session <id>")
}

func printBrowserResults(w io.Writer, result browsermgr.SetupResult) {
	for _, browser := range result.Browsers {
		status := "unchanged"
		if browser.Changed {
			status = "updated"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", browser.Name, browser.Version, status, browser.ExecutablePath)
	}
}

func printBrowserStatus(w io.Writer, status browsermgr.Status) {
	for _, browser := range status.Browsers {
		state := "not_installed"
		if browser.Installed {
			state = "installed"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", browser.Name, browser.Version, state, browser.ExecutablePath)
	}
}

func printEvalValue(w io.Writer, value interface{}) error {
	switch value := value.(type) {
	case nil:
		_, err := fmt.Fprintln(w, "null")
		return err
	case string:
		_, err := fmt.Fprintln(w, value)
		return err
	default:
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}
}
