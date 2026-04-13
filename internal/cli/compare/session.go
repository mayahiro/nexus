package comparecmd

import (
	"context"
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
	_, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: sessionID,
		Action: api.Action{
			Kind: "wait",
			Args: map[string]string{
				"target":     "selector",
				"value":      selector,
				"state":      "visible",
				"timeout_ms": strconv.Itoa(timeout),
			},
		},
	})
	return err
}

func observeCompareSession(ctx context.Context, client *rpc.Client, sessionID string) (api.Observation, error) {
	res, err := client.ObserveSession(ctx, api.ObserveSessionRequest{
		SessionID: sessionID,
		Options: api.ObserveOptions{
			WithText: true,
			WithTree: true,
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
		observation, err := observeCompareSession(waitCtx, client, sessionID)
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
