package chromium

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

type dialogExecutor func(context.Context, string, any, any) error

func (fn dialogExecutor) Execute(ctx context.Context, method string, params, result any) error {
	return fn(ctx, method, params, result)
}

func TestDialogState(t *testing.T) {
	backend := New()
	backend.targetInfo.ID = "page1"
	for _, kind := range []page.DialogType{page.DialogTypeAlert, page.DialogTypeConfirm, page.DialogTypePrompt, page.DialogTypeBeforeunload} {
		t.Run(string(kind), func(t *testing.T) {
			backend.trackDialogEvent("page1", &page.EventJavascriptDialogOpening{
				Type: kind, Message: "  message\n", URL: "https://example.com/frame", DefaultPrompt: " initial ",
			})
			backend.trackDialogEvent("other", &page.EventJavascriptDialogOpening{Type: page.DialogTypeAlert, Message: "other"})
			result, err := backend.dialogInContext(t.Context(), api.Action{Args: map[string]string{"operation": "get"}})
			if err != nil {
				t.Fatal(err)
			}
			want := api.DialogState{Open: true, Type: string(kind), Message: "  message\n", URL: "https://example.com/frame", DefaultPrompt: " initial "}
			if result.Value != want || !result.OK || result.Changed {
				t.Fatalf("unexpected state: %+v", result)
			}
			backend.trackDialogEvent("page1", &page.EventJavascriptDialogClosed{})
			result, err = backend.dialogInContext(t.Context(), api.Action{Args: map[string]string{"operation": "get"}})
			if err != nil || result.Value != (api.DialogState{}) || result.Changed {
				t.Fatalf("stale closed dialog: %+v, %v", result, err)
			}
		})
	}
}

func TestHandleDialog(t *testing.T) {
	for _, test := range []struct {
		name       string
		kind       string
		operation  string
		text       *string
		wantParams string
		wantError  string
	}{
		{name: "accept alert", kind: "alert", operation: "accept", wantParams: `{"accept":true}`},
		{name: "dismiss confirm", kind: "confirm", operation: "dismiss", wantParams: `{"accept":false}`},
		{name: "accept beforeunload", kind: "beforeunload", operation: "accept", wantParams: `{"accept":true}`},
		{name: "dismiss beforeunload", kind: "beforeunload", operation: "dismiss", wantParams: `{"accept":false}`},
		{name: "prompt default", kind: "prompt", operation: "accept", wantParams: `{"accept":true,"promptText":"initial"}`},
		{name: "prompt input", kind: "prompt", operation: "accept", text: new("  answer  "), wantParams: `{"accept":true,"promptText":"  answer  "}`},
		{name: "prompt empty", kind: "prompt", operation: "accept", text: new(""), wantParams: `{"accept":true,"promptText":""}`},
		{name: "dismiss prompt", kind: "prompt", operation: "dismiss", wantParams: `{"accept":false}`},
		{name: "no dialog", operation: "accept", wantError: "no JavaScript dialog is open"},
		{name: "invalid operation", kind: "prompt", operation: "invalid", wantError: "operation must be"},
		{name: "text on get", kind: "prompt", operation: "get", text: new(""), wantError: "only supported"},
		{name: "text on dismiss", kind: "prompt", operation: "dismiss", text: new(""), wantError: "only supported"},
		{name: "text on confirm", kind: "confirm", operation: "accept", text: new(""), wantError: "only supported"},
	} {
		t.Run(test.name, func(t *testing.T) {
			backend := New()
			backend.dialog = api.DialogState{Open: test.kind != "", Type: test.kind, DefaultPrompt: "initial"}
			before := backend.dialog
			var sent string
			ctx := cdp.WithExecutor(t.Context(), dialogExecutor(func(_ context.Context, method string, params, _ any) error {
				if method != page.CommandHandleJavaScriptDialog {
					t.Fatalf("unexpected CDP method: %s", method)
				}
				encoded, err := json.Marshal(params)
				sent = string(encoded)
				return err
			}))
			action := api.Action{Args: map[string]string{"operation": test.operation}}
			if test.text != nil {
				action.Args["text"] = *test.text
			}
			result, err := backend.dialogInContext(ctx, action)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) || sent != "" || backend.dialog != before {
					t.Fatalf("invalid input changed dialog: result=%+v err=%v sent=%s state=%+v", result, err, sent, backend.dialog)
				}
				return
			}
			if err != nil || sent != test.wantParams || !result.OK || !result.Changed || backend.dialog.Open {
				t.Fatalf("unexpected handle result: %+v err=%v sent=%s state=%+v", result, err, sent, backend.dialog)
			}
		})
	}
}

