# Inspect ガイド

`inspect` は1要素の computed style と指定元を集中的に確認するためのコマンドです。単一セッションの取得と old/new セッションの比較に対応します

English version: [docs/ai/inspect.md](inspect.md)

## 単一セッションの取得

```text
nxctl inspect 'role button --name "Submit"' --session work
nxctl inspect 'role link --name "Handbooks"' --session work --css-property width --json
nxctl inspect --selector '.link-list' --session work --css-property width
nxctl inspect 'role button' --session work --nth 2 --layout-context
```

`--css-property` を指定しない場合は `color`、`background-color`、`font-size`、`font-weight`、`line-height`、`display`、`visibility`、`opacity`、`pointer-events` を取得します

## old/new 比較

```text
nxctl inspect 'role button --name "Submit"' --old-session old --new-session new
nxctl inspect 'role button' --old-session old --new-session new --nth 2 --css-property color
nxctl inspect --selector 'aside.filters' --old-session old --new-session new --css-property width
nxctl inspect --old-scope-selector '#legacy-filters' --new-scope-selector 'aside.filters' --old-session old --new-session new --css-property width
```

単一セッションでは `--session`、比較では `--old-session` と `--new-session` の両方を使います。2つのモードは併用できません

## matched declaration と配信元

style source はデフォルトで取得されます。Chromium への matched style 取得は、選択した1要素と要求されたプロパティだけを対象にします。結果には直接指定、要求した longhand に展開される shorthand、inline style、presentation attribute、ancestor で報告された宣言が含まれます

text 出力は簡潔に保たれ、宣言が1件なら内容、複数なら件数を表示します。JSON は単一セッションでは `properties[].declarations`、比較では `properties[].old_declarations` と `properties[].new_declarations` に取得した全宣言を保持します

宣言には authored property と値、matching selector、origin、relation、`important`、`disabled`、`inline`、`inherited` などの flag、取得できた配信元を含みます。`line` と `column` は1始まりで、Chromium に配信された CSS resource 上の位置です。Chromium が `source_map_url` を返す場合は保持しますが、Nexus 自体は source map を元の SCSS まで解決しません

宣言一覧は cascade winner を断定しません。配列順だけから winner を推測せず、computed value と宣言 metadata、inheritance、importance、specificity、rule order、application state を組み合わせて判断してください

現在の declaration collector は pseudo-element style、active transition、animation value、source map を解釈しません。それらの効果が computed value に現れる場合はあります

## 取得状態

対象セッションには `style_sources_status` が入ります

- `complete`: matched declaration の取得が完了し、利用可能な配信元 metadata も取得できた状態
- `partial`: 宣言は取得できたものの、一部の stylesheet-backed declaration で配信元 metadata が不足した状態
- `unavailable`: 配信元の取得に失敗した状態で、最初の observation から得た computed value は維持される
- `disabled`: `--no-style-sources` により取得を省略した状態

取得不能時の診断は `style_sources_error` に入ります。対象と computed style の observation が済んでいれば、style source の失敗だけでは `inspect` 全体を失敗させません

matched declaration が不要な場合や取得コストを避けたい場合は `--no-style-sources` を使います

```text
nxctl inspect --selector '.link-list' --session work --css-property width --no-style-sources
```

## 対象と readiness のルール

- semantic locator は `@eN`、`role`、`text`、`label`、`testid`、`href` に対応
- `--selector` は CSS で一意に選択した1要素を対象とし、positional locator や `--nth` とは併用不可
- `--scope-selector` は semantic locator の探索範囲になり、locator がなければ scope root 自体が対象
- `--old-scope-selector` と `--new-scope-selector` は比較専用
- `--layout-context` は DOM ancestor の layout 向け computed property を追加
- rendering readiness が重要な場合は事前に `wait hydrated` または application 固有の `wait function` を実行

Nexus は最初に対象を observe し、その recent ref を検証してから targeted style collection を行います。2つの操作の間に rerender で要素 identity が変わった場合、宣言取得は `unavailable` となり、observe 済みの computed value は report に残ります
