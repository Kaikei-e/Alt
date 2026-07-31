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
// listener (KnowledgeHomeAdminService, AdminMonitorService, /metrics).
//
// Neither service authenticates its caller, so the loopback bind *is* the
// access control. A non-loopback address would publish admin RPCs on every
// interface, which is a startup failure rather than something to log and
// accept.
func LoadOperatorListenAddr() (string, error) {
	addr := strings.TrimSpace(os.Getenv(operatorListenAddrEnv))
	if addr == "" {
		addr = defaultOperatorListenAddr
	}
	if err := requireLoopbackAddr(operatorListenAddrEnv, addr); err != nil {
		return "", err
	}
	return addr, nil
}

// requireLoopbackAddr rejects any host:port that is reachable from outside the
// container. An empty host (":9102") means all interfaces, so it is refused
// too — that shorthand is exactly how an admin surface gets published by
// accident.
func requireLoopbackAddr(name, addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%s=%q is not a valid host:port: %w", name, addr, err)
	}
	if port == "" {
		return fmt.Errorf("%s=%q has no port", name, addr)
	}
	if host == "" {
		return fmt.Errorf("%s=%q binds every interface (empty host); "+
			"this listener carries unauthenticated admin surfaces and must bind loopback", name, addr)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("%s=%q host is neither an IP nor localhost", name, addr)
	}
	if !ip.IsLoopback() {
		return fmt.Errorf("%s=%q binds a non-loopback address; "+
			"this listener carries unauthenticated admin surfaces and must bind loopback", name, addr)
	}
	return nil
}

// harvesterHealthAddrEnv names the harvester's only listener.
const harvesterHealthAddrEnv = "HARVESTER_HEALTH_ADDR"

const defaultHarvesterHealthAddr = "127.0.0.1:9103"

// LoadHarvesterHealthAddr returns the bind address for cmd/harvester's health
// and metrics listener.
//
// The harvester serves no API. This listener exists so the container has a
// health probe and a local metrics endpoint, and it binds loopback for the
// same reason the operator listener does: nothing on it authenticates a
// caller, so reachability is the control.
func LoadHarvesterHealthAddr() (string, error) {
	return loadLoopbackAddr(harvesterHealthAddrEnv, defaultHarvesterHealthAddr)
}

// datahubHealthAddrEnv names data-hub's loopback health listener.
const datahubHealthAddrEnv = "DATAHUB_HEALTH_ADDR"

const defaultDataHubHealthAddr = "127.0.0.1:9104"

// LoadDataHubHealthAddr returns the bind address for cmd/datahub's loopback
// health and metrics listener.
//
// This is separate from the mutual-TLS listener on purpose. Every request on
// that listener must present a client certificate, so a container healthcheck
// would need one too — and handing the probe a certificate just to answer
// /health widens the surface for no gain. The probe gets a loopback listener
// instead; it carries no data-plane surface at all.
func LoadDataHubHealthAddr() (string, error) {
	return loadLoopbackAddr(datahubHealthAddrEnv, defaultDataHubHealthAddr)
}

func loadLoopbackAddr(env, fallback string) (string, error) {
	addr := strings.TrimSpace(os.Getenv(env))
	if addr == "" {
		addr = fallback
	}
	if err := requireLoopbackAddr(env, addr); err != nil {
		return "", err
	}
	return addr, nil
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
	listenAddr, err := requiredEnv("DATAHUB_LISTEN_ADDR")
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

	if _, _, err := net.SplitHostPort(listenAddr); err != nil {
		return nil, fmt.Errorf("DATAHUB_LISTEN_ADDR=%q is not a valid host:port: %w", listenAddr, err)
	}

	return &DataHubConfig{
		ListenAddr:   listenAddr,
		CertFile:     certFile,
		KeyFile:      keyFile,
		CAFile:       caFile,
		AllowedPeers: peers,
	}, nil
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
