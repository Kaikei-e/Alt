// PHASE R3: Deployment command implementation with clean separation
package deployment

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"deploy-cli/domain"
	"deploy-cli/rest/commands/shared"
	"deploy-cli/usecase/deployment_usecase"
	"deploy-cli/utils/colors"
)

// DeployCommand represents the deploy command with focused responsibility
type DeployCommand struct {
	shared      *shared.CommandShared
	usecase     *deployment_usecase.DeploymentUsecase
	validation  *DeployValidation
	output      *DeployOutput
}

// NewDeployCommand creates a new deploy command
func NewDeployCommand(shared *shared.CommandShared) *cobra.Command {
	deployCmd := &DeployCommand{
		shared:     shared,
		validation: NewDeployValidation(shared),
		output:     NewDeployOutput(shared),
	}

	cmd := &cobra.Command{
		Use:   "deploy <environment>",
		Short: "Deploy Alt RSS Reader services",
		Long: `Deploy Alt RSS Reader services to Kubernetes using Helm charts.

This command performs comprehensive deployment with automatic validation:
• Pre-deployment secret validation and conflict resolution
• Storage infrastructure setup and verification
• Namespace creation and configuration
• Helm chart deployment in proper dependency order
• Post-deployment health checking and validation

Supported environments: development, staging, production

Examples:
  # Deploy to production with automatic validation
  deploy-cli deployment deploy production

  # Deploy with custom image tags
  IMAGE_PREFIX=myregistry/alt TAG_BASE=20231201-abc123 deploy-cli deployment deploy production

  # Preview deployment without applying changes
  deploy-cli deployment deploy production --dry-run

  # Emergency deployment mode
  deploy-cli deployment deploy production --emergency-mode`,
		Args:              cobra.ExactArgs(1),
		PreRunE:           deployCmd.preRun,
		RunE:              deployCmd.run,
		PersistentPreRunE: shared.PersistentPreRunE,
	}

	// Add deployment-specific flags using the flags helper
	flags := NewDeployFlags()
	flags.AddToCommand(cmd)

	return cmd
}

// preRun performs pre-execution setup and validation
func (d *DeployCommand) preRun(cmd *cobra.Command, args []string) error {
	d.shared.Logger.InfoWithContext("initializing deployment command", map[string]interface{}{
		"environment": args[0],
	})

	// Parse and validate environment
	env, err := d.validation.ValidateEnvironment(args[0])
	if err != nil {
		return fmt.Errorf("environment validation failed: %w", err)
	}

	// Create deployment options from command flags
	options, err := d.createDeploymentOptions(cmd, env)
	if err != nil {
		return fmt.Errorf("deployment options creation failed: %w", err)
	}

	// Validate deployment options
	if err := d.validation.ValidateDeploymentOptions(options); err != nil {
		return fmt.Errorf("deployment options validation failed: %w", err)
	}

	// Initialize deployment usecase
	d.usecase = d.createDeploymentUsecase()

	return nil
}

// run executes the deployment
func (d *DeployCommand) run(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	
	d.output.PrintDeploymentStart()

	// Parse environment
	env, _ := domain.ParseEnvironment(args[0])

	// Create deployment options
	options, err := d.createDeploymentOptions(cmd, env)
	if err != nil {
		return fmt.Errorf("failed to create deployment options: %w", err)
	}

	// Process emergency mode if enabled
	if err := d.processEmergencyMode(cmd, options); err != nil {
		return fmt.Errorf("emergency mode processing failed: %w", err)
	}

	// Log deployment configuration
	d.output.PrintDeploymentConfiguration(options)

	// Execute deployment
	result, duration, err := d.executeDeployment(ctx, options)
	if err != nil {
		d.output.PrintDeploymentError(err)
		return err
	}

	// Print results and completion message
	d.output.PrintDeploymentResults(result, duration)
	d.output.PrintCompletionMessage(result, duration)

	return nil
}

// createDeploymentOptions creates deployment options from command flags and environment
func (d *DeployCommand) createDeploymentOptions(cmd *cobra.Command, env domain.Environment) (*domain.DeploymentOptions, error) {
	options := domain.NewDeploymentOptions()
	options.Environment = env

	// Get environment variables
	imagePrefix := d.shared.SystemDriver.GetEnvironmentVariable("IMAGE_PREFIX")
	if imagePrefix == "" {
		return nil, fmt.Errorf("IMAGE_PREFIX environment variable is required")
	}
	options.ImagePrefix = imagePrefix
	options.TagBase = d.shared.SystemDriver.GetEnvironmentVariable("TAG_BASE")

	// Parse flags using the flags helper
	flags := NewDeployFlags()
	if err := flags.ParseFromCommand(cmd, options); err != nil {
		return nil, fmt.Errorf("flag parsing failed: %w", err)
	}

	return options, nil
}

// processEmergencyMode processes emergency mode settings
func (d *DeployCommand) processEmergencyMode(cmd *cobra.Command, options *domain.DeploymentOptions) error {
	emergencyMode, _ := cmd.Flags().GetBool("emergency-mode")
	if !emergencyMode {
		return nil
	}

	colors.PrintWarning("🚨 EMERGENCY MODE ACTIVATED - Aggressive timeouts and minimal validation")
	
	// Apply emergency mode settings
	options.SkipStatefulSetRecovery = true
	options.AutoFixSecrets = true
	options.AutoCreateNamespaces = true
	options.SkipHealthChecks = true
	options.Timeout = 5 * time.Minute
	options.ForceUnlock = true

	d.shared.Logger.InfoWithContext("emergency mode configuration applied", map[string]interface{}{
		"skip_statefulset_recovery": options.SkipStatefulSetRecovery,
		"auto_fix_secrets":          options.AutoFixSecrets,
		"skip_health_checks":        options.SkipHealthChecks,
		"emergency_timeout":         options.Timeout.String(),
	})

	return nil
}

// executeDeployment executes the actual deployment
func (d *DeployCommand) executeDeployment(ctx context.Context, options *domain.DeploymentOptions) (*domain.DeploymentProgress, time.Duration, error) {
	start := time.Now()
	result, err := d.usecase.Deploy(ctx, options)
	duration := time.Since(start)

	if err != nil {
		return result, duration, fmt.Errorf("deployment execution failed: %w", err)
	}

	return result, duration, nil
}

// createDeploymentUsecase creates the deployment usecase with all dependencies
func (d *DeployCommand) createDeploymentUsecase() *deployment_usecase.DeploymentUsecase {
	return d.shared.DeploymentUsecaseFactory.CreateDeploymentUsecase()
}