package di

import (
	"log/slog"

	"alt/config"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"
	"alt/shared/driver/datahub_client"
)

// newDataHubClient builds the services.datahub.v1.DataHubService client shared by
// cmd/backend and cmd/harvester, and refuses to return without one.
//
// cmd/datahub does not call this. A process reaching its own RPC surface would
// pay two TLS handshakes and a JSON round trip to get to a usecase it can call
// directly, and would have to name itself in its own DATAHUB_ALLOWED_PEERS —
// the list that is the data plane's entire authorisation decision, and which
// means something narrower when the provider is on it.
//
// The panic is the point (CLAUDE.md rules 8 and 9). alt-data-hub owns alt_db
// after ADR-000954 Wave 3, so a binary that started without this client would
// not be degraded, it would be a process with no database that reports healthy
// — cmd/harvester's probe checks only its ops listener, and cmd/backend's
// checks only its REST port. Returning nil and letting the first gateway
// dereference it would move the failure from boot to whenever a user happened
// to request a proxied image.
//
// binary names the caller so the startup line distinguishes the two processes,
// which are otherwise identical in the log stream.
func newDataHubClient(binary string) datahubv1connect.DataHubServiceClient {
	cfg, err := config.LoadDataHubClientConfig()
	if err != nil {
		panic(binary + ": data-hub client configuration is invalid: " + err.Error())
	}

	client, err := datahub_client.New(cfg)
	if err != nil {
		panic(binary + ": failed to build the data-hub client: " + err.Error())
	}

	slog.Info("datahub_client_enabled",
		"binary", binary,
		"url", cfg.BaseURL,
		"server_name", cfg.ServerName,
		"cert_file", cfg.CertFile,
		"ca_file", cfg.CAFile,
	)
	return client
}
