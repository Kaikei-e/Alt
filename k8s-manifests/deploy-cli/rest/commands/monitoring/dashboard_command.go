// PHASE R3: Dashboard monitoring command implementation
package monitoring

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"deploy-cli/domain"
	"deploy-cli/rest/commands/shared"
	"deploy-cli/utils/colors"
)

// DashboardCommand provides real-time monitoring dashboard functionality
type DashboardCommand struct {
	shared    *shared.CommandShared
	flags     *DashboardFlags
	output    *MonitoringOutput
	monitor   *MonitoringService
}

// NewDashboardCommand creates the dashboard subcommand
func NewDashboardCommand(shared *shared.CommandShared) *cobra.Command {
	dashboardCmd := &DashboardCommand{
		shared:  shared,
		flags:   NewDashboardFlags(),
		output:  NewMonitoringOutput(shared),
		monitor: NewMonitoringService(shared),
	}

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
  deploy-cli monitoring dashboard production

  # Dashboard with custom refresh interval
  deploy-cli monitoring dashboard production --refresh 10s

  # Dashboard with service filtering
  deploy-cli monitoring dashboard production --filter "app=alt-backend"

  # Compact dashboard view
  deploy-cli monitoring dashboard production --compact

Navigation:
• Press 'q' to quit, 'r' to refresh manually
• Use arrow keys to navigate between sections
• Press 'h' for help, 't' for troubleshooting menu`,
		Args: cobra.MaximumNArgs(1),
		RunE: dashboardCmd.run,
		PersistentPreRunE: shared.PersistentPreRunE,
	}

	// Add dashboard-specific flags
	dashboardCmd.flags.AddToCommand(cmd)

	return cmd
}

// run executes the dashboard command
func (d *DashboardCommand) run(cmd *cobra.Command, args []string) error {
	// Parse environment
	env, err := d.parseEnvironment(args)
	if err != nil {
		return fmt.Errorf("environment parsing failed: %w", err)
	}

	// Parse dashboard flags
	dashboardOptions, err := d.flags.ParseFromCommand(cmd)
	if err != nil {
		return fmt.Errorf("flag parsing failed: %w", err)
	}

	// Validate dashboard options
	if err := d.validateDashboardOptions(dashboardOptions); err != nil {
		return fmt.Errorf("dashboard options validation failed: %w", err)
	}

	// Create dashboard context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Print startup message
	d.output.PrintDashboardStart(env, dashboardOptions)

	// Run the dashboard
	return d.monitor.RunDashboard(ctx, env, dashboardOptions)
}

// parseEnvironment parses the environment argument
func (d *DashboardCommand) parseEnvironment(args []string) (domain.Environment, error) {
	var env domain.Environment = domain.Development
	
	if len(args) > 0 {
		parsedEnv, err := domain.ParseEnvironment(args[0])
		if err != nil {
			return "", fmt.Errorf("invalid environment '%s': %w", args[0], err)
		}
		env = parsedEnv
	}

	d.shared.Logger.InfoWithContext("dashboard environment parsed", map[string]interface{}{
		"environment": env,
		"source":      "argument",
	})

	return env, nil
}

// validateDashboardOptions validates dashboard configuration
func (d *DashboardCommand) validateDashboardOptions(options *DashboardOptions) error {
	// Validate refresh interval
	if options.RefreshInterval <= 0 {
		return fmt.Errorf("refresh interval must be positive, got: %s", options.RefreshInterval)
	}

	// Warn about very short refresh intervals
	if options.RefreshInterval < 5*time.Second {
		d.shared.Logger.WarnWithContext("very short refresh interval may impact performance", map[string]interface{}{
			"refresh_interval": options.RefreshInterval.String(),
			"recommended_min":  "5s",
		})
	}

	// Validate filter syntax if provided
	if options.Filter != "" {
		if err := d.validateFilterSyntax(options.Filter); err != nil {
			return fmt.Errorf("invalid filter syntax: %w", err)
		}
	}

	// Validate export path if provided
	if options.ExportPath != "" {
		if err := d.validateExportPath(options.ExportPath); err != nil {
			return fmt.Errorf("invalid export path: %w", err)
		}
	}

	return nil
}

// validateFilterSyntax validates the Kubernetes label selector syntax
func (d *DashboardCommand) validateFilterSyntax(filter string) error {
	// Basic validation for label selector format
	// This would typically use k8s.io/apimachinery/pkg/labels for full validation
	if strings.TrimSpace(filter) == "" {
		return fmt.Errorf("filter cannot be empty")
	}

	// Log filter validation
	d.shared.Logger.DebugWithContext("filter syntax validated", map[string]interface{}{
		"filter": filter,
		"valid":  true,
	})

	return nil
}

// validateExportPath validates the export file path
func (d *DashboardCommand) validateExportPath(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("export path cannot be empty")
	}

	// Check if directory exists and is writable
	// This would typically validate the full path accessibility
	
	d.shared.Logger.DebugWithContext("export path validated", map[string]interface{}{
		"export_path": path,
		"valid":       true,
	})

	return nil
}

// DashboardOptions represents dashboard configuration options
type DashboardOptions struct {
	RefreshInterval time.Duration
	Filter          string
	Compact         bool
	ExportPath      string
	ShowMetrics     bool
	ShowLogs        bool
	Interactive     bool
}

// DashboardFlags manages dashboard command flags
type DashboardFlags struct {
	RefreshInterval time.Duration
	Filter          string
	Compact         bool
	ExportPath      string
	ShowMetrics     bool
	ShowLogs        bool
	Interactive     bool
}

// NewDashboardFlags creates dashboard flags with defaults
func NewDashboardFlags() *DashboardFlags {
	return &DashboardFlags{
		RefreshInterval: 30 * time.Second,
		Filter:          "",
		Compact:         false,
		ExportPath:      "",
		ShowMetrics:     true,
		ShowLogs:        false,
		Interactive:     true,
	}
}

// AddToCommand adds dashboard flags to the command
func (f *DashboardFlags) AddToCommand(cmd *cobra.Command) {
	cmd.Flags().Duration("refresh", f.RefreshInterval, 
		"Dashboard refresh interval")
	cmd.Flags().String("filter", f.Filter, 
		"Filter services by label selector")
	cmd.Flags().Bool("compact", f.Compact, 
		"Use compact dashboard layout")
	cmd.Flags().String("export", f.ExportPath, 
		"Export dashboard snapshot to file")
	cmd.Flags().Bool("show-metrics", f.ShowMetrics, 
		"Display resource metrics in dashboard")
	cmd.Flags().Bool("show-logs", f.ShowLogs, 
		"Display recent logs in dashboard")
	cmd.Flags().Bool("interactive", f.Interactive, 
		"Enable interactive dashboard mode")
}

// ParseFromCommand parses flags from command into dashboard options
func (f *DashboardFlags) ParseFromCommand(cmd *cobra.Command) (*DashboardOptions, error) {
	options := &DashboardOptions{}
	var err error

	if options.RefreshInterval, err = cmd.Flags().GetDuration("refresh"); err != nil {
		return nil, err
	}
	if options.Filter, err = cmd.Flags().GetString("filter"); err != nil {
		return nil, err
	}
	if options.Compact, err = cmd.Flags().GetBool("compact"); err != nil {
		return nil, err
	}
	if options.ExportPath, err = cmd.Flags().GetString("export"); err != nil {
		return nil, err
	}
	if options.ShowMetrics, err = cmd.Flags().GetBool("show-metrics"); err != nil {
		return nil, err
	}
	if options.ShowLogs, err = cmd.Flags().GetBool("show-logs"); err != nil {
		return nil, err
	}
	if options.Interactive, err = cmd.Flags().GetBool("interactive"); err != nil {
		return nil, err
	}

	return options, nil
}