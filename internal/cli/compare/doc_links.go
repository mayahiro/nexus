package comparecmd

import (
	"fmt"
	"io"
)

const docsBaseURL = "https://github.com/mayahiro/nexus/blob/main/docs/"

const (
	aiUsageDocURL           = docsBaseURL + "ai/usage.md"
	aiCompareDocURL         = docsBaseURL + "ai/compare.md"
	migrationPlaybookDocURL = docsBaseURL + "ai/playbooks/migration.md"
)

func printDocLink(w io.Writer, label string, url string) {
	fmt.Fprintf(w, "%s: %s\n", label, url)
}
