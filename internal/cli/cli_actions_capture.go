package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/rpc"
)

type screenshotCaptureOptions struct {
	Annotate bool
	Full     bool
	Locator  string
	Nth      int
}

func runScreenshot(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printScreenshotHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("screenshot", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	full := fs.Bool("full", false, "capture full page")
	annotate := fs.Bool("annotate", false, "draw node refs on top of the screenshot")
	locator := fs.String("locator", "", "capture a single element such as @e3 or label=Email")
	nth := fs.Int("nth", 0, "select nth match when --locator matches multiple nodes")
	pathArg := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		pathArg = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "screenshot"); err != nil {
		return 1
	}
	if isInvalidNthFlag(fs, *nth) {
		fmt.Fprintln(stderr, "--nth must be a positive integer")
		return 1
	}
	if *nth > 0 && strings.TrimSpace(*locator) == "" {
		fmt.Fprintln(stderr, "--nth requires --locator")
		return 1
	}
	if strings.TrimSpace(*locator) != "" && *full {
		fmt.Fprintln(stderr, "--full is not supported with --locator")
		return 1
	}

	if pathArg == "" && fs.NArg() == 1 {
		pathArg = fs.Arg(0)
	}
	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "screenshot accepts at most one path")
		return 1
	}
	if pathArg == "" {
		pathArg = "screenshot.png"
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	data, err := captureScreenshotBytes(ctx, client, *sessionID, screenshotCaptureOptions{
		Annotate: *annotate,
		Full:     *full,
		Locator:  strings.TrimSpace(*locator),
		Nth:      *nth,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := os.WriteFile(pathArg, data, 0o644); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "saved screenshot %s\n", pathArg)
	return 0
}

func captureScreenshotBytes(ctx context.Context, client *rpc.Client, sessionID string, opts screenshotCaptureOptions) ([]byte, error) {
	if opts.Locator != "" {
		return captureElementScreenshotBytes(ctx, client, sessionID, opts)
	}

	res, err := client.ObserveSession(ctx, api.ObserveSessionRequest{
		SessionID: sessionID,
		Options: api.ObserveOptions{
			WithScreenshot: true,
			WithTree:       opts.Annotate,
			FullScreenshot: opts.Full,
		},
	})
	if err != nil {
		return nil, err
	}
	if res.Observation.Screenshot == "" {
		return nil, fmt.Errorf("empty screenshot")
	}

	data, err := base64.StdEncoding.DecodeString(res.Observation.Screenshot)
	if err != nil {
		return nil, err
	}
	if !opts.Annotate {
		return data, nil
	}

	return annotateScreenshot(data, res.Observation.Tree)
}

func captureElementScreenshotBytes(ctx context.Context, client *rpc.Client, sessionID string, opts screenshotCaptureOptions) ([]byte, error) {
	observation, err := observeTreeForFind(ctx, client, sessionID)
	if err != nil {
		return nil, err
	}

	node, err := resolveScreenshotNode(observation.Tree, opts.Locator, nodeSelectionOptions{Nth: opts.Nth})
	if err != nil {
		return nil, err
	}

	rect, err := focusScreenshotNode(ctx, client, sessionID, node.ID)
	if err != nil {
		return nil, err
	}

	res, err := client.ObserveSession(ctx, api.ObserveSessionRequest{
		SessionID: sessionID,
		Options: api.ObserveOptions{
			WithScreenshot: true,
			WithTree:       opts.Annotate,
		},
	})
	if err != nil {
		return nil, err
	}
	if res.Observation.Screenshot == "" {
		return nil, fmt.Errorf("empty screenshot")
	}

	data, err := base64.StdEncoding.DecodeString(res.Observation.Screenshot)
	if err != nil {
		return nil, err
	}
	if opts.Annotate {
		data, err = annotateScreenshot(data, res.Observation.Tree)
		if err != nil {
			return nil, err
		}
	}

	return cropScreenshot(data, rect)
}

func resolveScreenshotNode(nodes []api.Node, locator string, selection nodeSelectionOptions) (api.Node, error) {
	terms, err := parseFlowLocator(locator)
	if err != nil {
		return api.Node{}, err
	}
	matches := selectNodes(nodes, func(node api.Node) bool {
		for _, term := range terms {
			if !matchesFlowLocatorTerm(node, term) {
				return false
			}
		}
		return true
	})
	return chooseNode(matches, locator, selection)
}

func focusScreenshotNode(ctx context.Context, client *rpc.Client, sessionID string, nodeID int) (api.Rect, error) {
	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: sessionID,
		Action: api.Action{
			Kind: "eval",
			Text: screenshotNodeRectJS(nodeID),
		},
	})
	if err != nil {
		return api.Rect{}, err
	}
	if !res.Result.OK {
		if strings.TrimSpace(res.Result.Message) != "" {
			return api.Rect{}, errors.New(res.Result.Message)
		}
		return api.Rect{}, fmt.Errorf("failed to focus screenshot node")
	}

	return parseRectValue(res.Result.Value)
}

