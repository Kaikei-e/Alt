// PHASE R3: Monitoring output formatting and display
package monitoring

import (
	"fmt"
	"strings"
	"time"

	"deploy-cli/domain"
	"deploy-cli/rest/commands/shared"
	"deploy-cli/utils/colors"
)

// MonitoringOutput handles output formatting for monitoring commands
type MonitoringOutput struct {
	shared *shared.CommandShared
}

// NewMonitoringOutput creates a new monitoring output handler
func NewMonitoringOutput(shared *shared.CommandShared) *MonitoringOutput {
	return &MonitoringOutput{
		shared: shared,
	}
}

// Dashboard output methods

// PrintDashboardStart prints dashboard startup message
func (o *MonitoringOutput) PrintDashboardStart(env domain.Environment, options *DashboardOptions) {
	fmt.Printf("%s Starting monitoring dashboard for %s...\n",
		colors.Blue("📊"), colors.Cyan(env.String()))
	
	if options.Filter != "" {
		fmt.Printf("   Filter: %s\n", colors.Yellow(options.Filter))
	}
	if options.Compact {
		fmt.Printf("   Mode: %s\n", colors.Green("Compact"))
	}
	if options.RefreshInterval > 0 {
		fmt.Printf("   Refresh: %s\n", colors.Cyan(options.RefreshInterval.String()))
	}
	
	fmt.Printf("Press Ctrl+C to stop monitoring\n\n")
}

// DisplayDashboard displays the main dashboard interface
func (o *MonitoringOutput) DisplayDashboard(dashboard *DashboardState, overview *ClusterOverview, services []ServiceStatus, metrics *MetricsSnapshot) {
	// Clear screen for refresh (in real implementation)
	fmt.Printf("\n" + strings.Repeat("=", 80) + "\n")
	
	// Header
	o.printDashboardHeader(dashboard, overview)
	
	// Services status table
	o.printServicesTable(services, dashboard.CompactMode)
	
	// Metrics section if enabled
	if dashboard.ShowMetrics && metrics != nil {
		o.printMetricsSection(metrics)
	}
	
	// Footer with controls
	o.printDashboardFooter(dashboard)
}

// PrintDashboardStop prints dashboard stop message
func (o *MonitoringOutput) PrintDashboardStop() {
	fmt.Printf("\n%s Dashboard monitoring stopped.\n", colors.Blue("📊"))
}

// Services monitoring output methods

// PrintServicesMonitoringStart prints services monitoring startup message
func (o *MonitoringOutput) PrintServicesMonitoringStart(services []string, env domain.Environment, options *ServicesOptions) {
	if len(services) == 0 {
		fmt.Printf("%s Monitoring all services in %s...\n",
			colors.Blue("🔍"), colors.Cyan(env.String()))
	} else {
		fmt.Printf("%s Monitoring services [%s] in %s...\n",
			colors.Blue("🔍"), 
			colors.Yellow(strings.Join(services, ", ")), 
			colors.Cyan(env.String()))
	}
	
	// Print monitoring options
	features := make([]string, 0)
	if options.Metrics {
		features = append(features, "metrics")
	}
	if options.Logs {
		features = append(features, "logs")
	}
	if options.Follow {
		features = append(features, "follow")
	}
	if len(features) > 0 {
		fmt.Printf("   Features: %s\n", colors.Green(strings.Join(features, ", ")))
	}
	
	fmt.Printf("\n")
}

// DisplayServicesStatus displays services status information
func (o *MonitoringOutput) DisplayServicesStatus(monitoring *ServicesMonitoring, servicesStatus []ServiceStatus) {
	fmt.Printf("\r" + strings.Repeat(" ", 80) + "\r") // Clear line
	fmt.Printf("%s Services Status - %s\n", 
		colors.Blue("📋"), 
		colors.Gray(monitoring.LastUpdate.Format("15:04:05")))
	
	// Services table
	o.printServicesDetailTable(servicesStatus, monitoring.Options.Details)
	
	if monitoring.Options.Watch {
		fmt.Printf("\nPress Ctrl+C to stop monitoring...\r")
	}
}

// PrintServicesMonitoringStop prints services monitoring stop message
func (o *MonitoringOutput) PrintServicesMonitoringStop() {
	fmt.Printf("\n%s Services monitoring stopped.\n", colors.Blue("🔍"))
}

