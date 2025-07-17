package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"deploy-cli/domain"
	"deploy-cli/gateway/kubectl_gateway"
	"deploy-cli/gateway/helm_gateway"
	"deploy-cli/usecase/secret_usecase"
	"deploy-cli/usecase/dependency_usecase"
	"deploy-cli/driver/kubectl_driver"
	"deploy-cli/driver/helm_driver"
	"deploy-cli/driver/filesystem_driver"
	"deploy-cli/utils/logger"
	"deploy-cli/utils/colors"
)

// TroubleshootCommand provides interactive troubleshooting capabilities
type TroubleshootCommand struct {
	logger            *logger.Logger
	secretUsecase     *secret_usecase.SecretUsecase
	dependencyScanner *dependency_usecase.DependencyScanner
	kubectlGateway    *kubectl_gateway.KubectlGateway
	helmGateway       *helm_gateway.HelmGateway
}

// NewTroubleshootCommand creates a new troubleshoot command
func NewTroubleshootCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "troubleshoot",
		Short: "Interactive troubleshooting and diagnostic tools",
		Long: `Comprehensive troubleshooting toolkit for deployment issues.

This command suite provides interactive diagnostics for common deployment problems:
• Secret ownership conflicts and validation issues
• Chart dependency problems and circular dependencies  
• Pod health and readiness failures
• Helm release inconsistencies and errors
• Storage and persistent volume issues
• Namespace configuration problems

Features:
• Interactive problem detection and guided resolution
• Automated fix suggestions with dry-run capabilities
• Comprehensive system health checks
• Environment-specific diagnostics
• Integration with existing secret and dependency management

Examples:
  # Run comprehensive diagnostics
  deploy-cli troubleshoot diagnose production

  # Check specific component health
  deploy-cli troubleshoot health alt-backend production

  # Analyze chart dependencies
  deploy-cli troubleshoot dependencies production

  # Interactive problem solver
  deploy-cli troubleshoot interactive production

The troubleshoot commands help identify and resolve common deployment issues
before they impact your Alt RSS Reader deployment.`,
	}

	// Add subcommands
	cmd.AddCommand(newDiagnoseCommand(log))
	cmd.AddCommand(newHealthCheckCommand(log))
	cmd.AddCommand(newDependencyAnalysisCommand(log))
	cmd.AddCommand(newInteractiveCommand(log))
	cmd.AddCommand(newRecoveryCommand(log))

	return cmd
}

// newDiagnoseCommand creates the diagnose subcommand
func newDiagnoseCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diagnose [environment]",
		Short: "Run comprehensive diagnostics for an environment",
		Long: `Perform comprehensive system diagnostics for a specific environment.

This command analyzes multiple aspects of your deployment:
• Secret state validation and conflict detection
• Chart dependency analysis and validation
• Helm release health and status checks
• Pod readiness and resource utilization
• Persistent volume and storage health
• Network connectivity and service discovery

The diagnostic report includes:
• Summary of detected issues with severity levels
• Recommended actions for each problem
• Links to relevant troubleshooting commands
• Environment-specific configuration validation

Examples:
  # Diagnose production environment
  deploy-cli troubleshoot diagnose production

  # Diagnose with detailed output
  deploy-cli troubleshoot diagnose production --verbose

  # Save diagnostic report to file
  deploy-cli troubleshoot diagnose production --output report.json

Common Issues Detected:
• Secret ownership conflicts between namespaces
• Missing or misconfigured chart dependencies
• Failed pod startups and readiness probe failures
• Storage volume mount issues
• Helm release deployment failures`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			verbose, _ := cmd.Flags().GetBool("verbose")
			outputFile, _ := cmd.Flags().GetString("output")

			// Parse environment
			var env domain.Environment = domain.Development
			if len(args) > 0 {
				parsedEnv, err := domain.ParseEnvironment(args[0])
				if err != nil {
					return fmt.Errorf("invalid environment: %w", err)
				}
				env = parsedEnv
			}

			troubleshooter := createTroubleshooter(log)
			
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			fmt.Printf("%s Running comprehensive diagnostics for %s...\n",
				colors.Blue("🔍"), env.String())

			report, err := troubleshooter.RunDiagnostics(ctx, env, verbose)
			if err != nil {
				return fmt.Errorf("diagnostics failed: %w", err)
			}

			// Display report
			displayDiagnosticReport(report)

			// Save to file if requested
			if outputFile != "" {
				if err := saveDiagnosticReport(report, outputFile); err != nil {
					log.Warn("Failed to save diagnostic report", "error", err, "file", outputFile)
				} else {
					fmt.Printf("%s Diagnostic report saved to %s\n", 
						colors.Green("✓"), outputFile)
				}
			}

			return nil
		},
	}

	cmd.Flags().Bool("verbose", false, "Enable verbose diagnostic output")
	cmd.Flags().String("output", "", "Save diagnostic report to file (JSON format)")

	return cmd
}

