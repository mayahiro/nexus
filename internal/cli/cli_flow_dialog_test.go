package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/browsermgr"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/daemon"
	"github.com/mayahiro/nexus/internal/rpc"
)

func TestResolveFlowDialogStep(t *testing.T) {
	for _, test := range []struct {
		input     string
		wantText  *string
		wantError string
	}{
		{input: `{"action":"dialog","target":"get"}`},
		{input: `{"action":"dialog","target":"accept"}`},
		{input: `{"action":"dialog","target":"accept","text":""}`, wantText: new("")},
		{input: `{"action":"dialog","target":"accept","text":" {{answer}}\n "}`, wantText: new(" 日本語\n ")},
		{input: `{"action":"fill","locator":"@e1","text":" value "}`, wantText: new("value")},
		{input: `{"action":"click","locator":"@e1","expect_dialog":true}`},
		{input: `{"action":"navigate","value":"https://example.com","expect_dialog":true}`},
		{input: `{"action":"dialog"}`, wantError: "target must be"},
		{input: `{"action":"dialog","target":"close"}`, wantError: "target must be"},
		{input: `{"action":"dialog","target":"dismiss","text":""}`, wantError: "text is only supported"},
		{input: `{"action":"dialog","target":"get","text":"value"}`, wantError: "text is only supported"},
		{input: `{"action":"dialog","target":"get","side":"typo"}`, wantError: "side must be"},
		{input: `{"action":"dialog","target":"get","timeout":0}`, wantError: "timeout must be"},
		{input: `{"action":"click","locator":"@e1","expect_dialog":true,"timeout":-1}`, wantError: "timeout must be"},
		{input: `{"action":"wait","target":"hydrated","expect_dialog":true}`, wantError: "only supported"},
		{input: `{"action":"dialog","target":"accept","expect_dialog":true}`, wantError: "only supported"},
		{input: `{"action":"click","expect_dialog":true}`, wantError: "requires locator"},
		{input: `{"action":"fill","locator":"@e1","expect_dialog":true}`, wantError: "requires text"},
		{input: `{"action":"navigate","expect_dialog":true}`, wantError: "requires value"},
	} {
		t.Run(test.input, func(t *testing.T) {
			var step flowStep
			if err := json.Unmarshal([]byte(test.input), &step); err != nil {
				t.Fatal(err)
			}
			resolved, err := resolveFlowStep(step, map[string]string{"answer": "日本語"})
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if (resolved.Text == nil) != (test.wantText == nil) || (test.wantText != nil && *resolved.Text != *test.wantText) {
				t.Fatalf("text presence or content changed: %+v", resolved.Text)
			}
		})
	}
}

type flowDialogRPCHandler struct {
	flowRPCHandler
	failSession string
	omitDialog  bool
}

func (h *flowDialogRPCHandler) ActSession(ctx context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if !req.Action.ExpectDialog && req.Action.Kind != "dialog" {
		return h.flowRPCHandler.ActSession(ctx, req)
	}
	h.mu.Lock()
	h.actRequests = append(h.actRequests, req)
	h.mu.Unlock()
	if req.Action.ExpectDialog {
		if req.SessionID == h.failSession {
			return api.ActSessionResponse{}, errors.New("ordinary action failure")
		}
		if h.omitDialog {
			return api.ActSessionResponse{Result: api.ActionResult{OK: true}}, nil
		}
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Changed: true, Message: "paused for prompt", Dialog: &api.DialogState{Open: true, Type: "prompt", Message: "Name?"}}}, nil
	}
	if req.Action.Args["operation"] == "get" {
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Message: `prompt dialog: "Name?"`, Value: api.DialogState{Open: true, Type: "prompt", Message: "Name?"}}}, nil
	}
	return api.ActSessionResponse{Result: api.ActionResult{OK: true, Changed: true, Message: "handled prompt"}}, nil
}

func serveFlowDialogRPC(t *testing.T, handler rpc.Handler) {
	t.Helper()
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
	done := make(chan error, 1)
	go func() { done <- rpc.Serve(ctx, listener, handler, rpc.ServeOptions{}) }()
	t.Cleanup(func() {
		cancel()
		listener.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("flow RPC server did not stop")
		}
	})
}

func runDialogFlow(t *testing.T, manifest any) (flowReport, int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flow.json")
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	code := Run(ctx, []string{"flow", "run", "--manifest", path, "--json"}, &stdout, &stderr)
	var report flowReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("invalid report: %v, stdout=%s stderr=%s", err, &stdout, &stderr)
	}
	return report, code
}

