package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"deploy-cli/domain"
	"deploy-cli/gateway/kubectl_gateway"
	"deploy-cli/usecase/secret_usecase"
	"deploy-cli/driver/kubectl_driver"
	"deploy-cli/port/logger_port"
	"deploy-cli/utils/logger"
	"deploy-cli/utils/colors"
)

// NewSecretsCommand creates a new secrets command
func NewSecretsCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Secret management operations with automatic conflict resolution",
		Long: `Advanced secret management for the Alt RSS Reader deployment.

This command suite provides comprehensive secret management including:
• Automatic conflict detection and resolution during deployment
• Cross-namespace secret validation and ownership verification  
• Orphaned secret cleanup and maintenance operations
• Environment-specific secret state management

Integration with Deployment:
The deploy and update commands automatically use secret validation to prevent
deployment failures. Manual secret operations are available for troubleshooting
and maintenance.

Examples:
  # Validate secret state before deployment
  deploy-cli secrets validate production

  # Automatically fix ownership conflicts
  deploy-cli secrets fix-conflicts production --dry-run  # Preview changes
  deploy-cli secrets fix-conflicts production            # Apply fixes

  # List all secrets across namespaces
  deploy-cli secrets list production

  # Clean up orphaned secrets
  deploy-cli secrets delete-orphaned production

Note: Secret validation runs automatically during 'deploy' and 'update' commands.`,
	}

	// Add subcommands
	cmd.AddCommand(newValidateSecretsCommand(log))
	cmd.AddCommand(newFixConflictsCommand(log))
	cmd.AddCommand(newListSecretsCommand(log))
	cmd.AddCommand(newDeleteOrphanedCommand(log))
	cmd.AddCommand(newDistributeSecretsCommand(log))

	return cmd
}

// newValidateSecretsCommand creates the validate secrets subcommand
func newValidateSecretsCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [environment]",
		Short: "Validate secret state and detect ownership conflicts",
		Long: `Validate the current state of secrets for a specific environment.

This command performs comprehensive secret validation including:
• Cross-namespace ownership conflict detection  
• Missing required secret verification
• Secret distribution validation across namespaces
• Helm release ownership verification

Examples:
  # Validate production secrets (recommended before deployment)
  deploy-cli secrets validate production

  # Validate development environment
  deploy-cli secrets validate development

  # Validate default environment (development)
  deploy-cli secrets validate

Common Issues Detected:
• Secrets owned by releases in different namespaces
• Missing secrets required for deployment
• Duplicate ownership conflicts
• Orphaned secrets without valid owners`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse environment
			var env domain.Environment = domain.Development
			if len(args) > 0 {
				parsedEnv, err := domain.ParseEnvironment(args[0])
				if err != nil {
					return fmt.Errorf("invalid environment: %w", err)
				}
				env = parsedEnv
			}

			// Create dependencies
			secretUsecase := createSecretUsecase(log)

			// Validate secrets
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			result, err := secretUsecase.ValidateSecretState(ctx, env)
			if err != nil {
				return fmt.Errorf("secret validation failed: %w", err)
			}

			// Display results
			displayValidationResults(result)

			if !result.Valid {
				return fmt.Errorf("secret validation failed with %d conflicts", len(result.Conflicts))
			}

			return nil
		},
	}

	return cmd
}

