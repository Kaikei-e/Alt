// PHASE R3: Troubleshoot command implementation with focused responsibility
package maintenance

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"deploy-cli/domain"
	"deploy-cli/rest/commands/shared"
)

// TroubleshootCommand provides comprehensive troubleshooting functionality
type TroubleshootCommand struct {
	shared  *shared.CommandShared
	service *MaintenanceService
	output  *MaintenanceOutput
}

// NewTroubleshootCommand creates the troubleshoot subcommand
func NewTroubleshootCommand(shared *shared.CommandShared) *cobra.Command {
	troubleshootCmd := &TroubleshootCommand{
		shared:  shared,
		service: NewMaintenanceService(shared),
		output:  NewMaintenanceOutput(shared),
	}

	cmd := &cobra.Command{
		Use:   "troubleshoot [environment]",
		Short: "Comprehensive troubleshooting and diagnosis tools",
		Long: `Interactive troubleshooting and diagnosis suite for deployment issues.

Troubleshooting Operations:
• Pod status analysis and failure diagnostics
• Service connectivity and endpoint validation
• PersistentVolume and storage issue detection
• Helm release status and rollback analysis
• Network connectivity and DNS resolution
• Resource usage and capacity planning
• Configuration validation and drift detection

Interactive Features:
• Step-by-step guided troubleshooting
• Automated issue detection and classification
• Suggested fixes with safety validation
• Real-time monitoring during fixes
• Rollback capabilities for failed fixes

Automated Analysis:
• Log aggregation and error pattern detection
• Resource dependency mapping
• Performance bottleneck identification
• Security configuration validation
• Compliance and best practice checks

Examples:
  # Interactive troubleshooting session
  deploy-cli maintenance troubleshoot production --interactive

  # Automated troubleshooting with fixes
  deploy-cli maintenance troubleshoot production --auto-fix

  # Specific component troubleshooting
  deploy-cli maintenance troubleshoot production --component alt-backend

  # Export troubleshooting report
  deploy-cli maintenance troubleshoot production --output-file report.json

Troubleshoot Categories:
• connectivity: Network and service connectivity issues
• resources: Resource allocation and capacity problems
• configuration: Config drift and validation issues
• performance: Performance bottlenecks and optimization
• security: Security misconfigurations and vulnerabilities`,
		Args: cobra.MaximumNArgs(1),
		RunE: troubleshootCmd.run,
		PersistentPreRunE: shared.PersistentPreRunE,
	}

	// Add troubleshoot-specific flags
	cmd.Flags().Bool("interactive", false,
		"Enable interactive troubleshooting mode")
	cmd.Flags().String("component", "",
		"Focus troubleshooting on specific component")
	cmd.Flags().StringSlice("categories", []string{},
		"Limit troubleshooting to specific categories")
	cmd.Flags().String("output-file", "",
		"Export troubleshooting report to file")
	cmd.Flags().Bool("export-logs", false,
		"Export relevant logs for analysis")
	cmd.Flags().Duration("log-window", 0,
		"Time window for log analysis (e.g., 1h, 30m)")
	cmd.Flags().Int("max-issues", 50,
		"Maximum number of issues to analyze")

	return cmd
}

// run executes the troubleshoot command
func (t *TroubleshootCommand) run(cmd *cobra.Command, args []string) error {
	// Parse environment
	env, err := t.parseEnvironment(args)
	if err != nil {
		return fmt.Errorf("environment parsing failed: %w", err)
	}

	// Parse troubleshoot options
	options, err := t.parseTroubleshootOptions(cmd)
	if err != nil {
		return fmt.Errorf("troubleshoot options parsing failed: %w", err)
	}

	// Validate troubleshoot options
	if err := t.validateTroubleshootOptions(options); err != nil {
		return fmt.Errorf("troubleshoot options validation failed: %w", err)
	}

	// Create troubleshoot context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Print troubleshoot start message
	t.output.PrintTroubleshootStart(env, options)

	// Execute troubleshooting operations
	result, err := t.service.ExecuteTroubleshooting(ctx, env, options)
	if err != nil {
		t.output.PrintTroubleshootError(err)
		return fmt.Errorf("troubleshoot execution failed: %w", err)
	}

	// Print troubleshoot results
	t.output.PrintTroubleshootResults(result)

	// Export results if requested
	if options.OutputFile != "" {
		if err := t.exportTroubleshootResults(result, options.OutputFile); err != nil {
			return fmt.Errorf("troubleshoot result export failed: %w", err)
		}
	}

	return nil
}

