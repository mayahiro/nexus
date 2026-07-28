package chromium

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp/kb"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/browsermgr"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

func TestAttachAndDetach(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix shell script required")
	}

	executable, argsPath, childPIDPath := writeFakeChromium(t)
	backend := New()

	err := backend.Attach(context.Background(), spec.SessionConfig{
		SessionID: "web1",
		TargetRef: executable,
		Options: map[string]string{
			"initial_url": "https://example.com",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	logs, err := backend.Logs(context.Background(), api.LogOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) == 0 {
		t.Fatal("expected logs")
	}
	if !strings.Contains(logs[0].Message, "DevTools listening on ws://127.0.0.1:9222/") {
		t.Fatalf("unexpected logs: %+v", logs)
	}

	argsData, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(argsData), "https://example.com") {
		t.Fatalf("initial url was not passed to chromium: %s", string(argsData))
	}
	if !strings.Contains(string(argsData), "--window-size=1920,1080") {
		t.Fatalf("default viewport was not passed to chromium: %s", string(argsData))
	}

	if err := backend.Detach(context.Background()); err != nil {
		t.Fatal(err)
	}
	childPID := readProcessID(t, childPIDPath)
	waitForProcessExit(t, childPID)
}

func TestCapabilities(t *testing.T) {
	capabilities := New().Capabilities()
	if !capabilities.Observe || !capabilities.Act || !capabilities.Screenshot || !capabilities.Logs || !capabilities.LayoutContext {
		t.Fatalf("unexpected capabilities: %+v", capabilities)
	}
}

func TestAttachRespectsViewportOption(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix shell script required")
	}

	executable, argsPath, _ := writeFakeChromium(t)
	backend := New()

	err := backend.Attach(context.Background(), spec.SessionConfig{
		SessionID: "web1",
		TargetRef: executable,
		Options: map[string]string{
			"viewport_width":  "1440",
			"viewport_height": "900",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Detach(context.Background())

	argsData, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(argsData), "--window-size=1440,900") {
		t.Fatalf("custom viewport was not passed to chromium: %s", string(argsData))
	}
}

func TestAttachRequiresTargetRef(t *testing.T) {
	backend := New()

	err := backend.Attach(context.Background(), spec.SessionConfig{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCurrentPageTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/list" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode([]pageTargetInfo{
			{ID: "worker", Type: "worker"},
			{ID: "page1", Type: "page", Title: "Example", URL: "https://example.com"},
		})
	}))
	defer server.Close()

	wsURL := strings.Replace(server.URL, "http://", "ws://", 1) + "/devtools/browser/test"
	target, err := currentPageTarget(context.Background(), wsURL)
	if err != nil {
		t.Fatal(err)
	}

	if target.ID != "page1" {
		t.Fatalf("unexpected target: %+v", target)
	}
}

