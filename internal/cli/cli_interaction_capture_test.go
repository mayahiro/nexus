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
