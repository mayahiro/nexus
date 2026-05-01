package comparecmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func writeIndentedJSONFile(path string, value any) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeCompareJSON(path string, report compareReport) error {
	return writeIndentedJSONFile(path, report)
}

func writeCompareMarkdown(path string, report compareReport) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	printCompareMarkdown(file, report)
	return nil
}

func writeCompareManifestMarkdown(path string, report compareManifestReport) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	printCompareManifestMarkdown(file, report)
	return nil
}

func printCompareReport(w io.Writer, report compareReport) {
	fmt.Fprintf(w, "old: %s", firstNonEmpty(report.Old.URL, report.Old.SessionID))
	if report.Old.Title != "" {
		fmt.Fprintf(w, " (%s)", report.Old.Title)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "new: %s", firstNonEmpty(report.New.URL, report.New.SessionID))
	if report.New.Title != "" {
		fmt.Fprintf(w, " (%s)", report.New.Title)
	}
	fmt.Fprintln(w)
	if report.Scope != nil {
		fmt.Fprintf(w, "scope: %s", compareScopeLabel(report.Scope))
		if report.Scope.Old.Tag != "" || report.Scope.New.Tag != "" {
			fmt.Fprintf(w, " (%s -> %s)", firstNonEmpty(report.Scope.Old.Tag, "?"), firstNonEmpty(report.Scope.New.Tag, "?"))
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "summary: %d findings\n", report.Summary.TotalFindings)
	if report.Summary.Same {
		fmt.Fprintln(w, "no significant differences")
		return
	}
	fmt.Fprintf(w, "critical: %d\n", report.Summary.Critical)
	fmt.Fprintf(w, "warning: %d\n", report.Summary.Warning)
	fmt.Fprintf(w, "info: %d\n", report.Summary.Info)

	if report.Summary.TitleChanged > 0 {
		fmt.Fprintf(w, "title_changed: %d\n", report.Summary.TitleChanged)
	}
	if report.Summary.PageTextChanged > 0 {
		fmt.Fprintf(w, "page_text_changed: %d\n", report.Summary.PageTextChanged)
	}
	if report.Summary.TextChanged > 0 {
		fmt.Fprintf(w, "text_changed: %d\n", report.Summary.TextChanged)
	}
	if report.Summary.MissingNodes > 0 {
		fmt.Fprintf(w, "missing_node: %d\n", report.Summary.MissingNodes)
	}
	if report.Summary.NewNodes > 0 {
		fmt.Fprintf(w, "new_node: %d\n", report.Summary.NewNodes)
	}
	if report.Summary.StateChanged > 0 {
		fmt.Fprintf(w, "state_changed: %d\n", report.Summary.StateChanged)
	}
	if report.Summary.CSSChanged > 0 {
		fmt.Fprintf(w, "css_changed: %d\n", report.Summary.CSSChanged)
	}
	if report.Summary.LayoutChanged > 0 {
		fmt.Fprintf(w, "layout_changed: %d\n", report.Summary.LayoutChanged)
	}

	fmt.Fprintln(w)
	for _, finding := range report.Findings {
		switch finding.Kind {
		case "title_changed":
			fmt.Fprintf(w, "[%s] [title_changed] %s: %q -> %q\n", finding.Severity, finding.Impact, finding.Old, finding.New)
		case "page_text_changed":
			fmt.Fprintf(w, "[%s] [page_text_changed] %s: %q -> %q\n", finding.Severity, finding.Impact, finding.Old, finding.New)
		case "missing_node":
			fmt.Fprintf(w, "[%s] [missing_node] %s %s %q%s\n", finding.Severity, finding.Impact, finding.Role, finding.Label, compareFindingPlainLocatorSuffix(finding))
		case "new_node":
			fmt.Fprintf(w, "[%s] [new_node] %s %s %q%s\n", finding.Severity, finding.Impact, finding.Role, finding.Label, compareFindingPlainLocatorSuffix(finding))
		case "text_changed":
			fmt.Fprintf(w, "[%s] [text_changed] %s %s %q %s: %q -> %q%s\n", finding.Severity, finding.Impact, finding.Role, finding.Label, finding.Field, finding.Old, finding.New, compareFindingPlainLocatorSuffix(finding))
		case "state_changed":
			fmt.Fprintf(w, "[%s] [state_changed] %s %s %q: %s -> %s%s\n", finding.Severity, finding.Impact, finding.Role, finding.Label, finding.Old, finding.New, compareFindingPlainLocatorSuffix(finding))
		case "css_changed":
			fmt.Fprintf(w, "[%s] [css_changed] %s %s %q %s: %q -> %q%s\n", finding.Severity, finding.Impact, finding.Role, finding.Label, finding.Field, finding.Old, finding.New, compareFindingPlainLocatorSuffix(finding))
		case "layout_changed":
			fmt.Fprintf(w, "[%s] [layout_changed] %s %s %q: %q -> %q%s\n", finding.Severity, finding.Impact, finding.Role, finding.Label, finding.Old, finding.New, compareFindingPlainLocatorSuffix(finding))
		}
	}
}

func printCompareManifestReport(w io.Writer, report compareManifestReport) {
	fmt.Fprintf(w, "manifest: %s\n", report.Manifest)
	fmt.Fprintf(w, "pages: %d total, %d compared, %d failed\n", report.Summary.TotalPages, report.Summary.ComparedPages, report.Summary.FailedPages)
	fmt.Fprintf(w, "same: %d\n", report.Summary.SamePages)
	fmt.Fprintf(w, "different: %d\n", report.Summary.DifferentPages)
	fmt.Fprintf(w, "summary: %d findings\n", report.Summary.TotalFindings)
	fmt.Fprintf(w, "critical: %d\n", report.Summary.Critical)
	fmt.Fprintf(w, "warning: %d\n", report.Summary.Warning)
	fmt.Fprintf(w, "info: %d\n", report.Summary.Info)
	if report.Summary.TotalFindings == 0 && report.Summary.FailedPages == 0 {
		fmt.Fprintln(w, "no significant differences")
	}
	for _, page := range report.Pages {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "[%s]\n", page.Name)
		if page.Error != "" {
			fmt.Fprintf(w, "error: %s\n", page.Error)
			continue
		}
		if page.Report != nil {
			printCompareReport(w, *page.Report)
		}
	}
}

