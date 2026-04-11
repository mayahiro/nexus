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
open -> state -> click/type/input/keys -> wait -> get/state -> close
```

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
nxctl click 3
nxctl input 4 "hello@example.com"
nxctl keys "Enter"
nxctl wait selector ".ready"
nxctl wait url "/done"
nxctl get title
nxctl screenshot
nxctl viewport 1280x720
nxctl close
```

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
