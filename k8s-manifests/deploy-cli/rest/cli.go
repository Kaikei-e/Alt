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
		Short: "Kubernetes deployment CLI with automatic secret management for Alt RSS Reader",
		Long: `A comprehensive deployment CLI tool for the Alt RSS Reader microservice architecture.

Features:
• Helm-based deployment with environment-specific configurations
• Automatic secret validation and conflict resolution
• Pre-deployment validation and health checking
• Comprehensive maintenance and cleanup operations
• Real-time monitoring and observability capabilities

Common Workflows:
  # Deploy with automatic secret validation
  deploy-cli deploy production

  # Check and fix secret conflicts
  deploy-cli secrets validate production
  deploy-cli secrets fix-conflicts production

  # Validate before deployment
  deploy-cli validate production
  deploy-cli deploy production

  # Troubleshoot deployment issues
  deploy-cli troubleshoot diagnose production
  deploy-cli troubleshoot interactive production

  # Monitor deployment health and metrics
  deploy-cli monitor dashboard production
  deploy-cli monitor services alt-backend production

  # Monitor deployment status
  deploy-cli helm-status production

Supported Environments: development, staging, production`,
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

	// Add update command
	updateCmd := commands.NewUpdateCommand(c.logger)
	c.rootCmd.AddCommand(updateCmd)

	// Add secrets command
	secretsCmd := commands.NewSecretsCommand(c.logger)
	c.rootCmd.AddCommand(secretsCmd)

	// Add troubleshoot command
	troubleshootCmd := commands.NewTroubleshootCommand(c.logger)
	c.rootCmd.AddCommand(troubleshootCmd)

	// Add monitor command
	monitorCmd := commands.NewMonitorCommand(c.logger)
	c.rootCmd.AddCommand(monitorCmd)

	// Add emergency reset command
	emergencyResetCmd := commands.NewEmergencyResetCommand(c.logger)
	c.rootCmd.AddCommand(emergencyResetCmd)

	// Add SSL certificates command
	sslCertificatesCmd := commands.NewSSLCertificatesCommand(c.logger)
	c.rootCmd.AddCommand(sslCertificatesCmd)

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

// SetVersion sets the version of the CLI
func (c *CLI) SetVersion(version string) {
	c.rootCmd.Version = version
}