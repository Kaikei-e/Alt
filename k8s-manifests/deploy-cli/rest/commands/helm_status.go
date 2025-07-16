package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	
	"deploy-cli/domain"
	"deploy-cli/utils/logger"
	"deploy-cli/utils/colors"
	"deploy-cli/driver/helm_driver"
	"deploy-cli/driver/kubectl_driver"
	"deploy-cli/gateway/helm_gateway"
	"deploy-cli/gateway/kubectl_gateway"
)

// HelmStatusCommand represents the helm-status command
type HelmStatusCommand struct {
	logger *logger.Logger
}

// NewHelmStatusCommand creates a new helm-status command
func NewHelmStatusCommand(logger *logger.Logger) *cobra.Command {
	statusCmd := &HelmStatusCommand{
		logger: logger,
	}
	
	cmd := &cobra.Command{
		Use:   "helm-status [environment]",
		Short: "Check Helm release status for deployment",
		Long: `Check the status of Helm releases for the specified environment.
		
This command checks:
- Helm release status (deployed, failed, pending, etc.)
- Kubernetes resources created by each release
- Pod status for application releases
- Recent revision history

Examples:
  # Check status for all releases in production
  deploy-cli helm-status production

  # Check status for specific chart
  deploy-cli helm-status production --chart alt-frontend
  
  # Show detailed pod information
  deploy-cli helm-status production --pods`,
		Args:    cobra.MaximumNArgs(1),
		RunE:    statusCmd.run,
	}
	
	// Add flags
	cmd.Flags().String("chart", "", "Check status for specific chart only")
	cmd.Flags().Bool("pods", false, "Show detailed pod information")
	cmd.Flags().Bool("history", false, "Show release history")
	cmd.Flags().String("charts-dir", "/home/koko/Documents/dev/Alt/charts", "Directory containing Helm charts")
	
	return cmd
}

// run executes the helm status check
func (h *HelmStatusCommand) run(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	
	colors.PrintInfo("Checking Helm release status")
	
	// Parse environment (default to production if not specified)
	env := domain.Production
	if len(args) > 0 {
		var err error
		env, err = domain.ParseEnvironment(args[0])
		if err != nil {
			return fmt.Errorf("invalid environment: %w", err)
		}
	}
	
	// Get flags
	chartName, _ := cmd.Flags().GetString("chart")
	showPods, _ := cmd.Flags().GetBool("pods")
	showHistory, _ := cmd.Flags().GetBool("history")
	chartsDir, _ := cmd.Flags().GetString("charts-dir")
	
	// Create drivers and gateways
	helmDriver := helm_driver.NewHelmDriver()
	kubectlDriver := kubectl_driver.NewKubectlDriver()
	loggerPort := NewLoggerPortAdapter(h.logger)
	helmGateway := helm_gateway.NewHelmGateway(helmDriver, loggerPort)
	kubectlGateway := kubectl_gateway.NewKubectlGateway(kubectlDriver, loggerPort)
	
	// Get charts to check
	var charts []domain.Chart
	if chartName != "" {
		chartConfig := domain.NewChartConfig(chartsDir)
		chart, err := chartConfig.GetChart(chartName)
		if err != nil {
			return fmt.Errorf("chart not found: %w", err)
		}
		charts = []domain.Chart{*chart}
	} else {
		chartConfig := domain.NewChartConfig(chartsDir)
		charts = chartConfig.AllCharts()
	}
	
	// Create deployment options
	options := domain.NewDeploymentOptions()
	options.Environment = env
	options.ChartsDir = chartsDir
	
	// Check status for each chart
	var successCount, failedCount int
	
	for _, chart := range charts {
		colors.PrintStep(fmt.Sprintf("Checking chart: %s", chart.Name))
		
		// Check Helm release status
		releaseStatus, err := h.checkHelmReleaseStatus(ctx, helmGateway, chart, options)
		if err != nil {
			colors.PrintError(fmt.Sprintf("Failed to check release status: %v", err))
			failedCount++
			continue
		}
		
		if releaseStatus != "" {
			colors.PrintSubInfo(fmt.Sprintf("Release Status: %s", releaseStatus))
			
			// Show history if requested
			if showHistory {
				history, err := h.getHelmHistory(ctx, helmGateway, chart, options)
				if err != nil {
					colors.PrintWarning(fmt.Sprintf("Failed to get history: %v", err))
				} else if history != "" {
					colors.PrintSubInfo("Release History:")
					fmt.Println(history)
				}
			}
			
			// Check pod status if requested and it's an application chart
			if showPods && chart.Type == domain.ApplicationChart {
				podStatus, err := h.checkPodStatus(ctx, kubectlGateway, chart, options)
				if err != nil {
					colors.PrintWarning(fmt.Sprintf("Failed to check pod status: %v", err))
				} else if podStatus != "" {
					colors.PrintSubInfo("Pod Status:")
					fmt.Println(podStatus)
				}
			}
		} else {
			colors.PrintWarning(fmt.Sprintf("Release not found: %s", chart.Name))
		}
		
		successCount++
	}
	
	// Print summary
	colors.PrintSuccess(fmt.Sprintf("Status check completed: %d charts checked, %d failed", successCount, failedCount))
	
	return nil
}

// checkHelmReleaseStatus checks the status of a Helm release
func (h *HelmStatusCommand) checkHelmReleaseStatus(ctx context.Context, helmGateway *helm_gateway.HelmGateway, chart domain.Chart, options *domain.DeploymentOptions) (string, error) {
	// Use helm status command
	namespace := options.GetNamespace(chart.Name)
	status, err := helmGateway.GetReleaseStatus(ctx, chart.Name, namespace)
	if err != nil {
		return "", fmt.Errorf("failed to get release status: %w", err)
	}
	
	return status.Status, nil
}

// getHelmHistory gets the release history
func (h *HelmStatusCommand) getHelmHistory(ctx context.Context, helmGateway *helm_gateway.HelmGateway, chart domain.Chart, options *domain.DeploymentOptions) (string, error) {
	// Use helm history command
	history, err := helmGateway.GetReleaseHistory(ctx, chart, options)
	if err != nil {
		return "", fmt.Errorf("failed to get release history: %w", err)
	}
	
	return history, nil
}

// checkPodStatus checks the status of pods for an application chart
func (h *HelmStatusCommand) checkPodStatus(ctx context.Context, kubectlGateway *kubectl_gateway.KubectlGateway, chart domain.Chart, options *domain.DeploymentOptions) (string, error) {
	// Get namespace for the chart
	namespace := options.GetNamespace(chart.Name)
	
	// Get pod status
	pods, err := kubectlGateway.GetPods(ctx, namespace, fmt.Sprintf("app.kubernetes.io/name=%s", chart.Name))
	if err != nil {
		return "", fmt.Errorf("failed to get pods: %w", err)
	}
	
	return pods, nil
}