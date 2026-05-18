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
nxctl compare https://old.example.com/orders https://new.example.com/orders --old-scope-selector '#legacy-orders' --new-scope-selector 'main [data-testid="orders"]'
nxctl compare https://old.example.com/orders https://new.example.com/orders --node-scope semantic --match-mode histogram --matching-debug --output-json compare-debug.json
nxctl compare https://old.example.com/orders https://new.example.com/orders --node-scope semantic --match-mode histogram --matching-debug --output-decisions-template pair-decisions.todo.jsonl
nxctl compare validate-decisions --decisions-file pair-decisions.jsonl --compare-json compare-debug.json
nxctl compare https://old.example.com/orders https://new.example.com/orders --node-scope semantic --match-mode histogram --decisions-file pair-decisions.jsonl
nxctl compare https://old.example.com/orders https://new.example.com/orders --node-scope all --scope-selector 'main > section.hero' --match-mode histogram --matching-debug --output-json compare-debug.json
nxctl compare https://old.example.com/orders https://new.example.com/orders --compare-css --css-property color --css-property pointer-events
nxctl flow run --manifest migration-flow.json
nxctl inspect 'role button --name "Submit"' --old-session old --new-session new
nxctl inspect --old-scope-selector '#legacy-summary' --new-scope-selector '[data-testid="order-summary"]' --old-session old --new-session new --css-property width
```

Use `--node-scope all` only with an explicit scope selector when wrappers or layout containers are part of the migration. It observes every visible element in that subtree and emits `structure_key` / `subtree_signature` metadata so histogram can anchor containers without changing the base fingerprint.

## If Next.js Or Another Modern Frontend Looks Incomplete

Assume timing first.

1. inspect the target area with `state`
2. confirm whether the target content is present
3. strengthen readiness
4. rerun compare before concluding there is a regression

## AI-Assisted Pair Review

When automatic matching leaves ambiguous or unmatched nodes, rerun compare with `--matching-debug --output-json compare-debug.json`.

Review `matching_debug.ambiguous_candidates`, then append high-confidence decisions to a JSONL file:

```jsonl
{"kind":"pair","old":"@e203","new":"@e222","confidence":"high","reason":"same role/name and nearby bbox"}
{"kind":"subtree_pair","old":"@e40","new":"@e72","confidence":"high","match_kind":"ordered_children","count":12,"reason":"same link list region"}
{"kind":"pair","old":"@e9","new":"?","confidence":"unknown","reason":"needs human review"}
```

Rerun compare with `--decisions-file pair-decisions.jsonl`. Only high-confidence `pair` and `subtree_pair` entries affect matching; other entries remain review notes. Accepted missing/new decisions are stamped back onto findings with `decision_kind`.
Use `--output-decisions-template pair-decisions.todo.jsonl` to generate editable unknown pair stubs from ambiguous candidates. Validate the reviewed file with `nxctl compare validate-decisions --decisions-file pair-decisions.jsonl --compare-json compare-debug.json`; each line also matches `docs/ai/compare-decisions.schema.json`.
