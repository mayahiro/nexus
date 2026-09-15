package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/rpc"
)

func validateFlowDialogStep(step flowStep) error {
	if !step.ExpectDialog && strings.TrimSpace(step.Action) != "dialog" {
		return nil
	}
	if side := flowStepSide(step); side != "old" && side != "new" && side != "both" {
		return errors.New("dialog step side must be old, new, or both")
	}
	if step.Timeout != nil && (*step.Timeout <= 0 || int64(*step.Timeout) > int64((1<<63-1)/time.Millisecond)) {
		return errors.New("dialog timeout must be a positive duration in milliseconds")
	}
	if step.ExpectDialog {
		switch strings.TrimSpace(step.Action) {
		case "click", "fill":
			if strings.TrimSpace(step.Locator) == "" {
				return errors.New("expected-dialog click or fill requires locator")
			}
			if step.Nth < 0 {
				return errors.New("nth must be a positive integer")
			}
			if strings.TrimSpace(step.Action) == "fill" && (step.Text == nil || *step.Text == "") {
				return errors.New("fill step requires text")
			}
		case "navigate":
			if strings.TrimSpace(step.Value) == "" {
				return errors.New("navigate step requires value")
			}
		default:
			return errors.New("expect_dialog is only supported for click, fill, or navigate steps")
		}
		return nil
	}
	switch strings.TrimSpace(step.Target) {
	case "get", "accept", "dismiss":
	default:
		return errors.New("dialog step target must be get, accept, or dismiss")
	}
	if step.Text != nil && strings.TrimSpace(step.Target) != "accept" {
		return errors.New("dialog step text is only supported with target accept")
	}
	return nil
}

func executeFlowDialogStep(ctx context.Context, client *rpc.Client, state flowExecutionState, step flowStep) (map[string]api.ActionResult, error) {
	if err := validateFlowDialogStep(step); err != nil {
		return nil, err
	}
	timeoutMS := 30000
	if step.Timeout != nil {
		timeoutMS = *step.Timeout
	}
	results := make(map[string]api.ActionResult)
	for _, target := range flowStepTargets(state, step) {
		result, err := executeFlowDialogTarget(ctx, client, target.SessionID, step, timeoutMS)
		if result != nil {
			results[target.Side] = *result
		}
		if err != nil {
			return results, fmt.Errorf("%s dialog step: %w", target.Side, err)
		}
	}
	return results, nil
}

func executeFlowDialogTarget(ctx context.Context, client *rpc.Client, sessionID string, step flowStep, timeoutMS int) (*api.ActionResult, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()
	action := api.Action{Kind: "dialog", Args: map[string]string{"operation": strings.TrimSpace(step.Target)}}
	if step.ExpectDialog {
		action = api.Action{ExpectDialog: true, Args: map[string]string{"timeout_ms": strconv.Itoa(timeoutMS)}}
		if strings.TrimSpace(step.Action) == "navigate" {
			action.Kind = "navigate"
			action.Args["url"] = strings.TrimSpace(step.Value)
		} else {
			node, err := resolveFlowLocator(ctx, client, sessionID, step.Locator, nodeSelectionOptions{Nth: step.Nth})
			if err != nil {
				return nil, err
			}
			action.Kind = "invoke"
			if strings.TrimSpace(step.Action) == "fill" {
				action.Kind = "fill"
			}
			action.NodeID, action.NodeRef, action.Selector = &node.ID, node.Ref, node.Selector
			if step.Text != nil {
				action.Text = *step.Text
			}
		}
	} else if step.Text != nil {
		action.Args["text"] = *step.Text
	}
	res, err := client.ActSession(ctx, api.ActSessionRequest{SessionID: sessionID, Action: action})
	if err != nil {
		return nil, err
	}
	if !res.Result.OK {
		message := res.Result.Message
		if message == "" {
			message = "dialog step failed"
		}
		return &res.Result, errors.New(message)
	}
	if step.ExpectDialog && (res.Result.Dialog == nil || !res.Result.Dialog.Open) {
		return &res.Result, errors.New("expected JavaScript dialog was not reported")
	}
	return &res.Result, nil
}
