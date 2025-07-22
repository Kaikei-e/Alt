// PHASE R3: Emergency command implementation with focused responsibility
package maintenance

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"deploy-cli/domain"
	"deploy-cli/rest/commands/shared"
)

// EmergencyCommand provides emergency recovery functionality
type EmergencyCommand struct {
	shared  *shared.CommandShared
	service *MaintenanceService
	output  *MaintenanceOutput
}

// NewEmergencyCommand creates the emergency subcommand
func NewEmergencyCommand(shared *shared.CommandShared) *cobra.Command {
	emergencyCmd := &EmergencyCommand{
		shared:  shared,
		service: NewMaintenanceService(shared),
		output:  NewMaintenanceOutput(shared),
	}

	cmd := &cobra.Command{
		Use:   "emergency <operation> [environment]",
		Short: "Emergency recovery and system reset operations",
		Long: `Emergency recovery operations for critical system failures and disasters.

WARNING: These are destructive operations designed for emergency situations.
Always ensure you have proper backups before using emergency commands.

Emergency Operations:
• System-wide emergency reset and recovery
• Failed deployment rollback and cleanup
• Database emergency procedures and recovery
• Network isolation and security containment
• Service dependency resolution and restart
• Resource quota emergency adjustments
• Critical path service restoration

Safety Features:
• Multi-level confirmation for destructive operations
• Automatic backup creation before changes
• Rollback capabilities for emergency operations
• Safe mode with reduced blast radius
• Emergency contact notifications
• Audit logging for all emergency actions

Available Sub-operations:
  reset        Complete system reset (DESTRUCTIVE)
  rollback     Emergency rollback to last known good state
  isolate      Isolate problematic components
  restore      Restore from emergency backup
  drain        Emergency node drain and evacuation
  scale-zero   Scale all deployments to zero (emergency stop)

Examples:
  # Complete emergency system reset (requires --confirm)
  deploy-cli maintenance emergency reset production --confirm

  # Emergency rollback to last stable deployment
  deploy-cli maintenance emergency rollback production

  # Isolate problematic service
  deploy-cli maintenance emergency isolate production --component alt-backend

  # Emergency scale-down to zero
  deploy-cli maintenance emergency scale-zero production --confirm

  # Safe mode emergency operations
  deploy-cli maintenance emergency reset production --safe-mode

Emergency Protocols:
• Immediate notification to on-call teams
• Automated incident creation and tracking  
• Real-time status updates and progress reporting
• Post-emergency automated system validation
• Comprehensive audit trail for compliance`,
		Args: cobra.RangeArgs(1, 2),
		RunE: emergencyCmd.run,
		PersistentPreRunE: shared.PersistentPreRunE,
	}

	// Add subcommands for different emergency operations
	cmd.AddCommand(emergencyCmd.createResetCommand())
	cmd.AddCommand(emergencyCmd.createRollbackCommand())
	cmd.AddCommand(emergencyCmd.createIsolateCommand())
	cmd.AddCommand(emergencyCmd.createRestoreCommand())
	cmd.AddCommand(emergencyCmd.createDrainCommand())
	cmd.AddCommand(emergencyCmd.createScaleZeroCommand())

	// Add emergency-specific flags
	cmd.PersistentFlags().Bool("confirm", false,
		"Confirm destructive emergency operation")
	cmd.PersistentFlags().Bool("safe-mode", false,
		"Enable safe mode with reduced blast radius")
	cmd.PersistentFlags().String("component", "",
		"Target specific component for emergency operation")
	cmd.PersistentFlags().String("backup-before", "",
		"Create backup before emergency operation")
	cmd.PersistentFlags().Bool("notify-oncall", true,
		"Notify on-call team of emergency operation")
	cmd.PersistentFlags().String("incident-id", "",
		"Associate with existing incident ID")

	return cmd
}

