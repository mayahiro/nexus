package cli

import (
	"context"
	"io"

	comparecmd "github.com/mayahiro/nexus/internal/cli/compare"
)

func runCompare(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	return comparecmd.Run(ctx, args, stdout, stderr, connectClient)
}

func printCompareHelp(w io.Writer) {
	comparecmd.PrintHelp(w)
}
