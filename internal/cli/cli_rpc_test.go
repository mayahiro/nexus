package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strconv"

	"github.com/mayahiro/nexus/internal/api"
)

type noopRPCHandler struct{}

func (noopRPCHandler) Ping(context.Context, api.PingRequest) (api.PingResponse, error) {
	return api.PingResponse{}, nil
}

func (noopRPCHandler) AttachSession(context.Context, api.AttachSessionRequest) (api.AttachSessionResponse, error) {
	return api.AttachSessionResponse{}, nil
}

func (noopRPCHandler) ListSessions(context.Context, api.ListSessionsRequest) (api.ListSessionsResponse, error) {
	return api.ListSessionsResponse{}, nil
}

func (noopRPCHandler) DetachSession(context.Context, api.DetachSessionRequest) (api.DetachSessionResponse, error) {
	return api.DetachSessionResponse{}, nil
}

func (noopRPCHandler) StopDaemon(context.Context, api.StopDaemonRequest) (api.StopDaemonResponse, error) {
	return api.StopDaemonResponse{Stopped: true}, nil
}

func (noopRPCHandler) ObserveSession(context.Context, api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	return api.ObserveSessionResponse{}, nil
}

func (noopRPCHandler) ActSession(context.Context, api.ActSessionRequest) (api.ActSessionResponse, error) {
	return api.ActSessionResponse{}, nil
}

type evalRPCHandler struct{ noopRPCHandler }
type clickRPCHandler struct{ noopRPCHandler }
type mouseRPCHandler struct{ noopRPCHandler }
type typeRPCHandler struct{ noopRPCHandler }
type keysRPCHandler struct{ noopRPCHandler }
type screenshotRPCHandler struct{ noopRPCHandler }
type annotateScreenshotRPCHandler struct{ noopRPCHandler }
type scrollRPCHandler struct{ noopRPCHandler }
type backRPCHandler struct{ noopRPCHandler }
type viewportRPCHandler struct{ noopRPCHandler }
type waitRPCHandler struct{ noopRPCHandler }
type getRPCHandler struct{ noopRPCHandler }
type findRPCHandler struct{ noopRPCHandler }
type selectUploadRPCHandler struct{ noopRPCHandler }

func (evalRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	switch req.Action.Text {
	case "document.title":
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Value: "Example Title"}}, nil
	case "[1, 2, 3]":
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Value: []interface{}{1, 2, 3}}}, nil
	case "false":
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Value: false}}, nil
	case "0":
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Value: 0}}, nil
	case `""`:
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Value: ""}}, nil
	default:
		return api.ActSessionResponse{}, nil
	}
}

func (clickRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if req.Action.Kind != "invoke" {
		return api.ActSessionResponse{}, nil
	}
	if req.Action.NodeID != nil {
		return api.ActSessionResponse{
			Result: api.ActionResult{
				OK:      true,
				Changed: true,
				Message: "clicked 3",
				Value:   map[string]interface{}{"id": float64(*req.Action.NodeID)},
			},
		}, nil
	}
	if req.Action.Args["x"] == "120" && req.Action.Args["y"] == "240" {
		return api.ActSessionResponse{
			Result: api.ActionResult{
				OK:      true,
				Changed: true,
				Message: "clicked 120 240",
				Value:   map[string]interface{}{"x": float64(120), "y": float64(240)},
			},
		}, nil
	}
	return api.ActSessionResponse{}, nil
}

func (mouseRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	switch req.Action.Kind {
	case "hover":
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Changed: true, Message: "hovered 3"}}, nil
	case "dblclick":
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Changed: true, Message: "double-clicked 3"}}, nil
	case "rightclick":
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Changed: true, Message: "right-clicked 3"}}, nil
	default:
		return api.ActSessionResponse{}, nil
	}
}

func (typeRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if (req.Action.Kind != "type" && req.Action.Kind != "fill") || req.Action.Text == "" {
		return api.ActSessionResponse{}, nil
	}
	message := "typed"
	if req.Action.Kind == "fill" {
		message = "filled"
	}
	if req.Action.NodeID != nil {
		if req.Action.Kind == "fill" {
			message = "filled into 3"
		} else {
			message = "typed into 3"
		}
	}
	return api.ActSessionResponse{
		Result: api.ActionResult{
			OK:      true,
			Changed: true,
			Message: message,
			Value:   map[string]interface{}{"text": req.Action.Text},
		},
	}, nil
}

func (keysRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if req.Action.Kind != "key" || len(req.Action.Keys) != 1 {
		return api.ActSessionResponse{}, nil
	}
	return api.ActSessionResponse{Result: api.ActionResult{OK: true, Changed: true, Message: "sent keys " + req.Action.Keys[0]}}, nil
}

func (screenshotRPCHandler) ObserveSession(_ context.Context, req api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	if !req.Options.WithScreenshot {
		return api.ObserveSessionResponse{}, nil
	}
	return api.ObserveSessionResponse{
		Observation: api.Observation{
			Screenshot: base64.StdEncoding.EncodeToString([]byte("pngdata")),
		},
	}, nil
}

