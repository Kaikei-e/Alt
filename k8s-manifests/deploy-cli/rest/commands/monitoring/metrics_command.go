// PHASE R3: Metrics monitoring command implementation
package monitoring

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"deploy-cli/domain"
	"deploy-cli/rest/commands/shared"
)

// MetricsCommand provides metrics collection and analysis functionality
type MetricsCommand struct {
	shared  *shared.CommandShared
	monitor *MonitoringService
	output  *MonitoringOutput
}

// NewMetricsCommand creates the metrics collection subcommand
func NewMetricsCommand(shared *shared.CommandShared) *cobra.Command {
	metricsCmd := &MetricsCommand{
		shared:  shared,
		monitor: NewMonitoringService(shared),
		output:  NewMonitoringOutput(shared),
	}

	cmd := &cobra.Command{
		Use:   "metrics [environment]",
		Short: "Collect and analyze performance metrics",
		Long: `Collect comprehensive performance metrics and generate analysis reports.

Metrics Collection:
• Resource utilization (CPU, memory, disk, network)
• Application performance metrics (response time, throughput)
• Infrastructure health metrics (node status, storage)
• Service-level metrics (request rates, error rates)
• Custom application metrics from services
• Historical trend analysis and anomaly detection

Examples:
  # Collect metrics for the last hour
  deploy-cli monitoring metrics production --duration 1h

  # Collect metrics with high resolution
  deploy-cli monitoring metrics production --interval 30s --duration 30m

  # Focus on specific metrics
  deploy-cli monitoring metrics production --focus cpu,memory,response_time

  # Generate detailed analysis report
  deploy-cli monitoring metrics production --analyze --output metrics-report.json`,
		Args: cobra.MaximumNArgs(1),
		RunE: metricsCmd.run,
		PersistentPreRunE: shared.PersistentPreRunE,
	}

	// Add metrics-specific flags
	cmd.Flags().Duration("duration", time.Hour, 
		"Duration for metrics collection")
	cmd.Flags().Duration("interval", time.Minute, 
		"Metrics collection interval")
	cmd.Flags().StringSlice("focus", []string{}, 
		"Focus on specific metrics (cpu,memory,disk,network)")
	cmd.Flags().Bool("analyze", false, 
		"Generate detailed analysis report")
	cmd.Flags().StringP("output", "o", "", 
		"Output file for metrics report")

	return cmd
}

// run executes the metrics collection command
func (m *MetricsCommand) run(cmd *cobra.Command, args []string) error {
	// Parse environment
	var env domain.Environment = domain.Development
	if len(args) > 0 {
		parsedEnv, err := domain.ParseEnvironment(args[0])
		if err != nil {
			return fmt.Errorf("invalid environment: %w", err)
		}
		env = parsedEnv
	}

	// Parse metrics options
	duration, _ := cmd.Flags().GetDuration("duration")
	interval, _ := cmd.Flags().GetDuration("interval")
	focus, _ := cmd.Flags().GetStringSlice("focus")
	analyze, _ := cmd.Flags().GetBool("analyze")
	output, _ := cmd.Flags().GetString("output")

	options := &MetricsOptions{
		Duration:   duration,
		Interval:   interval,
		Focus:      focus,
		Analyze:    analyze,
		OutputPath: output,
	}

	// Validate options
	if err := m.validateMetricsOptions(options); err != nil {
		return fmt.Errorf("metrics options validation failed: %w", err)
	}

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run metrics collection
	return m.monitor.CollectMetrics(ctx, env, options)
}

// validateMetricsOptions validates metrics collection options
func (m *MetricsCommand) validateMetricsOptions(options *MetricsOptions) error {
	if options.Duration <= 0 {
		return fmt.Errorf("duration must be positive, got: %s", options.Duration)
	}
	if options.Interval <= 0 {
		return fmt.Errorf("interval must be positive, got: %s", options.Interval)
	}
	if options.Interval > options.Duration {
		return fmt.Errorf("interval cannot be larger than duration")
	}

	return nil
}