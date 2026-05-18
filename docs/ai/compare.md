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
- migration-scoped passes using `--old-scope-selector` and `--new-scope-selector` when old and new DOM structures differ
- migration-friendly matching with `--match-mode stable`, `--match-mode heuristic`, or experimental `--match-mode histogram`
- broader semantic candidate collection with `--node-scope semantic`
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
nxctl compare validate-decisions --decisions-file pair-decisions.jsonl --compare-json compare-debug.json
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
When `--node-scope all` is used, compare requires `--scope-selector` or both side-specific scope selectors. All visible elements inside that scope are observed. Compare adds `structure_key` and `subtree_signature` to nodes, matching debug entries, and missing/new findings; histogram can use these as low-occurrence anchors without changing the base fingerprint.

## Pair Decisions

Use `--decisions-file <jsonl>` when an AI or human has reviewed ambiguous candidates and wants compare to reuse high-confidence pairings.

Each line is one JSON object. Validate each line against `docs/ai/compare-decisions.schema.json`. Compare applies high-confidence `pair` and `subtree_pair` entries before automatic matching:

```jsonl
{"kind":"pair","old":"@e203","new":"@e222","confidence":"high","reason":"bbox and role/name match; aria-label changed as an a11y improvement"}
{"kind":"subtree_pair","old":"@e40","new":"@e72","confidence":"high","match_kind":"ordered_children","count":12,"reason":"same link list region"}
{"kind":"pair","old":"@e9","new":"?","confidence":"unknown","reason":"needs review"}
{"kind":"accepted_removed","old":"@e45","reason":"legacy-only footer link intentionally removed"}
{"kind":"accepted_added","new":"@e88","reason":"new skip-link"}
{"kind":"accepted_finding","finding_id":"text_changed:3fa21c9d4b2a","reason":"approved copy change"}
{"kind":"regression_finding","finding_id":"layout_changed:4d2aa4107e9f","reason":"primary CTA moved below the fold"}
```

`old_fingerprint` and `new_fingerprint` may be included to detect stale refs. Non-high entries are accepted as review notes but are not used as anchors or matches.
`subtree_pair` currently supports `match_kind:"ordered_children"` for ref/fingerprint-based roots; it pairs the roots, then pairs observed direct children in DOM order. `count` is optional and validates the expected number of child pairs when present.

Use `--output-decisions-template <jsonl>` with `--matching-debug` to write editable `unknown` pair stubs from `matching_debug.ambiguous_candidates`.
Use `--output-finding-decisions-template <jsonl>` to write editable `unknown`-confidence stubs for current `critical` and `warning` findings. Review those lines, then set `confidence:"high"` or remove `confidence` to apply the `accepted_finding` or `regression_finding` decision on the next compare run.
Use `--review-dir <dir>` to write `compare.json`, `compare.md`, `pair-decisions.todo.jsonl`, `finding-decisions.todo.jsonl`, `old.png`, `new.png`, and `review-summary.json` in one pass. Screenshot capture is best-effort; unsupported backends keep the packet and record warnings in `review-summary.json`. It enables matching debug automatically so pair decision review has the needed context.
With `--manifest`, `--review-dir <dir>` writes `manifest.json`, `manifest.md`, a root `review-summary.json`, and `review-index.md`, then writes one page review packet directory per manifest page. Start with `review-index.md` to choose high-priority pages before opening each page packet.
Use `nxctl compare validate-decisions --decisions-file <jsonl> --compare-json <file>` to validate JSONL structure, supported kind names, duplicate high-confidence pairs/subtrees/finding decisions, and current refs/fingerprints/finding IDs before rerunning compare.
Use `nxctl compare normalize-decisions --decisions-file <jsonl> --compare-json <file> --output <jsonl>` to canonicalize kind/confidence/match_kind values, remove duplicate decisions, and catch stale refs or `finding_id` values before committing a reviewed decision file.
Use `nxctl compare audit-decisions --decisions-file <jsonl> --compare-json <file>` to check whether reviewed decisions were applied, are still pending, became stale, or conflict with another decision. Pair and subtree application audits require compare JSON produced with `--matching-debug`.
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
- `--layout-context` is capability-based; Chromium returns DOM ancestor layout CSS, and unsupported backends fail explicitly
