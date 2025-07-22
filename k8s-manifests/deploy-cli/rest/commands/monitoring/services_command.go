// PHASE R3: Services monitoring command implementation
package monitoring

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"deploy-cli/domain"
	"deploy-cli/rest/commands/shared"
)

// ServicesCommand provides service-specific monitoring functionality
type ServicesCommand struct {
	shared  *shared.CommandShared
	flags   *ServicesFlags
	output  *MonitoringOutput
	monitor *MonitoringService
}

// NewServicesCommand creates the services monitoring subcommand
func NewServicesCommand(shared *shared.CommandShared) *cobra.Command {
	servicesCmd := &ServicesCommand{
		shared:  shared,
		flags:   NewServicesFlags(),
		output:  NewMonitoringOutput(shared),
		monitor: NewMonitoringService(shared),
	}

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
  deploy-cli monitoring services alt-backend production

  # Monitor multiple services
  deploy-cli monitoring services alt-backend,postgres,meilisearch production

  # Monitor with detailed metrics
  deploy-cli monitoring services alt-backend production --metrics

  # Monitor with log streaming
  deploy-cli monitoring services alt-backend production --logs --lines 100

Available Services:
• Application: alt-backend, auth-service, alt-frontend
• Infrastructure: postgres, clickhouse, meilisearch, nginx
• Processing: pre-processor, search-indexer, tag-generator
• Operational: migrate, backup, monitoring`,
		Args: cobra.MaximumNArgs(2),
		RunE: servicesCmd.run,
		PersistentPreRunE: shared.PersistentPreRunE,
	}

	// Add services-specific flags
	servicesCmd.flags.AddToCommand(cmd)

	return cmd
}

// run executes the services monitoring command
func (s *ServicesCommand) run(cmd *cobra.Command, args []string) error {
	// Parse arguments
	services, env, err := s.parseArguments(args)
	if err != nil {
		return fmt.Errorf("argument parsing failed: %w", err)
	}

	// Parse services flags
	servicesOptions, err := s.flags.ParseFromCommand(cmd)
	if err != nil {
		return fmt.Errorf("flag parsing failed: %w", err)
	}

	// Validate services and options
	if err := s.validateServicesOptions(services, servicesOptions); err != nil {
		return fmt.Errorf("services options validation failed: %w", err)
	}

	// Create monitoring context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Print startup message
	s.output.PrintServicesMonitoringStart(services, env, servicesOptions)

	// Run services monitoring
	return s.monitor.MonitorServices(ctx, services, env, servicesOptions)
}

// parseArguments parses services and environment from command arguments
func (s *ServicesCommand) parseArguments(args []string) ([]string, domain.Environment, error) {
	var services []string
	var env domain.Environment = domain.Development

	// Parse services list
	if len(args) >= 1 {
		services = strings.Split(args[0], ",")
		// Trim whitespace from service names
		for i, service := range services {
			services[i] = strings.TrimSpace(service)
		}
	}

	// Parse environment
	if len(args) == 2 {
		parsedEnv, err := domain.ParseEnvironment(args[1])
		if err != nil {
			return nil, "", fmt.Errorf("invalid environment '%s': %w", args[1], err)
		}
		env = parsedEnv
	}

	s.shared.Logger.InfoWithContext("services monitoring arguments parsed", map[string]interface{}{
		"services":    services,
		"environment": env,
	})

	return services, env, nil
}

// validateServicesOptions validates services list and monitoring options
func (s *ServicesCommand) validateServicesOptions(services []string, options *ServicesOptions) error {
	// Validate service names if provided
	if len(services) > 0 {
		for _, service := range services {
			if err := s.validateServiceName(service); err != nil {
				return fmt.Errorf("invalid service name '%s': %w", service, err)
			}
		}
	}

	// Validate log lines count
	if options.Lines < 0 {
		return fmt.Errorf("log lines count must be non-negative, got: %d", options.Lines)
	}

	// Warn about very large log line counts
	if options.Lines > 1000 {
		s.shared.Logger.WarnWithContext("large log line count may impact performance", map[string]interface{}{
			"lines":          options.Lines,
			"recommended_max": 1000,
		})
	}

	// Validate incompatible options
	if !options.Logs && options.Follow {
		return fmt.Errorf("--follow requires --logs to be enabled")
	}

	return nil
}

// validateServiceName validates a service name against known services
func (s *ServicesCommand) validateServiceName(service string) error {
	if strings.TrimSpace(service) == "" {
		return fmt.Errorf("service name cannot be empty")
	}

	// Define known services (from TASK.md analysis)
	knownServices := map[string]bool{
		// Application services
		"alt-backend":    true,
		"auth-service":   true,
		"alt-frontend":   true,
		// Infrastructure services
		"postgres":       true,
		"auth-postgres":  true,
		"kratos-postgres": true,
		"clickhouse":     true,
		"meilisearch":    true,
		"nginx":          true,
		// Processing services
		"pre-processor":  true,
		"search-indexer": true,
		"tag-generator":  true,
		// Operational services
		"migrate":        true,
		"backup":         true,
		"monitoring":     true,
	}

	if !knownServices[service] {
		s.shared.Logger.WarnWithContext("unknown service name, proceeding anyway", map[string]interface{}{
			"service": service,
			"known":   false,
		})
	}

	return nil
}

// ServicesOptions represents services monitoring configuration
type ServicesOptions struct {
	Metrics bool
	Logs    bool
	Lines   int
	Follow  bool
	Watch   bool
	Details bool
}

// ServicesFlags manages services monitoring command flags
type ServicesFlags struct {
	Metrics bool
	Logs    bool
	Lines   int
	Follow  bool
	Watch   bool
	Details bool
}

// NewServicesFlags creates services flags with defaults
func NewServicesFlags() *ServicesFlags {
	return &ServicesFlags{
		Metrics: false,
		Logs:    false,
		Lines:   50,
		Follow:  false,
		Watch:   true,
		Details: false,
	}
}

// AddToCommand adds services flags to the command
func (f *ServicesFlags) AddToCommand(cmd *cobra.Command) {
	cmd.Flags().Bool("metrics", f.Metrics, 
		"Include detailed performance metrics")
	cmd.Flags().Bool("logs", f.Logs, 
		"Stream recent logs for monitored services")
	cmd.Flags().Int("lines", f.Lines, 
		"Number of log lines to show initially")
	cmd.Flags().Bool("follow", f.Follow, 
		"Follow log output continuously")
	cmd.Flags().Bool("watch", f.Watch, 
		"Continuously monitor service status")
	cmd.Flags().Bool("details", f.Details, 
		"Show detailed service information")
}

// ParseFromCommand parses flags from command into services options
func (f *ServicesFlags) ParseFromCommand(cmd *cobra.Command) (*ServicesOptions, error) {
	options := &ServicesOptions{}
	var err error

	if options.Metrics, err = cmd.Flags().GetBool("metrics"); err != nil {
		return nil, err
	}
	if options.Logs, err = cmd.Flags().GetBool("logs"); err != nil {
		return nil, err
	}
	if options.Lines, err = cmd.Flags().GetInt("lines"); err != nil {
		return nil, err
	}
	if options.Follow, err = cmd.Flags().GetBool("follow"); err != nil {
		return nil, err
	}
	if options.Watch, err = cmd.Flags().GetBool("watch"); err != nil {
		return nil, err
	}
	if options.Details, err = cmd.Flags().GetBool("details"); err != nil {
		return nil, err
	}

	return options, nil
}