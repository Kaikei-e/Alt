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
			_, err := fmt.Fprintln(cmd.OutOrStdout(), version)
			return err
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
		for _, line := range []string{
			fmt.Sprintf("altctl version %s\n", version),
			fmt.Sprintf("  commit:     %s\n", commit),
			fmt.Sprintf("  built:      %s\n", buildTime),
			fmt.Sprintf("  go version: %s\n", runtime.Version()),
			fmt.Sprintf("  platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH),
		} {
			if _, err := fmt.Fprint(w, line); err != nil {
				return err
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)

	versionCmd.Flags().Bool("short", false, "print version string only")
	versionCmd.Flags().Bool("json", false, "output as JSON")
}
