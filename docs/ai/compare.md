# Nexus AI Compare Guide

Use this guide when you need reliable compare results.

Related docs:

- README command overview: [README.md](../../README.md)
- migration execution order: [playbooks/migration.md](playbooks/migration.md)
- decision JSONL schema: [compare-decisions.schema.json](compare-decisions.schema.json)

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
- migration-scoped passes using `--old-scope-selector` and `--new-scope-selector` when old and new DOM structures differ
- migration-friendly matching with `--match-mode stable`, `--match-mode heuristic`, or experimental `--match-mode histogram`
- broader semantic candidate collection with `--node-scope semantic`
- important styles such as `color`, `background-color`, and `pointer-events`
- significant visual placement changes using `--compare-layout`

## Noise Control

- Keep `--ignore-text-regex` minimal
- Use `--mask-selector` for sensitive values, timestamps, and IDs that are expected to differ
- Use `--ignore-selector` only for nodes that are truly outside the compare target
- Use `tag=<value>` or `attr:<name>=<value>` selector rules for structural noise that lacks role/name/text identity
- With `--node-scope all`, rely on default structural ignores first, then use `--no-default-ignores` only when ignored nodes are the review target
- If a diff is suspicious, reduce the suppression rules and rerun

## Command Patterns

Page-to-page compare:

```text
nxctl compare https://old.example.com/orders https://new.example.com/orders --wait-selector '[data-testid="orders-loaded"]'
```

Scoped compare:

```text
nxctl compare https://old.example.com/products https://new.example.com/products --wait-selector '.ready' --scope-selector 'aside.filters' --compare-css --css-property width --css-property padding
nxctl compare https://old.example.com/products https://new.example.com/products --old-scope-selector '#legacy-filters' --new-scope-selector 'aside.filters'
```

Migration-friendly matching:

```text
nxctl compare https://old.example.com/orders https://new.example.com/orders --match-mode stable
nxctl compare https://old.example.com/orders https://new.example.com/orders --match-mode heuristic --scope-selector 'main'
nxctl compare https://old.example.com/orders https://new.example.com/orders --node-scope semantic --match-mode stable --scope-selector 'main'
nxctl compare https://old.example.com/orders https://new.example.com/orders --node-scope semantic --match-mode histogram
nxctl compare https://old.example.com/orders https://new.example.com/orders --node-scope semantic --match-mode histogram --matching-debug --output-json compare-debug.json
nxctl compare https://old.example.com/orders https://new.example.com/orders --node-scope semantic --match-mode histogram --matching-debug --output-decisions-template pair-decisions.todo.jsonl
nxctl compare https://old.example.com/orders https://new.example.com/orders --node-scope semantic --match-mode histogram --decisions-file pair-decisions.jsonl
nxctl compare https://old.example.com/orders https://new.example.com/orders --node-scope all --scope-selector 'main > section.hero' --match-mode histogram --matching-debug
nxctl compare https://old.example.com/orders https://new.example.com/orders --node-scope all --scope-selector 'svg.logo' --no-default-ignores --matching-debug
nxctl compare validate-decisions --decisions-file pair-decisions.jsonl --compare-json compare-debug.json
nxctl compare validate-decisions --decisions-file pair-decisions.jsonl --compare-json compare-debug.json --strict
nxctl compare validate-decisions --decisions-file pair-decisions.selectors.jsonl --compare-json compare-debug.json --old-session old --new-session new
nxctl compare repair-decisions --decisions-file pair-decisions.jsonl --compare-json compare-debug.json --output pair-decisions.repaired.jsonl
nxctl compare https://old.example.com/orders https://new.example.com/orders --node-scope semantic --match-mode histogram --review-dir review/orders
nxctl compare --manifest migration-pages.json --review-dir review/migration
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
nxctl inspect --old-scope-selector '#legacy-filters' --new-scope-selector 'aside.filters' --old-session old --new-session new --css-property width
nxctl inspect 'role button --name "Submit"' --old-session old --new-session new --layout-context
```

Style-focused compare:

```text
nxctl compare https://old.example.com/orders https://new.example.com/orders --compare-css --css-property color --css-property pointer-events
```

Use `--compare-css` without explicit properties for the stable default allowlist. Use repeated `--css-property` flags for a focused assertion. Use `--all-css-properties` for an exhaustive computed-style scan:

