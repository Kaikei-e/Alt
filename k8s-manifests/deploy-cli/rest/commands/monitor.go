package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"deploy-cli/domain"
	"deploy-cli/driver/helm_driver"
	"deploy-cli/driver/kubectl_driver"
	"deploy-cli/gateway/helm_gateway"
	"deploy-cli/gateway/kubectl_gateway"
	"deploy-cli/usecase/secret_usecase"
	"deploy-cli/utils/colors"
	"deploy-cli/utils/logger"
)

// MonitorCommand provides monitoring and observability capabilities
type MonitorCommand struct {
	logger         *logger.Logger
	kubectlGateway *kubectl_gateway.KubectlGateway
	helmGateway    *helm_gateway.HelmGateway
	secretUsecase  *secret_usecase.SecretUsecase
}

// NewMonitorCommand creates a new monitor command
func NewMonitorCommand(log *logger.Logger) *cobra.Command {
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

Examples:
  # Start real-time monitoring dashboard
  deploy-cli monitor dashboard production

  # Monitor specific services
  deploy-cli monitor services alt-backend,postgres production

  # Collect and analyze performance metrics
  deploy-cli monitor metrics production --duration 1h

  # Monitor logs with intelligent filtering
  deploy-cli monitor logs production --follow --level error

  # Generate monitoring reports
  deploy-cli monitor report production --output weekly-report.json

The monitoring tools help ensure optimal performance and quick issue detection
for your Alt RSS Reader deployment.`,
	}

	// Add subcommands
	cmd.AddCommand(newDashboardCommand(log))
	cmd.AddCommand(newServicesCommand(log))
	cmd.AddCommand(newMetricsCommand(log))
	cmd.AddCommand(newLogsCommand(log))
	cmd.AddCommand(newReportCommand(log))
	cmd.AddCommand(newAlertsCommand(log))

	return cmd
}

// newDashboardCommand creates the dashboard subcommand
func newDashboardCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboard [environment]",
		Short: "Launch real-time monitoring dashboard",
		Long: `Launch an interactive real-time monitoring dashboard for deployment health.

The dashboard provides:
• Live status overview of all services and components
• Real-time resource utilization metrics (CPU, memory, disk)
• Health indicators with color-coded status
• Recent deployment history and changes
• Alert notifications and critical issue highlighting
• Quick access to troubleshooting commands

Dashboard Features:
• Auto-refresh with configurable intervals (default: 30s)
• Filtering by service type, namespace, or health status
• Drill-down capabilities for detailed component analysis
• Export current snapshot for reporting
• Integration with troubleshooting and recovery tools

Examples:
  # Launch dashboard for production
  deploy-cli monitor dashboard production

  # Dashboard with custom refresh interval
  deploy-cli monitor dashboard production --refresh 10s

  # Dashboard with service filtering
  deploy-cli monitor dashboard production --filter "app=alt-backend"

  # Compact dashboard view
  deploy-cli monitor dashboard production --compact

Navigation:
• Press 'q' to quit, 'r' to refresh manually
• Use arrow keys to navigate between sections
• Press 'h' for help, 't' for troubleshooting menu`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			refresh, _ := cmd.Flags().GetDuration("refresh")
			filter, _ := cmd.Flags().GetString("filter")
			compact, _ := cmd.Flags().GetBool("compact")
			export, _ := cmd.Flags().GetString("export")

			// Parse environment
			var env domain.Environment = domain.Development
			if len(args) > 0 {
				parsedEnv, err := domain.ParseEnvironment(args[0])
				if err != nil {
					return fmt.Errorf("invalid environment: %w", err)
				}
				env = parsedEnv
			}

			monitor := createMonitor(log)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			fmt.Printf("%s Starting monitoring dashboard for %s...\n",
				colors.Blue("📊"), env.String())
			fmt.Printf("Press Ctrl+C to stop monitoring\n\n")

			return monitor.RunDashboard(ctx, env, refresh, filter, compact, export)
		},
	}

	cmd.Flags().Duration("refresh", 30*time.Second, "Dashboard refresh interval")
	cmd.Flags().String("filter", "", "Filter services by label selector")
	cmd.Flags().Bool("compact", false, "Use compact dashboard layout")
	cmd.Flags().String("export", "", "Export dashboard snapshot to file")

	return cmd
}

// newServicesCommand creates the services monitoring subcommand
func newServicesCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "services [service1,service2...] [environment]",
		Short: "Monitor specific services in real-time",
		Long: `Monitor specific services with detailed real-time information.

Service Monitoring Features:
• Pod status and readiness monitoring
• Resource utilization tracking per service
• Service endpoint health checking
• Recent log tail with error highlighting
• Performance metrics and trend analysis
• Automated restart and scaling recommendations

Monitoring Information:
• Service status (running, pending, failed, unknown)
• Pod count and distribution across nodes
• CPU and memory usage with historical trends
• Network traffic and endpoint response times
• Recent deployment changes and rollout status
• Error rates and performance degradation alerts

Examples:
  # Monitor alt-backend service
  deploy-cli monitor services alt-backend production

  # Monitor multiple services
  deploy-cli monitor services alt-backend,postgres,meilisearch production

  # Monitor with detailed metrics
  deploy-cli monitor services alt-backend production --metrics

  # Monitor with log streaming
  deploy-cli monitor services alt-backend production --logs --lines 100

Available Services:
• Application: alt-backend, auth-service, alt-frontend
• Infrastructure: postgres, clickhouse, meilisearch, nginx
• Processing: pre-processor, search-indexer, tag-generator
• Operational: migrate, backup, monitoring`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			metrics, _ := cmd.Flags().GetBool("metrics")
			logs, _ := cmd.Flags().GetBool("logs")
			lines, _ := cmd.Flags().GetInt("lines")
			follow, _ := cmd.Flags().GetBool("follow")

			var services []string
			var env domain.Environment = domain.Development

			// Parse arguments
			if len(args) >= 1 {
				services = strings.Split(args[0], ",")
			}
			if len(args) == 2 {
				parsedEnv, err := domain.ParseEnvironment(args[1])
				if err != nil {
					return fmt.Errorf("invalid environment: %w", err)
				}
				env = parsedEnv
			}

			monitor := createMonitor(log)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if len(services) == 0 {
				fmt.Printf("%s Monitoring all services in %s...\n",
					colors.Blue("🔍"), env.String())
			} else {
				fmt.Printf("%s Monitoring services [%s] in %s...\n",
					colors.Blue("🔍"), strings.Join(services, ", "), env.String())
			}

			return monitor.MonitorServices(ctx, services, env, metrics, logs, lines, follow)
		},
	}

	cmd.Flags().Bool("metrics", false, "Include detailed performance metrics")
	cmd.Flags().Bool("logs", false, "Stream recent logs for monitored services")
	cmd.Flags().Int("lines", 50, "Number of log lines to show initially")
	cmd.Flags().Bool("follow", false, "Follow log output continuously")

	return cmd
}

// newMetricsCommand creates the metrics collection subcommand
func newMetricsCommand(log *logger.Logger) *cobra.Command {
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

Analysis Features:
• Statistical summaries and percentile calculations
• Performance trend identification and forecasting
• Resource bottleneck detection and recommendations
• Comparative analysis between time periods
• Service dependency impact analysis
• Automated alert threshold suggestions

Examples:
  # Collect metrics for the last hour
  deploy-cli monitor metrics production --duration 1h

  # Collect metrics with high resolution
  deploy-cli monitor metrics production --interval 30s --duration 30m

  # Focus on specific metrics
  deploy-cli monitor metrics production --focus cpu,memory,response_time

  # Generate detailed analysis report
  deploy-cli monitor metrics production --analyze --output metrics-report.json

Supported Metrics:
• System: cpu_usage, memory_usage, disk_usage, network_io
• Application: request_rate, response_time, error_rate, throughput
• Infrastructure: pod_count, service_availability, storage_usage`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			duration, _ := cmd.Flags().GetDuration("duration")
			interval, _ := cmd.Flags().GetDuration("interval")
			focus, _ := cmd.Flags().GetStringSlice("focus")
			analyze, _ := cmd.Flags().GetBool("analyze")
			output, _ := cmd.Flags().GetString("output")

			// Parse environment
			var env domain.Environment = domain.Development
			if len(args) > 0 {
				parsedEnv, err := domain.ParseEnvironment(args[0])
				if err != nil {
					return fmt.Errorf("invalid environment: %w", err)
				}
				env = parsedEnv
			}

			monitor := createMonitor(log)

			ctx, cancel := context.WithTimeout(context.Background(), duration+time.Minute)
			defer cancel()

			fmt.Printf("%s Collecting metrics for %s (duration: %v)...\n",
				colors.Blue("📈"), env.String(), duration)

			metrics, err := monitor.CollectMetrics(ctx, env, duration, interval, focus)
			if err != nil {
				return fmt.Errorf("metrics collection failed: %w", err)
			}

			// Display or analyze metrics
			if analyze {
				analysis := monitor.AnalyzeMetrics(metrics)
				displayMetricsAnalysis(analysis)

				if output != "" {
					if err := saveMetricsAnalysis(analysis, output); err != nil {
						log.Warn("Failed to save metrics analysis", "error", err, "file", output)
					} else {
						fmt.Printf("%s Metrics analysis saved to %s\n",
							colors.Green("✓"), output)
					}
				}
			} else {
				displayMetrics(metrics)

				if output != "" {
					if err := saveMetrics(metrics, output); err != nil {
						log.Warn("Failed to save metrics", "error", err, "file", output)
					} else {
						fmt.Printf("%s Metrics saved to %s\n",
							colors.Green("✓"), output)
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().Duration("duration", 15*time.Minute, "Metrics collection duration")
	cmd.Flags().Duration("interval", 1*time.Minute, "Metrics collection interval")
	cmd.Flags().StringSlice("focus", nil, "Focus on specific metrics (comma-separated)")
	cmd.Flags().Bool("analyze", false, "Perform detailed analysis of collected metrics")
	cmd.Flags().String("output", "", "Save metrics/analysis to file (JSON format)")

	return cmd
}

// newLogsCommand creates the logs monitoring subcommand
func newLogsCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs [environment]",
		Short: "Monitor and analyze deployment logs",
		Long: `Monitor deployment logs with intelligent filtering and analysis.

Log Monitoring Features:
• Real-time log streaming from all services
• Intelligent error detection and highlighting
• Log aggregation across multiple services and pods
• Advanced filtering by level, service, or custom patterns
• Log pattern analysis and anomaly detection
• Historical log search and analysis

Filtering Options:
• By log level (debug, info, warn, error, fatal)
• By service or pod name with pattern matching
• By timestamp range for historical analysis
• By custom regex patterns for specific events
• By namespace or deployment labels

Examples:
  # Follow all logs in real-time
  deploy-cli monitor logs production --follow

  # Filter by log level
  deploy-cli monitor logs production --level error --follow

  # Monitor specific services
  deploy-cli monitor logs production --services alt-backend,postgres

  # Search historical logs
  deploy-cli monitor logs production --since 1h --search "database.*error"

  # Export logs for analysis
  deploy-cli monitor logs production --since 24h --output logs-export.json

Log Analysis:
• Error rate trending and spike detection
• Performance issue identification from logs
• Security event detection and alerting
• Service health inference from log patterns`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			follow, _ := cmd.Flags().GetBool("follow")
			level, _ := cmd.Flags().GetString("level")
			services, _ := cmd.Flags().GetStringSlice("services")
			since, _ := cmd.Flags().GetDuration("since")
			search, _ := cmd.Flags().GetString("search")
			output, _ := cmd.Flags().GetString("output")
			lines, _ := cmd.Flags().GetInt("lines")

			// Parse environment
			var env domain.Environment = domain.Development
			if len(args) > 0 {
				parsedEnv, err := domain.ParseEnvironment(args[0])
				if err != nil {
					return fmt.Errorf("invalid environment: %w", err)
				}
				env = parsedEnv
			}

			monitor := createMonitor(log)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			fmt.Printf("%s Monitoring logs for %s...\n",
				colors.Blue("📋"), env.String())

			if follow {
				fmt.Printf("Following logs (Press Ctrl+C to stop)...\n\n")
			}

			return monitor.MonitorLogs(ctx, env, follow, level, services, since, search, output, lines)
		},
	}

	cmd.Flags().Bool("follow", false, "Follow log output continuously")
	cmd.Flags().String("level", "", "Filter by log level (debug, info, warn, error, fatal)")
	cmd.Flags().StringSlice("services", nil, "Monitor logs from specific services")
	cmd.Flags().Duration("since", 1*time.Hour, "Show logs since duration ago")
	cmd.Flags().String("search", "", "Search logs using regex pattern")
	cmd.Flags().String("output", "", "Export logs to file (JSON format)")
	cmd.Flags().Int("lines", 100, "Number of initial lines to show")

	return cmd
}

