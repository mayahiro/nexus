# Nexus AI Migration Playbook

Use this playbook for migration projects such as legacy server-rendered systems moving to Rails, Next.js, or another modern stack.

Related docs:

- compare command behavior: [compare.md](../compare.md)
- decision JSONL schema: [compare-decisions.schema.json](../compare-decisions.schema.json)
- playbook index: [README.md](README.md)

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
nxctl compare validate-decisions --decisions-file pair-decisions.jsonl --compare-json compare-debug.json --strict
nxctl compare validate-decisions --decisions-file pair-decisions.selectors.jsonl --compare-json compare-debug.json --old-session old --new-session new
nxctl compare materialize-decisions --decisions-file pair-decisions.locators.jsonl --compare-json compare-debug.json --output pair-decisions.jsonl
nxctl compare materialize-decisions --decisions-file pair-decisions.selectors.jsonl --compare-json compare-debug.json --old-session old --new-session new --output pair-decisions.jsonl
nxctl compare repair-decisions --decisions-file pair-decisions.jsonl --compare-json compare-debug.json --output pair-decisions.repaired.jsonl
nxctl compare https://old.example.com/orders https://new.example.com/orders --node-scope semantic --match-mode histogram --decisions-file pair-decisions.jsonl
nxctl compare https://old.example.com/orders https://new.example.com/orders --node-scope all --scope-selector 'main > section.hero' --match-mode histogram --matching-debug --output-json compare-debug.json
nxctl compare https://old.example.com/orders https://new.example.com/orders --node-scope all --scope-selector 'svg.logo' --no-default-ignores --matching-debug --output-json compare-debug.json
nxctl compare https://old.example.com/orders https://new.example.com/orders --compare-css --css-property color --css-property pointer-events
nxctl flow run --manifest migration-flow.json
nxctl inspect 'role button --name "Submit"' --old-session old --new-session new
nxctl inspect --old-scope-selector '#legacy-summary' --new-scope-selector '[data-testid="order-summary"]' --old-session old --new-session new --css-property width
```

Use `--all-css-properties` only for an exhaustive follow-up inside a narrow scope. It is mutually exclusive with `--css-property` and can expose browser-default or inherited-property noise that the stable default allowlist intentionally avoids.

Use `--node-scope all` only with an explicit common or side-specific scope selector when wrappers or layout containers are part of the migration. It observes every visible element in that subtree and emits `structure_key` / `subtree_signature` metadata so histogram can anchor containers without changing the base fingerprint. For full node-scope semantics, default ignores, and matching debug fields, see [../compare.md#node-scope-selection](../compare.md#node-scope-selection).

Prefer this order when structural DOM differences matter:

1. Start with `--node-scope semantic --match-mode histogram` to identify durable anchors and visible content regressions.
2. Move to `--node-scope all --scope-selector <component-root>` only for the component whose wrappers, layout containers, or anonymous nodes need review.
3. Keep the scope to one meaningful region such as a hero, filter sidebar, navigation block, card grid, or migrated component root.
4. Add `--matching-debug --output-json compare-debug.json` and inspect unmatched nodes before writing decisions.
5. Use `--output-decisions-template` when repeated unmatched or ambiguous nodes need a reviewed JSONL handoff.

Treat `all` findings as structural evidence, not automatic regressions. Anonymous `div` and `span` nodes often need the surrounding `structure_key`, `subtree_signature`, text length, child shape, and screenshot context before they can be judged. SVG descendants, hidden nodes, and tool-specific skip markers are ignored by default in `all` mode. If the vector drawing internals are the migration target, rerun that focused scope with `--no-default-ignores`; otherwise compare the SVG root or accessible wrapper as the meaningful unit.

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
{"kind":"pair","old_locator":"role:button label:\"Save changes\"","new_locator":"href:/jobs","confidence":"high","reason":"same CTA"}
{"kind":"pair","old_selector":"#legacy .save","new_selector":"main .save","confidence":"high","reason":"same CTA"}
{"kind":"subtree_pair","old":"@e40","new":"@e72","confidence":"high","match_kind":"ordered_children","count":12,"reason":"same link list region"}
{"kind":"subtree_pair","old":"@e90","new":"@e120","confidence":"high","match_kind":"ordered_descendants","count":18,"reason":"same nested logo asset order"}
{"kind":"subtree_pair","old":"@e91","new":"@e121","confidence":"high","match_kind":"opaque_subtree","reason":"asset root is equivalent; internals intentionally differ"}
{"kind":"pair","old":"@e9","new":"?","confidence":"unknown","reason":"needs human review"}
{"kind":"accepted_finding","finding_id":"text_changed:3fa21c9d4b2a","reason":"approved copy change"}
```