func TestCurrentPageTargetTimesOutWhenDevToolsDoesNotRespond(t *testing.T) {
	previousTimeout := pageTargetTimeout
	pageTargetTimeout = 20 * time.Millisecond
	t.Cleanup(func() {
		pageTargetTimeout = previousTimeout
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/list" {
			http.NotFound(w, r)
			return
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	wsURL := strings.Replace(server.URL, "http://", "ws://", 1) + "/devtools/browser/test"
	_, err := currentPageTarget(context.Background(), wsURL)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected page target deadline, got: %v", err)
	}
}

func TestPageTargetContextKeepsPersistentContextAcrossOperations(t *testing.T) {
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	backend := &Backend{runCtx: runCtx}
	t.Cleanup(backend.closeRemoteContexts)

	resolveCalls := 0
	initializeCalls := 0
	var initializedTargetCtx context.Context
	operationCtx, targetInfo, release, err := backend.pageTargetContextWithDependencies(
		context.Background(),
		"ws://127.0.0.1:9222/devtools/browser/test",
		func(context.Context, string) (pageTargetInfo, error) {
			resolveCalls++
			return pageTargetInfo{
				ID:   "page1",
				Type: "page",
				URL:  "https://example.com",
			}, nil
		},
		func(_ context.Context, targetCtx context.Context) error {
			initializeCalls++
			initializedTargetCtx = targetCtx
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if operationCtx == nil {
		t.Fatal("expected operation context")
	}
	if targetInfo.ID != "page1" {
		t.Fatalf("unexpected target: %+v", targetInfo)
	}
	if resolveCalls != 1 {
		t.Fatalf("unexpected target resolve count: %d", resolveCalls)
	}
	if initializeCalls != 1 {
		t.Fatalf("unexpected target initialize count: %d", initializeCalls)
	}

	backend.mu.Lock()
	persistentTargetCtx := backend.targetCtx
	backend.mu.Unlock()
	if persistentTargetCtx == nil {
		t.Fatal("expected persistent target context")
	}
	if initializedTargetCtx != persistentTargetCtx {
		t.Fatal("target was not initialized with the persistent context")
	}

	release()
	if err := persistentTargetCtx.Err(); err != nil {
		t.Fatalf("persistent target context was canceled after first operation: %v", err)
	}

	secondOperationCtx, _, secondRelease, err := backend.pageTargetContextWithDependencies(
		context.Background(),
		"ws://127.0.0.1:9222/devtools/browser/test",
		func(context.Context, string) (pageTargetInfo, error) {
			t.Fatal("target was resolved again")
			return pageTargetInfo{}, nil
		},
		func(context.Context, context.Context) error {
			t.Fatal("target was initialized again")
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := secondOperationCtx.Err(); err != nil {
		t.Fatalf("second operation context is unavailable: %v", err)
	}
	secondRelease()
	if err := persistentTargetCtx.Err(); err != nil {
		t.Fatalf("persistent target context was canceled after second operation: %v", err)
	}
}

func TestDebugHTTPBaseURL(t *testing.T) {
	baseURL, err := debugHTTPBaseURL("ws://127.0.0.1:9222/devtools/browser/test")
	if err != nil {
		t.Fatal(err)
	}

	if baseURL != "http://127.0.0.1:9222" {
		t.Fatalf("unexpected base url: %s", baseURL)
	}
}

func TestParseTreeJSON(t *testing.T) {
	tree, err := parseTreeJSON(`[
		{
				"id": 1,
				"fingerprint": "button|button|submit|||||Submit|Submit",
				"structure_path": "html:1>body:1>form:1>button:1",
				"text_length": 6,
				"descendants": 1,
				"role": "button",
			"name": " Submit ",
			"text": " Submit ",
			"value": "",
			"styles": {"color": "rgb(0, 0, 0)"},
			"layout_context": [
				{
					"selector": "form.actions",
					"role": "form",
					"name": " Actions ",
					"styles": {"display": "flex", "gap": "8px"},
					"bounds": {"x": 0, "y": 0, "w": 100, "h": 50},
					"scrollable": false,
					"attrs": {"tag": "form", "class": "actions"}
				}
			],
			"bounds": {"x": 10, "y": 20, "w": 30, "h": 40},
			"document_bounds": {"x": 10, "y": 120, "w": 30, "h": 40},
			"visible": true,
			"enabled": true,
			"focused": false,
			"editable": false,
			"selectable": false,
			"invokable": true,
			"scrollable": false,
			"children": [2],
			"attrs": {"tag": "button"}
		},
		{
			"id": 2,
			"fingerprint": "input|textbox|search|search|||Search|Search|",
			"role": "textbox",
			"name": "Search",
			"text": "",
			"value": "hello",
			"styles": {"pointer-events": "auto"},
			"bounds": {"x": 50, "y": 60, "w": 70, "h": 80},
			"visible": true,
			"enabled": true,
			"focused": true,
			"editable": true,
			"selectable": false,
			"invokable": false,
			"scrollable": false,
			"children": [],
			"attrs": {"tag": "input", "type": "text"}
		}
	]`)
	if err != nil {
		t.Fatal(err)
	}

	if len(tree) != 2 {
		t.Fatalf("unexpected tree length: %d", len(tree))
	}
	if tree[0].Name != "Submit" {
		t.Fatalf("unexpected node: %+v", tree[0])
	}
	if tree[0].Ref != "@e1" || tree[1].Ref != "@e2" {
		t.Fatalf("expected refs: %+v", tree)
	}
	if len(tree[0].LocatorHints) == 0 || tree[0].LocatorHints[0].Kind != "role" || tree[0].LocatorHints[0].Command != `role button --name "Submit"` {
		t.Fatalf("expected locator hints: %+v", tree[0])
	}
	if len(tree[0].LocatorHints) < 2 || tree[0].LocatorHints[1].Kind != "text" || tree[0].LocatorHints[1].Command != `text "Submit"` {
		t.Fatalf("expected text locator hint: %+v", tree[0])
	}
	if len(tree[1].LocatorHints) == 0 || tree[1].LocatorHints[0].Kind != "role" || tree[1].LocatorHints[0].Command != `role textbox --name "Search"` {
		t.Fatalf("expected locator hints: %+v", tree[1])
	}
	if tree[0].Fingerprint == "" || tree[1].Fingerprint == "" {
		t.Fatalf("expected fingerprints: %+v", tree)
	}
	if tree[0].StructurePath != "html:1>body:1>form:1>button:1" || tree[0].TextLength != 6 || tree[0].Descendants != 1 {
		t.Fatalf("expected structure metadata: %+v", tree[0])
	}
	if len(tree[0].Children) != 1 || tree[0].Children[0] != 2 {
		t.Fatalf("unexpected node children: %+v", tree[0])
	}
	if tree[1].Value != "hello" || !tree[1].Focused || !tree[1].Editable {
		t.Fatalf("unexpected node: %+v", tree[1])
	}
	if tree[0].Styles["color"] != "rgb(0, 0, 0)" || tree[1].Styles["pointer-events"] != "auto" {
		t.Fatalf("unexpected node styles: %+v", tree)
	}
	if tree[0].DocumentBounds == nil || tree[0].DocumentBounds.Y != 120 {
		t.Fatalf("expected document bounds: %+v", tree[0].DocumentBounds)
	}
	if len(tree[0].LayoutContext) != 1 || tree[0].LayoutContext[0].Selector != "form.actions" || tree[0].LayoutContext[0].Styles["display"] != "flex" {
		t.Fatalf("unexpected layout context: %+v", tree[0].LayoutContext)
	}
}

func TestBuildLocatorHintsIncludesAriaLabelAndCSSFallback(t *testing.T) {
	hints := buildLocatorHints(api.Node{
		Role:     "button",
		Name:     "Points explanation",
		Selector: "html:nth-of-type(1) > body:nth-of-type(1) > button:nth-of-type(1)",
		Attrs: map[string]string{
			"aria-label": "Points explanation",
		},
	})
	kinds := map[string]bool{}
	for _, hint := range hints {
		kinds[hint.Kind] = true
	}
	if !kinds["aria-label"] || kinds["css"] {
		t.Fatalf("expected aria-label to avoid a noisy css fallback: %+v", hints)
	}

	fallback := buildLocatorHints(api.Node{
		Selector: "html:nth-of-type(1) > body:nth-of-type(1) > div:nth-of-type(1)",
	})
	if len(fallback) != 1 || fallback[0].Kind != "css" {
		t.Fatalf("expected css fallback hint: %+v", fallback)
	}
}

func TestEvalExpression(t *testing.T) {
	source := `Array.from(document.querySelectorAll("a")).map((a) => a.textContent)`
	script := evalExpression(source)

	if !strings.Contains(script, "await eval(") {
		t.Fatalf("unexpected script: %s", script)
	}
	if !strings.Contains(script, `document.querySelectorAll(\"a\")`) {
		t.Fatalf("unexpected script: %s", script)
	}
}

func TestObserveTreeExpressionNormalizesColorProperties(t *testing.T) {
	script := observeTreeExpression([]string{"color", "fill", "pointer-events"}, "", nil, "")

	if !strings.Contains(script, "normalizeStyleValue(property, style.getPropertyValue(property).trim())") {
		t.Fatalf("expected color normalization in script: %s", script)
	}
	if !strings.Contains(script, "rgba-float16") {
		t.Fatalf("expected float16 color normalization fallback in script: %s", script)
	}
	if !strings.Contains(script, "new Set(['fill', 'stroke'])") {
		t.Fatalf("expected non-suffix color properties in script: %s", script)
	}
}

func TestObserveTreeExpressionSupportsAllCSSProperties(t *testing.T) {
	script := observeTreeExpression([]string{"*"}, "", nil, "")
	if !strings.Contains(script, "properties.includes('*') ? Array.from(style) : properties") {
		t.Fatalf("expected exhaustive computed style enumeration: %s", script)
	}
}

func TestObserveTreeExpressionSupportsScopedCustomSelector(t *testing.T) {
	script := observeTreeExpressionWithSelector(nil, "#dialog", nil, "", `button[aria-label]`, true)
	for _, expected := range []string{
		`const scopeSelector = "#dialog";`,
		`const selector = "button[aria-label]";`,
		`const includeScopeRoot = false;`,
		`match selector is invalid`,
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("expected custom selector contract %q in script", expected)
		}
	}
}

func TestObserveTreeExpressionIncludesLayoutContext(t *testing.T) {
	script := observeTreeExpression(nil, "", []string{"display", "grid-template-columns"}, "")

	if !strings.Contains(script, "layout_context: layoutContextFor(el)") {
		t.Fatalf("expected layout context in script: %s", script)
	}
	if !strings.Contains(script, "grid-template-columns") {
		t.Fatalf("expected layout properties in script: %s", script)
	}
	if !strings.Contains(script, "parentElement") {
		t.Fatalf("expected ancestor traversal in script: %s", script)
	}
}

func TestObserveTreeExpressionUsesAriaLabelAsAccessibleName(t *testing.T) {
	script := observeTreeExpression(nil, "", nil, "")

	for _, expected := range []string{
		`const label = (el.getAttribute('aria-label') || '').trim();`,
		`if (label) return label;`,
		`attrs['aria-label'] = el.getAttribute('aria-label');`,
		`const name = nameFor(el);`,
		`name: name,`,
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("expected aria-label accessible name contract %q in script", expected)
		}
	}
}

func TestObserveTreeExpressionIncludesDefaultIgnoreAttrs(t *testing.T) {
	script := observeTreeExpression(nil, "", nil, "all")

	for _, value := range []string{"aria-hidden", "hidden", "data-nxctl-skip"} {
		if !strings.Contains(script, value) {
			t.Fatalf("expected %s attr in script: %s", value, script)
		}
	}
}

func TestObserveCandidateSelectorNodeScopes(t *testing.T) {
	current := observeCandidateSelector("")
	if !strings.Contains(current, "button") || strings.Contains(current, "h1") {
		t.Fatalf("unexpected current selector: %s", current)
	}

	actionable := observeCandidateSelector("actionable")
	if !strings.Contains(actionable, `[role="switch"]`) || strings.Contains(actionable, "main") {
		t.Fatalf("unexpected actionable selector: %s", actionable)
	}

	semantic := observeCandidateSelector("semantic")
	if !strings.Contains(semantic, "h1") || !strings.Contains(semantic, `[role="status"]`) || !strings.Contains(semantic, "[data-testid]") {
		t.Fatalf("unexpected semantic selector: %s", semantic)
	}

	if all := observeCandidateSelector("all"); all != "*" {
		t.Fatalf("unexpected all selector: %s", all)
	}
}

func TestScopeSelectorExpressionsIncludeCandidateHints(t *testing.T) {
	scripts := map[string]string{
		"observe": observeTreeExpression(nil, "aside.filters", nil, ""),
		"text":    scopeTextExpression("aside.filters"),
		"meta":    scopeMetaExpression("aside.filters"),
	}

	for name, script := range scripts {
		if !strings.Contains(script, "selectorHintSuffix") {
			t.Fatalf("expected selector hint suffix in %s script: %s", name, script)
		}
		if !strings.Contains(script, "matches.slice(0, 5)") {
			t.Fatalf("expected selector hints to be capped in %s script: %s", name, script)
		}
		if !strings.Contains(script, "candidates: ") {
			t.Fatalf("expected selector candidate label in %s script: %s", name, script)
		}
		if !strings.Contains(script, "bbox=") {
			t.Fatalf("expected selector candidate bounds in %s script: %s", name, script)
		}
	}
}

func TestClickExpression(t *testing.T) {
	script := clickExpression(7)

	if !strings.Contains(script, "nodeID - 1") {
		t.Fatalf("unexpected script: %s", script)
	}
	if !strings.Contains(script, "clicked") && !strings.Contains(script, "el.click()") {
		t.Fatalf("unexpected script: %s", script)
	}
	if !strings.Contains(script, "7") {
		t.Fatalf("unexpected script: %s", script)
	}
}

func TestNodePointExpression(t *testing.T) {
	script := nodePointExpression(5)

	if !strings.Contains(script, "scrollIntoView") {
		t.Fatalf("unexpected script: %s", script)
	}
	if !strings.Contains(script, "rect.left + rect.width / 2") {
		t.Fatalf("unexpected script: %s", script)
	}
	if !strings.Contains(script, "5") {
		t.Fatalf("unexpected script: %s", script)
	}
}

func TestTypeExpression(t *testing.T) {
	script := typeExpression(3, `hello "world"`)

	if !strings.Contains(script, "document.activeElement") {
		t.Fatalf("unexpected script: %s", script)
	}
	if !strings.Contains(script, "node is not editable") {
		t.Fatalf("unexpected script: %s", script)
	}
	if !strings.Contains(script, `hello \"world\"`) {
		t.Fatalf("unexpected script: %s", script)
	}
	if !strings.Contains(script, "3") {
		t.Fatalf("unexpected script: %s", script)
	}
	if !strings.Contains(script, "HTMLInputElement.prototype") || !strings.Contains(script, "valueDescriptor.set.call") {
		t.Fatalf("expected native value setter contract: %s", script)
	}
	if !strings.Contains(script, "new InputEvent('input'") || !strings.Contains(script, "composed: true") {
		t.Fatalf("expected framework-compatible input event contract: %s", script)
	}
}

func TestMarkTypeTargetExpression(t *testing.T) {
	script := markTypeTargetExpression(3, "token-1")

	if !strings.Contains(script, "data-nexus-type") {
		t.Fatalf("unexpected script: %s", script)
	}
	if !strings.Contains(script, "setSelectionRange") {
		t.Fatalf("unexpected script: %s", script)
	}
	if !strings.Contains(script, "3") {
		t.Fatalf("unexpected script: %s", script)
	}
	if !strings.Contains(script, "token-1") {
		t.Fatalf("unexpected script: %s", script)
	}
}

func TestMarkUploadSelectorExpressionSupportsHiddenInput(t *testing.T) {
	script := markUploadSelectorExpression(`input[type="file"]`, "token-1")
	if !strings.Contains(script, `input[type=\"file\"]`) ||
		!strings.Contains(script, "data-nexus-upload") ||
		strings.Contains(script, "visible(") {
		t.Fatalf("unexpected selector upload script: %s", script)
	}
	clearScript := clearMarkedUploadExpression("token-1")
	if !strings.Contains(clearScript, "removeAttribute") || !strings.Contains(clearScript, "token-1") {
		t.Fatalf("unexpected upload cleanup script: %s", clearScript)
	}
}

func TestClearMarkedTypeTargetExpression(t *testing.T) {
	script := clearMarkedTypeTargetExpression("token-1")

	if !strings.Contains(script, "removeAttribute") {
		t.Fatalf("unexpected script: %s", script)
	}
	if !strings.Contains(script, "token-1") {
		t.Fatalf("unexpected script: %s", script)
	}
}

func TestKeyProbeExpressions(t *testing.T) {
	install := installKeyProbeExpression("token-1")
	finish := finishKeyProbeExpression("token-1")

	if !strings.Contains(install, "addEventListener('keydown'") || !strings.Contains(install, "token-1") {
		t.Fatalf("unexpected key probe install script: %s", install)
	}
	if !strings.Contains(finish, "removeEventListener('keydown'") || !strings.Contains(finish, "return state.count") {
		t.Fatalf("unexpected key probe finish script: %s", finish)
	}
}

func TestStructurePathToSelector(t *testing.T) {
	selector := structurePathToSelector("html:1>body:1>main:2>button:3")
	if selector != "html:nth-of-type(1) > body:nth-of-type(1) > main:nth-of-type(2) > button:nth-of-type(3)" {
		t.Fatalf("unexpected selector: %q", selector)
	}
	for _, value := range []string{"", "html", "html:0", "html:x"} {
		if selector := structurePathToSelector(value); selector != "" {
			t.Fatalf("expected invalid structure path %q to return an empty selector, got %q", value, selector)
		}
	}
}

func TestParseNodeReference(t *testing.T) {
	nodeID, err := parseNodeReference("@e42")
	if err != nil || nodeID != 42 {
		t.Fatalf("unexpected parsed ref: id=%d err=%v", nodeID, err)
	}
	for _, value := range []string{"42", "@e0", "@ex", "@e-1"} {
		if _, err := parseNodeReference(value); err == nil {
			t.Fatalf("expected invalid ref %q to fail", value)
		}
	}
}

func TestJavascriptDialogTracking(t *testing.T) {
	backend := New()
	backend.targetInfo.ID = "page1"
	backend.trackDialogEvent("page1", &page.EventJavascriptDialogOpening{
		Type:    page.DialogTypeAlert,
		Message: "Blocked",
	})
	err := backend.javascriptDialogError()
	if err == nil || !strings.Contains(err.Error(), "alert") || !strings.Contains(err.Error(), "Blocked") {
		t.Fatalf("unexpected dialog error: %v", err)
	}
	backend.trackDialogEvent("other", &page.EventJavascriptDialogClosed{})
	if backend.javascriptDialogError() == nil {
		t.Fatal("expected events from another target to be ignored")
	}
	backend.trackDialogEvent("page1", &page.EventJavascriptDialogClosed{})
	if err := backend.javascriptDialogError(); err != nil {
		t.Fatalf("expected closed dialog state: %v", err)
	}
}

func TestValidateFullScreenshotSize(t *testing.T) {
	tests := []struct {
		name    string
		width   int64
		height  int64
		wantErr error
	}{
		{name: "within limits", width: 1000, height: 1000},
		{name: "at pixel limit", width: 12000, height: 10000},
		{name: "invalid width", width: 0, height: 1000, wantErr: errors.New("invalid")},
		{name: "width limit", width: maxFullScreenshotWidth + 1, height: 1, wantErr: errFullScreenshotTooLarge},
		{name: "height limit", width: 1, height: maxFullScreenshotHeight + 1, wantErr: errFullScreenshotTooLarge},
		{name: "pixel limit", width: 12001, height: 10000, wantErr: errFullScreenshotTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFullScreenshotSize(tt.width, tt.height)
			switch {
			case tt.wantErr == nil && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != nil && err == nil:
				t.Fatalf("expected error matching %v", tt.wantErr)
			case errors.Is(tt.wantErr, errFullScreenshotTooLarge) && !errors.Is(err, errFullScreenshotTooLarge):
				t.Fatalf("expected full screenshot limit error, got %v", err)
			case tt.wantErr != nil && !errors.Is(tt.wantErr, errFullScreenshotTooLarge) && !strings.Contains(err.Error(), tt.wantErr.Error()):
				t.Fatalf("expected error containing %q, got %v", tt.wantErr.Error(), err)
			}
		})
	}
}

func TestScreenshotAttemptContextPreservesShortRequestDeadline(t *testing.T) {
	requestCtx, requestCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer requestCancel()

	attemptCtx, attemptCancel := screenshotAttemptContext(requestCtx)
	defer attemptCancel()

	requestDeadline, requestOK := requestCtx.Deadline()
	attemptDeadline, attemptOK := attemptCtx.Deadline()
	if !requestOK || !attemptOK {
		t.Fatal("expected request and attempt deadlines")
	}
	difference := requestDeadline.Sub(attemptDeadline)
	if difference < 0 {
		difference = -difference
	}
	if difference > 10*time.Millisecond {
		t.Fatalf("short request deadline was divided or shortened: %s", difference)
	}
}

func TestReattachPageTargetIsBounded(t *testing.T) {
	backend := New()
	backend.runCtx = context.Background()
	backend.staleContexts = []remoteContext{{}}
	_, _, _, err := backend.reattachPageTarget(context.Background(), pageTargetInfo{ID: "page1"})
	if err == nil || !strings.Contains(err.Error(), "already reattached once") {
		t.Fatalf("unexpected bounded reattach error: %v", err)
	}
}

func TestHydrationBarrierWaitsForDOMQuiet(t *testing.T) {
	for _, expected := range []string{"DOMContentLoaded", "MutationObserver", "setTimeout", "requestAnimationFrame"} {
		if !strings.Contains(hydrationBarrierExpression, expected) {
			t.Fatalf("expected hydration barrier contract %q", expected)
		}
	}
}

func TestParseKeySpec(t *testing.T) {
	keyValue, modifiers, err := parseKeySpec("Enter")
	if err != nil {
		t.Fatal(err)
	}
	if keyValue != kb.Enter {
		t.Fatalf("unexpected key value: %q", keyValue)
	}
	if len(modifiers) != 0 {
		t.Fatalf("unexpected modifiers: %+v", modifiers)
	}

	keyValue, modifiers, err = parseKeySpec("Meta+L")
	if err != nil {
		t.Fatal(err)
	}
	if keyValue != "l" {
		t.Fatalf("unexpected key value: %q", keyValue)
	}
	if len(modifiers) != 1 || modifiers[0] != input.ModifierMeta {
		t.Fatalf("unexpected modifiers: %+v", modifiers)
	}

	keyValue, modifiers, err = parseKeySpec("Shift+Tab")
	if err != nil {
		t.Fatal(err)
	}
	if keyValue != kb.Tab {
		t.Fatalf("unexpected key value: %q", keyValue)
	}
	if len(modifiers) != 1 || modifiers[0] != input.ModifierShift {
		t.Fatalf("unexpected modifiers: %+v", modifiers)
	}
}

func TestScrollExpression(t *testing.T) {
	script := scrollExpression(0, "down", 0)
	if !strings.Contains(script, `"down"`) {
		t.Fatalf("unexpected script: %s", script)
	}
	if !strings.Contains(script, "window.scrollBy") {
		t.Fatalf("unexpected script: %s", script)
	}

	script = scrollExpression(4, "up", 500)
	if !strings.Contains(script, "nodeID > 0") {
		t.Fatalf("unexpected script: %s", script)
	}
	if !strings.Contains(script, "500") {
		t.Fatalf("unexpected script: %s", script)
	}
}

func TestWaitExpressions(t *testing.T) {
	script := waitTextExpression("Done")
	if !strings.Contains(script, "includes") || !strings.Contains(script, "Done") {
		t.Fatalf("unexpected wait text script: %s", script)
	}

	script, err := waitSelectorExpression(".ready", "visible")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, "querySelector") || !strings.Contains(script, ".ready") {
		t.Fatalf("unexpected wait selector script: %s", script)
	}

	script, err = waitSelectorExpression(".ready", "hidden")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, "return true") {
		t.Fatalf("unexpected wait selector hidden script: %s", script)
	}

	script, err = waitSelectorExpression(".ready", "attached")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, "!== null") {
		t.Fatalf("unexpected wait selector attached script: %s", script)
	}

	script, err = waitSelectorExpression(".ready", "detached")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, "=== null") {
		t.Fatalf("unexpected wait selector detached script: %s", script)
	}

	if _, err := waitSelectorExpression(".ready", "unknown"); err == nil {
		t.Fatal("expected error")
	}

	script = waitURLExpression("/done")
	if !strings.Contains(script, "window.location.href") || !strings.Contains(script, "/done") {
		t.Fatalf("unexpected wait url script: %s", script)
	}
}

