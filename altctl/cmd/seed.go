package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/compose"
	"github.com/alt-project/altctl/internal/output"
)

var seedCmd = &cobra.Command{
	Use:   "seed <profile>",
	Short: "Load seed data into the database",
	Long: `Load seed data into the database for development or E2E testing.

Available profiles:
  dev    - Comprehensive development data (multiple feeds, articles)
  e2e    - Deterministic E2E test data (fixed IDs for reproducibility)

The seed SQL files are located in db/seeds/.

Examples:
  altctl seed dev    # Load development seed data
  altctl seed e2e    # Load E2E test seed data`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeSeedProfiles,
	RunE:              runSeed,
}

func init() {
	rootCmd.AddCommand(seedCmd)
}

var seedProfiles = map[string]string{
	"dev": "db/seeds/dev-comprehensive.sql",
	"e2e": "db/seeds/e2e-integration.sql",
}

func runSeed(cmd *cobra.Command, args []string) error {
	printer := newPrinter()
	profile := args[0]

	sqlFile, ok := seedProfiles[profile]
	if !ok {
		return &output.CLIError{
			Summary:    fmt.Sprintf("unknown seed profile: %s", profile),
			Suggestion: "Available profiles: dev, e2e",
			ExitCode:   output.ExitUsageError,
		}
	}

	root := getProjectRoot()
	fullPath := filepath.Join(root, sqlFile)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return &output.CLIError{
			Summary:    fmt.Sprintf("seed file not found: %s", sqlFile),
			Detail:     fullPath,
			Suggestion: "Ensure db/seeds/ directory contains the seed SQL files",
			ExitCode:   output.ExitConfigError,
		}
	}

	printer.Header("Seeding Database")
	printer.Info("Profile: %s", printer.Bold(profile))
	printer.Info("File:    %s", sqlFile)
	fmt.Println()

	client := compose.NewClient(
		root,
		getComposeDir(),
		logger,
		dryRun,
	)

	// Read seed file and pipe it to psql via docker compose exec
	seedSQL, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("reading seed file: %w", err)
	}

	if dryRun {
		fmt.Printf("[dry-run] docker compose exec db psql -U ${POSTGRES_USER} -d ${POSTGRES_DB} < %s\n", sqlFile)
		printer.Success("Seed data loaded (dry-run)")
		return nil
	}

	ctx := context.Background()
	err = client.Exec(ctx, "db", []string{
		"psql", "-U", os.Getenv("POSTGRES_USER"), "-d", os.Getenv("POSTGRES_DB"),
		"-c", string(seedSQL),
	}, os.Stdout, os.Stderr)

	if err != nil {
		printer.Error("Failed to seed database: %v", err)
		return &output.CLIError{
			Summary:    "seed failed",
			Detail:     err.Error(),
			Suggestion: "Ensure 'db' service is running: altctl up db",
			ExitCode:   output.ExitComposeError,
		}
	}

	printer.Success("Seed data loaded successfully")
	return nil
}

func completeSeedProfiles(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	profiles := make([]string, 0, len(seedProfiles))
	for name := range seedProfiles {
		profiles = append(profiles, name)
	}
	return profiles, cobra.ShellCompDirectiveNoFileComp
}