```text
nxctl compare https://old.example.com/orders https://new.example.com/orders --all-css-properties
```

The exhaustive mode cannot be combined with `--css-property`. Compare manifests likewise reject `all_css_properties: true` together with `css_property` in the same defaults or page object. A page-level property list can still override exhaustive mode inherited from defaults. Exhaustive comparison can surface browser-version defaults, inherited values, and properties unrelated to the intended migration, so keep the node scope narrow and review the resulting noise rather than treating every finding as a regression.

Layout-focused compare:

```text
nxctl compare https://old.example.com/orders https://new.example.com/orders --compare-layout
```

`--compare-layout` reports significant viewport-relative bounds changes for matching nodes. It is useful for findings such as a control moving from center to left, but it does not infer the ancestor CSS change that caused the movement. Use `inspect --layout-context` for that follow-up.

## Match Modes

- `exact` is the default and preserves strict fingerprint-based matching
- `stable` matches unique identity keys such as `data-testid`, `id`, `href`, form labels, role/name, attributes, placeholders, and then fingerprints
- `heuristic` runs stable matching first, then only accepts mutual best score-based matches above the confidence threshold
- `histogram` is experimental; it anchors low-occurrence semantic identity keys, then applies exact and heuristic matching inside each anchored region
- use `stable` for migrations that preserve durable attributes but change text or implementation details
- use `heuristic` when stable keys are incomplete and the scope is already narrow
- use `histogram` with `--node-scope semantic` for early page-wide experiments where durable anchors exist across the page
- use `histogram` with `--node-scope all --scope-selector <css>` for focused subtree work where wrapper `div` and layout containers matter
- if a heuristic result looks suspicious, rerun with `--match-mode exact` or narrow the scope further

JSON findings include a stable `finding_id`. Findings produced from stable, heuristic, or histogram node pairs include `matched_by`, and heuristic findings include `match_score` and `match_reasons`.

## Node Scope Selection

Use the narrowest node scope that still covers the migration risk.

- `current` preserves the existing observed candidates and is the safest default for broad checks
- `actionable` focuses on controls and links when user interaction regressions matter most
- `semantic` adds named or content-bearing semantic nodes such as headings, landmarks, status, tables, images with names, and testid-tagged elements
- `all` observes every visible element in one explicit scope and is for focused structural review

`--node-scope all` requires `--scope-selector` or both `--old-scope-selector` and `--new-scope-selector`. Use it when wrapper elements, layout containers, or anonymous DOM structure are part of what changed, for example one hero, one sidebar, one card grid, or one migrated component root. Do not use it as a broad page-wide starting point; repeated anonymous wrappers, decorative elements, and SVG internals can produce more review work than signal.

When `all` is used, compare adds `structure_key` and `subtree_signature` to nodes, matching debug entries, and missing/new findings. `structure_key` is a DOM-order structural path. `subtree_signature` summarizes the node role/tag, text-length bucket, descendant-count bucket, direct child-count bucket, first child role/tag, and width bucket. Histogram matching can use these low-occurrence structural values as anchors without changing the base fingerprint, which keeps normal fingerprint behavior stable while making anonymous containers easier to reason about in debug output.

Default `all` ignores suppress common structural noise before matching: SVG descendants below the root `<svg>`, `script`, `style`, `link`, `meta`, `noscript`, `[hidden]`, `[aria-hidden="true"]`, and `[data-nxctl-skip="true"]`. This keeps decorative icons, hidden DOM, and tool-specific skip markers from dominating unmatched-node review. Add `--no-default-ignores` when the ignored nodes are the target, such as reviewing the internal geometry of one SVG asset.

Practical `all` workflow:

1. Select one stable component boundary with `--scope-selector`, or use side-specific selectors when old and new DOM roots differ.
2. Run `--match-mode histogram --matching-debug --output-json compare-debug.json`.
3. Inspect `matching_debug.unmatched_old`, `matching_debug.unmatched_new`, and ambiguous candidates before deciding whether noise is a real regression.
4. Use `--output-decisions-template` when the same region needs reviewed pair or accepted-added/removed decisions.
5. If hidden or decorative nodes still dominate the unmatched lists, narrow the scope or add explicit `--ignore-selector` rules after confirming they are outside the compare target.

