package chromium

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chromedp/cdproto/input"
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

	executable, argsPath := writeFakeChromium(t)
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
}

func TestAttachRespectsViewportOption(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix shell script required")
	}

	executable, argsPath := writeFakeChromium(t)
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
			"role": "button",
			"name": " Submit ",
			"text": " Submit ",
			"value": "",
			"bounds": {"x": 10, "y": 20, "w": 30, "h": 40},
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
	if len(tree[0].Children) != 1 || tree[0].Children[0] != 2 {
		t.Fatalf("unexpected node children: %+v", tree[0])
	}
	if tree[1].Value != "hello" || !tree[1].Focused || !tree[1].Editable {
		t.Fatalf("unexpected node: %+v", tree[1])
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

func TestClearMarkedTypeTargetExpression(t *testing.T) {
	script := clearMarkedTypeTargetExpression("token-1")

	if !strings.Contains(script, "removeAttribute") {
		t.Fatalf("unexpected script: %s", script)
	}
	if !strings.Contains(script, "token-1") {
		t.Fatalf("unexpected script: %s", script)
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

func writeFakeChromium(t *testing.T) (string, string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "fake-chromium.sh")
	argsPath := filepath.Join(dir, "args.txt")
	script := `#!/bin/sh
printf '%s\n' "$@" > "` + argsPath + `"
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
	return path, argsPath
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
<body>
  <input id="name" name="name" placeholder="Name">
  <button id="submit" onclick="document.getElementById('message').textContent = 'Hello, ' + document.getElementById('name').value">Submit</button>
  <div id="message"></div>
  <div id="hover-target" tabindex="0" onmouseenter="document.getElementById('hover-status').textContent='hovered'">Hover</div>
  <div id="hover-status"></div>
  <div id="dbl-target" ondblclick="document.getElementById('dbl-status').textContent='double clicked'">Double</div>
  <div id="dbl-status"></div>
  <div id="ctx-target" oncontextmenu="event.preventDefault(); document.getElementById('ctx-status').textContent='context menu'">Context</div>
  <div id="ctx-status"></div>
  <button id="detach-loader" onclick="document.getElementById('loader').remove()">Detach Loader</button>
  <div id="loader">loading</div>
  <a id="next" href="/next">Next</a>
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

	obs, err := backend.Observe(context.Background(), api.ObserveOptions{WithTree: true, WithText: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(obs.Text, "Submit") {
		t.Fatalf("unexpected observation text: %s", obs.Text)
	}

	nameID := requireNodeByAttr(t, obs.Tree, "id", "name")
	submitID := requireNodeByAttr(t, obs.Tree, "id", "submit")
	hoverID := requireNodeByAttr(t, obs.Tree, "id", "hover-target")
	dblID := requireNodeByAttr(t, obs.Tree, "id", "dbl-target")
	ctxID := requireNodeByAttr(t, obs.Tree, "id", "ctx-target")
	detachID := requireNodeByAttr(t, obs.Tree, "id", "detach-loader")
	nextID := requireNodeByAttr(t, obs.Tree, "id", "next")

	if _, err := backend.Act(context.Background(), api.Action{Kind: "type", NodeID: &nameID, Text: "hiro"}); err != nil {
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

	if _, err := backend.Act(context.Background(), api.Action{Kind: "hover", NodeID: &hoverID}); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Act(context.Background(), api.Action{Kind: "wait", Args: map[string]string{"target": "text", "value": "hovered", "timeout_ms": "5000"}}); err != nil {
		t.Fatal(err)
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
	if _, err := backend.Act(context.Background(), api.Action{Kind: "back"}); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Act(context.Background(), api.Action{Kind: "wait", Args: map[string]string{"target": "url", "value": server.URL, "timeout_ms": "5000"}}); err != nil {
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

	for _, node := range nodes {
		if node.Attrs[key] == value {
			if node.Fingerprint == "" {
				t.Fatalf("expected fingerprint for node %s", value)
			}
			return node.ID
		}
	}

	t.Fatalf("node with %s=%s not found", key, value)
	return 0
}
