package commands

import (
	"fmt"
	"time"
	
	"github.com/spf13/cobra"
	
	"deploy-cli/domain"
	"deploy-cli/utils/logger"
	"deploy-cli/utils/colors"
	"deploy-cli/usecase/deployment_usecase"
	"deploy-cli/usecase/secret_usecase"
	"deploy-cli/driver/helm_driver"
	"deploy-cli/driver/kubectl_driver"
	"deploy-cli/driver/filesystem_driver"
	"deploy-cli/driver/system_driver"
	"deploy-cli/gateway/helm_gateway"
	"deploy-cli/gateway/kubectl_gateway"
	"deploy-cli/gateway/filesystem_gateway"
	"deploy-cli/gateway/system_gateway"
	"deploy-cli/port/logger_port"
)

// DeployCommand represents the deploy command
type DeployCommand struct {
	logger  *logger.Logger
	usecase *deployment_usecase.DeploymentUsecase
}

// NewDeployCommand creates a new deploy command
func NewDeployCommand(logger *logger.Logger) *cobra.Command {
	deployCmd := &DeployCommand{
		logger: logger,
	}
	
	cmd := &cobra.Command{
		Use:   "deploy <environment>",
		Short: "Deploy Alt RSS Reader services",
		Long: `Deploy Alt RSS Reader services to Kubernetes using Helm charts.

This command performs comprehensive deployment with automatic validation:
• Pre-deployment secret validation and conflict resolution
• Storage infrastructure setup and verification  
• Namespace creation and configuration
• Helm chart deployment in proper dependency order
• Post-deployment health checking and validation

Automatic Secret Management:
The deployment process automatically validates and resolves secret conflicts
before deploying charts, preventing common deployment failures.

Supported environments:
  - development
  - staging  
  - production

Examples:
  # Deploy to production with automatic secret validation
  deploy-cli deploy production

  # Deploy with custom image tags
  IMAGE_PREFIX=myregistry/alt TAG_BASE=20231201-abc123 deploy-cli deploy production

  # Preview deployment without applying changes
  deploy-cli deploy production --dry-run

  # Deploy and restart all services  
  deploy-cli deploy production --restart

  # Force update pods even with identical manifests
  deploy-cli deploy production --force-update`,
		Args:    cobra.ExactArgs(1),
		PreRunE: deployCmd.preRun,
		RunE:    deployCmd.run,
	}
	
	// Add flags
	cmd.Flags().BoolP("dry-run", "d", false, "Perform dry-run (template charts without deploying)")
	cmd.Flags().BoolP("restart", "r", false, "Restart deployments after deployment")
	cmd.Flags().BoolP("force-update", "f", false, "Force pod updates even when manifests are identical")
	cmd.Flags().StringP("namespace", "n", "", "Override target namespace")
	cmd.Flags().Duration("timeout", 300*time.Second, "Timeout for deployment operations")
	cmd.Flags().String("charts-dir", "../charts", "Directory containing Helm charts")
	
	return cmd
}

// preRun performs pre-execution setup
func (d *DeployCommand) preRun(cmd *cobra.Command, args []string) error {
	d.logger.InfoWithContext("initializing deployment command", "environment", args[0])
	
	// Parse environment
	env, err := domain.ParseEnvironment(args[0])
	if err != nil {
		return fmt.Errorf("invalid environment: %w", err)
	}
	
	// Get environment variables
	imagePrefix := d.getEnvVar("IMAGE_PREFIX")
	if imagePrefix == "" {
		return fmt.Errorf("IMAGE_PREFIX environment variable is required")
	}
	
	// Create deployment options
	options := domain.NewDeploymentOptions()
	options.Environment = env
	options.ImagePrefix = imagePrefix
	options.TagBase = d.getEnvVar("TAG_BASE")
	
	// Set flags
	options.DryRun, _ = cmd.Flags().GetBool("dry-run")
	options.DoRestart, _ = cmd.Flags().GetBool("restart")
	options.ForceUpdate, _ = cmd.Flags().GetBool("force-update")
	options.TargetNamespace, _ = cmd.Flags().GetString("namespace")
	options.Timeout, _ = cmd.Flags().GetDuration("timeout")
	options.ChartsDir, _ = cmd.Flags().GetString("charts-dir")
	
	// Validate options
	if err := options.Validate(); err != nil {
		return fmt.Errorf("deployment options validation failed: %w", err)
	}
	
	// Create dependencies
	d.usecase = d.createDeploymentUsecase()
	
	return nil
}