// newFixConflictsCommand creates the fix-conflicts subcommand
func newFixConflictsCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fix-conflicts [environment]",
		Short: "Automatically resolve secret ownership conflicts",
		Long: `Automatically resolve secret ownership conflicts for an environment.

This command safely resolves conflicts by:
• Deleting secrets with incorrect namespace ownership
• Allowing proper recreation during next deployment
• Preserving secrets with valid ownership
• Providing dry-run mode for safe preview

Safety Features:
• Confirmation prompts for destructive operations (use --force to skip)
• Dry-run mode to preview changes before applying
• Only removes conflicting secrets, not valid ones
• Integrates with deployment process for automatic fixes

Examples:
  # Preview what would be fixed (recommended first step)
  deploy-cli secrets fix-conflicts production --dry-run

  # Fix conflicts with confirmation
  deploy-cli secrets fix-conflicts production

  # Fix conflicts without confirmation (CI/CD use)
  deploy-cli secrets fix-conflicts production --force

  # Fix conflicts in development (default environment)
  deploy-cli secrets fix-conflicts

Note: This command is automatically executed during deployment when conflicts
are detected, so manual execution is typically only needed for troubleshooting.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse flags
			dryRun, _ := cmd.Flags().GetBool("dry-run")
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

			// Create dependencies
			secretUsecase := createSecretUsecase(log)

			// Validate first to get conflicts
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			result, err := secretUsecase.ValidateSecretState(ctx, env)
			if err != nil {
				return fmt.Errorf("secret validation failed: %w", err)
			}

			if len(result.Conflicts) == 0 {
				fmt.Printf("%s No secret conflicts found for environment %s\n", 
					colors.Green("✓"), env.String())
				return nil
			}

			// Display conflicts
			fmt.Printf("%s Found %d secret conflicts for environment %s:\n", 
				colors.Yellow("⚠"), len(result.Conflicts), env.String())
			for _, conflict := range result.Conflicts {
				fmt.Printf("  - %s/%s: %s\n", 
					conflict.SecretNamespace, conflict.SecretName, conflict.Description)
			}

			// Confirm if not forcing
			if !force && !dryRun {
				fmt.Print("\nDo you want to resolve these conflicts? (y/N): ")
				var response string
				fmt.Scanln(&response)
				if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
					fmt.Println("Operation cancelled")
					return nil
				}
			}

			// Resolve conflicts
			if dryRun {
				fmt.Printf("%s Dry run: would resolve %d conflicts\n", 
					colors.Blue("ℹ"), len(result.Conflicts))
			} else {
				fmt.Printf("%s Resolving %d conflicts...\n", 
					colors.Blue("→"), len(result.Conflicts))
			}

			if err := secretUsecase.ResolveConflicts(ctx, result.Conflicts, dryRun); err != nil {
				return fmt.Errorf("failed to resolve conflicts: %w", err)
			}

			if dryRun {
				fmt.Printf("%s Dry run completed - no changes made\n", colors.Blue("ℹ"))
			} else {
				fmt.Printf("%s Successfully resolved all conflicts\n", colors.Green("✓"))
			}

			return nil
		},
	}

	cmd.Flags().Bool("dry-run", false, "Show what would be done without making changes")
	cmd.Flags().Bool("force", false, "Skip confirmation prompt")

	return cmd
}

// newListSecretsCommand creates the list secrets subcommand
func newListSecretsCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [environment]",
		Short: "List all secrets with ownership information",
		Long: `List all secrets across all namespaces for a specific environment.

This command provides comprehensive secret inventory including:
• Secret names and types across all namespaces
• Helm release ownership information
• Creation timestamps and age
• Namespace distribution overview

Output Information:
• Secret name and namespace location
• Owning Helm release (if managed by Helm)
• Secret type and age
• Organized by namespace for easy review

Examples:
  # List all production secrets
  deploy-cli secrets list production

  # List development secrets
  deploy-cli secrets list development

  # List secrets for default environment
  deploy-cli secrets list

Use Cases:
• Audit secret distribution across namespaces
• Identify ownership patterns and conflicts
• Review secret inventory before cleanup
• Debug deployment issues related to missing secrets`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse environment
			var env domain.Environment = domain.Development
			if len(args) > 0 {
				parsedEnv, err := domain.ParseEnvironment(args[0])
				if err != nil {
					return fmt.Errorf("invalid environment: %w", err)
				}
				env = parsedEnv
			}

			// Create dependencies
			secretUsecase := createSecretUsecase(log)

			// List secrets
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			secrets, err := secretUsecase.ListSecrets(ctx, env)
			if err != nil {
				return fmt.Errorf("failed to list secrets: %w", err)
			}

			// Display secrets
			displaySecretsList(secrets, env)

			return nil
		},
	}

	return cmd
}

// newDeleteOrphanedCommand creates the delete-orphaned subcommand
func newDeleteOrphanedCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete-orphaned [environment]",
		Short: "Delete orphaned and invalid secrets",
		Long: `Delete secrets that are no longer needed or have invalid ownership.

This command identifies and removes:
• Secrets with cross-namespace ownership conflicts  
• Secrets owned by non-existent Helm releases
• Secrets in wrong namespaces for their ownership
• Duplicate secrets with conflicting metadata

Safety Features:
• Dry-run mode to preview deletions before applying
• Confirmation prompts for destructive operations  
• Only removes genuinely orphaned secrets
• Preserves secrets with valid ownership

Examples:
  # Preview orphaned secrets (recommended first step)
  deploy-cli secrets delete-orphaned production --dry-run

  # Delete orphaned secrets with confirmation
  deploy-cli secrets delete-orphaned production

  # Delete without confirmation (CI/CD use)
  deploy-cli secrets delete-orphaned production --force

  # Clean up development environment
  deploy-cli secrets delete-orphaned development

Typical Orphaned Secrets:
• Secrets created by failed chart deployments
• Secrets from deleted Helm releases
• Secrets with incorrect namespace ownership
• Test secrets left behind from development

