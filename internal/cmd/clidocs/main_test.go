package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommandReferencesCurrent(t *testing.T) {
	references, err := renderReferences()
	if err != nil {
		t.Fatal(err)
	}
	for _, reference := range references {
		t.Run(reference.name, func(t *testing.T) {
			path := filepath.Join("..", "..", "..", "docs", reference.name)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read CLI reference: %v; run mise run docs", err)
			}
			if string(data) != reference.markdown {
				t.Fatal("CLI reference differs from the command graph; run mise run docs")
			}
		})
	}
}