func TestGetExpressions(t *testing.T) {
	script := getHTMLExpression(".hero")
	if !strings.Contains(script, "querySelector") || !strings.Contains(script, ".hero") {
		t.Fatalf("unexpected html script: %s", script)
	}

	script = getNodeExpression("bbox", 3)
	if !strings.Contains(script, `"bbox"`) || !strings.Contains(script, "3") {
		t.Fatalf("unexpected node script: %s", script)
	}

	script = getBBoxExpression(".hero")
	if !strings.Contains(script, "querySelector") || !strings.Contains(script, ".hero") || !strings.Contains(script, "getBoundingClientRect") {
		t.Fatalf("unexpected bbox script: %s", script)
	}

	script = getFocusedBBoxExpression(".hero")
	if !strings.Contains(script, "scrollIntoView") || !strings.Contains(script, ".hero") || !strings.Contains(script, "getBoundingClientRect") {
		t.Fatalf("unexpected focused bbox script: %s", script)
	}
}

func TestViewportOptions(t *testing.T) {
	if viewportWidth(nil) != 1920 {
		t.Fatalf("unexpected default viewport width: %d", viewportWidth(nil))
	}
	if viewportHeight(nil) != 1080 {
		t.Fatalf("unexpected default viewport height: %d", viewportHeight(nil))
	}

	options := map[string]string{
		"viewport_width":  "1440",
		"viewport_height": "900",
	}
	if viewportWidth(options) != 1440 {
		t.Fatalf("unexpected viewport width: %d", viewportWidth(options))
	}
	if viewportHeight(options) != 900 {
		t.Fatalf("unexpected viewport height: %d", viewportHeight(options))
	}
}

