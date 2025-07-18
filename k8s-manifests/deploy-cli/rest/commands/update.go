package commands

import (
	"context"
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
)

// UpdateCommand represents the update command
type UpdateCommand struct {
	logger  *logger.Logger
	usecase *deployment_usecase.DeploymentUsecase
}

// NewUpdateCommand creates a new update command
func NewUpdateCommand(logger *logger.Logger) *cobra.Command {
	updateCmd := &UpdateCommand{
		logger: logger,
	}
	
	cmd := &cobra.Command{
		Use:   "update <environment>",
		Short: "Force update pods and deployments",
		Long: `Force update pods and deployments in the specified environment.
		
This command performs targeted updates with automatic secret validation:
• Forces pod recreation even when manifests are identical
• Automatically validates and resolves secret conflicts before updating
• Useful for pulling new images with the same tag
• Enables rolling updates without full deployment process

Automatic Secret Management:  
Like the deploy command, update includes automatic secret validation and
conflict resolution to prevent update failures due to secret issues.

Use Cases:
• Pulling new Docker images with same tags
• Forcing rolling restarts for configuration changes
• Recovering from failed partial deployments
• Testing new images before full deployment

Examples:
  # Force update all pods in production (with secret validation)
  deploy-cli update production

  # Force update specific chart only
  deploy-cli update production --chart alt-frontend
  
  # Force update and restart all resources
  deploy-cli update production --restart

  # Update with custom timeout
  deploy-cli update production --timeout 10m`,
		Args:    cobra.ExactArgs(1),
		PreRunE: updateCmd.preRun,
		RunE:    updateCmd.run,
	}
	
	// Add flags
	cmd.Flags().String("chart", "", "Update specific chart only")
	cmd.Flags().BoolP("restart", "r", false, "Restart all resources after update")
	cmd.Flags().StringP("namespace", "n", "", "Override target namespace")
	cmd.Flags().Duration("timeout", 300*time.Second, "Timeout for update operations")
	cmd.Flags().String("charts-dir", "/home/koko/Documents/dev/Alt/charts", "Directory containing Helm charts")
	
	return cmd
}

// preRun performs pre-execution setup
func (u *UpdateCommand) preRun(cmd *cobra.Command, args []string) error {
	u.logger.InfoWithContext("initializing update command", "environment", args[0])
	
	// Create dependencies
	u.usecase = u.createDeploymentUsecase()
	
	return nil
}

// run executes the update
func (u *UpdateCommand) run(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	
	colors.PrintInfo("Starting forced update process")
	
	// Parse environment
	env, err := domain.ParseEnvironment(args[0])
	if err != nil {
		return fmt.Errorf("invalid environment: %w", err)
	}
	
	// Get flags
	chartName, _ := cmd.Flags().GetString("chart")
	doRestart, _ := cmd.Flags().GetBool("restart")
	targetNamespace, _ := cmd.Flags().GetString("namespace")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	chartsDir, _ := cmd.Flags().GetString("charts-dir")
	
	// Create deployment options with force update enabled
	options := domain.NewDeploymentOptions()
	options.Environment = env
	options.ForceUpdate = true
	options.DoRestart = doRestart
	options.TargetNamespace = targetNamespace
	options.Timeout = timeout
	options.ChartsDir = chartsDir
	
	// Set image prefix (required for validation)
	options.ImagePrefix = u.getEnvVar("IMAGE_PREFIX")
	if options.ImagePrefix == "" {
		return fmt.Errorf("IMAGE_PREFIX environment variable is required")
	}
	
	// Set tag base if provided
	options.TagBase = u.getEnvVar("TAG_BASE")
	
	if chartName != "" {
		// Update specific chart
		if err := u.updateSpecificChart(ctx, chartName, options); err != nil {
			colors.PrintError(fmt.Sprintf("Chart update failed: %v", err))
			return err
		}
	} else {
		// Update all charts
		if err := u.updateAllCharts(ctx, options); err != nil {
			colors.PrintError(fmt.Sprintf("Update failed: %v", err))
			return err
		}
	}
	
	colors.PrintSuccess("Update completed successfully")
	return nil
}

