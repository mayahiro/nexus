package comparecmd

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/rpc"
)

func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, connectClient func(context.Context) (*rpc.Client, error)) int {
	if isHelpArgs(args) {
		PrintHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	fs.SetOutput(stderr)

	positional := make([]string, 0, 2)
	for len(args) > 0 && len(positional) < 2 && !strings.HasPrefix(args[0], "-") {
		positional = append(positional, args[0])
		args = args[1:]
	}

	oldSession := fs.String("old-session", "", "old session id")
	newSession := fs.String("new-session", "", "new session id")
	oldURL := fs.String("old-url", "", "old url")
	newURL := fs.String("new-url", "", "new url")
	backend := fs.String("backend", "chromium", "browser backend")
	targetRef := fs.String("target-ref", "", "target ref")
	viewport := fs.String("viewport", "", "viewport as WIDTHxHEIGHT")
	manifestPath := fs.String("manifest", "", "compare manifest json")
	continueOnError := fs.Bool("continue-on-error", false, "continue after manifest page error")
	limit := fs.Int("limit", 0, "limit manifest pages")
	waitSelector := fs.String("wait-selector", "", "wait selector before compare")
	waitTimeout := fs.Int("wait-timeout", 10000, "wait timeout in ms")
	asJSON := fs.Bool("json", false, "print as json")
	outputJSON := fs.String("output-json", "", "write compare report json to file")
	outputMD := fs.String("output-md", "", "write compare report markdown to file")
	var ignoreRegex compareStringValues
	var ignoreSelector compareStringValues
	var maskSelector compareStringValues
	fs.Var(&ignoreRegex, "ignore-text-regex", "regex to strip from text before compare")
	fs.Var(&ignoreSelector, "ignore-selector", "node selector to ignore such as @e3, role=button, text=Save")
	fs.Var(&maskSelector, "mask-selector", "node selector to mask such as @e3, role=textbox, testid=user-id")

	if err := parseCommandFlags(fs, args, stderr, "compare"); err != nil {
		return 1
	}

	if strings.TrimSpace(*manifestPath) != "" {
		if len(positional) > 0 || fs.NArg() > 0 || *oldURL != "" || *newURL != "" || *oldSession != "" || *newSession != "" {
			fmt.Fprintln(stderr, "compare can not mix --manifest with urls or session flags")
			fmt.Fprintln(stderr, "hint: nxctl compare --manifest migration-pages.json")
			fmt.Fprintln(stderr, "hint: run `nxctl help compare` for details")
			return 1
		}
	} else if len(positional) == 2 && *oldURL == "" && *newURL == "" && *oldSession == "" && *newSession == "" {
		*oldURL = positional[0]
		*newURL = positional[1]
	} else if fs.NArg() == 2 && *oldURL == "" && *newURL == "" && *oldSession == "" && *newSession == "" {
		*oldURL = fs.Arg(0)
		*newURL = fs.Arg(1)
	} else if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "compare accepts either two urls, two sessions, or --manifest")
		PrintHelp(stderr)
		return 1
	}

	if *waitTimeout < 0 {
		fmt.Fprintln(stderr, "wait-timeout must be a non-negative integer")
		return 1
	}
	if *limit < 0 {
		fmt.Fprintln(stderr, "limit must be a non-negative integer")
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	base := compareRun{
		Backend:         *backend,
		TargetRef:       *targetRef,
		Viewport:        *viewport,
		WaitSelector:    *waitSelector,
		WaitTimeout:     *waitTimeout,
		IgnoreTextRegex: append([]string(nil), ignoreRegex...),
		IgnoreSelector:  append([]string(nil), ignoreSelector...),
		MaskSelector:    append([]string(nil), maskSelector...),
	}

	if strings.TrimSpace(*manifestPath) != "" {
		manifest, err := loadCompareManifest(*manifestPath)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		report, err := executeCompareManifest(ctx, client, paths, *manifestPath, manifest, base, *continueOnError, *limit)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if strings.TrimSpace(*outputJSON) != "" {
			if err := writeIndentedJSONFile(*outputJSON, report); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		}
		if strings.TrimSpace(*outputMD) != "" {
			if err := writeCompareManifestMarkdown(*outputMD, report); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		}
		if *asJSON {
			encoder := json.NewEncoder(stdout)
			encoder.SetIndent("", "  ")
			if err := encoder.Encode(report); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			return 0
		}
		printCompareManifestReport(stdout, report)
		return 0
	}

	base.OldEndpoint = compareEndpoint{SessionID: strings.TrimSpace(*oldSession), URL: strings.TrimSpace(*oldURL)}
	base.NewEndpoint = compareEndpoint{SessionID: strings.TrimSpace(*newSession), URL: strings.TrimSpace(*newURL)}
	if base.OldEndpoint.SessionID == "" && base.OldEndpoint.URL == "" && base.NewEndpoint.SessionID == "" && base.NewEndpoint.URL == "" {
		fmt.Fprintln(stderr, "compare requires either two urls, two sessions, or --manifest")
		fmt.Fprintln(stderr, "hint: nxctl compare https://old.example.com https://new.example.com")
		fmt.Fprintln(stderr, "hint: run `nxctl help compare` for details")
		return 1
	}

	report, err := executeCompare(ctx, client, paths, base)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if strings.TrimSpace(*outputJSON) != "" {
		if err := writeIndentedJSONFile(*outputJSON, report); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if strings.TrimSpace(*outputMD) != "" {
		if err := writeCompareMarkdown(*outputMD, report); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	printCompareReport(stdout, report)
	return 0
}

func executeCompare(ctx context.Context, client *rpc.Client, paths config.Paths, run compareRun) (compareReport, error) {
	if err := validateCompareEndpoint("old", run.OldEndpoint); err != nil {
		return compareReport{}, err
	}
	if err := validateCompareEndpoint("new", run.NewEndpoint); err != nil {
		return compareReport{}, err
	}
	if run.WaitTimeout < 0 {
		return compareReport{}, errors.New("wait-timeout must be a non-negative integer")
	}

	ignorePatterns, err := compileCompareRegexps(run.IgnoreTextRegex)
	if err != nil {
		return compareReport{}, err
	}
	ignoreRules, err := compileCompareSelectorRules(run.IgnoreSelector)
	if err != nil {
		return compareReport{}, err
	}
	maskRules, err := compileCompareSelectorRules(run.MaskSelector)
	if err != nil {
		return compareReport{}, err
	}

	oldPrepared, newPrepared, err := prepareCompareSessions(ctx, client, paths, run.OldEndpoint, run.NewEndpoint, run.Backend, run.TargetRef, run.Viewport)
	if err != nil {
		return compareReport{}, err
	}
	defer cleanupCompareSession(context.Background(), client, oldPrepared)
	defer cleanupCompareSession(context.Background(), client, newPrepared)

	for _, endpoint := range []struct {
		prepared preparedCompareSession
		source   compareEndpoint
	}{
		{prepared: oldPrepared, source: run.OldEndpoint},
		{prepared: newPrepared, source: run.NewEndpoint},
	} {
		if endpoint.source.URL == "" {
			continue
		}
		if err := waitForCompareURLReady(ctx, client, endpoint.prepared.SessionID); err != nil {
			return compareReport{}, err
		}
	}

	if strings.TrimSpace(run.WaitSelector) != "" {
		for _, prepared := range []preparedCompareSession{oldPrepared, newPrepared} {
			if err := waitForCompareSelector(ctx, client, prepared.SessionID, run.WaitSelector, run.WaitTimeout); err != nil {
				return compareReport{}, err
			}
		}
	}

	oldObservation, err := observeCompareSession(ctx, client, oldPrepared.SessionID)
	if err != nil {
		return compareReport{}, err
	}
	newObservation, err := observeCompareSession(ctx, client, newPrepared.SessionID)
	if err != nil {
		return compareReport{}, err
	}

	return buildCompareReport(
		buildCompareSnapshot(oldObservation, compareSnapshotOptions{
			IgnoreText: ignorePatterns,
			IgnoreNode: ignoreRules,
			MaskNode:   maskRules,
		}),
		buildCompareSnapshot(newObservation, compareSnapshotOptions{
			IgnoreText: ignorePatterns,
			IgnoreNode: ignoreRules,
			MaskNode:   maskRules,
		}),
	), nil
}
