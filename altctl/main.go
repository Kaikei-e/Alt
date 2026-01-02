// Package main is the entry point for altctl CLI
package main

import (
	"os"

	"github.com/alt-project/altctl/cmd"
)

// version is set at build time via ldflags
var version = "dev"

func main() {
	cmd.SetVersion(version)
	if err := cmd.Execute(); err != nil {
		os.Stderr.WriteString("Error: " + err.Error() + "\n")
		os.Exit(1)
	}
}
