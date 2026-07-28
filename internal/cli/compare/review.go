package comparecmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/rpc"
)

const (
	compareReviewFileReview                   = "REVIEW.md"
	compareReviewFileJSON                     = "compare.json"
	compareReviewFileMarkdown                 = "compare.md"
	compareReviewFilePairDecisionsTemplate    = "pair-decisions.todo.jsonl"
	compareReviewFileFindingDecisionsTemplate = "finding-decisions.todo.jsonl"
	compareReviewFileClusterDecisionsTemplate = "cluster-decisions.todo.jsonl"
	compareReviewFileOldScreenshot            = "old.png"
	compareReviewFileNewScreenshot            = "new.png"
	compareReviewFileFindingsDir              = "findings"
	compareReviewFileSummary                  = "review-summary.json"
	compareReviewFileManifestJSON             = "manifest.json"
	compareReviewFileManifestMarkdown         = "manifest.md"
	compareReviewFileIndex                    = "review-index.md"
	compareReviewFileIndexHTML                = "review-index.html"
	compareReviewMaxFindingClusters           = 5
	compareReviewMaxClusterFindingIDs         = 20
)

var compareReviewScreenshotTimeout = 30 * time.Second

type compareReviewScreenshots struct {
	Old      []byte
	New      []byte
	Warnings []string
}

type compareReviewPacketOptions struct {
	DecisionAudit *compareDecisionAuditReport
}

func writeCompareReviewPacket(dir string, report compareReport, screenshots compareReviewScreenshots, options compareReviewPacketOptions) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := compareReviewFiles{
		ReviewMarkdown:           filepath.Join(dir, compareReviewFileReview),
		CompareJSON:              filepath.Join(dir, compareReviewFileJSON),
		CompareMarkdown:          filepath.Join(dir, compareReviewFileMarkdown),
		PairDecisionsTemplate:    filepath.Join(dir, compareReviewFilePairDecisionsTemplate),
		FindingDecisionsTemplate: filepath.Join(dir, compareReviewFileFindingDecisionsTemplate),
		ClusterDecisionsTemplate: filepath.Join(dir, compareReviewFileClusterDecisionsTemplate),
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
	cropWarnings, err := writeCompareReviewFindingScreenshots(dir, report, screenshots, &files)
	if err != nil {
		return err
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
	if err := writeCompareFindingClusterDecisionsTemplate(files.ClusterDecisionsTemplate, compareFindingClusters(report.Findings, "")); err != nil {
		return err
	}
	summary := buildCompareReviewSummary(report, files, screenshots.Warnings, cropWarnings, options.DecisionAudit)
	if err := writeCompareReviewGuide(files.ReviewMarkdown, summary); err != nil {
		return err
	}
	return writeIndentedJSONFile(files.ReviewSummary, summary)
}

func buildCompareReviewSummary(report compareReport, files compareReviewFiles, screenshotWarnings []string, cropWarnings []string, decisionAudit *compareDecisionAuditReport) compareReviewSummary {
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
		FindingClusters:         compareFindingClusters(report.Findings, ""),
		ScreenshotWarnings:      append([]string(nil), screenshotWarnings...),
		CropWarnings:            append([]string(nil), cropWarnings...),
	}
	if decisionAudit != nil {
		auditSummary := decisionAudit.Summary
		summary.DecisionAudit = &auditSummary
		summary.DecisionAuditExamples = compareDecisionAuditExampleEntries(decisionAudit, 5)
	}
	if report.Scope != nil {
		summary.Scope = compareScopeLabel(report.Scope)
	}
	if report.MatchingDebug != nil {
		summary.AmbiguousCandidates = len(report.MatchingDebug.AmbiguousCandidates)
		summary.UnmatchedOld = len(report.MatchingDebug.UnmatchedOld)
		summary.UnmatchedNew = len(report.MatchingDebug.UnmatchedNew)
		summary.PairDecisionTemplate = compareDecisionTemplateCountsForDebug(report.MatchingDebug)
	}
	materializedPairDecisions := filepath.Join(filepath.Dir(files.PairDecisionsTemplate), "pair-decisions.materialized.jsonl")
	normalizedPairDecisions := filepath.Join(filepath.Dir(files.PairDecisionsTemplate), "pair-decisions.normalized.jsonl")
	normalizedClusterDecisions := filepath.Join(filepath.Dir(files.ClusterDecisionsTemplate), "cluster-decisions.normalized.jsonl")
	summary.NextCommands = []string{
		"nxctl compare validate-decisions --decisions-file " + files.PairDecisionsTemplate + " --compare-json " + files.CompareJSON,
		"nxctl compare materialize-decisions --decisions-file " + files.PairDecisionsTemplate + " --compare-json " + files.CompareJSON + " --output " + materializedPairDecisions,
		"nxctl compare normalize-decisions --decisions-file " + materializedPairDecisions + " --compare-json " + files.CompareJSON + " --output " + normalizedPairDecisions,
		"nxctl compare validate-decisions --decisions-file " + files.FindingDecisionsTemplate + " --compare-json " + files.CompareJSON,
		"nxctl compare validate-decisions --decisions-file " + files.ClusterDecisionsTemplate + " --compare-json " + files.CompareJSON,
		"nxctl compare normalize-decisions --decisions-file " + files.ClusterDecisionsTemplate + " --compare-json " + files.CompareJSON + " --review-summary " + files.ReviewSummary + " --output " + normalizedClusterDecisions,
	}
	return summary
}

func writeCompareReviewGuide(path string, summary compareReviewSummary) error {
	return os.WriteFile(path, []byte(renderCompareReviewGuide(summary)), 0o644)
}

