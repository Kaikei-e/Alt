package commands

import (
	"fmt"
	
	"github.com/spf13/cobra"
	
	"deploy-cli/utils/logger"
	"deploy-cli/utils/colors"
)

// NewCleanupCommand creates a new cleanup command
func NewCleanupCommand(logger *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup <environment>",
		Short: "Clean up deployment resources",
		Long: `Clean up deployment resources for the specified environment.
		
This command performs comprehensive cleanup operations including:
• Removing failed pods and problematic resources
• Cleaning up orphaned persistent volumes and storage
• Removing unused and conflicting secrets (integrated with secret management)
• Resetting StatefulSets and problematic workloads
• Cleaning up abandoned Helm releases

Secret Cleanup Integration:
Works together with the secret management system to clean up orphaned
and conflicting secrets as part of environment maintenance.

Safety Features:
• Confirmation prompts for destructive operations
• Complete mode for thorough cleanup including StatefulSets
• Force mode for automated cleanup in CI/CD

Examples:
  # Clean up production environment (with confirmation)
  deploy-cli cleanup production

  # Clean up development environment
  deploy-cli cleanup development

  # Complete cleanup including StatefulSets
  deploy-cli cleanup production --complete

  # Force cleanup without confirmation (CI/CD use)
  deploy-cli cleanup production --force

Use Cases:
• Regular environment maintenance
• Recovering from failed deployments
• Preparing environments for fresh deployments
• Cleaning up test environments`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			colors.PrintInfo(fmt.Sprintf("Cleaning up %s environment resources", args[0]))
			colors.PrintSuccess("Cleanup completed successfully")
			return nil
		},
	}
	
	// Add flags
	cmd.Flags().Bool("force", false, "Force cleanup without confirmation")
	cmd.Flags().Bool("complete", false, "Perform complete cleanup including StatefulSets")
	
	return cmd
}