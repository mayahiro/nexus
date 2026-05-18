package comparecmd

import (
	"bytes"
	"context"
	"encoding/base64"
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
	"unicode"

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
	compareReviewFileFindingsDir              = "findings"
	compareReviewFileSummary                  = "review-summary.json"
	compareReviewFileManifestJSON             = "manifest.json"
	compareReviewFileManifestMarkdown         = "manifest.md"
	compareReviewFileIndex                    = "review-index.md"
	compareReviewFileIndexHTML                = "review-index.html"
	compareReviewMaxFindingClusters           = 5
	compareReviewMaxClusterFindingIDs         = 20
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
	return writeIndentedJSONFile(files.ReviewSummary, buildCompareReviewSummary(report, files, screenshots.Warnings, cropWarnings))
}

func buildCompareReviewSummary(report compareReport, files compareReviewFiles, screenshotWarnings []string, cropWarnings []string) compareReviewSummary {
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
			FullScreenshot: true,
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
		ReviewIndexHTML:  filepath.Join(dir, compareReviewFileIndexHTML),
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
	if err := writeCompareManifestReviewHTMLIndex(files.ReviewIndexHTML, dir, report, files); err != nil {
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
		FindingClusters:  compareManifestFindingClusters(report),
	}
}

type compareManifestReviewHTMLData struct {
	Manifest            string
	TotalPages          int
	ComparedPages       int
	FailedPages         int
	SamePages           int
	DifferentPages      int
	TotalFindings       int
	CriticalFindings    int
	WarningFindings     int
	InfoFindings        int
	ManifestJSON        string
	ManifestMarkdown    string
	ReviewSummary       string
	ReviewIndex         string
	FindingClusters     []compareManifestReviewHTMLCluster
	FindingClustersMore int
	Pages               []compareManifestReviewHTMLPage
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
	Confidence string `json:"confidence"`
	Reason     string `json:"reason"`
}

func buildCompareManifestReviewHTMLData(rootDir string, report compareManifestReport, files compareManifestReviewFiles) compareManifestReviewHTMLData {
	data := compareManifestReviewHTMLData{
		Manifest:            report.Manifest,
		TotalPages:          report.Summary.TotalPages,
		ComparedPages:       report.Summary.ComparedPages,
		FailedPages:         report.Summary.FailedPages,
		SamePages:           report.Summary.SamePages,
		DifferentPages:      report.Summary.DifferentPages,
		TotalFindings:       report.Summary.TotalFindings,
		CriticalFindings:    report.Summary.Critical,
		WarningFindings:     report.Summary.Warning,
		InfoFindings:        report.Summary.Info,
		ManifestJSON:        compareReviewMarkdownLinkTarget(rootDir, files.ManifestJSON),
		ManifestMarkdown:    compareReviewMarkdownLinkTarget(rootDir, files.ManifestMarkdown),
		ReviewSummary:       compareReviewMarkdownLinkTarget(rootDir, files.ReviewSummary),
		ReviewIndex:         compareReviewMarkdownLinkTarget(rootDir, files.ReviewIndex),
		FindingClusters:     compareManifestReviewHTMLGlobalClusters(rootDir, report, files),
		FindingClustersMore: compareManifestReviewHTMLGlobalClusterOverflow(report),
		Pages:               make([]compareManifestReviewHTMLPage, 0, len(report.Pages)),
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
		AcceptedDecision:   compareManifestReviewFindingDecisionJSONL("accepted_finding", cluster.ExampleFindingID),
		RegressionDecision: compareManifestReviewFindingDecisionJSONL("regression_finding", cluster.ExampleFindingID),
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