func renderCompareReviewGuide(summary compareReviewSummary) string {
	var builder strings.Builder
	builder.WriteString("# Nexus Compare Review\n\n")
	builder.WriteString("This directory is the working packet for one compare review.\n\n")

	builder.WriteString("## Summary\n\n")
	fmt.Fprintf(&builder, "- Old: %s\n", firstNonEmpty(summary.Old, "(not recorded)"))
	fmt.Fprintf(&builder, "- New: %s\n", firstNonEmpty(summary.New, "(not recorded)"))
	if strings.TrimSpace(summary.Scope) != "" {
		fmt.Fprintf(&builder, "- Scope: %s\n", summary.Scope)
	}
	fmt.Fprintf(&builder, "- Findings: %d total, %d critical, %d warning, %d info\n", summary.TotalFindings, summary.CriticalFindings, summary.WarningFindings, summary.InfoFindings)
	if summary.MatchedNodes > 0 {
		fmt.Fprintf(&builder, "- Matched nodes: %d\n", summary.MatchedNodes)
	}
	if summary.AmbiguousCandidates > 0 || summary.UnmatchedOld > 0 || summary.UnmatchedNew > 0 {
		fmt.Fprintf(&builder, "- Matching review: %d ambiguous, %d unmatched old, %d unmatched new\n", summary.AmbiguousCandidates, summary.UnmatchedOld, summary.UnmatchedNew)
	}
	if summary.PairDecisionTemplate != nil {
		counts := summary.PairDecisionTemplate
		fmt.Fprintf(&builder, "- Pair decision template: %d ambiguous, %d unmatched old, %d unmatched new", counts.Ambiguous, counts.UnmatchedOld, counts.UnmatchedNew)
		extras := []string{}
		if counts.TruncatedOld > 0 {
			extras = append(extras, fmt.Sprintf("%d old truncated", counts.TruncatedOld))
		}
		if counts.TruncatedNew > 0 {
			extras = append(extras, fmt.Sprintf("%d new truncated", counts.TruncatedNew))
		}
		if counts.SkippedDuplicateOld > 0 {
			extras = append(extras, fmt.Sprintf("%d old duplicates skipped", counts.SkippedDuplicateOld))
		}
		if counts.SkippedDuplicateNew > 0 {
			extras = append(extras, fmt.Sprintf("%d new duplicates skipped", counts.SkippedDuplicateNew))
		}
		if len(extras) > 0 {
			fmt.Fprintf(&builder, " (%s)", strings.Join(extras, ", "))
		}
		builder.WriteString("\n")
	}
	if len(summary.FindingClusters) > 0 {
		fmt.Fprintf(&builder, "- Repeated finding clusters: %d\n", len(summary.FindingClusters))
	}
	if summary.DecisionAudit != nil {
		fmt.Fprintf(&builder, "- Decision audit: %s\n", compareDecisionAuditSummaryLabel(summary.DecisionAudit))
	}
	if len(summary.DecisionAuditExamples) > 0 {
		fmt.Fprintf(&builder, "- Decision audit examples: %s\n", compareDecisionAuditExampleLabels(summary.DecisionAuditExamples, 3))
	}
	if len(summary.ScreenshotWarnings) > 0 || len(summary.CropWarnings) > 0 {
		fmt.Fprintf(&builder, "- Capture warnings: %d screenshots, %d crops\n", len(summary.ScreenshotWarnings), len(summary.CropWarnings))
	}

	builder.WriteString("\n## Start Here\n\n")
	writeCompareReviewGuideFile(&builder, summary.Files.ReviewMarkdown, "this guide")
	writeCompareReviewGuideFile(&builder, summary.Files.ReviewSummary, "machine-readable packet metadata and next commands")
	writeCompareReviewGuideFile(&builder, summary.Files.CompareMarkdown, "human-readable compare findings")
	writeCompareReviewGuideFile(&builder, summary.Files.CompareJSON, "full compare data for AI review")
	writeCompareReviewGuideFile(&builder, summary.Files.PairDecisionsTemplate, "editable pair, unmatched, and subtree decision stubs")
	writeCompareReviewGuideFile(&builder, summary.Files.FindingDecisionsTemplate, "editable finding decision stubs")
	writeCompareReviewGuideFile(&builder, summary.Files.ClusterDecisionsTemplate, "editable repeated finding cluster decision stubs")
	writeCompareReviewGuideFile(&builder, summary.Files.OldScreenshot, "old full-page screenshot")
	writeCompareReviewGuideFile(&builder, summary.Files.NewScreenshot, "new full-page screenshot")
	writeCompareReviewGuideFile(&builder, summary.Files.FindingScreenshotsDir, "cropped finding screenshots")

	if len(summary.NextCommands) > 0 {
		builder.WriteString("\n## Commands\n\n")
		builder.WriteString("```sh\n")
		for _, command := range summary.NextCommands {
			fmt.Fprintf(&builder, "%s\n", command)
		}
		builder.WriteString("```\n")
	}

	builder.WriteString("\n## Decision Guidance\n\n")
	builder.WriteString("- Ambiguous candidates: keep `kind:\"pair\"`, set `new` or `new_locator`, and use `confidence:\"high\"` only when the correspondence is clear.\n")
	builder.WriteString("- Unmatched old nodes: pair them with a new node, convert them to `accepted_removed`, or leave them unknown when unsure.\n")
	builder.WriteString("- Unmatched new nodes: keep `accepted_added` for intentional additions, convert them to `pair` when they correspond to old nodes, or leave them unknown when unsure.\n")
	builder.WriteString("- Nested regions: use `subtree_pair` with `ordered_children`, `ordered_descendants`, or `opaque_subtree` only when a reviewed root correspondence is clear.\n")
	builder.WriteString("- Finding decisions: use `accepted_finding` for approved differences and `regression_finding` for real regressions.\n")
	builder.WriteString("- Finding clusters: use `accepted_finding_cluster` or `regression_finding_cluster` when repeated findings share one decision.\n")
	return builder.String()
}

func writeCompareReviewGuideFile(builder *strings.Builder, path string, description string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	fmt.Fprintf(builder, "- `%s`: %s\n", compareReviewGuideDisplayPath(path), description)
}

func compareReviewGuideDisplayPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}

func writeCompareReviewFindingScreenshots(dir string, report compareReport, screenshots compareReviewScreenshots, files *compareReviewFiles) ([]string, error) {
	warnings := []string{}
	findingDir := filepath.Join(dir, compareReviewFileFindingsDir)
	wrote := false
	for _, finding := range report.Findings {
		if !compareReviewFindingCropsIncluded(finding) {
			continue
		}
		for _, crop := range compareReviewFindingCrops(report, finding, screenshots) {
			if crop.Rect == nil || len(crop.Screenshot) == 0 {
				continue
			}
			data, err := cropCompareReviewScreenshot(crop.Screenshot, *crop.Rect)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s %s crop: %v", finding.FindingID, crop.Side, err))
				continue
			}
			if !wrote {
				if err := os.MkdirAll(findingDir, 0o755); err != nil {
					return warnings, err
				}
				files.FindingScreenshotsDir = findingDir
				wrote = true
			}
			path := filepath.Join(findingDir, compareReviewFindingCropFileName(finding.FindingID, crop.Side))
			if err := os.WriteFile(path, data, 0o644); err != nil {
				return warnings, err
			}
		}
	}
	return warnings, nil
}

type compareReviewFindingCrop struct {
	Side       string
	Screenshot []byte
	Rect       *api.Rect
}

func compareReviewFindingCrops(report compareReport, finding compareFinding, screenshots compareReviewScreenshots) []compareReviewFindingCrop {
	crops := make([]compareReviewFindingCrop, 0, 2)
	if finding.Kind != "new_node" {
		crops = append(crops, compareReviewFindingCrop{
			Side:       "old",
			Screenshot: screenshots.Old,
			Rect:       compareReviewFindingCropRect(report.Old.Nodes, finding),
		})
	}
	if finding.Kind != "missing_node" {
		crops = append(crops, compareReviewFindingCrop{
			Side:       "new",
			Screenshot: screenshots.New,
			Rect:       compareReviewFindingCropRect(report.New.Nodes, finding),
		})
	}
	return crops
}

func compareReviewFindingCropsIncluded(finding compareFinding) bool {
	if !compareManifestReviewHTMLFindingIncluded(finding) {
		return false
	}
	return strings.TrimSpace(finding.FindingID) != ""
}

func compareReviewFindingCropRect(nodes []compareSnapshotNode, finding compareFinding) *api.Rect {
	bestScore := 0
	var best *api.Rect
	for i := range nodes {
		node := nodes[i]
		rect := node.CropBounds
		if rect == nil {
			rect = node.MatchBounds
		}
		if rect == nil || !compareRectValid(*rect) {
			continue
		}
		score := compareReviewFindingNodeScore(node, finding)
		if score > bestScore {
			bestScore = score
			best = rect
		}
	}
	return best
}

func compareReviewFindingNodeScore(node compareSnapshotNode, finding compareFinding) int {
	score := 0
	if finding.Fingerprint != "" && node.Fingerprint == finding.Fingerprint {
		score += 6
	}
	if finding.StructureKey != "" && node.StructureKey == finding.StructureKey {
		score += 4
	}
	if finding.SubtreeSignature != "" && node.SubtreeSignature == finding.SubtreeSignature {
		score += 3
	}
	if finding.Locator != "" && compareNodeLocator(node) == finding.Locator {
		score += 6
	}
	if finding.Role != "" && node.Role == finding.Role {
		score += 2
	}
	if finding.Label != "" && node.Label == finding.Label {
		score += 3
	}
	return score
}

func cropCompareReviewScreenshot(data []byte, rect api.Rect) ([]byte, error) {
	source, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode screenshot: %w", err)
	}
	bounds := image.Rect(rect.X, rect.Y, rect.X+rect.W, rect.Y+rect.H).Intersect(source.Bounds())
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return nil, fmt.Errorf("bbox is outside the captured screenshot")
	}
	canvas := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(canvas, canvas.Bounds(), source, bounds.Min, draw.Src)
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		return nil, fmt.Errorf("encode crop: %w", err)
	}
	return output.Bytes(), nil
}

func compareReviewFindingCropFileName(findingID string, side string) string {
	name := compareReviewSafeFileToken(findingID)
	if name == "" {
		name = "finding"
	}
	return name + "-" + compareReviewSafeFileToken(side) + ".png"
}

func compareReviewSafeFileToken(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		ok := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-'
		if ok {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), ".-")
}

func captureCompareReviewScreenshots(ctx context.Context, client *rpc.Client, oldSessionID string, newSessionID string) compareReviewScreenshots {
	screenshots := compareReviewScreenshots{}
	oldScreenshot, oldWarning, err := captureCompareReviewScreenshot(ctx, client, oldSessionID)
	if err != nil {
		screenshots.Warnings = append(screenshots.Warnings, "old screenshot: "+err.Error())
	} else {
		screenshots.Old = oldScreenshot
		if oldWarning != "" {
			screenshots.Warnings = append(screenshots.Warnings, "old screenshot: "+oldWarning)
		}
	}
	newScreenshot, newWarning, err := captureCompareReviewScreenshot(ctx, client, newSessionID)
	if err != nil {
		screenshots.Warnings = append(screenshots.Warnings, "new screenshot: "+err.Error())
	} else {
		screenshots.New = newScreenshot
		if newWarning != "" {
			screenshots.Warnings = append(screenshots.Warnings, "new screenshot: "+newWarning)
		}
	}
	return screenshots
}