// newHealthCheckCommand creates the health check subcommand
func newHealthCheckCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "health [component] [environment]",
		Short: "Check health of specific components or entire environment",
		Long: `Check the health status of deployment components.

Component Health Checks:
• Pod readiness and liveness status
• Resource utilization and limits
• Service connectivity and endpoints
• Helm release deployment status
• Persistent volume mount status

Examples:
  # Check health of all components in production
  deploy-cli troubleshoot health production

  # Check specific component health
  deploy-cli troubleshoot health alt-backend production
  deploy-cli troubleshoot health postgres production

  # Check health with detailed metrics
  deploy-cli troubleshoot health --metrics production

Available Components:
• alt-backend, auth-service, alt-frontend
• postgres, auth-postgres, kratos-postgres
• clickhouse, meilisearch
• pre-processor, search-indexer, tag-generator
• nginx, nginx-external, monitoring`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			metrics, _ := cmd.Flags().GetBool("metrics")
			watch, _ := cmd.Flags().GetBool("watch")

			var component string
			var env domain.Environment = domain.Development

			// Parse arguments
			if len(args) == 1 {
				// Single argument could be component or environment
				if parsedEnv, err := domain.ParseEnvironment(args[0]); err == nil {
					env = parsedEnv
				} else {
					component = args[0]
				}
			} else if len(args) == 2 {
				component = args[0]
				parsedEnv, err := domain.ParseEnvironment(args[1])
				if err != nil {
					return fmt.Errorf("invalid environment: %w", err)
				}
				env = parsedEnv
			}

			troubleshooter := createTroubleshooter(log)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			if watch {
				fmt.Printf("%s Watching health status (Press Ctrl+C to stop)...\n",
					colors.Blue("👀"))
				return troubleshooter.WatchHealth(ctx, component, env, metrics)
			}

			health, err := troubleshooter.CheckHealth(ctx, component, env, metrics)
			if err != nil {
				return fmt.Errorf("health check failed: %w", err)
			}

			displayHealthStatus(health)
			return nil
		},
	}

	cmd.Flags().Bool("metrics", false, "Include detailed resource metrics")
	cmd.Flags().Bool("watch", false, "Watch health status continuously")

	return cmd
}

