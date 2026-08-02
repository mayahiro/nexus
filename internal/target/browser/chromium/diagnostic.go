package chromium

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/diagnostic"
)

const (
	maxBrowserOutputLines = 64
	maxBrowserOutputBytes = 4096
)

type browserOutputLine struct {
	stream  string
	message string
}

type browserOutputBuffer struct {
	mu         sync.Mutex
	lines      []browserOutputLine
	dropped    int
	redactions []string
}

func newBrowserOutputBuffer(redactions ...string) *browserOutputBuffer {
	cleaned := make([]string, 0, len(redactions))
	for _, value := range redactions {
		value = strings.TrimSpace(value)
		if value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return &browserOutputBuffer{
		lines:      make([]browserOutputLine, 0, maxBrowserOutputLines),
		redactions: cleaned,
	}
}

func (b *browserOutputBuffer) add(stream string, message string) {
	if b == nil {
		return
	}
	message = strings.TrimSpace(strings.ToValidUTF8(message, "?"))
	for _, redaction := range b.redactions {
		message = strings.ReplaceAll(message, redaction, "<redacted-path>")
	}
	if message == "" {
		return
	}
	if len(message) > maxBrowserOutputBytes {
		message = message[:maxBrowserOutputBytes] + "..."
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	line := browserOutputLine{stream: strings.TrimSpace(stream), message: message}
	if len(b.lines) == maxBrowserOutputLines {
		copy(b.lines, b.lines[1:])
		b.lines[len(b.lines)-1] = line
		b.dropped++
		return
	}
	b.lines = append(b.lines, line)
}

func (b *browserOutputBuffer) snapshot() ([]browserOutputLine, int) {
	if b == nil {
		return nil, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]browserOutputLine(nil), b.lines...), b.dropped
}

type chromiumProcessDiagnostics struct {
	mu               sync.Mutex
	executable       string
	pid              int
	expectedExit     bool
	versionAttempted bool
	product          string
	protocolVersion  string
	revision         string
	versionError     string
	output           *browserOutputBuffer
	chromedpVersion  string
	cdprotoVersion   string
	emit             func(string)
}

func newChromiumProcessDiagnostics(executable string, userDataDir string) *chromiumProcessDiagnostics {
	executable = strings.TrimSpace(executable)
	if executable != "" {
		executable = filepath.Base(executable)
	}
	if executable == "" {
		executable = "unknown"
	}
	return &chromiumProcessDiagnostics{
		executable:      executable,
		output:          newBrowserOutputBuffer(userDataDir),
		chromedpVersion: dependencyVersion("github.com/chromedp/chromedp"),
		cdprotoVersion:  dependencyVersion("github.com/chromedp/cdproto"),
	}
}

func (d *chromiumProcessDiagnostics) setPID(pid int) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.pid = pid
	d.mu.Unlock()
}

func (d *chromiumProcessDiagnostics) markExpectedExit() {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.expectedExit = true
	d.mu.Unlock()
}

func (d *chromiumProcessDiagnostics) beginVersionLookup() bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.versionAttempted {
		return false
	}
	d.versionAttempted = true
	return true
}

func (d *chromiumProcessDiagnostics) setVersion(protocolVersion string, product string, revision string, err error) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.protocolVersion = strings.TrimSpace(protocolVersion)
	d.product = strings.TrimSpace(product)
	d.revision = strings.TrimSpace(revision)
	if err != nil {
		d.versionError = err.Error()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			d.versionAttempted = false
		}
	} else {
		d.versionError = ""
	}
	d.mu.Unlock()
}

func (d *chromiumProcessDiagnostics) allowVersionRetry() {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.versionAttempted = false
	d.mu.Unlock()
}