SVGs deserve special care in `all` mode. Nexus treats the `<svg>` root as the default compare unit and ignores visible descendants like `path`, `g`, `line`, `circle`, and `polyline` unless `--no-default-ignores` is set. That is useful when a logo or icon was intentionally replaced by a differently structured asset. Use `--no-default-ignores` only when the vector drawing internals are the target of review.

## Pair Decisions

Use `--decisions-file <jsonl>` when an AI or human has reviewed ambiguous candidates and wants compare to reuse high-confidence pairings.

Each line is one JSON object. Validate each line against `docs/ai/compare-decisions.schema.json`. Compare applies high-confidence `pair` and `subtree_pair` entries before automatic matching:

```jsonl
{"kind":"pair","old":"@e203","new":"@e222","confidence":"high","reason":"bbox and role/name match; aria-label changed as an a11y improvement"}
{"kind":"pair","old_locator":"role:button label:\"Save changes\"","new_locator":"href:/jobs","confidence":"high","reason":"same CTA; materialize refs before compare"}
{"kind":"pair","old_selector":"#legacy .save","new_selector":"main .save","confidence":"high","reason":"same CTA; materialize selectors before compare"}
{"kind":"subtree_pair","old":"@e40","new":"@e72","confidence":"high","match_kind":"ordered_children","count":12,"reason":"same link list region"}
{"kind":"subtree_pair","old":"@e90","new":"@e120","confidence":"high","match_kind":"ordered_descendants","count":18,"reason":"same nested logo asset order"}
{"kind":"subtree_pair","old":"@e91","new":"@e121","confidence":"high","match_kind":"opaque_subtree","reason":"asset root is equivalent; internals intentionally differ"}
{"kind":"pair","old":"@e9","new":"?","confidence":"unknown","reason":"needs review"}
{"kind":"accepted_removed","old":"@e45","reason":"legacy-only footer link intentionally removed"}
{"kind":"accepted_added","new":"@e88","reason":"new skip-link"}
{"kind":"accepted_finding","finding_id":"text_changed:3fa21c9d4b2a","reason":"approved copy change"}
{"kind":"regression_finding","finding_id":"layout_changed:4d2aa4107e9f","reason":"primary CTA moved below the fold"}
{"kind":"accepted_finding_cluster","cluster_key":"warning | layout_changed | layout_changed |  | bounds |  |  |  | ","confidence":"high","reason":"same repeated acceptable layout shift"}
```

`old_fingerprint` and `new_fingerprint` may be included to detect stale refs. If the AI can identify a node by visible or semantic features but does not want to guess a current `@e...` ref, it can write `old_locator` or `new_locator` first. Locator terms support `@eN`, `role:button`, `label:"Save changes"`, `name:Save`, `text:Login`, `href:/jobs`, `testid:submit`, `fingerprint:<value>`, and `role=button&name=Save`. If the AI knows a stable CSS path in the live DOM, it can write `old_selector` or `new_selector` first. Run `nxctl compare validate-decisions --decisions-file <jsonl> --compare-json <file> --old-session <id> --new-session <id> --json` to preflight selectors before materialization; it checks each selector against the live DOM and verifies that the matched live node maps uniquely to one compare JSON node. Run `nxctl compare materialize-decisions --decisions-file <jsonl> --compare-json <file> --old-session <id> --new-session <id> --output <jsonl> --json` to resolve locator-only or selector-only decisions to concrete refs and review the `materialized[]` explanations; each locator must match exactly one compare JSON node, and each selector must match one live DOM node that maps uniquely to a compare JSON node. Non-high entries are accepted as review notes but are not used as anchors or matches.
`subtree_pair` supports `match_kind:"ordered_children"`, `match_kind:"ordered_descendants"`, and `match_kind:"opaque_subtree"` for concrete or materialized roots. `ordered_children` pairs the roots, then pairs observed direct children in DOM order. `ordered_descendants` pairs the roots, then flattens all observed descendants under each root in DOM order and pairs those descendants, including grandchildren and deeper nodes. `opaque_subtree` pairs the roots, treats matched descendants as intentionally opaque, suppresses internal matched-node findings, and downgrades unmatched internal missing/new descendants to `info`. The roots can start as `old_selector` / `new_selector`, `old_locator` / `new_locator`, or fingerprint metadata, but materialize those fields to concrete refs before compare. `count` is optional and validates the expected number of child or descendant pairs, excluding the root pair.

### Subtree Pair Match Kind Flow