func (annotateScreenshotRPCHandler) ObserveSession(_ context.Context, req api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	if !req.Options.WithScreenshot {
		return api.ObserveSessionResponse{}, nil
	}
	return api.ObserveSessionResponse{
		Observation: api.Observation{
			Screenshot: testPNGBase64(),
			Tree: []api.Node{
				{ID: 1, Ref: "@e1", Role: "button", Name: "Submit", Visible: true, Enabled: true, Bounds: api.Rect{X: 4, Y: 6, W: 18, H: 12}},
			},
		},
	}, nil
}

func (scrollRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if req.Action.Kind != "scroll" || (req.Action.Dir != "up" && req.Action.Dir != "down") {
		return api.ActSessionResponse{}, nil
	}
	return api.ActSessionResponse{
		Result: api.ActionResult{
			OK:      true,
			Changed: true,
			Message: "scrolled " + req.Action.Dir,
			Value:   map[string]interface{}{"dir": req.Action.Dir},
		},
	}, nil
}

func (backRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if req.Action.Kind != "back" {
		return api.ActSessionResponse{}, nil
	}
	return api.ActSessionResponse{Result: api.ActionResult{OK: true, Changed: true, Message: "went back"}}, nil
}

func (viewportRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if req.Action.Kind != "viewport" || req.Action.Args["width"] == "" || req.Action.Args["height"] == "" {
		return api.ActSessionResponse{}, nil
	}
	return api.ActSessionResponse{
		Result: api.ActionResult{
			OK:      true,
			Changed: true,
			Message: "set viewport " + req.Action.Args["width"] + "x" + req.Action.Args["height"],
			Value:   map[string]interface{}{"width": req.Action.Args["width"], "height": req.Action.Args["height"]},
		},
	}, nil
}

func (waitRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if req.Action.Kind != "wait" || req.Action.Args["target"] == "" {
		return api.ActSessionResponse{}, nil
	}
	return api.ActSessionResponse{Result: api.ActionResult{OK: true, Message: "waited for " + req.Action.Args["target"]}}, nil
}

func (getRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	if req.Action.Kind != "get" {
		return api.ActSessionResponse{}, nil
	}
	switch req.Action.Args["target"] {
	case "title":
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Value: "Example Title"}}, nil
	case "attributes":
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Value: map[string]interface{}{"href": "/docs"}}}, nil
	default:
		return api.ActSessionResponse{}, nil
	}
}

func (findRPCHandler) ObserveSession(context.Context, api.ObserveSessionRequest) (api.ObserveSessionResponse, error) {
	return api.ObserveSessionResponse{
		Observation: api.Observation{
			Tree: []api.Node{
				{ID: 1, Ref: "@e1", Role: "button", Name: "Submit", Visible: true, Enabled: true, Attrs: map[string]string{"data-testid": "submit-primary"}, LocatorHints: []api.LocatorHint{
					{Kind: "role", Value: "button", Name: "Submit", Command: `role button --name "Submit"`},
					{Kind: "text", Value: "Submit", Command: `text "Submit"`},
					{Kind: "testid", Value: "submit-primary", Command: `testid "submit-primary"`},
				}},
				{ID: 2, Ref: "@e2", Role: "link", Text: "Sign In", Visible: true, Enabled: true, Attrs: map[string]string{"href": "/signin"}, LocatorHints: []api.LocatorHint{
					{Kind: "role", Value: "link", Name: "Sign In", Command: `role link --name "Sign In"`},
					{Kind: "text", Value: "Sign In", Command: `text "Sign In"`},
					{Kind: "href", Value: "/signin", Command: `href "/signin"`},
				}},
				{ID: 3, Ref: "@e3", Role: "textbox", Name: "Email", Visible: true, Enabled: true, Editable: true, LocatorHints: []api.LocatorHint{
					{Kind: "role", Value: "textbox", Name: "Email", Command: `role textbox --name "Email"`},
					{Kind: "label", Value: "Email", Command: `label "Email"`},
				}},
				{ID: 4, Ref: "@e4", Role: "button", Name: "Cancel", Visible: true, Enabled: true},
			},
		},
	}, nil
}

func (findRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	switch req.Action.Kind {
	case "invoke":
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Changed: true, Message: "clicked " + strconv.Itoa(*req.Action.NodeID)}}, nil
	case "type":
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Changed: true, Message: "typed into " + strconv.Itoa(*req.Action.NodeID)}}, nil
	case "fill":
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Changed: true, Message: "filled into " + strconv.Itoa(*req.Action.NodeID)}}, nil
	case "get":
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Value: map[string]interface{}{"href": "/signin"}}}, nil
	default:
		return api.ActSessionResponse{}, nil
	}
}

func (selectUploadRPCHandler) ActSession(_ context.Context, req api.ActSessionRequest) (api.ActSessionResponse, error) {
	switch req.Action.Kind {
	case "select":
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Changed: true, Message: "selected " + req.Action.Text + " on 3"}}, nil
	case "upload":
		return api.ActSessionResponse{Result: api.ActionResult{OK: true, Changed: true, Message: "uploaded " + req.Action.Text + " to 4"}}, nil
	default:
		return api.ActSessionResponse{}, nil
	}
}

func testPNGBase64() string {
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.White)
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}
