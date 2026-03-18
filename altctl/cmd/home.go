package cmd

import (
	"github.com/spf13/cobra"
)

var homeCmd = &cobra.Command{
	Use:   "home",
	Short: "Knowledge Home operations",
	Long:  "Manage Knowledge Home projections, reprojections, and SLO status.",
}

func init() {
	rootCmd.AddCommand(homeCmd)
}
