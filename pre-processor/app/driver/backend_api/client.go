// Package backend_api provides a Connect-RPC client for alt-backend's BackendInternalService.
package backend_api

import (
	"net/http"

	"connectrpc.com/connect"

	"pre-processor/gen/proto/services/backend/v1/backendv1connect"
)

// Client wraps the BackendInternalService Connect-RPC client.
type Client struct {
	client backendv1connect.BackendInternalServiceClient
}

// NewClient creates a new backend API client. When httpClient is nil the
// package-level http.DefaultClient is used; callers that need mTLS pass a
// custom client built from tlsutil.LoadClientConfig. The serviceToken arg
// is retained for signature compatibility and ignored — authentication is
// established at the TLS transport layer.
func NewClient(baseURL, _ string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	c := backendv1connect.NewBackendInternalServiceClient(
		httpClient,
		baseURL,
	)
	return &Client{client: c}
}

func (c *Client) addAuth(_ connect.AnyRequest) {
	// No-op: authentication is handled by the TLS transport layer (mTLS).
}
