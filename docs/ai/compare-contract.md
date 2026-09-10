# Comparison Contract

[日本語](compare-contract_ja.md) | [Compare guide](compare.md)

## Compared values

Matching identifies corresponding nodes; it does not establish that their behavior is equal. Every matched pair compares normalized name, text, value, visibility, enabled/editable/selectable/invokable flags, and the following attributes and current states. Explicit `opaque_subtree` decisions continue to suppress internal findings.

| Data | Finding kind | Field |
| --- | --- | --- |
| Link destination, role, input type, placeholder | `attribute_changed` | `href`, `role`, `type`, `placeholder` |
| Native checkbox/radio checkedness | `state_changed` | `checked` |
| Native checkbox indeterminate state | `state_changed` | `indeterminate` |
| Observed native option selection | `state_changed` | `selected` |
| Selected option indices on a native select | `state_changed` | `selected_indices` |
| Explicit ARIA states | `state_changed` | The corresponding `aria-*` attribute |

`states` is an additive map in observations and compare snapshots. Native properties are read from the live DOM, so changing `.checked` is detected even if `value` and the HTML attribute do not change. `selected_indices` is a JSON array encoded as a string, such as `"[0,1]"`; this distinguishes multiple selections even when option values are identical. Native options still have to be collected by the chosen node scope and visibility rules to expose their own `selected` state.

The captured ARIA attributes are `aria-checked`, `aria-selected`, `aria-expanded`, `aria-pressed`, `aria-current`, `aria-invalid`, `aria-busy`, `aria-disabled`, `aria-readonly`, and `aria-required`. Only explicitly present attributes are captured. A missing map key differs from `"false"`, `"mixed"`, or an empty attribute; state findings display a missing side as `<absent>`. These are observed values, not computed accessibility-tree states.

The existing combined `state` field for the five boolean flags remains available. `summary.attribute_changed` counts attribute findings, and `summary.state_changed` also includes individual current-state findings. `exact` can still report missing/new nodes when identity changes; use `stable` with durable IDs or test IDs to retain a pair across role, text, or destination changes.

`semantic` collects every `actionable` candidate before adding semantic content. All scopes continue to apply visibility and scope boundaries.

## Filters

Selectors are evaluated against the original observed nodes. An ignore selector removes the selected node and its observed descendants; a mask selector keeps the subtree's structure and state while clearing its name, text, value, placeholder, and ARIA label. Use a selector that resolves to the intended subtree on both sides.

Ignore-text regular expressions normalize compared text and identity attributes before comparison fingerprints are derived. With explicit filters or default `all` exclusions, snapshots use `filtered:v2:` fingerprints derived from the filtered identifying values. Matching debug, findings, and decisions use those same comparison fingerprints. Browser observations and live `@eN` references are unchanged. Recreate fingerprint-pinned decisions when changing filters or upgrading from raw comparison fingerprints.

The normalized text of masked/ignored descendants is removed from ancestor name/text and from the page or scoped text aggregate, longest contributions first. Native input values are not removed from unrelated prose. The aggregate has no DOM source ranges: every occurrence of the same normalized text is removed there, including occurrences elsewhere or inside a longer string. Unselected sibling nodes retain their own text and can still produce node findings. For repeated content, use a narrow scope and inspect those node findings.

These filters suppress comparison content; they do not sanitize every artifact. Page titles, URLs, identity attributes such as IDs/test IDs/hrefs, CSS, screenshots, user-supplied decisions, and logs are not redacted by a node mask. Text referenced through labels outside the selected subtree can also remain. Review artifacts before sharing them and remove sensitive data at its source when full redaction is required.

## Finding IDs and saved reports

New reports contain `finding_id_version: 2` and IDs shaped like `text_changed:v2:<24 hexadecimal digits>`. The hash uses the page URL pair, scope, individual node occurrences, and full normalized change values, with unambiguous field encoding. Repeated identical controls receive distinct IDs. A changed suffix beyond a human-readable preview changes the ID. Session IDs are excluded; identical page/scope/node inputs reproduce the same IDs.

JSON findings preserve the full normalized `old` and `new` values. Text and Markdown previews are limited to 120 Unicode characters. `old_ref` and `new_ref` identify the applicable node occurrences and are used for decision audits and screenshot crops.

Finding decisions from the previous ID format are not automatically migrated or applied to new IDs. Generate a new finding decision template, review it, and rerun validation/audit against the new compare JSON. Decision JSONL `schema_version` remains `1`; it is independent of the finding-ID version. Old saved reports can still be read and audited with their own decisions. Copy IDs from generated artifacts instead of constructing them from examples.

Snapshots now serialize `original_index` and `structure_path`, as well as `states`, `type`, and `placeholder` when available. Nodes may be stored in fingerprint order; subtree matching and audit use original DOM order. Older reports without `original_index` fall back to positive observation IDs, which Chromium assigns in DOM order. Imported reports without either ordering field cannot provide that guarantee.

Manual pairs remain one-to-one in all matching modes. Crossing manual pairs remain accepted matches, but histogram uses a noncrossing subset as region boundaries so one node cannot be reused in two regions.

## Runtime behavior

Wait timeouts cover each CDP evaluation and the polling interval, including unresolved promises. An earlier caller deadline or cancellation takes precedence. Cancellation ends the wait request; it does not promise to terminate arbitrary JavaScript already running in the page.

Session startup reserves its ID while browser attachment runs outside the manager lock. Operations on other sessions can continue. Shutdown cancels pending startups, attempts every session cleanup, and aggregates failures. Failed cleanup stays registered for a later retry, including profile-directory removal after a browser exits.

The daemon build epoch is advanced for the observation and runtime changes. The additive RPC fields retain protocol version `v1`; update `nxctl` and `nxd` together.
