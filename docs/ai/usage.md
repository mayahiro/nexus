# Nexus AI Guide

This is the main entry point for AI agents that use Nexus.

## Start Here

- Read this guide first
- Use the specialized guides when the task becomes compare-heavy or flow-heavy
- Prefer command help plus these guides over guessing behavior

## Quick Links

- Compare guide: [docs/ai/compare.md](compare.md)
- Flow guide: [docs/ai/flow.md](flow.md)
- Playbooks: [docs/ai/playbooks/README.md](playbooks/README.md)
- Migration playbook: [docs/ai/playbooks/migration.md](playbooks/migration.md)
- Public overview: [README.md](../../README.md)

## Decision Shortcuts

- Use `compare` for independent page-to-page checks
- Use `flow run` when login or session continuity matters
- Use `navigate -> wait -> compare` when the landing page is not the page you want to inspect
- Start with text/content checks before adding CSS checks
- Prefer page-specific `--wait-selector` or `--wait-function` over relying on load completion alone

## Recommended Loop

```text
open/navigate -> state/find -> click/type/fill/input/keys -> wait -> compare/inspect -> close
```

## Quick Start

```text
nxctl --help
nxctl doctor
nxctl browser setup
nxctl open https://example.com
nxctl state
nxctl help compare
nxctl help flow
```

## Core Rules

- Treat `nxctl --help` and `nxctl help <command> [subcommand]` as the canonical command schema
- Read the `error[code]: ...` diagnostic and its command-specific usage line before retrying invalid input
- Reuse `@eN` refs from the latest `state` output when they are still fresh
- Prefer semantic locators such as `role`, `label`, `text`, `testid`, `href`, or `aria-label` when they are stable
- Use `find css` when the page does not expose a stable semantic locator
- Use `find ... --within @eN` to keep a search inside a recently observed container
- Use `fill` when you want replacement semantics
- Use `type` when you want keystroke-style input
- Use `screenshot --locator` when you need a PNG for one specific element instead of the whole viewport
- Use `get bbox --selector <css>` when you need viewport-relative bounds for an arbitrary CSS-selected element
- Use `get text|value|attributes|bbox --refs <@eN,@eN,...> --json` when you need values for several recent refs
- Use `click --refs <@eN,@eN,...>` only when sequential clicks are intentional, because page changes can stale later refs
- Use `batch --keep-going` only when later diagnostic steps remain useful after an earlier command fails
- Add `wait` after actions that trigger async UI updates
- Move to `inspect` when whole-page compare is too broad

Nexus serializes operations within one session. A queued operation still honors its own context deadline, so parallel callers fail by timeout instead of starting concurrent CDP work on the same tab.

## Choosing `fill`, `input`, or `type`

| Command | Target | Input behavior | Recommended use |
| --- | --- | --- | --- |
| `fill <NODE> <TEXT>` | Explicit observed node | Uses the native input or textarea value setter, then dispatches composed `input` and `change` events | Prefer when an existing value must be replaced, including React-controlled text fields |
| `input <NODE> <TEXT>` | Explicit observed node | Focuses the node and types at the current caret with browser key events | Prefer for empty fields or widgets that depend on keyboard events |
| `type <TEXT>` | Currently focused editable element | Types at the current caret with browser key events without selecting a node | Use only when a preceding action established focus unambiguously |

For controlled forms:

- Prefer `input` when the field is empty and the application needs real keyboard events
- Prefer `fill` when replacing an existing value, then verify with `get value <NODE>` or `state`
- `input`, `type`, and `keys` verify that a page-level `keydown` listener received an event when the probe can stay attached; a verified zero-delivery result is an error instead of a false success
- If a custom controlled component still restores the old value after `fill`, use a keyboard-oriented sequence that focuses and clears the field before `input`
- Avoid `type` when focus may have moved after a modal, navigation, or rerender

## Finding and Scoping Nodes

Examples:

- `nxctl find aria-label "Points explanation" click`
- `nxctl find css 'button[data-action="save"]' click`
- `nxctl find role button --all --within @e12`
- `nxctl find css ':scope > button' --all --within @e12`

