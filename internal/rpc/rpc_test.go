package rpc

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/mayahiro/nexus/internal/api"
)

type testHandler struct{}

func (testHandler) Ping(_ context.Context, _ api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{
		ProtocolVersion: api.ProtocolVersion,
		DaemonVersion:   api.DaemonVersion,
	}, nil
}

func (testHandler) AttachSession(_ context.Context, req api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	return api.AttachSessionResponse{
		Session: api.Session{
			ID:         req.SessionID,
			TargetType: req.TargetType,
			Backend:    req.Backend,
		},
	}, nil
}

func (testHandler) ListSessions(_ context.Context, _ api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{
		Sessions: []api.Session{
			{ID: "web1", TargetType: "browser", Backend: "chromium"},
		},
	}, nil
}

func (testHandler) DetachSession(_ context.Context, req api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{
		Session: api.Session{
			ID: req.SessionID,
		},
	}, nil
}

func (testHandler) StopDaemon(_ context.Context, _ api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (testHandler) ObserveSession(_ context.Context, req api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	return api.ObserveSessionResponse{
		Observation: api.Observation{
			SessionID:  req.SessionID,
			TargetType: "browser",
			Title:      "example",
		},
	}, nil
}

func (testHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	var value interface{}
	switch req.Action.Text {
	case "false":
		value = false
	case "0":
		value = 0
	case `""`:
		value = ""
	default:
		value = req.Action.Text
	}

	return api.ActSessionResponse{
		Result: api.ActionResult{
			OK:      true,
			Changed: false,
			Value:   value,
		},
	}, nil
}

func TestPing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	socket := filepath.Join(t.TempDir(), "nxd.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, listener, testHandler{}, ServeOptions{})
	}()

	client, err := Dial(context.Background(), socket)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	res, err := client.Ping(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if res.ProtocolVersion != api.ProtocolVersion {
		t.Fatalf("unexpected protocol version: %s", res.ProtocolVersion)
	}

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}

func TestSessionRPC(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	socket := filepath.Join(t.TempDir(), "nxd.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, listener, testHandler{}, ServeOptions{})
	}()

	client, err := Dial(context.Background(), socket)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	attached, err := client.AttachSession(context.Background(), api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  "web1",
		Backend:    "chromium",
	})
	if err != nil {
		t.Fatal(err)
	}
	if attached.Session.ID != "web1" {
		t.Fatalf("unexpected attach result: %+v", attached)
	}

	listed, err := client.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Sessions) != 1 || listed.Sessions[0].ID != "web1" {
		t.Fatalf("unexpected sessions result: %+v", listed)
	}

	detached, err := client.DetachSession(context.Background(), api.DetachSessionRequest{
		SessionID: "web1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if detached.Session.ID != "web1" {
		t.Fatalf("unexpected detach result: %+v", detached)
	}

	observed, err := client.ObserveSession(context.Background(), api.ObserveSessionRequest{
		SessionID: "web1",
		Options:   api.ObserveOptions{WithText: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Observation.SessionID != "web1" {
		t.Fatalf("unexpected observe result: %+v", observed)
	}

	acted, err := client.ActSession(context.Background(), api.ActSessionRequest{
		SessionID: "web1",
		Action: api.Action{
			Kind: "eval",
			Text: "document.title",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if acted.Result.Value != "document.title" {
		t.Fatalf("unexpected act result: %+v", acted)
	}

	acted, err = client.ActSession(context.Background(), api.ActSessionRequest{
		SessionID: "web1",
		Action: api.Action{
			Kind: "eval",
			Text: "false",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := acted.Result.Value.(bool); !ok || value {
		t.Fatalf("unexpected false act result: %+v", acted)
	}

	acted, err = client.ActSession(context.Background(), api.ActSessionRequest{
		SessionID: "web1",
		Action: api.Action{
			Kind: "eval",
			Text: "0",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := acted.Result.Value.(float64); !ok || value != 0 {
		t.Fatalf("unexpected zero act result: %+v", acted)
	}

	acted, err = client.ActSession(context.Background(), api.ActSessionRequest{
		SessionID: "web1",
		Action: api.Action{
			Kind: "eval",
			Text: `""`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := acted.Result.Value.(string); !ok || value != "" {
		t.Fatalf("unexpected empty-string act result: %+v", acted)
	}

	stopped, err := client.StopDaemon(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !stopped.Stopped {
		t.Fatalf("unexpected stop result: %+v", stopped)
	}

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rpc server did not stop")
	}
}
