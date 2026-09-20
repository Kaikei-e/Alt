package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// newAdminClient creates an AdminClient from command flags. The operator
// token is read via loadOperatorToken and sent as a Bearer token on every
// admin RPC — see addAdminFlags and loadOperatorToken.
func newAdminClient(cmd *cobra.Command) (*adminclient.AdminClient, error) {
	backendURL, _ := cmd.Flags().GetString("backend-url")
	token, err := loadOperatorToken(cmd)
	if err != nil {
		return nil, err
	}
	return adminclient.NewClient(backendURL, token), nil
}

// operatorTokenFileEnv overrides the operator token file path resolved by
// loadOperatorToken, taking priority over the default location but yielding
// to the --operator-token-file flag.
const operatorTokenFileEnv = "ALTCTL_OPERATOR_TOKEN_FILE"

// loadOperatorToken resolves the operator bearer token altctl presents to
// alt-backend's :9102 listener on every admin RPC
// (config.LoadOperatorAuth on the server side requires it unless that
// instance runs OPERATOR_AUTH=disabled).
//
// Resolution order: --operator-token-file flag, ALTCTL_OPERATOR_TOKEN_FILE,
// then secrets/backend_operator_token.txt under the project root — the same
// file compose/base.yaml mounts into alt-backend and alt-butterfly-facade as
// the backend_operator_token secret.
//
// A missing file at the *default* location is not an error: that is the
// legitimate "target alt-backend runs OPERATOR_AUTH=disabled" shape the
// dev/staging compose overlays use, and altctl has no way to know which
// stack it is pointed at from the flag alone. A file named explicitly via
// the flag or the environment variable that cannot be read is an error,
// since the operator asked for it by name.
func loadOperatorToken(cmd *cobra.Command) (string, error) {
	flagPath, _ := cmd.Flags().GetString("operator-token-file")
	envPath := os.Getenv(operatorTokenFileEnv)
	explicit := flagPath != "" || envPath != ""

	path := flagPath
	if path == "" {
		path = envPath
	}
	if path == "" {
		path = filepath.Join(getProjectRoot(), "secrets", "backend_operator_token.txt")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !explicit {
			// Only the default path may be absent (dev/staging run alt-backend
			// with OPERATOR_AUTH=disabled); say so loudly instead of letting the
			// operator discover it as a bare 401 from :9102 later.
			fmt.Fprintf(os.Stderr, "warning: operator token file %s not found; calling :9102 without a bearer (requires OPERATOR_AUTH=disabled on alt-backend)\n", path)
			return "", nil
		}
		return "", fmt.Errorf("read operator token file %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
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

// addAdminFlags adds the backend-url and operator-token-file flags to a
// command.
func addAdminFlags(cmd *cobra.Command) {
	// 9102 is alt-backend's internal Connect-RPC listener, which carries the
	// admin services (compose/core.yaml sets INTERNAL_PORT=9102 and publishes
	// it on 127.0.0.1 only) -- not the browser-facing :9101 or the public
	// HTTP API port. Do not change without also updating compose/core.yaml.
	cmd.Flags().String("backend-url", "http://localhost:9102", "alt-backend internal Connect-RPC admin API URL (default port 9102, see compose/core.yaml INTERNAL_PORT)")
	cmd.Flags().String("operator-token-file", "", "path to alt-backend's operator bearer token file (default: <project-root>/secrets/backend_operator_token.txt, or $ALTCTL_OPERATOR_TOKEN_FILE)")
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