// Metrics output methods

// PrintMetricsCollectionStart prints metrics collection startup message
func (o *MonitoringOutput) PrintMetricsCollectionStart(collection *MetricsCollection) {
	fmt.Printf("%s Starting metrics collection for %s...\n",
		colors.Blue("📈"), colors.Cyan(collection.Environment.String()))
	fmt.Printf("   Duration: %s\n", colors.Cyan(collection.Duration.String()))
	fmt.Printf("   Interval: %s\n", colors.Cyan(collection.Interval.String()))
	
	if len(collection.Focus) > 0 {
		fmt.Printf("   Focus: %s\n", colors.Yellow(strings.Join(collection.Focus, ", ")))
	}
	
	fmt.Printf("\n")
}

// DisplayMetricsProgress displays metrics collection progress
func (o *MonitoringOutput) DisplayMetricsProgress(collection *MetricsCollection, sample *MetricsSample) {
	elapsed := time.Since(collection.StartTime)
	progress := float64(elapsed) / float64(collection.Duration) * 100
	
	fmt.Printf("\r%s Progress: %.1f%% - %s", 
		colors.Blue("📈"), 
		progress,
		colors.Gray(sample.Timestamp.Format("15:04:05")))
}

// PrintMetricsCollectionComplete prints metrics collection completion message
func (o *MonitoringOutput) PrintMetricsCollectionComplete(collection *MetricsCollection) {
	fmt.Printf("\n%s Metrics collection completed in %s\n", 
		colors.Green("✓"), 
		colors.Cyan(time.Since(collection.StartTime).String()))
	
	if collection.OutputPath != "" {
		fmt.Printf("   Report saved to: %s\n", colors.Cyan(collection.OutputPath))
	}
}

// Private helper methods

// printDashboardHeader prints the dashboard header section
func (o *MonitoringOutput) printDashboardHeader(dashboard *DashboardState, overview *ClusterOverview) {
	fmt.Printf("%s Alt RSS Reader - Monitoring Dashboard\n", colors.Blue("📊"))
	fmt.Printf("Environment: %s | Last Update: %s\n\n",
		colors.Cyan(dashboard.Environment.String()),
		colors.Gray(dashboard.LastUpdate.Format("15:04:05")))
	
	// Cluster overview
	fmt.Printf("Cluster Overview:\n")
	fmt.Printf("  Nodes: %s/%d ready",
		o.colorizeCount(overview.NodesReady, overview.NodesCount),
		overview.NodesCount)
	fmt.Printf("  |  Pods: %s/%d running",
		o.colorizeCount(overview.PodsRunning, overview.PodsTotal),
		overview.PodsTotal)
	if overview.PodsPending > 0 {
		fmt.Printf("  |  Pending: %s", colors.Yellow(fmt.Sprintf("%d", overview.PodsPending)))
	}
	if overview.PodsFailed > 0 {
		fmt.Printf("  |  Failed: %s", colors.Red(fmt.Sprintf("%d", overview.PodsFailed)))
	}
	fmt.Printf("\n\n")
}

// printServicesTable prints the services status table
func (o *MonitoringOutput) printServicesTable(services []ServiceStatus, compact bool) {
	if len(services) == 0 {
		fmt.Printf("No services found\n")
		return
	}

	fmt.Printf("Services Status:\n")
	if compact {
		o.printCompactServicesTable(services)
	} else {
		o.printDetailedServicesTable(services)
	}
	fmt.Printf("\n")
}

// printCompactServicesTable prints a compact services table
func (o *MonitoringOutput) printCompactServicesTable(services []ServiceStatus) {
	for _, service := range services {
		status := o.colorizeStatus(service.Status)
		fmt.Printf("  %s %-20s %s %s\n", 
			o.getStatusIcon(service.Status),
			service.Name, 
			status,
			colors.Gray(service.Age))
	}
}

// printDetailedServicesTable prints a detailed services table
func (o *MonitoringOutput) printDetailedServicesTable(services []ServiceStatus) {
	fmt.Printf("  %-20s %-10s %-10s %-8s\n", "Name", "Status", "Pods", "Age")
	fmt.Printf("  %s\n", strings.Repeat("-", 50))
	
	for _, service := range services {
		status := o.colorizeStatus(service.Status)
		pods := o.colorizePods(service.Pods)
		fmt.Printf("  %-20s %-10s %-10s %-8s\n",
			service.Name, status, pods, service.Age)
	}
}

