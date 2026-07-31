package config

import (
	"fmt"
	"net"
	"os"
	"strings"
)

// This file holds the per-binary slice of configuration. alt-backend now ships
// as three processes (cmd/backend, cmd/harvester, cmd/datahub) built from one
// module, and each one reads a different subset of the environment.
//
// Two rules shape everything here (CLAUDE.md rules 8 and 9):
//
//   - A binary validates only the surfaces it actually opens. Port uniqueness
//     is meaningless to a process with no listeners, so those checks moved out
//     of the shared validateConfig path and into ValidateBackendListeners.
//   - Required config that is missing exits non-zero. "Unset" must never be an
//     implicit "disabled", because that makes a forgotten compose variable
//     indistinguishable from a deliberate opt-out.

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

func validatePort(name string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535, got %d", name, port)
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
// Neither service authenticates its caller, so the bind address is the access
// control — which is why the default is loopback and why widening it has to be
// typed out. The widening is not hypothetical: compose publishes
// `127.0.0.1:9102:9102` so altctl works from the host, and docker-proxy
// connects to the container over its eth0 address. A bind pinned to 127.0.0.1
// *inside* the netns is unreachable from there, so refusing an explicit
// OPERATOR_LISTEN_ADDR=":9102" would not harden anything — it would only take
// the operator workflow offline while leaving the published port in place.
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

// opsListenEnv names the listener every alt-backend binary opens.
const opsListenEnv = "OPS_LISTEN"

// defaultOpsListenAddr keeps a `go run ./cmd/...` on a laptop off the LAN.
// compose overrides it with ":9110" so Prometheus can scrape over alt-network.
const defaultOpsListenAddr = "127.0.0.1:9110"

// LoadOpsListenAddr returns the bind address of the ops listener shared by
// cmd/backend, cmd/harvester and cmd/datahub.
//
// One port, one shape, three binaries: /health for the compose probe and the
// healthcheck subcommand, /metrics for the three scrape jobs in
// observability/prometheus/prometheus.yml. Nothing else is mounted on it, so
// unlike the operator listener there is no admin surface for a wide bind to
// expose — ":9110" is the normal deployment value, not an accident.
//
// It is also why data-hub can be scraped at all: its data plane speaks mTLS,
// and giving Prometheus a client certificate would put a monitoring identity
// in DATAHUB_ALLOWED_PEERS.
func LoadOpsListenAddr() (string, error) {
	addr := strings.TrimSpace(os.Getenv(opsListenEnv))
	if addr == "" {
		return defaultOpsListenAddr, nil
	}
	if err := validateHostPort(opsListenEnv, addr); err != nil {
		return "", err
	}
	return addr, nil
}

// validateHostPort rejects anything net/http could not bind. An empty host is
// allowed — that is the "every interface in this netns" shorthand compose uses
// — but an empty port is not, because http.Server would then pick an ephemeral
// one and nothing would ever reach the listener.
func validateHostPort(name, addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%s=%q is not a valid host:port: %w", name, addr, err)
	}
	if strings.TrimSpace(port) == "" {
		return fmt.Errorf("%s=%q has no port", name, addr)
	}
	return nil
}

// ListenReach describes how far a bind address reaches, for the startup log.
type ListenReach string

const (
	// ReachLoopback: only a process inside this network namespace can connect.
	ReachLoopback ListenReach = "loopback_only"
	// ReachNetwork: anything that can route to the container can connect —
	// every other container on alt-network, and the host if the port is
	// published.
	ReachNetwork ListenReach = "container_network"
)

// ListenAddrReach classifies a bind address. An address it cannot parse is
// reported as ReachNetwork: the log line exists to warn, and guessing
// "loopback" for something unrecognised would make it lie in the one direction
// that matters.
func ListenAddrReach(addr string) ListenReach {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return ReachNetwork
	}
	if host == "" {
		return ReachNetwork
	}
	if host == "localhost" {
		return ReachLoopback
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return ReachLoopback
	}
	return ReachNetwork
}

// DataHubConfig is the mTLS listener configuration for cmd/datahub. Every
// field is required: data-hub is the only writer of alt-db on behalf of other
// services, and it has no plaintext surface to fall back to.
type DataHubConfig struct {
	ListenAddr   string
	CertFile     string
	KeyFile      string
	CAFile       string
	AllowedPeers []string
}