func captureCompareReviewScreenshot(ctx context.Context, client *rpc.Client, sessionID string) ([]byte, string, error) {
	captureCtx, cancel := context.WithTimeout(ctx, compareReviewScreenshotTimeout)
	defer cancel()

	res, err := client.ObserveSession(captureCtx, api.ObserveSessionRequest{
		SessionID: sessionID,
		Options: api.ObserveOptions{
			WithScreenshot: true,
			FullScreenshot: true,
			TimeoutMS:      int(compareReviewScreenshotTimeout / time.Millisecond),
		},
	})
	if err != nil {
		return nil, "", err
	}
	screenshot, err := res.Observation.ScreenshotBytes()
	if err != nil {
		return nil, "", err
	}
	if len(screenshot) == 0 {
		return nil, "", fmt.Errorf("empty screenshot")
	}
	return screenshot, strings.TrimSpace(res.Observation.Meta["screenshot_readiness_warning"]), nil
}

func writeCompareManifestReviewPacket(dir string, report compareManifestReport, pageDirectories []compareManifestReviewPageDirectory) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := compareManifestReviewFiles{
		ReviewMarkdown:           filepath.Join(dir, compareReviewFileReview),
		ManifestJSON:             filepath.Join(dir, compareReviewFileManifestJSON),
		ManifestMarkdown:         filepath.Join(dir, compareReviewFileManifestMarkdown),
		ReviewIndex:              filepath.Join(dir, compareReviewFileIndex),
		ReviewIndexHTML:          filepath.Join(dir, compareReviewFileIndexHTML),
		ClusterDecisionsTemplate: filepath.Join(dir, compareReviewFileClusterDecisionsTemplate),
		ReviewSummary:            filepath.Join(dir, compareReviewFileSummary),
		PageDirectories:          append([]compareManifestReviewPageDirectory(nil), pageDirectories...),
	}
	if err := writeIndentedJSONFile(files.ManifestJSON, report); err != nil {
		return err
	}
	if err := writeCompareManifestMarkdown(files.ManifestMarkdown, report); err != nil {
		return err
	}
	if err := writeCompareFindingClusterDecisionsTemplate(files.ClusterDecisionsTemplate, compareManifestFindingClusters(report)); err != nil {
		return err
	}
	if err := writeCompareManifestReviewIndex(files.ReviewIndex, dir, report, files); err != nil {
		return err
	}
	if err := writeCompareManifestReviewHTMLIndex(files.ReviewIndexHTML, dir, report, files); err != nil {
		return err
	}
	summary := buildCompareManifestReviewSummary(report, files)
	if err := writeCompareManifestReviewGuide(files.ReviewMarkdown, summary); err != nil {
		return err
	}
	return writeIndentedJSONFile(files.ReviewSummary, summary)
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
	fmt.Fprintf(&builder, "- Review Guide: %s\n", compareReviewMarkdownLink(rootDir, "REVIEW.md", files.ReviewMarkdown))
	fmt.Fprintf(&builder, "- Manifest JSON: %s\n", compareReviewMarkdownLink(rootDir, "manifest.json", files.ManifestJSON))
	fmt.Fprintf(&builder, "- Manifest Markdown: %s\n", compareReviewMarkdownLink(rootDir, "manifest.md", files.ManifestMarkdown))
	fmt.Fprintf(&builder, "- Cluster Decisions Template: %s\n", compareReviewMarkdownLink(rootDir, "cluster-decisions.todo.jsonl", files.ClusterDecisionsTemplate))
	fmt.Fprintf(&builder, "- Review Summary: %s\n\n", compareReviewMarkdownLink(rootDir, "review-summary.json", files.ReviewSummary))

	builder.WriteString("| Priority | Page | Findings | Pair decisions | Decision audit | Packet | Screenshots | Status |\n")
	builder.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- |\n")
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
		pairDecisions := compareManifestReviewPairDecisionTemplateLabel(directory.PairDecisionTemplate)
		decisionAudit := compareDecisionAuditSummaryLabel(directory.DecisionAudit)
		packet := compareManifestReviewPacketLinks(rootDir, directory)
		screenshots := compareManifestReviewScreenshotLinks(rootDir, directory)
		status := "ok"
		if page.Error != "" {
			status = page.Error
		} else if directory.Error != "" {
			status = directory.Error
		}
		fmt.Fprintf(&builder, "| %s | %s | %s | %s | %s | %s | %s | %s |\n",
			compareReviewMarkdownCell(priority),
			compareReviewMarkdownCell(firstNonEmpty(directory.Name, page.Name)),
			compareReviewMarkdownCell(findings),
			compareReviewMarkdownCell(pairDecisions),
			compareReviewMarkdownCell(decisionAudit),
			compareReviewMarkdownCell(packet),
			compareReviewMarkdownCell(screenshots),
			compareReviewMarkdownCell(status),
		)
	}
	builder.WriteString("\n")
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func writeCompareManifestReviewHTMLIndex(path string, rootDir string, report compareManifestReport, files compareManifestReviewFiles) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	tmpl, err := template.New("review-index").Parse(compareManifestReviewHTMLTemplate)
	if err != nil {
		return err
	}
	return tmpl.Execute(file, buildCompareManifestReviewHTMLData(rootDir, report, files))
}

func buildCompareManifestReviewSummary(report compareManifestReport, files compareManifestReviewFiles) compareManifestReviewSummary {
	return compareManifestReviewSummary{
		Manifest:             report.Manifest,
		TotalPages:           report.Summary.TotalPages,
		ComparedPages:        report.Summary.ComparedPages,
		FailedPages:          report.Summary.FailedPages,
		SamePages:            report.Summary.SamePages,
		DifferentPages:       report.Summary.DifferentPages,
		TotalFindings:        report.Summary.TotalFindings,
		CriticalFindings:     report.Summary.Critical,
		WarningFindings:      report.Summary.Warning,
		InfoFindings:         report.Summary.Info,
		PairDecisionTemplate: compareManifestReviewPairDecisionTemplateCounts(files.PageDirectories),
		DecisionAudit:        compareManifestReviewDecisionAuditSummary(files.PageDirectories),
		Files:                files,
		FindingClusters:      compareManifestFindingClusters(report),
	}
}

func writeCompareManifestReviewGuide(path string, summary compareManifestReviewSummary) error {
	return os.WriteFile(path, []byte(renderCompareManifestReviewGuide(summary)), 0o644)
}