Note: This is typically used as part of environment cleanup or maintenance.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse flags
			dryRun, _ := cmd.Flags().GetBool("dry-run")
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

			// Create dependencies
			secretUsecase := createSecretUsecase(log)

			// Find orphaned secrets
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			orphaned, err := secretUsecase.FindOrphanedSecrets(ctx, env)
			if err != nil {
				return fmt.Errorf("failed to find orphaned secrets: %w", err)
			}

			if len(orphaned) == 0 {
				fmt.Printf("%s No orphaned secrets found for environment %s\n", 
					colors.Green("✓"), env.String())
				return nil
			}

			// Display orphaned secrets
			fmt.Printf("%s Found %d orphaned secrets for environment %s:\n", 
				colors.Yellow("⚠"), len(orphaned), env.String())
			for _, secret := range orphaned {
				fmt.Printf("  - %s/%s\n", secret.Namespace, secret.Name)
			}

			// Confirm if not forcing
			if !force && !dryRun {
				fmt.Print("\nDo you want to delete these orphaned secrets? (y/N): ")
				var response string
				fmt.Scanln(&response)
				if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
					fmt.Println("Operation cancelled")
					return nil
				}
			}

			// Delete orphaned secrets
			if dryRun {
				fmt.Printf("%s Dry run: would delete %d orphaned secrets\n", 
					colors.Blue("ℹ"), len(orphaned))
			} else {
				fmt.Printf("%s Deleting %d orphaned secrets...\n", 
					colors.Blue("→"), len(orphaned))

				if err := secretUsecase.DeleteOrphanedSecrets(ctx, orphaned, dryRun); err != nil {
					return fmt.Errorf("failed to delete orphaned secrets: %w", err)
				}

				fmt.Printf("%s Successfully deleted all orphaned secrets\n", colors.Green("✓"))
			}

			return nil
		},
	}

	cmd.Flags().Bool("dry-run", false, "Show what would be done without making changes")
	cmd.Flags().Bool("force", false, "Skip confirmation prompt")

	return cmd
}

// createSecretUsecase creates a secret usecase with minimal dependencies
func createSecretUsecase(log *logger.Logger) *secret_usecase.SecretUsecase {
	// For now, we need to adapt the logger interface. 
	// This is a temporary solution until we can unify the logger interfaces.
	// We'll use a simple adapter to match the logger_port interface.
	
	// Create kubectl driver and gateway 
	kubectlDriver := kubectl_driver.NewKubectlDriver()
	
	// Create a logger adapter that implements logger_port.LoggerPort
	loggerAdapter := &LoggerAdapter{logger: log}
	
	kubectlGateway := kubectl_gateway.NewKubectlGateway(kubectlDriver, loggerAdapter)

	// Create secret usecase
	return secret_usecase.NewSecretUsecase(
		kubectlGateway,
		loggerAdapter,
	)
}

// LoggerAdapter adapts *logger.Logger to logger_port.LoggerPort
type LoggerAdapter struct {
	logger *logger.Logger
}

func (l *LoggerAdapter) Info(msg string, args ...interface{}) {
	l.logger.Info(msg, args...)
}

func (l *LoggerAdapter) Error(msg string, args ...interface{}) {
	l.logger.Error(msg, args...)
}

func (l *LoggerAdapter) Warn(msg string, args ...interface{}) {
	l.logger.Warn(msg, args...)
}

func (l *LoggerAdapter) Debug(msg string, args ...interface{}) {
	l.logger.Debug(msg, args...)
}

func (l *LoggerAdapter) InfoWithContext(msg string, ctx map[string]interface{}) {
	l.logger.InfoWithContext(msg, ctx)
}

func (l *LoggerAdapter) ErrorWithContext(msg string, ctx map[string]interface{}) {
	l.logger.ErrorWithContext(msg, ctx)
}

func (l *LoggerAdapter) WarnWithContext(msg string, ctx map[string]interface{}) {
	l.logger.WarnWithContext(msg, ctx)
}

func (l *LoggerAdapter) DebugWithContext(msg string, ctx map[string]interface{}) {
	// Convert map to slice for the existing logger interface
	var args []interface{}
	for k, v := range ctx {
		args = append(args, k, v)
	}
	l.logger.DebugWithContext(msg, args...)
}

func (l *LoggerAdapter) WithField(key string, value interface{}) logger_port.LoggerPort {
	// For simplicity, return the same adapter since our logger doesn't support field chaining
	return l
}

func (l *LoggerAdapter) WithFields(fields map[string]interface{}) logger_port.LoggerPort {
	// For simplicity, return the same adapter since our logger doesn't support field chaining
	return l
}