// createResetCommand creates the reset subcommand
func (e *EmergencyCommand) createResetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "reset [environment]",
		Short: "Complete emergency system reset (DESTRUCTIVE)",
		Long: `Perform a complete emergency system reset.

This is the most destructive operation available and should only be used
in catastrophic failure scenarios when normal recovery procedures have failed.

Reset Operations:
• Stop all running deployments and services
• Clear failed pods and error states  
• Reset StatefulSets to initial state
• Clear persistent volumes (with backup)
• Restart core system services
• Validate system health post-reset

Safety Requirements:
• Requires --confirm flag for execution
• Automatic backup creation before reset
• Multi-step confirmation process
• Real-time progress monitoring
• Automatic rollback on critical failures`,
		Args: cobra.MaximumNArgs(1),
		RunE: e.runReset,
	}
}

// createRollbackCommand creates the rollback subcommand  
func (e *EmergencyCommand) createRollbackCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "rollback [environment]",
		Short: "Emergency rollback to last known good state",
		Long: `Roll back to the last known good deployment state.

This operation attempts to restore the system to the most recent stable
configuration by reversing recent changes and deployments.`,
		Args: cobra.MaximumNArgs(1),
		RunE: e.runRollback,
	}
}

// createIsolateCommand creates the isolate subcommand
func (e *EmergencyCommand) createIsolateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "isolate [environment]",
		Short: "Isolate problematic components",
		Long: `Isolate problematic components to prevent cascade failures.

This operation removes failing components from the service mesh
while maintaining system stability for unaffected services.`,
		Args: cobra.MaximumNArgs(1),
		RunE: e.runIsolate,
	}
}

// createRestoreCommand creates the restore subcommand
func (e *EmergencyCommand) createRestoreCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "restore [environment]",
		Short: "Restore from emergency backup",
		Long: `Restore system state from emergency backup.

This operation restores the entire system from a previously created
backup, including configuration, data, and deployment state.`,
		Args: cobra.MaximumNArgs(1),
		RunE: e.runRestore,
	}
}

// createDrainCommand creates the drain subcommand
func (e *EmergencyCommand) createDrainCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "drain [environment]",
		Short: "Emergency node drain and evacuation",
		Long: `Perform emergency node drain and pod evacuation.

This operation safely evacuates pods from problematic nodes
and cordons nodes to prevent new pod scheduling.`,
		Args: cobra.MaximumNArgs(1),
		RunE: e.runDrain,
	}
}

// createScaleZeroCommand creates the scale-zero subcommand
func (e *EmergencyCommand) createScaleZeroCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "scale-zero [environment]",
		Short: "Scale all deployments to zero (emergency stop)",
		Long: `Scale all deployments to zero replicas for emergency stop.

This operation immediately stops all application workloads while
preserving the deployment configurations for later restoration.`,
		Args: cobra.MaximumNArgs(1),
		RunE: e.runScaleZero,
	}
}

// run executes the emergency command (shows help if no subcommand)
func (e *EmergencyCommand) run(cmd *cobra.Command, args []string) error {
	return cmd.Help()
}

// runReset executes the emergency reset operation
func (e *EmergencyCommand) runReset(cmd *cobra.Command, args []string) error {
	return e.executeEmergencyOperation(cmd, args, "reset")
}

// runRollback executes the emergency rollback operation
func (e *EmergencyCommand) runRollback(cmd *cobra.Command, args []string) error {
	return e.executeEmergencyOperation(cmd, args, "rollback")
}

// runIsolate executes the emergency isolate operation
func (e *EmergencyCommand) runIsolate(cmd *cobra.Command, args []string) error {
	return e.executeEmergencyOperation(cmd, args, "isolate")
}

// runRestore executes the emergency restore operation
func (e *EmergencyCommand) runRestore(cmd *cobra.Command, args []string) error {
	return e.executeEmergencyOperation(cmd, args, "restore")
}

