# AI による要素対応候補の判定

[English](compare-identity.md) | [Compare guide](compare.md)

`nxctl compare suggest-decisions` は、保存済みの単一ページの compare report に対して TypeSafe AI の Jev を使い、対応する要素を提案する実験的な機能
未対応の要素を調べ、既存の [decision JSONL 形式](compare-decisions.schema.json)を出力する
通常の `compare` はローカルで実行され、AI サービスを呼び出さない

対応付けは、特定の注文の保存ボタンなど、同じ論理的な UI 上の対象を識別する
動作の同等性や変更の承認を意味せず、対応した要素でも [比較仕様](compare-contract_ja.md)に従ってテキスト、属性、状態、CSS、layout の差分を検出する

## 利用手順

狭い scope で matching debug を含む比較を保存する

```text
nxctl compare https://old.example.com/orders https://new.example.com/orders --node-scope semantic --scope-selector main --match-mode stable --matching-debug --output-json compare-debug.json
```

API key、外部通信、daemon なしで実際の送信 body を確認する

```text
nxctl compare suggest-decisions --compare-json compare-debug.json --dry-run
```

JSON の `requests[]` には、検討対象となった旧要素のうち新しい候補があるものについて、1 件ずつ request が入る
送信前にページの文章と同一性の根拠を確認する
`--output-json requests.json` を指定すると、新しいファイルに preview を保存できる

環境変数 `TYPESAFE_API_KEY` を設定し、明示的に外部での判定を実行する

```text
nxctl compare suggest-decisions --compare-json compare-debug.json --output pair-suggestions.jsonl
```

この実行は限定した context を `https://api.typesafe.ai/v1/systemone` に送信する
別の endpoint は指定できない
既定のモデルは `jev-1.13.0` に固定し、`--model` で別の Jev version や公式 alias を指定できる
詳細は [TypeSafe API reference](https://docs.typesafe.ai/api) と [model specifications](https://docs.typesafe.ai/models) を参照する

既定のペア候補は `confidence:"tentative"` で、matching には適用されない
対応する要素と review report を確認し、承認したペアだけ `confidence:"high"` に変更する
レビュー済みのファイルを保存した比較に対して検証する

```text
nxctl compare validate-decisions --decisions-file pair-suggestions.jsonl --compare-json compare-debug.json --strict
nxctl compare https://old.example.com/orders https://new.example.com/orders --node-scope semantic --scope-selector main --match-mode stable --decisions-file pair-suggestions.jsonl
```

観測の scope と filter を揃える
出力する decision は ref と fingerprint を含むため、観測が変わった場合は黙って適用せず拒否する
ref が古くなった場合は既存の repair workflow を使い、結果をレビューする

## 任意の昇格

実際のページでモデルを評価した後、`--promote` によって `high` のペアを出力できる

```text
nxctl compare suggest-decisions --compare-json compare-debug.json --output pair-promoted.jsonl --promote --min-probability 0.98 --min-margin 0.30
```

昇格には次の条件をすべて満たす必要がある

- 選択された回答が `none` や `unknown` ではなく、渡した候補である
- Choice の候補確率が `--min-probability` 以上である
- 独立した Noul 質問の同一要素確率が同じ閾値以上である
- 次点の回答との確率差が `--min-margin` 以上である
- 検討対象の別の旧要素が同じ新要素を提案していない

既定の閾値は `0.95` と `0.20` であり、実験用の値であって実測した誤り率の保証ではない
Jev の `confidence` は記録するが、Nexus の `high` へ直接変換しない
確率条件を満たさない候補は tentative に保つ
`none`、`unknown`、競合する候補は `new:"?"` の未解決の `pair` として残し、削除、追加、finding の承認や opaque subtree を作らない

## 送信データと制約

request は role、name、label、text、test ID、input type、placeholder、href の path 部分を含む
観測済みの祖先を最大 3 件、DOM 順で前後の要素をそれぞれ最大 2 件含む
文字列は 240 Unicode characters に制限し、切り詰めた場合は `...` を付加する
観測にない context を live page から補完することはない

フォームの value、ページ全体の text、title、ページ URL、href の認証情報・host・query・fragment、fingerprint、CSS、screenshot、ローカル ref は独立した field として送信しない
text、label、test ID、href path には機密情報が含まれる場合があり、祖先が集約した text にも注意が必要
これは送信 field の allowlist であって完全な秘匿ではないため、dry-run の payload と [filter の制約](compare-contract_ja.md#filter)を確認する

候補は未対応の要素から、利用できる role、text/name、label、test ID、href path、順序、bounds の情報で順位付けする
既定では旧要素を最大 20 件、各旧要素の新しい候補を最大 5 件検討する
`--limit` は 1–200、`--max-candidates` は 1–20 を受理する
ref のない要素は除外し、ref 不足や limit によって除外された旧要素の数を report に記録する
候補の網羅性を保証しない

入力 report は 64 MiB、各 request body は 32 KiB に制限する
manifest 全体の report は対応せず、ページごとの report を使う
既存の対応済みペアは再判定しない
観測や候補抽出で相手が除外された場合はモデルで回復できない
現行 Jev は画像ではなくテキストを受け付け、この command は OpenAI の Decisions API を実装しない

`--timeout` は retry を含めた判定全体を制限し、既定は 30000 milliseconds
request は順次実行する
HTTP 429 と 529 は `Retry-After` を尊重し、exponential backoff で最大 2 回 retry でき、それ以外の error は command を失敗させる
provider の error body は出力せず、HTTP redirect にも追従しない

## 出力と評価

外部での判定が成功すると decision と `<output>.review.json`、または `--output-json` で指定した review path に出力する
両方のファイルは mode `0600` とし、既存 directory 内の新しい path が必要
既存ファイルは上書きしない
request の失敗や cancellation ではファイルを作らず、ファイル出力が失敗した場合は、その実行が新しく作ったファイルを削除する
`--json` は review report を標準出力にも表示する

review report は `schema_version:1`、provider と要求したモデル、question version、compare file の SHA-256、閾値、集計、`judgments[]` を含む
各 judgment は候補の ref/fingerprint、実際の request の SHA-256、返された model、確率分布、confidence、Noul の回答、token usage、昇格の根拠、decision を記録する
通常の判定 report は request body を含まず、dry-run report だけ `requests[]` を含む
これらのローカル artifact はページに由来する fingerprint を含む場合があり、共有前の確認が必要

昇格を利用する前に、正解を付けたページのペアで誤対応、対応の見逃し、未解決の結果、回帰の finding が残ることを評価する
異なる領域や record の同名 control、ラベル変更、a11y 改善、リンク先変更、日本語を含める
実際の workload で処理時間と token usage を測定する
TypeSafe は敵対的な入力や独立した質問間の構造的な不変条件などの [制約](https://docs.typesafe.ai/model-jaggedness/jev-1.13)を公開している
