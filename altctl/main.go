// Package main is the entry point for altctl CLI
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/alt-project/altctl/cmd"
	"github.com/alt-project/altctl/internal/output"
)

// Build-time variables set via ldflags
var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	cmd.SetVersion(version)
	cmd.SetBuildInfo(commit, buildTime)

	// SIGINT/SIGTERM cancel ctx instead of killing the process outright, so
	// cmd.Context() (used throughout cmd/ and internal/health's Ready-wait
	// poll loop) observes the cancellation and unwinds promptly -- and, via
	// internal/compose's executor already using exec.CommandContext, any
	// in-flight `docker` child process gets killed too instead of being
	// orphaned.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cmd.ExecuteContext(ctx); err != nil {
		var cliErr *output.CLIError
		if errors.As(err, &cliErr) {
			printer := output.NewPrinter(false)
			printer.FormatError(cliErr)
			os.Exit(cliErr.ExitCode)
		}
		if ctx.Err() != nil {
			os.Stderr.WriteString("\ninterrupted — stack may be partially started; run altctl doctor\n")
			os.Exit(output.ExitGeneral)
		}
		os.Stderr.WriteString("Error: " + err.Error() + "\n")
		os.Exit(1)
	}
}
