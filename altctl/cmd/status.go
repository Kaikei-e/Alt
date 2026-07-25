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
	"github.com/alt-project/altctl/internal/doctor"
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
		return watchStatus(cmd.Context(), interval, jsonOutput)
	}

	return showStatus(cmd.Context(), jsonOutput)
}

func showStatus(ctx context.Context, jsonOutput bool) error {
	printer := newPrinter()
	registry, err := loadRegistry()
	if err != nil {
		return err
	}

	// H1: the aggregate compose file set below (every stack's compose file)
	// includes logging.yaml, whose `${DOCKER_GROUP_ID:?...}` fails
	// interpolation before the docker daemon is ever touched if the var is
	// unset -- otherwise indistinguishable from a dead/unreachable daemon
	// once it lands in psError below. Inject the same harmless read-only
	// placeholder cmd/doctor.go uses for its own aggregate probe (see
	// doctor.EnsureDockerGroupIDEnv's doc comment) around this PS call only.
	restoreDockerGroupID := doctor.EnsureDockerGroupIDEnv()
	defer restoreDockerGroupID()

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
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	statuses, err := client.PS(ctx, files)
	if err != nil {
		// A PS failure (daemon unreachable, permission denied, compose
		// syntax error, ...) must be a real, non-zero-exit error -- not
		// silently swallowed into the "No services running" message below,
		// which would make a down Docker daemon indistinguishable from an
		// empty stack.
		return psError(err)
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

// psError wraps a client.PS failure into a user-facing CLIError with a
// non-zero exit code. Kept separate from the empty-but-successful PS case
// (no services running, which is not an error) so a down/unreachable Docker
// daemon is never indistinguishable from an idle stack.
//
// H1: showStatus already injects a placeholder DOCKER_GROUP_ID for its own
// PS call (see doctor.EnsureDockerGroupIDEnv), but this hint still checks
// the failure text for "DOCKER_GROUP_ID" as a defense-in-depth check --
// e.g. a caller that reaches psError through a path that didn't go through
// showStatus's placeholder, or a compose error that references the var for
// an unrelated reason -- so the message points at the actual remediation
// instead of blaming the docker daemon for a config-interpolation failure.
func psError(err error) error {
	if strings.Contains(err.Error(), "DOCKER_GROUP_ID") {
		return &output.CLIError{
			Summary:    "failed to get service status from Docker",
			Detail:     err.Error(),
			Suggestion: "DOCKER_GROUP_ID is not set. Export it before running altctl: `export DOCKER_GROUP_ID=$(scripts/get-docker-gid.sh)`",
			ExitCode:   output.ExitComposeError,
		}
	}
	return &output.CLIError{
		Summary:    "failed to get service status from Docker",
		Detail:     err.Error(),
		Suggestion: "Ensure the Docker daemon is running and reachable (try `docker info`) and that you have permission to use it",
		ExitCode:   output.ExitComposeError,
	}
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

	printer.PrintHints("status")
	return nil
}

func watchStatus(ctx context.Context, interval time.Duration, jsonOutput bool) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial display
	if err := showStatus(ctx, jsonOutput); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Clear screen (ANSI escape)
			fmt.Print("\033[H\033[2J")
			if err := showStatus(ctx, jsonOutput); err != nil {
				return err
			}
		}
	}
}
