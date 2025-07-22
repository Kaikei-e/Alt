// PHASE R3: Monitoring command root with focused responsibility
package monitoring

import (
	"github.com/spf13/cobra"

	"deploy-cli/rest/commands/shared"
)

// MonitorCommand provides the root monitoring command with subcommands
type MonitorCommand struct {
	shared *shared.CommandShared
}

// NewMonitorCommand creates a new monitor command with organized subcommands
func NewMonitorCommand(shared *shared.CommandShared) *cobra.Command {
	monitorCmd := &MonitorCommand{
		shared: shared,
	}

	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Monitoring and observability tools for deployment health",
		Long: `Comprehensive monitoring and observability suite for Alt RSS Reader deployment.

This command suite provides real-time monitoring and observability features:
• Live deployment status and health monitoring
• Resource utilization tracking and alerts
• Performance metrics collection and analysis  
• Log aggregation and intelligent filtering
• Automated alerting for critical issues
• Historical trend analysis and reporting

Features:
• Real-time dashboard with color-coded status indicators
• Customizable monitoring intervals and alert thresholds
• Export capabilities for metrics and logs
• Integration with existing troubleshooting tools
• Environment-specific monitoring configurations
• Automated anomaly detection and recommendations

Available Commands:
  dashboard    Launch real-time monitoring dashboard
  services     Monitor specific services in real-time
  metrics      Collect and analyze performance metrics
  logs         Monitor and analyze service logs
  report       Generate monitoring reports
  alerts       Manage monitoring alerts and notifications

Examples:
  # Start real-time monitoring dashboard
  deploy-cli monitoring dashboard production

  # Monitor specific services
  deploy-cli monitoring services alt-backend,postgres production

  # Collect and analyze performance metrics
  deploy-cli monitoring metrics production --duration 1h

The monitoring tools help ensure optimal performance and quick issue detection
for your Alt RSS Reader deployment.`,
		PersistentPreRunE: shared.PersistentPreRunE,
	}

	// Add monitoring subcommands
	cmd.AddCommand(NewDashboardCommand(shared))
	cmd.AddCommand(NewServicesCommand(shared))
	cmd.AddCommand(NewMetricsCommand(shared))
	cmd.AddCommand(NewLogsCommand(shared))
	cmd.AddCommand(NewReportCommand(shared))
	cmd.AddCommand(NewAlertsCommand(shared))

	// Add monitoring-specific global flags
	monitorCmd.addMonitoringGlobalFlags(cmd)

	return cmd
}

// addMonitoringGlobalFlags adds monitoring-specific global flags
func (m *MonitorCommand) addMonitoringGlobalFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String("format", "table",
		"Output format for monitoring data (table, json, yaml)")
	cmd.PersistentFlags().Bool("no-headers", false,
		"Disable table headers in output")
	cmd.PersistentFlags().Bool("timestamps", true,
		"Include timestamps in monitoring output")
	cmd.PersistentFlags().Duration("timeout", 0,
		"Timeout for monitoring operations (0 = no timeout)")
}