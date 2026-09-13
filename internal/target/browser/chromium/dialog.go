package chromium

import (
	"context"
	"errors"
	"fmt"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/mayahiro/nexus/internal/api"
)

type javascriptDialogBlockedError struct {
	operation string
	dialog    api.DialogState
}

func (e *javascriptDialogBlockedError) Error() string {
	return fmt.Sprintf("%s is blocked by an open %s JavaScript dialog: %q; use dialog get, dialog accept, or dialog dismiss in the same session before continuing; the operation may have partially executed", e.operation, e.dialog.Type, e.dialog.Message)
}

// Called under opMu. Cancel only the request's CDP waits, keeping the target
// connection and page script alive so the next operation can handle the dialog.
func runWithDialogGuard[T any](b *Backend, ctx context.Context, operation string, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	b.mu.Lock()
	b.dialogInterrupt = func(dialog api.DialogState) {
		cancel(&javascriptDialogBlockedError{operation: operation, dialog: dialog})
	}
	if b.dialog.Open {
		b.dialogInterrupt(b.dialog)
	}
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.dialogInterrupt = nil
		b.mu.Unlock()
	}()

	if err := context.Cause(ctx); err != nil {
		return zero, err
	}
	result, err := fn(ctx)
	var blocked *javascriptDialogBlockedError
	if errors.As(context.Cause(ctx), &blocked) {
		return zero, blocked
	}
	return result, err
}

func (b *Backend) dialogViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	return withBackendPageTargetContext(b, ctx, devtoolsURL, func(targetCtx context.Context, _ pageTargetInfo) (*api.ActionResult, error) {
		var result *api.ActionResult
		err := chromedp.Run(targetCtx, chromedp.ActionFunc(func(runCtx context.Context) error {
			var err error
			result, err = b.dialogInContext(runCtx, action)
			return err
		}))
		return result, err
	})
}

func (b *Backend) dialogInContext(ctx context.Context, action api.Action) (*api.ActionResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	operation := action.Args["operation"]
	text, hasText := action.Args["text"]
	if operation != "get" && operation != "accept" && operation != "dismiss" {
		return nil, errors.New("dialog operation must be get, accept, or dismiss")
	}
	if hasText && operation != "accept" {
		return nil, errors.New("dialog text is only supported when accepting a prompt")
	}
	b.mu.Lock()
	dialog := b.dialog
	sequence := b.dialogSequence
	b.mu.Unlock()

	if operation == "get" {
		message := "no JavaScript dialog is open"
		if dialog.Open {
			message = fmt.Sprintf("%s dialog: %q", dialog.Type, dialog.Message)
			if dialog.Type == "prompt" {
				message += fmt.Sprintf(" (default: %q)", dialog.DefaultPrompt)
			}
		}
		return &api.ActionResult{OK: true, Message: message, Value: dialog}, nil
	}
	if !dialog.Open {
		return nil, errors.New("no JavaScript dialog is open")
	}
	if hasText && dialog.Type != "prompt" {
		return nil, errors.New("dialog text is only supported when accepting a prompt")
	}
	accept := operation == "accept"
	// cdproto's string field omits empty prompt text. Use a pointer so an explicit
	// empty response is sent, and preserve the initial value when text is omitted.
	params := struct {
		Accept     bool    `json:"accept"`
		PromptText *string `json:"promptText,omitempty"`
	}{Accept: accept}
	if accept && dialog.Type == "prompt" {
		if !hasText {
			text = dialog.DefaultPrompt
		}
		params.PromptText = &text
	}
	if err := cdp.Execute(ctx, page.CommandHandleJavaScriptDialog, &params, nil); err != nil {
		return nil, fmt.Errorf("%s JavaScript dialog: %w", operation, err)
	}
	// A script can open its next dialog before the command response arrives.
	// Do not clear that successor's state.
	b.mu.Lock()
	if b.dialogSequence == sequence {
		b.dialog = api.DialogState{}
	}
	b.mu.Unlock()
	message := "dismissed"
	if accept {
		message = "accepted"
	}
	return &api.ActionResult{
		OK:      true,
		Changed: true,
		Message: fmt.Sprintf("%s %s dialog", message, dialog.Type),
		Value: map[string]any{
			"type":     dialog.Type,
			"accepted": accept,
		},
	}, nil
}