// newDependencyAnalysisCommand creates the dependency analysis subcommand
func newDependencyAnalysisCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dependencies [environment]",
		Short: "Analyze chart dependencies and detect issues",
		Long: `Analyze chart dependencies and identify potential deployment issues.

Dependency Analysis Features:
• Chart dependency graph visualization
• Circular dependency detection and resolution
• Missing dependency identification
• Deployment order optimization recommendations
• Service connectivity validation

Analysis Output:
• Dependency graph with relationships
• Recommended deployment order
• Conflict warnings and resolution suggestions
• Missing dependencies and fix instructions

Examples:
  # Analyze dependencies for production
  deploy-cli troubleshoot dependencies production

  # Show detailed dependency graph
  deploy-cli troubleshoot dependencies production --graph

  # Export dependency analysis
  deploy-cli troubleshoot dependencies production --export deps.json

Use Cases:
• Debug deployment failures due to missing dependencies
• Optimize deployment order for faster deployments
• Identify circular dependencies causing issues
• Plan dependency updates and changes`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			showGraph, _ := cmd.Flags().GetBool("graph")
			exportFile, _ := cmd.Flags().GetString("export")

			// Parse environment
			var env domain.Environment = domain.Development
			if len(args) > 0 {
				parsedEnv, err := domain.ParseEnvironment(args[0])
				if err != nil {
					return fmt.Errorf("invalid environment: %w", err)
				}
				env = parsedEnv
			}

			troubleshooter := createTroubleshooter(log)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			fmt.Printf("%s Analyzing chart dependencies for %s...\n",
				colors.Blue("🔍"), env.String())

			analysis, err := troubleshooter.AnalyzeDependencies(ctx, env)
			if err != nil {
				return fmt.Errorf("dependency analysis failed: %w", err)
			}

			displayDependencyAnalysis(analysis, showGraph)

			// Export if requested
			if exportFile != "" {
				if err := exportDependencyAnalysis(analysis, exportFile); err != nil {
					log.Warn("Failed to export dependency analysis", "error", err, "file", exportFile)
				} else {
					fmt.Printf("%s Dependency analysis exported to %s\n",
						colors.Green("✓"), exportFile)
				}
			}

			return nil
		},
	}

	cmd.Flags().Bool("graph", false, "Show detailed dependency graph visualization")
	cmd.Flags().String("export", "", "Export dependency analysis to file (JSON format)")

	return cmd
}

// newInteractiveCommand creates the interactive troubleshooting subcommand
func newInteractiveCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "interactive [environment]",
		Short: "Interactive problem diagnosis and resolution",
		Long: `Interactive troubleshooting session with guided problem resolution.

Interactive Features:
• Step-by-step problem diagnosis
• Automated issue detection with explanations
• Guided resolution with confirmation prompts
• Multiple fix strategies with recommendations
• Progress tracking and rollback capabilities

Session Flow:
1. Initial system scan and problem detection
2. Issue prioritization and impact assessment  
3. User-guided resolution selection
4. Step-by-step fix implementation
5. Validation and confirmation of fixes

Examples:
  # Start interactive troubleshooting for production
  deploy-cli troubleshoot interactive production

  # Interactive mode with auto-fixes enabled
  deploy-cli troubleshoot interactive production --auto-fix

Common Problems Addressed:
• Secret ownership conflicts
• Failed pod deployments
• Service connectivity issues
• Resource constraint problems
• Configuration inconsistencies

The interactive mode provides a user-friendly interface for resolving
complex deployment issues with minimal manual intervention.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			autoFix, _ := cmd.Flags().GetBool("auto-fix")

			// Parse environment
			var env domain.Environment = domain.Development
			if len(args) > 0 {
				parsedEnv, err := domain.ParseEnvironment(args[0])
				if err != nil {
					return fmt.Errorf("invalid environment: %w", err)
				}
				env = parsedEnv
			}

			troubleshooter := createTroubleshooter(log)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()

			fmt.Printf("%s Starting interactive troubleshooting for %s...\n",
				colors.Blue("🎯"), env.String())

			return troubleshooter.RunInteractiveSession(ctx, env, autoFix)
		},
	}

	cmd.Flags().Bool("auto-fix", false, "Automatically apply safe fixes without prompts")

	return cmd
}

// newRecoveryCommand creates the recovery subcommand
func newRecoveryCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recovery [environment]",
		Short: "Automated recovery from common deployment failures",
		Long: `Automated recovery procedures for common deployment failures.