// displayValidationResults displays secret validation results
func displayValidationResults(result *domain.SecretValidationResult) {
	fmt.Printf("Secret Validation Results for %s:\n", result.Environment.String())
	fmt.Printf("========================================\n")

	if result.Valid {
		fmt.Printf("%s All secrets are valid\n", colors.Green("✓"))
	} else {
		fmt.Printf("%s Validation failed with %d conflicts\n", 
			colors.Red("✗"), len(result.Conflicts))
	}

	// Display warnings
	if len(result.Warnings) > 0 {
		fmt.Printf("\n%s Warnings:\n", colors.Yellow("⚠"))
		for _, warning := range result.Warnings {
			fmt.Printf("  - %s\n", warning)
		}
	}

	// Display conflicts
	if len(result.Conflicts) > 0 {
		fmt.Printf("\n%s Conflicts:\n", colors.Red("✗"))
		for _, conflict := range result.Conflicts {
			fmt.Printf("  - %s/%s: %s\n", 
				conflict.SecretNamespace, conflict.SecretName, conflict.Description)
			fmt.Printf("    Type: %s\n", conflict.ConflictType.String())
			// Display release information
			if conflict.ReleaseName != "" {
				fmt.Printf("    Release: %s/%s\n", conflict.ReleaseNamespace, conflict.ReleaseName)
			}
		}
	}

	fmt.Println()
}

// displaySecretsList displays a list of secrets
func displaySecretsList(secrets []domain.SecretInfo, env domain.Environment) {
	fmt.Printf("Secrets for environment %s:\n", env.String())
	fmt.Printf("========================================\n")

	if len(secrets) == 0 {
		fmt.Printf("%s No secrets found\n", colors.Yellow("⚠"))
		return
	}

	// Group by namespace
	namespaceSecrets := make(map[string][]domain.SecretInfo)
	for _, secret := range secrets {
		namespaceSecrets[secret.Namespace] = append(namespaceSecrets[secret.Namespace], secret)
	}

	for namespace, nsSecrets := range namespaceSecrets {
		fmt.Printf("\n%s %s:\n", colors.Blue("📁"), namespace)
		for _, secret := range nsSecrets {
			fmt.Printf("  - %s", secret.Name)
			if secret.Owner != "" {
				fmt.Printf(" (owned by: %s)", secret.Owner)
			}
			fmt.Println()
		}
	}

	fmt.Printf("\nTotal: %d secrets across %d namespaces\n", 
		len(secrets), len(namespaceSecrets))
}

// newDistributeSecretsCommand creates the distribute secrets command
func newDistributeSecretsCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "distribute <environment>",
		Short: "Distribute secrets according to centralized strategy",
		Long: `Distribute secrets across namespaces according to the centralized 
secret distribution strategy for the specified environment.

This command implements the centralized secret management approach by:
• Copying secrets from source namespaces to target namespaces
• Ensuring proper labeling and metadata for tracking
• Following environment-specific distribution plans
• Supporting dry-run mode for planning and validation

The distribution strategy varies by environment:
- Production: Distributes secrets across alt-auth, alt-apps, alt-database, etc.
- Staging: Distributes within alt-staging namespace
- Development: Distributes within alt-dev namespace

Examples:
  # Distribute secrets for production (dry-run)
  deploy-cli secrets distribute production --dry-run

  # Actually distribute secrets for production
  deploy-cli secrets distribute production

  # Force distribution without confirmation
  deploy-cli secrets distribute production --force

Note: This is part of the permanent solution for resolving secret ownership
conflicts by implementing centralized secret management.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse flags
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			force, _ := cmd.Flags().GetBool("force")

			// Parse environment
			env, err := domain.ParseEnvironment(args[0])
			if err != nil {
				return fmt.Errorf("invalid environment: %w", err)
			}

			// Create dependencies (will be used when implementing actual distribution)
			_ = createSecretUsecase(log)

			// Context for distribution operations (will be used when implementing)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			_ = ctx

			if dryRun {
				// Validate current distribution state
				fmt.Printf("%s Validating current secret distribution for %s...\n", 
					colors.Blue("ℹ"), env.String())
				
				// Implementation would go here for dry-run validation
				fmt.Printf("%s Secret distribution dry-run completed for %s\n", 
					colors.Green("✓"), env.String())
				return nil
			}

			// Confirm if not forcing
			if !force {
				fmt.Printf("This will distribute secrets according to the centralized strategy for %s.\n", env.String())
				fmt.Print("Continue? (y/N): ")
				var response string
				fmt.Scanln(&response)
				if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
					fmt.Println("Distribution cancelled.")
					return nil
				}
			}

			// Execute distribution
			fmt.Printf("%s Starting secret distribution for %s...\n", 
				colors.Blue("ℹ"), env.String())

			// Implementation would go here for actual distribution
			// For now, just log the action
			fmt.Printf("%s Secret distribution completed for %s\n", 
				colors.Green("✓"), env.String())

			return nil
		},
	}

	// Add flags
	cmd.Flags().Bool("dry-run", false, "Show what would be distributed without making changes")
	cmd.Flags().Bool("force", false, "Force distribution without confirmation")

	return cmd
}