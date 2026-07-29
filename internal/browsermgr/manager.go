package browsermgr

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mayahiro/nexus/internal/config"
)

const BrowserChromium = "chromium"

var ErrUnsupportedPlatform = errors.New("unsupported platform")
var ErrBrowserNotInstalled = errors.New("browser not installed")
var ErrUnknownBrowser = errors.New("unknown browser")

type Installation struct {
	Name           string    `json:"name"`
	Version        string    `json:"version,omitempty"`
	ExecutablePath string    `json:"executable_path,omitempty"`
	Installed      bool      `json:"installed"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
}

type Status struct {
	Browsers []Installation `json:"browsers"`
}

type SetupResult struct {
	Browsers []InstallResult `json:"browsers"`
}

type UninstallResult struct {
	Browsers []InstallResult `json:"browsers"`
}

type InstallResult struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	ExecutablePath string `json:"executable_path"`
	Changed        bool   `json:"changed"`
}

type manifest struct {
	Browsers map[string]Installation `json:"browsers"`
}

type Manager struct {
	paths             config.Paths
	client            *http.Client
	chromeVersionsURL string
	now               func() time.Time
}

type chromeVersionsResponse struct {
	Channels map[string]struct {
		Version   string `json:"version"`
		Downloads struct {
			Chrome []struct {
				Platform string `json:"platform"`
				URL      string `json:"url"`
			} `json:"chrome"`
		} `json:"downloads"`
	} `json:"channels"`
}

func New(paths config.Paths) *Manager {
	return &Manager{
		paths:             paths,
		client:            &http.Client{Timeout: 2 * time.Minute},
		chromeVersionsURL: "https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json",
		now:               time.Now,
	}
}

func (m *Manager) Setup(ctx context.Context) (SetupResult, error) {
	return m.install(ctx, false)
}

func (m *Manager) Update(ctx context.Context) (SetupResult, error) {
	return m.install(ctx, true)
}

func (m *Manager) Uninstall(_ context.Context, names ...string) (UninstallResult, error) {
	manifest, err := m.loadManifest()
	if err != nil {
		return UninstallResult{}, err
	}

	targets, err := normalizeBrowserNames(names)
	if err != nil {
		return UninstallResult{}, err
	}

	results := make([]InstallResult, 0, len(targets))
	for _, name := range targets {
		installation := manifest.Browsers[name]
		targetDir := filepath.Join(m.browserRootDir(), name)
		if err := os.RemoveAll(targetDir); err != nil {
			return UninstallResult{}, err
		}
		delete(manifest.Browsers, name)
		results = append(results, InstallResult{
			Name:           name,
			Version:        installation.Version,
			ExecutablePath: installation.ExecutablePath,
			Changed:        installation.ExecutablePath != "",
		})
	}

	if err := m.saveManifest(manifest); err != nil {
		return UninstallResult{}, err
	}

	return UninstallResult{Browsers: results}, nil
}

func (m *Manager) Status() (Status, error) {
	manifest, err := m.loadManifest()
	if err != nil {
		return Status{}, err
	}

	names := []string{BrowserChromium}
	browsers := make([]Installation, 0, len(names))
	for _, name := range names {
		installation := Installation{Name: name}
		if installed, ok := manifest.Browsers[name]; ok {
			installation = installed
			installation.Installed = installation.ExecutableExists()
		}
		browsers = append(browsers, installation)
	}

	return Status{Browsers: browsers}, nil
}

func (m *Manager) Resolve(name string) (Installation, error) {
	switch name {
	case BrowserChromium:
	default:
		return Installation{}, fmt.Errorf("%w: %s", ErrUnknownBrowser, name)
	}

	manifest, err := m.loadManifest()
	if err != nil {
		return Installation{}, err
	}

	installation, ok := manifest.Browsers[name]
	if !ok || !installation.ExecutableExists() {
		return Installation{}, fmt.Errorf("%w: %s", ErrBrowserNotInstalled, name)
	}

	installation.Installed = true
	return installation, nil
}

func (m *Manager) install(ctx context.Context, force bool) (SetupResult, error) {
	if runtime.GOOS != "darwin" {
		return SetupResult{}, fmt.Errorf("%w: %s", ErrUnsupportedPlatform, runtime.GOOS)
	}

	if err := os.MkdirAll(m.browserRootDir(), 0o755); err != nil {
		return SetupResult{}, err
	}
	if err := os.MkdirAll(m.downloadCacheDir(), 0o755); err != nil {
		return SetupResult{}, err
	}

	manifest, err := m.loadManifest()
	if err != nil {
		return SetupResult{}, err
	}

	chromium, err := m.installChromium(ctx, manifest, force)
	if err != nil {
		return SetupResult{}, err
	}
	manifest.Browsers[BrowserChromium] = chromium.installation

	if err := m.saveManifest(manifest); err != nil {
		return SetupResult{}, err
	}

	return SetupResult{
		Browsers: []InstallResult{chromium.result},
	}, nil
}

func (m *Manager) installChromium(ctx context.Context, manifest manifest, force bool) (installState, error) {
	version, downloadURL, err := m.resolveChromium(ctx)
	if err != nil {
		return installState{}, err
	}

	if current, ok := manifest.Browsers[BrowserChromium]; ok && !force && current.Version == version && current.ExecutableExists() {
		return installState{
			installation: current,
			result: InstallResult{
				Name:           BrowserChromium,
				Version:        current.Version,
				ExecutablePath: current.ExecutablePath,
			},
		}, nil
	}

	archivePath, err := m.downloadFile(ctx, downloadURL, filepath.Join(m.downloadCacheDir(), "chromium-"+version+".zip"))
	if err != nil {
		return installState{}, err
	}
	defer os.Remove(archivePath)

	stageDir, err := os.MkdirTemp(m.browserRootDir(), "chromium-stage-")
	if err != nil {
		return installState{}, err
	}
	defer os.RemoveAll(stageDir)

	if err := unzip(archivePath, stageDir); err != nil {
		return installState{}, err
	}

	executablePath, err := findChromiumExecutable(stageDir)
	if err != nil {
		return installState{}, err
	}

	targetDir := filepath.Join(m.browserRootDir(), BrowserChromium)
	if err := replaceDir(stageDir, targetDir); err != nil {
		return installState{}, err
	}

	installation := Installation{
		Name:           BrowserChromium,
		Version:        version,
		ExecutablePath: strings.Replace(executablePath, stageDir, targetDir, 1),
		Installed:      true,
		UpdatedAt:      m.now(),
	}

	return installState{
		installation: installation,
		result: InstallResult{
			Name:           BrowserChromium,
			Version:        version,
			ExecutablePath: installation.ExecutablePath,
			Changed:        true,
		},
	}, nil
}

type installState struct {
	installation Installation
	result       InstallResult
}

func (m *Manager) resolveChromium(ctx context.Context) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.chromeVersionsURL, nil)
	if err != nil {
		return "", "", err
	}

	var payload chromeVersionsResponse
	if err := m.doJSON(req, &payload); err != nil {
		return "", "", err
	}

	channel, ok := payload.Channels["Stable"]
	if !ok {
		return "", "", errors.New("stable chromium channel not found")
	}

	platform := chromePlatform()
	for _, download := range channel.Downloads.Chrome {
		if download.Platform == platform {
			return channel.Version, download.URL, nil
		}
	}

	return "", "", errors.New("chromium download not found for current macos platform")
}

func (m *Manager) doJSON(req *http.Request, target interface{}) error {
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected response status: %s", resp.Status)
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

func (m *Manager) downloadFile(ctx context.Context, url string, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "nexus")

	resp, err := m.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected response status: %s", resp.Status)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return "", err
	}

	return path, nil
}

func (m *Manager) loadManifest() (manifest, error) {
	path := m.manifestPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return manifest{Browsers: map[string]Installation{}}, nil
		}
		return manifest{}, err
	}

	var result manifest
	if err := json.Unmarshal(data, &result); err != nil {
		return manifest{}, err
	}
	if result.Browsers == nil {
		result.Browsers = map[string]Installation{}
	}
	return result, nil
}

func (m *Manager) saveManifest(manifest manifest) error {
	if err := os.MkdirAll(filepath.Dir(m.manifestPath()), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.manifestPath(), data, 0o644)
}

func (m *Manager) browserRootDir() string {
	return filepath.Join(m.paths.Data, "browser")
}

func (m *Manager) downloadCacheDir() string {
	return filepath.Join(m.paths.Cache, "downloads")
}

func (m *Manager) manifestPath() string {
	return filepath.Join(m.browserRootDir(), "manifest.json")
}

func (i Installation) ExecutableExists() bool {
	if i.ExecutablePath == "" {
		return false
	}
	info, err := os.Stat(i.ExecutablePath)
	return err == nil && !info.IsDir()
}

func chromePlatform() string {
	switch runtime.GOARCH {
	case "arm64":
		return "mac-arm64"
	case "amd64":
		return "mac-x64"
	default:
		return ""
	}
}

func normalizeBrowserNames(names []string) ([]string, error) {
	if len(names) == 0 {
		return []string{BrowserChromium}, nil
	}

	targets := make([]string, 0, len(names))
	seen := map[string]struct{}{}
	for _, name := range names {
		switch name {
		case BrowserChromium:
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			targets = append(targets, name)
		default:
			return nil, fmt.Errorf("%w: %s", ErrUnknownBrowser, name)
		}
	}

	return targets, nil
}

func unzip(src string, dst string) error {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		target := filepath.Join(dst, file.Name)
		if !strings.HasPrefix(target, filepath.Clean(dst)+string(os.PathSeparator)) && filepath.Clean(target) != filepath.Clean(dst) {
			return errors.New("invalid zip entry path")
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		in, err := file.Open()
		if err != nil {
			return err
		}

		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.Mode())
		if err != nil {
			in.Close()
			return err
		}

		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			in.Close()
			return err
		}

		if err := out.Close(); err != nil {
			in.Close()
			return err
		}
		if err := in.Close(); err != nil {
			return err
		}
	}

	return nil
}

func findChromiumExecutable(root string) (string, error) {
	var found string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, filepath.Join("Contents", "MacOS", "Google Chrome for Testing")) {
			found = path
			return io.EOF
		}
		return nil
	})
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if found == "" {
		return "", errors.New("chromium executable not found in archive")
	}
	return found, nil
}

func replaceDir(stageDir string, targetDir string) error {
	if err := os.RemoveAll(targetDir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
		return err
	}
	return os.Rename(stageDir, targetDir)
}