Choose `subtree_pair` only when the old root and new root are clearly the same logical region, component, or asset. If the root equivalence is uncertain, keep the decision at individual `pair`, `accepted_removed`, or `accepted_added` entries.

1. Is the root itself the same semantic unit on both sides?
   - No: do not use `subtree_pair`; narrow the scope or write smaller decisions.
   - Yes: continue.
2. Should internal matched-node findings still be reviewed?
   - No: use `match_kind:"opaque_subtree"` when the root is equivalent but internals intentionally changed.
   - Yes: continue.
3. Do only the direct children correspond in the same DOM order?
   - Yes: use `match_kind:"ordered_children"`.
   - No: continue.
4. Do all observed descendants, including grandchildren and deeper nodes, correspond in the same DOM order?
   - Yes: use `match_kind:"ordered_descendants"`.
   - No: avoid one broad `subtree_pair`; split into smaller roots or explicit `pair` decisions.

`ordered_descendants` is still order-based matching, not fuzzy semantic matching. Use it when wrappers or nesting make direct-child pairing too shallow, but the descendant sequence is stable enough to review as one region. Use `opaque_subtree` only when hiding internal differences is intentional, such as a replaced logo asset or a component whose inner markup is not part of the current regression target.

| Situation | Prefer |
| --- | --- |
| Only direct children intentionally correspond in order | `subtree_pair` with `match_kind:"ordered_children"` |
| Nested descendants intentionally correspond in DOM order | `subtree_pair` with `match_kind:"ordered_descendants"` |
| Root is the semantic unit and internals intentionally changed | `subtree_pair` with `match_kind:"opaque_subtree"` |
| SVG/icon internals are decorative noise | default `--node-scope all` ignores |
| SVG/vector internals are the target of review | `--no-default-ignores` with a narrow scope |
| One repeated node type is known noise | `--ignore-selector`, including `tag=<name>` or `attr:<name>=<value>` |

| Example | Prefer | Why |
| --- | --- | --- |
| A header nav list whose old and new roots are the same navigation region and whose link items appear in the same direct-child order | `ordered_children` | The direct children are the intended comparison units, and deeper descendants can be handled by normal matching. |
| A nested sidebar menu or card list where wrappers are stable enough that every observed descendant lines up in DOM order | `ordered_descendants` | Grandchildren and deeper nodes need deterministic pairing, and the full descendant order is meaningful. |
| An old SVG logo and a new optimized SVG logo represent the same brand mark but use different internal `path` / `g` structure | `opaque_subtree` | The asset root is the reviewed unit, and internal geometry differences would be noise for this compare. |
| Decorative icon SVG paths dominate `missing_node` / `new_node` output in `--node-scope all` | Default `all` ignores | The internal vector nodes are not the review target, so no pair decision is needed. |
| The internal geometry of one SVG is the review target | `--no-default-ignores` with a narrow scope, then explicit `pair` or `ordered_descendants` only if order is reliable | Default ignores would hide the nodes that must be reviewed. |
| A hero section was redesigned and descendant order no longer maps cleanly | Smaller `pair` decisions or accepted added/removed decisions | A broad subtree pair would manufacture matches that do not represent the visual change. |

