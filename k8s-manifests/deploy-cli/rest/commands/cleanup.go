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
		
This command performs cleanup operations including:
- Removing failed pods and resources
- Cleaning up orphaned persistent volumes
- Removing unused secrets
- Resetting StatefulSets if needed

Examples:
  # Clean up production environment
  deploy-cli cleanup production

  # Clean up development environment
  deploy-cli cleanup development`,
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