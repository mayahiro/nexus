# JavaScript dialogs

[Japanese guide](dialogs_ja.md)

The Chromium backend can inspect and handle browser-native `alert`, `confirm`, `prompt`, and `beforeunload` dialogs in the selected session. These dialogs are outside the DOM and do not have `@eN` refs. File choosers, permission prompts, and operating-system dialogs are outside this command's scope.

## Commands

```text
nxctl dialog get --session work
nxctl dialog get --session work --json
nxctl dialog accept --session work
nxctl dialog accept --text "Alice" --session work
nxctl dialog accept --text "" --session work
nxctl dialog dismiss --session work
```

All three subcommands accept `--session` and `--json`. The session defaults to `default`. Use `nxctl help dialog` or `nxctl help dialog accept` to confirm the installed command's arguments.

- `get` reports the current dialog without running page JavaScript. No open dialog is a successful result, with `value.open: false` in JSON. It does not wait for a future dialog
- `accept` acknowledges an alert, returns `true` from a confirm, submits a prompt, or allows a pending `beforeunload` navigation
- `accept --text` is valid only for a prompt. Omitting `--text` preserves its initial value; `--text ""` explicitly submits an empty string. Whitespace and Unicode are preserved
- `dismiss` closes an alert, returns `false` from a confirm, returns `null` from a prompt, or cancels a pending `beforeunload` navigation
- `accept` and `dismiss` fail when no dialog is open. They handle the current dialog once and do not install an automatic policy for future dialogs

## Continue a blocked operation

```text
nxctl click @e3 --session work
nxctl dialog get --session work
nxctl dialog accept --session work
nxctl state --session work
```

When a dialog opens during `click`, `eval`, navigation, or another page operation, Nexus stops waiting and returns a non-zero exit status with the dialog type, message, and handling commands. Page observation and screenshots also fail promptly while a dialog is open. The session stays attached so the next command can handle it.

The triggering operation may have partially executed. Handling the dialog lets the page script or navigation continue, but does not resume the original CLI command or return its lost `eval` result. Inspect the resulting page before deciding whether another action is needed. Do not blindly repeat a click, form submission, or navigation. Use `wait` after handling when the page continues updating asynchronously.

This changes dialog-blocked operations from generic timeouts to explicit errors. Normal operations retain their existing output. A default `batch` stops on that error; separate commands allow the agent to inspect the dialog and decide how to respond. A script can open another dialog immediately after one closes, so check `dialog get` again when needed.

## In flows

Use `{"action":"dialog","target":"get|accept|dismiss"}` as a flow step, choosing one target. `side` can be `old`, `new`, or `both` (the default). Set `text` only with `target: "accept"` for a prompt; omitting it preserves the initial value, while `"text": ""` sends an empty string. Dialog text supports variable substitution and preserves whitespace and Unicode.

Add `"expect_dialog": true` to the `click`, `fill`, or `navigate` step that opens the dialog. This opt-in requires a new dialog to open before the step can complete, including dialogs opened asynchronously after the action's CDP response. An already-open dialog, an unrelated action error, or expiry of the wait remains a failure. Without this field, the existing error behavior is unchanged.

For example, this scenario handles a prompt on both existing sessions:

```json
{
  "scenarios": [{
    "name": "name-prompt",
    "old": { "session": "old" },
    "new": { "session": "new" },
    "steps": [
      { "action": "click", "locator": "testid=rename", "expect_dialog": true, "timeout": 5000 },
      { "action": "dialog", "target": "get" },
      { "action": "dialog", "target": "accept", "text": "Alice" },
      { "action": "wait", "target": "text", "value": "Alice" },
      { "action": "compare" }
    ]
  }]
}
```

Replace `accept` with `dismiss` to cancel, and omit `text` for a confirm or alert. Expected-dialog steps and dialog steps accept a positive `timeout` in milliseconds, defaulting to 30000 per side. For expected-dialog steps this bounds locator resolution, the action, and waiting for the dialog. A `get` step reports the current state immediately; it does not wait for a future dialog.

The flow handles `old` and then `new` in each `both` step. A completed expected-dialog step means the dialog appeared; the page action may still be suspended. Place the handling step before further page observation or interaction, then use `wait` to verify the resulting page. `continue_on_error` is unnecessary for an expected dialog and still applies to actual failures as before. If a later side fails, earlier side results remain in the report.

JSON step reports add `dialogs.old` and/or `dialogs.new`, each containing an action result. Expected-dialog results expose the open state in `dialog`; `get` exposes it in `value`; handling results expose `value.type` and `value.accepted`. Text reports include the per-side result messages. Existing manifests and ordinary step output keep their current behavior.

For a dialog during page load in a fresh flow session, use endpoint `url: "about:blank"`, a `dialog` / `get` step, then a `navigate` step with `expect_dialog: true`.

`beforeunload` still depends on Chrome's user-activation conditions. Flow's locator clicks run through JavaScript and do not establish that activation. Reuse sessions that have received a real user interaction, such as `nxctl click <X> <Y> --session <ID>`, before running a flow that expects a leave confirmation.

## Dialogs during page load

Dialog tracking uses events from the session's CDP connection, which starts with the first page operation. To capture dialogs that appear during initial page load, establish the connection on a blank page before navigation:

```text
nxctl open about:blank --session work
nxctl dialog get --session work
nxctl navigate https://example.com --session work
nxctl dialog get --session work
```

`beforeunload` appears only when Chrome permits it, including its [user-activation requirements](https://developer.mozilla.org/en-US/docs/Web/API/Window/beforeunload_event#usage_notes). Use an actual user interaction such as a coordinate click when testing this behavior. Nexus does not force Chrome to show a suppressed dialog.

## JSON output

`dialog get --json` returns the standard action-result envelope. An open prompt looks like:

```json
{
  "ok": true,
  "changed": false,
  "message": "prompt dialog: \"Name?\" (default: \"initial\")",
  "value": {
    "open": true,
    "type": "prompt",
    "message": "Name?",
    "url": "https://example.com/",
    "default_prompt": "initial"
  }
}
```

Empty strings in the dialog state are omitted. `url` identifies the frame that opened the dialog. Successful `accept` and `dismiss` results set `changed: true`; `value.type` identifies the handled dialog and `value.accepted` reports the response. This response does not assert that subsequent page work has completed or that another dialog is absent.

The backend uses Chrome DevTools Protocol's [`Page.javascriptDialogOpening`, `Page.javascriptDialogClosed`, and `Page.handleJavaScriptDialog`](https://github.com/ChromeDevTools/devtools-protocol/blob/master/pdl/domains/Page.pdl).
