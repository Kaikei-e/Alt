package cmd

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/output"
)

var uuidRegexp = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// validateReprojectMode checks that mode is one of the allowed values.
func validateReprojectMode(mode string) error {
	switch mode {
	case "dry_run", "shadow", "live":
		return nil
	default:
		return fmt.Errorf("invalid mode %q: must be dry_run, shadow, or live", mode)
	}
}

// validateUUID checks that s is a valid UUID v4 format.
func validateUUID(s string) error {
	if !uuidRegexp.MatchString(s) {
		return fmt.Errorf("invalid UUID %q: expected format xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx", s)
	}
	return nil
}

// reprojectCmd is the parent for all reproject subcommands.
var reprojectCmd = &cobra.Command{
	Use:   "reproject",
	Short: "Manage Knowledge Home reprojection runs",
	Long: `Start, monitor, compare, swap, and rollback Knowledge Home reprojection runs.

Examples:
  altctl home reproject start --mode=dry_run --from=1 --to=2
  altctl home reproject status --run-id=<uuid>
  altctl home reproject compare --run-id=<uuid>
  altctl home reproject swap --run-id=<uuid>
  altctl home reproject rollback --run-id=<uuid>`,
}

// --- start ---

var reprojectStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a new reprojection run",
	Long: `Start a new Knowledge Home reprojection run.

Modes:
  dry_run  - Simulate reprojection without writing
  shadow   - Write to shadow table for comparison
  live     - Write directly to production table

Examples:
  altctl home reproject start --mode=dry_run --from=1 --to=2
  altctl home reproject start --mode=shadow --from=1 --to=2 --range-start=2026-01-01T00:00:00Z`,
	RunE: runReprojectStart,
}

