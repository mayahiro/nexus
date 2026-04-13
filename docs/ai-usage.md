# AI Usage Guide for Nexus

This document is a short operational guide for AI agents that use Nexus as a tool.

## 1. Preconditions

- `nxctl` and `nxd` are installed
- `nxctl browser setup` has been run if you want to use managed browsers
- `chromium` is the primary backend
- `lightpanda` is experimental and has a narrower support surface

## 2. Recommended Starting Sequence

```text
nxctl doctor
nxctl browser setup
nxctl open https://example.com
nxctl state
```

The default execution loop is:

```text
open -> state/find -> click/type/input/keys -> wait -> get/state -> close
```

`state` emits element refs such as `@e1`. Reuse those refs in node-targeting commands instead of relying on raw indexes when possible.
When a stable semantic locator is clearer than a ref, prefer `find role`, `find text`, `find label`, `find testid`, or `find href`.
`state` also includes short locator hints for each node so you can promote a recent `@eN` observation into a semantic locator without re-deriving it yourself.
If `find` reports multiple matches, narrow the query or fall back to `@eN` from the latest `state`.

## 3. Session Model

- the default session is `default`
- use `--session <id>` to target another session
- `close` closes the default session
- `close --all` closes every session
- `detach --session <id>` detaches a specific session

Use `sessions --json` when you need to inspect current state across multiple sessions.

## 4. Backend Guidance

### chromium

- primary backend
- supports `observe`, `act`, `screenshot`, and `logs`
- use this by default

### lightpanda

- experimental backend
- supports `open`, `observe`, and `state`
- treat it as observation-first
- do not assume the same success rate or feature coverage as `chromium`

## 5. Common Commands

```text
nxctl open https://example.com
nxctl state
nxctl state --role button --limit 20
nxctl click @e3
nxctl find role button click --name "Submit"
nxctl find role button --all
nxctl find role link get attributes --name "Docs"
nxctl find label "Email" input "hello@example.com"
nxctl input @e4 "hello@example.com"
nxctl batch --cmd "state" --cmd "find role button --all"
nxctl keys "Enter"
nxctl wait selector ".ready"
nxctl wait url "/done"
nxctl wait navigation
nxctl wait function "window.appReady === true"
nxctl compare https://old.example.com/orders https://new.example.com/orders --wait-selector ".ready"
nxctl compare https://old.example.com/orders https://new.example.com/orders --ignore-selector role=link&text=Legacy --mask-selector role=textbox&name=Email
nxctl compare https://old.example.com/orders https://new.example.com/orders --output-json compare.json --output-md compare.md
nxctl compare --manifest migration-pages.json --continue-on-error --output-json compare.json
nxctl get attributes @e3
nxctl screenshot
nxctl screenshot annotated.png --annotate
nxctl viewport 1280x720
nxctl close
```

When you need to compare migrated screens, use `compare` with two URLs or two existing sessions.
Start with `--wait-selector` on a stable ready marker and add `--ignore-text-regex` for dynamic timestamps or IDs that should not count as meaningful differences.
Use `--ignore-selector` to drop nodes from comparison and `--mask-selector` to keep the node while suppressing text and value differences.
Rules support `@eN`, `field=value`, and simple AND conditions such as `role=textbox&name=Email`.
For multi-page audits, put URL or session pairs into a manifest JSON file and run `compare --manifest`.

When `state` gets too large, filter the tree before reading it.
Start with `--role`, `--name`, `--text`, `--testid`, `--href`, and `--limit`.

Most command flags can be placed before or after positional arguments.

## 6. Viewport

- browser sessions default to `1920x1080`
- `open` and `attach browser` accept `--viewport <width>x<height>`
- use `viewport <width>x<height>` to change it later

Examples:

```text
nxctl open https://example.com --viewport 1440x900
nxctl viewport 1280x720
```

## 7. Common Failure Modes

- `open` or `attach browser` fails before `browser setup` has installed managed browsers
- `lightpanda` may not support the same action commands as `chromium`
- dynamic DOM updates can change `state` indexes
- prefer `@eN` refs from the latest `state` output over raw numeric indexes
- action commands are often more reliable when followed by `wait`

## 8. Recommended Inspection Order

When behavior is not what you expect, inspect in this order:

1. `nxctl state`
2. `nxctl wait ...`
3. `nxctl get ...`
4. `nxctl screenshot`
5. `nxctl sessions --json`

## 9. References

- public overview: [`README.md`](../README.md)
