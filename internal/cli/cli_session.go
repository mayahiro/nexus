package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	nagicli "github.com/mayahiro/nagicli-go"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/rpc"
)

func runCloseInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	sessionID := nagiStringValue(invocation, "session")
	closeAll := nagiBoolValue(invocation, "all")

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	if closeAll {
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

	res, err := client.DetachSession(ctx, api.DetachSessionRequest{SessionID: sessionID})
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

func runSessionsInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	asJSON := nagiBoolValue(invocation, "json")

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

	if asJSON {
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

func runDetachInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	sessionID := nagiStringValue(invocation, "session")

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.DetachSession(ctx, api.DetachSessionRequest{
		SessionID: sessionID,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "detached %s\n", res.Session.ID)
	return 0
}
