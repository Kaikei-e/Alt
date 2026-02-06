package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var docsCmd = &cobra.Command{
	Use:    "docs",
	Short:  "Generate documentation in various formats",
	Long:   `Generate man pages or markdown documentation for all altctl commands.`,
	Hidden: true,
	RunE:   runDocs,
}

func init() {
	rootCmd.AddCommand(docsCmd)

	docsCmd.Flags().String("format", "man", "output format: man, markdown")
	docsCmd.Flags().StringP("output", "o", "./docs", "output directory")
}

func runDocs(cmd *cobra.Command, args []string) error {
	format, _ := cmd.Flags().GetString("format")
	outputDir, _ := cmd.Flags().GetString("output")

	switch format {
	case "man":
		header := &doc.GenManHeader{
			Title:   "ALTCTL",
			Section: "1",
		}
		return doc.GenManTree(rootCmd, header, outputDir)
	case "markdown":
		return doc.GenMarkdownTree(rootCmd, outputDir)
	default:
		return fmt.Errorf("unknown format %q: use 'man' or 'markdown'", format)
	}
}