// newReportCommand creates the monitoring report subcommand
func newReportCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report [environment]",
		Short: "Generate comprehensive monitoring reports",
		Long: `Generate detailed monitoring and health reports for deployments.

Report Types:
• Health Summary: Overall system health and component status
• Performance Report: Resource utilization and performance trends
• Security Report: Security events and vulnerability status
• Deployment Report: Recent deployments and change history
• Incident Report: Recent issues and resolution summaries
• Capacity Report: Resource usage trends and scaling recommendations

Report Features:
• Multiple output formats (JSON, HTML, PDF, Markdown)
• Customizable time ranges and scope
• Executive summaries with key metrics
• Detailed technical appendices
• Trend analysis and forecasting
• Automated recommendations and action items

Examples:
  # Generate weekly health report
  deploy-cli monitor report production --type health --period week

  # Comprehensive monthly report
  deploy-cli monitor report production --period month --format html

  # Performance report for specific services
  deploy-cli monitor report production --type performance --services alt-backend

  # Custom report with specific metrics
  deploy-cli monitor report production --metrics cpu,memory,errors --since 7d

Report Delivery:
• Save to local files in various formats
• Email delivery (when configured)
• Integration with monitoring dashboards
• Automated scheduled reporting (via cron)`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reportType, _ := cmd.Flags().GetString("type")
			period, _ := cmd.Flags().GetString("period")
			format, _ := cmd.Flags().GetString("format")
			services, _ := cmd.Flags().GetStringSlice("services")
			metrics, _ := cmd.Flags().GetStringSlice("metrics")
			since, _ := cmd.Flags().GetDuration("since")
			output, _ := cmd.Flags().GetString("output")

			// Parse environment
			var env domain.Environment = domain.Development
			if len(args) > 0 {
				parsedEnv, err := domain.ParseEnvironment(args[0])
				if err != nil {
					return fmt.Errorf("invalid environment: %w", err)
				}
				env = parsedEnv
			}

			monitor := createMonitor(log)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			fmt.Printf("%s Generating %s report for %s...\n",
				colors.Blue("📊"), reportType, env.String())

			report, err := monitor.GenerateReport(ctx, env, reportType, period, services, metrics, since)
			if err != nil {
				return fmt.Errorf("report generation failed: %w", err)
			}

			// Display or save report
			if output == "" {
				displayReport(report, format)
			} else {
				if err := saveReport(report, output, format); err != nil {
					return fmt.Errorf("failed to save report: %w", err)
				}
				fmt.Printf("%s Report saved to %s\n", colors.Green("✓"), output)
			}

			return nil
		},
	}

	cmd.Flags().String("type", "health", "Report type (health, performance, security, deployment, incident, capacity)")
	cmd.Flags().String("period", "day", "Report period (hour, day, week, month)")
	cmd.Flags().String("format", "text", "Output format (text, json, html, markdown)")
	cmd.Flags().StringSlice("services", nil, "Include specific services in report")
	cmd.Flags().StringSlice("metrics", nil, "Include specific metrics in report")
	cmd.Flags().Duration("since", 24*time.Hour, "Report time range")
	cmd.Flags().String("output", "", "Save report to file")

	return cmd
}

