package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/alt-project/altctl/internal/output"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show current configuration",
	Long: `Display the current altctl configuration.

Examples:
  altctl config                # Show all config
  altctl config --path         # Show config file path
  altctl config --json         # Output as JSON`,
	RunE: runConfig,
}

func init() {
	rootCmd.AddCommand(configCmd)

	configCmd.Flags().Bool("path", false, "show config file path")
	configCmd.Flags().Bool("json", false, "output as JSON")
}

func runConfig(cmd *cobra.Command, args []string) error {
	printer := output.NewPrinter(cfg.Output.Colors)

	showPath, _ := cmd.Flags().GetBool("path")
	jsonOutput, _ := cmd.Flags().GetBool("json")

	if showPath {
		configFile := viper.ConfigFileUsed()
		if configFile == "" {
			printer.Info("No config file found (using defaults)")
		} else {
			printer.Info("Config file: %s", configFile)
		}
		return nil
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(cfg)
	}

	// Print configuration as table
	printer.Header("Current Configuration")

	table := output.NewTable([]string{"KEY", "VALUE"})
	table.AddRow([]string{"project.root", cfg.Project.Root})
	table.AddRow([]string{"project.docker_context", cfg.Project.DockerContext})
	table.AddRow([]string{"compose.dir", cfg.Compose.Dir})
	table.AddRow([]string{"compose.base_file", cfg.Compose.BaseFile})
	table.AddRow([]string{"defaults.stacks", fmt.Sprintf("%v", cfg.Defaults.Stacks)})
	table.AddRow([]string{"logging.level", cfg.Logging.Level})
	table.AddRow([]string{"logging.format", cfg.Logging.Format})
	table.AddRow([]string{"output.colors", fmt.Sprintf("%v", cfg.Output.Colors)})
	table.AddRow([]string{"output.progress", fmt.Sprintf("%v", cfg.Output.Progress)})
	table.Render()

	// Show stack overrides if any
	if len(cfg.Stacks) > 0 {
		fmt.Println()
		printer.Header("Stack Overrides")
		for name, override := range cfg.Stacks {
			printer.Info("%s:", printer.Bold(name))
			if override.RequiresGPU {
				printer.Print("    requires_gpu: true")
			}
			if override.StartupTimeout > 0 {
				printer.Print("    startup_timeout: %s", override.StartupTimeout)
			}
			if len(override.ExtraFiles) > 0 {
				printer.Print("    extra_files: %v", override.ExtraFiles)
			}
		}
	}

	// Show effective compose directory
	fmt.Println()
	printer.Info("Effective compose dir: %s", getComposeDir())

	return nil
}
