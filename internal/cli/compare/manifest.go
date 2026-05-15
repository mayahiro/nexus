package comparecmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/rpc"
)

func executeCompareManifest(ctx context.Context, client *rpc.Client, paths config.Paths, manifestPath string, manifest compareManifest, base compareRun, continueOnError bool, limit int) (compareManifestReport, error) {
	pages := manifest.Pages
	if len(pages) == 0 {
		return compareManifestReport{}, errors.New("manifest requires at least one page")
	}
	if limit > 0 && limit < len(pages) {
		pages = pages[:limit]
	}

	report := compareManifestReport{
		Manifest: manifestPath,
		Pages:    make([]compareManifestPageReport, 0, len(pages)),
	}

	for i, page := range pages {
		name := compareManifestPageName(page, i)
		run := mergeCompareManifestPage(base, manifest.Defaults, page)
		single, err := executeCompare(ctx, client, paths, run)
		if err != nil {
			if !continueOnError {
				return compareManifestReport{}, fmt.Errorf("manifest %s failed: %w", name, err)
			}
			report.Pages = append(report.Pages, compareManifestPageReport{
				Name:  name,
				Error: err.Error(),
			})
			continue
		}
		report.Pages = append(report.Pages, compareManifestPageReport{
			Name:   name,
			Report: &single,
		})
	}

	report.Summary = summarizeCompareManifest(report.Pages)
	return report, nil
}

func loadCompareManifest(path string) (compareManifest, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return compareManifest{}, err
	}
	var manifest compareManifest
	if err := json.Unmarshal(bytes, &manifest); err != nil {
		return compareManifest{}, fmt.Errorf("invalid compare manifest %q: %w", path, err)
	}
	if len(manifest.Pages) == 0 {
		return compareManifest{}, fmt.Errorf("manifest %q requires at least one page", path)
	}
	return manifest, nil
}