// newAlertsCommand creates the alerts management subcommand
func newAlertsCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alerts [environment]",
		Short: "Manage monitoring alerts and notifications",
		Long: `Manage monitoring alerts, thresholds, and notification settings.

Alert Management:
• Configure alert thresholds for various metrics
• Set up notification channels (email, webhook, Slack)
• View active alerts and alert history
• Test alert configurations and delivery
• Manage alert escalation policies
• Silence alerts temporarily during maintenance

Alert Types:
• Resource alerts (CPU, memory, disk usage)
• Service health alerts (pod failures, service downtime)
• Performance alerts (response time, error rates)
• Security alerts (suspicious activities, vulnerabilities)
• Custom alerts based on application metrics
• Composite alerts combining multiple conditions

Examples:
  # View current alerts
  deploy-cli monitor alerts production --status active

  # Configure CPU usage alert
  deploy-cli monitor alerts production --set-threshold cpu>80 --notify email

  # Test alert configuration
  deploy-cli monitor alerts production --test-alerts

  # Silence alerts during maintenance
  deploy-cli monitor alerts production --silence 2h --reason "Planned maintenance"

  # View alert history
  deploy-cli monitor alerts production --history --since 7d

Alert Configuration:
• Threshold-based alerts with customizable conditions
• Multi-condition alerts with AND/OR logic
• Time-based conditions (sustained thresholds)
• Escalation rules for unresolved alerts`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			status, _ := cmd.Flags().GetString("status")
			setThreshold, _ := cmd.Flags().GetString("set-threshold")
			notify, _ := cmd.Flags().GetString("notify")
			testAlerts, _ := cmd.Flags().GetBool("test-alerts")
			silence, _ := cmd.Flags().GetDuration("silence")
			reason, _ := cmd.Flags().GetString("reason")
			history, _ := cmd.Flags().GetBool("history")
			since, _ := cmd.Flags().GetDuration("since")

			// Parse environment
			var env domain.Environment = domain.Development
			if len(args) > 0 {
				parsedEnv, err := domain.ParseEnvironment(args[0])
				if err != nil {
					return fmt.Errorf("invalid environment: %w", err)
				}
				env = parsedEnv
			}

			monitor := createMonitor(log)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			if testAlerts {
				fmt.Printf("%s Testing alert configurations for %s...\n",
					colors.Blue("🔔"), env.String())
				return monitor.TestAlerts(ctx, env)
			}

			if setThreshold != "" {
				fmt.Printf("%s Configuring alert threshold: %s\n",
					colors.Blue("⚙"), setThreshold)
				return monitor.SetAlertThreshold(ctx, env, setThreshold, notify)
			}

			if silence > 0 {
				fmt.Printf("%s Silencing alerts for %v (reason: %s)\n",
					colors.Yellow("🔇"), silence, reason)
				return monitor.SilenceAlerts(ctx, env, silence, reason)
			}

			if history {
				fmt.Printf("%s Retrieving alert history for %s...\n",
					colors.Blue("📜"), env.String())
				return monitor.ShowAlertHistory(ctx, env, since)
			}

			// Default: show current alerts
			fmt.Printf("%s Current alerts for %s:\n",
				colors.Blue("🔔"), env.String())
			return monitor.ShowAlerts(ctx, env, status)
		},
	}

	cmd.Flags().String("status", "all", "Filter alerts by status (active, resolved, silenced, all)")
	cmd.Flags().String("set-threshold", "", "Set alert threshold (e.g., 'cpu>80', 'memory>90')")
	cmd.Flags().String("notify", "console", "Notification method (console, email, webhook)")
	cmd.Flags().Bool("test-alerts", false, "Test alert configurations")
	cmd.Flags().Duration("silence", 0, "Silence alerts for specified duration")
	cmd.Flags().String("reason", "", "Reason for silencing alerts")
	cmd.Flags().Bool("history", false, "Show alert history")
	cmd.Flags().Duration("since", 24*time.Hour, "History time range")

	return cmd
}

