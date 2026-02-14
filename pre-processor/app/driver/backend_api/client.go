// Package backend_api provides a Connect-RPC client for alt-backend's BackendInternalService.
package backend_api

import (
	"net/http"

	"connectrpc.com/connect"

	"pre-processor/gen/proto/clients/preprocessor-backend/v1/backendv1connect"
)

const serviceTokenHeader = "X-Service-Token"

// Client wraps the BackendInternalService Connect-RPC client.
type Client struct {
	client       backendv1connect.BackendInternalServiceClient
	serviceToken string
}

// NewClient creates a new backend API client.
func NewClient(baseURL string, serviceToken string) *Client {
	c := backendv1connect.NewBackendInternalServiceClient(
		http.DefaultClient,
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