// parseEnvironment parses the environment argument
func (t *TroubleshootCommand) parseEnvironment(args []string) (domain.Environment, error) {
	var env domain.Environment = domain.Development

	if len(args) > 0 {
		parsedEnv, err := domain.ParseEnvironment(args[0])
		if err != nil {
			return "", fmt.Errorf("invalid environment '%s': %w", args[0], err)
		}
		env = parsedEnv
	}

	t.shared.Logger.InfoWithContext("troubleshoot environment parsed", map[string]interface{}{
		"environment": env,
	})

	return env, nil
}

// parseTroubleshootOptions parses troubleshoot flags into options
func (t *TroubleshootCommand) parseTroubleshootOptions(cmd *cobra.Command) (*TroubleshootOptions, error) {
	options := &TroubleshootOptions{}
	var err error

	// Parse troubleshoot-specific flags
	if options.Interactive, err = cmd.Flags().GetBool("interactive"); err != nil {
		return nil, err
	}
	if options.Component, err = cmd.Flags().GetString("component"); err != nil {
		return nil, err
	}
	if options.Categories, err = cmd.Flags().GetStringSlice("categories"); err != nil {
		return nil, err
	}
	if options.OutputFile, err = cmd.Flags().GetString("output-file"); err != nil {
		return nil, err
	}
	if options.ExportLogs, err = cmd.Flags().GetBool("export-logs"); err != nil {
		return nil, err
	}
	if options.LogWindow, err = cmd.Flags().GetDuration("log-window"); err != nil {
		return nil, err
	}
	if options.MaxIssues, err = cmd.Flags().GetInt("max-issues"); err != nil {
		return nil, err
	}

	// Parse global maintenance flags
	if options.AutoFix, err = cmd.Flags().GetBool("auto-fix"); err != nil {
		return nil, err
	}
	if options.DryRun, err = cmd.Flags().GetBool("dry-run"); err != nil {
		return nil, err
	}
	if options.Verbose, err = cmd.Flags().GetBool("verbose"); err != nil {
		return nil, err
	}
	if options.Timeout, err = cmd.Flags().GetDuration("timeout"); err != nil {
		return nil, err
	}

	// Set defaults
	if len(options.Categories) == 0 {
		options.Categories = []string{"connectivity", "resources", "configuration"}
	}
	if options.MaxIssues <= 0 {
		options.MaxIssues = 50
	}

	return options, nil
}

// validateTroubleshootOptions validates troubleshoot configuration
func (t *TroubleshootCommand) validateTroubleshootOptions(options *TroubleshootOptions) error {
	// Validate categories
	validCategories := map[string]bool{
		"connectivity":   true,
		"resources":      true,
		"configuration":  true,
		"performance":    true,
		"security":       true,
	}

	for _, category := range options.Categories {
		if !validCategories[category] {
			return fmt.Errorf("invalid troubleshoot category: %s", category)
		}
	}

	// Validate max issues limit
	if options.MaxIssues < 1 || options.MaxIssues > 1000 {
		return fmt.Errorf("max-issues must be between 1 and 1000, got: %d", options.MaxIssues)
	}

	return nil
}

// exportTroubleshootResults exports troubleshooting results to file
func (t *TroubleshootCommand) exportTroubleshootResults(result *TroubleshootResult, filename string) error {
	t.shared.Logger.InfoWithContext("exporting troubleshoot results", map[string]interface{}{
		"filename": filename,
		"issues":   len(result.Issues),
		"fixes":    len(result.Fixes),
	})

	// TODO: Implement result export logic
	return nil
}