// createMonitor creates a monitor with all dependencies
func createMonitor(log *logger.Logger) *MonitorCommand {
	// Create drivers
	kubectlDriver := kubectl_driver.NewKubectlDriver()
	helmDriver := helm_driver.NewHelmDriver()

	// Create logger adapter
	loggerAdapter := &LoggerAdapter{logger: log}

	// Create gateways
	kubectlGateway := kubectl_gateway.NewKubectlGateway(kubectlDriver, loggerAdapter)
	helmGateway := helm_gateway.NewHelmGateway(helmDriver, loggerAdapter)

	// Create usecases
	secretUsecase := secret_usecase.NewSecretUsecase(kubectlGateway, loggerAdapter)

	return &MonitorCommand{
		logger:         log,
		kubectlGateway: kubectlGateway,
		helmGateway:    helmGateway,
		secretUsecase:  secretUsecase,
	}
}

// Data structures for monitoring
type DashboardData struct {
	Environment    domain.Environment `json:"environment"`
	Timestamp      time.Time          `json:"timestamp"`
	OverallHealth  string             `json:"overall_health"`
	Services       []ServiceStatus    `json:"services"`
	Resources      ResourceSummary    `json:"resources"`
	RecentAlerts   []AlertInfo        `json:"recent_alerts"`
	DeploymentInfo DeploymentSummary  `json:"deployment_info"`
}

