# Nexus AI Migration Playbook

Use this playbook for migration projects such as legacy server-rendered systems moving to Rails, Next.js, or another modern stack.

## Main Idea

Do not treat migration compare as one giant pass.

Split the work into separate checks:

- major text
- actionable controls
- important styles

## Before Running Compare

Decide these first:

1. which page or journey you are validating
2. what state counts as ready
3. which differences are meaningful
4. which differences are noise

## Recommended Order

1. validate text and labels
2. validate controls and states
3. validate important styles

This reduces noise and makes findings easier to interpret.

## Readiness In Modern Frontends

SSR alone is not enough.

Modern apps may still be:

- hydrating
- replacing DOM after load
- fetching data after the first HTML arrives
- showing fallback or skeleton content

If compare runs too early, a missing element on the new system may be a timing issue instead of a product bug.

## Default Pattern

```text
login -> navigate -> wait -> compare
```

If login is already done:

```text
navigate -> wait -> compare
```

## Wait Strategy

Prefer:

- a ready marker in the target page
- a selector inside the main content
- a function that checks the target data count or target application state

Avoid:

- `footer`
- sidebar-only authentication indicators
- generic layout markers that appear before the target content is stable

## Commands That Usually Work Well

```text
nxctl compare https://old.example.com/orders https://new.example.com/orders --wait-selector '[data-testid="orders-loaded"]'
nxctl compare https://old.example.com/orders https://new.example.com/orders --compare-css --css-property color --css-property pointer-events
nxctl flow run --manifest migration-flow.json
nxctl inspect 'role button --name "Submit"' --old-session old --new-session new
```

## If Next.js Or Another Modern Frontend Looks Incomplete

Assume timing first.

1. inspect the target area with `state`
2. confirm whether the target content is present
3. strengthen readiness
4. rerun compare before concluding there is a regression
