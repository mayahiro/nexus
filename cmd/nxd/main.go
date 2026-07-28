package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/daemon"
)

func main() {
	var verbose bool
	flag.BoolVar(&verbose, "verbose", false, "write detailed daemon and screenshot diagnostics")
	flag.BoolVar(&verbose, "v", false, "write detailed daemon and screenshot diagnostics")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "unexpected arguments: %v\n", flag.Args())
		os.Exit(2)
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	logPath := ""
	if base := os.Getenv(daemon.ProcessLogBaseEnv); base != "" {
		logPath = daemon.ProcessLogPath(base, os.Getpid())
	}
	if err := daemon.Run(ctx, paths, daemon.RunOptions{
		LogPath: logPath,
		Verbose: verbose,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