func renderCompareManifestReviewGuide(summary compareManifestReviewSummary) string {
	var builder strings.Builder
	builder.WriteString("# Nexus Manifest Review\n\n")
	builder.WriteString("This directory is the entry point for a multi-page compare review.\n\n")

	builder.WriteString("## Summary\n\n")
	if strings.TrimSpace(summary.Manifest) != "" {
		fmt.Fprintf(&builder, "- Manifest: %s\n", summary.Manifest)
	}
	fmt.Fprintf(&builder, "- Pages: %d total, %d compared, %d failed\n", summary.TotalPages, summary.ComparedPages, summary.FailedPages)
	fmt.Fprintf(&builder, "- Page results: %d same, %d different\n", summary.SamePages, summary.DifferentPages)
	fmt.Fprintf(&builder, "- Findings: %d total, %d critical, %d warning, %d info\n", summary.TotalFindings, summary.CriticalFindings, summary.WarningFindings, summary.InfoFindings)
	if summary.PairDecisionTemplate != nil {
		counts := summary.PairDecisionTemplate
		fmt.Fprintf(&builder, "- Pair decision workload: %s\n", compareManifestReviewPairDecisionTemplateLabel(counts))
		if counts.TruncatedOld > 0 || counts.TruncatedNew > 0 || counts.SkippedDuplicateOld > 0 || counts.SkippedDuplicateNew > 0 {
			fmt.Fprintf(&builder, "- Pair decision filtering: %d old truncated, %d new truncated, %d old duplicates skipped, %d new duplicates skipped\n", counts.TruncatedOld, counts.TruncatedNew, counts.SkippedDuplicateOld, counts.SkippedDuplicateNew)
		}
	}
	if len(summary.FindingClusters) > 0 {
		fmt.Fprintf(&builder, "- Repeated finding clusters: %d\n", len(summary.FindingClusters))
	}
	if summary.DecisionAudit != nil {
		fmt.Fprintf(&builder, "- Decision audit: %s\n", compareDecisionAuditSummaryLabel(summary.DecisionAudit))
	}

	builder.WriteString("\n## Start Here\n\n")
	writeCompareReviewGuideFile(&builder, summary.Files.ReviewMarkdown, "this guide")
	writeCompareReviewGuideFile(&builder, summary.Files.ReviewIndexHTML, "static visual overview for the manifest")
	writeCompareReviewGuideFile(&builder, summary.Files.ReviewIndex, "markdown page priority index")
	writeCompareReviewGuideFile(&builder, summary.Files.ReviewSummary, "machine-readable manifest review metadata")
	writeCompareReviewGuideFile(&builder, summary.Files.ManifestMarkdown, "human-readable manifest compare report")
	writeCompareReviewGuideFile(&builder, summary.Files.ManifestJSON, "full manifest compare data")
	writeCompareReviewGuideFile(&builder, summary.Files.ClusterDecisionsTemplate, "editable manifest-level repeated finding cluster decision stubs")

	if len(summary.FindingClusters) > 0 {
		builder.WriteString("\n## Commands\n\n")
		builder.WriteString("```sh\n")
		fmt.Fprintf(&builder, "nxctl compare validate-decisions --decisions-file %s --review-summary %s\n", compareReviewGuideDisplayPath(summary.Files.ClusterDecisionsTemplate), compareReviewGuideDisplayPath(summary.Files.ReviewSummary))
		fmt.Fprintf(&builder, "nxctl compare normalize-decisions --decisions-file %s --review-summary %s --output cluster-decisions.normalized.jsonl\n", compareReviewGuideDisplayPath(summary.Files.ClusterDecisionsTemplate), compareReviewGuideDisplayPath(summary.Files.ReviewSummary))
		builder.WriteString("```\n")
	}

	builder.WriteString("\n## Priority\n\n")
	builder.WriteString("- Start with failed pages; they need rerun or environment investigation before findings are trustworthy.\n")
	builder.WriteString("- Review pages with critical findings next.\n")
	builder.WriteString("- Revisit pages with pending, stale, or conflicting decisions before trusting the current findings.\n")
	builder.WriteString("- Review pages with high pair decision workload when matching looks unstable, even if finding counts are lower.\n")
	builder.WriteString("- Check repeated finding clusters before reviewing every page one by one.\n")
	builder.WriteString("- Use warning-heavy pages to decide whether scope or decision templates need tightening.\n")
	writeCompareManifestReviewGuidePages(&builder, summary.Files.PageDirectories)

	if len(summary.FindingClusters) > 0 {
		builder.WriteString("\n## Finding Clusters\n\n")
		builder.WriteString("Review `cluster-decisions.todo.jsonl` when repeated findings share one review decision.\n")
	}

	builder.WriteString("\n## Page Packets\n\n")
	builder.WriteString("Open a page packet's `REVIEW.md` before editing its decision templates. Page packets contain screenshots, crops, compare output, and page-local next commands.\n")
	return builder.String()
}

func writeCompareManifestReviewGuidePages(builder *strings.Builder, directories []compareManifestReviewPageDirectory) {
	if len(directories) == 0 {
		return
	}
	ordered := append([]compareManifestReviewPageDirectory(nil), directories...)
	sort.SliceStable(ordered, func(i int, j int) bool {
		return compareManifestReviewPriorityRank(ordered[i].Priority) < compareManifestReviewPriorityRank(ordered[j].Priority)
	})
	builder.WriteString("\nHigh-priority page packets:\n\n")
	limit := min(len(ordered), 10)
	for _, directory := range ordered[:limit] {
		status := firstNonEmpty(directory.Error, directory.Priority, "unknown")
		findings := fmt.Sprintf("%d findings, %d critical, %d warning", directory.TotalFindings, directory.CriticalFindings, directory.WarningFindings)
		if directory.Error != "" {
			findings = "not compared"
		}
		pairDecisions := compareManifestReviewPairDecisionTemplateLabel(directory.PairDecisionTemplate)
		if pairDecisions != "" {
			pairDecisions = "; " + pairDecisions
		}
		audit := compareDecisionAuditSummaryLabel(directory.DecisionAudit)
		if audit != "" {
			audit = "; " + audit
		}
		fmt.Fprintf(builder, "- `%s`: %s; %s%s%s; open `%s`\n", firstNonEmpty(directory.Name, filepath.Base(directory.Directory)), status, findings, pairDecisions, audit, filepath.Join(compareReviewGuideDisplayPath(directory.Directory), compareReviewFileReview))
	}
	if len(ordered) > limit {
		fmt.Fprintf(builder, "- %d more page packets are listed in `%s`.\n", len(ordered)-limit, compareReviewGuideDisplayPath("review-index.md"))
	}
}

func compareManifestReviewPairDecisionTemplateCounts(directories []compareManifestReviewPageDirectory) *compareDecisionTemplateCounts {
	total := compareDecisionTemplateCounts{}
	for _, directory := range directories {
		if directory.PairDecisionTemplate == nil {
			continue
		}
		counts := directory.PairDecisionTemplate
		total.Ambiguous += counts.Ambiguous
		total.UnmatchedOld += counts.UnmatchedOld
		total.UnmatchedNew += counts.UnmatchedNew
		total.TruncatedOld += counts.TruncatedOld
		total.TruncatedNew += counts.TruncatedNew
		total.SkippedDuplicateOld += counts.SkippedDuplicateOld
		total.SkippedDuplicateNew += counts.SkippedDuplicateNew
	}
	if total.Ambiguous == 0 && total.UnmatchedOld == 0 && total.UnmatchedNew == 0 && total.TruncatedOld == 0 && total.TruncatedNew == 0 && total.SkippedDuplicateOld == 0 && total.SkippedDuplicateNew == 0 {
		return nil
	}
	return &total
}

func compareManifestReviewDecisionAuditSummary(directories []compareManifestReviewPageDirectory) *compareDecisionAuditSummary {
	total := compareDecisionAuditSummary{}
	used := false
	for _, directory := range directories {
		if directory.DecisionAudit == nil {
			continue
		}
		audit := directory.DecisionAudit
		used = true
		total.TotalDecisions += audit.TotalDecisions
		total.Applied += audit.Applied
		total.Pending += audit.Pending
		total.Stale += audit.Stale
		total.Conflicts += audit.Conflicts
		total.Errors += audit.Errors
		total.Warnings += audit.Warnings
		total.CompareJSONUsed = total.CompareJSONUsed || audit.CompareJSONUsed
	}
	if !used {
		return nil
	}
	return &total
}

func compareManifestReviewPairDecisionTemplateLabel(counts *compareDecisionTemplateCounts) string {
	if counts == nil {
		return ""
	}
	total := counts.Ambiguous + counts.UnmatchedOld + counts.UnmatchedNew
	values := []string{}
	if total > 0 {
		values = append(values, fmt.Sprintf("%d total", total))
	}
	if counts.Ambiguous > 0 {
		values = append(values, fmt.Sprintf("%d ambiguous", counts.Ambiguous))
	}
	if counts.UnmatchedOld > 0 {
		values = append(values, fmt.Sprintf("%d old", counts.UnmatchedOld))
	}
	if counts.UnmatchedNew > 0 {
		values = append(values, fmt.Sprintf("%d new", counts.UnmatchedNew))
	}
	if counts.TruncatedOld > 0 || counts.TruncatedNew > 0 {
		values = append(values, fmt.Sprintf("%d truncated", counts.TruncatedOld+counts.TruncatedNew))
	}
	if counts.SkippedDuplicateOld > 0 || counts.SkippedDuplicateNew > 0 {
		values = append(values, fmt.Sprintf("%d duplicates skipped", counts.SkippedDuplicateOld+counts.SkippedDuplicateNew))
	}
	return strings.Join(values, ", ")
}

func compareDecisionAuditSummaryLabel(summary *compareDecisionAuditSummary) string {
	if summary == nil {
		return ""
	}
	unresolved := summary.Pending + summary.Stale + summary.Conflicts
	values := []string{
		fmt.Sprintf("%d decisions", summary.TotalDecisions),
		fmt.Sprintf("%d applied", summary.Applied),
		fmt.Sprintf("%d unresolved", unresolved),
	}
	if summary.Pending > 0 {
		values = append(values, fmt.Sprintf("%d pending", summary.Pending))
	}
	if summary.Stale > 0 {
		values = append(values, fmt.Sprintf("%d stale", summary.Stale))
	}
	if summary.Conflicts > 0 {
		values = append(values, fmt.Sprintf("%d conflicts", summary.Conflicts))
	}
	if summary.Errors > 0 {
		values = append(values, fmt.Sprintf("%d errors", summary.Errors))
	}
	if summary.Warnings > 0 {
		values = append(values, fmt.Sprintf("%d warnings", summary.Warnings))
	}
	return strings.Join(values, ", ")
}