func screenshotNodeRectJS(nodeID int) string {
	return `(function () {
  const selector = [
    'button',
    'a[href]',
    'input',
    'textarea',
    'select',
    '[role="button"]',
    '[role="link"]',
    '[role="tab"]',
    '[role="checkbox"]',
    '[role="radio"]',
    '[contenteditable="true"]',
    '[contenteditable=""]',
    '[onclick]',
    '[tabindex]'
  ].join(',');

  const visible = (el) => {
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    if (el.hidden) return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  };

  const candidates = Array.from(document.querySelectorAll(selector))
    .filter((el) => visible(el));

  const el = candidates[` + strconv.Itoa(nodeID-1) + `];
  if (!el) {
    throw new Error('node not found');
  }

  el.scrollIntoView({block: 'center', inline: 'center'});
  const rect = el.getBoundingClientRect();
  return {
    x: Math.round(rect.x),
    y: Math.round(rect.y),
    w: Math.round(rect.width),
    h: Math.round(rect.height)
  };
})()`
}

func parseRectValue(value interface{}) (api.Rect, error) {
	fields, ok := value.(map[string]interface{})
	if !ok {
		return api.Rect{}, fmt.Errorf("invalid screenshot bounds")
	}

	rect := api.Rect{}
	var err error
	if rect.X, err = rectField(fields, "x"); err != nil {
		return api.Rect{}, err
	}
	if rect.Y, err = rectField(fields, "y"); err != nil {
		return api.Rect{}, err
	}
	if rect.W, err = rectField(fields, "w"); err != nil {
		return api.Rect{}, err
	}
	if rect.H, err = rectField(fields, "h"); err != nil {
		return api.Rect{}, err
	}
	if rect.W <= 0 || rect.H <= 0 {
		return api.Rect{}, fmt.Errorf("invalid screenshot bounds")
	}
	return rect, nil
}

func rectField(fields map[string]interface{}, key string) (int, error) {
	value, ok := fields[key]
	if !ok {
		return 0, fmt.Errorf("invalid screenshot bounds")
	}
	number, ok := value.(float64)
	if !ok {
		return 0, fmt.Errorf("invalid screenshot bounds")
	}
	return int(number), nil
}

func cropScreenshot(data []byte, rect api.Rect) ([]byte, error) {
	source, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode screenshot: %w", err)
	}

	bounds := image.Rect(rect.X, rect.Y, rect.X+rect.W, rect.Y+rect.H).Intersect(source.Bounds())
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return nil, fmt.Errorf("element is outside the captured screenshot")
	}

	canvas := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(canvas, canvas.Bounds(), source, bounds.Min, draw.Src)

	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		return nil, fmt.Errorf("encode screenshot: %w", err)
	}
	return output.Bytes(), nil
}

