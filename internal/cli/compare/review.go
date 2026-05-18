package comparecmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/rpc"
)

const (
	compareReviewFileJSON                     = "compare.json"
	compareReviewFileMarkdown                 = "compare.md"
	compareReviewFilePairDecisionsTemplate    = "pair-decisions.todo.jsonl"
	compareReviewFileFindingDecisionsTemplate = "finding-decisions.todo.jsonl"
	compareReviewFileOldScreenshot            = "old.png"
	compareReviewFileNewScreenshot            = "new.png"
	compareReviewFileSummary                  = "review-summary.json"
	compareReviewFileManifestJSON             = "manifest.json"
	compareReviewFileManifestMarkdown         = "manifest.md"
)

type compareReviewScreenshots struct {
	Old      []byte
	New      []byte
	Warnings []string
}

func writeCompareReviewPacket(dir string, report compareReport, screenshots compareReviewScreenshots) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := compareReviewFiles{
		CompareJSON:              filepath.Join(dir, compareReviewFileJSON),
		CompareMarkdown:          filepath.Join(dir, compareReviewFileMarkdown),
		PairDecisionsTemplate:    filepath.Join(dir, compareReviewFilePairDecisionsTemplate),
		FindingDecisionsTemplate: filepath.Join(dir, compareReviewFileFindingDecisionsTemplate),
		ReviewSummary:            filepath.Join(dir, compareReviewFileSummary),
	}
	if len(screenshots.Old) > 0 {
		files.OldScreenshot = filepath.Join(dir, compareReviewFileOldScreenshot)
		if err := os.WriteFile(files.OldScreenshot, screenshots.Old, 0o644); err != nil {
			return err
		}
	}
	if len(screenshots.New) > 0 {
		files.NewScreenshot = filepath.Join(dir, compareReviewFileNewScreenshot)
		if err := os.WriteFile(files.NewScreenshot, screenshots.New, 0o644); err != nil {
			return err
		}
	}
	if err := writeCompareJSON(files.CompareJSON, report); err != nil {
		return err
	}
	if err := writeCompareMarkdown(files.CompareMarkdown, report); err != nil {
		return err
	}
	if err := writeCompareDecisionsTemplate(files.PairDecisionsTemplate, report.MatchingDebug); err != nil {
		return err
	}
	if err := writeCompareFindingDecisionsTemplate(files.FindingDecisionsTemplate, report); err != nil {
		return err
	}
	return writeIndentedJSONFile(files.ReviewSummary, buildCompareReviewSummary(report, files, screenshots.Warnings))
}

func buildCompareReviewSummary(report compareReport, files compareReviewFiles, screenshotWarnings []string) compareReviewSummary {
	summary := compareReviewSummary{
		Old:                     firstNonEmpty(report.Old.URL, report.Old.SessionID),
		New:                     firstNonEmpty(report.New.URL, report.New.SessionID),
		Same:                    report.Summary.Same,
		TotalFindings:           report.Summary.TotalFindings,
		CriticalFindings:        report.Summary.Critical,
		WarningFindings:         report.Summary.Warning,
		InfoFindings:            report.Summary.Info,
		MatchedNodes:            report.Summary.MatchedNodes,
		AmbiguousMatchesSkipped: report.Summary.AmbiguousMatchesSkipped,
		Files:                   files,
		ScreenshotWarnings:      append([]string(nil), screenshotWarnings...),
	}
	if report.Scope != nil {
		summary.Scope = compareScopeLabel(report.Scope)
	}
	if report.MatchingDebug != nil {
		summary.AmbiguousCandidates = len(report.MatchingDebug.AmbiguousCandidates)
		summary.UnmatchedOld = len(report.MatchingDebug.UnmatchedOld)
		summary.UnmatchedNew = len(report.MatchingDebug.UnmatchedNew)
	}
	summary.NextCommands = []string{
		"nxctl compare validate-decisions --decisions-file " + files.PairDecisionsTemplate + " --compare-json " + files.CompareJSON,
		"nxctl compare validate-decisions --decisions-file " + files.FindingDecisionsTemplate + " --compare-json " + files.CompareJSON,
		"nxctl compare normalize-decisions --decisions-file " + files.PairDecisionsTemplate + " --compare-json " + files.CompareJSON + " --output " + filepath.Join(filepath.Dir(files.PairDecisionsTemplate), "pair-decisions.normalized.jsonl"),
	}
	return summary
}

func captureCompareReviewScreenshots(ctx context.Context, client *rpc.Client, oldSessionID string, newSessionID string) compareReviewScreenshots {
	screenshots := compareReviewScreenshots{}
	oldScreenshot, err := captureCompareReviewScreenshot(ctx, client, oldSessionID)
	if err != nil {
		screenshots.Warnings = append(screenshots.Warnings, "old screenshot: "+err.Error())
	} else {
		screenshots.Old = oldScreenshot
	}
	newScreenshot, err := captureCompareReviewScreenshot(ctx, client, newSessionID)
	if err != nil {
		screenshots.Warnings = append(screenshots.Warnings, "new screenshot: "+err.Error())
	} else {
		screenshots.New = newScreenshot
	}
	return screenshots
}

func captureCompareReviewScreenshot(ctx context.Context, client *rpc.Client, sessionID string) ([]byte, error) {
	res, err := client.ObserveSession(ctx, api.ObserveSessionRequest{
		SessionID: sessionID,
		Options: api.ObserveOptions{
			WithScreenshot: true,
		},
	})
	if err != nil {
		return nil, err
	}
	screenshot := strings.TrimSpace(res.Observation.Screenshot)
	if screenshot == "" {
		return nil, fmt.Errorf("empty screenshot")
	}
	return base64.StdEncoding.DecodeString(screenshot)
}

func writeCompareManifestReviewPacket(dir string, report compareManifestReport, pageDirectories []compareManifestReviewPageDirectory) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := compareManifestReviewFiles{
		ManifestJSON:     filepath.Join(dir, compareReviewFileManifestJSON),
		ManifestMarkdown: filepath.Join(dir, compareReviewFileManifestMarkdown),
		ReviewSummary:    filepath.Join(dir, compareReviewFileSummary),
		PageDirectories:  append([]compareManifestReviewPageDirectory(nil), pageDirectories...),
	}
	if err := writeIndentedJSONFile(files.ManifestJSON, report); err != nil {
		return err
	}
	if err := writeCompareManifestMarkdown(files.ManifestMarkdown, report); err != nil {
		return err
	}
	return writeIndentedJSONFile(files.ReviewSummary, buildCompareManifestReviewSummary(report, files))
}

func buildCompareManifestReviewSummary(report compareManifestReport, files compareManifestReviewFiles) compareManifestReviewSummary {
	return compareManifestReviewSummary{
		Manifest:         report.Manifest,
		TotalPages:       report.Summary.TotalPages,
		ComparedPages:    report.Summary.ComparedPages,
		FailedPages:      report.Summary.FailedPages,
		SamePages:        report.Summary.SamePages,
		DifferentPages:   report.Summary.DifferentPages,
		TotalFindings:    report.Summary.TotalFindings,
		CriticalFindings: report.Summary.Critical,
		WarningFindings:  report.Summary.Warning,
		InfoFindings:     report.Summary.Info,
		Files:            files,
	}
}
