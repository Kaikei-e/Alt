package rest

import (
	"context"
	"fmt"
	"os"
	
	"github.com/spf13/cobra"
	
	"deploy-cli/rest/commands"
	"deploy-cli/utils/logger"
	"deploy-cli/utils/colors"
)

// CLI represents the command line interface
type CLI struct {
	rootCmd *cobra.Command
	logger  *logger.Logger
}

// NewCLI creates a new CLI instance
func NewCLI(logger *logger.Logger) *CLI {
	cli := &CLI{
		logger: logger,
	}
	
	cli.rootCmd = &cobra.Command{
		Use:   "deploy-cli",
		Short: "Kubernetes deployment CLI tool for Alt RSS Reader",
		Long: `A deployment CLI tool for the Alt RSS Reader microservice architecture.
This tool provides Helm-based deployment capabilities with environment-specific
configurations and comprehensive validation.`,
		Version: "1.0.0",
	}
	
	cli.setupCommands()
	cli.setupGlobalFlags()
	
	return cli
}

// Execute runs the CLI
func (c *CLI) Execute(ctx context.Context) error {
	return c.rootCmd.ExecuteContext(ctx)
}

// setupCommands sets up all CLI commands
func (c *CLI) setupCommands() {
	// Add deploy command
	deployCmd := commands.NewDeployCommand(c.logger)
	c.rootCmd.AddCommand(deployCmd)
	
	// Add validate command
	validateCmd := commands.NewValidateCommand(c.logger)
	c.rootCmd.AddCommand(validateCmd)
	
	// Add cleanup command
	cleanupCmd := commands.NewCleanupCommand(c.logger)
	c.rootCmd.AddCommand(cleanupCmd)
	
	// Add validate-manifests command
	validateManifestsCmd := commands.NewValidateManifestsCommand(c.logger)
	c.rootCmd.AddCommand(validateManifestsCmd)
	
	// Add helm-status command
	helmStatusCmd := commands.NewHelmStatusCommand(c.logger)
	c.rootCmd.AddCommand(helmStatusCmd)
	
	// Add version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("deploy-cli version %s\n", c.rootCmd.Version)
		},
	}
	c.rootCmd.AddCommand(versionCmd)
}

// setupGlobalFlags sets up global flags
func (c *CLI) setupGlobalFlags() {
	// Add global flags
	c.rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose logging")
	c.rootCmd.PersistentFlags().Bool("no-color", false, "Disable colored output")
	
	// Pre-run hook to handle global flags
	c.rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		// Handle verbose flag
		if verbose, _ := cmd.Flags().GetBool("verbose"); verbose {
			c.logger = logger.NewLoggerWithLevel(logger.DebugLevel)
		}
		
		// Handle no-color flag
		if noColor, _ := cmd.Flags().GetBool("no-color"); noColor {
			colors.DisableColor()
		}
	}
}

// GetLogger returns the logger instance
func (c *CLI) GetLogger() *logger.Logger {
	return c.logger
}

// PrintUsage prints the usage information
func (c *CLI) PrintUsage() {
	c.rootCmd.Help()
}

// PrintVersion prints the version information
func (c *CLI) PrintVersion() {
	fmt.Printf("deploy-cli version %s\n", c.rootCmd.Version)
}

// SetArgs sets the command line arguments (useful for testing)
func (c *CLI) SetArgs(args []string) {
	c.rootCmd.SetArgs(args)
}

// SetOutput sets the output writer (useful for testing)
func (c *CLI) SetOutput(output *os.File) {
	c.rootCmd.SetOutput(output)
}