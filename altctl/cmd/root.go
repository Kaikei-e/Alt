// Package cmd contains all CLI commands for altctl
package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/alt-project/altctl/internal/config"
	"github.com/alt-project/altctl/internal/output"
)

var (
	cfgFile    string
	verbose    bool
	dryRun     bool
	quiet      bool
	colorFlag  string
	colorMode  output.ColorMode
	projectDir string
	cfg        *config.Config
	logger     *slog.Logger
	version    = "dev"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "altctl",
	Short: "Alt platform orchestration CLI",
	Long: `altctl is a CLI tool for managing the Alt platform's Docker Compose services.

It provides simplified orchestration of the platform's microservices through
stack-based management with automatic dependency resolution.

Example usage:
  altctl up                    # Start default stacks (db, auth, core, workers)
  altctl up ai                 # Start AI stack with dependencies
  altctl down                  # Stop all running stacks
  altctl status                # Show service status by stack
  altctl list                  # List available stacks

Exit Codes:
  0  Success
  1  General error
  2  Usage error (invalid arguments or unknown stack)
  3  Docker Compose error
  4  Configuration error
  5  Timeout`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return initConfig()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

// SetVersion sets the version string for the CLI
func SetVersion(v string) {
	version = v
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is .altctl.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "show commands without executing")
	rootCmd.PersistentFlags().StringVar(&projectDir, "project-dir", "", "Alt project directory (default: auto-detect)")
	rootCmd.PersistentFlags().StringVar(&colorFlag, "color", "auto", "color output: always, auto, never")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "suppress non-error output")

	// Bind flags to viper
	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	_ = viper.BindPFlag("dry_run", rootCmd.PersistentFlags().Lookup("dry-run"))
	_ = viper.BindPFlag("project.root", rootCmd.PersistentFlags().Lookup("project-dir"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() error {
	var err error

	// Validate --quiet and --verbose are not both set
	if quiet && verbose {
		return fmt.Errorf("--quiet and --verbose are mutually exclusive")
	}

	// Parse --color flag
	colorMode, err = output.ParseColorMode(colorFlag)
	if err != nil {
		return err
	}

	// Setup logger
	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Load configuration
	cfg, err = config.Load(cfgFile, projectDir)
	if err != nil {
		return &output.CLIError{
			Summary:    "failed to load configuration",
			Detail:     err.Error(),
			Suggestion: "Check .altctl.yaml syntax or use --config flag",
			ExitCode:   output.ExitConfigError,
		}
	}

	// Override config colors based on --color flag
	cfg.Output.Colors = output.ResolveColors(colorMode, cfg.Output.Colors)

	// Update logger based on config
	if cfg.Logging.Level == "debug" || verbose {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
	}

	logger.Debug("configuration loaded",
		"project_root", cfg.Project.Root,
		"compose_dir", cfg.Compose.Dir,
		"default_stacks", cfg.Defaults.Stacks,
	)

	return nil
}

// newPrinter creates a Printer using resolved color/quiet settings
func newPrinter() *output.Printer {
	return output.NewPrinterWithOptions(output.PrinterOptions{
		ColorMode:    colorMode,
		ConfigColors: cfg.Output.Colors,
		Quiet:        quiet,
	})
}

// getProjectRoot returns the project root directory
func getProjectRoot() string {
	if cfg != nil && cfg.Project.Root != "" {
		return cfg.Project.Root
	}
	// Try to find project root by looking for compose.yaml
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	// Walk up to find compose.yaml or compose/ directory
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "compose.yaml")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "compose")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd
		}
		dir = parent
	}
}

// getComposeDir returns the compose files directory
func getComposeDir() string {
	root := getProjectRoot()
	if cfg != nil && cfg.Compose.Dir != "" {
		return filepath.Join(root, cfg.Compose.Dir)
	}
	return filepath.Join(root, "compose")
}