// run executes the deployment
func (d *DeployCommand) run(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	
	colors.PrintInfo("Starting OSS-optimized deployment workflow")
	
	// Parse environment
	env, _ := domain.ParseEnvironment(args[0])
	
	// Create deployment options
	options := domain.NewDeploymentOptions()
	options.Environment = env
	options.ImagePrefix = d.getEnvVar("IMAGE_PREFIX")
	options.TagBase = d.getEnvVar("TAG_BASE")
	
	// Set flags
	options.DryRun, _ = cmd.Flags().GetBool("dry-run")
	options.DoRestart, _ = cmd.Flags().GetBool("restart")
	options.ForceUpdate, _ = cmd.Flags().GetBool("force-update")
	options.TargetNamespace, _ = cmd.Flags().GetString("namespace")
	options.Timeout, _ = cmd.Flags().GetDuration("timeout")
	options.ChartsDir, _ = cmd.Flags().GetString("charts-dir")
	
	// Execute deployment
	start := time.Now()
	result, err := d.usecase.Deploy(ctx, options)
	duration := time.Since(start)
	
	if err != nil {
		colors.PrintError(fmt.Sprintf("Deployment failed: %v", err))
		return err
	}
	
	// Print results
	d.printDeploymentResults(result, duration)
	
	// Print appropriate completion message based on results
	d.printCompletionMessage(result, duration)
	
	return nil
}

// createDeploymentUsecase creates the deployment usecase with all dependencies
func (d *DeployCommand) createDeploymentUsecase() *deployment_usecase.DeploymentUsecase {
	// Create drivers
	systemDriver := system_driver.NewSystemDriver()
	helmDriver := helm_driver.NewHelmDriver()
	kubectlDriver := kubectl_driver.NewKubectlDriver()
	filesystemDriver := filesystem_driver.NewFileSystemDriver()
	
	// Create logger port adapter
	loggerPort := NewLoggerPortAdapter(d.logger)
	
	// Create gateways
	systemGateway := system_gateway.NewSystemGateway(systemDriver, loggerPort)
	helmGateway := helm_gateway.NewHelmGateway(helmDriver, loggerPort)
	kubectlGateway := kubectl_gateway.NewKubectlGateway(kubectlDriver, loggerPort)
	filesystemGateway := filesystem_gateway.NewFileSystemGateway(filesystemDriver, loggerPort)
	
	// Create usecase
	// Create secret usecase
	secretUsecase := secret_usecase.NewSecretUsecase(kubectlGateway, loggerPort)
	
	return deployment_usecase.NewDeploymentUsecase(
		helmGateway,
		kubectlGateway,
		filesystemGateway,
		systemGateway,
		secretUsecase,
		loggerPort,
		filesystemDriver,
	)
}

// printCompletionMessage prints the appropriate completion message based on results
func (d *DeployCommand) printCompletionMessage(result *domain.DeploymentProgress, duration time.Duration) {
	successCount := result.GetSuccessCount()
	failedCount := result.GetFailedCount()
	skippedCount := result.GetSkippedCount()
	totalCount := result.TotalCharts

	if totalCount == 0 {
		colors.PrintWarning("No charts found to deploy")
		return
	}

	if successCount == 0 && failedCount == 0 && skippedCount == totalCount {
		colors.PrintWarning(fmt.Sprintf("All %d charts were skipped - no deployment performed in %s", totalCount, duration))
		return
	}

	if failedCount == 0 && successCount > 0 {
		colors.PrintSuccess(fmt.Sprintf("OSS-optimized deployment completed successfully in %s (%d charts deployed)", duration, successCount))
	} else if failedCount > 0 && successCount > 0 {
		colors.PrintWarning(fmt.Sprintf("Deployment completed with mixed results in %s (%d successful, %d failed, %d skipped)", duration, successCount, failedCount, skippedCount))
	} else if failedCount > 0 && successCount == 0 {
		colors.PrintError(fmt.Sprintf("Deployment failed in %s (%d charts failed, %d skipped)", duration, failedCount, skippedCount))
	}
}

