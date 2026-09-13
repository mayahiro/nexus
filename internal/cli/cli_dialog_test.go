package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/rpc"
)

type dialogRPCHandler struct {
	noopRPCHandler
	requests chan<- api.ActSessionRequest
}

func (h dialogRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	h.requests <- req
	if req.SessionID == "rpc-error" {
		return api.ActSessionResponse{}, errors.New("no JavaScript dialog is open")
	}
	if req.SessionID == "failed" {
		return api.ActSessionResponse{Result: api.ActionResult{Message: "dialog handling failed"}}, nil
	}
	if req.Action.Args["operation"] == "get" {
		return api.ActSessionResponse{Result: api.ActionResult{
			OK: true, Message: `prompt dialog: "Name?" (default: "initial")`,
			Value: api.DialogState{Open: true, Type: "prompt", Message: "Name?", DefaultPrompt: "initial"},
		}}, nil
	}
	return api.ActSessionResponse{Result: api.ActionResult{OK: true, Changed: true, Message: "handled prompt dialog"}}, nil
}

func TestDialogCLI(t *testing.T) {
	configureXDGTestEnv(t)
	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	requests := make(chan api.ActSessionRequest, 1)
	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, dialogRPCHandler{requests: requests}, rpc.ServeOptions{})
	}()
	defer func() {
		cancel()
		listener.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(2 * time.Second):
			t.Error("RPC server did not stop")
		}
	}()

	for _, test := range []struct {
		name      string
		args      []string
		session   string
		action    map[string]string
		asJSON    bool
		wantError string
	}{
		{name: "get text", args: []string{"dialog", "get"}, session: "default", action: map[string]string{"operation": "get"}},
		{name: "get JSON", args: []string{"dialog", "get", "--session", "work", "--json"}, session: "work", action: map[string]string{"operation": "get"}, asJSON: true},
		{name: "accept default", args: []string{"dialog", "accept"}, session: "default", action: map[string]string{"operation": "accept"}},
		{name: "accept text", args: []string{"dialog", "accept", "--session", "work", "--text", " 日本語\n "}, session: "work", action: map[string]string{"operation": "accept", "text": " 日本語\n "}},
		{name: "accept empty", args: []string{"dialog", "accept", "--text", "", "--json"}, session: "default", action: map[string]string{"operation": "accept", "text": ""}, asJSON: true},
		{name: "dismiss", args: []string{"dialog", "dismiss", "--json"}, session: "default", action: map[string]string{"operation": "dismiss"}, asJSON: true},
		{name: "RPC error", args: []string{"dialog", "accept", "--session", "rpc-error"}, session: "rpc-error", action: map[string]string{"operation": "accept"}, wantError: "no JavaScript dialog is open"},
		{name: "failed result", args: []string{"dialog", "dismiss", "--session", "failed"}, session: "failed", action: map[string]string{"operation": "dismiss"}, wantError: "dialog handling failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(t.Context(), test.args, &stdout, &stderr)
			var req api.ActSessionRequest
			select {
			case req = <-requests:
			case <-time.After(2 * time.Second):
				t.Fatalf("dialog request was not sent: code=%d stderr=%s", code, &stderr)
			}
			if req.SessionID != test.session || req.Action.Kind != "dialog" || !reflect.DeepEqual(req.Action.Args, test.action) {
				t.Fatalf("incorrect dialog request: %+v", req)
			}
			if test.wantError != "" {
				if code != 1 || !strings.Contains(stderr.String(), test.wantError) || stdout.Len() != 0 {
					t.Fatalf("incorrect failure: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
				}
				return
			}
			if code != 0 || stderr.Len() != 0 {
				t.Fatalf("dialog failed: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
			}
			if test.asJSON {
				var result struct {
					OK      bool            `json:"ok"`
					Changed bool            `json:"changed"`
					Value   api.DialogState `json:"value"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || !result.OK {
					t.Fatalf("invalid dialog JSON: %s, %v", &stdout, err)
				}
				if test.action["operation"] == "get" && (!result.Value.Open || result.Value.Type != "prompt" || result.Value.DefaultPrompt != "initial" || result.Changed) {
					t.Fatalf("dialog state lost in RPC: %+v", result)
				}
			} else if !strings.Contains(stdout.String(), "prompt dialog") {
				t.Fatalf("missing dialog text: %s", &stdout)
			}
		})
	}
}