func TestSelectAndUploadExpressions(t *testing.T) {
	script := selectExpression(3, "two")
	if !strings.Contains(script, "SELECT") || !strings.Contains(script, "two") {
		t.Fatalf("unexpected select script: %s", script)
	}

	script = markUploadNodeExpression(4, "token-1")
	if !strings.Contains(script, "file input") && !strings.Contains(script, "data-nexus-upload") {
		t.Fatalf("unexpected upload script: %s", script)
	}
	if !strings.Contains(script, "token-1") {
		t.Fatalf("unexpected upload script: %s", script)
	}
}

func writeFakeChromium(t *testing.T) (string, string, string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "fake-chromium.sh")
	argsPath := filepath.Join(dir, "args.txt")
	childPIDPath := filepath.Join(dir, "child.pid")
	script := `#!/bin/sh
printf '%s\n' "$@" > "` + argsPath + `"
sleep 300 &
printf '%s\n' "$!" > "` + childPIDPath + `"
for arg in "$@"; do
  case "$arg" in
    --user-data-dir=*)
      user_data_dir="${arg#--user-data-dir=}"
      ;;
  esac
done
echo "DevTools listening on ws://127.0.0.1:9222/devtools/browser/test"
while true; do
  sleep 1
done
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, argsPath, childPIDPath
}

func readProcessID(t *testing.T, path string) int {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func waitForProcessExit(t *testing.T, pid int) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("process %d did not exit", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestChromiumE2E(t *testing.T) {
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
		switch r.URL.Path {
		case "/":
			fmt.Fprint(w, `<!doctype html>
