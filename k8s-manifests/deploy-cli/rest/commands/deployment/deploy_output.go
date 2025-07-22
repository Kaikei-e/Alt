// PHASE R3: Deployment command output formatting and display
package deployment

import (
	"fmt"
	"time"

	"deploy-cli/domain"
	"deploy-cli/rest/commands/shared"
	"deploy-cli/utils/colors"
)

// DeployOutput handles output formatting and display for deployment commands
type DeployOutput struct {
	shared *shared.CommandShared
}

// NewDeployOutput creates a new deployment output handler
func NewDeployOutput(shared *shared.CommandShared) *DeployOutput {
	return &DeployOutput{
		shared: shared,
	}
}

// PrintDeploymentStart prints the deployment start message
func (o *DeployOutput) PrintDeploymentStart() {
	colors.PrintInfo("Starting OSS-optimized deployment workflow")
}

// PrintDeploymentConfiguration prints the deployment configuration
func (o *DeployOutput) PrintDeploymentConfiguration(options *domain.DeploymentOptions) {
	o.shared.Logger.InfoWithContext("deployment configuration", map[string]interface{}{
		"environment":  options.Environment,
		"charts_dir":   options.ChartsDir,
		"namespace":    options.TargetNamespace,
		"force_update": options.ForceUpdate,
		"dry_run":      options.DryRun,
		"timeout":      options.Timeout.String(),
	})

	// Print emergency mode warning if applicable
	if o.isEmergencyMode(options) {
		colors.PrintWarning("🚨 EMERGENCY MODE ACTIVATED")
		o.printEmergencyModeDetails(options)
	}

	// Print health check skip warning if applicable
	if options.SkipHealthChecks {
		colors.PrintWarning("⚠️ HEALTH CHECKS DISABLED - Deployment will proceed without waiting for service readiness")
	}

	// Print auto-fix features if enabled
	if o.hasAutoFixFeatures(options) {
		o.printAutoFixFeatures(options)
	}
}

// PrintDeploymentResults prints detailed deployment results
func (o *DeployOutput) PrintDeploymentResults(result *domain.DeploymentProgress, duration time.Duration) {
	colors.PrintInfo("Deployment Summary")

	// Print summary statistics
	o.printSummaryStatistics(result, duration)

	// Print detailed results for each chart
	if len(result.Results) > 0 {
		colors.PrintInfo("Detailed Results")
		o.printDetailedResults(result.Results)
	}

	// Print warnings and recommendations
	o.printWarningsAndRecommendations(result)
}

// PrintCompletionMessage prints the appropriate completion message based on results
func (o *DeployOutput) PrintCompletionMessage(result *domain.DeploymentProgress, duration time.Duration) {
	successCount := result.GetSuccessCount()
	failedCount := result.GetFailedCount()
	skippedCount := result.GetSkippedCount()
	totalCount := result.TotalCharts

	// Handle edge cases
	if totalCount == 0 {
		colors.PrintWarning("No charts found to deploy")
		return
	}

	if successCount == 0 && failedCount == 0 && skippedCount == totalCount {
		colors.PrintWarning(fmt.Sprintf("All %d charts were skipped - no deployment performed in %s", 
			totalCount, duration))
		return
	}

	// Print completion message based on results
	switch {
	case failedCount == 0 && successCount > 0:
		o.printSuccessMessage(successCount, duration)
	case failedCount > 0 && successCount > 0:
		o.printMixedResultsMessage(successCount, failedCount, skippedCount, duration)
	case failedCount > 0 && successCount == 0:
		o.printFailureMessage(failedCount, skippedCount, duration)
	}
}

// PrintDeploymentError prints deployment error information
func (o *DeployOutput) PrintDeploymentError(err error) {
	colors.PrintError(fmt.Sprintf("Deployment failed: %v", err))
	
	// Log detailed error information
	o.shared.Logger.ErrorWithContext("deployment execution failed", map[string]interface{}{
		"error":       err.Error(),
		"error_type":  fmt.Sprintf("%T", err),
	})
}

// Private helper methods

// printSummaryStatistics prints the summary statistics box
func (o *DeployOutput) printSummaryStatistics(result *domain.DeploymentProgress, duration time.Duration) {
	fmt.Printf("  Total Charts: %d\n", result.TotalCharts)
	fmt.Printf("  Successful: %s\n", colors.Green(fmt.Sprintf("%d", result.GetSuccessCount())))
	fmt.Printf("  Failed: %s\n", colors.Red(fmt.Sprintf("%d", result.GetFailedCount())))
	fmt.Printf("  Skipped: %s\n", colors.Yellow(fmt.Sprintf("%d", result.GetSkippedCount())))
	fmt.Printf("  Duration: %s\n", colors.Cyan(duration.String()))

	// Add deployment rate information
	if duration > 0 && result.TotalCharts > 0 {
		rate := float64(result.TotalCharts) / duration.Minutes()
		fmt.Printf("  Deployment Rate: %s charts/min\n", colors.Cyan(fmt.Sprintf("%.2f", rate)))
	}
}

// printDetailedResults prints detailed results for each chart
func (o *DeployOutput) printDetailedResults(results []domain.ChartDeploymentResult) {
	for _, r := range results {
		status := o.getStatusIcon(r.Status)
		
		fmt.Printf("  %s %s → %s (%s)",
			status,
			r.ChartName,
			r.Namespace,
			r.Duration)

		// Add additional context for the result
		if r.Status == domain.DeploymentStatusSuccess {
			if r.IsUpgrade {
				fmt.Printf(" %s", colors.Blue("(upgraded)"))
			} else {
				fmt.Printf(" %s", colors.Green("(installed)"))
			}
		}
		
		fmt.Println()

		// Print error details if any
		if r.Error != nil {
			fmt.Printf("    Error: %s\n", colors.Red(r.Error.Error()))
		}

		// Print warnings if any
		if len(r.Warnings) > 0 {
			for _, warning := range r.Warnings {
				fmt.Printf("    Warning: %s\n", colors.Yellow(warning))
			}
		}
	}
}

