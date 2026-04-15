package cmd

import (
	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/adminclient"
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
// is established at the TLS transport layer (mTLS); no service token is
// passed through the CLI.
func newAdminClient(cmd *cobra.Command) (*adminclient.AdminClient, error) {
	backendURL, _ := cmd.Flags().GetString("backend-url")
	return adminclient.NewClient(backendURL, ""), nil
}

// newSovereignClient creates a SovereignClient from command flags.
func newSovereignClient(cmd *cobra.Command) *sovereignclient.SovereignClient {
	sovereignURL, _ := cmd.Flags().GetString("sovereign-url")
	return sovereignclient.NewClient(sovereignURL)
}

// addAdminFlags adds the backend-url flag to a command. Authentication is
// transport-layer (mTLS); no service-token flag is exposed.
func addAdminFlags(cmd *cobra.Command) {
	cmd.Flags().String("backend-url", "http://localhost:9001", "alt-backend admin API URL")
}

// addSovereignFlags adds sovereign-url flag to a command.
func addSovereignFlags(cmd *cobra.Command) {
	cmd.Flags().String("sovereign-url", "http://localhost:9511", "knowledge-sovereign metrics API URL")
}
