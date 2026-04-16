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
- Add `wait` after actions that trigger async UI updates
- Move to `inspect` when whole-page compare is too broad

## Which Guide To Open Next

- Open [docs/ai/compare.md](compare.md) when the task is about compare timing, noise, or compare scope
- Open [docs/ai/flow.md](flow.md) when the task requires login, session reuse, or multi-step navigation
- Open [docs/ai/playbooks/migration.md](playbooks/migration.md) when the task is a legacy-to-new-system migration audit