func TestHandleDialogFailureAndSuccessor(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("failure=%t", fail), func(t *testing.T) {
			backend := New()
			backend.trackDialogEvent("page1", &page.EventJavascriptDialogOpening{Type: page.DialogTypeAlert, Message: "first"})
			commandErr := errors.New("CDP failure")
			ctx := cdp.WithExecutor(t.Context(), dialogExecutor(func(context.Context, string, any, any) error {
				if fail {
					return commandErr
				}
				backend.trackDialogEvent("page1", &page.EventJavascriptDialogClosed{})
				backend.trackDialogEvent("page1", &page.EventJavascriptDialogOpening{Type: page.DialogTypePrompt, Message: "second"})
				return nil
			}))
			_, err := backend.dialogInContext(ctx, api.Action{Args: map[string]string{"operation": "accept"}})
			if fail {
				if !errors.Is(err, commandErr) || backend.dialog.Message != "first" {
					t.Fatalf("failed command discarded state: %v, %+v", err, backend.dialog)
				}
			} else if err != nil || backend.dialog.Message != "second" || !backend.dialog.Open {
				t.Fatalf("successor dialog was lost: %v, %+v", err, backend.dialog)
			}
		})
	}
}

func TestDialogGuard(t *testing.T) {
	backend := New()
	backend.targetInfo.ID = "page1"
	requestCtx, requestCancel := context.WithTimeout(t.Context(), time.Second)
	defer requestCancel()
	_, err := runWithDialogGuard(backend, requestCtx, "eval", func(ctx context.Context) (int, error) {
		backend.trackDialogEvent("other", &page.EventJavascriptDialogOpening{Type: page.DialogTypeAlert})
		if ctx.Err() != nil {
			t.Fatal("another target interrupted the operation")
		}
		go backend.trackDialogEvent("page1", &page.EventJavascriptDialogOpening{Type: page.DialogTypeConfirm, Message: "Continue?"})
		<-ctx.Done()
		return 0, ctx.Err()
	})
	var blocked *javascriptDialogBlockedError
	if !errors.As(err, &blocked) || blocked.dialog.Type != "confirm" || requestCtx.Err() != nil || backend.dialogInterrupt != nil {
		t.Fatalf("dialog did not release operation cleanly: %v", err)
	}
	_, err = runWithDialogGuard(backend, requestCtx, "observe", func(context.Context) (int, error) {
		t.Fatal("operation ran while dialog was already open")
		return 0, nil
	})
	if !errors.As(err, &blocked) {
		t.Fatalf("expected immediate dialog error, got %v", err)
	}
	backend.trackDialogEvent("page1", &page.EventJavascriptDialogClosed{})
	result, err := runWithDialogGuard(backend, requestCtx, "eval", func(context.Context) (int, error) { return 42, nil })
	if err != nil || result != 42 {
		t.Fatalf("operation did not resume after dialog: %d, %v", result, err)
	}
	requestCancel()
	_, err = runWithDialogGuard(backend, requestCtx, "eval", func(context.Context) (int, error) {
		t.Fatal("canceled request ran")
		return 0, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lost request cancellation: %v", err)
	}
}

func TestDialogChromiumE2E(t *testing.T) {
	if os.Getenv("NEXUS_E2E") != "1" {
		t.Skip("set NEXUS_E2E=1 to run real chromium e2e")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("chromium e2e is only supported on darwin")
	}
	executable := resolveChromiumForE2E(t)
	if executable == "" {
		t.Skip("chromium executable not available for e2e")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/onload" {
			fmt.Fprint(w, `<!doctype html><html><body><script>window.loadResult = prompt('on load', 'initial')</script></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body>
<button id="alert" onclick="alert('clicked'); window.clicked = true">Alert</button>
<button id="leave" onclick="window.onbeforeunload = event => { event.preventDefault(); event.returnValue = ''; }">Enable leave confirmation</button>
</body></html>`)
	}))
	defer server.Close()
	backend := New()
	if err := backend.Attach(t.Context(), spec.SessionConfig{SessionID: "dialog-e2e", TargetRef: executable, Options: map[string]string{"initial_url": "about:blank"}}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := backend.Detach(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	act := func(action api.Action) (*api.ActionResult, error) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		return backend.Act(ctx, action)
	}
	mustAct := func(t *testing.T, action api.Action) *api.ActionResult {
		t.Helper()
		result, err := act(action)
		if err != nil || result == nil || !result.OK {
			t.Fatalf("action %s failed: %+v, %v", action.Kind, result, err)
		}
		return result
	}
	dialogAction := func(operation string) api.Action {
		return api.Action{Kind: "dialog", Args: map[string]string{"operation": operation}}
	}
	assertBlocked := func(t *testing.T, action api.Action) {
		t.Helper()
		_, err := act(action)
		var blocked *javascriptDialogBlockedError
		if !errors.As(err, &blocked) {
			t.Fatalf("expected dialog interruption for %s, got %v", action.Kind, err)
		}
	}
	if state := mustAct(t, dialogAction("get")).Value.(api.DialogState); state.Open {
		t.Fatalf("unexpected initial dialog: %+v", state)
	}
	mustAct(t, api.Action{Kind: "navigate", Args: map[string]string{"url": server.URL}})
	mustAct(t, api.Action{Kind: "wait", Args: map[string]string{"target": "selector", "value": "#alert", "state": "visible", "timeout_ms": "5000"}})
	for _, test := range []struct {
		name      string
		source    string
		kind      string
		operation string
		text      *string
		want      any
	}{
		{name: "alert", source: `globalThis.dialogResult = (alert('message'), 'resumed')`, kind: "alert", operation: "accept", want: "resumed"},
		{name: "confirm accept", source: `globalThis.dialogResult = confirm('message')`, kind: "confirm", operation: "accept", want: true},
		{name: "confirm dismiss", source: `globalThis.dialogResult = confirm('message')`, kind: "confirm", operation: "dismiss", want: false},
		{name: "prompt input", source: `globalThis.dialogResult = prompt('message', 'initial')`, kind: "prompt", operation: "accept", text: new(" 日本語 \n"), want: " 日本語 \n"},
		{name: "prompt empty", source: `globalThis.dialogResult = prompt('message', 'initial')`, kind: "prompt", operation: "accept", text: new(""), want: ""},
		{name: "prompt default", source: `globalThis.dialogResult = prompt('message', 'initial')`, kind: "prompt", operation: "accept", want: "initial"},
		{name: "prompt dismiss", source: `globalThis.dialogResult = prompt('message', 'initial')`, kind: "prompt", operation: "dismiss", want: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertBlocked(t, api.Action{Kind: "eval", Text: test.source})
			state := mustAct(t, dialogAction("get")).Value.(api.DialogState)
			if !state.Open || state.Type != test.kind || state.Message != "message" || state.URL != server.URL+"/" {
				t.Fatalf("incorrect dialog state: %+v", state)
			}
			handle := dialogAction(test.operation)
			if test.text != nil {
				handle.Args["text"] = *test.text
			}
			mustAct(t, handle)
			if got := mustAct(t, api.Action{Kind: "eval", Text: "globalThis.dialogResult"}).Value; !reflect.DeepEqual(got, test.want) {
				t.Fatalf("dialog response: got %#v, want %#v", got, test.want)
			}
		})
	}

	assertBlocked(t, api.Action{Kind: "invoke", Selector: "#alert"})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	_, err := backend.Observe(ctx, api.ObserveOptions{WithTree: true, WithScreenshot: true})
	cancel()
	var blocked *javascriptDialogBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("observation blocked on dialog: %v", err)
	}
	mustAct(t, dialogAction("accept"))
	if got := mustAct(t, api.Action{Kind: "eval", Text: "window.clicked"}).Value; got != true {
		t.Fatalf("click did not resume: %v", got)
	}

	assertBlocked(t, api.Action{Kind: "eval", Text: `alert('first'); alert('second')`})
	mustAct(t, dialogAction("accept"))
	deadline := time.Now().Add(2 * time.Second)
	for !mustAct(t, dialogAction("get")).Value.(api.DialogState).Open {
		if time.Now().After(deadline) {
			t.Fatal("successor dialog did not open")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if state := mustAct(t, dialogAction("get")).Value.(api.DialogState); state.Message != "second" {
		t.Fatalf("lost successor dialog: %+v", state)
	}
	mustAct(t, dialogAction("dismiss"))

	// A real mouse event gives beforeunload the sticky user activation it needs.
	mustAct(t, api.Action{Kind: "invoke", Args: map[string]string{"x": "150", "y": "18"}})
	for _, operation := range []string{"dismiss", "accept"} {
		assertBlocked(t, api.Action{Kind: "navigate", Args: map[string]string{"url": server.URL + "/next"}})
		if state := mustAct(t, dialogAction("get")).Value.(api.DialogState); state.Type != "beforeunload" {
			t.Fatalf("expected beforeunload: %+v", state)
		}
		mustAct(t, dialogAction(operation))
		wantURL := server.URL + "/"
		if operation == "accept" {
			wantURL = server.URL + "/next"
			mustAct(t, api.Action{Kind: "wait", Args: map[string]string{"target": "url", "value": "/next", "timeout_ms": "5000"}})
		}
		if got := mustAct(t, api.Action{Kind: "eval", Text: "location.href"}).Value; got != wantURL {
			t.Fatalf("beforeunload %s: got URL %v, want %s", operation, got, wantURL)
		}
	}
	if _, err := act(dialogAction("accept")); err == nil || !strings.Contains(err.Error(), "no JavaScript dialog is open") {
		t.Fatalf("expected error without a dialog: %v", err)
	}

	assertBlocked(t, api.Action{Kind: "navigate", Args: map[string]string{"url": server.URL + "/onload"}})
	if state := mustAct(t, dialogAction("get")).Value.(api.DialogState); state.Type != "prompt" || state.Message != "on load" {
		t.Fatalf("page-load dialog was not tracked: %+v", state)
	}
	loadResponse := dialogAction("accept")
	loadResponse.Args["text"] = "loaded"
	mustAct(t, loadResponse)
	if got := mustAct(t, api.Action{Kind: "eval", Text: "window.loadResult"}).Value; got != "loaded" {
		t.Fatalf("page-load script did not resume: %v", got)
	}
}
