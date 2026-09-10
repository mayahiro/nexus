# Nexus CLI reference

[Japanese guide](cli-reference_ja.md)

This reference is generated from the same Nagi command graph as `nxctl --help` and `nxctl help <command> [subcommand]`. For an installed version, use its help to confirm the accepted arguments and options. Finite choices appear as possible values where the command graph declares them.

For workflow guidance, see the [AI usage guide](ai/usage.md). Both language editions reproduce the generated help in English so it can be compared directly with the CLI.

Maintainers: edit the command graph, then run `mise run docs` from the repository root. The equivalent command is `go run ./internal/cmd/clidocs`. `go test ./internal/cmd/clidocs` checks that both reference files match the graph. Do not edit the generated command sections by hand. The introductions are maintained in `internal/cmd/clidocs/intro.md` and `intro_ja.md`.

## Commands

- [nxctl](#nxctl)
- [nxctl attach](#nxctl-attach)
- [nxctl attach browser](#nxctl-attach-browser)
- [nxctl back](#nxctl-back)
- [nxctl batch](#nxctl-batch)
- [nxctl browser](#nxctl-browser)
- [nxctl browser setup](#nxctl-browser-setup)
- [nxctl browser update](#nxctl-browser-update)
- [nxctl browser status](#nxctl-browser-status)
- [nxctl browser uninstall](#nxctl-browser-uninstall)
- [nxctl click](#nxctl-click)
- [nxctl compare](#nxctl-compare)
- [nxctl compare validate-decisions](#nxctl-compare-validate-decisions)
- [nxctl compare normalize-decisions](#nxctl-compare-normalize-decisions)
- [nxctl compare materialize-decisions](#nxctl-compare-materialize-decisions)
- [nxctl compare repair-decisions](#nxctl-compare-repair-decisions)
- [nxctl compare audit-decisions](#nxctl-compare-audit-decisions)
- [nxctl close](#nxctl-close)
- [nxctl dblclick](#nxctl-dblclick)
- [nxctl eval](#nxctl-eval)
- [nxctl fill](#nxctl-fill)
- [nxctl find](#nxctl-find)
- [nxctl find role](#nxctl-find-role)
- [nxctl find text](#nxctl-find-text)
- [nxctl find label](#nxctl-find-label)
- [nxctl find testid](#nxctl-find-testid)
- [nxctl find href](#nxctl-find-href)
- [nxctl find aria-label](#nxctl-find-aria-label)
- [nxctl find css](#nxctl-find-css)
- [nxctl flow](#nxctl-flow)
- [nxctl flow run](#nxctl-flow-run)
- [nxctl get](#nxctl-get)
- [nxctl hover](#nxctl-hover)
- [nxctl inspect](#nxctl-inspect)
- [nxctl input](#nxctl-input)
- [nxctl keys](#nxctl-keys)
- [nxctl navigate](#nxctl-navigate)
- [nxctl open](#nxctl-open)
- [nxctl observe](#nxctl-observe)
- [nxctl rightclick](#nxctl-rightclick)
- [nxctl scroll](#nxctl-scroll)
- [nxctl screenshot](#nxctl-screenshot)
- [nxctl select](#nxctl-select)
- [nxctl sessions](#nxctl-sessions)
- [nxctl state](#nxctl-state)
- [nxctl type](#nxctl-type)
- [nxctl upload](#nxctl-upload)
- [nxctl viewport](#nxctl-viewport)
- [nxctl wait](#nxctl-wait)
- [nxctl detach](#nxctl-detach)
- [nxctl daemon](#nxctl-daemon)
- [nxctl doctor](#nxctl-doctor)

## nxctl

Control managed browser sessions and compare interfaces

### Usage

    nxctl [OPTIONS] <COMMAND>

### Commands

- **attach**: Attach a managed target as a named session
- **back**: Navigate one session back
- **batch**: Run multiple nxctl commands sequentially
- **browser**: Manage browser installations
- **click**: Click one or more observed nodes or coordinates
- **compare**: Compare browser interfaces and manage matching decisions
- **close**: Close one or all sessions
- **dblclick**: Double\-click one observed node
- **eval**: Evaluate JavaScript in one session
- **fill**: Replace the value of one observed node
- **find**: Find observed nodes and optionally act on one
- **flow**: Run browser comparison workflows
- **get**: Read values from one browser session
- **hover**: Hover one observed node
- **inspect**: Inspect one node in one session or compare it across two sessions
- **input**: Type text into one observed node
- **keys**: Send a key sequence to one session
- **navigate**: Navigate an attached browser session
- **open**: Open a URL in a managed browser session
- **observe**: Observe one attached session
- **rightclick**: Right\-click one observed node
- **scroll**: Scroll a page or observed node
- **screenshot**: Capture a page or element screenshot
- **select**: Select a value on one observed node
- **sessions**: List attached sessions
- **state**: Show an AI\-readable page state
- **type**: Type text into the active page
- **upload**: Upload a file through a file input
- **viewport**: Set the browser viewport
- **wait**: Wait for a browser condition
- **detach**: Detach one session without stopping the daemon
- **daemon**: Run the Nexus daemon
- **doctor**: Check configuration\, daemon\, and protocol status
- **help**: Print this message or the help of the given command

### Options

- **\-h\, \-\-help**: Print help

### Links

- [AI usage guide](<https://github.com/mayahiro/nexus/blob/main/docs/ai/usage.md>)
- [Migration playbook](<https://github.com/mayahiro/nexus/blob/main/docs/ai/playbooks/migration.md>)

## nxctl attach

Attach a managed target as a named session

### Usage

    nxctl attach browser --session <ID> [--backend chromium] [--url <URL>] [--viewport <WIDTHxHEIGHT>] [--target-ref <PATH>]

### Commands

- **browser**: Attach a browser session

### Options

- **\-h\, \-\-help**: Print help

## nxctl attach browser

Attach a browser session

### Usage

    nxctl attach browser --session <ID> [--backend chromium] [--url <URL>] [--viewport <WIDTHxHEIGHT>] [--target-ref <PATH>]

### Options

- **\-\-session \<ID\>**: Session identifier \[required\]
- **\-\-backend \<NAME\>**: Browser backend \[default\: chromium\] \[possible\: chromium\]
- **\-\-url \<URL\>**: Initial URL
- **\-\-viewport \<WIDTHxHEIGHT\>**: Browser viewport
- **\-\-target\-ref \<PATH\>**: Browser executable or target reference
- **\-h\, \-\-help**: Print help

## nxctl back

Navigate one session back

### Usage

    nxctl back [--session <ID>] [--json]

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-h\, \-\-help**: Print help

## nxctl batch

Run multiple nxctl commands sequentially

### Usage

    nxctl batch --cmd "COMMAND" [--cmd "COMMAND"]... [--keep-going] [--json]

### Options

- **\-\-cmd \<COMMAND\>\.\.\.**: Command to execute \[required\]
- **\-\-keep\-going**: continue after failed commands
- **\-\-json**: Print JSON
- **\-h\, \-\-help**: Print help

### Notes

Commands run in order\; failures stop the batch unless \-\-keep\-going is set

## nxctl browser

Manage browser installations

### Usage

    nxctl browser setup [OPTIONS]

    nxctl browser update [OPTIONS]

    nxctl browser status [OPTIONS]

    nxctl browser uninstall [--name chromium]

### Commands

- **setup**: Install managed browsers
- **update**: Update managed browsers
- **status**: Show managed browser status
- **uninstall**: Uninstall managed browsers

### Options

- **\-h\, \-\-help**: Print help

## nxctl browser setup

Install managed browsers

### Usage

    nxctl browser setup [OPTIONS]

### Options

- **\-h\, \-\-help**: Print help

## nxctl browser update

Update managed browsers

### Usage

    nxctl browser update [OPTIONS]

### Options

- **\-h\, \-\-help**: Print help

## nxctl browser status

Show managed browser status

### Usage

    nxctl browser status [OPTIONS]

### Options

- **\-h\, \-\-help**: Print help

## nxctl browser uninstall

Uninstall managed browsers

### Usage

    nxctl browser uninstall [--name chromium]

### Options

- **\-\-name \<NAME\>**: Browser name \[possible\: chromium\]
- **\-h\, \-\-help**: Print help

## nxctl click

Click one or more observed nodes or coordinates

### Usage

    nxctl click <index|@eN> [--session <id>] [--json]

    nxctl click --refs <@eN,@eN,...> [--session <id>] [--json]

    nxctl click <x> <y> [--session <id>] [--json]

### Arguments

- **\[TARGETS\]\.\.\.**

### Options

- **\-\-session \<VALUE\>**: session id \[default\: default\]
- **\-\-json**: print as json
- **\-\-refs \<VALUE\>**: comma\-separated node refs
- **\-h\, \-\-help**: Print help

## nxctl compare

Compare browser interfaces and manage matching decisions

### Usage

    nxctl compare <old-url> <new-url> [--backend chromium] [--target-ref <path>] [--viewport <width>x<height>] [--match-mode exact|stable|heuristic|histogram] [--node-scope current|actionable|semantic|all] [--matching-debug] [--decisions-file <jsonl>] [--review-dir <dir>] [--wait-selector <css>] [--scope-selector <css>] [--old-scope-selector <css>] [--new-scope-selector <css>] [--wait-function <js>] [--wait-network-idle] [--wait-timeout <ms>] [--compare-css] [--all-css-properties] [--css-property <name>]... [--compare-layout] [--no-default-ignores] [--ignore-text-regex <regex>]... [--ignore-selector <rule>]... [--mask-selector <rule>]... [--output-decisions-template <jsonl>] [--output-finding-decisions-template <jsonl>] [--output-json <file>] [--output-md <file>] [--json]

    nxctl compare (--old-session <id>|--old-url <url>) (--new-session <id>|--new-url <url>) [--backend chromium] [--target-ref <path>] [--viewport <width>x<height>] [--match-mode exact|stable|heuristic|histogram] [--node-scope current|actionable|semantic|all] [--matching-debug] [--decisions-file <jsonl>] [--review-dir <dir>] [--wait-selector <css>] [--scope-selector <css>] [--old-scope-selector <css>] [--new-scope-selector <css>] [--wait-function <js>] [--wait-network-idle] [--wait-timeout <ms>] [--compare-css] [--all-css-properties] [--css-property <name>]... [--compare-layout] [--no-default-ignores] [--ignore-text-regex <regex>]... [--ignore-selector <rule>]... [--mask-selector <rule>]... [--output-decisions-template <jsonl>] [--output-finding-decisions-template <jsonl>] [--output-json <file>] [--output-md <file>] [--json]

    nxctl compare --manifest <file> [--backend chromium] [--target-ref <path>] [--viewport <width>x<height>] [--match-mode exact|stable|heuristic|histogram] [--node-scope current|actionable|semantic|all] [--matching-debug] [--decisions-file <jsonl>] [--review-dir <dir>] [--wait-selector <css>] [--scope-selector <css>] [--old-scope-selector <css>] [--new-scope-selector <css>] [--wait-function <js>] [--wait-network-idle] [--wait-timeout <ms>] [--compare-css] [--all-css-properties] [--css-property <name>]... [--compare-layout] [--no-default-ignores] [--ignore-text-regex <regex>]... [--ignore-selector <rule>]... [--mask-selector <rule>]... [--continue-on-error] [--limit <n>] [--output-json <file>] [--output-md <file>] [--json]

    nxctl compare validate-decisions --decisions-file <jsonl> [--compare-json <file>] [--review-summary <file>] [--old-session <id>] [--new-session <id>] [--strict] [--json]

    nxctl compare normalize-decisions --decisions-file <jsonl> [--compare-json <file>] [--review-summary <file>] [--output <jsonl>] [--json]

    nxctl compare materialize-decisions --decisions-file <jsonl> --compare-json <file> [--old-session <id>] [--new-session <id>] [--output <jsonl>] [--json]

    nxctl compare repair-decisions --decisions-file <jsonl> --compare-json <file> [--old-session <id>] [--new-session <id>] [--output <jsonl>] [--json]

    nxctl compare audit-decisions --decisions-file <jsonl> --compare-json <file> [--json]

### Commands

- **validate\-decisions**: Validate compare decision records
- **normalize\-decisions**: Normalize compare decision records
- **materialize\-decisions**: Materialize decision selectors as observed refs
- **repair\-decisions**: Repair stale refs in compare decision records
- **audit\-decisions**: Audit compare decisions against one report

### Arguments

- **\[URLS\]\.\.\.**

### Options

- **\-\-old\-session \<ID\>**: old session id
- **\-\-new\-session \<ID\>**: new session id
- **\-\-old\-url \<URL\>**: old url
- **\-\-new\-url \<URL\>**: new url
- **\-\-backend \<NAME\>**: browser backend \[default\: chromium\]
- **\-\-target\-ref \<PATH\>**: target ref
- **\-\-viewport \<WIDTHxHEIGHT\>**: viewport as WIDTHxHEIGHT
- **\-\-match\-mode \<MODE\>**: node match mode \[default\: exact\]
- **\-\-node\-scope \<SCOPE\>**: node scope \[default\: current\]
- **\-\-matching\-debug**: include matching debug details
- **\-\-decisions\-file \<FILE\>**: read pairing decisions from JSONL
- **\-\-output\-decisions\-template \<FILE\>**: write a decisions template
- **\-\-output\-finding\-decisions\-template \<FILE\>**: write a finding decisions template
- **\-\-manifest \<FILE\>**: compare manifest json
- **\-\-continue\-on\-error**: continue after manifest page error
- **\-\-limit \<N\>**: limit manifest pages \[default\: 0\]
- **\-\-wait\-selector \<CSS\>**: wait selector before compare
- **\-\-scope\-selector \<CSS\>**: common CSS scope
- **\-\-old\-scope\-selector \<CSS\>**: old side CSS scope
- **\-\-new\-scope\-selector \<CSS\>**: new side CSS scope
- **\-\-wait\-function \<EXPRESSION\>**: wait javascript expression
- **\-\-wait\-network\-idle**: wait for network idle
- **\-\-compare\-css**: compare computed css values
- **\-\-all\-css\-properties**: compare every computed css property
- **\-\-compare\-layout**: compare element bounds
- **\-\-no\-default\-ignores**: disable default ignored nodes
- **\-\-wait\-timeout \<MS\>**: wait timeout in milliseconds \[default\: 10000\]
- **\-\-json**: print as json
- **\-\-output\-json \<FILE\>**: write compare report json
- **\-\-output\-md \<FILE\>**: write compare report markdown
- **\-\-review\-dir \<DIR\>**: write an AI review packet
- **\-\-css\-property \<NAME\>\.\.\.**: computed css property to compare
- **\-\-ignore\-text\-regex \<REGEX\>\.\.\.**: regex to strip from text
- **\-\-ignore\-selector \<RULE\>\.\.\.**: node selector to ignore
- **\-\-mask\-selector \<RULE\>\.\.\.**: node selector to mask
- **\-h\, \-\-help**: Print help

### Notes

Locator rules support \@eN and role\, name\, text\, testid\, href\, or combined role and name terms

\-\-matching\-debug includes anchors\, regions\, ambiguous candidates\, and unmatched nodes in JSON and Markdown reports

\-\-decisions\-file applies reviewed pair\, subtree\, and finding decisions before and after automatic matching

\-\-node\-scope all requires an explicit common scope or both old and new scopes

\-\-scope\-selector applies to both sides and old or new scope selectors override it per side

\-\-all\-css\-properties is exhaustive and can produce browser\-version noise\; use \-\-compare\-css for the stable default allowlist

### Links

- [Compare guide](<https://github.com/mayahiro/nexus/blob/main/docs/ai/compare.md>)
- [Migration playbook](<https://github.com/mayahiro/nexus/blob/main/docs/ai/playbooks/migration.md>)
- [AI usage guide](<https://github.com/mayahiro/nexus/blob/main/docs/ai/usage.md>)

## nxctl compare validate\-decisions

Validate compare decision records

### Usage

    nxctl compare validate-decisions --decisions-file <jsonl> [--compare-json <file>] [--review-summary <file>] [--old-session <id>] [--new-session <id>] [--strict] [--json]

### Options

- **\-\-decisions\-file \<FILE\>**: decisions JSONL file to validate
- **\-\-compare\-json \<FILE\>**: compare report JSON
- **\-\-review\-summary \<FILE\>**: review summary JSON
- **\-\-old\-session \<ID\>**: old browser session
- **\-\-new\-session \<ID\>**: new browser session
- **\-\-strict**: treat warnings as errors
- **\-\-json**: print as json
- **\-h\, \-\-help**: Print help

### Links

- [Compare guide](<https://github.com/mayahiro/nexus/blob/main/docs/ai/compare.md>)

## nxctl compare normalize\-decisions

Normalize compare decision records

### Usage

    nxctl compare normalize-decisions --decisions-file <jsonl> [--compare-json <file>] [--review-summary <file>] [--output <jsonl>] [--json]

### Options

- **\-\-decisions\-file \<FILE\>**: decisions JSONL file to normalize
- **\-\-compare\-json \<FILE\>**: compare report JSON
- **\-\-review\-summary \<FILE\>**: review summary JSON
- **\-\-output \<FILE\>**: output decisions JSONL
- **\-\-json**: print as json
- **\-h\, \-\-help**: Print help

### Links

- [Compare guide](<https://github.com/mayahiro/nexus/blob/main/docs/ai/compare.md>)

## nxctl compare materialize\-decisions

Materialize decision selectors as observed refs

### Usage

    nxctl compare materialize-decisions --decisions-file <jsonl> --compare-json <file> [--old-session <id>] [--new-session <id>] [--output <jsonl>] [--json]

### Options

- **\-\-decisions\-file \<FILE\>**: decisions JSONL file to materialize
- **\-\-compare\-json \<FILE\>**: compare report JSON
- **\-\-old\-session \<ID\>**: old browser session
- **\-\-new\-session \<ID\>**: new browser session
- **\-\-output \<FILE\>**: output decisions JSONL
- **\-\-json**: print as json
- **\-h\, \-\-help**: Print help

### Links

- [Compare guide](<https://github.com/mayahiro/nexus/blob/main/docs/ai/compare.md>)

## nxctl compare repair\-decisions

Repair stale refs in compare decision records

### Usage

    nxctl compare repair-decisions --decisions-file <jsonl> --compare-json <file> [--old-session <id>] [--new-session <id>] [--output <jsonl>] [--json]

### Options

- **\-\-decisions\-file \<FILE\>**: decisions JSONL file to repair
- **\-\-compare\-json \<FILE\>**: compare report JSON
- **\-\-old\-session \<ID\>**: old browser session
- **\-\-new\-session \<ID\>**: new browser session
- **\-\-output \<FILE\>**: output decisions JSONL
- **\-\-json**: print as json
- **\-h\, \-\-help**: Print help

### Links

- [Compare guide](<https://github.com/mayahiro/nexus/blob/main/docs/ai/compare.md>)

## nxctl compare audit\-decisions

Audit compare decisions against one report

### Usage

    nxctl compare audit-decisions --decisions-file <jsonl> --compare-json <file> [--json]

### Options

- **\-\-decisions\-file \<FILE\>**: decisions JSONL file to audit
- **\-\-compare\-json \<FILE\>**: compare report JSON
- **\-\-json**: print as json
- **\-h\, \-\-help**: Print help

### Links

- [Compare guide](<https://github.com/mayahiro/nexus/blob/main/docs/ai/compare.md>)

## nxctl close

Close one or all sessions

### Usage

    nxctl close [--session <id>]

    nxctl close --all

### Options

- **\-\-session \<VALUE\>**: session id \[default\: default\]
- **\-\-all**: close all sessions
- **\-h\, \-\-help**: Print help

### Constraints

- **target**: at most one of \-\-session\, \-\-all \[command line\]

## nxctl dblclick

Double\-click one observed node

### Usage

    nxctl dblclick <NODE> [--session <ID>] [--json]

### Arguments

- **\<NODE\>**: Observed node index or \@eN ref

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-h\, \-\-help**: Print help

## nxctl eval

Evaluate JavaScript in one session

### Usage

    nxctl eval <SOURCE> [--world main|persistent] [--session <ID>] [--json]

### Arguments

- **\<SOURCE\>**: JavaScript source

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-\-world \<WORLD\>**: JavaScript execution world \[default\: main\] \[possible\: main\, persistent\]
- **\-h\, \-\-help**: Print help

### Notes

Persistent world state survives eval calls until the page navigates\; store values on globalThis

## nxctl fill

Replace the value of one observed node

### Usage

    nxctl fill <NODE> <TEXT> [--session <ID>] [--json]

### Arguments

- **\<NODE\>**: Observed node index or \@eN ref
- **\<VALUE\>**: Text\, value\, or path

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-h\, \-\-help**: Print help

## nxctl find

Find observed nodes and optionally act on one

### Usage

    nxctl find role <QUERY> <click|input|fill|get> [VALUE] [--name <TEXT>] [--within <@eN>] [--nth <N>] [--session <ID>] [--json]

    nxctl find role <QUERY> --all [--name <TEXT>] [--within <@eN>] [--session <ID>] [--json]

    nxctl find text <QUERY> <click|input|fill|get> [VALUE] [--within <@eN>] [--nth <N>] [--session <ID>] [--json]

    nxctl find text <QUERY> --all [--within <@eN>] [--session <ID>] [--json]

    nxctl find label <QUERY> <click|input|fill|get> [VALUE] [--within <@eN>] [--nth <N>] [--session <ID>] [--json]

    nxctl find label <QUERY> --all [--within <@eN>] [--session <ID>] [--json]

    nxctl find testid <QUERY> <click|input|fill|get> [VALUE] [--within <@eN>] [--nth <N>] [--session <ID>] [--json]

    nxctl find testid <QUERY> --all [--within <@eN>] [--session <ID>] [--json]

    nxctl find href <QUERY> <click|input|fill|get> [VALUE] [--within <@eN>] [--nth <N>] [--session <ID>] [--json]

    nxctl find href <QUERY> --all [--within <@eN>] [--session <ID>] [--json]

    nxctl find aria-label <QUERY> <click|input|fill|get> [VALUE] [--within <@eN>] [--nth <N>] [--session <ID>] [--json]

    nxctl find aria-label <QUERY> --all [--within <@eN>] [--session <ID>] [--json]

    nxctl find css <QUERY> <click|input|fill|get> [VALUE] [--within <@eN>] [--nth <N>] [--session <ID>] [--json]

    nxctl find css <QUERY> --all [--within <@eN>] [--session <ID>] [--json]

### Commands

- **role**: Find by semantic role
- **text**: Find by visible text
- **label**: Find by form label
- **testid**: Find by test identifier
- **href**: Find by link target
- **aria\-label**: Find by aria\-label
- **css**: Find by CSS selector

### Options

- **\-h\, \-\-help**: Print help

### Notes

\-\-within requires a recent \@eN ref and evaluates the query inside that container

Refs become stale after navigation\, URL changes\, or stable\-identity changes at the referenced selector

### Links

- [AI usage guide](<https://github.com/mayahiro/nexus/blob/main/docs/ai/usage.md>)

## nxctl find role

Find by semantic role

### Usage

    nxctl find role <QUERY> <click|input|fill|get> [VALUE] [--name <TEXT>] [--within <@eN>] [--nth <N>] [--session <ID>] [--json]

    nxctl find role <QUERY> --all [--name <TEXT>] [--within <@eN>] [--session <ID>] [--json]

### Arguments

- **\<QUERY\>**: Locator query or CSS selector
- **\[ACTION\]**: Action to execute
- **\[ACTION\-VALUE\]**: Action value

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-\-all**: List all matching nodes
- **\-\-nth \<N\>**: Choose the nth matching node
- **\-\-within \<\@eN\>**: Limit the search to a previously observed node
- **\-\-name \<TEXT\>**: Accessible name
- **\-h\, \-\-help**: Print help

## nxctl find text

Find by visible text

### Usage

    nxctl find text <QUERY> <click|input|fill|get> [VALUE] [--within <@eN>] [--nth <N>] [--session <ID>] [--json]

    nxctl find text <QUERY> --all [--within <@eN>] [--session <ID>] [--json]

### Arguments

- **\<QUERY\>**: Locator query or CSS selector
- **\[ACTION\]**: Action to execute
- **\[ACTION\-VALUE\]**: Action value

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-\-all**: List all matching nodes
- **\-\-nth \<N\>**: Choose the nth matching node
- **\-\-within \<\@eN\>**: Limit the search to a previously observed node
- **\-h\, \-\-help**: Print help

## nxctl find label

Find by form label

### Usage

    nxctl find label <QUERY> <click|input|fill|get> [VALUE] [--within <@eN>] [--nth <N>] [--session <ID>] [--json]

    nxctl find label <QUERY> --all [--within <@eN>] [--session <ID>] [--json]

### Arguments

- **\<QUERY\>**: Locator query or CSS selector
- **\[ACTION\]**: Action to execute
- **\[ACTION\-VALUE\]**: Action value

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-\-all**: List all matching nodes
- **\-\-nth \<N\>**: Choose the nth matching node
- **\-\-within \<\@eN\>**: Limit the search to a previously observed node
- **\-h\, \-\-help**: Print help

## nxctl find testid

Find by test identifier

### Usage

    nxctl find testid <QUERY> <click|input|fill|get> [VALUE] [--within <@eN>] [--nth <N>] [--session <ID>] [--json]

    nxctl find testid <QUERY> --all [--within <@eN>] [--session <ID>] [--json]

### Arguments

- **\<QUERY\>**: Locator query or CSS selector
- **\[ACTION\]**: Action to execute
- **\[ACTION\-VALUE\]**: Action value

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-\-all**: List all matching nodes
- **\-\-nth \<N\>**: Choose the nth matching node
- **\-\-within \<\@eN\>**: Limit the search to a previously observed node
- **\-h\, \-\-help**: Print help

## nxctl find href

Find by link target

### Usage

    nxctl find href <QUERY> <click|input|fill|get> [VALUE] [--within <@eN>] [--nth <N>] [--session <ID>] [--json]

    nxctl find href <QUERY> --all [--within <@eN>] [--session <ID>] [--json]

### Arguments

- **\<QUERY\>**: Locator query or CSS selector
- **\[ACTION\]**: Action to execute
- **\[ACTION\-VALUE\]**: Action value

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-\-all**: List all matching nodes
- **\-\-nth \<N\>**: Choose the nth matching node
- **\-\-within \<\@eN\>**: Limit the search to a previously observed node
- **\-h\, \-\-help**: Print help

## nxctl find aria\-label

Find by aria\-label

### Usage

    nxctl find aria-label <QUERY> <click|input|fill|get> [VALUE] [--within <@eN>] [--nth <N>] [--session <ID>] [--json]

    nxctl find aria-label <QUERY> --all [--within <@eN>] [--session <ID>] [--json]

### Arguments

- **\<QUERY\>**: Locator query or CSS selector
- **\[ACTION\]**: Action to execute
- **\[ACTION\-VALUE\]**: Action value

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-\-all**: List all matching nodes
- **\-\-nth \<N\>**: Choose the nth matching node
- **\-\-within \<\@eN\>**: Limit the search to a previously observed node
- **\-h\, \-\-help**: Print help

## nxctl find css

Find by CSS selector

### Usage

    nxctl find css <QUERY> <click|input|fill|get> [VALUE] [--within <@eN>] [--nth <N>] [--session <ID>] [--json]

    nxctl find css <QUERY> --all [--within <@eN>] [--session <ID>] [--json]

### Arguments

- **\<QUERY\>**: Locator query or CSS selector
- **\[ACTION\]**: Action to execute
- **\[ACTION\-VALUE\]**: Action value

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-\-all**: List all matching nodes
- **\-\-nth \<N\>**: Choose the nth matching node
- **\-\-within \<\@eN\>**: Limit the search to a previously observed node
- **\-h\, \-\-help**: Print help

## nxctl flow

Run browser comparison workflows

### Usage

    nxctl flow run --manifest <FILE> [--scenario <NAME>] [--matrix <NAME>] [--continue-on-error] [--output-json <FILE>] [--json]

### Commands

- **run**: Run a flow manifest

### Options

- **\-h\, \-\-help**: Print help

### Links

- [Flow guide](<https://github.com/mayahiro/nexus/blob/main/docs/ai/flow.md>)

## nxctl flow run

Run a flow manifest

### Usage

    nxctl flow run --manifest <FILE> [--scenario <NAME>] [--matrix <NAME>] [--continue-on-error] [--output-json <FILE>] [--json]

### Options

- **\-\-manifest \<FILE\>**: Flow manifest JSON \[required\]
- **\-\-scenario \<NAME\>**: Scenario name
- **\-\-matrix \<NAME\>**: Matrix name
- **\-\-continue\-on\-error**: Continue after scenario failure
- **\-\-output\-json \<FILE\>**: Write flow report JSON
- **\-\-json**: Print JSON
- **\-h\, \-\-help**: Print help

## nxctl get

Read values from one browser session

### Usage

    nxctl get title [--session <ID>] [--json]

    nxctl get html [--selector <CSS>] [--session <ID>] [--json]

    nxctl get bbox --selector <CSS> [--session <ID>] [--json]

    nxctl get text|value|attributes|bbox <NODE> [--session <ID>] [--json]

    nxctl get text|value|attributes|bbox --refs <NODES> [--session <ID>] [--json]

### Arguments

- **\<TARGET\>**: title\, html\, text\, value\, attributes\, or bbox
- **\[NODE\]**: Observed node

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-\-selector \<CSS\>**: CSS selector for html or bbox
- **\-\-refs \<NODES\>**: Comma\-separated node refs
- **\-h\, \-\-help**: Print help

## nxctl hover

Hover one observed node

### Usage

    nxctl hover <NODE> [--session <ID>] [--json]

### Arguments

- **\<NODE\>**: Observed node index or \@eN ref

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-h\, \-\-help**: Print help

## nxctl inspect

Inspect one node in one session or compare it across two sessions

### Usage

    nxctl inspect <LOCATOR> --session <ID> [--nth <N>] [--scope-selector <CSS>] [--css-property <NAME>]... [--no-style-sources] [--layout-context] [--json]

    nxctl inspect --selector <CSS> --session <ID> [--css-property <NAME>]... [--no-style-sources] [--layout-context] [--json]

    nxctl inspect --scope-selector <CSS> --session <ID> [--css-property <NAME>]... [--no-style-sources] [--layout-context] [--json]

    nxctl inspect <LOCATOR> --old-session <ID> --new-session <ID> [--nth <N>] [--scope-selector <CSS>] [--old-scope-selector <CSS>] [--new-scope-selector <CSS>] [--css-property <NAME>]... [--no-style-sources] [--layout-context] [--json]

    nxctl inspect --selector <CSS> --old-session <ID> --new-session <ID> [--old-scope-selector <CSS>] [--new-scope-selector <CSS>] [--css-property <NAME>]... [--no-style-sources] [--layout-context] [--json]

    nxctl inspect --scope-selector <CSS> --old-session <ID> --new-session <ID> [--old-scope-selector <CSS>] [--new-scope-selector <CSS>] [--css-property <NAME>]... [--no-style-sources] [--layout-context] [--json]

    nxctl inspect --old-scope-selector <CSS> --new-scope-selector <CSS> --old-session <ID> --new-session <ID> [--css-property <NAME>]... [--no-style-sources] [--layout-context] [--json]

### Arguments

- **\[LOCATOR\]**: Node locator

### Options

- **\-\-session \<ID\>**: Session identifier for single\-session inspection
- **\-\-old\-session \<ID\>**: Old session identifier
- **\-\-new\-session \<ID\>**: New session identifier
- **\-\-selector \<CSS\>**: Raw CSS selector to inspect
- **\-\-scope\-selector \<CSS\>**: Common CSS scope
- **\-\-old\-scope\-selector \<CSS\>**: Old\-side CSS scope
- **\-\-new\-scope\-selector \<CSS\>**: New\-side CSS scope
- **\-\-css\-property \<NAME\>\.\.\.**: Computed CSS property
- **\-\-nth \<N\>**: Choose the nth matching node
- **\-\-no\-style\-sources**: Skip matched declaration and source location collection
- **\-\-layout\-context**: Include ancestor layout context
- **\-\-json**: Print JSON
- **\-h\, \-\-help**: Print help

### Notes

Locator forms include \@eN\, role\, text\, label\, testid\, and href

Style sources are collected by default and do not claim a cascade winner

A side\-specific scope is available only for old\/new comparison

### Links

- [Inspect guide](<https://github.com/mayahiro/nexus/blob/main/docs/ai/inspect.md>)

## nxctl input

Type text into one observed node

### Usage

    nxctl input <NODE> <TEXT> [--session <ID>] [--json]

### Arguments

- **\<NODE\>**: Observed node index or \@eN ref
- **\<VALUE\>**: Text\, value\, or path

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-h\, \-\-help**: Print help

## nxctl keys

Send a key sequence to one session

### Usage

    nxctl keys <KEYS> [--session <ID>] [--json]

### Arguments

- **\<KEYS\>**: Key specification

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-h\, \-\-help**: Print help

## nxctl navigate

Navigate an attached browser session

### Usage

    nxctl navigate <URL> [--session <ID>] [--json]

### Arguments

- **\<URL\>**: Destination URL

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-h\, \-\-help**: Print help

## nxctl open

Open a URL in a managed browser session

### Usage

    nxctl open <URL> [--session <ID>] [--backend chromium] [--viewport <WIDTHxHEIGHT>] [--target-ref <PATH>]

### Arguments

- **\<URL\>**: Initial URL

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-backend \<NAME\>**: Browser backend \[default\: chromium\] \[possible\: chromium\]
- **\-\-viewport \<WIDTHxHEIGHT\>**: Browser viewport
- **\-\-target\-ref \<PATH\>**: Browser executable or target reference
- **\-h\, \-\-help**: Print help

## nxctl observe

Observe one attached session

### Usage

    nxctl observe --session <ID> [--json] [--text] [--tree] [--screenshot] [--full] [--recover-target] [--verbose] [--timeout <MS>]

### Options

- **\-\-session \<ID\>**: Session identifier \[required\]
- **\-\-json**: Print JSON
- **\-\-text**: Include page text
- **\-\-tree**: Include the observed node tree
- **\-\-screenshot**: Include a screenshot
- **\-\-full**: Capture a full\-page screenshot
- **\-\-recover\-target**: Replace an unresponsive tab and retry\, losing transient page state
- **\-\-verbose**: Write every request and capture stage to the daemon output
- **\-\-timeout \<MS\>**: Overall screenshot recovery timeout in milliseconds \[default\: 30000\]
- **\-h\, \-\-help**: Print help

### Notes

Each capture attempt is capped at 10000 ms within the overall timeout

## nxctl rightclick

Right\-click one observed node

### Usage

    nxctl rightclick <NODE> [--session <ID>] [--json]

### Arguments

- **\<NODE\>**: Observed node index or \@eN ref

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-h\, \-\-help**: Print help

## nxctl scroll

Scroll a page or observed node

### Usage

    nxctl scroll up|down [--session <ID>] [--node <INDEX>] [--amount <PX>] [--json]

### Arguments

- **\<DIRECTION\>**: Scroll direction

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-\-node \<INDEX\>**: Observed node index \[default\: 0\]
- **\-\-amount \<PX\>**: Scroll amount in pixels \[default\: 0\]
- **\-h\, \-\-help**: Print help

## nxctl screenshot

Capture a page or element screenshot

### Usage

    nxctl screenshot [path] [--session <id>] [--full] [--annotate] [--recover-target] [--verbose] [--locator <locator>] [--nth <n>] [--timeout <ms>]

### Arguments

- **\[PATHS\]\.\.\.**

### Options

- **\-\-session \<VALUE\>**: session id \[default\: default\]
- **\-\-full**: capture full page
- **\-\-annotate**: draw node refs on the screenshot
- **\-\-recover\-target**: replace an unresponsive tab and retry\, losing transient page state
- **\-\-verbose**: write every request and capture stage to the daemon output
- **\-\-locator \<VALUE\>**: capture a single element
- **\-\-nth \<N\>**: select nth locator match
- **\-\-timeout \<MS\>**: overall screenshot recovery timeout in milliseconds \[default\: 30000\]
- **\-h\, \-\-help**: Print help

### Notes

Locator forms include \@eN\, role\, name\, text\, label\, testid\, and href

Viewport capture is the default\; \-\-full captures the full page within safety limits

Each capture attempt is capped at 10000 ms within the overall timeout

A failed capture automatically reattaches to the same target once\; \-\-recover\-target additionally permits tab replacement

Failures always flush buffered diagnostics\; \-\-verbose also writes successful stages

## nxctl select

Select a value on one observed node

### Usage

    nxctl select <NODE> <VALUE> [--session <ID>] [--json]

### Arguments

- **\<NODE\>**: Observed node index or \@eN ref
- **\<VALUE\>**: Text\, value\, or path

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-h\, \-\-help**: Print help

## nxctl sessions

List attached sessions

### Usage

    nxctl sessions [--json]

### Options

- **\-\-json**: Print JSON
- **\-h\, \-\-help**: Print help

## nxctl state

Show an AI\-readable page state

### Usage

    nxctl state [--session <ID>] [--role <ROLE>] [--name <TEXT>] [--text <TEXT>] [--testid <VALUE>] [--href <VALUE>] [--limit <N>] [--json]

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-role \<ROLE\>**: Filter by semantic role
- **\-\-name \<TEXT\>**: Filter by accessible name
- **\-\-text \<TEXT\>**: Filter by text
- **\-\-testid \<VALUE\>**: Filter by test identifier
- **\-\-href \<VALUE\>**: Filter by href
- **\-\-limit \<N\>**: Maximum nodes to print \[default\: 0\]
- **\-\-json**: Print JSON
- **\-h\, \-\-help**: Print help

## nxctl type

Type text into the active page

### Usage

    nxctl type <TEXT> [--session <ID>] [--json]

### Arguments

- **\<VALUE\>**: Text to type

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-h\, \-\-help**: Print help

## nxctl upload

Upload a file through a file input

### Usage

    nxctl upload <NODE> <PATH> [--session <ID>] [--json]

    nxctl upload --selector <CSS> <PATH> [--session <ID>] [--json]

### Arguments

- **\[VALUES\]\.\.\.**: Node and path\, or path with \-\-selector

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-\-selector \<CSS\>**: Select a file input directly\, including a hidden input
- **\-h\, \-\-help**: Print help

### Notes

\-\-selector must match exactly one input\[type\=file\]

## nxctl viewport

Set the browser viewport

### Usage

    nxctl viewport <WIDTHxHEIGHT> [--session <ID>] [--json]

### Arguments

- **\<VIEWPORT\>**: Viewport dimensions

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-h\, \-\-help**: Print help

## nxctl wait

Wait for a browser condition

### Usage

    nxctl wait selector <CSS> [--state attached|detached|visible|hidden] [--timeout <MS>] [--session <ID>] [--json]

    nxctl wait text <VALUE> [--timeout <MS>] [--session <ID>] [--json]

    nxctl wait url <VALUE> [--timeout <MS>] [--session <ID>] [--json]

    nxctl wait navigation [--timeout <MS>] [--session <ID>] [--json]

    nxctl wait hydrated [--timeout <MS>] [--session <ID>] [--json]

    nxctl wait function <EXPRESSION> [--timeout <MS>] [--session <ID>] [--json]

### Arguments

- **\<TARGET\>**: Wait target
- **\[VALUE\]**: Wait value

### Options

- **\-\-session \<ID\>**: Session identifier \[default\: default\]
- **\-\-json**: Print JSON
- **\-\-state \<STATE\>**: Selector state \[default\: visible\] \[possible\: attached\, detached\, visible\, hidden\]
- **\-\-timeout \<MS\>**: Wait timeout in milliseconds \[default\: 30000\]
- **\-h\, \-\-help**: Print help

### Notes

hydrated waits for DOMContentLoaded\, animation frames\, and a DOM mutation quiet window\; it is not a React\-internal signal

### Links

- [Compare guide](<https://github.com/mayahiro/nexus/blob/main/docs/ai/compare.md>)

## nxctl detach

Detach one session without stopping the daemon

### Usage

    nxctl detach --session <ID>

### Options

- **\-\-session \<ID\>**: Session identifier \[required\]
- **\-h\, \-\-help**: Print help

## nxctl daemon

Run the Nexus daemon

### Usage

    nxctl daemon [--verbose]

### Options

- **\-\-verbose**: Write every stage for every daemon request
- **\-h\, \-\-help**: Print help

### Notes

Auto\-started daemon processes write to a PID\-specific nxd\.\<pid\>\.log

Failures flush buffered stages and environment details even without \-\-verbose

## nxctl doctor

Check configuration\, daemon\, and protocol status

### Usage

    nxctl doctor [OPTIONS]

### Options

- **\-h\, \-\-help**: Print help

### Notes

Starts nxd temporarily when needed and stops it after the check
