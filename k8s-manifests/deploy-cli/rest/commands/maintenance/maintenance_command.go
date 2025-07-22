// PHASE R3: Maintenance command root with focused responsibility
package maintenance

import (
	"github.com/spf13/cobra"

	"deploy-cli/rest/commands/shared"
)

// MaintenanceCommand provides the root maintenance command with subcommands
type MaintenanceCommand struct {
	shared *shared.CommandShared
}

// NewMaintenanceCommand creates a new maintenance command with organized subcommands
func NewMaintenanceCommand(shared *shared.CommandShared) *cobra.Command {
	maintenanceCmd := &MaintenanceCommand{
		shared: shared,
	}

	cmd := &cobra.Command{
		Use:   "maintenance",
		Short: "Maintenance and cleanup tools for deployment management",
		Long: `Comprehensive maintenance and cleanup suite for Alt RSS Reader deployment.

This command suite provides essential maintenance operations:
• System cleanup and resource management
• Troubleshooting and diagnostic tools
• Emergency recovery operations
• Health checks and system validation
• Automated repair and recovery procedures

Features:
• Safe cleanup with confirmation prompts
• Comprehensive troubleshooting with automated fixes
• Emergency recovery for critical system failures
• Detailed diagnostics with actionable recommendations
• Integration with deployment and monitoring tools

Available Commands:
  cleanup      Clean up deployment resources and artifacts
  troubleshoot Comprehensive troubleshooting and diagnosis
  emergency    Emergency recovery and system reset operations
  diagnose     System diagnostics with automated analysis
  repair       Automated repair operations for common issues

Examples:
  # Clean up failed pods and orphaned resources
  deploy-cli maintenance cleanup production

  # Run comprehensive troubleshooting
  deploy-cli maintenance troubleshoot production --interactive

  # Emergency system recovery
  deploy-cli maintenance emergency reset production --confirm

The maintenance tools help ensure system health and quick recovery
from deployment issues.`,
		PersistentPreRunE: shared.PersistentPreRunE,
	}

	// Add maintenance subcommands
	cmd.AddCommand(NewCleanupCommand(shared))
	cmd.AddCommand(NewTroubleshootCommand(shared))
	cmd.AddCommand(NewEmergencyCommand(shared))
	cmd.AddCommand(NewDiagnoseCommand(shared))
	cmd.AddCommand(NewRepairCommand(shared))

	// Add maintenance-specific global flags
	maintenanceCmd.addMaintenanceGlobalFlags(cmd)

	return cmd
}

// addMaintenanceGlobalFlags adds maintenance-specific global flags
func (m *MaintenanceCommand) addMaintenanceGlobalFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool("dry-run", false,
		"Show what would be done without making changes")
	cmd.PersistentFlags().Bool("force", false,
		"Force operations without confirmation prompts")
	cmd.PersistentFlags().Bool("verbose", false,
		"Enable verbose output for maintenance operations")
	cmd.PersistentFlags().Duration("timeout", 0,
		"Timeout for maintenance operations (0 = no timeout)")
	cmd.PersistentFlags().Bool("auto-fix", false,
		"Enable automatic fixes where safe")
}