func annotateScreenshot(data []byte, nodes []api.Node) ([]byte, error) {
	source, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode screenshot: %w", err)
	}

	bounds := source.Bounds()
	canvas := image.NewRGBA(bounds)
	draw.Draw(canvas, bounds, source, bounds.Min, draw.Src)

	palette := []color.RGBA{
		{R: 255, G: 59, B: 48, A: 255},
		{R: 0, G: 122, B: 255, A: 255},
		{R: 52, G: 199, B: 89, A: 255},
		{R: 255, G: 149, B: 0, A: 255},
		{R: 175, G: 82, B: 222, A: 255},
	}

	for _, node := range nodes {
		if node.Bounds.W <= 0 || node.Bounds.H <= 0 {
			continue
		}

		rect := image.Rect(
			node.Bounds.X,
			node.Bounds.Y,
			node.Bounds.X+node.Bounds.W,
			node.Bounds.Y+node.Bounds.H,
		).Intersect(bounds)
		if rect.Dx() <= 0 || rect.Dy() <= 0 {
			continue
		}

		boxColor := palette[(node.ID-1)%len(palette)]
		drawOutline(canvas, rect, boxColor, 2)

		label := node.Ref
		if label == "" {
			label = fmt.Sprintf("%d", node.ID)
		}
		drawNodeLabel(canvas, rect, label, boxColor)
	}

	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		return nil, fmt.Errorf("encode screenshot: %w", err)
	}
	return output.Bytes(), nil
}

func drawOutline(img *image.RGBA, rect image.Rectangle, stroke color.RGBA, thickness int) {
	for offset := 0; offset < thickness; offset++ {
		top := image.Rect(rect.Min.X, rect.Min.Y+offset, rect.Max.X, rect.Min.Y+offset+1)
		bottom := image.Rect(rect.Min.X, rect.Max.Y-offset-1, rect.Max.X, rect.Max.Y-offset)
		left := image.Rect(rect.Min.X+offset, rect.Min.Y, rect.Min.X+offset+1, rect.Max.Y)
		right := image.Rect(rect.Max.X-offset-1, rect.Min.Y, rect.Max.X-offset, rect.Max.Y)
		fillRect(img, top, stroke)
		fillRect(img, bottom, stroke)
		fillRect(img, left, stroke)
		fillRect(img, right, stroke)
	}
}

func fillRect(img *image.RGBA, rect image.Rectangle, fill color.RGBA) {
	rect = rect.Intersect(img.Bounds())
	if rect.Dx() <= 0 || rect.Dy() <= 0 {
		return
	}
	draw.Draw(img, rect, &image.Uniform{C: fill}, image.Point{}, draw.Src)
}

func drawNodeLabel(img *image.RGBA, rect image.Rectangle, label string, accent color.RGBA) {
	face := basicfont.Face7x13
	textWidth := font.MeasureString(face, label).Round()
	labelHeight := face.Metrics().Height.Round() + 4
	labelRect := image.Rect(rect.Min.X, rect.Min.Y-labelHeight, rect.Min.X+textWidth+6, rect.Min.Y)
	if labelRect.Min.Y < img.Bounds().Min.Y {
		labelRect = image.Rect(rect.Min.X, rect.Min.Y, rect.Min.X+textWidth+6, rect.Min.Y+labelHeight)
	}
	labelRect = labelRect.Intersect(img.Bounds())
	if labelRect.Dx() <= 0 || labelRect.Dy() <= 0 {
		return
	}

	fillRect(img, labelRect, accent)
	drawer := font.Drawer{
		Dst:  img,
		Src:  image.White,
		Face: face,
		Dot: fixed.Point26_6{
			X: fixed.I(labelRect.Min.X + 3),
			Y: fixed.I(labelRect.Min.Y + face.Metrics().Ascent.Round() + 2),
		},
	}
	drawer.DrawString(label)
}