`--within` accepts only a recent `@eN` ref. Nexus resolves that ref before searching and evaluates CSS selectors relative to the container.

Refs are generation checked. A ref is rejected when the main document loader or URL changed, when an explicit navigation cleared the generation, or when the element at its structural selector no longer has the same stable identity. Run `state` or `find` again after navigation or a major rerender.

## Readiness and Eval Worlds

Use `wait hydrated` when a page needs a generic post-load stabilization barrier before interaction. It waits for `DOMContentLoaded`, two animation frames, 100 ms without DOM mutations, and two more animation frames. It is a browser rendering heuristic and cannot prove that a framework attached every event handler.

Use `wait function <EXPRESSION>` when the application exposes a stronger readiness condition.

`eval` uses the main world by default. `eval --world persistent` uses a named isolated world whose `globalThis` values survive later persistent eval calls on the same document:

```text
nxctl eval 'globalThis.probe = {count: 1}' --world persistent
nxctl eval 'globalThis.probe.count += 1' --world persistent
```

The persistent world is cleared on navigation, CDP target reattachment, or target replacement. It is isolated from page JavaScript, so use the main world when code must access application-owned globals directly.

## Targeted Screenshot

Use `screenshot --locator` when the task needs a PNG artifact for one control or content block.

Examples:

- `nxctl screenshot email.png --locator label=Email`
- `nxctl screenshot submit.png --locator @e1`
- `nxctl screenshot cta.png --locator role=button&name=Submit`
- `nxctl screenshot second-button.png --locator role=button --nth 2`

Rules:

- `--locator` supports `@eN`, `role=...`, `name=...`, `text=...`, `label=...`, `testid=...`, `href=...`, and combined forms such as `role=button&name=Submit`
- use `--nth` only when multiple nodes intentionally share the same locator
- `--full` is not supported together with `--locator`
- viewport capture is the default; `--full` opts into a full-page capture
- screenshot capture times out after 30000 ms by default; `screenshot --timeout <MS>` and `observe --screenshot --timeout <MS>` apply the overall deadline on both the client and daemon
- each `Page.captureScreenshot` attempt is capped at 10000 ms; a larger overall timeout budgets recovery work but does not extend an individual attempt
- after a capture failure, Nexus creates one fresh CDP connection to the same target and retries without losing page state; repeated failures do not accumulate more connections
- add `--recover-target` to permit a final tab replacement and URL reload when same-target recovery fails; Nexus prints a warning because transient page state is lost
- `--recover-target` is not supported together with `--locator`
- full-page captures are rejected above 16384 px width, 50000 px height, or 120 million pixels
- an open JavaScript alert, confirm, prompt, or beforeunload dialog is reported explicitly instead of being treated as a generic capture timeout
- refresh the locator from a recent `state` output if the page changed after navigation or interaction
- in flow manifests, use `{"action":"screenshot","path":"...","locator":"..."}` for the same targeted capture behavior

Mouse position is kept on the session target, so a viewport screenshot taken after `hover @eN` preserves the hover state unless another action moves the pointer or the page rerenders the target.

Canvas content, including map renderers, is captured visually but usually has no semantic DOM nodes for `find`, `state`, or coordinate assertions. Verify canvas internals through screenshot review or an application-specific JavaScript/API assertion.

## File Uploads

Use `upload <@eN> <PATH>` when the file input is present in the latest observed tree. Hidden file inputs do not receive refs, so target them directly:

```text
nxctl upload --selector 'input[type="file"]' ./artifact.png
```

The selector must resolve to exactly one `input[type=file]`. Nexus resolves the local path to an absolute path and passes it through CDP without requiring the input to be visible.

## Which Guide To Open Next

- Open [docs/ai/compare.md](compare.md) when the task is about compare timing, noise, or compare scope
- Open [docs/ai/flow.md](flow.md) when the task requires login, session reuse, or multi-step navigation
- Open [docs/ai/playbooks/migration.md](playbooks/migration.md) when the task is a legacy-to-new-system migration audit