// printWarningsAndRecommendations prints warnings and recommendations
func (o *DeployOutput) printWarningsAndRecommendations(result *domain.DeploymentProgress) {
	// Print recommendations for failed deployments
	if result.GetFailedCount() > 0 {
		colors.PrintInfo("Troubleshooting Recommendations")
		fmt.Println("  • Check the error details above")
		fmt.Println("  • Verify cluster resource availability")
		fmt.Println("  • Consider using --emergency-mode for critical issues")
		fmt.Println("  • Use --diagnostic-report for detailed analysis")
	}

	// Print performance recommendations for slow deployments
	if len(result.Results) > 0 {
		slowCharts := o.findSlowCharts(result.Results, 5*time.Minute)
		if len(slowCharts) > 0 {
			colors.PrintInfo("Performance Recommendations")
			fmt.Printf("  • %d charts took longer than 5 minutes to deploy\n", len(slowCharts))
			fmt.Println("  • Consider optimizing resource requests/limits")
			fmt.Println("  • Check for network or storage bottlenecks")
		}
	}
}

// printSuccessMessage prints success completion message
func (o *DeployOutput) printSuccessMessage(successCount int, duration time.Duration) {
	colors.PrintSuccess(fmt.Sprintf("OSS-optimized deployment completed successfully in %s (%d charts deployed)", 
		duration, successCount))
}

// printMixedResultsMessage prints mixed results completion message
func (o *DeployOutput) printMixedResultsMessage(successCount, failedCount, skippedCount int, duration time.Duration) {
	colors.PrintWarning(fmt.Sprintf("Deployment completed with mixed results in %s (%d successful, %d failed, %d skipped)", 
		duration, successCount, failedCount, skippedCount))
}

// printFailureMessage prints failure completion message
func (o *DeployOutput) printFailureMessage(failedCount, skippedCount int, duration time.Duration) {
	colors.PrintError(fmt.Sprintf("Deployment failed in %s (%d charts failed, %d skipped)", 
		duration, failedCount, skippedCount))
}

// printEmergencyModeDetails prints emergency mode configuration details
func (o *DeployOutput) printEmergencyModeDetails(options *domain.DeploymentOptions) {
	fmt.Println("  Emergency mode configuration:")
	fmt.Printf("    • Skip StatefulSet recovery: %s\n", colors.BoolColor(options.SkipStatefulSetRecovery))
	fmt.Printf("    • Skip health checks: %s\n", colors.BoolColor(options.SkipHealthChecks))
	fmt.Printf("    • Auto-fix secrets: %s\n", colors.BoolColor(options.AutoFixSecrets))
	fmt.Printf("    • Emergency timeout: %s\n", colors.Cyan(options.Timeout.String()))
}

// printAutoFixFeatures prints enabled auto-fix features
func (o *DeployOutput) printAutoFixFeatures(options *domain.DeploymentOptions) {
	colors.PrintInfo("Auto-fix features enabled:")
	
	if options.AutoFixSecrets {
		fmt.Println("  ✓ Automatic secret conflict resolution")
	}
	if options.AutoCreateNamespaces {
		fmt.Println("  ✓ Automatic namespace creation")
	}
	if options.AutoFixStorage {
		fmt.Println("  ✓ Automatic storage configuration")
	}
}

// getStatusIcon returns the appropriate status icon for a deployment result
func (o *DeployOutput) getStatusIcon(status domain.DeploymentStatus) string {
	switch status {
	case domain.DeploymentStatusSuccess:
		return colors.Green("✓")
	case domain.DeploymentStatusFailed:
		return colors.Red("✗")
	case domain.DeploymentStatusSkipped:
		return colors.Yellow("⚠")
	case domain.DeploymentStatusInProgress:
		return colors.Blue("⟳")
	default:
		return colors.Gray("?")
	}
}

// isEmergencyMode determines if emergency mode is enabled
func (o *DeployOutput) isEmergencyMode(options *domain.DeploymentOptions) bool {
	return options.SkipHealthChecks && 
		   options.SkipStatefulSetRecovery && 
		   options.Timeout <= 5*time.Minute
}

// hasAutoFixFeatures checks if any auto-fix features are enabled
func (o *DeployOutput) hasAutoFixFeatures(options *domain.DeploymentOptions) bool {
	return options.AutoFixSecrets || 
		   options.AutoCreateNamespaces || 
		   options.AutoFixStorage
}

// findSlowCharts finds charts that took longer than the specified threshold to deploy
func (o *DeployOutput) findSlowCharts(results []domain.ChartDeploymentResult, threshold time.Duration) []domain.ChartDeploymentResult {
	var slowCharts []domain.ChartDeploymentResult
	
	for _, result := range results {
		if result.Duration > threshold {
			slowCharts = append(slowCharts, result)
		}
	}
	
	return slowCharts
}

// PrintDryRunResults prints dry-run specific results
func (o *DeployOutput) PrintDryRunResults(result *domain.DeploymentProgress) {
	colors.PrintInfo("Dry-run Results")
	fmt.Println("  The following changes would be applied:")
	
	for _, r := range result.Results {
		if r.Status == domain.DeploymentStatusSuccess {
			fmt.Printf("  ✓ %s would be deployed to %s\n", 
				colors.Green(r.ChartName), 
				colors.Cyan(r.Namespace))
		}
	}
	
	colors.PrintWarning("This was a dry-run. No actual changes were made.")
}