func runReprojectStart(cmd *cobra.Command, args []string) error {
	mode, _ := cmd.Flags().GetString("mode")
	from, _ := cmd.Flags().GetString("from")
	to, _ := cmd.Flags().GetString("to")
	rangeStart, _ := cmd.Flags().GetString("range-start")
	rangeEnd, _ := cmd.Flags().GetString("range-end")

	// Validate required flags
	if mode == "" {
		return fmt.Errorf("required flag \"mode\" not set")
	}
	if err := validateReprojectMode(mode); err != nil {
		return err
	}
	if from == "" {
		return fmt.Errorf("required flag \"from\" not set")
	}
	if to == "" {
		return fmt.Errorf("required flag \"to\" not set")
	}

	client, err := newAdminClient(cmd)
	if err != nil {
		return err
	}

	reqBody := map[string]string{
		"mode":        mode,
		"fromVersion": from,
		"toVersion":   to,
	}
	if rangeStart != "" {
		reqBody["rangeStart"] = rangeStart
	}
	if rangeEnd != "" {
		reqBody["rangeEnd"] = rangeEnd
	}

	var resp struct {
		RunID     string `json:"runId"`
		Status    string `json:"status"`
		Message   string `json:"message"`
		CreatedAt string `json:"createdAt"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Call(ctx, "StartReproject", reqBody, &resp); err != nil {
		return fmt.Errorf("start reprojection: %w", err)
	}

	printer := newPrinter()
	printer.Success("Reprojection started")

	table := output.NewTable([]string{"FIELD", "VALUE"})
	table.AddRow([]string{"Run ID", resp.RunID})
	table.AddRow([]string{"Status", resp.Status})
	table.AddRow([]string{"Mode", mode})
	table.AddRow([]string{"From Version", from})
	table.AddRow([]string{"To Version", to})
	if resp.Message != "" {
		table.AddRow([]string{"Message", resp.Message})
	}
	if resp.CreatedAt != "" {
		table.AddRow([]string{"Created At", resp.CreatedAt})
	}
	table.Render()

	return nil
}

// --- status ---

var reprojectStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get reprojection run status",
	Long: `Get the current status of a reprojection run.

Example:
  altctl home reproject status --run-id=550e8400-e29b-41d4-a716-446655440000`,
	RunE: runReprojectStatus,
}

func runReprojectStatus(cmd *cobra.Command, args []string) error {
	runID, _ := cmd.Flags().GetString("run-id")
	if runID == "" {
		return fmt.Errorf("required flag \"run-id\" not set")
	}
	if err := validateUUID(runID); err != nil {
		return err
	}

	client, err := newAdminClient(cmd)
	if err != nil {
		return err
	}

	reqBody := map[string]string{"runId": runID}
	var resp struct {
		RunID       string  `json:"runId"`
		Status      string  `json:"status"`
		Mode        string  `json:"mode"`
		FromVersion string  `json:"fromVersion"`
		ToVersion   string  `json:"toVersion"`
		Progress    float64 `json:"progress"`
		StartedAt   string  `json:"startedAt"`
		UpdatedAt   string  `json:"updatedAt"`
		Error       string  `json:"error"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Call(ctx, "GetReprojectStatus", reqBody, &resp); err != nil {
		return fmt.Errorf("get reprojection status: %w", err)
	}

	printer := newPrinter()
	printer.Header("Reprojection Status")

	table := output.NewTable([]string{"FIELD", "VALUE"})
	table.AddRow([]string{"Run ID", resp.RunID})
	table.AddRow([]string{"Status", resp.Status})
	table.AddRow([]string{"Mode", resp.Mode})
	table.AddRow([]string{"From Version", resp.FromVersion})
	table.AddRow([]string{"To Version", resp.ToVersion})
	table.AddRow([]string{"Progress", fmt.Sprintf("%.1f%%", resp.Progress*100)})
	if resp.StartedAt != "" {
		table.AddRow([]string{"Started At", resp.StartedAt})
	}
	if resp.UpdatedAt != "" {
		table.AddRow([]string{"Updated At", resp.UpdatedAt})
	}
	if resp.Error != "" {
		table.AddRow([]string{"Error", resp.Error})
	}
	table.Render()

	return nil
}

// --- compare ---

var reprojectCompareCmd = &cobra.Command{
	Use:   "compare",
	Short: "Compare reprojection results with current data",
	Long: `Compare the results of a shadow reprojection run against current production data.

Example:
  altctl home reproject compare --run-id=550e8400-e29b-41d4-a716-446655440000`,
	RunE: runReprojectCompare,
}

func runReprojectCompare(cmd *cobra.Command, args []string) error {
	runID, _ := cmd.Flags().GetString("run-id")
	if runID == "" {
		return fmt.Errorf("required flag \"run-id\" not set")
	}
	if err := validateUUID(runID); err != nil {
		return err
	}

	client, err := newAdminClient(cmd)
	if err != nil {
		return err
	}

	reqBody := map[string]string{"runId": runID}
	var resp struct {
		RunID        string `json:"runId"`
		TotalRecords int    `json:"totalRecords"`
		Matched      int    `json:"matched"`
		Diffs        int    `json:"diffs"`
		OnlyInOld    int    `json:"onlyInOld"`
		OnlyInNew    int    `json:"onlyInNew"`
		Summary      string `json:"summary"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := client.Call(ctx, "CompareReproject", reqBody, &resp); err != nil {
		return fmt.Errorf("compare reprojection: %w", err)
	}

	printer := newPrinter()
	printer.Header("Reprojection Comparison")

	table := output.NewTable([]string{"METRIC", "COUNT"})
	table.AddRow([]string{"Total Records", fmt.Sprintf("%d", resp.TotalRecords)})
	table.AddRow([]string{"Matched", fmt.Sprintf("%d", resp.Matched)})
	table.AddRow([]string{"Diffs", fmt.Sprintf("%d", resp.Diffs)})
	table.AddRow([]string{"Only in Old", fmt.Sprintf("%d", resp.OnlyInOld)})
	table.AddRow([]string{"Only in New", fmt.Sprintf("%d", resp.OnlyInNew)})
	table.Render()

	if resp.Summary != "" {
		fmt.Println()
		printer.Info("Summary: %s", resp.Summary)
	}

	return nil
}

// --- swap ---

var reprojectSwapCmd = &cobra.Command{
	Use:   "swap",
	Short: "Swap shadow projection to production",
	Long: `Atomically swap the shadow reprojection table to become the production table.

This operation requires a completed shadow reprojection run.

Example:
  altctl home reproject swap --run-id=550e8400-e29b-41d4-a716-446655440000`,
	RunE: runReprojectSwap,
}

func runReprojectSwap(cmd *cobra.Command, args []string) error {
	runID, _ := cmd.Flags().GetString("run-id")
	if runID == "" {
		return fmt.Errorf("required flag \"run-id\" not set")
	}
	if err := validateUUID(runID); err != nil {
		return err
	}

	client, err := newAdminClient(cmd)
	if err != nil {
		return err
	}

	reqBody := map[string]string{"runId": runID}
	var resp struct {
		RunID     string `json:"runId"`
		Status    string `json:"status"`
		Message   string `json:"message"`
		SwappedAt string `json:"swappedAt"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Call(ctx, "SwapReproject", reqBody, &resp); err != nil {
		return fmt.Errorf("swap reprojection: %w", err)
	}

	printer := newPrinter()
	printer.Success("Projection swapped successfully")

	table := output.NewTable([]string{"FIELD", "VALUE"})
	table.AddRow([]string{"Run ID", resp.RunID})
	table.AddRow([]string{"Status", resp.Status})
	if resp.Message != "" {
		table.AddRow([]string{"Message", resp.Message})
	}
	if resp.SwappedAt != "" {
		table.AddRow([]string{"Swapped At", resp.SwappedAt})
	}
	table.Render()

	return nil
}

// --- rollback ---

var reprojectRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Rollback a swapped projection",
	Long: `Rollback a previously swapped projection to restore the original production table.

Example:
  altctl home reproject rollback --run-id=550e8400-e29b-41d4-a716-446655440000`,
	RunE: runReprojectRollback,
}

func runReprojectRollback(cmd *cobra.Command, args []string) error {
	runID, _ := cmd.Flags().GetString("run-id")
	if runID == "" {
		return fmt.Errorf("required flag \"run-id\" not set")
	}
	if err := validateUUID(runID); err != nil {
		return err
	}

	client, err := newAdminClient(cmd)
	if err != nil {
		return err
	}

	reqBody := map[string]string{"runId": runID}
	var resp struct {
		RunID       string `json:"runId"`
		Status      string `json:"status"`
		Message     string `json:"message"`
		RolledBackAt string `json:"rolledBackAt"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Call(ctx, "RollbackReproject", reqBody, &resp); err != nil {
		return fmt.Errorf("rollback reprojection: %w", err)
	}

	printer := newPrinter()
	printer.Success("Projection rolled back successfully")

	table := output.NewTable([]string{"FIELD", "VALUE"})
	table.AddRow([]string{"Run ID", resp.RunID})
	table.AddRow([]string{"Status", resp.Status})
	if resp.Message != "" {
		table.AddRow([]string{"Message", resp.Message})
	}
	if resp.RolledBackAt != "" {
		table.AddRow([]string{"Rolled Back At", resp.RolledBackAt})
	}
	table.Render()

	return nil
}

// --- init ---

func init() {
	homeCmd.AddCommand(reprojectCmd)

	// Shared flags for all reproject subcommands
	for _, cmd := range []*cobra.Command{reprojectStartCmd, reprojectStatusCmd, reprojectCompareCmd, reprojectSwapCmd, reprojectRollbackCmd} {
		addAdminFlags(cmd)
	}

	// start-specific flags
	reprojectStartCmd.Flags().String("mode", "", "reprojection mode: dry_run, shadow, or live (required)")
	reprojectStartCmd.Flags().String("from", "", "source projection version (required)")
	reprojectStartCmd.Flags().String("to", "", "target projection version (required)")
	reprojectStartCmd.Flags().String("range-start", "", "optional start of time range (RFC3339)")
	reprojectStartCmd.Flags().String("range-end", "", "optional end of time range (RFC3339)")

	// run-id flag for status/compare/swap/rollback
	for _, cmd := range []*cobra.Command{reprojectStatusCmd, reprojectCompareCmd, reprojectSwapCmd, reprojectRollbackCmd} {
		cmd.Flags().String("run-id", "", "reprojection run ID (UUID, required)")
	}

	reprojectCmd.AddCommand(reprojectStartCmd)
	reprojectCmd.AddCommand(reprojectStatusCmd)
	reprojectCmd.AddCommand(reprojectCompareCmd)
	reprojectCmd.AddCommand(reprojectSwapCmd)
	reprojectCmd.AddCommand(reprojectRollbackCmd)
}
