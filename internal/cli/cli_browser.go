package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/browsermgr"
	"github.com/mayahiro/nexus/internal/config"
)

func runBrowser(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printBrowserHelp(stdout)
		return 0
	}
	if len(args) == 0 {
		printBrowserHelp(stderr)
		return 1
	}

	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	manager := newBrowserManager(paths)

	switch args[0] {
	case "setup":
		result, err := manager.Setup(ctx)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		printBrowserResults(stdout, result)
		return 0
	case "update":
		result, err := manager.Update(ctx)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		printBrowserResults(stdout, result)
		return 0
	case "status":
		status, err := manager.Status()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		printBrowserStatus(stdout, status)
		return 0
	case "uninstall":
		fs := flag.NewFlagSet("browser uninstall", flag.ContinueOnError)
		fs.SetOutput(stderr)
		name := fs.String("name", "", "browser name")
		if err := parseCommandFlags(fs, args[1:], stderr, "browser"); err != nil {
			return 1
		}
		names := []string{}
		if *name != "" {
			names = append(names, *name)
		}
		result, err := manager.Uninstall(ctx, names...)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		printBrowserResults(stdout, browsermgr.SetupResult{Browsers: result.Browsers})
		return 0
	default:
		if args[0] == "help" {
			printBrowserHelp(stdout)
			return 0
		}
		fmt.Fprintf(stderr, "unknown browser subcommand: %s\n", args[0])
		printBrowserHelp(stderr)
		return 1
	}
}

func runAttach(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printAttachHelp(stdout)
		return 0
	}
	if len(args) == 0 {
		printAttachHelp(stderr)
		return 1
	}

	switch args[0] {
	case "browser":
		return runAttachBrowser(ctx, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown target type: %s\n", args[0])
		printAttachHelp(stderr)
		return 1
	}
}

func runOpen(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printOpenHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("open", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	backend := fs.String("backend", "chromium", "browser backend")
	targetRef := fs.String("target-ref", "", "target ref")
	viewport := fs.String("viewport", "", "viewport as WIDTHxHEIGHT")
	urlArg := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		urlArg = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "open"); err != nil {
		return 1
	}

	if urlArg == "" && fs.NArg() == 1 {
		urlArg = fs.Arg(0)
	}

	if urlArg == "" || fs.NArg() > 1 {
		fmt.Fprintln(stderr, "open requires a url")
		printCommandHint(stderr, "open", "nxctl open https://example.com --session work")
		return 1
	}

	openArgs := []string{
		"--session", *sessionID,
		"--backend", *backend,
		"--url", urlArg,
	}
	if *viewport != "" {
		openArgs = append(openArgs, "--viewport", *viewport)
	}
	if *targetRef != "" {
		openArgs = append(openArgs, "--target-ref", *targetRef)
	}

	return runAttachBrowser(ctx, openArgs, stdout, stderr)
}

func runNavigate(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printNavigateHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("navigate", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "default", "session id")
	asJSON := fs.Bool("json", false, "print as json")
	urlArg := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		urlArg = args[0]
		args = args[1:]
	}

	if err := parseCommandFlags(fs, args, stderr, "navigate"); err != nil {
		return 1
	}

	if urlArg == "" && fs.NArg() == 1 {
		urlArg = fs.Arg(0)
	}

	if urlArg == "" || fs.NArg() > 1 {
		fmt.Fprintln(stderr, "navigate requires a url")
		printCommandHint(stderr, "navigate", "nxctl navigate https://example.com --session work")
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: *sessionID,
		Action: api.Action{
			Kind: "navigate",
			Args: map[string]string{
				"url": urlArg,
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

	if *asJSON {
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

	fmt.Fprintf(stdout, "navigated to %s\n", urlArg)
	return 0
}

func runAttachBrowser(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printAttachBrowserHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("attach browser", flag.ContinueOnError)
	fs.SetOutput(stderr)

	sessionID := fs.String("session", "", "session id")
	backend := fs.String("backend", "chromium", "browser backend")
	targetRef := fs.String("target-ref", "", "target ref")
	initialURL := fs.String("url", "", "initial url")
	viewport := fs.String("viewport", "", "viewport as WIDTHxHEIGHT")

	if err := parseCommandFlags(fs, args, stderr, "attach"); err != nil {
		return 1
	}

	if *sessionID == "" {
		fmt.Fprintln(stderr, "--session is required")
		printCommandHint(stderr, "attach", "nxctl attach browser --session work --backend chromium --url https://example.com")
		return 1
	}

	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	resolvedTargetRef := *targetRef
	if resolvedTargetRef == "" {
		installation, err := newBrowserManager(paths).Resolve(*backend)
		if err != nil {
			if errors.Is(err, browsermgr.ErrBrowserNotInstalled) {
				fmt.Fprintf(stderr, "%s is not installed. run `nxctl browser setup` first\n", *backend)
				return 1
			}
			fmt.Fprintln(stderr, err)
			return 1
		}
		resolvedTargetRef = installation.ExecutablePath
	}

	options := map[string]string{}
	if *initialURL != "" {
		options["initial_url"] = *initialURL
	}
	width, height, err := resolvedViewport(*viewport)
	if err != nil {
		fmt.Fprintln(stderr, err)
		printCommandHint(stderr, "attach", "nxctl attach browser --session work --viewport 1440x900")
		return 1
	}
	options["viewport_width"] = strconv.Itoa(width)
	options["viewport_height"] = strconv.Itoa(height)

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	res, err := client.AttachSession(ctx, api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  *sessionID,
		TargetRef:  resolvedTargetRef,
		Backend:    *backend,
		Options:    options,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "attached %s %s (%s) %s\n", res.Session.TargetType, res.Session.ID, res.Session.Backend, res.Session.TargetRef)
	return 0
}