func TestFlowDialogRun(t *testing.T) {
	for _, test := range []struct {
		name        string
		side        string
		failSession string
		omitDialog  bool
	}{
		{name: "both sides"}, {name: "old only", side: "old"}, {name: "new only", side: "new"},
		{name: "second side fails", failSession: "new"}, {name: "missing dialog result", omitDialog: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := &flowDialogRPCHandler{failSession: test.failSession, omitDialog: test.omitDialog}
			serveFlowDialogRPC(t, handler)
			manifest := flowManifest{Scenarios: []flowScenario{{Name: "prompt", Old: flowEndpoint{Session: "old"}, New: flowEndpoint{Session: "new"}, Steps: []flowStep{
				{Action: "click", Locator: "@e2", ExpectDialog: true, Timeout: new(1234)},
				{Action: "dialog", Target: "get"},
				{Action: "dialog", Target: "accept", Text: new("")},
				{Action: "wait", Target: "hydrated"},
			}}}}
			for i := range manifest.Scenarios[0].Steps {
				manifest.Scenarios[0].Steps[i].Side = test.side
			}
			report, code := runDialogFlow(t, manifest)
			if test.failSession != "" || test.omitDialog {
				if code != 1 || report.Summary.FailedSteps != 1 || len(report.Scenarios[0].Steps) != 1 {
					t.Fatalf("failure swallowed: %+v code=%d", report, code)
				}
				if test.failSession == "new" && len(report.Scenarios[0].Steps[0].Dialogs) != 1 {
					t.Fatal("first side's result was lost")
				}
				return
			}
			if code != 0 || report.Summary.FailedSteps != 0 || len(report.Scenarios[0].Steps) != 4 {
				t.Fatalf("unexpected flow result: %+v code=%d", report, code)
			}
			sides := []string{"old", "new"}
			if test.side != "" {
				sides = []string{test.side}
			}
			if len(handler.actRequests) != 4*len(sides) {
				t.Fatalf("unexpected request count: %d", len(handler.actRequests))
			}
			for i, req := range handler.actRequests[:len(sides)] {
				if req.SessionID != sides[i] || !req.Action.ExpectDialog || req.Action.Args["timeout_ms"] != "1234" {
					t.Fatalf("invalid trigger request: %+v", req)
				}
			}
			for _, req := range handler.actRequests[2*len(sides) : 3*len(sides)] {
				text, supplied := req.Action.Args["text"]
				if !supplied || text != "" {
					t.Fatalf("empty prompt text lost: %+v", req)
				}
			}
			if len(report.Scenarios[0].Steps[0].Dialogs) != len(sides) || len(report.Scenarios[0].Steps[1].Dialogs) != len(sides) {
				t.Fatal("missing per-side dialog results")
			}
			var output bytes.Buffer
			printFlowReport(&output, report)
			for _, side := range sides {
				if !strings.Contains(output.String(), `dialog[`+side+`]: prompt dialog: "Name?"`) {
					t.Fatalf("dialog state not printed: %s", &output)
				}
			}
		})
	}
}

