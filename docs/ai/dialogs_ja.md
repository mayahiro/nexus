# JavaScript ダイアログ

[English guide](dialogs.md)

Chromium backend では、選択した session の browser-native な `alert`、`confirm`、`prompt`、`beforeunload` を確認、操作できる
これらのダイアログは DOM の外にあり、`@eN` ref を持たない
ファイル選択、権限許可、OS のダイアログは、このコマンドの対象外

## コマンド

```text
nxctl dialog get --session work
nxctl dialog get --session work --json
nxctl dialog accept --session work
nxctl dialog accept --text "Alice" --session work
nxctl dialog accept --text "" --session work
nxctl dialog dismiss --session work
```

3つの subcommand はすべて `--session` と `--json` に対応し、既定の session は `default`
インストールした版の引数は `nxctl help dialog` または `nxctl help dialog accept` で確認する

- `get` はページの JavaScript を実行せずに現在のダイアログを返す。開いていない場合も成功し、JSON の `value.open` は `false` になる。今後開くダイアログは待たない
- `accept` は alert を閉じ、confirm では `true` を返し、prompt の値を送信し、`beforeunload` では保留中のページ遷移を許可する
- `accept --text` は prompt 専用。省略すると初期値を保持し、`--text ""` は空文字を送信する。空白と Unicode は保持する
- `dismiss` は alert を閉じ、confirm では `false`、prompt では `null` を返し、`beforeunload` では保留中のページ遷移をキャンセルする
- `accept` と `dismiss` はダイアログが開いていなければ失敗する。現在のダイアログを1回処理し、今後のダイアログに対する自動応答は設定しない

## ダイアログで停止した操作を続ける

```text
nxctl click @e3 --session work
nxctl dialog get --session work
nxctl dialog accept --session work
nxctl state --session work
```

`click`、`eval`、ページ遷移などの実行中にダイアログが開くと、Nexus は応答待ちを終了し、ダイアログの種類、本文、処理コマンドを示して非ゼロの終了コードを返す
ページ観測と screenshot も、ダイアログが開いている間は速やかに失敗する
session は接続を維持するため、次のコマンドでダイアログを処理できる

元の操作は一部が実行済みの場合がある
ダイアログを処理するとページの script や遷移は継続するが、元の CLI コマンドは再開せず、取得できなかった `eval` の結果も返さない
ページの状態を確認してから次の操作を判断し、click、フォーム送信、ページ遷移をそのまま繰り返さないこと
ダイアログ処理後も非同期の更新が続く場合は `wait` を使う

ダイアログで停止した操作は、従来の一般的な timeout から明示的なエラーへ変わる
通常の操作の出力は維持する
既定の `batch` はこのエラーで停止するため、個別のコマンドでダイアログを確認して応答を判断できる
閉じた直後に次のダイアログが開く場合もあるため、必要に応じて再度 `dialog get` で確認する

## flow 内での操作

flow のステップに `{"action":"dialog","target":"get|accept|dismiss"}` を指定し、target を1つ選ぶ
`side` は `old`、`new`、`both` に対応し、既定は `both`
`text` は prompt に応答する `target: "accept"` でのみ指定する
省略時は初期値を保持し、`"text": ""` は空文字を送信する
ダイアログの text は変数展開に対応し、空白と Unicode を保持する

ダイアログを開く `click`、`fill`、`navigate` ステップに `"expect_dialog": true` を指定する
この指定では、新しいダイアログが開くまでステップを完了せず、action の CDP 応答後に非同期で開く場合も待機する
すでに開いているダイアログ、無関係な action エラー、待機期限の超過は失敗として扱う
指定がない場合のエラー処理は従来どおり

既存の両 session で prompt を処理する例

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

キャンセルする場合は `accept` を `dismiss` に置き換え、confirm や alert では `text` を省略する
expect_dialog ステップと dialog ステップは正のミリ秒値の `timeout` に対応し、既定は各 side で 30000ms
expect_dialog ステップでは locator の解決、action、ダイアログの待機を合わせた上限になる
get ステップは現在の状態をすぐ返し、今後開くダイアログは待たない

`both` の各ステップは old、new の順に実行する
expect_dialog ステップの完了は、ダイアログが開いたことを示し、ページの action は停止中の場合がある
次のページ観測や操作の前にダイアログの処理ステップを置き、`wait` で結果のページ状態を確認する
想定したダイアログのために `continue_on_error` を指定する必要はなく、実際の失敗には従来どおり適用される
後の side が失敗した場合も、それまでの side の結果はレポートに保持する

JSON のステップレポートに `dialogs.old` または `dialogs.new` を追加し、それぞれに action result を保持する
expect_dialog の結果は `dialog`、get の結果は `value` にダイアログの状態を格納する
応答操作の結果は `value.type` と `value.accepted` を格納する
text レポートにも side ごとの結果メッセージを出力する
既存 manifest と通常ステップの出力の挙動は維持する

新しい flow session の読み込み中に出るダイアログには、endpoint の `url: "about:blank"`、dialog / get ステップ、`expect_dialog: true` 付き navigate ステップの順を使う
`beforeunload` には引き続き Chrome の user activation の条件が適用される
flow の locator click は JavaScript 経由で実行するため、その activation を満たさない
離脱確認を想定する flow では、事前に `nxctl click <X> <Y> --session <ID>` などの実際のユーザー操作を受けた session を再利用する

## ページ読み込み時のダイアログ

ダイアログの監視には session の CDP 接続のイベントを使い、接続は最初のページ操作で開始する
初回読み込み時のダイアログを捕捉するには、空のページで接続を確立してから遷移する

```text
nxctl open about:blank --session work
nxctl dialog get --session work
nxctl navigate https://example.com --session work
nxctl dialog get --session work
```

`beforeunload` は [user activation](https://developer.mozilla.org/en-US/docs/Web/API/Window/beforeunload_event#usage_notes) など Chrome の表示条件を満たす場合だけ開く
検証時は座標 click など、実際のユーザー操作を行う
Nexus は Chrome が抑制したダイアログを強制表示しない

## JSON 出力

`dialog get --json` は共通の action-result envelope を返す
prompt が開いている場合の例

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

ダイアログの状態に含まれる空文字の項目は省略する
`url` はダイアログを開いた frame を示す
`accept` と `dismiss` が成功すると `changed` は `true` になり、`value.type` は処理したダイアログの種類、`value.accepted` は応答を表す
この応答は、その後のページ処理の完了や、次のダイアログが存在しないことを保証しない

backend は Chrome DevTools Protocol の [`Page.javascriptDialogOpening`、`Page.javascriptDialogClosed`、`Page.handleJavaScriptDialog`](https://github.com/ChromeDevTools/devtools-protocol/blob/master/pdl/domains/Page.pdl) を使用する
