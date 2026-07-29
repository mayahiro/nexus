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

- `defaults`: shared defaults such as `backend`, `target_ref`, `viewport`, `wait_timeout`, `match_mode`, `node_scope`, `matching_debug`, `compare_css`, `all_css_properties`, `compare_layout`, `no_default_ignores`, `scope_selector`, `old_scope_selector`, `new_scope_selector`, `css_property`, `ignore_text_regex`, `ignore_selector`, and `mask_selector`
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
- `backend` (`chromium`)
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
- `timeout` for `wait` and `screenshot`, in milliseconds
- `locator` for `click`, `fill`, and targeted `screenshot`
- `nth` for repeated locator matches
- `text` for `fill`
- `value` for `wait`, `navigate`, and `viewport`
- `path`, `full`, and `annotate` for `screenshot`

`screenshot` writes a PNG to `path`.
With `side: both`, Nexus automatically writes `-old` and `-new` suffixed files.
When `locator` is present, `screenshot` captures just the matched element instead of the whole viewport.
Use `nth` when multiple nodes intentionally share the same locator.
Screenshot capture times out after 30000 ms by default. Each capture attempt, including its readiness check, is capped at 10000 ms. The paint-readiness barrier is best-effort for at most 1000 ms; when fallback capture is used, the step report keeps a side-specific entry in `warnings`. Use an explicit `wait` step before the screenshot when final visual readiness is part of the assertion. Set `timeout` on the step to budget enough time for same-target reconnect; it does not extend one capture attempt beyond 10000 ms. Flow screenshot steps do not opt into destructive tab replacement.
`full` is not supported together with `locator`.
For a generic post-load stabilization barrier, use a wait step with `"target": "hydrated"` and no `value`. It is a DOM-quiet rendering heuristic; use `"target": "function"` with an application expression when a stronger readiness signal exists.
`compare` supports step-level overrides such as `match_mode`, `node_scope`, `matching_debug`, `compare_css`, `all_css_properties`, `compare_layout`, `no_default_ignores`, `scope_selector`, `old_scope_selector`, `new_scope_selector`, `css_property`, `ignore_text_regex`, `ignore_selector`, and `mask_selector`.
`all_css_properties` and `css_property` are alternative modes. Do not set both in the same defaults or step object; the manifest is rejected. A step-level `css_property` list overrides an inherited exhaustive mode.
Set step-level `compare_css` to false without another step-level CSS mode to disable inherited CSS comparison.

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
