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

`state` prints AI-friendly element refs such as `@e1`, and those refs can be reused in node-targeting commands like `click`, `fill`, `input`, `select`, `upload`, `hover`, and `get`.
`state` also prints short locator hints derived from the current tree, so an agent can switch from `@eN` refs to `find role|text|label|testid|href` without recomputing selectors.
Use `fill` when you want to replace the current value, and `type` when you want keystroke-style input against the current focus or target.

When you want a meaning-based locator instead of a ref, use `find`:

- `find role button click --name "Submit"`
- `find role link get attributes --name "Docs"`
- `find role button --all`
- `find text "Sign in" click`
- `find label "Email" fill "hello@example.com"`
- `find label "Email" input "hello@example.com"`

If multiple nodes match, `find` fails with candidate refs so you can narrow the query or switch to `@eN` from `state`.

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
nxctl find role button click --name "Submit"
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
nxctl compare https://old.example.com/orders https://new.example.com/orders --compare-css
nxctl compare https://old.example.com/orders https://new.example.com/orders --css-property color --css-property pointer-events
nxctl compare https://old.example.com/orders https://new.example.com/orders --ignore-selector role=link&text=Legacy --mask-selector role=textbox&name=Email
nxctl compare https://old.example.com/orders https://new.example.com/orders --output-json compare.json --output-md compare.md
nxctl compare --manifest migration-pages.json --output-md compare.md
nxctl flow run --manifest login-flow.json --json
nxctl inspect 'role button --name "Submit"' --old-session old --new-session new
nxctl inspect 'text "Sign In"' --old-session old --new-session new --css-property color
nxctl get attributes @e3
nxctl screenshot
nxctl screenshot annotated.png --annotate
nxctl viewport 1280x720
nxctl close
```

In compare manifests, `backend`, `viewport`, `compare_css`, and `css_property` can be set in `defaults` and overridden per page.

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

Most command flags can be placed before or after positional arguments.
Examples: `nxctl open --session work https://example.com`, `nxctl navigate --session work https://example.com/docs`, `nxctl click @e3 --json`

`compare` waits for document load completion on URL-based runs.
Use `--wait-function`, `--wait-network-idle`, or `--wait-selector` when the page keeps updating after load and you need stronger readiness.
Use `--compare-css` to compare a default computed-style allowlist on matching fingerprints.
Use `--css-property` one or more times when you want explicit computed-style properties instead of the default list.
Node-level compare findings include a best-effort `locator` when Nexus can infer a reusable selector from shared attributes such as `label`, `testid`, or `href`.
Color-valued computed styles are normalized to sRGB `rgb(...)` or `rgba(...)` before comparison to reduce notation-only diffs from values such as `lab(...)` or `oklab(...)`.
Use `inspect` when you already have two sessions and want computed-style values for one semantic locator instead of a whole-page diff.

`flow run` currently supports `wait`, `navigate`, `click`, `fill`, `viewport`, and `compare` steps.
Scenarios can define `old` and `new` endpoints, optional `matrix` names, and string variables for simple `{{ name }}` substitution.
Existing sessions can be reused through `old.session` and `new.session`, and scenario-start viewport overrides are applied even when a session already exists.

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