func printCompareMarkdown(w io.Writer, report compareReport) {
	fmt.Fprintln(w, "# Compare Report")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- Old: `%s`\n", firstNonEmpty(report.Old.URL, report.Old.SessionID))
	fmt.Fprintf(w, "- New: `%s`\n", firstNonEmpty(report.New.URL, report.New.SessionID))
	if report.Scope != nil {
		fmt.Fprintf(w, "- Scope: `%s`\n", compareScopeLabel(report.Scope))
	}
	if report.Old.Title != "" || report.New.Title != "" {
		fmt.Fprintf(w, "- Titles: `%s` -> `%s`\n", report.Old.Title, report.New.Title)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Summary")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- Total findings: %d\n", report.Summary.TotalFindings)
	fmt.Fprintf(w, "- Critical: %d\n", report.Summary.Critical)
	fmt.Fprintf(w, "- Warning: %d\n", report.Summary.Warning)
	fmt.Fprintf(w, "- Info: %d\n", report.Summary.Info)
	if report.Summary.Same {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "No significant differences.")
		return
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Findings")
	fmt.Fprintln(w)
	for _, finding := range report.Findings {
		switch finding.Kind {
		case "title_changed":
			fmt.Fprintf(w, "- [%s] `%s`: `%s` -> `%s`\n", finding.Severity, finding.Impact, finding.Old, finding.New)
		case "page_text_changed":
			fmt.Fprintf(w, "- [%s] `%s`: `%s` -> `%s`\n", finding.Severity, finding.Impact, finding.Old, finding.New)
		case "missing_node", "new_node":
			fmt.Fprintf(w, "- [%s] `%s`: `%s` `%s`%s\n", finding.Severity, finding.Impact, finding.Role, finding.Label, compareFindingMarkdownLocatorSuffix(finding))
		case "text_changed":
			fmt.Fprintf(w, "- [%s] `%s`: `%s` `%s` `%s` `%s` -> `%s`%s\n", finding.Severity, finding.Impact, finding.Role, finding.Label, finding.Field, finding.Old, finding.New, compareFindingMarkdownLocatorSuffix(finding))
		case "state_changed":
			fmt.Fprintf(w, "- [%s] `%s`: `%s` `%s` `%s` -> `%s`%s\n", finding.Severity, finding.Impact, finding.Role, finding.Label, finding.Old, finding.New, compareFindingMarkdownLocatorSuffix(finding))
		case "css_changed":
			fmt.Fprintf(w, "- [%s] `%s`: `%s` `%s` `%s` `%s` -> `%s`%s\n", finding.Severity, finding.Impact, finding.Role, finding.Label, finding.Field, finding.Old, finding.New, compareFindingMarkdownLocatorSuffix(finding))
		case "layout_changed":
			fmt.Fprintf(w, "- [%s] `%s`: `%s` `%s` `%s` -> `%s`%s\n", finding.Severity, finding.Impact, finding.Role, finding.Label, finding.Old, finding.New, compareFindingMarkdownLocatorSuffix(finding))
		}
	}
}

