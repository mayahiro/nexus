package cli

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/rpc"
)

func TestClick(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, clickRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"click", "3"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected click exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "clicked 3" {
		t.Fatalf("unexpected click output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"click", "120", "240"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected coordinate click exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "clicked 120 240" {
		t.Fatalf("unexpected coordinate click output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"click", "@e3"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected click ref exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "clicked @e3" {
		t.Fatalf("unexpected click ref output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"click", "3", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected click --json exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "clicked 3"`) {
		t.Fatalf("unexpected click --json output: %s", stdout.String())
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

func TestHoverDblclickRightclick(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, mouseRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"hover", "3"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected hover exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "hovered 3" {
		t.Fatalf("unexpected hover output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"dblclick", "3"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected dblclick exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "double-clicked 3" {
		t.Fatalf("unexpected dblclick output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"rightclick", "3", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected rightclick --json exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "right-clicked 3"`) {
		t.Fatalf("unexpected rightclick --json output: %s", stdout.String())
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

func TestTypeAndInput(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, typeRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"type", "hello"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected type exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "typed" {
		t.Fatalf("unexpected type output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"input", "3", "hello"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected input exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "typed into 3" {
		t.Fatalf("unexpected input output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"input", "@e3", "hello"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected input ref exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "typed into 3" {
		t.Fatalf("unexpected input ref output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"input", "3", "hello", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected input --json exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "typed into 3"`) {
		t.Fatalf("unexpected input --json output: %s", stdout.String())
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

func TestKeys(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, keysRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"keys", "Enter"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected keys exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "sent keys Enter" {
		t.Fatalf("unexpected keys output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"keys", "Meta+L", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected keys --json exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "sent keys Meta+L"`) {
		t.Fatalf("unexpected keys --json output: %s", stdout.String())
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

func TestScreenshot(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, screenshotRPCHandler{}, rpc.ServeOptions{})
	}()

	tempDir := t.TempDir()
	t.Chdir(tempDir)

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"screenshot"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected screenshot exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "saved screenshot screenshot.png" {
		t.Fatalf("unexpected screenshot output: %s", stdout.String())
	}

	data, err := os.ReadFile(filepath.Join(tempDir, "screenshot.png"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "pngdata" {
		t.Fatalf("unexpected screenshot data: %q", string(data))
	}

	stdout.Reset()
	customPath := filepath.Join(tempDir, "full.png")
	if code := Run(context.Background(), []string{"screenshot", customPath, "--full"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected full screenshot exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "saved screenshot "+customPath {
		t.Fatalf("unexpected full screenshot output: %s", stdout.String())
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

func TestScreenshotAnnotate(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, annotateScreenshotRPCHandler{}, rpc.ServeOptions{})
	}()

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "annotated.png")

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"screenshot", outputPath, "--annotate"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected screenshot --annotate exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "saved screenshot "+outputPath {
		t.Fatalf("unexpected screenshot --annotate output: %s", stdout.String())
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("expected annotated png: %v", err)
	}

	rgba := image.NewRGBA(img.Bounds())
	drawBounds := img.Bounds()
	for y := drawBounds.Min.Y; y < drawBounds.Max.Y; y++ {
		for x := drawBounds.Min.X; x < drawBounds.Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}

	if !hasNonWhitePixel(rgba) {
		t.Fatalf("expected annotation pixels in output")
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

func TestScroll(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, scrollRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"scroll", "down"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected scroll exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "scrolled down" {
		t.Fatalf("unexpected scroll output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"scroll", "up", "--amount", "500", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected scroll --json exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "scrolled up"`) {
		t.Fatalf("unexpected scroll --json output: %s", stdout.String())
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

func TestBack(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, backRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"back"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected back exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "went back" {
		t.Fatalf("unexpected back output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"back", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected back --json exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "went back"`) {
		t.Fatalf("unexpected back --json output: %s", stdout.String())
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

func TestViewport(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, viewportRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"viewport", "1280x720"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected viewport exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "set viewport 1280x720" {
		t.Fatalf("unexpected viewport output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"viewport", "1440x900", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected viewport --json exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "set viewport 1440x900"`) {
		t.Fatalf("unexpected viewport --json output: %s", stdout.String())
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

func TestWait(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, waitRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"wait", "selector", ".ready"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected wait selector exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "waited for selector" {
		t.Fatalf("unexpected wait selector output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"wait", "text", "Done", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected wait text exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "waited for text"`) {
		t.Fatalf("unexpected wait text output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"wait", "url", "/done"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected wait url exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "waited for url" {
		t.Fatalf("unexpected wait url output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"wait", "navigation"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected wait navigation exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "waited for navigation" {
		t.Fatalf("unexpected wait navigation output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"wait", "function", "window.appReady === true", "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected wait function exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "waited for function"`) {
		t.Fatalf("unexpected wait function output: %s", stdout.String())
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

func TestSelectAndUpload(t *testing.T) {
	configureXDGTestEnv(t)

	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Socket), 0o755); err != nil {
		t.Fatal(err)
	}

	uploadFile := filepath.Join(t.TempDir(), "upload.txt")
	if err := os.WriteFile(uploadFile, []byte("upload"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- rpc.Serve(ctx, listener, selectUploadRPCHandler{}, rpc.ServeOptions{})
	}()

	var stdout bytes.Buffer
	if code := Run(context.Background(), []string{"select", "3", "two"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected select exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "selected two on 3" {
		t.Fatalf("unexpected select output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"select", "@e3", "two"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected select ref exit code: %d\n%s", code, stdout.String())
	}
	if strings.TrimSpace(stdout.String()) != "selected two on @e3" {
		t.Fatalf("unexpected select ref output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"upload", "4", uploadFile, "--json"}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected upload exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"message": "uploaded `) {
		t.Fatalf("unexpected upload output: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"upload", "@e4", uploadFile}, &stdout, &stdout); code != 0 {
		t.Fatalf("unexpected upload ref exit code: %d\n%s", code, stdout.String())
	}
	if !strings.Contains(strings.TrimSpace(stdout.String()), "uploaded "+uploadFile+" to @e4") {
		t.Fatalf("unexpected upload ref output: %s", stdout.String())
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

func hasNonWhitePixel(img *image.RGBA) bool {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if r != 0xffff || g != 0xffff || b != 0xffff || a != 0xffff {
				return true
			}
		}
	}
	return false
}