func mergeCompareManifestPage(base compareRun, defaults compareManifestDefaults, page compareManifestPage) compareRun {
	run := compareRun{
		OldEndpoint: compareEndpoint{
			SessionID: strings.TrimSpace(page.OldSession),
			URL:       strings.TrimSpace(page.OldURL),
		},
		NewEndpoint: compareEndpoint{
			SessionID: strings.TrimSpace(page.NewSession),
			URL:       strings.TrimSpace(page.NewURL),
		},
		Backend:          base.Backend,
		TargetRef:        base.TargetRef,
		Viewport:         base.Viewport,
		MatchMode:        base.MatchMode,
		WaitSelector:     base.WaitSelector,
		ScopeSelector:    base.ScopeSelector,
		OldScopeSelector: base.OldScopeSelector,
		NewScopeSelector: base.NewScopeSelector,
		WaitFunction:     base.WaitFunction,
		WaitNetworkIdle:  base.WaitNetworkIdle,
		CompareCSS:       base.CompareCSS,
		CompareLayout:    base.CompareLayout,
		WaitTimeout:      base.WaitTimeout,
		CSSProperties:    append([]string(nil), base.CSSProperties...),
		IgnoreTextRegex:  append([]string(nil), base.IgnoreTextRegex...),
		IgnoreSelector:   append([]string(nil), base.IgnoreSelector...),
		MaskSelector:     append([]string(nil), base.MaskSelector...),
	}

	if defaults.WaitSelector != "" {
		run.WaitSelector = defaults.WaitSelector
	}
	if defaults.ScopeSelector != "" {
		run.ScopeSelector = defaults.ScopeSelector
		run.OldScopeSelector = ""
		run.NewScopeSelector = ""
	}
	if defaults.OldScopeSelector != "" {
		run.OldScopeSelector = defaults.OldScopeSelector
	}
	if defaults.NewScopeSelector != "" {
		run.NewScopeSelector = defaults.NewScopeSelector
	}
	if strings.TrimSpace(defaults.Backend) != "" {
		run.Backend = strings.TrimSpace(defaults.Backend)
	}
	if strings.TrimSpace(defaults.Viewport) != "" {
		run.Viewport = strings.TrimSpace(defaults.Viewport)
	}
	if strings.TrimSpace(defaults.MatchMode) != "" {
		run.MatchMode = strings.TrimSpace(defaults.MatchMode)
	}
	if defaults.WaitFunction != "" {
		run.WaitFunction = defaults.WaitFunction
	}
	if defaults.WaitNetworkIdle {
		run.WaitNetworkIdle = true
	}
	if defaults.CompareCSS {
		run.CompareCSS = true
	}
	if defaults.CompareLayout {
		run.CompareLayout = true
	}
	if defaults.WaitTimeout != nil {
		run.WaitTimeout = *defaults.WaitTimeout
	}
	if len(defaults.CSSProperty) > 0 {
		run.CSSProperties = append([]string(nil), defaults.CSSProperty...)
	}
	run.IgnoreTextRegex = append(run.IgnoreTextRegex, defaults.IgnoreTextRegex...)
	run.IgnoreSelector = append(run.IgnoreSelector, defaults.IgnoreSelector...)
	run.MaskSelector = append(run.MaskSelector, defaults.MaskSelector...)

	if page.WaitSelector != nil {
		run.WaitSelector = strings.TrimSpace(*page.WaitSelector)
	}
	if page.ScopeSelector != nil {
		run.ScopeSelector = strings.TrimSpace(*page.ScopeSelector)
		run.OldScopeSelector = ""
		run.NewScopeSelector = ""
	}
	if page.OldScopeSelector != nil {
		run.OldScopeSelector = strings.TrimSpace(*page.OldScopeSelector)
	}
	if page.NewScopeSelector != nil {
		run.NewScopeSelector = strings.TrimSpace(*page.NewScopeSelector)
	}
	if page.Backend != nil && strings.TrimSpace(*page.Backend) != "" {
		run.Backend = strings.TrimSpace(*page.Backend)
	}
	if page.Viewport != nil && strings.TrimSpace(*page.Viewport) != "" {
		run.Viewport = strings.TrimSpace(*page.Viewport)
	}
	if page.MatchMode != nil {
		run.MatchMode = strings.TrimSpace(*page.MatchMode)
	}
	if page.WaitFunction != nil {
		run.WaitFunction = strings.TrimSpace(*page.WaitFunction)
	}
	if page.WaitNetworkIdle != nil {
		run.WaitNetworkIdle = *page.WaitNetworkIdle
	}
	if page.CompareCSS != nil {
		run.CompareCSS = *page.CompareCSS
		if !*page.CompareCSS && len(page.CSSProperty) == 0 {
			run.CSSProperties = nil
		}
	}
	if page.CompareLayout != nil {
		run.CompareLayout = *page.CompareLayout
	}
	if page.WaitTimeout != nil {
		run.WaitTimeout = *page.WaitTimeout
	}
	if len(page.CSSProperty) > 0 {
		run.CSSProperties = append([]string(nil), page.CSSProperty...)
	}
	run.IgnoreTextRegex = append(run.IgnoreTextRegex, page.IgnoreTextRegex...)
	run.IgnoreSelector = append(run.IgnoreSelector, page.IgnoreSelector...)
	run.MaskSelector = append(run.MaskSelector, page.MaskSelector...)
	return run
}

func compareManifestPageName(page compareManifestPage, index int) string {
	if strings.TrimSpace(page.Name) != "" {
		return strings.TrimSpace(page.Name)
	}
	return fmt.Sprintf("page[%d]", index)
}

func summarizeCompareManifest(pages []compareManifestPageReport) compareManifestSummary {
	summary := compareManifestSummary{
		TotalPages: len(pages),
	}
	for _, page := range pages {
		if page.Error != "" {
			summary.FailedPages++
			continue
		}
		if page.Report == nil {
			summary.FailedPages++
			continue
		}
		summary.ComparedPages++
		if page.Report.Summary.Same {
			summary.SamePages++
		} else {
			summary.DifferentPages++
		}
		summary.TotalFindings += page.Report.Summary.TotalFindings
		summary.Critical += page.Report.Summary.Critical
		summary.Warning += page.Report.Summary.Warning
		summary.Info += page.Report.Summary.Info
	}
	return summary
}
