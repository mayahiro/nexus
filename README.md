# Nexus: An A-eye for Agents

Nexus is an A-eye for agents, designed to observe, navigate, and compare browser state in practical local workflows on macOS.

It is inspired by Browser Use CLI, but it is not a compatibility project. Nexus is a native Go implementation with a session-oriented daemon, managed browser installs, and a CLI designed for practical local automation without a Python runtime dependency.

## Status

Nexus is currently in an early usable stage.

- macOS only
- Chromium is the primary backend
- Lightpanda is supported as an experimental backend for observation-oriented workflows
- The current install path is `go install`

## Install

Prerequisites:

- macOS
- Go 1.26.1
- `$(go env GOPATH)/bin` or `~/go/bin` on `PATH`

Install:

```text
git clone https://github.com/mayahiro/nexus.git
cd nexus
go install ./cmd/nxctl ./cmd/nxd
```

## Quick Start

```text
nxctl doctor
nxctl browser setup
nxctl open https://example.com
nxctl navigate https://example.com/docs
nxctl state
nxctl close
```

`nxctl doctor` starts `nxd` temporarily if needed, validates local daemon connectivity, and stops it again after the check.

## Core Concepts

Nexus is built around sessions.

- `nxctl` is the user-facing CLI
- `nxd` is the local daemon
- each browser attachment becomes a named session
- commands operate against the default session unless `--session` is provided
- `close` stops `nxd` when it closes the last remaining session
- `close --all` closes every session and stops `nxd`

The primary interaction loop is:

```text
open/navigate -> state/find -> click/type/fill/input/keys -> wait/get/state
```

`state` prints AI-friendly element refs such as `@e1`, and those refs can be reused in node-targeting commands like `click`, `fill`, `input`, `select`, `upload`, `hover`, `get`, and `screenshot`.
`state` also prints short locator hints derived from the current tree, so an agent can switch from `@eN` refs to `find role|text|label|testid|href` without recomputing selectors.
Use `fill` when you want to replace the current value, and `type` when you want keystroke-style input against the current focus or target.

When you want a meaning-based locator instead of a ref, use `find`:

- `find role button click --name "Submit"`
- `find role button click --nth 2`
- `find role link get attributes --name "Docs"`
- `find role button --all`
- `find text "Sign in" click`
- `find label "Email" fill "hello@example.com"`
- `find label "Email" input "hello@example.com"`

If multiple nodes match, `find` fails with candidate refs so you can narrow the query, select one with `--nth`, or switch to `@eN` from `state`.

## Browser Management

Managed browsers are installed by Nexus itself.

```text
nxctl browser setup
nxctl browser update
nxctl browser status
nxctl browser uninstall
```

Current behavior:

- `browser setup` installs stable Chromium and stable Lightpanda
- `browser update` refreshes both
- `browser uninstall` removes managed browser installs
- download archives are not kept after successful install or update

## Main Commands

Examples:

```text
nxctl open https://example.com
nxctl open https://example.com --viewport 1440x900
nxctl navigate https://example.com/docs
nxctl state
nxctl state --role button --limit 20
nxctl click @e3
nxctl click --refs @e1,@e2,@e3
nxctl find role button click --name "Submit"
nxctl find role button click --nth 2
nxctl find role link get attributes --name "Docs"
nxctl find label "Email" fill "hello@example.com"
nxctl find label "Email" input "hello@example.com"
nxctl click 120 240
nxctl fill @e4 "hello@example.com"
nxctl input @e4 "hello@example.com"
nxctl batch --cmd "state" --cmd "find role button --all"
nxctl keys "Enter"
nxctl wait selector ".ready"
nxctl wait url "/done"
nxctl wait navigation
nxctl wait function "window.appReady === true"
nxctl compare https://old.example.com/orders https://new.example.com/orders --wait-function "window.appReady === true"
nxctl compare https://old.example.com/orders https://new.example.com/orders --wait-network-idle
nxctl compare https://old.example.com/orders https://new.example.com/orders --wait-selector ".ready"
nxctl compare https://old.example.com/orders https://new.example.com/orders --scope-selector "aside.filters"
nxctl compare https://old.example.com/orders https://new.example.com/orders --old-scope-selector "#legacy-filters" --new-scope-selector "aside.filters"
nxctl compare https://old.example.com/orders https://new.example.com/orders --match-mode stable
nxctl compare https://old.example.com/orders https://new.example.com/orders --compare-css
nxctl compare https://old.example.com/orders https://new.example.com/orders --css-property color --css-property pointer-events
nxctl compare https://old.example.com/orders https://new.example.com/orders --compare-layout
nxctl compare https://old.example.com/orders https://new.example.com/orders --ignore-selector role=link&text=Legacy --mask-selector role=textbox&name=Email
nxctl compare https://old.example.com/orders https://new.example.com/orders --output-json compare.json --output-md compare.md
nxctl compare --manifest migration-pages.json --output-md compare.md
nxctl flow run --manifest login-flow.json --json
nxctl inspect 'role button --name "Submit"' --old-session old --new-session new
nxctl inspect 'role button' --old-session old --new-session new --nth 2 --css-property color
nxctl inspect 'text "Sign In"' --old-session old --new-session new --css-property color
nxctl inspect --selector "aside.filters" --old-session old --new-session new --css-property width
nxctl inspect --old-scope-selector "#legacy-filters" --new-scope-selector "aside.filters" --old-session old --new-session new --css-property width
nxctl inspect 'role button --name "Submit"' --old-session old --new-session new --layout-context
nxctl get attributes @e3
nxctl get attributes --refs @e1,@e2,@e3 --json
nxctl get bbox --selector ".hero"
nxctl get bbox --refs @e1,@e2,@e3 --json
nxctl screenshot
nxctl screenshot annotated.png --annotate
nxctl screenshot email.png --locator label=Email
nxctl screenshot submit.png --locator @e1
nxctl viewport 1280x720
nxctl close
```

In compare manifests, `backend`, `viewport`, `match_mode`, `scope_selector`, `old_scope_selector`, `new_scope_selector`, `compare_css`, `compare_layout`, and `css_property` can be set in `defaults` and overridden per page.

`flow run` executes a scenario manifest while keeping old/new sessions alive across ordered steps. Use it for login flows, multi-step journeys, and responsive checks that should repeat the same flow across matrices such as desktop and mobile.

Available command groups include:

- browser management: `browser setup`, `browser update`, `browser status`, `browser uninstall`
- navigation: `open`, `navigate`, `back`, `scroll`
- inspection: `state`, `observe`, `get`, `screenshot`
- targeted style diff: `inspect`
- interaction: `click`, `hover`, `dblclick`, `rightclick`, `type`, `fill`, `input`, `keys`, `select`, `upload`, `eval`, `find`
- migration diff: `compare`
- scenario flow: `flow run`
- automation flow: `batch`
- session control: `sessions`, `detach`, `close`

Run `nxctl help <command>` for command-specific usage.
Run `nxctl --help` or `nxctl -h` for the top-level command list and documentation links.

Most command flags can be placed before or after positional arguments.
Examples: `nxctl open --session work https://example.com`, `nxctl navigate --session work https://example.com/docs`, `nxctl click @e3 --json`