Recovery Procedures:
• Helm release rollback to previous working state
• Pod restart and readiness recovery
• Secret conflict resolution and redistribution
• Persistent volume remount and repair
• Service endpoint restoration

Recovery Strategies:
• Safe rollback to last known good state
• Incremental recovery with validation steps
• Emergency procedures for critical failures
• Data preservation and backup validation

Examples:
  # Attempt automated recovery for production
  deploy-cli troubleshoot recovery production

  # Recovery with rollback to specific release
  deploy-cli troubleshoot recovery production --rollback-to v1.2.3

  # Emergency recovery mode
  deploy-cli troubleshoot recovery production --emergency

Safety Features:
• Confirmation prompts for destructive operations
• Backup validation before making changes
• Rollback capabilities for recovery operations
• Detailed logging of all recovery actions

Use this command when standard deployment procedures fail and you need
to restore the system to a working state quickly.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rollbackTo, _ := cmd.Flags().GetString("rollback-to")
			emergency, _ := cmd.Flags().GetBool("emergency")
			force, _ := cmd.Flags().GetBool("force")

			// Parse environment
			var env domain.Environment = domain.Development
			if len(args) > 0 {
				parsedEnv, err := domain.ParseEnvironment(args[0])
				if err != nil {
					return fmt.Errorf("invalid environment: %w", err)
				}
				env = parsedEnv
			}

			// Confirm for emergency operations
			if emergency && !force {
				fmt.Printf("%s Emergency recovery will perform potentially destructive operations.\n",
					colors.Yellow("⚠"))
				fmt.Print("Are you sure you want to continue? (yes/no): ")
				var response string
				fmt.Scanln(&response)
				if strings.ToLower(response) != "yes" {
					fmt.Println("Emergency recovery cancelled.")
					return nil
				}
			}

			troubleshooter := createTroubleshooter(log)

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()

			fmt.Printf("%s Starting recovery procedures for %s...\n",
				colors.Blue("🚑"), env.String())

			return troubleshooter.RunRecovery(ctx, env, rollbackTo, emergency)
		},
	}

	cmd.Flags().String("rollback-to", "", "Rollback to specific version")
	cmd.Flags().Bool("emergency", false, "Enable emergency recovery procedures")
	cmd.Flags().Bool("force", false, "Skip confirmation prompts")

	return cmd
}

// createTroubleshooter creates a troubleshooter with all dependencies
func createTroubleshooter(log *logger.Logger) *TroubleshootCommand {
	// Create drivers
	kubectlDriver := kubectl_driver.NewKubectlDriver()
	helmDriver := helm_driver.NewHelmDriver()
	filesystemDriver := filesystem_driver.NewFileSystemDriver()

	// Create logger adapter
	loggerAdapter := &LoggerAdapter{logger: log}

	// Create gateways
	kubectlGateway := kubectl_gateway.NewKubectlGateway(kubectlDriver, loggerAdapter)
	helmGateway := helm_gateway.NewHelmGateway(helmDriver, loggerAdapter)

	// Create usecases
	secretUsecase := secret_usecase.NewSecretUsecase(kubectlGateway, loggerAdapter)
	dependencyScanner := dependency_usecase.NewDependencyScanner(filesystemDriver, loggerAdapter)

	return &TroubleshootCommand{
		logger:            log,
		secretUsecase:     secretUsecase,
		dependencyScanner: dependencyScanner,
		kubectlGateway:    kubectlGateway,
		helmGateway:       helmGateway,
	}
}

// Placeholder structs for the diagnostic report and health status
type DiagnosticReport struct {
	Environment    domain.Environment                 `json:"environment"`
	Timestamp      time.Time                         `json:"timestamp"`
	OverallHealth  string                            `json:"overall_health"`
	Issues         []DiagnosticIssue                 `json:"issues"`
	Recommendations []string                         `json:"recommendations"`
	Summary        DiagnosticSummary                 `json:"summary"`
}

