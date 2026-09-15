package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	nagicli "github.com/mayahiro/nagicli-go"

	"github.com/mayahiro/nexus/internal/api"
)

func newNagiDialogCommand() *nagicli.Command {
	command := nagicli.NewCommand("dialog").
		About("Inspect and handle JavaScript dialogs").
		RequireSubcommand().
		SubcommandUsage(nagicli.SubcommandUsageExpanded).
		Note("Supports alert, confirm, prompt, and beforeunload in the selected session").
		Note("Page operations blocked by a dialog return an error; handle the dialog before continuing")
	for _, entry := range []struct {
		name string
		help string
	}{
		{"get", "Show the currently open JavaScript dialog"},
		{"accept", "Accept the current JavaScript dialog"},
		{"dismiss", "Dismiss the current JavaScript dialog"},
	} {
		usage := "[--session <ID>] [--json]"
		subcommand := nagicli.NewCommand(entry.name).
			About(entry.help).
			Option(nagiSessionOption()).
			Option(nagiJSONFlag()).
			Handle(nagiRunHandler(runDialogInvocation(entry.name)))
		if entry.name == "accept" {
			usage = "[--text <TEXT>] " + usage
			subcommand.Option(nagiValueOption("text", "TEXT",
				"Prompt response, including an empty string; defaults to the prompt's initial value")).
				Note("--text is valid only for prompt dialogs; accepting beforeunload allows navigation")
		}
		subcommand.UsageVariant("default", usage)
		command.Subcommand(subcommand)
	}
	return command
}

func runDialogInvocation(operation string) nagiCommandRunner {
	return func(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
		client, err := connectClient(ctx)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		defer client.Close()

		action := api.Action{Kind: "dialog", Args: map[string]string{"operation": operation}}
		if value, ok := invocation.RawValue("text"); ok {
			action.Args["text"] = value
		}
		res, err := client.ActSession(ctx, api.ActSessionRequest{
			SessionID: nagiStringValue(invocation, "session"),
			Action:    action,
		})
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if !res.Result.OK {
			fmt.Fprintln(stderr, res.Result.Message)
			return 1
		}
		if nagiBoolValue(invocation, "json") {
			encoder := json.NewEncoder(stdout)
			encoder.SetIndent("", "  ")
			if err := encoder.Encode(res.Result); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		} else {
			fmt.Fprintln(stdout, res.Result.Message)
		}
		return 0
	}
}