func (d *chromiumProcessDiagnostics) environment() []diagnostic.Field {
	fields := append(diagnostic.RuntimeEnvironment(),
		diagnostic.Value("daemon_version", api.DaemonVersion),
		diagnostic.Value("protocol_version", api.ProtocolVersion),
		diagnostic.Value("daemon_pid", os.Getpid()),
	)
	if d == nil {
		return append(fields,
			diagnostic.Value("browser_executable", "unknown"),
			diagnostic.Value("browser_pid", 0),
			diagnostic.Value("chromedp_version", dependencyVersion("github.com/chromedp/chromedp")),
			diagnostic.Value("cdproto_version", dependencyVersion("github.com/chromedp/cdproto")),
		)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	fields = append(fields,
		diagnostic.Value("browser_executable", d.executable),
		diagnostic.Value("browser_pid", d.pid),
		diagnostic.Value("browser_product", fallbackDiagnosticValue(d.product)),
		diagnostic.Value("browser_protocol_version", fallbackDiagnosticValue(d.protocolVersion)),
		diagnostic.Value("browser_revision", fallbackDiagnosticValue(d.revision)),
		diagnostic.Value("chromedp_version", d.chromedpVersion),
		diagnostic.Value("cdproto_version", d.cdprotoVersion),
	)
	if d.versionError != "" {
		fields = append(fields, diagnostic.Value("browser_version_error", d.versionError))
	}
	return fields
}

func (d *chromiumProcessDiagnostics) logUnexpectedExit(waitErr error) {
	if d == nil {
		return
	}
	d.mu.Lock()
	expected := d.expectedExit
	pid := d.pid
	d.mu.Unlock()
	if expected {
		return
	}

	exitErr := waitErr
	if exitErr == nil {
		exitErr = errors.New("chromium exited unexpectedly")
	}
	trace := diagnostic.New("chromium_process", false, d.emit, d.environment()...)
	trace.Event("chromium_process", "unexpected_exit",
		diagnostic.Value("browser_pid", pid),
		diagnostic.Value("error", exitErr),
	)
	appendBrowserOutput(trace, d)
	trace.Finish(exitErr)
}

func (b *Backend) currentProcessDiagnostics() *chromiumProcessDiagnostics {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.processDiagnostics
}

func (b *Backend) ensureBrowserVersion(ctx context.Context) {
	processDiagnostics := b.currentProcessDiagnostics()
	if processDiagnostics == nil || !processDiagnostics.beginVersionLookup() {
		return
	}

	trace := diagnostic.FromContext(ctx)
	trace.Event("browser_version", "start")
	protocolVersion, product, revision, err := readBrowserVersion(ctx)
	processDiagnostics.setVersion(protocolVersion, product, revision, err)
	trace.SetEnvironment(processDiagnostics.environment()...)
	fields := []diagnostic.Field{
		diagnostic.Value("browser_product", fallbackDiagnosticValue(product)),
		diagnostic.Value("browser_protocol_version", fallbackDiagnosticValue(protocolVersion)),
		diagnostic.Value("browser_revision", fallbackDiagnosticValue(revision)),
	}
	if err != nil {
		fields = append(fields, diagnostic.Value("error", err))
	}
	trace.Event("browser_version", "finish", fields...)
}

func readBrowserVersion(ctx context.Context) (protocolVersion string, product string, revision string, resultErr error) {
	resultErr = chromedp.Run(ctx, chromedp.ActionFunc(func(runCtx context.Context) error {
		chromedpContext := chromedp.FromContext(runCtx)
		if chromedpContext == nil || chromedpContext.Browser == nil {
			return errors.New("chromium browser context is unavailable")
		}
		var err error
		protocolVersion, product, revision, _, _, err = cdpbrowser.GetVersion().Do(
			cdp.WithExecutor(runCtx, chromedpContext.Browser),
		)
		return err
	}))
	return protocolVersion, product, revision, resultErr
}

func readBrowserVersionHTTP(ctx context.Context, devtoolsURL string) (string, string, string, error) {
	baseURL, err := debugHTTPBaseURL(devtoolsURL)
	if err != nil {
		return "", "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/json/version", nil)
	if err != nil {
		return "", "", "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", "", fmt.Errorf("read browser version: %s", resp.Status)
	}
	var version struct {
		ProtocolVersion string `json:"Protocol-Version"`
		Product         string `json:"Browser"`
		Revision        string `json:"WebKit-Version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		return "", "", "", err
	}
	return version.ProtocolVersion, version.Product, version.Revision, nil
}

func appendBrowserOutput(trace *diagnostic.Trace, processDiagnostics *chromiumProcessDiagnostics) {
	if trace == nil || processDiagnostics == nil {
		return
	}
	lines, dropped := processDiagnostics.output.snapshot()
	trace.Event("browser_output", "snapshot",
		diagnostic.Value("lines", len(lines)),
		diagnostic.Value("dropped_lines", dropped),
	)
	for _, line := range lines {
		trace.Event("browser_output", "line",
			diagnostic.Value("stream", line.stream),
			diagnostic.Value("message", line.message),
		)
	}
}

func beginBrowserOperation(ctx context.Context, request string, stage string, verbose bool, processDiagnostics *chromiumProcessDiagnostics, fields ...diagnostic.Field) (context.Context, *diagnostic.Trace, bool, time.Time) {
	trace := diagnostic.FromContext(ctx)
	owned := trace == nil
	if owned {
		trace = diagnostic.New(request, verbose, nil, processDiagnostics.environment()...)
		ctx = diagnostic.WithTrace(ctx, trace)
	} else {
		trace.SetVerbose(verbose)
		trace.SetEnvironment(processDiagnostics.environment()...)
	}
	startedAt := time.Now()
	trace.Event(stage, "start", fields...)
	return ctx, trace, owned, startedAt
}

func finishBrowserOperation(trace *diagnostic.Trace, owned bool, stage string, startedAt time.Time, processDiagnostics *chromiumProcessDiagnostics, err error) {
	fields := []diagnostic.Field{
		diagnostic.Value("duration_ms", time.Since(startedAt).Milliseconds()),
	}
	if err != nil {
		fields = append(fields, diagnostic.Value("error", err))
	}
	trace.Event(stage, "finish", fields...)
	if err != nil {
		trace.SetEnvironment(processDiagnostics.environment()...)
		appendBrowserOutput(trace, processDiagnostics)
	}
	if owned {
		trace.Finish(err)
	}
}

func dependencyVersion(path string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dependency := range info.Deps {
		if dependency.Path != path {
			continue
		}
		if dependency.Replace != nil {
			dependency = dependency.Replace
		}
		version := strings.TrimSpace(dependency.Version)
		if version != "" {
			return version
		}
		return "unknown"
	}
	return "unknown"
}

func fallbackDiagnosticValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}
