# Inspect Guide

Use `inspect` when one element needs a focused computed-style explanation. It supports one-session inspection and old/new session comparison

Japanese version: [docs/ai/inspect_ja.md](inspect_ja.md)

## One-Session Inspection

```text
nxctl inspect 'role button --name "Submit"' --session work
nxctl inspect 'role link --name "Handbooks"' --session work --css-property width --json
nxctl inspect --selector '.link-list' --session work --css-property width
nxctl inspect 'role button' --session work --nth 2 --layout-context
```

Without `--css-property`, `inspect` uses the focused default list: `color`, `background-color`, `font-size`, `font-weight`, `line-height`, `display`, `visibility`, `opacity`, and `pointer-events`

## Old/New Comparison

```text
nxctl inspect 'role button --name "Submit"' --old-session old --new-session new
nxctl inspect 'role button' --old-session old --new-session new --nth 2 --css-property color
nxctl inspect --selector 'aside.filters' --old-session old --new-session new --css-property width
nxctl inspect --old-scope-selector '#legacy-filters' --new-scope-selector 'aside.filters' --old-session old --new-session new --css-property width
```

Use `--session` for one-session mode, or both `--old-session` and `--new-session` for comparison mode. Do not combine the two modes

## Matched Declarations and Source Locations

Style sources are collected by default. Chromium is asked for matched styles only for the selected node and requested properties. The result can include direct declarations, shorthand declarations that expand to a requested longhand, inline styles, presentation attributes, and declarations reported for ancestors

The text output stays compact. It prints a declaration directly when there is one and a count when there are several. JSON preserves every collected declaration in `properties[].declarations` for one-session mode and `properties[].old_declarations` or `properties[].new_declarations` for comparison mode

Declaration metadata includes the authored property and value, matching selector information, origin, relation, flags such as `important`, `disabled`, `inline`, or `inherited`, and a best-effort source location. `line` and `column` are one-based and refer to the CSS resource delivered to Chromium. `source_map_url` is reported when Chromium exposes one, but Nexus does not resolve source maps to an original SCSS file

The declaration list does not identify or claim the cascade winner. Do not infer the winner from array order alone. Use the computed value together with declaration metadata, inheritance, importance, specificity, rule order, and application state when further analysis is required

The current declaration collector does not interpret pseudo-element styles, active transitions, animation values, or source maps. Their effects can still appear in the computed value

## Source Collection Status

The selected session object contains `style_sources_status`

- `complete`: matched declaration collection completed with available source metadata
- `partial`: declarations were collected, but source metadata was missing for at least one stylesheet-backed declaration
- `unavailable`: source collection failed; computed values from the initial observation remain available
- `disabled`: `--no-style-sources` was used and source collection was skipped

`style_sources_error` contains a diagnostic when collection is unavailable. A source collection failure does not fail the whole `inspect` command after the target and computed styles were observed

Use `--no-style-sources` when matched declarations are unnecessary or their cost is undesirable

```text
nxctl inspect --selector '.link-list' --session work --css-property width --no-style-sources
```

## Target and Readiness Rules

- semantic locators support `@eN`, `role`, `text`, `label`, `testid`, and `href`
- `--selector` selects exactly one CSS-matched element and cannot be combined with a positional locator or `--nth`
- `--scope-selector` either scopes a semantic locator or identifies the inspected root when no locator is supplied
- `--old-scope-selector` and `--new-scope-selector` are comparison-only
- `--layout-context` adds a focused set of computed layout properties for DOM ancestors
- use `wait hydrated` or an application-specific `wait function` before `inspect` when rendering readiness matters

Nexus first observes and selects the node, then validates its recent ref before targeted style collection. If a rerender changes the node identity between those operations, declaration collection becomes unavailable while the already observed computed values remain in the report