<html>
<head>
  <style>
    #hover-target { width: 120px; height: 40px; background: rgb(0, 0, 255); }
    #hover-target:hover { background: rgb(255, 0, 0); }
  </style>
</head>
<body>
  <input id="name" name="name" placeholder="Name">
  <input id="email" type="email" name="email" placeholder="Email">
  <input id="replace" name="replace" value="before@example.com">
  <input id="hidden-upload" type="file" style="display:none">
  <button id="submit" onclick="document.getElementById('message').textContent = 'Hello, ' + document.getElementById('name').value">Submit</button>
  <div id="message"></div>
  <div id="hover-target" tabindex="0" onmouseenter="document.getElementById('hover-status').textContent='hovered'">Hover</div>
  <div id="hover-status"></div>
  <div id="dbl-target" ondblclick="document.getElementById('dbl-status').textContent='double clicked'">Double</div>
  <div id="dbl-status"></div>
  <div id="ctx-target" oncontextmenu="event.preventDefault(); document.getElementById('ctx-status').textContent='context menu'">Context</div>
  <div id="ctx-status"></div>
  <div id="key-status"></div>
  <div id="fill-status"></div>
  <button id="detach-loader" onclick="document.getElementById('loader').remove()">Detach Loader</button>
  <div id="loader">loading</div>
  <section id="dialog"><button id="aria-button" aria-label="Points explanation">?</button></section>
  <a id="next" href="/next">Next</a>
  <script>
    document.addEventListener('keydown', (event) => {
      document.getElementById('key-status').textContent = 'key:' + event.key
    })
    document.getElementById('replace').addEventListener('input', (event) => {
      document.getElementById('fill-status').textContent = 'filled:' + event.target.value
    })
  </script>
