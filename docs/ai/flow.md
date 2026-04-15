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

## When To Avoid Flow

- the task is a single compare between two URLs
- the scenario does not need login or session reuse
- the compare target can be reached directly without any shared state
