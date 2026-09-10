package comparecmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/browsermgr"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/target/browser/chromium"
	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

func TestCompareChromiumE2E(t *testing.T) {
	if os.Getenv("NEXUS_E2E") != "1" || runtime.GOOS != "darwin" {
		t.Skip("set NEXUS_E2E=1 on macOS to run real Chromium comparison tests")
	}
	executable := strings.TrimSpace(os.Getenv("NEXUS_E2E_CHROMIUM_PATH"))
	if executable == "" {
		paths, err := config.DefaultPaths()
		if err != nil {
			t.Fatal(err)
		}
		installation, err := browsermgr.New(paths).Resolve(browsermgr.BrowserChromium)
		if err != nil {
			t.Fatal(err)
		}
		executable = installation.ExecutablePath
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<!doctype html><html><head><title>Comparison fixture</title></head><body>
<label><input id="check" type="checkbox" value="yes">Consent</label>
<select id="select" multiple size="2"><option value="same" selected>First</option><option value="same">Second</option></select>
<div role="switch" aria-checked="false">Toggle</div><div role="menuitem">Menu</div>
<a id="docs" href="/docs">Docs</a>
<main id="container">Account <span data-testid="account"><span id="secret">account-old@example.invalid</span></span></main>
</body></html>`)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	b := chromium.New()
	defer func() {
		if err := b.Detach(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	if err := b.Attach(ctx, spec.SessionConfig{SessionID: "compare-e2e", TargetRef: executable, Options: map[string]string{"initial_url": server.URL}}); err != nil {
		t.Fatal(err)
	}
	act := func(action api.Action) {
		t.Helper()
		if _, err := b.Act(ctx, action); err != nil {
			t.Fatal(err)
		}
	}
	act(api.Action{Kind: "wait", Args: map[string]string{"target": "selector", "value": "#check", "state": "visible", "timeout_ms": "5000"}})
	observe := func(scope, selector string) api.Observation {
		t.Helper()
		observation, err := b.Observe(ctx, api.ObserveOptions{WithTree: true, WithText: true, NodeScope: scope, ScopeSelector: selector})
		if err != nil {
			t.Fatal(err)
		}
		return *observation
	}
	t.Run("native-and-aria-states", func(t *testing.T) {
		before := observe("actionable", "")
		act(api.Action{Kind: "eval", Text: `document.querySelector('#check').checked = true; document.querySelector('#check').indeterminate = true; document.querySelector('#select').options[1].selected = true; document.querySelector('[role=switch]').setAttribute('aria-checked', 'mixed')`})
		after := observe("actionable", "")
		for _, mode := range []string{"exact", "stable", "heuristic", "histogram"} {
			report := buildCompareReport(buildCompareSnapshot(before, compareSnapshotOptions{}), buildCompareSnapshot(after, compareSnapshotOptions{}), nil, mode)
			states := map[string]bool{}
			for _, finding := range report.Findings {
				if finding.Kind == "state_changed" {
					states[finding.Field] = true
				}
			}
			for _, field := range []string{"checked", "indeterminate", "selected_indices", "aria-checked"} {
				if !states[field] {
					t.Fatalf("%s missed %s: %+v", mode, field, report.Findings)
				}
			}
		}
	})
	t.Run("semantic-includes-actionable", func(t *testing.T) {
		actionable, semantic := observe("actionable", ""), observe("semantic", "")
		paths := map[string]bool{}
		for _, node := range semantic.Tree {
			paths[node.StructurePath] = true
		}
		for _, node := range actionable.Tree {
			if !paths[node.StructurePath] {
				t.Fatalf("semantic omitted %s", node.Role)
			}
		}
	})
	t.Run("href-change", func(t *testing.T) {
		before := observe("current", "")
		act(api.Action{Kind: "eval", Text: `document.querySelector('#docs').setAttribute('href', '/billing')`})
		after := observe("current", "")
		for _, mode := range []string{"exact", "stable", "heuristic", "histogram"} {
			report := buildCompareReport(buildCompareSnapshot(before, compareSnapshotOptions{}), buildCompareSnapshot(after, compareSnapshotOptions{}), nil, mode)
			if report.Summary.Same {
				t.Fatalf("%s missed href change", mode)
			}
			if mode != "exact" && report.Summary.AttributeChanged != 1 {
				t.Fatalf("%s did not identify href: %+v", mode, report.Findings)
			}
		}
	})
	t.Run("masked-subtree", func(t *testing.T) {
		before := observe("all", "#container")
		act(api.Action{Kind: "eval", Text: `document.querySelector('#secret').textContent = 'account-new@example.invalid'`})
		after := observe("all", "#container")
		rules, err := compileCompareSelectorRules([]string{"testid=account"})
		if err != nil {
			t.Fatal(err)
		}
		for _, ignore := range []bool{false, true} {
			options := compareSnapshotOptions{NodeScope: "all", MaskNode: rules}
			if ignore {
				options.MaskNode, options.IgnoreNode = nil, rules
			}
			for _, mode := range []string{"exact", "stable", "heuristic", "histogram"} {
				report := buildCompareReportWithDebug(buildCompareSnapshot(before, options), buildCompareSnapshot(after, options), nil, mode, true)
				if !report.Summary.Same {
					t.Fatalf("%s ignore=%v leaked differences: %+v", mode, ignore, report.Findings)
				}
				payload, err := json.Marshal(report)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(payload), "account-old@") || strings.Contains(string(payload), "account-new@") {
					t.Fatalf("unmasked snapshot: %s", payload)
				}
			}
		}
	})
	t.Run("unresolved-promise-timeout", func(t *testing.T) {
		waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Second)
		defer waitCancel()
		started := time.Now()
		_, err := b.Act(waitCtx, api.Action{Kind: "wait", Args: map[string]string{"target": "function", "value": "new Promise(() => {})", "timeout_ms": "25"}})
		if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "25ms") {
			t.Fatalf("unexpected wait result: %v", err)
		}
		if time.Since(started) > time.Second {
			t.Fatal("wait ignored its deadline")
		}
		act(api.Action{Kind: "wait", Args: map[string]string{"target": "function", "value": "Promise.resolve(true)", "timeout_ms": "1000"}})
	})
}
