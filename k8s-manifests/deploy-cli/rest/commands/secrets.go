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
		Short: "Secret management operations",
		Long: `Manage Kubernetes secrets for the Alt RSS Reader deployment.
This includes validation, conflict detection, and automatic resolution.`,
	}

	// Add subcommands
	cmd.AddCommand(newValidateSecretsCommand(log))
	cmd.AddCommand(newFixConflictsCommand(log))
	cmd.AddCommand(newListSecretsCommand(log))
	cmd.AddCommand(newDeleteOrphanedCommand(log))

	return cmd
}

// newValidateSecretsCommand creates the validate secrets subcommand
func newValidateSecretsCommand(log *logger.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [environment]",
		Short: "Validate secret state for an environment",
		Long: `Validate the current state of secrets for a specific environment.
This checks for ownership conflicts, missing secrets, and other issues.`,
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
		Short: "Automatically resolve secret conflicts",
		Long: `Automatically resolve secret ownership conflicts for an environment.
This will delete conflicting secrets and recreate them with correct ownership.`,
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
		Short: "List all secrets for an environment",
		Long: `List all secrets across all namespaces for a specific environment.
Shows secret names, namespaces, and ownership information.`,
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
		Short: "Delete orphaned secrets",
		Long: `Delete secrets that are no longer needed or have invalid ownership.
This includes secrets in wrong namespaces or with outdated ownership metadata.`,
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