// printServicesDetailTable prints detailed services table for services monitoring
func (o *MonitoringOutput) printServicesDetailTable(services []ServiceStatus, showDetails bool) {
	if len(services) == 0 {
		fmt.Printf("No services found\n")
		return
	}

	if showDetails {
		// Detailed table with more information
		fmt.Printf("  %-20s %-10s %-10s %-8s %-12s\n", 
			"Name", "Status", "Pods", "Age", "Ready")
		fmt.Printf("  %s\n", strings.Repeat("-", 70))
		
		for _, service := range services {
			status := o.colorizeStatus(service.Status)
			pods := o.colorizePods(service.Pods)
			ready := o.colorizeReady(service.Ready)
			fmt.Printf("  %-20s %-10s %-10s %-8s %-12s\n",
				service.Name, status, pods, service.Age, ready)
		}
	} else {
		// Simple table
		o.printDetailedServicesTable(services)
	}
}

// printMetricsSection prints the metrics section
func (o *MonitoringOutput) printMetricsSection(metrics *MetricsSnapshot) {
	fmt.Printf("Resource Metrics:\n")
	fmt.Printf("  CPU: %s%%", o.colorizePercentage(metrics.CPUUsage))
	fmt.Printf("  |  Memory: %s%%", o.colorizePercentage(metrics.MemoryUsage))
	fmt.Printf("  |  Disk: %s%%", o.colorizePercentage(metrics.DiskUsage))
	fmt.Printf("  |  Network: %s MB/s\n", colors.Cyan(fmt.Sprintf("%.1f", metrics.NetworkIO)))
	fmt.Printf("\n")
}

// printDashboardFooter prints the dashboard footer with controls
func (o *MonitoringOutput) printDashboardFooter(dashboard *DashboardState) {
	fmt.Printf("%s\n", strings.Repeat("-", 80))
	if dashboard.Interactive {
		fmt.Printf("Controls: [q]uit | [r]efresh | [h]elp | [t]roubleshooting\n")
	} else {
		fmt.Printf("Press Ctrl+C to stop monitoring\n")
	}
}

// Color helper methods

// colorizeStatus returns colored status text
func (o *MonitoringOutput) colorizeStatus(status string) string {
	switch strings.ToLower(status) {
	case "running":
		return colors.Green(status)
	case "pending":
		return colors.Yellow(status)
	case "failed", "error":
		return colors.Red(status)
	case "unknown":
		return colors.Gray(status)
	default:
		return status
	}
}

// colorizePods returns colored pods count text
func (o *MonitoringOutput) colorizePods(pods string) string {
	if strings.Contains(pods, "/") {
		parts := strings.Split(pods, "/")
		if len(parts) == 2 && parts[0] == parts[1] {
			return colors.Green(pods)
		} else {
			return colors.Yellow(pods)
		}
	}
	return pods
}

// colorizeReady returns colored ready status
func (o *MonitoringOutput) colorizeReady(ready bool) string {
	if ready {
		return colors.Green("Ready")
	}
	return colors.Red("Not Ready")
}

// colorizeCount returns colored count with status
func (o *MonitoringOutput) colorizeCount(current, total int) string {
	if current == total {
		return colors.Green(fmt.Sprintf("%d", current))
	} else if current == 0 {
		return colors.Red(fmt.Sprintf("%d", current))
	}
	return colors.Yellow(fmt.Sprintf("%d", current))
}

// colorizePercentage returns colored percentage based on value
func (o *MonitoringOutput) colorizePercentage(value float64) string {
	text := fmt.Sprintf("%.1f", value)
	if value >= 90 {
		return colors.Red(text)
	} else if value >= 75 {
		return colors.Yellow(text)
	}
	return colors.Green(text)
}

// getStatusIcon returns appropriate icon for status
func (o *MonitoringOutput) getStatusIcon(status string) string {
	switch strings.ToLower(status) {
	case "running":
		return colors.Green("●")
	case "pending":
		return colors.Yellow("●")
	case "failed", "error":
		return colors.Red("●")
	case "unknown":
		return colors.Gray("●")
	default:
		return "●"
	}
}