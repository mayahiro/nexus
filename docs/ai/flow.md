# Nexus AI Flow Guide

Use `flow run` when the task requires session continuity.

## When To Use Flow

- login is required before compare
- the same journey must run against old and new systems
- one scenario should be replayed across multiple matrices such as desktop and mobile

Use plain `compare` when the task is only an independent page-to-page check.

## Preferred Pattern

The default successful pattern is:

```text
navigate -> wait -> compare
```

Use `navigate` when the post-login landing page is not the page you actually need to inspect.
Insert `screenshot` before `compare` when the flow should keep visual artifacts.

## Manifest Summary

Use a JSON manifest with these top-level keys:

- `defaults`: shared defaults such as `backend`, `target_ref`, `viewport`, `wait_timeout`, `compare_css`, `scope_selector`, `css_property`, `ignore_text_regex`, `ignore_selector`, and `mask_selector`
- `matrices`: named viewport or variable sets that a scenario can replay against
- `scenarios`: the runnable flow list

Each scenario usually includes:

- `name`
- `matrix`
- `variables`
- `old`
- `new`
- `steps`

Each `old` and `new` endpoint supports:

- `url` or `session`
- `backend`
- `target_ref`
- `viewport`

Use `url` when Nexus should attach a fresh browser session.
Use `session` when the flow should reuse an existing session.
String values support simple `{{ name }}` substitution from `scenario.variables` and `matrix.variables`.

## Supported Steps

The currently implemented flow actions are:

- `wait`
- `navigate`
- `click`
- `fill`
- `viewport`
- `screenshot`
- `compare`

Useful step fields:

- `side`: `old`, `new`, or `both`
- `continue_on_error`
- `timeout` for `wait`
- `locator` for `click`, `fill`, and targeted `screenshot`
- `nth` for repeated locator matches
- `text` for `fill`
- `value` for `wait`, `navigate`, and `viewport`
- `path`, `full`, and `annotate` for `screenshot`

`screenshot` writes a PNG to `path`.
With `side: both`, Nexus automatically writes `-old` and `-new` suffixed files.
When `locator` is present, `screenshot` captures just the matched element instead of the whole viewport.
Use `nth` when multiple nodes intentionally share the same locator.
`full` is not supported together with `locator`.
`compare` supports step-level overrides such as `compare_css`, `scope_selector`, `css_property`, `ignore_text_regex`, `ignore_selector`, and `mask_selector`.

## Why `navigate` Matters

- it removes noisy menu-click steps
- it makes the target page explicit
- it keeps compare responsibility separate from navigation responsibility

## Minimal Scenario Shape

```json
{
  "scenarios": [
    {
      "name": "orders",
      "old": { "session": "old" },
      "new": { "session": "new" },
      "steps": [
        {
          "action": "navigate",
          "value": "https://example.com/orders"
        },
        {
          "action": "wait",
          "target": "selector",
          "value": "[data-testid='orders-loaded']"
        },
        {
          "action": "compare",
          "name": "orders"
        }
      ]
    }
  ]
}
```

## Flow Rules

- prefer `navigate` over `click` when the goal is simply to reach a known page
- prefer a page-specific wait target over a layout-level wait target
- keep scenarios short and outcome-oriented
- use matrices only when the same scenario truly needs to run in multiple viewports
- use `screenshot` only at checkpoints that need a PNG artifact; with `side: both`, the saved files automatically gain `-old` and `-new` suffixes

## When To Avoid Flow

- the task is a single compare between two URLs
- the scenario does not need login or session reuse
- the compare target can be reached directly without any shared state
