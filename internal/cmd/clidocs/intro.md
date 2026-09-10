# Nexus CLI reference

[Japanese guide](cli-reference_ja.md)

This reference is generated from the same Nagi command graph as `nxctl --help` and `nxctl help <command> [subcommand]`. For an installed version, use its help to confirm the accepted arguments and options. Finite choices appear as possible values where the command graph declares them.

For workflow guidance, see the [AI usage guide](ai/usage.md). Both language editions reproduce the generated help in English so it can be compared directly with the CLI.

Maintainers: edit the command graph, then run `mise run docs` from the repository root. The equivalent command is `go run ./internal/cmd/clidocs`. `go test ./internal/cmd/clidocs` checks that both reference files match the graph. Do not edit the generated command sections by hand. The introductions are maintained in `internal/cmd/clidocs/intro.md` and `intro_ja.md`.

## Commands
