// PHASE R3: Cleanup command implementation with focused responsibility
package maintenance

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"deploy-cli/domain"
	"deploy-cli/rest/commands/shared"
)

// CleanupCommand provides comprehensive cleanup functionality
type CleanupCommand struct {
	shared  *shared.CommandShared
	service *MaintenanceService
	output  *MaintenanceOutput
}

// NewCleanupCommand creates the cleanup subcommand
func NewCleanupCommand(shared *shared.CommandShared) *cobra.Command {
	cleanupCmd := &CleanupCommand{
		shared:  shared,
		service: NewMaintenanceService(shared),
		output:  NewMaintenanceOutput(shared),
	}

	cmd := &cobra.Command{
		Use:   "cleanup [environment]",
		Short: "Clean up deployment resources and artifacts",
		Long: `Comprehensive cleanup operations for deployment resources and artifacts.

Cleanup Operations:
• Failed pods and error states
• Orphaned PersistentVolumes and PVCs
• Unused secrets and configmaps
• StatefulSet recovery and reset
• Abandoned Helm releases
• Temporary files and caches

Safety Features:
• Confirmation prompts for destructive operations
• Dry-run mode to preview changes
• Force mode for emergency situations
• Selective cleanup by resource type
• Backup creation for critical resources

Examples:
  # Interactive cleanup with confirmations
  deploy-cli maintenance cleanup production

  # Dry-run to see what would be cleaned
  deploy-cli maintenance cleanup production --dry-run

  # Force cleanup without confirmations
  deploy-cli maintenance cleanup production --force

  # Clean specific resource types
  deploy-cli maintenance cleanup production --pods --pvs

  # Complete cleanup including StatefulSets
  deploy-cli maintenance cleanup production --complete

Cleanup Types:
• pods: Remove failed and completed pods
• pvs: Clean orphaned PersistentVolumes  
• secrets: Remove unused secrets
• statefulsets: Reset StatefulSet state
• helm: Clean abandoned Helm releases
• all: Complete system cleanup`,
		Args: cobra.MaximumNArgs(1),
		RunE: cleanupCmd.run,
		PersistentPreRunE: shared.PersistentPreRunE,
	}

	// Add cleanup-specific flags
	cmd.Flags().Bool("pods", false, 
		"Clean up failed and completed pods")
	cmd.Flags().Bool("pvs", false, 
		"Clean up orphaned PersistentVolumes")
	cmd.Flags().Bool("secrets", false, 
		"Clean up unused secrets")
	cmd.Flags().Bool("statefulsets", false, 
		"Reset StatefulSet state")
	cmd.Flags().Bool("helm", false, 
		"Clean up abandoned Helm releases")
	cmd.Flags().Bool("all", false, 
		"Perform complete cleanup (all resource types)")
	cmd.Flags().Bool("complete", false, 
		"Include StatefulSets in complete cleanup")
	cmd.Flags().Bool("confirm", false, 
		"Skip confirmation prompts")
	cmd.Flags().StringSlice("exclude", []string{}, 
		"Exclude specific resources from cleanup")

	return cmd
}

// run executes the cleanup command
func (c *CleanupCommand) run(cmd *cobra.Command, args []string) error {
	// Parse environment
	env, err := c.parseEnvironment(args)
	if err != nil {
		return fmt.Errorf("environment parsing failed: %w", err)
	}

	// Parse cleanup options
	options, err := c.parseCleanupOptions(cmd)
	if err != nil {
		return fmt.Errorf("cleanup options parsing failed: %w", err)
	}

	// Validate cleanup options
	if err := c.validateCleanupOptions(options); err != nil {
		return fmt.Errorf("cleanup options validation failed: %w", err)
	}

	// Create cleanup context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Print cleanup start message
	c.output.PrintCleanupStart(env, options)

	// Execute cleanup operations
	result, err := c.service.ExecuteCleanup(ctx, env, options)
	if err != nil {
		c.output.PrintCleanupError(err)
		return fmt.Errorf("cleanup execution failed: %w", err)
	}

	// Print cleanup results
	c.output.PrintCleanupResults(result)

	return nil
}

// parseEnvironment parses the environment argument
func (c *CleanupCommand) parseEnvironment(args []string) (domain.Environment, error) {
	var env domain.Environment = domain.Development
	
	if len(args) > 0 {
		parsedEnv, err := domain.ParseEnvironment(args[0])
		if err != nil {
			return "", fmt.Errorf("invalid environment '%s': %w", args[0], err)
		}
		env = parsedEnv
	}

	c.shared.Logger.InfoWithContext("cleanup environment parsed", map[string]interface{}{
		"environment": env,
	})

	return env, nil
}

