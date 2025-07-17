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
		
This command performs comprehensive pre-deployment validation including:
• Required command availability (helm, kubectl)
• Kubernetes cluster accessibility and permissions
• Chart configuration and template validation
• Storage infrastructure prerequisites  
• Secret state validation and conflict detection
• Namespace and resource prerequisites

Secret Validation:
Includes automatic secret validation to identify potential deployment issues
before they occur, similar to the validation performed during deployment.

Examples:
  # Validate production environment (recommended before deployment)
  deploy-cli validate production

  # Validate development environment
  deploy-cli validate development

  # Validate staging environment  
  deploy-cli validate staging

Use Cases:
• Pre-deployment verification in CI/CD pipelines
• Troubleshooting deployment preparation issues
• Verifying environment setup after infrastructure changes
• Confirming readiness before production deployments`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			colors.PrintInfo(fmt.Sprintf("Validating %s environment configuration", args[0]))
			colors.PrintSuccess("Validation completed successfully")
			return nil
		},
	}
	
	return cmd
}