Use `--output-decisions-template <jsonl>` with `--matching-debug` to write editable review stubs from `matching_debug.ambiguous_candidates`, `unmatched_old`, and `unmatched_new`. Ambiguous and unmatched-old stubs start as `unknown` pair decisions; unmatched-new stubs start as `unknown` `accepted_added` decisions. Stubs include `old_locator`, `new_locator`, `old_selector`, or `new_selector` when Nexus can infer them. Selector hints are emitted only when a selector derived from id, testid, name, href, aria-label, or structure path is unique within the observed compare nodes. Candidate notes include each new candidate's ref, locator, selector, score, shared keys, and differing keys so an AI can either choose a concrete ref or write a locator/selector and materialize it. Nodes already covered by an ambiguous candidate are suppressed from unmatched stubs to reduce duplicate review work. Unmatched template output is capped to keep very noisy pages reviewable; inspect `matching_debug.unmatched_old` and `matching_debug.unmatched_new` in `compare.json` for any remaining nodes.
Use `--output-finding-decisions-template <jsonl>` to write editable `unknown`-confidence stubs for current `critical` and `warning` findings. Review those lines, then set `confidence:"high"` or remove `confidence` to apply the `accepted_finding` or `regression_finding` decision on the next compare run.
Use `--review-dir <dir>` to write `REVIEW.md`, `compare.json`, `compare.md`, `pair-decisions.todo.jsonl`, `finding-decisions.todo.jsonl`, `cluster-decisions.todo.jsonl`, full-page `old.png` and `new.png`, best-effort cropped finding screenshots under `findings/`, repeated `finding_clusters`, and `review-summary.json` in one pass. Start with `REVIEW.md`; it summarizes the packet, points to the files to inspect, lists validation/materialization/normalization commands, and states the decision guidance for ambiguous, unmatched, finding, and cluster review. Screenshot and crop capture are best-effort; unsupported backends keep the packet and record warnings in `review-summary.json`. Crops use document-relative bounds when the backend provides them, so long pages can still produce finding crops outside the initial viewport. It enables matching debug automatically so pair decision review has the needed context, and `review-summary.json` includes `pair_decision_template_counts`, plus `decision_audit` counts and unresolved `decision_audit_examples` when `--decisions-file` is supplied.
With `--manifest`, `--review-dir <dir>` writes a root `REVIEW.md`, `manifest.json`, `manifest.md`, root `cluster-decisions.todo.jsonl`, a root `review-summary.json`, `review-index.md`, and `review-index.html`, then writes one page review packet directory per manifest page. Start with the root `REVIEW.md` for the review order, pair-decision workload summary, decision audit summary, cluster-decision guidance, and links to the visual/markdown indexes. Use `review-index.md` to choose high-priority pages before opening each page packet; it includes finding counts, page-level pair decision template counts, and decision audit counts. Open `review-index.html` for a static side-by-side screenshot overview with repeated finding clusters, cropped finding screenshots, the first critical and warning findings, plus copyable `accepted_finding_cluster` and `regression_finding_cluster` JSONL stubs. Each page packet includes its own `REVIEW.md`.
Use `nxctl compare validate-decisions --decisions-file <jsonl> --compare-json <file> --review-summary <file>` to validate JSONL structure, supported kind names, unsupported `schema_version`, unknown fields, fields that do not apply to the current `kind`, duplicate high-confidence pairs/subtrees/finding decisions, current refs/fingerprints/finding IDs, and repeated finding cluster keys before rerunning compare. Add `--strict` to turn unknown or kind-unused field warnings into errors. Add `--old-session` or `--new-session` to preflight selector-backed decisions; the JSON summary reports successful selector fields as `selector_preflighted`.
Use `nxctl compare materialize-decisions --decisions-file <jsonl> --compare-json <file> --old-session <id> --new-session <id> --output <jsonl> --json` to convert `old_locator`, `new_locator`, `old_selector`, and `new_selector` fields into concrete `old` and `new` refs. The JSON report includes `materialized[]` entries with `line`, `side`, `source`, `value`, `ref`, `matched_by`, `node`, and selector-backed `live_node` so an AI or human can review why a ref was chosen. This is the handoff point for AI-written locator or selector decisions: run it before feeding locator-only or selector-only high-confidence pair, subtree, added, or removed decisions into compare. Omit a session flag only when the file does not contain selectors for that side.
Use `nxctl compare repair-decisions --decisions-file <jsonl> --compare-json <file> --output <jsonl> --json` when an older reviewed file has stale `old` or `new` refs. Repair checks only existing concrete refs that are missing from the current compare JSON or whose fingerprint no longer matches; it then tries selector, locator, and fingerprint metadata in that order. Selector-backed repair requires `--old-session` or `--new-session`. Ambiguous or unresolved repairs are left unchanged and reported as warnings. The JSON report includes `repaired[]` entries with `old_ref`, `new_ref`, `source`, `matched_by`, `node`, and selector-backed `live_node`.
Use `nxctl compare normalize-decisions --decisions-file <jsonl> --compare-json <file> --review-summary <file> --output <jsonl>` to canonicalize kind/confidence/match_kind values, materialize high-confidence `accepted_finding_cluster` and `regression_finding_cluster` entries into per-finding decisions, remove duplicate decisions, and catch stale refs or `finding_id` values before committing a reviewed decision file. `--review-summary` is enough to expand clusters from a manifest-level review packet; add `--compare-json` when you also want stale ref and finding ID validation.
Use `nxctl compare audit-decisions --decisions-file <jsonl> --compare-json <file> --json` to check whether reviewed decisions were applied, are still pending, became stale, or conflict with another decision. The JSON report includes `entries[]` with each decision state's `status`, `reason`, `expected`, `actual`, conflict line metadata, and repair hints for stale refs or finding IDs. Pair and subtree application audits require compare JSON produced with `--matching-debug`.
`accepted_removed` and `accepted_added` entries mark the corresponding missing/new finding as `info`; `accepted_finding` marks an existing finding by `finding_id` as approved `info`; `regression_finding`, `regression_removed`, and `unexpected_added` keep the finding explicit with `decision_kind` metadata.
Confidence is intentionally conservative in the current matcher: only `high` pair and subtree decisions become anchors, and finding decisions apply only when confidence is omitted or `high`. `tentative` and `unknown` stay in the JSONL as review evidence so later soft-match workflows can consume the same file format.