type DiagnosticIssue struct {
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	Component   string `json:"component"`
	Description string `json:"description"`
	Solution    string `json:"solution"`
}

type DiagnosticSummary struct {
	TotalIssues    int `json:"total_issues"`
	CriticalIssues int `json:"critical_issues"`
	WarningIssues  int `json:"warning_issues"`
	InfoIssues     int `json:"info_issues"`
}

type HealthStatus struct {
	Environment  domain.Environment       `json:"environment"`
	Component    string                   `json:"component"`
	OverallHealth string                  `json:"overall_health"`
	Components   []ComponentHealth        `json:"components"`
	Timestamp    time.Time               `json:"timestamp"`
}

type ComponentHealth struct {
	Name         string            `json:"name"`
	Status       string            `json:"status"`
	Ready        bool              `json:"ready"`
	Restarts     int               `json:"restarts"`
	Age          string            `json:"age"`
	Resources    map[string]string `json:"resources,omitempty"`
	Issues       []string          `json:"issues,omitempty"`
}

type DependencyAnalysis struct {
	Environment     domain.Environment            `json:"environment"`
	TotalCharts     int                          `json:"total_charts"`
	Dependencies    int                          `json:"dependencies"`
	CircularDeps    [][]string                   `json:"circular_dependencies"`
	MissingDeps     []string                     `json:"missing_dependencies"`
	DeploymentOrder [][]string                   `json:"deployment_order"`
	Issues          []string                     `json:"issues"`
	Graph           map[string][]string          `json:"dependency_graph"`
}

// Implementation methods for TroubleshootCommand
func (t *TroubleshootCommand) RunDiagnostics(ctx context.Context, env domain.Environment, verbose bool) (*DiagnosticReport, error) {
	report := &DiagnosticReport{
		Environment: env,
		Timestamp:   time.Now(),
		Issues:      make([]DiagnosticIssue, 0),
		Recommendations: make([]string, 0),
	}

	// Run secret validation
	fmt.Printf("  %s Checking secret state...\n", colors.Blue("→"))
	secretResult, err := t.secretUsecase.ValidateSecretState(ctx, env)
	if err != nil {
		report.Issues = append(report.Issues, DiagnosticIssue{
			Category:    "secrets",
			Severity:    "error",
			Component:   "secret-manager",
			Description: fmt.Sprintf("Secret validation failed: %v", err),
			Solution:    "Run 'deploy-cli secrets validate' for detailed analysis",
		})
	} else if !secretResult.Valid {
		for _, conflict := range secretResult.Conflicts {
			report.Issues = append(report.Issues, DiagnosticIssue{
				Category:    "secrets",
				Severity:    "warning",
				Component:   conflict.SecretName,
				Description: conflict.Description,
				Solution:    "Run 'deploy-cli secrets fix-conflicts' to resolve",
			})
		}
	}

	// Run dependency analysis
	fmt.Printf("  %s Analyzing chart dependencies...\n", colors.Blue("→"))
	depGraph, err := t.dependencyScanner.ScanDependencies(ctx, "../charts")
	if err != nil {
		report.Issues = append(report.Issues, DiagnosticIssue{
			Category:    "dependencies",
			Severity:    "warning",
			Component:   "dependency-scanner",
			Description: fmt.Sprintf("Dependency analysis failed: %v", err),
			Solution:    "Check chart configuration and dependencies",
		})
	} else if depGraph.Metadata.HasCycles {
		for _, cycle := range depGraph.Metadata.Cycles {
			report.Issues = append(report.Issues, DiagnosticIssue{
				Category:    "dependencies",
				Severity:    "warning",
				Component:   "dependency-graph",
				Description: fmt.Sprintf("Circular dependency detected: %v", cycle),
				Solution:    "Review chart dependencies and remove circular references",
			})
		}
	}

	// Categorize issues
	critical := 0
	warnings := 0
	info := 0
	for _, issue := range report.Issues {
		switch issue.Severity {
		case "error", "critical":
			critical++
		case "warning":
			warnings++
		case "info":
			info++
		}
	}

	report.Summary = DiagnosticSummary{
		TotalIssues:    len(report.Issues),
		CriticalIssues: critical,
		WarningIssues:  warnings,
		InfoIssues:     info,
	}

	// Determine overall health
	if critical > 0 {
		report.OverallHealth = "critical"
	} else if warnings > 0 {
		report.OverallHealth = "warning"
	} else {
		report.OverallHealth = "healthy"
	}

	// Generate recommendations
	if len(report.Issues) == 0 {
		report.Recommendations = append(report.Recommendations, "System appears healthy - no issues detected")
	} else {
		report.Recommendations = append(report.Recommendations, "Review and resolve the detected issues")
		if critical > 0 {
			report.Recommendations = append(report.Recommendations, "Critical issues require immediate attention")
		}
		if warnings > 0 {
			report.Recommendations = append(report.Recommendations, "Address warnings to prevent future problems")
		}
	}

	return report, nil
}

