package altdb

import (
	"net/http"

	"alt/gen/proto/services/datahub/v1/datahubv1connect"

	"connectrpc.com/connect"
)

// NewDataHubServiceClient builds the Connect-RPC client every alt_db-backed
// adapter in this package shares. It exists so that the codec choice is made
// in exactly one place: the composition root and the Pact CDC test both call
// it, which is what makes the recorded contract the bytes production sends
// rather than a readable approximation of them.
//
// The codec is pinned to JSON via connect.WithProtoJSON() per ADR-000764
// (Connect-RPC over HTTP/1.1 + protojson). Connect-go otherwise defaults to
// application/proto, and a binary body would make the pact interactions
// unverifiable — the same trap documented on sovereign_client's client.
//
// httpClient carries the mutual TLS: alt-data-hub authenticates callers by
// peer certificate and nothing else, so there is no interceptor here and no
// credential in the request.
func NewDataHubServiceClient(httpClient *http.Client, baseURL string) datahubv1connect.DataHubServiceClient {
	return datahubv1connect.NewDataHubServiceClient(httpClient, baseURL, connect.WithProtoJSON())
}
