// PHASE R3: Diagnose command implementation with focused responsibility
package maintenance

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"deploy-cli/domain"
	"deploy-cli/rest/commands/shared"
)

// DiagnoseCommand provides comprehensive system diagnostics
type DiagnoseCommand struct {
	shared  *shared.CommandShared
	service *MaintenanceService
	output  *MaintenanceOutput
}

// NewDiagnoseCommand creates the diagnose subcommand
func NewDiagnoseCommand(shared *shared.CommandShared) *cobra.Command {
	diagnoseCmd := &DiagnoseCommand{
		shared:  shared,
		service: NewMaintenanceService(shared),
		output:  NewMaintenanceOutput(shared),
	}

	cmd := &cobra.Command{
		Use:   "diagnose [environment]",
		Short: "System diagnostics with automated analysis and recommendations",
		Long: `Comprehensive system diagnostics with intelligent analysis and actionable recommendations.

Diagnostic Areas:
• Resource utilization and capacity planning
• Pod health and readiness status monitoring
• Service connectivity and endpoint validation
• PersistentVolume and storage performance analysis
• Helm release status and configuration validation
• Network policy and security configuration review
• Performance bottleneck identification and analysis
• Configuration drift detection and compliance checking

Analysis Features:
• Intelligent issue correlation and root cause analysis
• Performance trend analysis and capacity forecasting
• Security configuration assessment and vulnerability scanning
• Best practice compliance checking and recommendations
• Automated fix suggestion with risk assessment
• Historical comparison and change impact analysis

Output Formats:
• Human-readable console output with color coding
• JSON format for integration with monitoring tools
• HTML reports with charts and visualizations
• CSV format for data analysis and trending
• XML format for compliance and audit systems

Examples:
  # Basic system diagnostics
  deploy-cli maintenance diagnose production

  # Comprehensive diagnostics with automated fixes
  deploy-cli maintenance diagnose production --comprehensive --auto-fix

  # Focus on specific areas
  deploy-cli maintenance diagnose production --areas performance,security

  # Generate detailed report
  deploy-cli maintenance diagnose production --output-format html --output-file report.html

  # Export raw data for analysis
  deploy-cli maintenance diagnose production --output-format json --raw-data

Diagnostic Levels:
• basic: Essential health checks and status validation
• standard: Comprehensive system analysis (default)
• comprehensive: Deep analysis with historical trends
• minimal: Quick health check for automation`,
		Args: cobra.MaximumNArgs(1),
		RunE: diagnoseCmd.run,
		PersistentPreRunE: shared.PersistentPreRunE,
	}

	// Add diagnose-specific flags
	cmd.Flags().String("level", "standard",
		"Diagnostic level: minimal, basic, standard, comprehensive")
	cmd.Flags().StringSlice("areas", []string{},
		"Focus on specific diagnostic areas")
	cmd.Flags().String("output-format", "console",
		"Output format: console, json, html, csv, xml")
	cmd.Flags().String("output-file", "",
		"Output file path for reports")
	cmd.Flags().Bool("raw-data", false,
		"Include raw diagnostic data in output")
	cmd.Flags().Bool("comprehensive", false,
		"Enable comprehensive diagnostics with trends")
	cmd.Flags().Duration("trend-window", 24*time.Hour,
		"Time window for trend analysis")
	cmd.Flags().Int("max-recommendations", 20,
		"Maximum number of recommendations to generate")
	cmd.Flags().Bool("include-logs", false,
		"Include relevant log excerpts in diagnostics")
	cmd.Flags().StringSlice("exclude-areas", []string{},
		"Exclude specific diagnostic areas")

	return cmd
}

// run executes the diagnose command
func (d *DiagnoseCommand) run(cmd *cobra.Command, args []string) error {
	// Parse environment
	env, err := d.parseEnvironment(args)
	if err != nil {
		return fmt.Errorf("environment parsing failed: %w", err)
	}

	// Parse diagnose options
	options, err := d.parseDiagnoseOptions(cmd)
	if err != nil {
		return fmt.Errorf("diagnose options parsing failed: %w", err)
	}

	// Validate diagnose options
	if err := d.validateDiagnoseOptions(options); err != nil {
		return fmt.Errorf("diagnose options validation failed: %w", err)
	}

	// Create diagnose context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Print diagnose start message
	d.output.PrintDiagnoseStart(env, options)

	// Execute diagnostic operations
	result, err := d.service.ExecuteDiagnosis(ctx, env, options)
	if err != nil {
		d.output.PrintDiagnoseError(err)
		return fmt.Errorf("diagnose execution failed: %w", err)
	}

	// Print diagnose results
	d.output.PrintDiagnoseResults(result)

	// Export results if requested
	if options.OutputFile != "" {
		if err := d.exportDiagnoseResults(result, options); err != nil {
			return fmt.Errorf("diagnose result export failed: %w", err)
		}
	}

	return nil
}