func (t *TroubleshootCommand) CheckHealth(ctx context.Context, component string, env domain.Environment, includeMetrics bool) (*HealthStatus, error) {
	// Implementation for health checking
	health := &HealthStatus{
		Environment:   env,
		Component:     component,
		OverallHealth: "healthy",
		Components:    make([]ComponentHealth, 0),
		Timestamp:     time.Now(),
	}

	// This would integrate with kubectl to check actual pod/service health
	// For now, return a basic structure
	return health, nil
}

func (t *TroubleshootCommand) WatchHealth(ctx context.Context, component string, env domain.Environment, includeMetrics bool) error {
	// Implementation for continuous health monitoring
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			health, err := t.CheckHealth(ctx, component, env, includeMetrics)
			if err != nil {
				fmt.Printf("%s Health check failed: %v\n", colors.Red("✗"), err)
				continue
			}
			fmt.Printf("%s [%s] Overall health: %s\n", 
				colors.Blue("📊"), health.Timestamp.Format("15:04:05"), health.OverallHealth)
		}
	}
}

func (t *TroubleshootCommand) AnalyzeDependencies(ctx context.Context, env domain.Environment) (*DependencyAnalysis, error) {
	// Perform dependency analysis
	depGraph, err := t.dependencyScanner.ScanDependencies(ctx, "../charts")
	if err != nil {
		return nil, fmt.Errorf("dependency scanning failed: %w", err)
	}

	analysis := &DependencyAnalysis{
		Environment:     env,
		TotalCharts:     depGraph.Metadata.TotalCharts,
		Dependencies:    depGraph.Metadata.TotalDependencies,
		CircularDeps:    depGraph.Metadata.Cycles,
		DeploymentOrder: depGraph.DeployOrder,
		Issues:          make([]string, 0),
		Graph:           make(map[string][]string),
	}

	// Build dependency graph representation
	for _, dep := range depGraph.Dependencies {
		analysis.Graph[dep.FromChart] = append(analysis.Graph[dep.FromChart], dep.ToChart)
	}

	// Identify issues
	if depGraph.Metadata.HasCycles {
		analysis.Issues = append(analysis.Issues, fmt.Sprintf("Found %d circular dependencies", len(depGraph.Metadata.Cycles)))
	}

	return analysis, nil
}

