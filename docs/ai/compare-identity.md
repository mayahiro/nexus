# AI Element Identity Suggestions

[日本語](compare-identity_ja.md) | [Compare guide](compare.md)

`nxctl compare suggest-decisions` experimentally uses TypeSafe AI's Jev to suggest corresponding elements in a saved, single-page compare report. It examines unmatched nodes and writes the existing [decision JSONL format](compare-decisions.schema.json). Ordinary `compare` remains local and does not call an AI service.

Correspondence identifies the same logical UI occurrence, such as the save control for a particular order. It does not establish equal behavior or approve a change. Matched pairs still produce text, attribute, state, CSS, and layout findings under the [comparison contract](compare-contract.md).

## Workflow

Capture a narrow comparison with matching debug:

```text
nxctl compare https://old.example.com/orders https://new.example.com/orders --node-scope semantic --scope-selector main --match-mode stable --matching-debug --output-json compare-debug.json
```

Preview the exact outgoing request bodies without an API key, remote calls, or a daemon:

```text
nxctl compare suggest-decisions --compare-json compare-debug.json --dry-run
```

The JSON preview contains `requests[]`, with one request for each considered old node that has eligible new candidates. Review the page text and identity evidence before sending it. Use `--output-json requests.json` to save this preview to a new file.

Set `TYPESAFE_API_KEY` in your environment, then explicitly invoke remote evaluation:

```text
nxctl compare suggest-decisions --compare-json compare-debug.json --output pair-suggestions.jsonl
```

This invocation sends the bounded context to `https://api.typesafe.ai/v1/systemone`. No alternate endpoint is accepted. The default model is pinned to `jev-1.13.0`; `--model` can select a different Jev version or an official alias. See the [TypeSafe API reference](https://docs.typesafe.ai/api) and [model specifications](https://docs.typesafe.ai/models).

By default, proposed pairs have `confidence:"tentative"` and do not affect matching. Review the paired occurrences and the review report, and change only approved pairs to `confidence:"high"`. Validate the reviewed file against the captured comparison:

```text
nxctl compare validate-decisions --decisions-file pair-suggestions.jsonl --compare-json compare-debug.json --strict
nxctl compare https://old.example.com/orders https://new.example.com/orders --node-scope semantic --scope-selector main --match-mode stable --decisions-file pair-suggestions.jsonl
```

Keep the capture scope and filters consistent. Generated decisions include both refs and fingerprints, so a changed observation is rejected rather than silently applied; use the existing repair workflow and review the result when refs become stale.

## Optional Promotion

After evaluating the model on your own pages, `--promote` can emit `high` pairs:

```text
nxctl compare suggest-decisions --compare-json compare-debug.json --output pair-promoted.jsonl --promote --min-probability 0.98 --min-margin 0.30
```

Promotion requires all of the following:

- The selected answer is a supplied candidate, rather than `none` or `unknown`
- Its Choice probability meets `--min-probability`
- An independent Noul question's same-element probability meets the same threshold
- Its probability exceeds the runner-up by at least `--min-margin`
- No other considered old node proposes the same new occurrence

The default thresholds are `0.95` and `0.20`. They are experimental defaults, not a measured error guarantee. Jev's returned `confidence` is recorded but is not directly mapped to Nexus's `high`. Failed probability gates leave a candidate tentative. `none`, `unknown`, and conflicting proposals leave an unresolved `pair` with `new:"?"`; they never create accepted removals, additions, findings, or opaque subtrees.

## Data and Limits

The request includes role, name, label, text, test ID, input type, placeholder, and the path portion of href. It includes up to three observed ancestors and two preceding and two following observed nodes in DOM order. Each string is limited to 240 Unicode characters, with `...` appended when truncated. Missing observation context is not reconstructed from a live page.

Form values, whole-page text, titles, page URLs, href credentials/hosts/queries/fragments, fingerprints, CSS, screenshots, and local refs are not sent as dedicated fields. Text, labels, test IDs, and href paths can still contain sensitive information, including text aggregated by an ancestor. This is an allowlist, not full redaction; inspect dry-run payloads and the [filter limits](compare-contract.md#filters).

Candidates are ranked from unmatched nodes using available role, text/name, label, test ID, href path, order, and bounds evidence. Defaults consider up to 20 old nodes and five new candidates per old node. `--limit` accepts 1–200 and `--max-candidates` accepts 1–20. Nodes without refs are skipped. The report counts old nodes omitted by missing refs or the limit; it does not claim exhaustive candidate coverage.

The input report is limited to 64 MiB and each request body to 32 KiB. A manifest-level report is not supported; use individual page reports. Existing matched pairs are not reconsidered. The model cannot recover a true counterpart excluded by observation or candidate retrieval. Current Jev accepts text rather than screenshots; this command does not implement OpenAI's Decisions API.

`--timeout` bounds the complete evaluation, including retries, and defaults to 30000 milliseconds. Requests run sequentially. HTTP 429 and 529 can be retried twice with exponential backoff, honoring `Retry-After`; other errors fail the command. Provider error bodies are not printed, and HTTP redirects are not followed.

## Output and Evaluation

A successful remote run writes the decisions and `<output>.review.json`, or the review path selected by `--output-json`. Both files use mode `0600` and must be new paths in existing directories. Existing files are never overwritten. A request failure or cancellation does not write output; a failed file write removes files newly created by that run. `--json` also prints the review report.

The review report has `schema_version:1`, provider and requested model, question version, compare-file SHA-256, thresholds, summary counts, and `judgments[]`. Each judgment records candidate refs/fingerprints, the exact request SHA-256, the returned model, probability distributions, confidence, Noul answers, token usage, promotion evidence, and its decision. Normal evaluation reports omit request bodies; only dry-run reports contain `requests[]`. These local artifacts can contain page-derived fingerprints and must be reviewed before sharing.

Evaluate false pairings, missed pairings, unresolved results, and retained regression findings on labeled page pairs before promoting suggestions. Include repeated controls in different regions or records, renamed labels, a11y changes, changed link destinations, and Japanese text. Measure end-to-end time and token usage on your workload. TypeSafe documents [limitations](https://docs.typesafe.ai/model-jaggedness/jev-1.13), including adversarial content and independent questions that need not satisfy structural invariants.
