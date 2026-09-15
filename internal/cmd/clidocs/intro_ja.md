# Nexus CLI リファレンス

[English guide](cli-reference.md)

このリファレンスは `nxctl --help` と `nxctl help <command> [subcommand]` と同じ Nagi command graph から生成します。導入済みのバージョンで受理される引数と option は、そのバージョンの help で確認してください。有限の選択肢を command graph に定義した項目は、候補値も表示します

操作手順は [AI usage guide](ai/usage.md) を参照してください。両言語版とも、実際の CLI と照合できるよう生成 help 本文の英語表現を保持します

保守時は command graph を編集し、リポジトリのルートで `mise run docs` を実行してください。同じ処理を `go run ./internal/cmd/clidocs` でも実行できます。`go test ./internal/cmd/clidocs` で両方の参照ファイルと graph の一致を確認します。生成された command の節は直接編集しないでください。案内文は `internal/cmd/clidocs/intro.md` と `intro_ja.md` で管理します

## コマンド一覧