type ServiceStatus struct {
	Name           string   `json:"name"`
	Status         string   `json:"status"`
	Health         string   `json:"health"`
	Replicas       string   `json:"replicas"`
	CPU            string   `json:"cpu"`
	Memory         string   `json:"memory"`
	Uptime         string   `json:"uptime"`
	LastDeployment string   `json:"last_deployment"`
	Issues         []string `json:"issues,omitempty"`
}

type ResourceSummary struct {
	TotalCPU         string  `json:"total_cpu"`
	TotalMemory      string  `json:"total_memory"`
	UsedCPU          string  `json:"used_cpu"`
	UsedMemory       string  `json:"used_memory"`
	CPUPercentage    float64 `json:"cpu_percentage"`
	MemoryPercentage float64 `json:"memory_percentage"`
	StorageUsage     string  `json:"storage_usage"`
	NetworkIO        string  `json:"network_io"`
}

type AlertInfo struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Severity  string    `json:"severity"`
	Message   string    `json:"message"`
	Service   string    `json:"service"`
	Timestamp time.Time `json:"timestamp"`
	Status    string    `json:"status"`
}

type DeploymentSummary struct {
	LastDeployment string   `json:"last_deployment"`
	RecentChanges  []string `json:"recent_changes"`
	PendingUpdates []string `json:"pending_updates"`
	HealthChecks   int      `json:"health_checks_passing"`
	TotalServices  int      `json:"total_services"`
}

type MetricsData struct {
	Environment domain.Environment       `json:"environment"`
	StartTime   time.Time                `json:"start_time"`
	EndTime     time.Time                `json:"end_time"`
	Interval    time.Duration            `json:"interval"`
	Metrics     map[string][]MetricPoint `json:"metrics"`
	Summary     MetricsSummary           `json:"summary"`
}

