package cmd

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	commit    = "unknown"
	buildTime = "unknown"
)

// SetBuildInfo sets the commit hash and build time
func SetBuildInfo(c, bt string) {
	commit = c
	buildTime = bt
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print the version, build information, and Go runtime version.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		short, _ := cmd.Flags().GetBool("short")
		jsonOutput, _ := cmd.Flags().GetBool("json")

		if short {
			fmt.Fprintln(cmd.OutOrStdout(), version)
			return nil
		}

		if jsonOutput {
			info := map[string]string{
				"version":   version,
				"commit":    commit,
				"built":     buildTime,
				"goVersion": runtime.Version(),
				"platform":  runtime.GOOS + "/" + runtime.GOARCH,
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(info)
		}

		w := cmd.OutOrStdout()
		fmt.Fprintf(w, "altctl version %s\n", version)
		fmt.Fprintf(w, "  commit:     %s\n", commit)
		fmt.Fprintf(w, "  built:      %s\n", buildTime)
		fmt.Fprintf(w, "  go version: %s\n", runtime.Version())
		fmt.Fprintf(w, "  platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)

	versionCmd.Flags().Bool("short", false, "print version string only")
	versionCmd.Flags().Bool("json", false, "output as JSON")
}