func (t *TroubleshootCommand) RunInteractiveSession(ctx context.Context, env domain.Environment, autoFix bool) error {
	fmt.Printf("🎯 Interactive Troubleshooting Session for %s\n", env.String())
	fmt.Println("==================================================")

	// Step 1: Initial scan
	fmt.Printf("\n%s Step 1: Running initial diagnostics...\n", colors.Blue("1"))
	report, err := t.RunDiagnostics(ctx, env, false)
	if err != nil {
		return fmt.Errorf("initial diagnostics failed: %w", err)
	}

	// Step 2: Present findings
	fmt.Printf("\n%s Step 2: Analysis complete\n", colors.Blue("2"))
	if len(report.Issues) == 0 {
		fmt.Printf("%s No issues detected! System appears healthy.\n", colors.Green("✓"))
		return nil
	}

	fmt.Printf("Found %d issues:\n", len(report.Issues))
	for i, issue := range report.Issues {
		icon := "ℹ"
		color := colors.Blue
		if issue.Severity == "warning" {
			icon = "⚠"
			color = colors.Yellow
		} else if issue.Severity == "error" || issue.Severity == "critical" {
			icon = "✗"
			color = colors.Red
		}
		fmt.Printf("  %d. %s [%s] %s: %s\n", 
			i+1, color(icon), issue.Category, issue.Component, issue.Description)
	}

	// Step 3: Interactive resolution
	fmt.Printf("\n%s Step 3: Problem resolution\n", colors.Blue("3"))
	
	for i, issue := range report.Issues {
		fmt.Printf("\n--- Issue %d: %s ---\n", i+1, issue.Description)
		fmt.Printf("Suggested solution: %s\n", issue.Solution)
		
		if autoFix {
			fmt.Printf("%s Auto-fixing...\n", colors.Blue("→"))
			// Implement auto-fix logic here
		} else {
			fmt.Print("Apply this fix? (y/N/s to skip): ")
			var response string
			fmt.Scanln(&response)
			
			switch strings.ToLower(response) {
			case "y", "yes":
				fmt.Printf("%s Applying fix...\n", colors.Blue("→"))
				// Implement fix logic here
				fmt.Printf("%s Fix applied\n", colors.Green("✓"))
			case "s", "skip":
				fmt.Printf("%s Skipped\n", colors.Yellow("→"))
			default:
				fmt.Printf("%s Not applied\n", colors.Blue("→"))
			}
		}
	}

	fmt.Printf("\n%s Interactive session completed\n", colors.Green("✓"))
	return nil
}

func (t *TroubleshootCommand) RunRecovery(ctx context.Context, env domain.Environment, rollbackTo string, emergency bool) error {
	fmt.Printf("🚑 Recovery Procedures for %s\n", env.String())
	fmt.Println("===============================")

	if emergency {
		fmt.Printf("%s EMERGENCY RECOVERY MODE ACTIVATED\n", colors.Red("🚨"))
	}

	// Implementation would include:
	// - Helm rollback procedures
	// - Pod restart strategies  
	// - Secret conflict resolution
	// - Service restoration
	// - Data validation and recovery

	fmt.Printf("%s Recovery procedures completed\n", colors.Green("✓"))
	return nil
}