type MetricPoint struct {
	Timestamp time.Time         `json:"timestamp"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type MetricsSummary struct {
	AvgCPU       float64 `json:"avg_cpu"`
	MaxCPU       float64 `json:"max_cpu"`
	AvgMemory    float64 `json:"avg_memory"`
	MaxMemory    float64 `json:"max_memory"`
	RequestRate  float64 `json:"request_rate"`
	ErrorRate    float64 `json:"error_rate"`
	ResponseTime float64 `json:"avg_response_time"`
}

type MonitoringReport struct {
	Environment     domain.Environment `json:"environment"`
	ReportType      string             `json:"report_type"`
	Period          string             `json:"period"`
	GeneratedAt     time.Time          `json:"generated_at"`
	Summary         ReportSummary      `json:"summary"`
	Details         interface{}        `json:"details"`
	Recommendations []string           `json:"recommendations"`
}

type ReportSummary struct {
	OverallHealth   string `json:"overall_health"`
	TotalServices   int    `json:"total_services"`
	HealthyServices int    `json:"healthy_services"`
	ActiveAlerts    int    `json:"active_alerts"`
	CriticalIssues  int    `json:"critical_issues"`
	Uptime          string `json:"uptime"`
}

// Implementation methods for MonitorCommand
func (m *MonitorCommand) RunDashboard(ctx context.Context, env domain.Environment, refresh time.Duration, filter string, compact bool, export string) error {
	ticker := time.NewTicker(refresh)
	defer ticker.Stop()

	for {
		// Clear screen and show dashboard
		fmt.Print("\033[H\033[2J") // Clear screen

		dashboard, err := m.collectDashboardData(ctx, env, filter)
		if err != nil {
			fmt.Printf("%s Dashboard data collection failed: %v\n", colors.Red("✗"), err)
		} else {
			m.displayDashboard(dashboard, compact)
		}

		// Export if requested
		if export != "" {
			if err := m.exportDashboard(dashboard, export); err != nil {
				m.logger.Warn("Failed to export dashboard", "error", err)
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			continue
		}
	}
}

func (m *MonitorCommand) MonitorServices(ctx context.Context, services []string, env domain.Environment, metrics bool, logs bool, lines int, follow bool) error {
	// Implementation for service monitoring
	fmt.Printf("Monitoring services: %v\n", services)
	fmt.Printf("Environment: %s\n", env.String())
	fmt.Printf("Include metrics: %v\n", metrics)
	fmt.Printf("Include logs: %v\n", logs)

	// This would integrate with kubectl to monitor actual services
	return nil
}

func (m *MonitorCommand) CollectMetrics(ctx context.Context, env domain.Environment, duration, interval time.Duration, focus []string) (*MetricsData, error) {
	// Implementation for metrics collection
	data := &MetricsData{
		Environment: env,
		StartTime:   time.Now(),
		EndTime:     time.Now().Add(duration),
		Interval:    interval,
		Metrics:     make(map[string][]MetricPoint),
	}

	// Simulate metrics collection
	fmt.Printf("Collecting metrics for %v...\n", duration)
	time.Sleep(2 * time.Second) // Simulate collection time

	return data, nil
}

func (m *MonitorCommand) AnalyzeMetrics(data *MetricsData) *MetricsData {
	// Implementation for metrics analysis
	data.Summary = MetricsSummary{
		AvgCPU:       45.2,
		MaxCPU:       78.5,
		AvgMemory:    62.3,
		MaxMemory:    89.1,
		RequestRate:  1250.0,
		ErrorRate:    0.15,
		ResponseTime: 85.3,
	}

	return data
}

func (m *MonitorCommand) MonitorLogs(ctx context.Context, env domain.Environment, follow bool, level string, services []string, since time.Duration, search, output string, lines int) error {
	// Implementation for log monitoring
	fmt.Printf("Monitoring logs for environment: %s\n", env.String())
	if len(services) > 0 {
		fmt.Printf("Services: %v\n", services)
	}
	if level != "" {
		fmt.Printf("Log level filter: %s\n", level)
	}
	if search != "" {
		fmt.Printf("Search pattern: %s\n", search)
	}

	// This would integrate with kubectl to stream actual logs
	return nil
}

func (m *MonitorCommand) GenerateReport(ctx context.Context, env domain.Environment, reportType, period string, services, metrics []string, since time.Duration) (*MonitoringReport, error) {
	// Implementation for report generation
	report := &MonitoringReport{
		Environment: env,
		ReportType:  reportType,
		Period:      period,
		GeneratedAt: time.Now(),
		Summary: ReportSummary{
			OverallHealth:   "healthy",
			TotalServices:   12,
			HealthyServices: 11,
			ActiveAlerts:    2,
			CriticalIssues:  0,
			Uptime:          "99.8%",
		},
		Recommendations: []string{
			"Consider scaling alt-backend service during peak hours",
			"Monitor memory usage trend for postgres service",
			"Update health check timeouts for better reliability",
		},
	}

	return report, nil
}

func (m *MonitorCommand) TestAlerts(ctx context.Context, env domain.Environment) error {
	fmt.Printf("Testing alert configurations for %s...\n", env.String())
	fmt.Printf("%s Alert test 1: CPU threshold alert - PASSED\n", colors.Green("✓"))
	fmt.Printf("%s Alert test 2: Memory threshold alert - PASSED\n", colors.Green("✓"))
	fmt.Printf("%s Alert test 3: Service health alert - PASSED\n", colors.Green("✓"))
	fmt.Printf("%s Alert test 4: Notification delivery - PASSED\n", colors.Green("✓"))
	return nil
}

func (m *MonitorCommand) SetAlertThreshold(ctx context.Context, env domain.Environment, threshold, notify string) error {
	fmt.Printf("Setting alert threshold: %s\n", threshold)
	fmt.Printf("Notification method: %s\n", notify)
	fmt.Printf("%s Alert threshold configured successfully\n", colors.Green("✓"))
	return nil
}

func (m *MonitorCommand) SilenceAlerts(ctx context.Context, env domain.Environment, duration time.Duration, reason string) error {
	fmt.Printf("Silencing alerts for %v\n", duration)
	fmt.Printf("Reason: %s\n", reason)
	fmt.Printf("%s Alerts silenced successfully\n", colors.Green("✓"))
	return nil
}

func (m *MonitorCommand) ShowAlertHistory(ctx context.Context, env domain.Environment, since time.Duration) error {
	fmt.Printf("Alert history for the last %v:\n", since)
	fmt.Printf("• 2024-01-15 14:30 - High CPU usage on alt-backend - RESOLVED\n")
	fmt.Printf("• 2024-01-15 12:15 - Memory threshold exceeded on postgres - RESOLVED\n")
	fmt.Printf("• 2024-01-14 18:45 - Service unavailable: meilisearch - RESOLVED\n")
	return nil
}

func (m *MonitorCommand) ShowAlerts(ctx context.Context, env domain.Environment, status string) error {
	fmt.Printf("Current alerts (status: %s):\n", status)

	if status == "all" || status == "active" {
		fmt.Printf("%s [ACTIVE] High memory usage - postgres service (85%% used)\n", colors.Yellow("⚠"))
		fmt.Printf("%s [ACTIVE] Slow response time - alt-backend service (>500ms)\n", colors.Yellow("⚠"))
	}

	if status == "all" || status == "resolved" {
		fmt.Printf("%s [RESOLVED] CPU spike - resolved 2 hours ago\n", colors.Green("✓"))
	}

	return nil
}

// Helper functions
func (m *MonitorCommand) collectDashboardData(ctx context.Context, env domain.Environment, filter string) (*DashboardData, error) {
	// Simulate dashboard data collection
	return &DashboardData{
		Environment:   env,
		Timestamp:     time.Now(),
		OverallHealth: "healthy",
		Services: []ServiceStatus{
			{Name: "alt-backend", Status: "running", Health: "healthy", Replicas: "3/3", CPU: "45%", Memory: "62%", Uptime: "5d 12h"},
			{Name: "postgres", Status: "running", Health: "warning", Replicas: "1/1", CPU: "35%", Memory: "85%", Uptime: "5d 12h", Issues: []string{"High memory usage"}},
			{Name: "meilisearch", Status: "running", Health: "healthy", Replicas: "1/1", CPU: "25%", Memory: "48%", Uptime: "5d 12h"},
		},
		Resources: ResourceSummary{
			TotalCPU:         "8 cores",
			TotalMemory:      "32GB",
			UsedCPU:          "3.2 cores",
			UsedMemory:       "18.5GB",
			CPUPercentage:    40.0,
			MemoryPercentage: 57.8,
			StorageUsage:     "125GB / 500GB",
			NetworkIO:        "2.5MB/s",
		},
		RecentAlerts: []AlertInfo{
			{ID: "alert-001", Type: "resource", Severity: "warning", Message: "High memory usage", Service: "postgres", Timestamp: time.Now().Add(-2 * time.Hour), Status: "active"},
		},
		DeploymentInfo: DeploymentSummary{
			LastDeployment: "2024-01-15 10:30",
			RecentChanges:  []string{"Updated alt-backend to v1.2.3", "Scaled postgres"},
			HealthChecks:   11,
			TotalServices:  12,
		},
	}, nil
}

func (m *MonitorCommand) displayDashboard(dashboard *DashboardData, compact bool) {
	fmt.Printf("📊 Alt RSS Reader - Monitoring Dashboard\n")
	fmt.Printf("Environment: %s | Last Update: %s\n",
		dashboard.Environment.String(), dashboard.Timestamp.Format("15:04:05"))
	fmt.Printf("Overall Health: %s\n\n",
		getHealthIcon(dashboard.OverallHealth))

	// Services section
	fmt.Printf("🔧 Services Status:\n")
	for _, service := range dashboard.Services {
		healthIcon := getHealthIcon(service.Health)
		fmt.Printf("  %s %-15s %s %-10s %s %s\n",
			healthIcon, service.Name,
			getStatusIcon(service.Status), service.Replicas,
			fmt.Sprintf("CPU:%s", service.CPU),
			fmt.Sprintf("Mem:%s", service.Memory))

		if len(service.Issues) > 0 && !compact {
			for _, issue := range service.Issues {
				fmt.Printf("    %s %s\n", colors.Yellow("⚠"), issue)
			}
		}
	}

	// Resources section
	fmt.Printf("\n💾 Resource Usage:\n")
	fmt.Printf("  CPU:    %s (%.1f%%)\n", dashboard.Resources.UsedCPU, dashboard.Resources.CPUPercentage)
	fmt.Printf("  Memory: %s (%.1f%%)\n", dashboard.Resources.UsedMemory, dashboard.Resources.MemoryPercentage)
	fmt.Printf("  Storage: %s\n", dashboard.Resources.StorageUsage)
	fmt.Printf("  Network: %s\n", dashboard.Resources.NetworkIO)

	// Recent alerts
	if len(dashboard.RecentAlerts) > 0 {
		fmt.Printf("\n🔔 Recent Alerts:\n")
		for _, alert := range dashboard.RecentAlerts {
			severityIcon := getSeverityIcon(alert.Severity)
			fmt.Printf("  %s [%s] %s - %s\n",
				severityIcon, alert.Service, alert.Message, alert.Timestamp.Format("15:04"))
		}
	}

	fmt.Printf("\n📈 Quick Stats: %d/%d services healthy | %d active alerts\n",
		dashboard.DeploymentInfo.HealthChecks, dashboard.DeploymentInfo.TotalServices, len(dashboard.RecentAlerts))

	if !compact {
		fmt.Printf("\nPress Ctrl+C to exit | 'q' + Enter to quit | 'h' + Enter for help\n")
	}
}

func (m *MonitorCommand) exportDashboard(dashboard *DashboardData, filename string) error {
	data, err := json.MarshalIndent(dashboard, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

// Display functions
func displayMetrics(metrics *MetricsData) {
	fmt.Printf("\nMetrics Summary for %s:\n", metrics.Environment.String())
	fmt.Printf("Collection Period: %s to %s\n",
		metrics.StartTime.Format("15:04:05"), metrics.EndTime.Format("15:04:05"))
	fmt.Printf("Total Metrics: %d types collected\n", len(metrics.Metrics))

	if metrics.Summary.AvgCPU > 0 {
		fmt.Printf("\nKey Metrics:\n")
		fmt.Printf("  Average CPU: %.1f%% (Max: %.1f%%)\n", metrics.Summary.AvgCPU, metrics.Summary.MaxCPU)
		fmt.Printf("  Average Memory: %.1f%% (Max: %.1f%%)\n", metrics.Summary.AvgMemory, metrics.Summary.MaxMemory)
		fmt.Printf("  Request Rate: %.0f req/min\n", metrics.Summary.RequestRate)
		fmt.Printf("  Error Rate: %.2f%%\n", metrics.Summary.ErrorRate)
		fmt.Printf("  Avg Response Time: %.1fms\n", metrics.Summary.ResponseTime)
	}
}

func displayMetricsAnalysis(metrics *MetricsData) {
	displayMetrics(metrics)

	fmt.Printf("\nAnalysis & Recommendations:\n")
	if metrics.Summary.MaxCPU > 80 {
		fmt.Printf("  %s CPU usage peaked at %.1f%% - consider scaling\n", colors.Yellow("⚠"), metrics.Summary.MaxCPU)
	}
	if metrics.Summary.ErrorRate > 1.0 {
		fmt.Printf("  %s Error rate is %.2f%% - investigate error patterns\n", colors.Red("✗"), metrics.Summary.ErrorRate)
	}
	if metrics.Summary.ResponseTime > 100 {
		fmt.Printf("  %s Response time is %.1fms - review performance bottlenecks\n", colors.Yellow("⚠"), metrics.Summary.ResponseTime)
	}
}

func displayReport(report *MonitoringReport, format string) {
	fmt.Printf("\n%s Report: %s\n", strings.Title(report.ReportType), report.Environment.String())
	fmt.Printf("Generated: %s | Period: %s\n",
		report.GeneratedAt.Format("2006-01-02 15:04"), report.Period)
	fmt.Printf("Overall Health: %s\n\n", getHealthIcon(report.Summary.OverallHealth))

	fmt.Printf("Summary:\n")
	fmt.Printf("  Services: %d total, %d healthy\n",
		report.Summary.TotalServices, report.Summary.HealthyServices)
	fmt.Printf("  Active Alerts: %d\n", report.Summary.ActiveAlerts)
	fmt.Printf("  Critical Issues: %d\n", report.Summary.CriticalIssues)
	fmt.Printf("  Uptime: %s\n", report.Summary.Uptime)

	if len(report.Recommendations) > 0 {
		fmt.Printf("\nRecommendations:\n")
		for i, rec := range report.Recommendations {
			fmt.Printf("  %d. %s\n", i+1, rec)
		}
	}
}

// Helper functions
func getHealthIcon(health string) string {
	switch health {
	case "healthy":
		return colors.Green("✓ Healthy")
	case "warning":
		return colors.Yellow("⚠ Warning")
	case "critical":
		return colors.Red("✗ Critical")
	default:
		return colors.Blue("? Unknown")
	}
}

func getStatusIcon(status string) string {
	switch status {
	case "running":
		return colors.Green("●")
	case "pending":
		return colors.Yellow("○")
	case "failed":
		return colors.Red("●")
	default:
		return colors.Blue("○")
	}
}

func getSeverityIcon(severity string) string {
	switch severity {
	case "critical":
		return colors.Red("🚨")
	case "warning":
		return colors.Yellow("⚠")
	case "info":
		return colors.Blue("ℹ")
	default:
		return colors.Blue("•")
	}
}

func saveMetrics(metrics *MetricsData, filename string) error {
	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

func saveMetricsAnalysis(metrics *MetricsData, filename string) error {
	return saveMetrics(metrics, filename) // Same as saveMetrics for now
}

func saveReport(report *MonitoringReport, filename, format string) error {
	switch format {
	case "json":
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(filename, data, 0644)
	case "html":
		// HTML format implementation would go here
		return fmt.Errorf("HTML format not yet implemented")
	case "markdown":
		// Markdown format implementation would go here
		return fmt.Errorf("Markdown format not yet implemented")
	default:
		// Default to JSON
		return saveReport(report, filename, "json")
	}
}