func printCompareManifestMarkdown(w io.Writer, report compareManifestReport) {
	fmt.Fprintln(w, "# Compare Manifest Report")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- Manifest: `%s`\n", report.Manifest)
	fmt.Fprintf(w, "- Pages: %d total / %d compared / %d failed\n", report.Summary.TotalPages, report.Summary.ComparedPages, report.Summary.FailedPages)
	fmt.Fprintf(w, "- Same: %d\n", report.Summary.SamePages)
	fmt.Fprintf(w, "- Different: %d\n", report.Summary.DifferentPages)
	fmt.Fprintf(w, "- Total findings: %d\n", report.Summary.TotalFindings)
	fmt.Fprintf(w, "- Critical: %d\n", report.Summary.Critical)
	fmt.Fprintf(w, "- Warning: %d\n", report.Summary.Warning)
	fmt.Fprintf(w, "- Info: %d\n", report.Summary.Info)
	for _, page := range report.Pages {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "## %s\n", page.Name)
		fmt.Fprintln(w)
		if page.Error != "" {
			fmt.Fprintf(w, "Error: %s\n", page.Error)
			continue
		}
		if page.Report == nil {
			continue
		}
		fmt.Fprintf(w, "- Old: `%s`\n", firstNonEmpty(page.Report.Old.URL, page.Report.Old.SessionID))
		fmt.Fprintf(w, "- New: `%s`\n", firstNonEmpty(page.Report.New.URL, page.Report.New.SessionID))
		if page.Report.Scope != nil {
			fmt.Fprintf(w, "- Scope: `%s`\n", compareScopeLabel(page.Report.Scope))
		}
		fmt.Fprintf(w, "- Findings: %d\n", page.Report.Summary.TotalFindings)
		fmt.Fprintf(w, "- Critical: %d\n", page.Report.Summary.Critical)
		fmt.Fprintf(w, "- Warning: %d\n", page.Report.Summary.Warning)
		fmt.Fprintf(w, "- Info: %d\n", page.Report.Summary.Info)
		if page.Report.Summary.Same {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "No significant differences.")
			continue
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, "### Findings")
		fmt.Fprintln(w)
		for _, finding := range page.Report.Findings {
			switch finding.Kind {
			case "title_changed":
				fmt.Fprintf(w, "- [%s] `%s`: `%s` -> `%s`\n", finding.Severity, finding.Impact, finding.Old, finding.New)
			case "page_text_changed":
				fmt.Fprintf(w, "- [%s] `%s`: `%s` -> `%s`\n", finding.Severity, finding.Impact, finding.Old, finding.New)
			case "missing_node", "new_node":
				fmt.Fprintf(w, "- [%s] `%s`: `%s` `%s`%s\n", finding.Severity, finding.Impact, finding.Role, finding.Label, compareFindingMarkdownLocatorSuffix(finding))
			case "text_changed":
				fmt.Fprintf(w, "- [%s] `%s`: `%s` `%s` `%s` `%s` -> `%s`%s\n", finding.Severity, finding.Impact, finding.Role, finding.Label, finding.Field, finding.Old, finding.New, compareFindingMarkdownLocatorSuffix(finding))
			case "state_changed":
				fmt.Fprintf(w, "- [%s] `%s`: `%s` `%s` `%s` -> `%s`%s\n", finding.Severity, finding.Impact, finding.Role, finding.Label, finding.Old, finding.New, compareFindingMarkdownLocatorSuffix(finding))
			case "css_changed":
				fmt.Fprintf(w, "- [%s] `%s`: `%s` `%s` `%s` `%s` -> `%s`%s\n", finding.Severity, finding.Impact, finding.Role, finding.Label, finding.Field, finding.Old, finding.New, compareFindingMarkdownLocatorSuffix(finding))
			case "layout_changed":
				fmt.Fprintf(w, "- [%s] `%s`: `%s` `%s` `%s` -> `%s`%s\n", finding.Severity, finding.Impact, finding.Role, finding.Label, finding.Old, finding.New, compareFindingMarkdownLocatorSuffix(finding))
			}
		}
	}
}

func compareScopeLabel(scope *compareScope) string {
	if scope == nil {
		return ""
	}
	if strings.TrimSpace(scope.Selector) != "" {
		return strings.TrimSpace(scope.Selector)
	}
	oldSelector := strings.TrimSpace(scope.Old.Selector)
	newSelector := strings.TrimSpace(scope.New.Selector)
	if oldSelector == "" && newSelector == "" {
		return ""
	}
	return "old=" + oldSelector + " new=" + newSelector
}

func compareFindingPlainLocatorSuffix(finding compareFinding) string {
	if strings.TrimSpace(finding.Locator) == "" {
		return ""
	}
	return fmt.Sprintf(" [locator: %s]", finding.Locator)
}

func compareFindingMarkdownLocatorSuffix(finding compareFinding) string {
	if strings.TrimSpace(finding.Locator) == "" {
		return ""
	}
	return fmt.Sprintf(" locator `%s`", finding.Locator)
}