func TestFlowDialogChromiumE2E(t *testing.T) {
	if os.Getenv("NEXUS_E2E") != "1" || runtime.GOOS != "darwin" {
		t.Skip("set NEXUS_E2E=1 on macOS to run Chromium e2e")
	}
	executable := strings.TrimSpace(os.Getenv("NEXUS_E2E_CHROMIUM_PATH"))
	if executable == "" {
		if paths, err := config.DefaultPaths(); err == nil {
			if install, err := browsermgr.New(paths).Resolve(browsermgr.BrowserChromium); err == nil {
				executable = install.ExecutablePath
			}
		}
	}
	if executable == "" {
		t.Skip("set NEXUS_E2E_CHROMIUM_PATH or install a managed browser")
	}
	server := daemon.NewServer(func() {})
	serveFlowDialogRPC(t, server)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/onload" {
			fmt.Fprint(w, `<!doctype html><script>window.loaded=prompt('loaded','initial')</script><p>Loaded</p>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body>
<button data-testid="confirm" onclick="window.confirmed=confirm('Continue?')">Confirm</button>
<button data-testid="prompt" onclick="window.answer=prompt('Name?','initial')">Prompt</button>
<button data-testid="async" onclick="setTimeout(()=>{alert('delayed');window.done=true},50)">Alert</button>
<button data-testid="leave" style="position:absolute;left:8px;top:80px" onclick="window.onbeforeunload=event=>{event.preventDefault();event.returnValue=''}">Enable leave confirmation</button>
<input data-testid="fill" onchange="alert('filled');window.filled=this.value">
</body></html>`)
	}))
	defer site.Close()
	steps := []flowStep{
		{Action: "dialog", Target: "get"},
		{Action: "navigate", Value: site.URL},
		{Action: "wait", Target: "selector", Value: "button", State: "visible"},
		{Action: "click", Locator: "testid=confirm", ExpectDialog: true, Timeout: new(3000)},
		{Action: "dialog", Target: "get"},
		{Action: "dialog", Target: "dismiss"},
		{Action: "wait", Target: "function", Value: "window.confirmed === false"},
	}
	for _, answer := range []*string{new("  日本語  "), new(""), nil} {
		want := "initial"
		if answer != nil {
			want = *answer
		}
		encoded, _ := json.Marshal(want)
		steps = append(steps,
			flowStep{Action: "click", Locator: "testid=prompt", ExpectDialog: true, Timeout: new(3000)},
			flowStep{Action: "dialog", Target: "accept", Text: answer},
			flowStep{Action: "wait", Target: "function", Value: "window.answer === " + string(encoded)},
		)
	}
	steps = append(steps,
		flowStep{Action: "click", Locator: "testid=async", ExpectDialog: true, Timeout: new(3000)},
		flowStep{Action: "dialog", Target: "accept"},
		flowStep{Action: "wait", Target: "function", Value: "window.done === true"},
		flowStep{Action: "fill", Locator: "testid=fill", Text: new("value"), ExpectDialog: true, Timeout: new(3000)},
		flowStep{Action: "dialog", Target: "accept"},
		flowStep{Action: "wait", Target: "function", Value: `window.filled === "value"`},
		flowStep{Action: "navigate", Value: site.URL + "/onload", ExpectDialog: true, Timeout: new(3000)},
		flowStep{Action: "dialog", Target: "accept", Text: new("loaded")},
		flowStep{Action: "wait", Target: "function", Value: `window.loaded === "loaded"`},
		flowStep{Action: "compare"},
	)
	manifest := flowManifest{Scenarios: []flowScenario{{Name: "flow-dialog-e2e", Old: flowEndpoint{URL: "about:blank", TargetRef: executable}, New: flowEndpoint{URL: "about:blank", TargetRef: executable}, Steps: steps}}}
	report, code := runDialogFlow(t, manifest)
	if code != 0 || report.Summary.FailedSteps != 0 || report.Summary.TotalSteps != len(steps) || report.Summary.TotalCompares != 1 {
		t.Fatalf("flow e2e failed: code=%d report=%+v", code, report)
	}
	for _, side := range []string{"old", "new"} {
		if result := report.Scenarios[0].Steps[3].Dialogs[side]; result.Dialog == nil || result.Dialog.Type != "confirm" {
			t.Fatalf("dialog missing on %s: %+v", side, result)
		}
	}
	t.Run("beforeunload", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		for _, side := range []string{"old", "new"} {
			_, err := server.AttachSession(ctx, api.AttachSessionRequest{
				TargetType: "browser", SessionID: side, TargetRef: executable,
				Options: map[string]string{"initial_url": site.URL},
			})
			if err != nil {
				t.Fatal(err)
			}
			// Flow's selector clicks use JavaScript. Establish sticky user
			// activation with a real mouse event before reusing the session.
			for _, action := range []api.Action{
				{Kind: "wait", Args: map[string]string{"target": "selector", "value": `[data-testid="leave"]`, "state": "visible"}},
				{Kind: "invoke", Args: map[string]string{"x": "20", "y": "90"}},
			} {
				res, err := server.ActSession(ctx, api.ActSessionRequest{SessionID: side, Action: action})
				if err != nil || !res.Result.OK {
					t.Fatalf("prepare %s user activation: %+v, %v", side, res, err)
				}
			}
		}
		manifest := flowManifest{Scenarios: []flowScenario{{Name: "leave", Old: flowEndpoint{Session: "old"}, New: flowEndpoint{Session: "new"}, Steps: []flowStep{
			{Action: "navigate", Value: site.URL + "/next", ExpectDialog: true, Timeout: new(3000)},
			{Action: "dialog", Target: "dismiss"},
			{Action: "wait", Target: "function", Value: `location.pathname === "/"`},
			{Action: "navigate", Value: site.URL + "/next", ExpectDialog: true, Timeout: new(3000)},
			{Action: "dialog", Target: "accept"},
			{Action: "wait", Target: "function", Value: `location.pathname === "/next"`},
		}}}}
		report, code := runDialogFlow(t, manifest)
		if code != 0 || report.Summary.FailedSteps != 0 || report.Summary.TotalSteps != 6 {
			t.Fatalf("beforeunload flow failed: code=%d report=%+v", code, report)
		}
		for _, side := range []string{"old", "new"} {
			for _, index := range []int{0, 3} {
				if dialog := report.Scenarios[0].Steps[index].Dialogs[side].Dialog; dialog == nil || dialog.Type != "beforeunload" {
					t.Fatalf("beforeunload missing on %s: %+v", side, dialog)
				}
			}
		}
	})
}