## Matching Debug

Use `--matching-debug` when collecting false positive or false negative examples. It keeps plain terminal output unchanged and adds `matching_debug` to JSON and markdown reports.

`matching_debug` includes:

- `mode`
- accepted `matches` with `matched_by`, score, and reasons
- selected low-occurrence `anchors`
- anchored `regions` with old/new original index ranges and match counts
- `ambiguous_candidates` with old node context, ranked new candidates, scores, shared keys, differing keys, and bounds
- `ambiguous_matches_skipped`
- `unmatched_old` and `unmatched_new`

## Node Scopes

- `current` is the default and preserves the existing compare candidate set
- `actionable` keeps control-oriented nodes such as buttons, links, inputs, tabs, options, and other interactive widgets
- `semantic` keeps actionable nodes plus named or content-bearing semantic nodes such as headings, landmarks, status, tables, images, and `data-testid` nodes
- use `semantic` with `--scope-selector` and `--match-mode stable` first; move to `heuristic` or experimental `histogram` only when stable keys are incomplete

JSON summaries include `matched_nodes`, `exact_matches`, `stable_matches`, `heuristic_matches`, `histogram_matches`, `decision_matches`, and `ambiguous_matches_skipped` when applicable.

## Failure Triage

If the new page looks incomplete:

1. Run `state` just before compare
2. Check whether the target content is really present
3. Strengthen readiness with `--wait-selector` or `--wait-function`
4. Narrow the compare scope before assuming there is a product bug

## Evidence Limits and Known False Positives

- A focused `--css-property` run proves only the requested properties; it says nothing about omitted properties
- The default `--compare-css` allowlist is intentionally stable and incomplete; use `--all-css-properties` only when exhaustive evidence is required
- If the old side lacks an accessible name or semantic role that exists on the new side, one logical control can appear as a `missing` and `new` pair instead of a changed match
- Responsive controls implemented with different old/new DOM structures can produce many `new` nodes even when the visible behavior is equivalent
- Use `--matching-debug`, a narrow scope, and screenshots together before classifying matching-heavy `missing` or `new` findings
- Canvas pixels can be compared visually, but canvas-internal objects do not become semantic compare nodes

## Scope Selector Rules

- `--scope-selector` accepts a raw CSS selector and restricts compare to exactly one matched subtree on each side
- `--old-scope-selector` and `--new-scope-selector` override the compare subtree per side
- if only one side-specific scope selector is set, `--scope-selector` must provide the missing side's fallback
- positional selectors such as `:nth-child()` and `:nth-of-type()` are allowed
- prefer stable ids, classes, or attributes before positional selectors
- if the selector matches 0 or multiple elements on either side, compare fails early
- if the selector matches multiple elements, the error includes up to five candidate hints to help refine the selector

## Inspect Selector Rules

- `inspect --selector` accepts a raw CSS selector and compares the computed styles for exactly one matched element on each side
- `inspect --scope-selector` limits semantic locator inspection to exactly one matched subtree on each side
- `inspect --old-scope-selector` and `inspect --new-scope-selector` override the inspect subtree per side
- when no semantic locator is provided, side-specific scope selectors identify the inspected roots
- positional selectors such as `:nth-child()` and `:nth-of-type()` are allowed
- do not combine `--selector` with a positional inspect locator
- do not combine `--selector` with `--nth`
- if an inspect selector or inspect scope selector matches multiple elements, the error includes up to five candidate hints
- use `inspect --layout-context` when ancestor layout CSS may explain the target element's size, position, wrapping, or overflow
- `--layout-context` returns DOM ancestor layout CSS from Chromium