// updateSpecificChart updates a specific chart
func (u *UpdateCommand) updateSpecificChart(ctx context.Context, chartName string, options *domain.DeploymentOptions) error {
	colors.PrintStep(fmt.Sprintf("Updating chart: %s", chartName))
	
	// Get chart configuration
	chartConfig := domain.NewChartConfig(options.ChartsDir)
	chart, err := chartConfig.GetChart(chartName)
	if err != nil {
		return fmt.Errorf("chart not found: %w", err)
	}
	
	// Deploy the chart with force update
	helmGateway := u.createHelmGateway()
	if err := helmGateway.DeployChart(ctx, *chart, options); err != nil {
		return fmt.Errorf("failed to update chart %s: %w", chartName, err)
	}
	
	colors.PrintSuccess(fmt.Sprintf("Chart %s updated successfully", chartName))
	return nil
}

// updateAllCharts updates all charts
func (u *UpdateCommand) updateAllCharts(ctx context.Context, options *domain.DeploymentOptions) error {
	colors.PrintStep("Updating all charts with force update")
	
	// Use the deployment usecase to deploy all charts
	progress, err := u.usecase.Deploy(ctx, options)
	if err != nil {
		return fmt.Errorf("chart updates failed: %w", err)
	}
	
	// Print results
	colors.PrintSuccess(fmt.Sprintf("Update completed: %d successful, %d failed, %d skipped", 
		progress.GetSuccessCount(), progress.GetFailedCount(), progress.GetSkippedCount()))
	
	return nil
}

// createDeploymentUsecase creates the deployment usecase with all dependencies
func (u *UpdateCommand) createDeploymentUsecase() *deployment_usecase.DeploymentUsecase {
	// Create drivers
	systemDriver := system_driver.NewSystemDriver()
	helmDriver := helm_driver.NewHelmDriver()
	kubectlDriver := kubectl_driver.NewKubectlDriver()
	filesystemDriver := filesystem_driver.NewFileSystemDriver()
	
	// Create logger port adapter
	loggerPort := NewLoggerPortAdapter(u.logger)
	
	// Create gateways
	systemGateway := system_gateway.NewSystemGateway(systemDriver, loggerPort)
	helmGateway := helm_gateway.NewHelmGateway(helmDriver, loggerPort)
	kubectlGateway := kubectl_gateway.NewKubectlGateway(kubectlDriver, loggerPort)
	filesystemGateway := filesystem_gateway.NewFileSystemGateway(filesystemDriver, loggerPort)
	
	// Create usecase
	// Create secret usecase
	secretUsecase := secret_usecase.NewSecretUsecase(kubectlGateway, loggerPort)
	
	// Create SSL certificate usecase
	sslUsecase := secret_usecase.NewSSLCertificateUsecase(secretUsecase, loggerPort)
	
	return deployment_usecase.NewDeploymentUsecase(
		helmGateway,
		kubectlGateway,
		filesystemGateway,
		systemGateway,
		secretUsecase,
		sslUsecase,
		loggerPort,
		filesystemDriver,
	)
}

// createHelmGateway creates a helm gateway
func (u *UpdateCommand) createHelmGateway() *helm_gateway.HelmGateway {
	helmDriver := helm_driver.NewHelmDriver()
	loggerPort := NewLoggerPortAdapter(u.logger)
	return helm_gateway.NewHelmGateway(helmDriver, loggerPort)
}

// getEnvVar gets an environment variable
func (u *UpdateCommand) getEnvVar(key string) string {
	systemDriver := system_driver.NewSystemDriver()
	return systemDriver.GetEnvironmentVariable(key)
}