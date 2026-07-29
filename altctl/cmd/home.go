package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/adminclient"
	"github.com/alt-project/altctl/internal/output"
	"github.com/alt-project/altctl/internal/sovereignclient"
)

var homeCmd = &cobra.Command{
	Use:   "home",
	Short: "Knowledge Home operations",
	Long: `Manage Knowledge Home projections, reprojections, SLO status, snapshots, retention, and storage.

Examples:
  altctl home health                        # Projection health
  altctl home slo                           # SLO status
  altctl home reproject start --mode=live   # Start reprojection
  altctl home snapshot list                 # List snapshots
  altctl home retention status              # Retention log
  altctl home storage                       # Storage stats
  altctl home audit                         # Run projection audit
  altctl home backfill trigger              # Trigger backfill`,
}

func init() {
	rootCmd.AddCommand(homeCmd)
}

// newAdminClient creates an AdminClient from command flags. Authentication
// is expected at the network/gateway layer; no service token is passed
// through the CLI.
func newAdminClient(cmd *cobra.Command) (*adminclient.AdminClient, error) {
	backendURL, _ := cmd.Flags().GetString("backend-url")
	return adminclient.NewClient(backendURL), nil
}

// newSovereignClient creates a SovereignClient from command flags. The admin
// token is read from the operator's environment and must match the token the
// target knowledge-sovereign instance was started with — in compose that is
// secrets/sovereign_admin_token.txt, so:
//
//	ADMIN_TOKEN=$(cat secrets/sovereign_admin_token.txt) altctl home storage
//
// Omitting it yields HTTP 401 unless that instance runs ADMIN_AUTH=disabled.
func newSovereignClient(cmd *cobra.Command) *sovereignclient.SovereignClient {
	sovereignURL, _ := cmd.Flags().GetString("sovereign-url")
	return sovereignclient.NewClient(sovereignURL, os.Getenv("ADMIN_TOKEN"))
}

// addAdminFlags adds the backend-url flag to a command. Authentication is
// network/gateway-layer; no service-token flag is exposed.
func addAdminFlags(cmd *cobra.Command) {
	// 9102 is alt-backend's internal Connect-RPC listener, which carries the
	// admin services (compose/core.yaml sets INTERNAL_PORT=9102 and publishes
	// it on 127.0.0.1 only) -- not the browser-facing :9101 or the public
	// HTTP API port. Do not change without also updating compose/core.yaml.
	cmd.Flags().String("backend-url", "http://localhost:9102", "alt-backend internal Connect-RPC admin API URL (default port 9102, see compose/core.yaml INTERNAL_PORT)")
}

// addSovereignFlags adds sovereign-url flag to a command.
func addSovereignFlags(cmd *cobra.Command) {
	cmd.Flags().String("sovereign-url", "http://localhost:9511", "knowledge-sovereign metrics API URL")
}

// callAdminRPC invokes an admin Connect-RPC method with a 30s timeout.
func callAdminRPC(cmd *cobra.Command, method string, reqBody, respBody interface{}) error {
	client, err := newAdminClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()
	if err := client.Call(ctx, method, reqBody, respBody); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	return nil
}

// callAndRenderTable calls an admin RPC and renders a simple two-column table.
func callAndRenderTable(cmd *cobra.Command, method, header string, columns []string, reqBody, respBody interface{}, rows func() [][]string) error {
	if err := callAdminRPC(cmd, method, reqBody, respBody); err != nil {
		return err
	}
	printer := newPrinter()
	printer.Header(header)
	table := output.NewTable(columns)
	for _, row := range rows() {
		table.AddRow(row)
	}
	table.Render()
	return nil
}