func compareDecisionAuditExampleEntries(report *compareDecisionAuditReport, limit int) []compareDecisionAuditEntry {
	if report == nil || limit <= 0 {
		return nil
	}
	examples := make([]compareDecisionAuditEntry, 0, min(len(report.Entries), limit))
	for _, entry := range report.Entries {
		if entry.Status == "applied" {
			continue
		}
		examples = append(examples, entry)
		if len(examples) >= limit {
			return examples
		}
	}
	return examples
}

func compareDecisionAuditExampleLabels(entries []compareDecisionAuditEntry, limit int) string {
	if limit <= 0 || len(entries) == 0 {
		return ""
	}
	values := make([]string, 0, min(len(entries), limit))
	for index, entry := range entries {
		if index >= limit {
			break
		}
		label := fmt.Sprintf("line %d %s", entry.Line, entry.Status)
		if entry.Field != "" {
			label += " " + entry.Field
		}
		if entry.Reason != "" {
			label += ": " + entry.Reason
		}
		values = append(values, label)
	}
	if len(entries) > limit {
		values = append(values, fmt.Sprintf("%d more", len(entries)-limit))
	}
	return strings.Join(values, "; ")
}

type compareManifestReviewHTMLData struct {
	Manifest                 string
	TotalPages               int
	ComparedPages            int
	FailedPages              int
	SamePages                int
	DifferentPages           int
	TotalFindings            int
	CriticalFindings         int
	WarningFindings          int
	InfoFindings             int
	ManifestJSON             string
	ManifestMarkdown         string
	ClusterDecisionsTemplate string
	ReviewSummary            string
	ReviewIndex              string
	FindingClusters          []compareManifestReviewHTMLCluster
	FindingClustersMore      int
	Pages                    []compareManifestReviewHTMLPage
}

type compareManifestReviewHTMLPage struct {
	Name                     string
	Priority                 string
	Findings                 string
	Status                   string
	FindingClusters          []compareManifestReviewHTMLCluster
	FindingClustersMore      int
	FindingPreview           []compareManifestReviewHTMLFinding
	FindingPreviewMore       int
	CompareMarkdown          string
	CompareJSON              string
	PairDecisionsTemplate    string
	FindingDecisionsTemplate string
	ClusterDecisionsTemplate string
	OldScreenshot            string
	NewScreenshot            string
	OldScreenshotMissing     bool
	NewScreenshotMissing     bool
}

type compareManifestReviewHTMLFinding struct {
	Severity           string
	Kind               string
	Impact             string
	Target             string
	Locator            string
	FindingID          string
	OldCrop            string
	NewCrop            string
	AcceptedDecision   string
	RegressionDecision string
}

type compareManifestReviewHTMLCluster struct {
	Count              int
	Severity           string
	Kind               string
	Impact             string
	Target             string
	Field              string
	Old                string
	New                string
	Pages              string
	ExampleFindingID   string
	OldCrop            string
	NewCrop            string
	AcceptedDecision   string
	RegressionDecision string
}

type compareManifestReviewFindingDecision struct {
	Kind       string `json:"kind"`
	FindingID  string `json:"finding_id"`
	ClusterKey string `json:"cluster_key,omitempty"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason"`
}

func buildCompareManifestReviewHTMLData(rootDir string, report compareManifestReport, files compareManifestReviewFiles) compareManifestReviewHTMLData {
	data := compareManifestReviewHTMLData{
		Manifest:                 report.Manifest,
		TotalPages:               report.Summary.TotalPages,
		ComparedPages:            report.Summary.ComparedPages,
		FailedPages:              report.Summary.FailedPages,
		SamePages:                report.Summary.SamePages,
		DifferentPages:           report.Summary.DifferentPages,
		TotalFindings:            report.Summary.TotalFindings,
		CriticalFindings:         report.Summary.Critical,
		WarningFindings:          report.Summary.Warning,
		InfoFindings:             report.Summary.Info,
		ManifestJSON:             compareReviewMarkdownLinkTarget(rootDir, files.ManifestJSON),
		ManifestMarkdown:         compareReviewMarkdownLinkTarget(rootDir, files.ManifestMarkdown),
		ClusterDecisionsTemplate: compareReviewMarkdownLinkTarget(rootDir, files.ClusterDecisionsTemplate),
		ReviewSummary:            compareReviewMarkdownLinkTarget(rootDir, files.ReviewSummary),
		ReviewIndex:              compareReviewMarkdownLinkTarget(rootDir, files.ReviewIndex),
		FindingClusters:          compareManifestReviewHTMLGlobalClusters(rootDir, report, files),
		FindingClustersMore:      compareManifestReviewHTMLGlobalClusterOverflow(report),
		Pages:                    make([]compareManifestReviewHTMLPage, 0, len(report.Pages)),
	}
	for i, page := range report.Pages {
		directory := compareManifestReviewPageDirectory{Name: page.Name}
		if i < len(files.PageDirectories) {
			directory = files.PageDirectories[i]
		}
		data.Pages = append(data.Pages, buildCompareManifestReviewHTMLPage(rootDir, page, directory))
	}
	sort.SliceStable(data.Pages, func(i int, j int) bool {
		return compareManifestReviewPriorityRank(data.Pages[i].Priority) < compareManifestReviewPriorityRank(data.Pages[j].Priority)
	})
	return data
}

func buildCompareManifestReviewHTMLPage(rootDir string, page compareManifestPageReport, directory compareManifestReviewPageDirectory) compareManifestReviewHTMLPage {
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
	status := "ok"
	if page.Error != "" {
		status = page.Error
	} else if directory.Error != "" {
		status = directory.Error
	}
	htmlPage := compareManifestReviewHTMLPage{
		Name:                firstNonEmpty(directory.Name, page.Name),
		Priority:            priority,
		Findings:            compareManifestReviewFindingsLabel(directory, page),
		Status:              status,
		FindingClusters:     compareManifestReviewHTMLFindingClusters(rootDir, directory, page.Report),
		FindingClustersMore: compareManifestReviewHTMLFindingClusterOverflow(page.Report),
		FindingPreview:      compareManifestReviewHTMLFindings(rootDir, directory, page.Report),
		FindingPreviewMore:  compareManifestReviewHTMLFindingOverflow(page.Report),
	}
	if strings.TrimSpace(directory.Directory) == "" || directory.Error != "" {
		return htmlPage
	}
	htmlPage.CompareMarkdown = compareReviewMarkdownLinkTarget(rootDir, filepath.Join(directory.Directory, compareReviewFileMarkdown))
	htmlPage.CompareJSON = compareReviewMarkdownLinkTarget(rootDir, filepath.Join(directory.Directory, compareReviewFileJSON))
	htmlPage.PairDecisionsTemplate = compareReviewMarkdownLinkTarget(rootDir, filepath.Join(directory.Directory, compareReviewFilePairDecisionsTemplate))
	htmlPage.FindingDecisionsTemplate = compareReviewMarkdownLinkTarget(rootDir, filepath.Join(directory.Directory, compareReviewFileFindingDecisionsTemplate))
	htmlPage.ClusterDecisionsTemplate = compareReviewMarkdownLinkTarget(rootDir, filepath.Join(directory.Directory, compareReviewFileClusterDecisionsTemplate))
	if directory.OldScreenshot != "" {
		htmlPage.OldScreenshot = compareReviewMarkdownLinkTarget(rootDir, directory.OldScreenshot)
	} else {
		htmlPage.OldScreenshotMissing = true
	}
	if directory.NewScreenshot != "" {
		htmlPage.NewScreenshot = compareReviewMarkdownLinkTarget(rootDir, directory.NewScreenshot)
	} else {
		htmlPage.NewScreenshotMissing = true
	}
	return htmlPage
}

