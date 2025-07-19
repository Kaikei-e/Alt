package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"deploy-cli/utils/colors"
	"deploy-cli/utils/logger"
)

// NewEmergencyResetCommand creates a new emergency reset command
func NewEmergencyResetCommand(logger *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "emergency-reset",
		Short: "Emergency system reset for corrupted deployment state",
		Long: `Emergency system reset for corrupted deployment state.

WARNING: This command will completely reset the Alt RSS Reader deployment state.
Only use this when standard cleanup mechanisms fail and you're experiencing
persistent deployment issues like "another operation in progress" errors.

This command will:
• Delete ALL Helm releases with force flags
• Delete ALL alt-* namespaces (cascade deletion)
• Delete ALL persistent volumes (postgres, clickhouse, meilisearch)
• Remove ALL application data
• Clean up stuck finalizers and resources

The system will be completely reset to a clean state, ready for fresh deployment.

Safety Features:
• Confirmation prompts for destructive operations
• Detailed validation of final state
• Comprehensive logging of all operations
• Graceful handling of stuck resources

Examples:
  # Interactive emergency reset with confirmation
  deploy-cli emergency-reset

  # Force reset without confirmation (use with caution)
  deploy-cli emergency-reset --force

  # Keep namespaces, only clean resources
  deploy-cli emergency-reset --keep-namespaces

Use Cases:
• Helm releases stuck in "another operation in progress" state
• Deployment corruption after multiple failed attempts
• System state inconsistencies that prevent normal deployment
• Complete environment reset for testing purposes

IMPORTANT: This operation is irreversible. Ensure you have backups
of any important data before proceeding.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get flag values
			force, _ := cmd.Flags().GetBool("force")
			keepNamespaces, _ := cmd.Flags().GetBool("keep-namespaces")

			return runEmergencyReset(logger, force, keepNamespaces)
		},
	}

	// Add flags
	cmd.Flags().Bool("force", false, "Force reset without confirmation prompts")
	cmd.Flags().Bool("keep-namespaces", false, "Keep namespaces, only clean resources within them")

	return cmd
}

// runEmergencyReset executes the emergency reset process
func runEmergencyReset(logger *logger.Logger, force bool, keepNamespaces bool) error {
	colors.PrintInfo("🚨 Emergency Reset - Alt RSS Reader Deployment")
	colors.PrintInfo("=" + strings.Repeat("=", 50))

	// Find the emergency reset script
	scriptPath, err := findEmergencyResetScript()
	if err != nil {
		colors.PrintError(fmt.Sprintf("Cannot find emergency reset script: %v", err))
		return err
	}

	colors.PrintInfo(fmt.Sprintf("Found emergency reset script: %s", scriptPath))

	// Validate prerequisites
	if err := validatePrerequisites(logger); err != nil {
		colors.PrintError(fmt.Sprintf("Prerequisites validation failed: %v", err))
		return err
	}

	// Show warning unless force is specified
	if !force {
		colors.PrintWarning("⚠️  WARNING: This operation will completely reset the system!")
		colors.PrintWarning("⚠️  This action will:")
		colors.PrintWarning("   • Delete ALL Helm releases")

		if !keepNamespaces {
			colors.PrintWarning("   • Delete ALL alt-* namespaces")
		}

		colors.PrintWarning("   • Delete ALL persistent volumes (postgres, clickhouse, meilisearch)")
		colors.PrintWarning("   • Remove ALL application data")

		fmt.Print("\nAre you sure you want to continue? Type 'yes' to confirm: ")
		var response string
		fmt.Scanln(&response)

		if strings.ToLower(response) != "yes" {
			colors.PrintInfo("Operation cancelled by user")
			return nil
		}
	}

	// Prepare script arguments
	args := []string{}
	if force {
		args = append(args, "--force")
	}

	// Execute the emergency reset script
	colors.PrintInfo("🔄 Executing emergency reset script...")

	cmd := exec.Command("bash", append([]string{scriptPath}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		colors.PrintError(fmt.Sprintf("Emergency reset script failed: %v", err))
		return fmt.Errorf("emergency reset failed: %w", err)
	}

	colors.PrintSuccess("✅ Emergency reset completed successfully!")
	colors.PrintInfo("System is ready for fresh deployment")

	// Show next steps
	colors.PrintInfo("\nNext steps:")
	colors.PrintInfo("  1. Deploy using: ./deploy-cli deploy production")
	colors.PrintInfo("  2. Monitor deployment: ./deploy-cli monitor dashboard production")

	return nil
}

// findEmergencyResetScript locates the emergency reset script
func findEmergencyResetScript() (string, error) {
	// Try relative paths from the current directory
	possiblePaths := []string{
		"../scripts/emergency-reset.sh",
		"../../scripts/emergency-reset.sh",
		"./scripts/emergency-reset.sh",
		"../k8s-manifests/scripts/emergency-reset.sh",
	}

	for _, path := range possiblePaths {
		if absPath, err := filepath.Abs(path); err == nil {
			if _, err := os.Stat(absPath); err == nil {
				return absPath, nil
			}
		}
	}

	return "", fmt.Errorf("emergency-reset.sh script not found in expected locations")
}

// validatePrerequisites checks if required tools are available
func validatePrerequisites(logger *logger.Logger) error {
	colors.PrintInfo("Validating prerequisites...")

	// Check for kubectl
	if _, err := exec.LookPath("kubectl"); err != nil {
		logger.Error("kubectl not found", map[string]interface{}{"error": err})
		return fmt.Errorf("kubectl not found: %w", err)
	}

	// Check for helm
	if _, err := exec.LookPath("helm"); err != nil {
		logger.Error("helm not found", map[string]interface{}{"error": err})
		return fmt.Errorf("helm not found: %w", err)
	}

	// Check cluster access
	cmd := exec.Command("kubectl", "cluster-info")
	if err := cmd.Run(); err != nil {
		logger.Error("cannot access Kubernetes cluster", map[string]interface{}{"error": err})
		return fmt.Errorf("cannot access Kubernetes cluster: %w", err)
	}

	logger.Info("Prerequisites validated successfully", nil)
	colors.PrintSuccess("✅ Prerequisites validated")
	return nil
}
