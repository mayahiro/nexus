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

- Reuse `@eN` refs from the latest `state` output when they are still fresh
- Prefer semantic locators such as `role`, `label`, `text`, `testid`, or `href` when they are stable
- Use `fill` when you want replacement semantics
- Use `type` when you want keystroke-style input
- Use `screenshot --locator` when you need a PNG for one specific element instead of the whole viewport
- Use `get bbox --selector <css>` when you need viewport-relative bounds for an arbitrary CSS-selected element
- Use `get text|value|attributes|bbox --refs <@eN,@eN,...> --json` when you need values for several recent refs
- Use `click --refs <@eN,@eN,...>` only when sequential clicks are intentional, because page changes can stale later refs
- Add `wait` after actions that trigger async UI updates
- Move to `inspect` when whole-page compare is too broad

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
- refresh the locator from a recent `state` output if the page changed after navigation or interaction
- in flow manifests, use `{"action":"screenshot","path":"...","locator":"..."}` for the same targeted capture behavior

## Which Guide To Open Next

- Open [docs/ai/compare.md](compare.md) when the task is about compare timing, noise, or compare scope
- Open [docs/ai/flow.md](flow.md) when the task requires login, session reuse, or multi-step navigation
- Open [docs/ai/playbooks/migration.md](playbooks/migration.md) when the task is a legacy-to-new-system migration audit
