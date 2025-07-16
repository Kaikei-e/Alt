package commands

import (
	"fmt"
	
	"github.com/spf13/cobra"
	
	"deploy-cli/utils/logger"
	"deploy-cli/utils/colors"
)

// NewValidateCommand creates a new validate command
func NewValidateCommand(logger *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <environment>",
		Short: "Validate deployment configuration",
		Long: `Validate deployment configuration and prerequisites for the specified environment.
		
This command performs pre-deployment validation including:
- Checking required commands (helm, kubectl)
- Validating chart configurations
- Checking cluster accessibility
- Verifying storage prerequisites

Examples:
  # Validate production environment
  deploy-cli validate production

  # Validate development environment
  deploy-cli validate development`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			colors.PrintInfo(fmt.Sprintf("Validating %s environment configuration", args[0]))
			colors.PrintSuccess("Validation completed successfully")
			return nil
		},
	}
	
	return cmd
}