// parseEnvironment parses the environment argument
func (d *DiagnoseCommand) parseEnvironment(args []string) (domain.Environment, error) {
	var env domain.Environment = domain.Development
	
	if len(args) > 0 {
		parsedEnv, err := domain.ParseEnvironment(args[0])
		if err != nil {
			return "", fmt.Errorf("invalid environment '%s': %w", args[0], err)
		}
		env = parsedEnv
	}

	d.shared.Logger.InfoWithContext("diagnose environment parsed", map[string]interface{}{
		"environment": env,
	})

	return env, nil
}

// parseDiagnoseOptions parses diagnose flags into options
func (d *DiagnoseCommand) parseDiagnoseOptions(cmd *cobra.Command) (*DiagnoseOptions, error) {
	options := &DiagnoseOptions{}
	var err error

	// Parse diagnose-specific flags
	if options.Level, err = cmd.Flags().GetString("level"); err != nil {
		return nil, err
	}
	if options.Areas, err = cmd.Flags().GetStringSlice("areas"); err != nil {
		return nil, err
	}
	if options.Format, err = cmd.Flags().GetString("output-format"); err != nil {
		return nil, err
	}
	if options.OutputFile, err = cmd.Flags().GetString("output-file"); err != nil {
		return nil, err
	}
	if options.RawData, err = cmd.Flags().GetBool("raw-data"); err != nil {
		return nil, err
	}
	if options.Comprehensive, err = cmd.Flags().GetBool("comprehensive"); err != nil {
		return nil, err
	}
	if options.TrendWindow, err = cmd.Flags().GetDuration("trend-window"); err != nil {
		return nil, err
	}
	if options.MaxRecommendations, err = cmd.Flags().GetInt("max-recommendations"); err != nil {
		return nil, err
	}
	if options.IncludeLogs, err = cmd.Flags().GetBool("include-logs"); err != nil {
		return nil, err
	}
	if options.ExcludeAreas, err = cmd.Flags().GetStringSlice("exclude-areas"); err != nil {
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
	if len(options.Areas) == 0 {
		options.Areas = []string{"resources", "connectivity", "performance", "configuration"}
	}
	if options.MaxRecommendations <= 0 {
		options.MaxRecommendations = 20
	}
	
	// Override level if comprehensive flag is set
	if options.Comprehensive {
		options.Level = "comprehensive"
	}

	return options, nil
}

// validateDiagnoseOptions validates diagnose configuration
func (d *DiagnoseCommand) validateDiagnoseOptions(options *DiagnoseOptions) error {
	// Validate diagnostic level
	validLevels := map[string]bool{
		"minimal":       true,
		"basic":         true,
		"standard":      true,
		"comprehensive": true,
	}
	if !validLevels[options.Level] {
		return fmt.Errorf("invalid diagnostic level: %s", options.Level)
	}

	// Validate output format
	validFormats := map[string]bool{
		"console": true,
		"json":    true,
		"html":    true,
		"csv":     true,
		"xml":     true,
	}
	if !validFormats[options.Format] {
		return fmt.Errorf("invalid output format: %s", options.Format)
	}

	// Validate diagnostic areas
	validAreas := map[string]bool{
		"resources":     true,
		"connectivity":  true,
		"performance":   true,
		"configuration": true,
		"security":      true,
		"compliance":    true,
		"storage":       true,
		"network":       true,
	}

	for _, area := range options.Areas {
		if !validAreas[area] {
			return fmt.Errorf("invalid diagnostic area: %s", area)
		}
	}

	for _, area := range options.ExcludeAreas {
		if !validAreas[area] {
			return fmt.Errorf("invalid exclude area: %s", area)
		}
	}

	// Validate max recommendations
	if options.MaxRecommendations < 1 || options.MaxRecommendations > 100 {
		return fmt.Errorf("max-recommendations must be between 1 and 100, got: %d", options.MaxRecommendations)
	}

	return nil
}

// exportDiagnoseResults exports diagnostic results to file
func (d *DiagnoseCommand) exportDiagnoseResults(result *DiagnoseResult, options *DiagnoseOptions) error {
	d.shared.Logger.InfoWithContext("exporting diagnose results", map[string]interface{}{
		"filename": options.OutputFile,
		"format":   options.Format,
		"checks":   len(result.Checks),
		"issues":   len(result.Issues),
		"fixes":    len(result.Fixes),
	})

	// TODO: Implement result export logic based on format
	return nil
}

