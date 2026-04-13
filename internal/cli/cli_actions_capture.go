package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"os"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/mayahiro/nexus/internal/api"
)

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
	pathArg := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		pathArg = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "screenshot"); err != nil {
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

	res, err := client.ObserveSession(ctx, api.ObserveSessionRequest{
		SessionID: *sessionID,
		Options: api.ObserveOptions{
			WithScreenshot: true,
			WithTree:       *annotate,
			FullScreenshot: *full,
		},
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if res.Observation.Screenshot == "" {
		fmt.Fprintln(stderr, "empty screenshot")
		return 1
	}

	data, err := base64.StdEncoding.DecodeString(res.Observation.Screenshot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *annotate {
		data, err = annotateScreenshot(data, res.Observation.Tree)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if err := os.WriteFile(pathArg, data, 0o644); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "saved screenshot %s\n", pathArg)
	return 0
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