</body>
</html>`)
		case "/next":
			fmt.Fprint(w, `<!doctype html><html><head><title>Next</title></head><body><h1>Second</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	backend := New()
	if err := backend.Attach(context.Background(), spec.SessionConfig{
		SessionID: "web-e2e",
		TargetRef: executable,
		Options: map[string]string{
			"initial_url": server.URL,
		},
	}); err != nil {
		t.Fatal(err)
	}
	defer backend.Detach(context.Background())

	if _, err := backend.Act(context.Background(), api.Action{Kind: "wait", Args: map[string]string{"target": "url", "value": server.URL, "timeout_ms": "10000"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Act(context.Background(), api.Action{Kind: "wait", Args: map[string]string{"target": "selector", "value": "#submit", "state": "visible", "timeout_ms": "10000"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Act(context.Background(), api.Action{Kind: "wait", Args: map[string]string{"target": "hydrated", "timeout_ms": "10000"}}); err != nil {
		t.Fatal(err)
	}

	obs, err := backend.Observe(context.Background(), api.ObserveOptions{WithTree: true, WithText: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(obs.Text, "Submit") {
		t.Fatalf("unexpected observation text: %s", obs.Text)
	}
	ariaButton := requireNodeByAttrNode(t, obs.Tree, "id", "aria-button")
	if ariaButton.Name != "Points explanation" {
		t.Fatalf("unexpected aria-label accessible name: %+v", ariaButton)
	}

	allNodes, err := backend.Observe(context.Background(), api.ObserveOptions{WithTree: true, NodeScope: "all"})
	if err != nil {
		t.Fatal(err)
	}
	dialog := requireNodeByAttrNode(t, allNodes.Tree, "id", "dialog")
	scoped, err := backend.Observe(context.Background(), api.ObserveOptions{
		WithTree:         true,
		MatchSelector:    "button[aria-label]",
		WithinRef:        dialog.Ref,
		ExcludeScopeRoot: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped.Tree) != 1 || scoped.Tree[0].Attrs["id"] != "aria-button" {
		t.Fatalf("unexpected scoped selector observation: %+v", scoped.Tree)
	}
	obs, err = backend.Observe(context.Background(), api.ObserveOptions{WithTree: true, WithText: true})
	if err != nil {
		t.Fatal(err)
	}

	evalRes, err := backend.Act(context.Background(), api.Action{Kind: "eval", Text: `document.getElementById("submit").textContent.trim()`})
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := evalRes.Value.(string); !ok || value != "Submit" {
		t.Fatalf("unexpected eval value: %#v", evalRes.Value)
	}

	evalRes, err = backend.Act(context.Background(), api.Action{Kind: "eval", Text: `false`})
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := evalRes.Value.(bool); !ok || value {
		t.Fatalf("unexpected eval false value: %#v", evalRes.Value)
	}
	for expected := 1; expected <= 2; expected++ {
		evalRes, err = backend.Act(context.Background(), api.Action{
			Kind: "eval",
			Text: `globalThis.nexusCounter = (globalThis.nexusCounter || 0) + 1`,
			Args: map[string]string{"world": "persistent"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if value, ok := evalRes.Value.(float64); !ok || int(value) != expected {
			t.Fatalf("unexpected persistent eval value: %#v", evalRes.Value)
		}
	}

	if _, err := backend.Act(context.Background(), api.Action{Kind: "wait", Args: map[string]string{"target": "function", "value": `document.getElementById("submit") !== null`, "timeout_ms": "5000"}}); err != nil {
		t.Fatal(err)
	}

	nameID := requireNodeByAttr(t, obs.Tree, "id", "name")
	emailID := requireNodeByAttr(t, obs.Tree, "id", "email")
	replaceID := requireNodeByAttr(t, obs.Tree, "id", "replace")
	submitID := requireNodeByAttr(t, obs.Tree, "id", "submit")
	hoverNode := requireNodeByAttrNode(t, obs.Tree, "id", "hover-target")
	hoverID := hoverNode.ID
	dblID := requireNodeByAttr(t, obs.Tree, "id", "dbl-target")
	ctxID := requireNodeByAttr(t, obs.Tree, "id", "ctx-target")
	detachID := requireNodeByAttr(t, obs.Tree, "id", "detach-loader")
	nextNode := requireNodeByAttrNode(t, obs.Tree, "id", "next")
	nextID := nextNode.ID

	typeResult, err := backend.Act(context.Background(), api.Action{Kind: "type", NodeID: &nameID, Text: "hiro"})
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := typeResult.Value.(map[string]interface{}); !ok || value["delivery_verified"] != true {
		t.Fatalf("type delivery was not verified: %#v", typeResult.Value)
	}
	if _, err := backend.Act(context.Background(), api.Action{Kind: "type", NodeID: &emailID, Text: "user@example.com"}); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Act(context.Background(), api.Action{Kind: "fill", NodeID: &replaceID, Text: "after@example.com"}); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Act(context.Background(), api.Action{Kind: "wait", Args: map[string]string{"target": "text", "value": "filled:after@example.com", "timeout_ms": "5000"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Act(context.Background(), api.Action{Kind: "invoke", NodeID: &submitID}); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Act(context.Background(), api.Action{Kind: "wait", Args: map[string]string{"target": "text", "value": "Hello, hiro", "timeout_ms": "5000"}}); err != nil {
		t.Fatal(err)
	}

	res, err := backend.Act(context.Background(), api.Action{Kind: "get", Args: map[string]string{"target": "html", "selector": "#message"}})
	if err != nil {
		t.Fatal(err)
	}
	if value, _ := res.Value.(string); !strings.Contains(value, "Hello, hiro") {
		t.Fatalf("unexpected message value: %#v", res.Value)
	}

	res, err = backend.Act(context.Background(), api.Action{Kind: "get", Args: map[string]string{"target": "value", "selector": "#email"}})
	if err != nil {
		t.Fatal(err)
	}
	if value, _ := res.Value.(string); value != "user@example.com" {
		t.Fatalf("unexpected email value: %#v", res.Value)
	}

	res, err = backend.Act(context.Background(), api.Action{Kind: "get", Args: map[string]string{"target": "value", "selector": "#replace"}})
	if err != nil {
		t.Fatal(err)
	}
	if value, _ := res.Value.(string); value != "after@example.com" {
		t.Fatalf("unexpected fill value: %#v", res.Value)
	}

	uploadPath := filepath.Join(t.TempDir(), "artifact.txt")
	if err := os.WriteFile(uploadPath, []byte("upload"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Act(context.Background(), api.Action{
		Kind:     "upload",
		Selector: "#hidden-upload",
		Text:     uploadPath,
	}); err != nil {
		t.Fatal(err)
	}
	res, err = backend.Act(context.Background(), api.Action{
		Kind: "eval",
		Text: `document.getElementById("hidden-upload").files[0].name`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if value, _ := res.Value.(string); value != "artifact.txt" {
		t.Fatalf("unexpected uploaded file: %#v", res.Value)
	}

	if _, err := backend.Act(context.Background(), api.Action{Kind: "hover", NodeID: &hoverID}); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Act(context.Background(), api.Action{Kind: "wait", Args: map[string]string{"target": "text", "value": "hovered", "timeout_ms": "5000"}}); err != nil {
		t.Fatal(err)
	}
	hoverScreenshot, err := backend.Observe(context.Background(), api.ObserveOptions{WithScreenshot: true})
	if err != nil {
		t.Fatal(err)
	}
	image, err := png.Decode(bytes.NewReader(hoverScreenshot.ScreenshotData))
	if err != nil {
		t.Fatal(err)
	}
	sample := color.RGBAModel.Convert(image.At(int(hoverNode.Bounds.X)+5, int(hoverNode.Bounds.Y)+5)).(color.RGBA)
	if sample.R < 200 || sample.G > 80 || sample.B > 80 {
		t.Fatalf("hover style was not preserved in screenshot: %+v", sample)
	}
	if _, err := backend.Act(context.Background(), api.Action{
		Kind:    "get",
		NodeID:  &hoverID,
		NodeRef: hoverNode.Ref,
		Args:    map[string]string{"target": "text"},
	}); err != nil {
		t.Fatalf("tree-less screenshot invalidated a current ref: %v", err)
	}

	if _, err := backend.Act(context.Background(), api.Action{Kind: "dblclick", NodeID: &dblID}); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Act(context.Background(), api.Action{Kind: "wait", Args: map[string]string{"target": "text", "value": "double clicked", "timeout_ms": "5000"}}); err != nil {
		t.Fatal(err)
	}

	if _, err := backend.Act(context.Background(), api.Action{Kind: "rightclick", NodeID: &ctxID}); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Act(context.Background(), api.Action{Kind: "wait", Args: map[string]string{"target": "text", "value": "context menu", "timeout_ms": "5000"}}); err != nil {
		t.Fatal(err)
	}

	if _, err := backend.Act(context.Background(), api.Action{Kind: "invoke", NodeID: &detachID}); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Act(context.Background(), api.Action{Kind: "wait", Args: map[string]string{"target": "selector", "value": "#loader", "state": "detached", "timeout_ms": "5000"}}); err != nil {
		t.Fatal(err)
	}

	if _, err := backend.Act(context.Background(), api.Action{Kind: "invoke", NodeID: &nextID}); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Act(context.Background(), api.Action{Kind: "wait", Args: map[string]string{"target": "url", "value": "/next", "timeout_ms": "5000"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Act(context.Background(), api.Action{Kind: "invoke", NodeID: &nextID, NodeRef: nextNode.Ref}); err == nil || !strings.Contains(err.Error(), "stale node ref") {
		t.Fatalf("expected stale ref after navigation, got %v", err)
	}
	if _, err := backend.Act(context.Background(), api.Action{Kind: "back"}); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Act(context.Background(), api.Action{Kind: "wait", Args: map[string]string{"target": "url", "value": server.URL, "timeout_ms": "5000"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Act(context.Background(), api.Action{Kind: "navigate", Args: map[string]string{"url": server.URL + "/next"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Act(context.Background(), api.Action{Kind: "wait", Args: map[string]string{"target": "url", "value": "/next", "timeout_ms": "5000"}}); err != nil {
		t.Fatal(err)
	}
}

func resolveChromiumForE2E(t *testing.T) string {
	t.Helper()

	if path := strings.TrimSpace(os.Getenv("NEXUS_E2E_CHROMIUM_PATH")); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	paths, err := config.DefaultPaths()
	if err == nil {
		if installation, err := browsermgr.New(paths).Resolve(browsermgr.BrowserChromium); err == nil {
			return installation.ExecutablePath
		}
	}

	systemPaths := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	}
	for _, path := range systemPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

func requireNodeByAttr(t *testing.T, nodes []api.Node, key string, value string) int {
	t.Helper()

	return requireNodeByAttrNode(t, nodes, key, value).ID
}

func requireNodeByAttrNode(t *testing.T, nodes []api.Node, key string, value string) api.Node {
	t.Helper()

	for _, node := range nodes {
		if node.Attrs[key] == value {
			if node.Fingerprint == "" {
				t.Fatalf("expected fingerprint for node %s", value)
			}
			return node
		}
	}

	t.Fatalf("node with %s=%s not found", key, value)
	return api.Node{}
}
