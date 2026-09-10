# 比較の仕様

[English](compare-contract.md) | [Compare guide](compare.md)

## 比較する値

Matching はノードの対応を決める処理であり、挙動が同じことの証明ではない
対応したノードでは、正規化した name、text、value、visible、enabled/editable/selectable/invokable と、以下の属性・現在の状態を比較する
明示的な `opaque_subtree` decision による内部 finding の抑制は継続する

| データ | Finding kind | Field |
| --- | --- | --- |
| リンク先、role、input type、placeholder | `attribute_changed` | `href`、`role`、`type`、`placeholder` |
| Native checkbox/radio の checkedness | `state_changed` | `checked` |
| Native checkbox の indeterminate 状態 | `state_changed` | `indeterminate` |
| 観測した native option の選択状態 | `state_changed` | `selected` |
| Native select で選択中の option index | `state_changed` | `selected_indices` |
| 明示された ARIA state | `state_changed` | 対応する `aria-*` 属性 |

Observation と compare snapshot に `states` map を追加する
Native property は現在の DOM から取得するため、value や HTML 属性を変えずに `.checked` を変えた場合も検出する
`selected_indices` は `"[0,1]"` のように JSON array を文字列化した値で、option の value が同じ場合の複数選択も区別する
Native option 自体の `selected` を取得するには、選択した node scope と可視性の条件で option が収集される必要がある

取得する ARIA 属性は `aria-checked`、`aria-selected`、`aria-expanded`、`aria-pressed`、`aria-current`、`aria-invalid`、`aria-busy`、`aria-disabled`、`aria-readonly`、`aria-required`
明示された属性だけを保持し、map key の欠落と `"false"`、`"mixed"`、空の属性値を区別する
State finding では欠落した側を `<absent>` と表示する
これらは観測した値であり、accessibility tree の計算済み状態ではない

既存の5種類の bool をまとめた `state` field は継続する
`summary.attribute_changed` は属性 finding を数え、`summary.state_changed` は個別の現在状態も数える
Identity が変わると `exact` は missing/new を返す場合があるため、role、text、リンク先の変更を同じ pair として扱いたい場合は durable な ID/test ID と `stable` を使う

`semantic` は `actionable` の全候補に semantic content を追加する
どの scope でも可視性と scope の境界を適用する

## Filter

Selector は元の観測ノードに対して評価する
Ignore selector は対象ノードと観測済みの子孫を除外する
Mask selector は subtree の構造と状態を保ち、name、text、value、placeholder、ARIA label を空にする
左右で意図した subtree に一致する selector を使う

Ignore-text regex は比較用 fingerprint の生成前に、比較するテキストと identity 属性を正規化する
明示的な filter または既定の `all` 除外がある場合、snapshot は処理済みの識別情報から `filtered:v2:` fingerprint を生成する
Matching debug、finding、decision も同じ比較用 fingerprint を使う
Browser observation と live `@eN` ref は変更しない
Filter を変えた場合や生の比較用 fingerprint から更新する場合は、fingerprint を指定した decision を作り直す

Mask/ignore した子孫の正規化 text は、長い文字列から順に ancestor の name/text とページまたは scoped text 全体から除去する
Native input の value を無関係な本文から除去することはしない
本文全体には DOM の出所を示す範囲情報がないため、同じ正規化 text が別の場所や長い文字列の一部に含まれる場合も、その出現箇所をすべて除去する
選択していない sibling node の text は保持し、node finding として比較できる
同じ内容が繰り返される場合は scope を狭め、個別の node finding も確認する

これらの filter は比較内容の抑制であり、全 artifact の秘匿処理ではない
Node mask はページ title、URL、ID/test ID/href などの identity 属性、CSS、screenshot、ユーザー指定の decision、log を秘匿しない
選択した subtree の外にある label 経由のテキストも残る場合がある
共有前に artifact を確認し、完全な秘匿が必要なデータは取得元から除く

## Finding ID と保存済み report

新しい report は `finding_id_version: 2` と `text_changed:v2:<24桁の16進数>` 形式の ID を持つ
Hash には左右のページ URL、scope、個別ノード、正規化後の変更内容全体を、field の区切りが曖昧にならない形式で含める
同じ見た目の control が複数あっても個別に ID を持ち、人向け preview の末尾以降で変更内容が変わった場合も ID が変わる
Session ID は含めず、ページ・scope・ノードの入力が同じなら ID を再現する

JSON finding の `old` と `new` は正規化後の値全体を保持する
Text と Markdown の preview は120 Unicode文字に制限する
`old_ref` と `new_ref` は対象の個別ノードを示し、decision audit と screenshot crop に使う

以前の ID 形式を使った finding decision は、新しい ID に自動変換・適用しない
新しい finding decision template を生成してレビューし、新しい compare JSON に対して validation/audit を実行する
Decision JSONL の `schema_version` は `1` のままで、finding ID の version とは独立している
古い保存済み report は、それに対応する decision とともに引き続き読込・audit できる
ID は example から組み立てず、生成した artifact からコピーする

Snapshot は `original_index`、`structure_path` と、取得できた場合の `states`、`type`、`placeholder` を保存する
Node 配列が fingerprint 順でも、subtree matching と audit では元の DOM 順を使う
`original_index` がない古い report は、Chromium が DOM 順に割り当てた正の observation ID を使う
両方の順序情報がない外部 report では、この保証はできない

全 matching mode で手動 pair の一対一対応を保つ
交差する手動 pair も対応として保持するが、histogram の region 境界には交差しない subset を使い、同じノードが2つの region で再利用されないようにする

## Runtime の挙動

Wait timeout は、解決しない Promise を含む個々の CDP 評価と polling 間隔にも適用する
呼び出し元の deadline が早い場合やキャンセルされた場合は、そちらを優先する
キャンセルは wait request を終える処理であり、ページ内で実行中の任意の JavaScript を終了する保証はない

Session の起動中は ID を予約し、manager lock の外で browser を attach するため、他の session の操作を継続できる
Shutdown は起動中の session をキャンセルし、全 session の cleanup を試みて error を集約する
失敗した cleanup は再試行できるよう登録を残し、browser 終了後の profile directory 削除失敗も再試行の対象にする

Observation と runtime の変更に合わせて daemon build epoch を更新する
RPC field は追加であり protocol version は `v1` を継続するため、`nxctl` と `nxd` を揃えて更新する
