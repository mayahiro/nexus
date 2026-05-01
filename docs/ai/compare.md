# Nexus AI Compare Guide

Use this guide when you need reliable compare results.

## Default Approach

1. Decide what you are comparing
2. Decide what "ready" means for that page
3. Run compare with the smallest meaningful scope
4. Split the work into multiple passes when the diff is noisy

## Readiness Rules

- Treat `document.readyState === "complete"` as a baseline, not a guarantee
- Prefer a page-specific `--wait-selector`
- Use `--wait-function` when readiness depends on data count or application state
- Use `--wait-network-idle` only as a supporting signal

Good wait targets:

- a ready marker such as `[data-testid="orders-loaded"]`
- a main-content selector such as `main table tbody tr`
- a function such as `document.querySelectorAll("tbody tr").length > 0`

Bad wait targets:

- `footer`
- a layout element that appears before the page content is ready
- an authentication-only indicator in the sidebar when the compare target is the main content

## Compare Scope

Do not try to validate everything in one pass.

Recommended passes:

- major text and labels
- actionable controls such as buttons, links, and form fields
- container-scoped passes such as one filters sidebar or one hero section using `--scope-selector`
- important styles such as `color`, `background-color`, and `pointer-events`
- significant visual placement changes using `--compare-layout`

## Noise Control

- Keep `--ignore-text-regex` minimal
- Use `--mask-selector` for sensitive values, timestamps, and IDs that are expected to differ
- Use `--ignore-selector` only for nodes that are truly outside the compare target
- If a diff is suspicious, reduce the suppression rules and rerun

## Command Patterns

Page-to-page compare:

```text
nxctl compare https://old.example.com/orders https://new.example.com/orders --wait-selector '[data-testid="orders-loaded"]'
```

Scoped compare:

```text
nxctl compare https://old.example.com/products https://new.example.com/products --wait-selector '.ready' --scope-selector 'aside.filters' --compare-css --css-property width --css-property padding
```

Session-to-session compare:

```text
nxctl compare --old-session old --new-session new --wait-function 'document.querySelectorAll("tbody tr").length > 0'
```

Targeted inspection:

```text
nxctl inspect 'role button --name "Submit"' --old-session old --new-session new
nxctl inspect 'role button' --old-session old --new-session new --nth 2 --css-property color
nxctl inspect --selector 'aside.filters' --old-session old --new-session new --css-property width
nxctl inspect 'role button --name "Submit"' --old-session old --new-session new --layout-context
```

Style-focused compare:

```text
nxctl compare https://old.example.com/orders https://new.example.com/orders --compare-css --css-property color --css-property pointer-events
```

Layout-focused compare:

```text
nxctl compare https://old.example.com/orders https://new.example.com/orders --compare-layout
```

`--compare-layout` reports significant viewport-relative bounds changes for matching nodes. It is useful for findings such as a control moving from center to left, but it does not infer the ancestor CSS change that caused the movement. Use `inspect --layout-context` for that follow-up.

## Failure Triage

If the new page looks incomplete:

1. Run `state` just before compare
2. Check whether the target content is really present
3. Strengthen readiness with `--wait-selector` or `--wait-function`
4. Narrow the compare scope before assuming there is a product bug

## Scope Selector Rules

- `--scope-selector` accepts a raw CSS selector and restricts compare to exactly one matched subtree on each side
- positional selectors such as `:nth-child()` and `:nth-of-type()` are allowed
- prefer stable ids, classes, or attributes before positional selectors
- if the selector matches 0 or multiple elements on either side, compare fails early

## Inspect Selector Rules

- `inspect --selector` accepts a raw CSS selector and compares the computed styles for exactly one matched element on each side
- positional selectors such as `:nth-child()` and `:nth-of-type()` are allowed
- do not combine `--selector` with a positional inspect locator
- do not combine `--selector` with `--nth`
- use `inspect --layout-context` when ancestor layout CSS may explain the target element's size, position, wrapping, or overflow
- `--layout-context` is capability-based; Chromium returns DOM ancestor layout CSS, and unsupported backends fail explicitly