// LoadDataHubConfig reads the data-hub listener configuration, failing on any
// missing value.
//
// This deliberately does not use tlsutil.OptionsFromEnv, which fails open
// twice: MTLS_CLIENT_AUTH unset yields tls.NoClientCert (TLS that never
// verifies the caller), and MTLS_ALLOWED_PEERS unset yields no allowlist (any
// certificate the shared CA issued is accepted, i.e. any service may
// impersonate any other). Client auth is not configurable here at all —
// cmd/datahub always requires and verifies — and the allowlist is required
// config.
func LoadDataHubConfig() (*DataHubConfig, error) {
	listenAddr, err := LoadDataHubListenAddr()
	if err != nil {
		return nil, err
	}
	certFile, err := requiredEnv("DATAHUB_TLS_CERT_FILE")
	if err != nil {
		return nil, err
	}
	keyFile, err := requiredEnv("DATAHUB_TLS_KEY_FILE")
	if err != nil {
		return nil, err
	}
	caFile, err := requiredEnv("DATAHUB_TLS_CA_FILE")
	if err != nil {
		return nil, err
	}
	rawPeers, err := requiredEnv("DATAHUB_ALLOWED_PEERS")
	if err != nil {
		return nil, err
	}

	peers := make([]string, 0, strings.Count(rawPeers, ",")+1)
	for _, p := range strings.Split(rawPeers, ",") {
		if s := strings.TrimSpace(p); s != "" {
			peers = append(peers, s)
		}
	}
	if len(peers) == 0 {
		return nil, fmt.Errorf("DATAHUB_ALLOWED_PEERS contains no usable entries: " +
			"an empty allowlist accepts every certificate the shared CA issued")
	}

	return &DataHubConfig{
		ListenAddr:   listenAddr,
		CertFile:     certFile,
		KeyFile:      keyFile,
		CAFile:       caFile,
		AllowedPeers: peers,
	}, nil
}

// LoadDataHubListenAddr reads just the mutual-TLS listener's bind address.
//
// It is split out of LoadDataHubConfig because the healthcheck subcommand
// needs it and nothing else: the probe dials this port to prove the mTLS
// listener goroutine is alive, and a probe that had to load certificates and
// the peer allowlist first would fail for reasons unrelated to liveness
// (ADR-000784).
func LoadDataHubListenAddr() (string, error) {
	addr, err := requiredEnv("DATAHUB_LISTEN_ADDR")
	if err != nil {
		return "", err
	}
	if err := validateHostPort("DATAHUB_LISTEN_ADDR", addr); err != nil {
		return "", err
	}
	return addr, nil
}

func requiredEnv(name string) (string, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return "", fmt.Errorf("%s is required and must not be empty", name)
	}
	return v, nil
}

// ValidateBackendConfig checks the upstreams cmd/backend calls.
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
	}
	return requireAll("backend", required)
}

// ValidateHarvesterConfig checks the upstreams the scheduled jobs call.
//
// SOVEREIGN_URL is required in every environment, unlike the backend's
// production-only check: the outbox worker appends knowledge events through
// the sovereign client, and a disabled client no-ops every append while the
// worker still marks the outbox row PROCESSED. That silently violates the
// append-first invariant instead of failing visibly.
func ValidateHarvesterConfig(cfg *Config) error {
	required := []struct {
		env   string
		value string
	}{
		{"SOVEREIGN_URL", cfg.Sovereign.URL},
		{"RAG_ORCHESTRATOR_URL", cfg.Rag.OrchestratorURL},
	}
	return requireAll("harvester", required)
}

// ValidateDataHubConfig checks the upstreams BackendInternalService calls.
func ValidateDataHubConfig(cfg *Config) error {
	required := []struct {
		env   string
		value string
	}{
		{"AUTH_HUB_URL", cfg.AuthHub.URL},
		{"BACKEND_TOKEN_SECRET", cfg.Auth.BackendTokenSecret},
		{"SOVEREIGN_URL", cfg.Sovereign.URL},
		{"MQHUB_CONNECT_URL", cfg.MQHub.ConnectURL},
	}
	if err := requireAll("datahub", required); err != nil {
		return err
	}

	// mqhub_connect.Client no-ops every publish when disabled, so the article
	// RPCs would answer 200 while emitting no events at all — the exact shape
	// of ADR-000928. Development may opt out explicitly; nothing else may.
	if !cfg.MQHub.Enabled && cfg.AppEnv != "development" {
		return fmt.Errorf("datahub config: MQHUB_ENABLED=false in APP_ENV=%s would make every "+
			"BackendInternalService article RPC succeed while publishing no events", cfg.AppEnv)
	}
	return nil
}

func requireAll(binary string, required []struct {
	env   string
	value string
}) error {
	var missing []string
	for _, r := range required {
		if strings.TrimSpace(r.value) == "" {
			missing = append(missing, r.env)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s config: required values are unset: %s", binary, strings.Join(missing, ", "))
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
// The listener they configured served the user API, the admin API and
// BackendInternalService from one socket, and MTLS_CLIENT_AUTH decided —
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