// printDeploymentResults prints the deployment results
func (d *DeployCommand) printDeploymentResults(result *domain.DeploymentProgress, duration time.Duration) {
	colors.PrintInfo("Deployment Summary")
	
	fmt.Printf("  Total Charts: %d\n", result.TotalCharts)
	fmt.Printf("  Successful: %s\n", colors.Green(fmt.Sprintf("%d", result.GetSuccessCount())))
	fmt.Printf("  Failed: %s\n", colors.Red(fmt.Sprintf("%d", result.GetFailedCount())))
	fmt.Printf("  Skipped: %s\n", colors.Yellow(fmt.Sprintf("%d", result.GetSkippedCount())))
	fmt.Printf("  Duration: %s\n", colors.Cyan(duration.String()))
	
	// Print detailed results
	if len(result.Results) > 0 {
		colors.PrintInfo("Detailed Results")
		for _, r := range result.Results {
			status := ""
			switch r.Status {
			case domain.DeploymentStatusSuccess:
				status = colors.Green("✓")
			case domain.DeploymentStatusFailed:
				status = colors.Red("✗")
			case domain.DeploymentStatusSkipped:
				status = colors.Yellow("⚠")
			}
			
			fmt.Printf("  %s %s → %s (%s)\n", 
				status, 
				r.ChartName, 
				r.Namespace, 
				r.Duration)
				
			if r.Error != nil {
				fmt.Printf("    Error: %s\n", colors.Red(r.Error.Error()))
			}
		}
	}
}

// getEnvVar gets an environment variable value
func (d *DeployCommand) getEnvVar(key string) string {
	systemDriver := system_driver.NewSystemDriver()
	return systemDriver.GetEnvironmentVariable(key)
}

// LoggerPortAdapter adapts the logger to the logger port interface
type LoggerPortAdapter struct {
	logger *logger.Logger
}

// NewLoggerPortAdapter creates a new logger port adapter
func NewLoggerPortAdapter(logger *logger.Logger) logger_port.LoggerPort {
	return &LoggerPortAdapter{logger: logger}
}

// Info logs an info message
func (l *LoggerPortAdapter) Info(msg string, args ...interface{}) {
	l.logger.InfoWithContext(msg, args...)
}

// Error logs an error message
func (l *LoggerPortAdapter) Error(msg string, args ...interface{}) {
	l.logger.ErrorWithContext(msg, args...)
}

// Warn logs a warning message
func (l *LoggerPortAdapter) Warn(msg string, args ...interface{}) {
	l.logger.WarnWithContext(msg, args...)
}

// Debug logs a debug message
func (l *LoggerPortAdapter) Debug(msg string, args ...interface{}) {
	l.logger.DebugWithContext(msg, args...)
}

// InfoWithContext logs an info message with context
func (l *LoggerPortAdapter) InfoWithContext(msg string, context map[string]interface{}) {
	args := make([]interface{}, 0, len(context)*2)
	for k, v := range context {
		args = append(args, k, v)
	}
	l.logger.InfoWithContext(msg, args...)
}

// ErrorWithContext logs an error message with context
func (l *LoggerPortAdapter) ErrorWithContext(msg string, context map[string]interface{}) {
	args := make([]interface{}, 0, len(context)*2)
	for k, v := range context {
		args = append(args, k, v)
	}
	l.logger.ErrorWithContext(msg, args...)
}

// WarnWithContext logs a warning message with context
func (l *LoggerPortAdapter) WarnWithContext(msg string, context map[string]interface{}) {
	args := make([]interface{}, 0, len(context)*2)
	for k, v := range context {
		args = append(args, k, v)
	}
	l.logger.WarnWithContext(msg, args...)
}

// DebugWithContext logs a debug message with context
func (l *LoggerPortAdapter) DebugWithContext(msg string, context map[string]interface{}) {
	args := make([]interface{}, 0, len(context)*2)
	for k, v := range context {
		args = append(args, k, v)
	}
	l.logger.DebugWithContext(msg, args...)
}

// WithField adds a field to the logger context
func (l *LoggerPortAdapter) WithField(key string, value interface{}) logger_port.LoggerPort {
	return &LoggerPortAdapter{logger: l.logger.WithContext(key, value)}
}

// WithFields adds multiple fields to the logger context
func (l *LoggerPortAdapter) WithFields(fields map[string]interface{}) logger_port.LoggerPort {
	newLogger := l.logger
	for key, value := range fields {
		newLogger = newLogger.WithContext(key, value)
	}
	return &LoggerPortAdapter{logger: newLogger}
}