package browsermgr

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/mayahiro/nexus/internal/config"
)

func TestSetupAndStatus(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("browser setup is macOS only")
	}

	server := newBrowserTestServer(t)
	defer server.Close()

	paths := testPaths(t)
	manager := New(paths)
	manager.client = server.Client()
	manager.chromeVersionsURL = server.URL + "/chrome.json"
	manager.lightpandaLatestURL = server.URL + "/lightpanda.json"
	manager.now = func() time.Time { return time.Unix(100, 0).UTC() }

	result, err := manager.Setup(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Browsers) != 2 {
		t.Fatalf("unexpected browser count: %d", len(result.Browsers))
	}

	status, err := manager.Status()
	if err != nil {
		t.Fatal(err)
	}

	if len(status.Browsers) != 2 {
		t.Fatalf("unexpected status count: %d", len(status.Browsers))
	}

	for _, browser := range status.Browsers {
		if !browser.Installed {
			t.Fatalf("browser not installed: %+v", browser)
		}
		if _, err := os.Stat(browser.ExecutablePath); err != nil {
			t.Fatalf("missing executable: %v", err)
		}
	}

	resolved, err := manager.Resolve(BrowserChromium)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.Installed || resolved.ExecutablePath == "" {
		t.Fatalf("unexpected resolved browser: %+v", resolved)
	}
}

func TestUpdateReplacesVersions(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("browser setup is macOS only")
	}

	state := &testServerState{
		chromiumVersion:   "1.0.0",
		lightpandaVersion: "v0.1.0",
	}
	server := newBrowserUpdateServer(t, state)
	defer server.Close()

	paths := testPaths(t)
	manager := New(paths)
	manager.client = server.Client()
	manager.chromeVersionsURL = server.URL + "/chrome.json"
	manager.lightpandaLatestURL = server.URL + "/lightpanda.json"

	if _, err := manager.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}

	state.chromiumVersion = "2.0.0"
	state.lightpandaVersion = "v0.2.0"

	result, err := manager.Update(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if result.Browsers[0].Version != "2.0.0" {
		t.Fatalf("unexpected chromium version: %+v", result.Browsers[0])
	}
	if result.Browsers[1].Version != "v0.2.0" {
		t.Fatalf("unexpected lightpanda version: %+v", result.Browsers[1])
	}
}

func TestResolveMissingBrowser(t *testing.T) {
	paths := testPaths(t)
	manager := New(paths)

	_, err := manager.Resolve(BrowserChromium)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrBrowserNotInstalled) {
		t.Fatalf("unexpected error: %v", err)
	}
}

type testServerState struct {
	chromiumVersion   string
	lightpandaVersion string
}

func newBrowserTestServer(t *testing.T) *httptest.Server {
	return newBrowserUpdateServer(t, &testServerState{
		chromiumVersion:   "1.0.0",
		lightpandaVersion: "v0.1.0",
	})
}

func newBrowserUpdateServer(t *testing.T, state *testServerState) *httptest.Server {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("/chrome.json", func(w http.ResponseWriter, r *http.Request) {
		payload := map[string]any{
			"channels": map[string]any{
				"Stable": map[string]any{
					"version": state.chromiumVersion,
					"downloads": map[string]any{
						"chrome": []map[string]string{
							{
								"platform": chromePlatform(),
								"url":      serverURL(r) + "/chromium.zip",
							},
						},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(payload)
	})
	handler.HandleFunc("/lightpanda.json", func(w http.ResponseWriter, r *http.Request) {
		payload := map[string]any{
			"tag_name": state.lightpandaVersion,
			"assets": []map[string]string{
				{
					"name":                 lightpandaAssetName(),
					"browser_download_url": serverURL(r) + "/lightpanda",
				},
			},
		}
		json.NewEncoder(w).Encode(payload)
	})
	handler.HandleFunc("/chromium.zip", func(w http.ResponseWriter, r *http.Request) {
		writeZip(t, w, state.chromiumVersion)
	})
	handler.HandleFunc("/lightpanda", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("#!/bin/sh\necho " + state.lightpandaVersion + "\n"))
	})

	return httptest.NewServer(handler)
}

func writeZip(t *testing.T, w http.ResponseWriter, version string) {
	t.Helper()

	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	path := filepath.Join("chrome-mac-arm64", "Google Chrome for Testing.app", "Contents", "MacOS", "Google Chrome for Testing")
	file, err := zipWriter.CreateHeader(&zip.FileHeader{
		Name:   path,
		Method: zip.Deflate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(version)); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	w.Write(buf.Bytes())
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}

func testPaths(t *testing.T) config.Paths {
	t.Helper()

	root, err := os.MkdirTemp("/tmp", "nexus-browsermgr-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.RemoveAll(root)
	})

	return config.Paths{
		Data:  filepath.Join(root, "data"),
		Cache: filepath.Join(root, "cache"),
	}
}
