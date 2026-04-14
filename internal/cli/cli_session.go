package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/rpc"
)

func runClose(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printCloseHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("close", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	closeAll := fs.Bool("all", false, "close all sessions")

	if err := parseCommandFlags(fs, args, stderr, "close"); err != nil {
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	if *closeAll {
		listed, err := client.ListSessions(ctx)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		for _, session := range listed.Sessions {
			if _, err := client.DetachSession(ctx, api.DetachSessionRequest{SessionID: session.ID}); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		}
		if err := stopDaemonIfNoSessions(ctx, client); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, "closed all sessions")
		return 0
	}

	res, err := client.DetachSession(ctx, api.DetachSessionRequest{SessionID: *sessionID})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := stopDaemonIfNoSessions(ctx, client); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "closed %s\n", res.Session.ID)
	return 0
}

func stopDaemonIfNoSessions(ctx context.Context, client *rpc.Client) error {
	listed, err := client.ListSessions(ctx)
	if err != nil {
		return err
	}
	if len(listed.Sessions) != 0 {
		return nil
	}
	_, err = client.StopDaemon(ctx)
	return err
}

func runSessions(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printSessionsHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("sessions", flag.ContinueOnError)
	fs.SetOutput(stderr)

	asJSON := fs.Bool("json", false, "print as json")
	if err := parseCommandFlags(fs, args, stderr, "sessions"); err != nil {
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ListSessions(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(res.Sessions); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	if len(res.Sessions) == 0 {
		fmt.Fprintln(stdout, "no sessions")
		return 0
	}

	for _, session := range res.Sessions {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", session.ID, session.TargetType, session.Backend, session.TargetRef)
	}

	return 0
}

func runDetach(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printDetachHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("detach", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "", "session id")
	if err := parseCommandFlags(fs, args, stderr, "detach"); err != nil {
		return 1
	}

	if *sessionID == "" {
		fmt.Fprintln(stderr, "--session is required")
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.DetachSession(ctx, api.DetachSessionRequest{
		SessionID: *sessionID,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "detached %s\n", res.Session.ID)
	return 0
}
