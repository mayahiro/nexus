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
	compareReviewFileIndex                    = "review-index.md"
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
		ReviewIndex:      filepath.Join(dir, compareReviewFileIndex),
		ReviewSummary:    filepath.Join(dir, compareReviewFileSummary),
		PageDirectories:  append([]compareManifestReviewPageDirectory(nil), pageDirectories...),
	}
	if err := writeIndentedJSONFile(files.ManifestJSON, report); err != nil {
		return err
	}
	if err := writeCompareManifestMarkdown(files.ManifestMarkdown, report); err != nil {
		return err
	}
	if err := writeCompareManifestReviewIndex(files.ReviewIndex, dir, report, files); err != nil {
		return err
	}
	return writeIndentedJSONFile(files.ReviewSummary, buildCompareManifestReviewSummary(report, files))
}

func writeCompareManifestReviewIndex(path string, rootDir string, report compareManifestReport, files compareManifestReviewFiles) error {
	var builder strings.Builder
	builder.WriteString("# Compare Review Index\n\n")
	if strings.TrimSpace(report.Manifest) != "" {
		builder.WriteString("Manifest: `")
		builder.WriteString(report.Manifest)
		builder.WriteString("`\n\n")
	}
	fmt.Fprintf(&builder, "Summary: %d pages, %d compared, %d failed, %d findings (critical %d, warning %d, info %d).\n\n",
		report.Summary.TotalPages,
		report.Summary.ComparedPages,
		report.Summary.FailedPages,
		report.Summary.TotalFindings,
		report.Summary.Critical,
		report.Summary.Warning,
		report.Summary.Info,
	)
	fmt.Fprintf(&builder, "- Manifest JSON: %s\n", compareReviewMarkdownLink(rootDir, "manifest.json", files.ManifestJSON))
	fmt.Fprintf(&builder, "- Manifest Markdown: %s\n", compareReviewMarkdownLink(rootDir, "manifest.md", files.ManifestMarkdown))
	fmt.Fprintf(&builder, "- Review Summary: %s\n\n", compareReviewMarkdownLink(rootDir, "review-summary.json", files.ReviewSummary))

	builder.WriteString("| Priority | Page | Findings | Packet | Screenshots | Status |\n")
	builder.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for i, page := range report.Pages {
		directory := compareManifestReviewPageDirectory{Name: page.Name}
		if i < len(files.PageDirectories) {
			directory = files.PageDirectories[i]
		}
		priority := directory.Priority
		if priority == "" && page.Error != "" {
			priority = "error"
		}
		if priority == "" && page.Report != nil {
			priority = compareManifestReviewPriority(page.Report.Summary)
		}
		if priority == "" {
			priority = "unknown"
		}
		findings := compareManifestReviewFindingsLabel(directory, page)
		packet := compareManifestReviewPacketLinks(rootDir, directory)
		screenshots := compareManifestReviewScreenshotLinks(rootDir, directory)
		status := "ok"
		if page.Error != "" {
			status = page.Error
		} else if directory.Error != "" {
			status = directory.Error
		}
		fmt.Fprintf(&builder, "| %s | %s | %s | %s | %s | %s |\n",
			compareReviewMarkdownCell(priority),
			compareReviewMarkdownCell(firstNonEmpty(directory.Name, page.Name)),
			compareReviewMarkdownCell(findings),
			compareReviewMarkdownCell(packet),
			compareReviewMarkdownCell(screenshots),
			compareReviewMarkdownCell(status),
		)
	}
	builder.WriteString("\n")
	return os.WriteFile(path, []byte(builder.String()), 0o644)
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

func compareManifestReviewPriority(summary compareSummary) string {
	if summary.Critical > 0 {
		return "critical"
	}
	if summary.Warning > 0 {
		return "warning"
	}
	if summary.Info > 0 || summary.TotalFindings > 0 {
		return "info"
	}
	return "clean"
}

func compareManifestReviewFindingsLabel(directory compareManifestReviewPageDirectory, page compareManifestPageReport) string {
	if directory.Error != "" || page.Error != "" {
		return "-"
	}
	total := directory.TotalFindings
	critical := directory.CriticalFindings
	warning := directory.WarningFindings
	info := directory.InfoFindings
	if page.Report != nil {
		total = page.Report.Summary.TotalFindings
		critical = page.Report.Summary.Critical
		warning = page.Report.Summary.Warning
		info = page.Report.Summary.Info
	}
	return fmt.Sprintf("%d (C%d/W%d/I%d)", total, critical, warning, info)
}

func compareManifestReviewPacketLinks(rootDir string, directory compareManifestReviewPageDirectory) string {
	if strings.TrimSpace(directory.Directory) == "" || directory.Error != "" {
		return "-"
	}
	links := []string{
		compareReviewMarkdownLink(rootDir, "md", filepath.Join(directory.Directory, compareReviewFileMarkdown)),
		compareReviewMarkdownLink(rootDir, "json", filepath.Join(directory.Directory, compareReviewFileJSON)),
		compareReviewMarkdownLink(rootDir, "pairs", filepath.Join(directory.Directory, compareReviewFilePairDecisionsTemplate)),
		compareReviewMarkdownLink(rootDir, "findings", filepath.Join(directory.Directory, compareReviewFileFindingDecisionsTemplate)),
	}
	return strings.Join(links, " / ")
}

func compareManifestReviewScreenshotLinks(rootDir string, directory compareManifestReviewPageDirectory) string {
	if strings.TrimSpace(directory.Directory) == "" || directory.Error != "" {
		return "-"
	}
	links := make([]string, 0, 2)
	if directory.OldScreenshot != "" {
		links = append(links, compareReviewMarkdownLink(rootDir, "old", directory.OldScreenshot))
	} else {
		links = append(links, "old missing")
	}
	if directory.NewScreenshot != "" {
		links = append(links, compareReviewMarkdownLink(rootDir, "new", directory.NewScreenshot))
	} else {
		links = append(links, "new missing")
	}
	return strings.Join(links, " / ")
}

func compareReviewMarkdownLink(rootDir string, label string, target string) string {
	if strings.TrimSpace(target) == "" {
		return ""
	}
	return "[" + label + "](" + compareReviewMarkdownLinkTarget(rootDir, target) + ")"
}

func compareReviewMarkdownLinkTarget(rootDir string, target string) string {
	rel, err := filepath.Rel(rootDir, target)
	if err != nil {
		rel = target
	}
	return strings.ReplaceAll(filepath.ToSlash(rel), " ", "%20")
}

func compareReviewMarkdownCell(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