func compareManifestReviewHTMLFindings(rootDir string, directory compareManifestReviewPageDirectory, report *compareReport) []compareManifestReviewHTMLFinding {
	if report == nil {
		return nil
	}
	previews := make([]compareManifestReviewHTMLFinding, 0, 5)
	for _, finding := range report.Findings {
		if !compareManifestReviewHTMLFindingIncluded(finding) {
			continue
		}
		if len(previews) >= 5 {
			break
		}
		previews = append(previews, compareManifestReviewHTMLFinding{
			Severity:           finding.Severity,
			Kind:               finding.Kind,
			Impact:             finding.Impact,
			Target:             compareManifestReviewFindingTarget(finding),
			Locator:            finding.Locator,
			FindingID:          finding.FindingID,
			OldCrop:            compareManifestReviewFindingCropLink(rootDir, directory.Directory, finding.FindingID, "old"),
			NewCrop:            compareManifestReviewFindingCropLink(rootDir, directory.Directory, finding.FindingID, "new"),
			AcceptedDecision:   compareManifestReviewFindingDecisionJSONL("accepted_finding", finding.FindingID),
			RegressionDecision: compareManifestReviewFindingDecisionJSONL("regression_finding", finding.FindingID),
		})
	}
	return previews
}

func compareManifestReviewHTMLFindingClusters(rootDir string, directory compareManifestReviewPageDirectory, report *compareReport) []compareManifestReviewHTMLCluster {
	if report == nil {
		return nil
	}
	return compareManifestReviewHTMLClusters(rootDir, directory, compareFindingClusters(report.Findings, ""))
}

func compareManifestReviewHTMLGlobalClusters(rootDir string, report compareManifestReport, files compareManifestReviewFiles) []compareManifestReviewHTMLCluster {
	clusters := compareManifestFindingClusters(report)
	if len(clusters) > compareReviewMaxFindingClusters {
		clusters = clusters[:compareReviewMaxFindingClusters]
	}
	htmlClusters := make([]compareManifestReviewHTMLCluster, 0, len(clusters))
	for _, cluster := range clusters {
		directory := compareManifestReviewClusterDirectory(report, files, cluster.ExampleFindingID)
		htmlClusters = append(htmlClusters, compareManifestReviewHTMLClusterFromSummary(rootDir, directory, cluster))
	}
	return htmlClusters
}

func compareManifestReviewHTMLClusters(rootDir string, directory compareManifestReviewPageDirectory, clusters []compareFindingCluster) []compareManifestReviewHTMLCluster {
	if len(clusters) > compareReviewMaxFindingClusters {
		clusters = clusters[:compareReviewMaxFindingClusters]
	}
	htmlClusters := make([]compareManifestReviewHTMLCluster, 0, len(clusters))
	for _, cluster := range clusters {
		htmlClusters = append(htmlClusters, compareManifestReviewHTMLClusterFromSummary(rootDir, directory, cluster))
	}
	return htmlClusters
}

func compareManifestReviewHTMLClusterFromSummary(rootDir string, directory compareManifestReviewPageDirectory, cluster compareFindingCluster) compareManifestReviewHTMLCluster {
	return compareManifestReviewHTMLCluster{
		Count:              cluster.Count,
		Severity:           cluster.Severity,
		Kind:               cluster.Kind,
		Impact:             cluster.Impact,
		Target:             compareFindingClusterTarget(cluster),
		Field:              cluster.Field,
		Old:                cluster.Old,
		New:                cluster.New,
		Pages:              strings.Join(cluster.Pages, ", "),
		ExampleFindingID:   cluster.ExampleFindingID,
		OldCrop:            compareManifestReviewFindingCropLink(rootDir, directory.Directory, cluster.ExampleFindingID, "old"),
		NewCrop:            compareManifestReviewFindingCropLink(rootDir, directory.Directory, cluster.ExampleFindingID, "new"),
		AcceptedDecision:   compareManifestReviewClusterDecisionJSONL("accepted_finding_cluster", cluster.Key),
		RegressionDecision: compareManifestReviewClusterDecisionJSONL("regression_finding_cluster", cluster.Key),
	}
}

func compareManifestReviewClusterDirectory(report compareManifestReport, files compareManifestReviewFiles, findingID string) compareManifestReviewPageDirectory {
	findingID = strings.TrimSpace(findingID)
	if findingID == "" {
		return compareManifestReviewPageDirectory{}
	}
	for i, page := range report.Pages {
		if page.Report == nil {
			continue
		}
		for _, finding := range page.Report.Findings {
			if finding.FindingID == findingID && i < len(files.PageDirectories) {
				return files.PageDirectories[i]
			}
		}
	}
	return compareManifestReviewPageDirectory{}
}

func compareManifestReviewHTMLFindingClusterOverflow(report *compareReport) int {
	if report == nil {
		return 0
	}
	return compareReviewOverflow(len(compareFindingClusters(report.Findings, "")), compareReviewMaxFindingClusters)
}

func compareManifestReviewHTMLGlobalClusterOverflow(report compareManifestReport) int {
	return compareReviewOverflow(len(compareManifestFindingClusters(report)), compareReviewMaxFindingClusters)
}

func compareReviewOverflow(total int, limit int) int {
	if total <= limit {
		return 0
	}
	return total - limit
}

func compareManifestReviewFindingCropLink(rootDir string, pageDir string, findingID string, side string) string {
	if strings.TrimSpace(pageDir) == "" || strings.TrimSpace(findingID) == "" {
		return ""
	}
	path := filepath.Join(pageDir, compareReviewFileFindingsDir, compareReviewFindingCropFileName(findingID, side))
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return compareReviewMarkdownLinkTarget(rootDir, path)
}

func compareManifestReviewHTMLFindingOverflow(report *compareReport) int {
	if report == nil {
		return 0
	}
	total := 0
	for _, finding := range report.Findings {
		if compareManifestReviewHTMLFindingIncluded(finding) {
			total++
		}
	}
	if total <= 5 {
		return 0
	}
	return total - 5
}

func compareManifestReviewHTMLFindingIncluded(finding compareFinding) bool {
	return finding.Severity == "critical" || finding.Severity == "warning"
}

