package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// ValidateBackendListeners checks the three published/loopback listener ports
// cmd/backend opens. harvester and data-hub open none of them, so this is not
// part of the shared validateConfig path.
func ValidateBackendListeners(cfg *Config) error {
	if err := validatePort("SERVER_PORT", cfg.Server.Port); err != nil {
		return err
	}
	if err := validatePort("CONNECT_PORT", cfg.Server.ConnectPort); err != nil {
		return err
	}
	if cfg.Server.Port == cfg.Server.ConnectPort {
		return fmt.Errorf("SERVER_PORT and CONNECT_PORT must differ, both are %d", cfg.Server.Port)
	}
	return nil
}

// operatorListenAddrEnv names the backend's operator listener bind address.
const operatorListenAddrEnv = "OPERATOR_LISTEN_ADDR"

// defaultOperatorListenAddr keeps the pre-split port so an unset variable
// binds exactly where the old internal listener did.
const defaultOperatorListenAddr = "127.0.0.1:9102"

// LoadOperatorListenAddr returns the bind address for the backend's operator
// listener (KnowledgeHomeAdminService, AdminMonitorService).
//
// The bind address used to be the entire access control, since neither
// service authenticated its caller. It no longer is — LoadOperatorAuth gates
// every RPC on this listener with a bearer token — but the bind stays the
// network-reachability floor underneath that gate (defense in depth: a
// leaked token still cannot be used from outside the container network), so
// widening it still has to be typed out. The widening is not hypothetical:
// compose publishes `127.0.0.1:9102:9102` so altctl works from the host, and
// docker-proxy connects to the container over its eth0 address. A bind
// pinned to 127.0.0.1 *inside* the netns is unreachable from there, so
// refusing an explicit OPERATOR_LISTEN_ADDR=":9102" would not harden
// anything — it would only take the operator workflow offline while leaving
// the published port in place.
//
// What replaces the refusal is a startup line: cmd/backend logs the bind and
// ListenAddrReach(addr), so "this admin port answers the container network" is
// something an operator reads in the log rather than infers from a compose
// file (CLAUDE.md rule 8).
func LoadOperatorListenAddr() (string, error) {
	addr := strings.TrimSpace(os.Getenv(operatorListenAddrEnv))
	if addr == "" {
		// Unset means loopback, always. "Wider" is only ever an explicit value.
		return defaultOperatorListenAddr, nil
	}
	if err := validateHostPort(operatorListenAddrEnv, addr); err != nil {
		return "", err
	}
	return addr, nil
}

// ValidateBackendConfig checks the upstreams cmd/backend calls.
//
// VAPID_PUBLIC_KEY is on this list rather than treated as an optional feature
// switch. alt.push.v1.PushService.GetPushConfig has exactly one job — hand the
// browser the key to pass as applicationServerKey — and an empty one is not a
// disabled feature: pushManager.subscribe rejects it, so every user who
// opened the notification settings would see a failure whose only trace is in
// their browser console. Missing required config exits non-zero
// (CLAUDE.md rule 9); an intentional opt-out is a compose file that does not
// start cmd/backend's push surface, not an unset variable.
func ValidateBackendConfig(cfg *Config) error {
	required := []struct {
		env   string
		value string
	}{
		{"SEARCH_INDEXER_CONNECT_URL", cfg.SearchIndexer.ConnectURL},
		{"MQHUB_CONNECT_URL", cfg.MQHub.ConnectURL},
		{"RAG_ORCHESTRATOR_URL", cfg.Rag.OrchestratorURL},
		{"RAG_ORCHESTRATOR_CONNECT_URL", cfg.Rag.OrchestratorConnectURL},
		{"PRE_PROCESSOR_URL", cfg.PreProcessor.URL},
		{"PRE_PROCESSOR_CONNECT_URL", cfg.PreProcessor.ConnectURL},
		{"VAPID_PUBLIC_KEY", cfg.WebPush.PublicKey},
	}
	if err := requireAll("backend", required); err != nil {
		return err
	}
	if err := ValidateRAGAuthConfig(&cfg.Rag, cfg.AppEnv); err != nil {
		return fmt.Errorf("backend config: %w", err)
	}
	if cfg.Rag.APIToken != "" {
		slog.Info("rag_api_auth_enabled", "binary", "backend")
	} else {
		slog.Warn("rag_api_auth_disabled", "binary", "backend", "reason", "RAG_API_AUTH=disabled (explicit opt-out)")
	}
	return nil
}

// backendRejectedMTLSEnv names the variables that configured the backend's old
// mixed-surface :9443 listener. cmd/datahub replaced it, so nothing in
// cmd/backend reads them any more.
var backendRejectedMTLSEnv = []string{
	"MTLS_LISTEN",
	"MTLS_PORT",
	"MTLS_CLIENT_AUTH",
	"MTLS_ALLOWED_PEERS",
}

// RejectBackendMTLSListenerEnv fails startup when any of the old mTLS-listener
// variables is still present in cmd/backend's environment.
//
// The listener they configured served the user API, the admin API and the
// service-to-service API from one socket, and MTLS_CLIENT_AUTH decided —
// defaulting to "do not verify" — whether the client certificate meant
// anything at all. All of that moved to cmd/datahub, where verification is
// unconditional and the peer allowlist is required config.
//
// A leftover value would be read by nobody. That is precisely the failure this
// split exists to remove: config present, behaviour absent, no signal either
// way (CLAUDE.md rule 8). MTLS_CERT_FILE / MTLS_KEY_FILE / MTLS_CA_FILE are
// deliberately not on this list — the backend still presents that leaf as a
// client for the rag-orchestrator Connect hop.
func RejectBackendMTLSListenerEnv() error {
	var present []string
	for _, name := range backendRejectedMTLSEnv {
		if _, ok := os.LookupEnv(name); ok {
			present = append(present, name)
		}
	}
	if len(present) > 0 {
		return fmt.Errorf("backend config: %s configure the removed :9443 listener and are read by nothing; "+
			"the mutual-TLS surface now belongs to cmd/datahub (DATAHUB_* variables)", strings.Join(present, ", "))
	}
	return nil
}
