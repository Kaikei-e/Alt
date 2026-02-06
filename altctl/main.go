// Package main is the entry point for altctl CLI
package main

import (
	"errors"
	"os"

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
	if err := cmd.Execute(); err != nil {
		var cliErr *output.CLIError
		if errors.As(err, &cliErr) {
			printer := output.NewPrinter(false)
			printer.FormatError(cliErr)
			os.Exit(cliErr.ExitCode)
		}
		os.Stderr.WriteString("Error: " + err.Error() + "\n")
		os.Exit(1)
	}
}
