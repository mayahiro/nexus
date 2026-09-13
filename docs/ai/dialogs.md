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
