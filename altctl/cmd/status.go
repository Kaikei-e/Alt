package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/compose"
	"github.com/alt-project/altctl/internal/output"
	"github.com/alt-project/altctl/internal/stack"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show running services per stack",
	Long: `Display the status of all services grouped by stack.

Examples:
  altctl status                # Show all service status
  altctl status --json         # Output as JSON
  altctl status --watch        # Continuous status updates`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)

	statusCmd.Flags().Bool("json", false, "output as JSON")
	statusCmd.Flags().BoolP("watch", "w", false, "watch for changes")
	statusCmd.Flags().Duration("interval", 2*time.Second, "watch interval")
}

func runStatus(cmd *cobra.Command, args []string) error {
	jsonOutput, _ := cmd.Flags().GetBool("json")
	watch, _ := cmd.Flags().GetBool("watch")
	interval, _ := cmd.Flags().GetDuration("interval")

	if watch {
		return watchStatus(interval, jsonOutput)
	}

	return showStatus(jsonOutput)
}

func showStatus(jsonOutput bool) error {
	printer := output.NewPrinter(cfg.Output.Colors)
	registry := stack.NewRegistry()

	// Create compose client
	client := compose.NewClient(
		getProjectRoot(),
		getComposeDir(),
		logger,
		dryRun,
	)

	// Get all compose files
	var files []string
	for _, s := range registry.All() {
		if s.ComposeFile != "" {
			files = append(files, s.ComposeFile)
		}
	}

	// Get service status
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	statuses, err := client.PS(ctx, files)
	if err != nil {
		// Non-fatal: compose files might not exist yet
		logger.Debug("failed to get status", "error", err)
	}

	// Build a map of service name to status
	statusMap := make(map[string]compose.ServiceStatus)
	for _, s := range statuses {
		statusMap[s.Name] = s
	}

	if jsonOutput {
		return outputStatusJSON(registry, statusMap)
	}

	return outputStatusTable(printer, registry, statusMap)
}

func outputStatusJSON(registry *stack.Registry, statusMap map[string]compose.ServiceStatus) error {
	type stackStatus struct {
		Name     string                           `json:"name"`
		Services map[string]compose.ServiceStatus `json:"services"`
	}

	var result []stackStatus
	for _, s := range registry.All() {
		ss := stackStatus{
			Name:     s.Name,
			Services: make(map[string]compose.ServiceStatus),
		}
		for _, svc := range s.Services {
			if status, ok := statusMap[svc]; ok {
				ss.Services[svc] = status
			}
		}
		if len(ss.Services) > 0 {
			result = append(result, ss)
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func outputStatusTable(printer *output.Printer, registry *stack.Registry, statusMap map[string]compose.ServiceStatus) error {
	// Group services by stack
	for _, s := range registry.All() {
		var runningServices []compose.ServiceStatus
		for _, svc := range s.Services {
			if status, ok := statusMap[svc]; ok {
				runningServices = append(runningServices, status)
			}
		}

		if len(runningServices) == 0 {
			continue
		}

		printer.Header(fmt.Sprintf("%s Stack", strings.Title(s.Name)))

		table := output.NewTable([]string{"SERVICE", "STATE", "HEALTH", "PORTS"})
		for _, status := range runningServices {
			state := status.State
			if strings.Contains(state, "Up") {
				state = printer.StatusBadge("running") + " " + state
			} else {
				state = printer.StatusBadge(state) + " " + state
			}

			health := status.Health
			if health == "" {
				health = "-"
			}

			ports := status.Ports
			if ports == "" {
				ports = "-"
			}

			table.AddRow([]string{status.Name, state, health, ports})
		}
		table.Render()
		fmt.Println()
	}

	// Summary
	totalRunning := len(statusMap)
	if totalRunning == 0 {
		printer.Warning("No services running")
	} else {
		printer.Info("Total: %d service(s) running", totalRunning)
	}

	return nil
}

func watchStatus(interval time.Duration, jsonOutput bool) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial display
	if err := showStatus(jsonOutput); err != nil {
		return err
	}

	for range ticker.C {
		// Clear screen (ANSI escape)
		fmt.Print("\033[H\033[2J")
		if err := showStatus(jsonOutput); err != nil {
			return err
		}
	}

	return nil
}
