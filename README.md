# Nexus

Nexus is a browser automation gateway for AI-driven workflows on macOS.

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

The primary interaction loop is:

```text
open -> state/find -> click/type/input/keys -> wait/get/state
```

`state` prints AI-friendly element refs such as `@e1`, and those refs can be reused in node-targeting commands like `click`, `input`, `select`, `upload`, `hover`, and `get`.
`state` also prints short locator hints derived from the current tree, so an agent can switch from `@eN` refs to `find role|text|label|testid|href` without recomputing selectors.

When you want a meaning-based locator instead of a ref, use `find`:

- `find role button click --name "Submit"`
- `find role link get attributes --name "Docs"`
- `find role button --all`
- `find text "Sign in" click`
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
nxctl state
nxctl state --role button --limit 20
nxctl click @e3
nxctl find role button click --name "Submit"
nxctl find role link get attributes --name "Docs"
nxctl find label "Email" input "hello@example.com"
nxctl click 120 240
nxctl input @e4 "hello@example.com"
nxctl batch --cmd "state" --cmd "find role button --all"
nxctl keys "Enter"
nxctl wait selector ".ready"
nxctl wait url "/done"
nxctl wait navigation
nxctl wait function "window.appReady === true"
nxctl compare https://old.example.com/orders https://new.example.com/orders --wait-selector ".ready"
nxctl compare https://old.example.com/orders https://new.example.com/orders --ignore-selector @e3 --mask-selector testid=user-id
nxctl get attributes @e3
nxctl screenshot
nxctl screenshot annotated.png --annotate
nxctl viewport 1280x720
nxctl close
```

Available command groups include:

- browser management: `browser setup`, `browser update`, `browser status`, `browser uninstall`
- navigation: `open`, `back`, `scroll`
- inspection: `state`, `observe`, `get`, `screenshot`
- interaction: `click`, `hover`, `dblclick`, `rightclick`, `type`, `input`, `keys`, `select`, `upload`, `eval`, `find`
- migration diff: `compare`
- automation flow: `batch`
- session control: `sessions`, `detach`, `close`

Run `nxctl help <command>` for command-specific usage.

Most command flags can be placed before or after positional arguments.
Examples: `nxctl open --session work https://example.com`, `nxctl click @e3 --json`

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

- AI usage guide: [`docs/ai-usage.md`](docs/ai-usage.md)
