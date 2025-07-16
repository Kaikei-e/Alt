package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	
	"deploy-cli/domain"
	"deploy-cli/utils/logger"
	"deploy-cli/utils/colors"
	"deploy-cli/usecase/deployment_usecase"
	"deploy-cli/driver/helm_driver"
	"deploy-cli/driver/kubectl_driver"
	"deploy-cli/driver/filesystem_driver"
	"deploy-cli/driver/system_driver"
	"deploy-cli/gateway/helm_gateway"
	"deploy-cli/gateway/kubectl_gateway"
	"deploy-cli/gateway/filesystem_gateway"
	"deploy-cli/gateway/system_gateway"
)

// ValidateManifestsCommand represents the validate-manifests command
type ValidateManifestsCommand struct {
	logger  *logger.Logger
	usecase *deployment_usecase.DeploymentUsecase
}

// NewValidateManifestsCommand creates a new validate-manifests command
func NewValidateManifestsCommand(logger *logger.Logger) *cobra.Command {
	validateCmd := &ValidateManifestsCommand{
		logger: logger,
	}
	
	cmd := &cobra.Command{
		Use:   "validate-manifests <environment>",
		Short: "Validate Helm manifests for deployment",
		Long: `Validate Helm manifests for the specified environment.
		
This command checks:
- Helm chart templates can be rendered
- Generated manifests are valid YAML
- All required values are present
- Resource definitions are complete

Examples:
  # Validate production manifests
  deploy-cli validate-manifests production

  # Validate with custom charts directory
  deploy-cli validate-manifests production --charts-dir /path/to/charts`,
		Args:    cobra.ExactArgs(1),
		PreRunE: validateCmd.preRun,
		RunE:    validateCmd.run,
	}
	
	// Add flags
	cmd.Flags().String("charts-dir", "../charts", "Directory containing Helm charts")
	cmd.Flags().Bool("show-manifests", false, "Show generated manifests for each chart")
	
	return cmd
}

// preRun performs pre-execution setup
func (v *ValidateManifestsCommand) preRun(cmd *cobra.Command, args []string) error {
	v.logger.InfoWithContext("initializing validate-manifests command", "environment", args[0])
	
	// Create dependencies
	v.usecase = v.createDeploymentUsecase()
	
	return nil
}

// run executes the validation
func (v *ValidateManifestsCommand) run(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	
	colors.PrintInfo("Starting Helm manifest validation")
	
	// Parse environment
	env, err := domain.ParseEnvironment(args[0])
	if err != nil {
		return fmt.Errorf("invalid environment: %w", err)
	}
	
	// Get flags
	chartsDir, _ := cmd.Flags().GetString("charts-dir")
	showManifests, _ := cmd.Flags().GetBool("show-manifests")
	
	// Create deployment options
	options := domain.NewDeploymentOptions()
	options.Environment = env
	options.ChartsDir = chartsDir
	options.DryRun = true // Always dry run for validation
	
	// Validate manifests
	if err := v.validateManifests(ctx, options, showManifests); err != nil {
		colors.PrintError(fmt.Sprintf("Manifest validation failed: %v", err))
		return err
	}
	
	colors.PrintSuccess("All Helm manifests validated successfully")
	return nil
}

// validateManifests validates helm manifests
func (v *ValidateManifestsCommand) validateManifests(ctx context.Context, options *domain.DeploymentOptions, showManifests bool) error {
	colors.PrintStep("Validating Helm manifests")
	
	// Get chart configuration
	chartConfig := domain.NewChartConfig(options.ChartsDir)
	allCharts := chartConfig.AllCharts()
	
	helmDriver := helm_driver.NewHelmDriver()
	loggerPort := NewLoggerPortAdapter(v.logger)
	helmGateway := helm_gateway.NewHelmGateway(helmDriver, loggerPort)
	
	var validationErrors []string
	successCount := 0
	
	for _, chart := range allCharts {
		colors.PrintProgress(fmt.Sprintf("Validating manifest: %s", chart.Name))
		
		// Try to template the chart
		manifest, err := helmGateway.TemplateChart(ctx, chart, options)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Sprintf("Chart %s: %v", chart.Name, err))
			continue
		}
		
		// Check if manifest is empty
		if strings.TrimSpace(manifest) == "" {
			validationErrors = append(validationErrors, fmt.Sprintf("Chart %s: generated manifest is empty", chart.Name))
			continue
		}
		
		// Count resources in manifest
		resourceCount := strings.Count(manifest, "---")
		if resourceCount == 0 {
			resourceCount = 1 // Single resource without --- separator
		}
		
		colors.PrintSubInfo(fmt.Sprintf("Chart %s: %d resources generated", chart.Name, resourceCount))
		
		// Show manifest if requested
		if showManifests {
			fmt.Printf("\n--- Chart: %s ---\n", chart.Name)
			fmt.Println(manifest)
			fmt.Println("--- End of Chart ---\n")
		}
		
		successCount++
	}
	
	if len(validationErrors) > 0 {
		return fmt.Errorf("manifest validation failed:\n%s", strings.Join(validationErrors, "\n"))
	}
	
	colors.PrintSubInfo(fmt.Sprintf("All %d manifests validated successfully", successCount))
	return nil
}

// createDeploymentUsecase creates the deployment usecase with all dependencies
func (v *ValidateManifestsCommand) createDeploymentUsecase() *deployment_usecase.DeploymentUsecase {
	// Create drivers
	systemDriver := system_driver.NewSystemDriver()
	helmDriver := helm_driver.NewHelmDriver()
	kubectlDriver := kubectl_driver.NewKubectlDriver()
	filesystemDriver := filesystem_driver.NewFileSystemDriver()
	
	// Create logger port adapter
	loggerPort := NewLoggerPortAdapter(v.logger)
	
	// Create gateways
	systemGateway := system_gateway.NewSystemGateway(systemDriver, loggerPort)
	helmGateway := helm_gateway.NewHelmGateway(helmDriver, loggerPort)
	kubectlGateway := kubectl_gateway.NewKubectlGateway(kubectlDriver, loggerPort)
	filesystemGateway := filesystem_gateway.NewFileSystemGateway(filesystemDriver, loggerPort)
	
	// Create usecase
	return deployment_usecase.NewDeploymentUsecase(
		helmGateway,
		kubectlGateway,
		filesystemGateway,
		systemGateway,
		loggerPort,
	)
}