func compareManifestReviewFindingTarget(finding compareFinding) string {
	parts := make([]string, 0, 3)
	if finding.Role != "" {
		parts = append(parts, finding.Role)
	}
	if finding.Label != "" {
		parts = append(parts, finding.Label)
	}
	if finding.Field != "" {
		parts = append(parts, finding.Field)
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	return firstNonEmpty(finding.Locator, finding.Fingerprint, finding.StructureKey, finding.SubtreeSignature)
}

type compareFindingClusterBuilder struct {
	cluster compareFindingCluster
	index   int
	pages   map[string]bool
}

func compareFindingClusters(findings []compareFinding, pageName string) []compareFindingCluster {
	builders := map[string]*compareFindingClusterBuilder{}
	order := []string{}
	for _, finding := range findings {
		compareAddFindingCluster(builders, &order, finding, pageName)
	}
	return compareSortedFindingClusters(builders, order)
}

func compareManifestFindingClusters(report compareManifestReport) []compareFindingCluster {
	builders := map[string]*compareFindingClusterBuilder{}
	order := []string{}
	for _, page := range report.Pages {
		if page.Report == nil {
			continue
		}
		for _, finding := range page.Report.Findings {
			compareAddFindingCluster(builders, &order, finding, page.Name)
		}
	}
	return compareSortedFindingClusters(builders, order)
}

func compareAddFindingCluster(builders map[string]*compareFindingClusterBuilder, order *[]string, finding compareFinding, pageName string) {
	if !compareManifestReviewHTMLFindingIncluded(finding) {
		return
	}
	key := compareFindingClusterKey(finding)
	builder := builders[key]
	if builder == nil {
		builder = &compareFindingClusterBuilder{
			cluster: compareFindingCluster{
				Key:              key,
				Severity:         finding.Severity,
				Kind:             finding.Kind,
				Impact:           finding.Impact,
				DecisionKind:     finding.DecisionKind,
				Field:            finding.Field,
				Role:             finding.Role,
				Label:            finding.Label,
				Old:              compareFindingClusterValuePart(finding.Old, finding.Kind),
				New:              compareFindingClusterValuePart(finding.New, finding.Kind),
				ExampleFindingID: strings.TrimSpace(finding.FindingID),
			},
			index: len(*order),
			pages: map[string]bool{},
		}
		builders[key] = builder
		*order = append(*order, key)
	}
	builder.cluster.Count++
	findingID := strings.TrimSpace(finding.FindingID)
	if findingID != "" {
		if builder.cluster.ExampleFindingID == "" {
			builder.cluster.ExampleFindingID = findingID
		}
		if len(builder.cluster.FindingIDs) < compareReviewMaxClusterFindingIDs {
			builder.cluster.FindingIDs = append(builder.cluster.FindingIDs, findingID)
		} else {
			builder.cluster.MoreFindingIDs++
		}
	}
	pageName = strings.TrimSpace(pageName)
	if pageName != "" && !builder.pages[pageName] {
		builder.pages[pageName] = true
		builder.cluster.Pages = append(builder.cluster.Pages, pageName)
	}
}

func compareSortedFindingClusters(builders map[string]*compareFindingClusterBuilder, order []string) []compareFindingCluster {
	clusters := make([]compareFindingCluster, 0, len(builders))
	indexes := map[string]int{}
	for _, key := range order {
		builder := builders[key]
		if builder == nil || builder.cluster.Count < 2 {
			continue
		}
		clusters = append(clusters, builder.cluster)
		indexes[builder.cluster.Key] = builder.index
	}
	sort.SliceStable(clusters, func(i int, j int) bool {
		if rankI, rankJ := compareFindingClusterSeverityRank(clusters[i].Severity), compareFindingClusterSeverityRank(clusters[j].Severity); rankI != rankJ {
			return rankI < rankJ
		}
		if clusters[i].Count != clusters[j].Count {
			return clusters[i].Count > clusters[j].Count
		}
		return indexes[clusters[i].Key] < indexes[clusters[j].Key]
	})
	return clusters
}

func compareFindingClusterSeverityRank(severity string) int {
	switch severity {
	case "critical":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}

func compareFindingClusterKey(finding compareFinding) string {
	parts := []string{
		strings.TrimSpace(finding.Severity),
		strings.TrimSpace(finding.Kind),
		strings.TrimSpace(finding.Impact),
		strings.TrimSpace(finding.DecisionKind),
		strings.TrimSpace(finding.Field),
		strings.TrimSpace(finding.Role),
		strings.TrimSpace(finding.Label),
		compareFindingClusterValuePart(finding.Old, finding.Kind),
		compareFindingClusterValuePart(finding.New, finding.Kind),
	}
	return strings.Join(parts, " | ")
}

func compareFindingClusterValuePart(value string, kind string) string {
	switch kind {
	case "missing_node", "new_node", "layout_changed":
		return ""
	default:
		return strings.TrimSpace(value)
	}
}

func compareFindingClusterTarget(cluster compareFindingCluster) string {
	parts := make([]string, 0, 3)
	if cluster.Role != "" {
		parts = append(parts, cluster.Role)
	}
	if cluster.Label != "" {
		parts = append(parts, cluster.Label)
	}
	if cluster.Field != "" {
		parts = append(parts, cluster.Field)
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	return strings.TrimSpace(firstNonEmpty(cluster.Impact, cluster.Kind))
}

func compareManifestReviewFindingDecisionJSONL(kind string, findingID string) string {
	findingID = strings.TrimSpace(findingID)
	if findingID == "" {
		return ""
	}
	bytes, err := json.Marshal(compareManifestReviewFindingDecision{
		Kind:       kind,
		FindingID:  findingID,
		Confidence: "high",
		Reason:     "",
	})
	if err != nil {
		return ""
	}
	return string(bytes)
}

func compareManifestReviewClusterDecisionJSONL(kind string, clusterKey string) string {
	if strings.TrimSpace(clusterKey) == "" {
		return ""
	}
	bytes, err := json.Marshal(compareManifestReviewFindingDecision{
		Kind:       kind,
		ClusterKey: clusterKey,
		Confidence: "high",
		Reason:     "",
	})
	if err != nil {
		return ""
	}
	return string(bytes)
}

func compareManifestReviewPriorityRank(priority string) int {
	switch priority {
	case "error":
		return 0
	case "critical":
		return 1
	case "warning":
		return 2
	case "info":
		return 3
	case "clean":
		return 4
	default:
		return 5
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
		compareReviewMarkdownLink(rootDir, "clusters", filepath.Join(directory.Directory, compareReviewFileClusterDecisionsTemplate)),
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

const compareManifestReviewHTMLTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Compare Review Index</title>
<style>
:root { color-scheme: light; --border:#d0d7de; --text:#24292f; --muted:#57606a; --bg:#f6f8fa; --panel:#ffffff; --critical:#cf222e; --warning:#9a6700; --info:#0969da; --clean:#1a7f37; --error:#8250df; }
* { box-sizing:border-box; }
body { margin:0; font:14px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; color:var(--text); background:var(--bg); }
a { color:var(--info); text-decoration:none; }
a:hover { text-decoration:underline; }
header.site { padding:24px 28px 18px; background:var(--panel); border-bottom:1px solid var(--border); }
h1 { margin:0 0 8px; font-size:24px; }
.manifest { margin:0 0 16px; color:var(--muted); }
.summary { display:grid; grid-template-columns:repeat(auto-fit,minmax(120px,1fr)); gap:8px; max-width:980px; }
.metric { padding:10px 12px; border:1px solid var(--border); border-radius:6px; background:#fff; }
.metric b { display:block; font-size:18px; }
.top-links { display:flex; flex-wrap:wrap; gap:10px; margin-top:14px; }
.top-links a, .packet-links a { display:inline-flex; align-items:center; min-height:28px; padding:4px 8px; border:1px solid var(--border); border-radius:6px; background:#fff; }
main { padding:18px 28px 32px; }
.page { margin:0 0 18px; border:1px solid var(--border); border-radius:8px; background:var(--panel); overflow:hidden; }
.page-header { display:flex; align-items:flex-start; justify-content:space-between; gap:16px; padding:14px 16px; border-bottom:1px solid var(--border); }
.page h2 { margin:0; font-size:18px; }
.meta { color:var(--muted); margin-top:4px; }
.badge { flex:0 0 auto; padding:4px 8px; border-radius:999px; color:#fff; font-weight:600; }
.priority-error .badge { background:var(--error); }
.priority-critical .badge { background:var(--critical); }
.priority-warning .badge { background:var(--warning); }
.priority-info .badge { background:var(--info); }
.priority-clean .badge { background:var(--clean); }
.priority-unknown .badge { background:#6e7781; }
.page-body { padding:14px 16px 16px; }
.packet-links { display:flex; flex-wrap:wrap; gap:8px; margin-bottom:14px; }
.findings-preview { margin:0 0 14px; border:1px solid var(--border); border-radius:6px; overflow:hidden; }
.findings-preview h3 { margin:0; padding:8px 10px; font-size:14px; border-bottom:1px solid var(--border); background:#f6f8fa; }
.clusters-preview { margin:0 0 14px; border:1px solid var(--border); border-radius:6px; overflow:hidden; background:#fff; }
.clusters-preview h3 { margin:0; padding:8px 10px; font-size:14px; border-bottom:1px solid var(--border); background:#f6f8fa; }
.cluster { padding:9px 10px; border-bottom:1px solid var(--border); }
.cluster:last-child { border-bottom:0; }
.cluster-head { display:flex; flex-wrap:wrap; gap:6px; align-items:center; margin-bottom:4px; }
.cluster-count { padding:2px 6px; border-radius:999px; color:#fff; background:#6e7781; font-size:12px; font-weight:600; }
.cluster-pages, .cluster-target, .cluster-values { color:var(--muted); }
.finding { padding:9px 10px; border-bottom:1px solid var(--border); }
.finding:last-child { border-bottom:0; }
.finding-head { display:flex; flex-wrap:wrap; gap:6px; align-items:center; margin-bottom:4px; }
.finding-severity { padding:2px 6px; border-radius:999px; color:#fff; font-size:12px; font-weight:600; }
.severity-critical { background:var(--critical); }
.severity-warning { background:var(--warning); }
.finding-kind { font-weight:600; }
.finding-impact, .finding-target, .finding-locator { color:var(--muted); }
code { font-family:ui-monospace,SFMono-Regular,Consolas,monospace; font-size:12px; word-break:break-all; }
.finding-crops { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:8px; margin-top:8px; }
.finding-crops figure { border-radius:4px; }
.finding-crops figcaption { padding:5px 7px; font-size:12px; }
.decision-stubs { margin-top:8px; }
.decision-stubs summary { cursor:pointer; color:var(--info); font-weight:600; }
.decision-stubs pre { margin:6px 0 0; padding:8px; overflow:auto; border:1px solid var(--border); border-radius:6px; background:#f6f8fa; }
.shots { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:14px; }
figure { margin:0; border:1px solid var(--border); border-radius:6px; overflow:hidden; background:#fff; }
figcaption { padding:8px 10px; border-top:1px solid var(--border); color:var(--muted); font-weight:600; }
img { display:block; width:100%; height:auto; background:#fff; }
.missing { min-height:180px; display:flex; align-items:center; justify-content:center; color:var(--muted); background:#f6f8fa; }
@media (max-width: 760px) { header.site, main { padding-left:16px; padding-right:16px; } .page-header { display:block; } .badge { display:inline-flex; margin-top:10px; } .shots { grid-template-columns:1fr; } }
</style>
</head>
<body>
<header class="site">
<h1>Compare Review Index</h1>
{{if .Manifest}}<p class="manifest">{{.Manifest}}</p>{{end}}
<div class="summary">
<div class="metric"><b>{{.TotalPages}}</b>pages</div>
<div class="metric"><b>{{.ComparedPages}}</b>compared</div>
<div class="metric"><b>{{.FailedPages}}</b>failed</div>
<div class="metric"><b>{{.TotalFindings}}</b>findings</div>
<div class="metric"><b>{{.CriticalFindings}}</b>critical</div>
<div class="metric"><b>{{.WarningFindings}}</b>warning</div>
<div class="metric"><b>{{.InfoFindings}}</b>info</div>
</div>
<nav class="top-links" aria-label="Review files">
<a href="{{.ManifestJSON}}">manifest.json</a>
<a href="{{.ManifestMarkdown}}">manifest.md</a>
{{if .ClusterDecisionsTemplate}}<a href="{{.ClusterDecisionsTemplate}}">cluster decisions</a>{{end}}
<a href="{{.ReviewSummary}}">review-summary.json</a>
<a href="{{.ReviewIndex}}">review-index.md</a>
</nav>
</header>
<main>
{{if .FindingClusters}}
<section class="clusters-preview">
<h3>Repeated finding clusters</h3>
{{range .FindingClusters}}
<div class="cluster">
<div class="cluster-head">
<span class="finding-severity severity-{{.Severity}}">{{.Severity}}</span>
<span class="cluster-count">{{.Count}} similar</span>
<span class="finding-kind">{{.Kind}}</span>
{{if .Impact}}<span class="finding-impact">{{.Impact}}</span>{{end}}
</div>
{{if .Target}}<div class="cluster-target">{{.Target}}</div>{{end}}
{{if .Pages}}<div class="cluster-pages">{{.Pages}}</div>{{end}}
{{if or .Old .New}}<div class="cluster-values"><code>{{.Old}}</code> -> <code>{{.New}}</code></div>{{end}}
{{if .ExampleFindingID}}<code>{{.ExampleFindingID}}</code>{{end}}
{{if or .OldCrop .NewCrop}}
<div class="finding-crops">
{{if .OldCrop}}<figure><a href="{{.OldCrop}}"><img src="{{.OldCrop}}" alt="{{.ExampleFindingID}} old cluster crop"></a><figcaption>Old crop</figcaption></figure>{{end}}
{{if .NewCrop}}<figure><a href="{{.NewCrop}}"><img src="{{.NewCrop}}" alt="{{.ExampleFindingID}} new cluster crop"></a><figcaption>New crop</figcaption></figure>{{end}}
</div>
{{end}}
{{if .AcceptedDecision}}<details class="decision-stubs"><summary>representative decision JSONL</summary><pre><code>{{.AcceptedDecision}}
{{.RegressionDecision}}</code></pre></details>{{end}}
</div>
{{end}}
{{if .FindingClustersMore}}<div class="cluster cluster-pages">+{{.FindingClustersMore}} more repeated clusters</div>{{end}}
</section>
{{end}}
{{range .Pages}}
<section class="page priority-{{.Priority}}">
<div class="page-header">
<div>
<h2>{{.Name}}</h2>
<div class="meta">{{.Findings}} · {{.Status}}</div>
</div>
<span class="badge">{{.Priority}}</span>
</div>
<div class="page-body">
<nav class="packet-links" aria-label="{{.Name}} packet files">
{{if .CompareMarkdown}}<a href="{{.CompareMarkdown}}">compare.md</a>{{end}}
{{if .CompareJSON}}<a href="{{.CompareJSON}}">compare.json</a>{{end}}
{{if .PairDecisionsTemplate}}<a href="{{.PairDecisionsTemplate}}">pair decisions</a>{{end}}
{{if .FindingDecisionsTemplate}}<a href="{{.FindingDecisionsTemplate}}">finding decisions</a>{{end}}
{{if .ClusterDecisionsTemplate}}<a href="{{.ClusterDecisionsTemplate}}">cluster decisions</a>{{end}}
</nav>
{{if .FindingClusters}}
<div class="clusters-preview">
<h3>Repeated finding clusters</h3>
{{range .FindingClusters}}
<div class="cluster">
<div class="cluster-head">
<span class="finding-severity severity-{{.Severity}}">{{.Severity}}</span>
<span class="cluster-count">{{.Count}} similar</span>
<span class="finding-kind">{{.Kind}}</span>
{{if .Impact}}<span class="finding-impact">{{.Impact}}</span>{{end}}
</div>
{{if .Target}}<div class="cluster-target">{{.Target}}</div>{{end}}
{{if or .Old .New}}<div class="cluster-values"><code>{{.Old}}</code> -> <code>{{.New}}</code></div>{{end}}
{{if .ExampleFindingID}}<code>{{.ExampleFindingID}}</code>{{end}}
{{if or .OldCrop .NewCrop}}
<div class="finding-crops">
{{if .OldCrop}}<figure><a href="{{.OldCrop}}"><img src="{{.OldCrop}}" alt="{{.ExampleFindingID}} old cluster crop"></a><figcaption>Old crop</figcaption></figure>{{end}}
{{if .NewCrop}}<figure><a href="{{.NewCrop}}"><img src="{{.NewCrop}}" alt="{{.ExampleFindingID}} new cluster crop"></a><figcaption>New crop</figcaption></figure>{{end}}
</div>
{{end}}
{{if .AcceptedDecision}}<details class="decision-stubs"><summary>representative decision JSONL</summary><pre><code>{{.AcceptedDecision}}
{{.RegressionDecision}}</code></pre></details>{{end}}
</div>
{{end}}
{{if .FindingClustersMore}}<div class="cluster cluster-pages">+{{.FindingClustersMore}} more repeated clusters</div>{{end}}
</div>
{{end}}
{{if .FindingPreview}}
<div class="findings-preview">
<h3>Critical and warning findings</h3>
{{range .FindingPreview}}
<div class="finding">
<div class="finding-head">
<span class="finding-severity severity-{{.Severity}}">{{.Severity}}</span>
<span class="finding-kind">{{.Kind}}</span>
{{if .Impact}}<span class="finding-impact">{{.Impact}}</span>{{end}}
</div>
{{if .Target}}<div class="finding-target">{{.Target}}</div>{{end}}
{{if .Locator}}<div class="finding-locator">{{.Locator}}</div>{{end}}
{{if .FindingID}}<code>{{.FindingID}}</code>{{end}}
{{if or .OldCrop .NewCrop}}
<div class="finding-crops">
{{if .OldCrop}}<figure><a href="{{.OldCrop}}"><img src="{{.OldCrop}}" alt="{{.FindingID}} old crop"></a><figcaption>Old crop</figcaption></figure>{{end}}
{{if .NewCrop}}<figure><a href="{{.NewCrop}}"><img src="{{.NewCrop}}" alt="{{.FindingID}} new crop"></a><figcaption>New crop</figcaption></figure>{{end}}
</div>
{{end}}
{{if .AcceptedDecision}}<details class="decision-stubs"><summary>decision JSONL</summary><pre><code>{{.AcceptedDecision}}
{{.RegressionDecision}}</code></pre></details>{{end}}
</div>
{{end}}
{{if .FindingPreviewMore}}<div class="finding finding-impact">+{{.FindingPreviewMore}} more critical or warning findings</div>{{end}}
</div>
{{end}}
<div class="shots">
<figure>
{{if .OldScreenshot}}<a href="{{.OldScreenshot}}"><img src="{{.OldScreenshot}}" alt="{{.Name}} old screenshot"></a>{{else}}<div class="missing">old screenshot missing</div>{{end}}
<figcaption>Old</figcaption>
</figure>
<figure>
{{if .NewScreenshot}}<a href="{{.NewScreenshot}}"><img src="{{.NewScreenshot}}" alt="{{.Name}} new screenshot"></a>{{else}}<div class="missing">new screenshot missing</div>{{end}}
<figcaption>New</figcaption>
</figure>
</div>
</div>
</section>
{{end}}
</main>
</body>
</html>
`
