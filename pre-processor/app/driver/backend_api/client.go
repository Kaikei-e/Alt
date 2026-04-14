// Package backend_api provides a Connect-RPC client for alt-backend's BackendInternalService.
package backend_api

import (
	"net/http"

	"connectrpc.com/connect"

	"pre-processor/gen/proto/services/backend/v1/backendv1connect"
)

const serviceTokenHeader = "X-Service-Token"

// Client wraps the BackendInternalService Connect-RPC client.
type Client struct {
	client       backendv1connect.BackendInternalServiceClient
	serviceToken string
}

// NewClient creates a new backend API client. When httpClient is nil the
// package-level http.DefaultClient is used; callers that need mTLS pass a
// custom client built from tlsutil.LoadClientConfig.
func NewClient(baseURL, serviceToken string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	c := backendv1connect.NewBackendInternalServiceClient(
		httpClient,
		baseURL,
	)
	return &Client{
		client:       c,
		serviceToken: serviceToken,
	}
}

func (c *Client) addAuth(req connect.AnyRequest) {
	if c.serviceToken != "" {
		req.Header().Set(serviceTokenHeader, c.serviceToken)
	}
}