// parseCleanupOptions parses cleanup flags into options
func (c *CleanupCommand) parseCleanupOptions(cmd *cobra.Command) (*CleanupOptions, error) {
	options := &CleanupOptions{}
	var err error

	// Parse resource type flags
	if options.CleanPods, err = cmd.Flags().GetBool("pods"); err != nil {
		return nil, err
	}
	if options.CleanPVs, err = cmd.Flags().GetBool("pvs"); err != nil {
		return nil, err
	}
	if options.CleanSecrets, err = cmd.Flags().GetBool("secrets"); err != nil {
		return nil, err
	}
	if options.CleanStatefulSets, err = cmd.Flags().GetBool("statefulsets"); err != nil {
		return nil, err
	}
	if options.CleanHelm, err = cmd.Flags().GetBool("helm"); err != nil {
		return nil, err
	}

	// Parse operation flags
	if options.CleanAll, err = cmd.Flags().GetBool("all"); err != nil {
		return nil, err
	}
	if options.Complete, err = cmd.Flags().GetBool("complete"); err != nil {
		return nil, err
	}
	if options.SkipConfirmation, err = cmd.Flags().GetBool("confirm"); err != nil {
		return nil, err
	}
	if options.Exclude, err = cmd.Flags().GetStringSlice("exclude"); err != nil {
		return nil, err
	}

	// Parse global maintenance flags
	if options.DryRun, err = cmd.Flags().GetBool("dry-run"); err != nil {
		return nil, err
	}
	if options.Force, err = cmd.Flags().GetBool("force"); err != nil {
		return nil, err
	}
	if options.Verbose, err = cmd.Flags().GetBool("verbose"); err != nil {
		return nil, err
	}

	// Set defaults if no specific resource types selected
	if !options.hasAnyResourceTypeSelected() {
		if options.CleanAll {
			options.setAllResourceTypes()
		} else {
			// Default to safe cleanup operations
			options.CleanPods = true
			options.CleanSecrets = true
		}
	}

	return options, nil
}

// validateCleanupOptions validates cleanup configuration
func (c *CleanupCommand) validateCleanupOptions(options *CleanupOptions) error {
	// Validate that at least some cleanup operation is selected
	if !options.hasAnyOperationSelected() {
		return fmt.Errorf("no cleanup operations selected")
	}

	// Warn about destructive operations
	if options.CleanStatefulSets && !options.Force && !options.DryRun {
		c.shared.Logger.WarnWithContext("StatefulSet cleanup is destructive", map[string]interface{}{
			"operation": "statefulsets",
			"force":     options.Force,
			"dry_run":   options.DryRun,
		})
	}

	// Validate exclude list
	for _, exclude := range options.Exclude {
		if exclude == "" {
			return fmt.Errorf("empty exclude entry is not allowed")
		}
	}

	return nil
}

// CleanupOptions represents cleanup configuration
type CleanupOptions struct {
	// Resource types to clean
	CleanPods         bool
	CleanPVs          bool
	CleanSecrets      bool
	CleanStatefulSets bool
	CleanHelm         bool

	// Operation options
	CleanAll         bool
	Complete         bool
	SkipConfirmation bool
	Exclude          []string

	// Global options
	DryRun  bool
	Force   bool
	Verbose bool
}

// hasAnyResourceTypeSelected checks if any resource type is selected for cleanup
func (o *CleanupOptions) hasAnyResourceTypeSelected() bool {
	return o.CleanPods || o.CleanPVs || o.CleanSecrets || 
		   o.CleanStatefulSets || o.CleanHelm
}

// hasAnyOperationSelected checks if any cleanup operation is selected
func (o *CleanupOptions) hasAnyOperationSelected() bool {
	return o.hasAnyResourceTypeSelected() || o.CleanAll
}

// setAllResourceTypes enables all resource type cleanups
func (o *CleanupOptions) setAllResourceTypes() {
	o.CleanPods = true
	o.CleanPVs = true
	o.CleanSecrets = true
	o.CleanHelm = true
	
	if o.Complete {
		o.CleanStatefulSets = true
	}
}

// GetSelectedResourceTypes returns a list of selected resource types
func (o *CleanupOptions) GetSelectedResourceTypes() []string {
	var types []string
	
	if o.CleanPods {
		types = append(types, "pods")
	}
	if o.CleanPVs {
		types = append(types, "persistent-volumes")
	}
	if o.CleanSecrets {
		types = append(types, "secrets")
	}
	if o.CleanStatefulSets {
		types = append(types, "statefulsets")
	}
	if o.CleanHelm {
		types = append(types, "helm-releases")
	}
	
	return types
}

// IsResourceExcluded checks if a resource is in the exclude list
func (o *CleanupOptions) IsResourceExcluded(resource string) bool {
	for _, exclude := range o.Exclude {
		if exclude == resource {
			return true
		}
	}
	return false
}