package comparecmd

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/browsermgr"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/rpc"
)

func prepareCompareSessions(ctx context.Context, client *rpc.Client, paths config.Paths, oldEndpoint compareEndpoint, newEndpoint compareEndpoint, backend string, targetRef string, viewport string) (preparedCompareSession, preparedCompareSession, error) {
	resolvedTargetRef := strings.TrimSpace(targetRef)
	if resolvedTargetRef == "" && (oldEndpoint.URL != "" || newEndpoint.URL != "") {
		installation, err := browsermgr.New(paths).Resolve(backend)
		if err != nil {
			return preparedCompareSession{}, preparedCompareSession{}, err
		}
		resolvedTargetRef = installation.ExecutablePath
	}

	width, height, err := resolvedViewport(viewport)
	if err != nil {
		return preparedCompareSession{}, preparedCompareSession{}, err
	}

	oldPrepared, err := prepareCompareSession(ctx, client, "old", oldEndpoint, backend, resolvedTargetRef, width, height)
	if err != nil {
		return preparedCompareSession{}, preparedCompareSession{}, err
	}
	newPrepared, err := prepareCompareSession(ctx, client, "new", newEndpoint, backend, resolvedTargetRef, width, height)
	if err != nil {
		cleanupCompareSession(context.Background(), client, oldPrepared)
		return preparedCompareSession{}, preparedCompareSession{}, err
	}

	return oldPrepared, newPrepared, nil
}

func prepareCompareSession(ctx context.Context, client *rpc.Client, label string, endpoint compareEndpoint, backend string, targetRef string, width int, height int) (preparedCompareSession, error) {
	if endpoint.SessionID != "" {
		return preparedCompareSession{SessionID: endpoint.SessionID}, nil
	}

	sessionID := fmt.Sprintf("compare-%s-%s", label, newCompareSessionSuffix())
	res, err := client.AttachSession(ctx, api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  sessionID,
		TargetRef:  targetRef,
		Backend:    backend,
		Options: map[string]string{
			"initial_url":     endpoint.URL,
			"viewport_width":  strconv.Itoa(width),
			"viewport_height": strconv.Itoa(height),
		},
	})
	if err != nil {
		return preparedCompareSession{}, err
	}

	return preparedCompareSession{
		SessionID: res.Session.ID,
		Detach:    true,
	}, nil
}

func cleanupCompareSession(ctx context.Context, client *rpc.Client, prepared preparedCompareSession) {
	if !prepared.Detach || prepared.SessionID == "" {
		return
	}
	detachCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, _ = client.DetachSession(detachCtx, api.DetachSessionRequest{SessionID: prepared.SessionID})
}

func waitForCompareSelector(ctx context.Context, client *rpc.Client, sessionID string, selector string, timeout int) error {
	return runCompareWaitAction(ctx, client, sessionID, map[string]string{
		"target":     "selector",
		"value":      selector,
		"state":      "visible",
		"timeout_ms": strconv.Itoa(timeout),
	})
}

func waitForCompareFunction(ctx context.Context, client *rpc.Client, sessionID string, source string, timeout int) error {
	return runCompareWaitAction(ctx, client, sessionID, map[string]string{
		"target":     "function",
		"value":      source,
		"timeout_ms": strconv.Itoa(timeout),
	})
}

func runCompareWaitAction(ctx context.Context, client *rpc.Client, sessionID string, args map[string]string) error {
	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: sessionID,
		Action: api.Action{
			Kind: "wait",
			Args: args,
		},
	})
	if err != nil {
		return err
	}
	if res.Result.OK {
		return nil
	}
	if strings.TrimSpace(res.Result.Message) != "" {
		return errors.New(res.Result.Message)
	}
	return fmt.Errorf("wait %s failed", strings.TrimSpace(args["target"]))
}

func observeCompareSession(ctx context.Context, client *rpc.Client, sessionID string, cssProperties []string) (api.Observation, error) {
	res, err := client.ObserveSession(ctx, api.ObserveSessionRequest{
		SessionID: sessionID,
		Options: api.ObserveOptions{
			WithText:      true,
			WithTree:      true,
			CSSProperties: append([]string(nil), cssProperties...),
		},
	})
	if err != nil {
		return api.Observation{}, err
	}
	return res.Observation, nil
}

func observeScopedCompareSession(ctx context.Context, client *rpc.Client, sessionID string, cssProperties []string, scopeSelector string) (api.Observation, error) {
	res, err := client.ObserveSession(ctx, api.ObserveSessionRequest{
		SessionID: sessionID,
		Options: api.ObserveOptions{
			WithText:      true,
			WithTree:      true,
			CSSProperties: append([]string(nil), cssProperties...),
			ScopeSelector: strings.TrimSpace(scopeSelector),
		},
	})
	if err != nil {
		return api.Observation{}, err
	}
	return res.Observation, nil
}

func waitForCompareURLReady(ctx context.Context, client *rpc.Client, sessionID string) error {
	waitCtx, cancel := context.WithTimeout(ctx, compareURLReadyTimeout)
	defer cancel()

	for {
		observation, err := observeCompareSession(waitCtx, client, sessionID, nil)
		if err != nil {
			return err
		}
		currentURL := strings.TrimSpace(observation.URLOrScreen)
		if currentURL != "" && currentURL != "about:blank" {
			return nil
		}

		select {
		case <-waitCtx.Done():
			return fmt.Errorf("session %s stayed on about:blank", sessionID)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func waitForCompareDocumentReady(ctx context.Context, client *rpc.Client, sessionID string, timeout int) error {
	return waitForCompareFunction(ctx, client, sessionID, `document.readyState === "complete"`, timeout)
}

func waitForCompareNetworkIdle(ctx context.Context, client *rpc.Client, sessionID string, timeout int) error {
	idleWindow := strconv.FormatInt(compareNetworkIdleWindow.Milliseconds(), 10)
	return waitForCompareFunction(ctx, client, sessionID, `(function () {
  if (document.readyState !== "complete") {
    return false;
  }
  const now = performance.now();
  let last = 0;
  const navigation = performance.getEntriesByType("navigation");
  if (navigation.length > 0) {
    const entry = navigation[0];
    last = Math.max(last, entry.loadEventEnd || entry.domComplete || entry.responseEnd || 0);
  }
  const resources = performance.getEntriesByType("resource");
  for (const entry of resources) {
    const end = entry.responseEnd || entry.fetchStart || entry.startTime || 0;
    if (end > last) {
      last = end;
    }
  }
  return now - last >= `+idleWindow+`;
})()`, timeout)
}
