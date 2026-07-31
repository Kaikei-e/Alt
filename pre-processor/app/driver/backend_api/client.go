// Package backend_api provides a Connect-RPC client for alt-data-hub's
// DataHubService (alt.datahub.v1).
//
// The package name still says "backend_api" because the host it dials has not
// moved — alt-data-hub answers on the same address alt-backend's internal
// listener used to. What changed in ADR-000954 D7 is the procedure namespace:
// services.backend.v1.BackendInternalService became alt.datahub.v1.DataHubService
// with identical RPC names and message fields.
package backend_api

import (
	"net/http"
	"time"

	"connectrpc.com/connect"

	"pre-processor/gen/proto/alt/datahub/v1/datahubv1connect"
)

// Client wraps the DataHubService Connect-RPC client.
type Client struct {
	client datahubv1connect.DataHubServiceClient
}

// defaultClientTimeout bounds every Connect-RPC call issued by the fallback
// client below. http.DefaultClient has Timeout=0 (no deadline), which would
// let a stalled backend hang every RPC indefinitely.
const defaultClientTimeout = 30 * time.Second

// NewClient creates a new alt-data-hub client. When httpClient is nil a
// timeout-bounded client is used; callers that need mTLS pass a custom
// client built from tlsutil.LoadClientConfig. The serviceToken arg is
// retained for signature compatibility and ignored — authentication is
// established at the TLS transport layer.
func NewClient(baseURL, _ string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultClientTimeout}
	}
	c := datahubv1connect.NewDataHubServiceClient(
		httpClient,
		baseURL,
	)
	return &Client{client: c}
}

func (c *Client) addAuth(_ connect.AnyRequest) {
	// No-op: authentication is handled by the TLS transport layer (mTLS).
}
