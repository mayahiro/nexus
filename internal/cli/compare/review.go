package comparecmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
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
	compareReviewFileIndexHTML                = "review-index.html"
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
	}
}

type compareManifestReviewHTMLData struct {
	Manifest         string
	TotalPages       int
	ComparedPages    int
	FailedPages      int
	SamePages        int
	DifferentPages   int
	TotalFindings    int
	CriticalFindings int
	WarningFindings  int
	InfoFindings     int
	ManifestJSON     string
	ManifestMarkdown string
	ReviewSummary    string
	ReviewIndex      string
	Pages            []compareManifestReviewHTMLPage
}

type compareManifestReviewHTMLPage struct {
	Name                     string
	Priority                 string
	Findings                 string
	Status                   string
	CompareMarkdown          string
	CompareJSON              string
	PairDecisionsTemplate    string
	FindingDecisionsTemplate string
	OldScreenshot            string
	NewScreenshot            string
	OldScreenshotMissing     bool
	NewScreenshotMissing     bool
}

func buildCompareManifestReviewHTMLData(rootDir string, report compareManifestReport, files compareManifestReviewFiles) compareManifestReviewHTMLData {
	data := compareManifestReviewHTMLData{
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
		ManifestJSON:     compareReviewMarkdownLinkTarget(rootDir, files.ManifestJSON),
		ManifestMarkdown: compareReviewMarkdownLinkTarget(rootDir, files.ManifestMarkdown),
		ReviewSummary:    compareReviewMarkdownLinkTarget(rootDir, files.ReviewSummary),
		ReviewIndex:      compareReviewMarkdownLinkTarget(rootDir, files.ReviewIndex),
		Pages:            make([]compareManifestReviewHTMLPage, 0, len(report.Pages)),
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
		Name:     firstNonEmpty(directory.Name, page.Name),
		Priority: priority,
		Findings: compareManifestReviewFindingsLabel(directory, page),
		Status:   status,
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