When a decision uses `old_selector` or `new_selector`, first run `nxctl compare validate-decisions --decisions-file pair-decisions.selectors.jsonl --compare-json compare-debug.json --old-session old --new-session new --json` to preflight selector uniqueness and compare JSON mapping; successful selector fields are counted as `selector_preflighted`. When a decision uses `old_locator` or `new_locator`, run `nxctl compare materialize-decisions --decisions-file pair-decisions.locators.jsonl --compare-json compare-debug.json --output pair-decisions.jsonl --json`. Each locator must match exactly one compare JSON node, then the command writes the concrete `old` or `new` ref. When materializing selector decisions, include `--old-session` or `--new-session`; each selector must match one live DOM node that maps uniquely back to the compare JSON node. Inspect `materialized[]` in the JSON report to see each resolved line's source field, input value, ref, match strategy, current compare node summary, and selector-backed live node summary.
Rerun compare with `--decisions-file pair-decisions.jsonl`. Only high-confidence `pair` and `subtree_pair` entries affect matching; other entries remain review notes. Use the [`subtree_pair` match kind flow](../compare.md#subtree-pair-match-kind-flow) before choosing a broad subtree decision: `ordered_children` is for direct-child order, `ordered_descendants` is for stable nested descendant order, and `opaque_subtree` is for equivalent roots whose internals intentionally changed. Accepted missing/new decisions and finding-level decisions are stamped back onto findings with `decision_kind`.
Use `--review-dir review/orders` when starting a review pass to produce `REVIEW.md`, `compare.json`, `compare.md`, pair/finding/cluster decision templates, full-page screenshots, cropped finding screenshots, repeated finding clusters, and `review-summary.json` together. Start with `REVIEW.md`; the pair template includes old locators/selectors and new candidate notes with refs, locators, selectors, scores, and shared/differing keys, suppresses ambiguous/unmatched duplicates, and records review volume in `pair_decision_template_counts`. Selector hints are included only when Nexus can derive a unique selector from observed compare nodes. When `--decisions-file` is supplied, `review-summary.json` also records `decision_audit` counts and unresolved examples for applied, pending, stale, and conflicting decisions.
For a manifest run, use `nxctl compare --manifest migration-pages.json --review-dir review/migration` to produce a root `REVIEW.md`, manifest-level summaries, root `cluster-decisions.todo.jsonl`, `review-index.md`, `review-index.html`, and one review packet directory per page. Start with the root `REVIEW.md`, then use `review-index.md` to prioritize failed, critical, high pair-decision workload pages, or pages with pending/stale/conflicting decisions. Open `review-index.html` for a static side-by-side screenshot overview with repeated finding clusters, cropped finding screenshots, finding IDs, and copyable decision JSONL stubs visible. Review `cluster-decisions.todo.jsonl` when one decision applies to a repeated cluster. Each page packet has its own `REVIEW.md` so it can be reviewed independently.
Use `--output-finding-decisions-template finding-decisions.todo.jsonl` after a compare run to produce `unknown`-confidence review stubs for current critical and warning findings.
Use `nxctl compare validate-decisions --decisions-file cluster-decisions.todo.jsonl --review-summary review-summary.json` to validate manifest-level repeated cluster decisions.
Use `nxctl compare validate-decisions --decisions-file pair-decisions.jsonl --compare-json compare-debug.json --strict` before reusing AI-authored decisions when you want unknown fields, kind-unused fields, or unsupported `schema_version` values to fail fast instead of remaining warnings.
Use `nxctl compare repair-decisions --decisions-file pair-decisions.jsonl --compare-json compare-debug.json --output pair-decisions.repaired.jsonl` when a reused decision file has stale refs. It updates stale concrete `old` or `new` refs only when selector, locator, or fingerprint metadata resolves uniquely in the current run; unresolved refs remain unchanged as warnings. Inspect `repaired[]` in the JSON report to see old/new refs, match strategy, current compare node summary, and selector-backed live node summary.
Use `nxctl compare normalize-decisions --decisions-file pair-decisions.jsonl --compare-json compare-debug.json --review-summary review-summary.json --output pair-decisions.normalized.jsonl` before reusing a reviewed file to materialize finding-cluster decisions, remove duplicate decisions, and catch stale refs or finding IDs.
Use `nxctl compare audit-decisions --decisions-file pair-decisions.jsonl --compare-json compare-debug.json --json` after rerunning compare to confirm reviewed decisions were applied and to surface stale or conflicting entries. Inspect `entries[]` for `reason`, `expected`, `actual`, `conflict_line`, and `repair_hint` before running repair or rewriting stale decisions.
Use `--output-decisions-template pair-decisions.todo.jsonl` to generate editable unknown stubs from ambiguous candidates and unmatched old/new nodes. Unmatched old entries can be paired or changed to `accepted_removed`; unmatched new entries start as `accepted_added`. Validate the reviewed file with `nxctl compare validate-decisions --decisions-file pair-decisions.jsonl --compare-json compare-debug.json`; add `--old-session` or `--new-session` for selector-backed decisions. Each line also matches `docs/ai/compare-decisions.schema.json`.