`compare` waits for document load completion on URL-based runs.
Use `--wait-function`, `--wait-network-idle`, or `--wait-selector` when the page keeps updating after load and you need stronger readiness.
Use `--scope-selector` when you want to restrict compare to one CSS-selected subtree such as `aside.filters` or `main section.hero`.
`--scope-selector` accepts a raw CSS selector, requires exactly one match on each side, and may use positional selectors such as `:nth-child()` or `:nth-of-type()`, though stable ids, classes, or attributes are preferred.
When a scope selector matches multiple elements, Nexus fails with short hints for up to five matched candidates so you can refine the selector without dumping the full HTML.
Use `--old-scope-selector` and `--new-scope-selector` when old and new pages need different subtree selectors.
If one side-specific scope selector is set without the other, `--scope-selector` must provide the missing side's fallback.
Use `--match-mode exact|stable|heuristic` to control node pairing. `exact` is the default and preserves fingerprint matching, `stable` uses unique identity keys such as `data-testid`, `id`, `href`, and labels before falling back to fingerprints, and `heuristic` adds conservative score-based matching for migration diffs.
Use `--compare-css` to compare a default computed-style allowlist on matching nodes.
Use `--css-property` one or more times when you want explicit computed-style properties instead of the default list.
Use `--compare-layout` when you want opt-in viewport-relative bounds findings for matching nodes, such as a button moving from center to left. Layout findings report observed placement changes; use `inspect --layout-context` when you need ancestor CSS context to investigate why the movement happened.
Node-level compare findings include a best-effort `locator` when Nexus can infer a reusable selector from shared attributes such as `label`, `testid`, or `href`.
Color-valued computed styles are normalized to sRGB `rgb(...)` or `rgba(...)` before comparison to reduce notation-only diffs from values such as `lab(...)` or `oklab(...)`.
Use `inspect` when you already have two sessions and want computed-style values for one semantic locator instead of a whole-page diff.
Use `inspect --selector` when you need the computed styles for one CSS-selected container rather than a semantic locator.
`inspect --selector` accepts a raw CSS selector, requires exactly one match on each side, allows positional selectors such as `:nth-child()` and `:nth-of-type()`, and does not support `--nth`.
When an inspect selector or inspect scope selector matches multiple elements, Nexus reports up to five matched candidates as hints.
Use `inspect --scope-selector` to resolve a semantic locator inside one CSS-selected subtree, or `--old-scope-selector` and `--new-scope-selector` when the old and new subtree selectors differ.
When `inspect` has no semantic locator, side-specific scope selectors identify the inspected roots, matching `inspect --selector` behavior for different DOM structures.
Use `inspect --layout-context` when the target element is affected by ancestor layout. Chromium returns DOM ancestor context with a focused layout CSS allowlist; unsupported backends fail with a capability error.
Use `--nth` with `find` or `inspect` when repeated controls intentionally share the same semantic locator.
Use `get bbox --selector <css>` when you need the viewport-relative bounds for any CSS-selected element without running ad hoc JavaScript.
Use `get text|value|attributes|bbox --refs <@eN,@eN,...>` when you need read-only values for several recent refs in one command.
Use `click --refs <@eN,@eN,...>` only when sequential clicks are intentional; Nexus stops at the first failed click.

`flow run` currently supports `wait`, `navigate`, `click`, `fill`, `viewport`, `screenshot`, and `compare` steps.
Scenarios can define `old` and `new` endpoints, optional `matrix` names, and string variables for simple `{{ name }}` substitution.
Existing sessions can be reused through `old.session` and `new.session`, and scenario-start viewport overrides are applied even when a session already exists.
Screenshot steps write PNG files to the provided `path`. When `side` is omitted and both sessions are captured, Nexus writes `-old` and `-new` suffixed files automatically.
Screenshot steps can also target one element with `locator` and optional `nth`, using the same selector DSL as flow `click` and `fill`.

## Viewport

Browser sessions default to a `1920x1080` viewport.

You can override it at attach time:

```text
nxctl open https://example.com --viewport 1440x900
nxctl attach browser --session work --backend chromium --viewport 1440x900
```

You can also change it later:

```text
nxctl viewport 1280x720
nxctl viewport 1280x720 --session work
```

## Runtime Paths

Nexus follows the XDG Base Directory convention.

```text
config:  $XDG_CONFIG_HOME/nexus/config.yaml
state:   $XDG_STATE_HOME/nexus/
runtime: $XDG_RUNTIME_DIR/nexus/nxd.sock
data:    $XDG_DATA_HOME/nexus/
cache:   $XDG_CACHE_HOME/nexus/
```

Fallbacks:

```text
~/.config/nexus/config.yaml
~/.local/state/nexus/
~/.local/share/nexus/
~/.cache/nexus/
```

## Documentation

- AI guide: [`docs/ai/usage.md`](docs/ai/usage.md)
- AI compare guide: [`docs/ai/compare.md`](docs/ai/compare.md)
- AI flow guide: [`docs/ai/flow.md`](docs/ai/flow.md)
- AI playbooks: [`docs/ai/playbooks/README.md`](docs/ai/playbooks/README.md)
- AI migration playbook: [`docs/ai/playbooks/migration.md`](docs/ai/playbooks/migration.md)