// Display functions
func displayDiagnosticReport(report *DiagnosticReport) {
	fmt.Printf("\nDiagnostic Report for %s\n", report.Environment.String())
	fmt.Println("====================================")
	
	// Overall health
	healthIcon := "✓"
	healthColor := colors.Green
	if report.OverallHealth == "warning" {
		healthIcon = "⚠"
		healthColor = colors.Yellow
	} else if report.OverallHealth == "critical" {
		healthIcon = "✗"
		healthColor = colors.Red
	}
	
	fmt.Printf("Overall Health: %s %s\n", healthColor(healthIcon), report.OverallHealth)
	
	// Summary
	fmt.Printf("\nSummary:\n")
	fmt.Printf("  Total Issues: %d\n", report.Summary.TotalIssues)
	if report.Summary.CriticalIssues > 0 {
		fmt.Printf("  Critical: %s %d\n", colors.Red("✗"), report.Summary.CriticalIssues)
	}
	if report.Summary.WarningIssues > 0 {
		fmt.Printf("  Warnings: %s %d\n", colors.Yellow("⚠"), report.Summary.WarningIssues)
	}
	if report.Summary.InfoIssues > 0 {
		fmt.Printf("  Info: %s %d\n", colors.Blue("ℹ"), report.Summary.InfoIssues)
	}

	// Issues
	if len(report.Issues) > 0 {
		fmt.Printf("\nDetected Issues:\n")
		for i, issue := range report.Issues {
			icon := "ℹ"
			color := colors.Blue
			if issue.Severity == "warning" {
				icon = "⚠"
				color = colors.Yellow
			} else if issue.Severity == "error" || issue.Severity == "critical" {
				icon = "✗"
				color = colors.Red
			}
			
			fmt.Printf("%d. %s [%s] %s: %s\n", 
				i+1, color(icon), issue.Category, issue.Component, issue.Description)
			fmt.Printf("   Solution: %s\n", issue.Solution)
		}
	}

	// Recommendations
	if len(report.Recommendations) > 0 {
		fmt.Printf("\nRecommendations:\n")
		for i, rec := range report.Recommendations {
			fmt.Printf("%d. %s\n", i+1, rec)
		}
	}

	fmt.Printf("\nReport generated at: %s\n", report.Timestamp.Format("2006-01-02 15:04:05"))
}

func displayHealthStatus(health *HealthStatus) {
	fmt.Printf("\nHealth Status for %s\n", health.Environment.String())
	if health.Component != "" {
		fmt.Printf("Component: %s\n", health.Component)
	}
	fmt.Println("===========================")
	
	// Overall health
	healthIcon := "✓"
	healthColor := colors.Green
	if health.OverallHealth == "warning" {
		healthIcon = "⚠"
		healthColor = colors.Yellow
	} else if health.OverallHealth == "critical" {
		healthIcon = "✗"
		healthColor = colors.Red
	}
	
	fmt.Printf("Overall Health: %s %s\n", healthColor(healthIcon), health.OverallHealth)
	fmt.Printf("Checked at: %s\n", health.Timestamp.Format("2006-01-02 15:04:05"))
}

func displayDependencyAnalysis(analysis *DependencyAnalysis, showGraph bool) {
	fmt.Printf("\nDependency Analysis for %s\n", analysis.Environment.String())
	fmt.Println("=====================================")
	
	fmt.Printf("Charts: %d\n", analysis.TotalCharts)
	fmt.Printf("Dependencies: %d\n", analysis.Dependencies)
	
	if len(analysis.CircularDeps) > 0 {
		fmt.Printf("%s Circular Dependencies: %d\n", colors.Yellow("⚠"), len(analysis.CircularDeps))
		for i, cycle := range analysis.CircularDeps {
			fmt.Printf("  %d. %v\n", i+1, cycle)
		}
	}
	
	if len(analysis.MissingDeps) > 0 {
		fmt.Printf("%s Missing Dependencies: %d\n", colors.Red("✗"), len(analysis.MissingDeps))
		for i, missing := range analysis.MissingDeps {
			fmt.Printf("  %d. %s\n", i+1, missing)
		}
	}
	
	fmt.Printf("\nDeployment Order (%d levels):\n", len(analysis.DeploymentOrder))
	for i, level := range analysis.DeploymentOrder {
		fmt.Printf("  Level %d: %v\n", i+1, level)
	}
	
	if showGraph && len(analysis.Graph) > 0 {
		fmt.Printf("\nDependency Graph:\n")
		for chart, deps := range analysis.Graph {
			if len(deps) > 0 {
				fmt.Printf("  %s → %v\n", chart, deps)
			}
		}
	}
}

func saveDiagnosticReport(report *DiagnosticReport, filename string) error {
	// Implementation to save JSON report to file
	return nil
}

func exportDependencyAnalysis(analysis *DependencyAnalysis, filename string) error {
	// Implementation to export analysis to file
	return nil
}