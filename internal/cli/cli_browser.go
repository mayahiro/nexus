package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	nagicli "github.com/mayahiro/nagicli-go"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/browsermgr"
	"github.com/mayahiro/nexus/internal/config"
)

func runBrowserSetupInvocation(ctx context.Context, _ *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	manager := newBrowserManager(paths)
	result, err := manager.Setup(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	printBrowserResults(stdout, result)
	return 0
}

func runBrowserUpdateInvocation(ctx context.Context, _ *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	result, err := newBrowserManager(paths).Update(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	printBrowserResults(stdout, result)
	return 0
}

func runBrowserStatusInvocation(_ context.Context, _ *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	status, err := newBrowserManager(paths).Status()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	printBrowserStatus(stdout, status)
	return 0
}

func runBrowserUninstallInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	names := []string{}
	if name := nagiStringValue(invocation, "name"); name != "" {
		names = append(names, name)
	}
	result, err := newBrowserManager(paths).Uninstall(ctx, names...)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	printBrowserResults(stdout, browsermgr.SetupResult{Browsers: result.Browsers})
	return 0
}

type attachBrowserArguments struct {
	SessionID  string
	Backend    string
	TargetRef  string
	InitialURL string
	Viewport   nagiViewport
}

func runOpenInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	viewport, ok := nagiViewportValue(invocation, "viewport")
	if !ok {
		viewport = nagiViewport{Width: defaultViewportWidth, Height: defaultViewportHeight}
	}
	return executeAttachBrowser(ctx, attachBrowserArguments{
		SessionID:  nagiStringValue(invocation, "session"),
		Backend:    nagiStringValue(invocation, "backend"),
		TargetRef:  nagiStringValue(invocation, "target-ref"),
		InitialURL: nagiStringValue(invocation, "url"),
		Viewport:   viewport,
	}, stdout, stderr)
}

func runNavigateInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	sessionID := nagiStringValue(invocation, "session")
	asJSON := nagiBoolValue(invocation, "json")
	url := nagiStringValue(invocation, "url")

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: sessionID,
		Action: api.Action{
			Kind: "navigate",
			Args: map[string]string{
				"url": url,
			},
		},
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if !res.Result.OK {
		if res.Result.Message != "" {
			fmt.Fprintln(stderr, res.Result.Message)
		}
		return 1
	}

	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(res.Result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	if res.Result.Message != "" {
		fmt.Fprintln(stdout, res.Result.Message)
		return 0
	}

	fmt.Fprintf(stdout, "navigated to %s\n", url)
	return 0
}

func runAttachBrowserInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	viewport, ok := nagiViewportValue(invocation, "viewport")
	if !ok {
		viewport = nagiViewport{Width: defaultViewportWidth, Height: defaultViewportHeight}
	}
	return executeAttachBrowser(ctx, attachBrowserArguments{
		SessionID:  nagiStringValue(invocation, "session"),
		Backend:    nagiStringValue(invocation, "backend"),
		TargetRef:  nagiStringValue(invocation, "target-ref"),
		InitialURL: nagiStringValue(invocation, "url"),
		Viewport:   viewport,
	}, stdout, stderr)
}

func executeAttachBrowser(ctx context.Context, arguments attachBrowserArguments, stdout io.Writer, stderr io.Writer) int {
	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	resolvedTargetRef := arguments.TargetRef
	if resolvedTargetRef == "" {
		installation, err := newBrowserManager(paths).Resolve(arguments.Backend)
		if err != nil {
			if errors.Is(err, browsermgr.ErrBrowserNotInstalled) {
				fmt.Fprintf(stderr, "%s is not installed. run `nxctl browser setup` first\n", arguments.Backend)
				return 1
			}
			fmt.Fprintln(stderr, err)
			return 1
		}
		resolvedTargetRef = installation.ExecutablePath
	}

	options := map[string]string{}
	if arguments.InitialURL != "" {
		options["initial_url"] = arguments.InitialURL
	}
	options["viewport_width"] = strconv.Itoa(arguments.Viewport.Width)
	options["viewport_height"] = strconv.Itoa(arguments.Viewport.Height)

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.AttachSession(ctx, api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  arguments.SessionID,
		TargetRef:  resolvedTargetRef,
		Backend:    arguments.Backend,
		Options:    options,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "attached %s %s (%s) %s\n", res.Session.TargetType, res.Session.ID, res.Session.Backend, res.Session.TargetRef)
	return 0
}
