// Command clidocs regenerates the CLI reference from the repository root.
package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mayahiro/nagicli-go/document"

	"github.com/mayahiro/nexus/internal/cli"
)

//go:embed intro.md
var introduction string

//go:embed intro_ja.md
var introductionJA string

func main() {
	if err := writeReferences(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func writeReferences() error {
	references, err := renderReferences()
	if err != nil {
		return err
	}
	for _, reference := range references {
		path := filepath.Join("docs", reference.name)
		if err := os.WriteFile(path, []byte(reference.markdown), 0o644); err != nil {
			return fmt.Errorf("write CLI reference: %w", err)
		}
	}
	return nil
}

type referenceDocument struct {
	name     string
	markdown string
}

func renderReferences() ([]referenceDocument, error) {
	documents, err := cli.CommandHelpDocuments()
	if err != nil {
		return nil, err
	}

	var index, body strings.Builder
	renderer := document.MarkdownRenderer{}
	for _, help := range documents {
		path := help.CommandPath()
		fmt.Fprintf(&index, "- [%s](#%s)\n", strings.Join(path, " "), strings.Join(path, "-"))
		body.WriteByte('\n')
		for line := range strings.SplitAfterSeq(renderer.Render(help), "\n") {
			// Renderer headings sit one level below the reference title.
			// Its code blocks are indented, so heading-like examples are untouched.
			if strings.HasPrefix(line, "#") {
				body.WriteByte('#')
			}
			// Nagi 0.4 retains terminal alignment in option labels. Spaces
			// after the opening ** prevent CommonMark from rendering bold.
			if label, ok := strings.CutPrefix(line, "- **"); ok {
				line = "- **" + strings.TrimLeft(label, " ")
			}
			body.WriteString(line)
		}
	}
	content := index.String() + body.String()
	return []referenceDocument{
		{name: "cli-reference.md", markdown: strings.TrimRight(introduction, "\n") + "\n\n" + content},
		{name: "cli-reference_ja.md", markdown: strings.TrimRight(introductionJA, "\n") + "\n\n" + content},
	}, nil
}