// runDrain executes the emergency drain operation
func (e *EmergencyCommand) runDrain(cmd *cobra.Command, args []string) error {
	return e.executeEmergencyOperation(cmd, args, "drain")
}

// runScaleZero executes the emergency scale-zero operation
func (e *EmergencyCommand) runScaleZero(cmd *cobra.Command, args []string) error {
	return e.executeEmergencyOperation(cmd, args, "scale-zero")
}

// executeEmergencyOperation executes the specified emergency operation
func (e *EmergencyCommand) executeEmergencyOperation(cmd *cobra.Command, args []string, operation string) error {
	// Parse environment
	env, err := e.parseEnvironment(args)
	if err != nil {
		return fmt.Errorf("environment parsing failed: %w", err)
	}

	// Parse emergency options
	options, err := e.parseEmergencyOptions(cmd, operation)
	if err != nil {
		return fmt.Errorf("emergency options parsing failed: %w", err)
	}

	// Validate emergency options
	if err := e.validateEmergencyOptions(options); err != nil {
		return fmt.Errorf("emergency options validation failed: %w", err)
	}

	// Create emergency context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Print emergency start message
	e.output.PrintEmergencyStart(env, options)

	// Execute emergency operations
	result, err := e.service.ExecuteEmergencyReset(ctx, env, options)
	if err != nil {
		e.output.PrintEmergencyError(err)
		return fmt.Errorf("emergency execution failed: %w", err)
	}

	// Print emergency results
	e.output.PrintEmergencyResults(result)

	return nil
}

// parseEnvironment parses the environment argument
func (e *EmergencyCommand) parseEnvironment(args []string) (domain.Environment, error) {
	var env domain.Environment = domain.Development
	
	if len(args) > 0 {
		parsedEnv, err := domain.ParseEnvironment(args[0])
		if err != nil {
			return "", fmt.Errorf("invalid environment '%s': %w", args[0], err)
		}
		env = parsedEnv
	}

	e.shared.Logger.InfoWithContext("emergency environment parsed", map[string]interface{}{
		"environment": env,
	})

	return env, nil
}

// parseEmergencyOptions parses emergency flags into options
func (e *EmergencyCommand) parseEmergencyOptions(cmd *cobra.Command, operation string) (*EmergencyOptions, error) {
	options := &EmergencyOptions{
		Operation: operation,
	}
	var err error

	// Parse emergency-specific flags
	if options.Confirm, err = cmd.Flags().GetBool("confirm"); err != nil {
		return nil, err
	}
	if options.SafeMode, err = cmd.Flags().GetBool("safe-mode"); err != nil {
		return nil, err
	}
	if options.Component, err = cmd.Flags().GetString("component"); err != nil {
		return nil, err
	}
	if options.BackupBefore, err = cmd.Flags().GetString("backup-before"); err != nil {
		return nil, err
	}
	if options.NotifyOnCall, err = cmd.Flags().GetBool("notify-oncall"); err != nil {
		return nil, err
	}
	if options.IncidentID, err = cmd.Flags().GetString("incident-id"); err != nil {
		return nil, err
	}

	// Parse global maintenance flags
	if options.Force, err = cmd.Flags().GetBool("force"); err != nil {
		return nil, err
	}
	if options.Verbose, err = cmd.Flags().GetBool("verbose"); err != nil {
		return nil, err
	}
	if options.Timeout, err = cmd.Flags().GetDuration("timeout"); err != nil {
		return nil, err
	}

	return options, nil
}

// validateEmergencyOptions validates emergency configuration
func (e *EmergencyCommand) validateEmergencyOptions(options *EmergencyOptions) error {
	// Require confirmation for destructive operations
	destructiveOps := map[string]bool{
		"reset":      true,
		"scale-zero": true,
	}

	if destructiveOps[options.Operation] && !options.Confirm && !options.Force {
		return fmt.Errorf("operation '%s' requires --confirm or --force flag", options.Operation)
	}

	return nil
}

