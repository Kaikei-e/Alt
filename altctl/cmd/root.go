// Package cmd contains all CLI commands for altctl
package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/alt-project/altctl/internal/compose"
	"github.com/alt-project/altctl/internal/config"
	"github.com/alt-project/altctl/internal/output"
	"github.com/alt-project/altctl/internal/stack"
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

// Execute adds all child commands to the root command and sets flags
// appropriately, running under a background context. Prefer ExecuteContext
// in main.go so Ctrl-C (SIGINT/SIGTERM) can cancel in-flight compose
// invocations and the Ready-wait poll loop; Execute remains for callers
// (tests) that don't need signal-driven cancellation.
func Execute() error {
	return rootCmd.Execute()
}

// ExecuteContext runs the root command under ctx: cancelling ctx (main.go
// wires this to signal.NotifyContext for SIGINT/SIGTERM) propagates via
// cobra's cmd.Context() into every subcommand's compose invocations
// (internal/compose's executor already uses exec.CommandContext, so
// in-flight `docker` child processes are killed) and into the
// internal/health Ready-wait poll loop, which checks ctx before every
// poll/sleep and returns promptly instead of blocking for a full
// PollInterval or the whole startup timeout.
func ExecuteContext(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
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

// getConfigFilePath returns the altctl config file the stack registry
// should read for stack semantics (depends_on, optional, provides/...,
// overlays/excluded) that can't be derived from compose/*.yaml alone.
func getConfigFilePath() string {
	if cfg != nil && cfg.ConfigFilePath != "" {
		return cfg.ConfigFilePath
	}
	return filepath.Join(getProjectRoot(), ".altctl.yaml")
}

// loadRegistry builds the stack registry from the effective compose
// directory and altctl config file. Stacks are derived from compose/*.yaml
// at call time (see internal/stack.NewRegistry), so this can fail if
// .altctl.yaml declares a stack with no matching compose file, or if the
// compose directory itself can't be read.
func loadRegistry() (*stack.Registry, error) {
	registry, err := stack.NewRegistry(getComposeDir(), getConfigFilePath())
	if err != nil {
		return nil, &output.CLIError{
			Summary:    "failed to load stack registry",
			Detail:     err.Error(),
			Suggestion: "Check compose/*.yaml and the 'stacks:'/'overlays:' sections of .altctl.yaml",
			ExitCode:   output.ExitConfigError,
		}
	}
	return registry, nil
}

// newComposeClient builds the *compose.Client every lifecycle command (up,
// down, restart, rebuild, logs, exec) uses. It's a package-level factory
// var rather than a direct compose.NewClient(...) call at each call site so
// tests can substitute a client wired to a fake compose.Executor (via
// compose.NewClientWithExecutor) to capture the exact argv a command
// builds -- file list, --profile, service names, --no-deps/--force-recreate
// -- instead of only being able to assert against --dry-run log text (see
// M2 test-honesty fix: those capture-exact-argv assertions are what would
// have caught C3/C4/H2 in the first place). Tests must restore this to
// defaultComposeClient (or set their own) via t.Cleanup.
var newComposeClient = defaultComposeClient

// defaultComposeClient is newComposeClient's production implementation.
func defaultComposeClient() *compose.Client {
	return compose.NewClient(getProjectRoot(), getComposeDir(), logger, dryRun)
}
