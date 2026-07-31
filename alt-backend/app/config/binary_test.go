package config

import (
	"strings"
	"testing"
	"time"
)

func baseConfig() *Config {
	return &Config{
		AppEnv: "development",
		Server: ServerConfig{
			Port:         9000,
			ConnectPort:  9101,
			InternalPort: 9102,
			ReadTimeout:  300 * time.Second,
			WriteTimeout: 300 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		SearchIndexer: SearchIndexerConfig{ConnectURL: "http://search-indexer:9301"},
		PreProcessor:  PreProcessorConfig{URL: "http://pre-processor:9200", ConnectURL: "http://pre-processor:9202"},
		Rag:           RAGConfig{OrchestratorURL: "http://rag-orchestrator:9010", OrchestratorConnectURL: "http://rag-orchestrator:9011"},
		MQHub:         MQHubConfig{Enabled: true, ConnectURL: "http://mq-hub:9500"},
		AuthHub:       AuthHubConfig{URL: "http://auth-hub:8888"},
		Auth:          AuthConfig{BackendTokenSecret: "a-backend-token-secret-value"},
		Sovereign:     SovereignConfig{URL: "http://knowledge-sovereign:9500"},
	}
}

// Port range and uniqueness only mean something for the binary that opens
// those listeners. harvester and data-hub open neither, so leaving the checks
// in the shared path would fail them on values nothing reads.
func TestValidateBackendListeners(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{name: "defaults are valid", mutate: func(*Config) {}},
		{name: "rest port out of range", mutate: func(c *Config) { c.Server.Port = 0 }, wantErr: true},
		{name: "connect port out of range", mutate: func(c *Config) { c.Server.ConnectPort = 70000 }, wantErr: true},
		{name: "rest and connect ports collide", mutate: func(c *Config) { c.Server.ConnectPort = c.Server.Port }, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.mutate(cfg)
			err := ValidateBackendListeners(cfg)
			if tt.wantErr && err == nil {
				t.Fatal("expected an error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// The operator surface (KnowledgeHomeAdminService, AdminMonitorService) has no
// authentication of its own, so the default stays loopback. An *explicit*
// OPERATOR_LISTEN_ADDR may bind wider: compose publishes 127.0.0.1:9102:9102
// for altctl, and docker-proxy connects from the container's eth0 — a loopback
// bind inside the netns is unreachable from it, which took the host-side
// operator workflow offline. The widening has to be typed out by a human and
// is announced at startup (see ListenAddrReach).
func TestLoadOperatorListenAddr(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		want    string
		wantErr bool
	}{
		{name: "defaults to loopback", env: "", want: "127.0.0.1:9102"},
		{name: "explicit loopback IPv4", env: "127.0.0.1:9500", want: "127.0.0.1:9500"},
		{name: "explicit loopback IPv6", env: "[::1]:9500", want: "[::1]:9500"},
		{name: "localhost name", env: "localhost:9102", want: "localhost:9102"},
		{name: "explicit all interfaces is allowed", env: "0.0.0.0:9102", want: "0.0.0.0:9102"},
		{name: "explicit bare port is allowed", env: ":9102", want: ":9102"},
		{name: "explicit routable address is allowed", env: "10.0.0.4:9102", want: "10.0.0.4:9102"},
		{name: "missing port", env: "127.0.0.1", wantErr: true},
		{name: "empty port", env: "127.0.0.1:", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("OPERATOR_LISTEN_ADDR", tt.env)
			}
			got, err := LoadOperatorListenAddr()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("LoadOperatorListenAddr() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("LoadOperatorListenAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}

// OPS_LISTEN is the one listener all three binaries open: /health for the
// compose probe, /metrics for Prometheus, nothing else and no authentication.
// Because it carries nothing worth authenticating, a non-loopback bind is a
// normal deployment (compose sets ":9110" so Prometheus can reach it over
// alt-network) rather than an accident to refuse.
func TestLoadOpsListenAddr(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		want    string
		wantErr bool
	}{
		{name: "defaults to loopback", env: "", want: "127.0.0.1:9110"},
		{name: "compose binds every interface in the netns", env: ":9110", want: ":9110"},
		{name: "explicit 0.0.0.0", env: "0.0.0.0:9110", want: "0.0.0.0:9110"},
		{name: "explicit loopback", env: "127.0.0.1:19110", want: "127.0.0.1:19110"},
		{name: "surrounding whitespace is trimmed", env: "  :9110  ", want: ":9110"},
		{name: "missing port", env: "127.0.0.1", wantErr: true},
		{name: "empty port", env: ":", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("OPS_LISTEN", tt.env)
			}
			got, err := LoadOpsListenAddr()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("LoadOpsListenAddr() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("LoadOpsListenAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ListenAddrReach is what turns a relaxed bind rule into an audible one: the
// binaries print it at startup so "this admin port answers the container
// network" is a line in the log rather than something an operator has to infer
// from a compose file.
func TestListenAddrReach(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want ListenReach
	}{
		{name: "loopback IPv4", addr: "127.0.0.1:9102", want: ReachLoopback},
		{name: "loopback IPv6", addr: "[::1]:9102", want: ReachLoopback},
		{name: "localhost name", addr: "localhost:9102", want: ReachLoopback},
		{name: "bare port", addr: ":9110", want: ReachNetwork},
		{name: "all interfaces", addr: "0.0.0.0:9110", want: ReachNetwork},
		{name: "IPv6 all interfaces", addr: "[::]:9110", want: ReachNetwork},
		{name: "routable address", addr: "10.0.0.4:9102", want: ReachNetwork},
		{name: "unparseable is reported as reachable", addr: "nonsense", want: ReachNetwork},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ListenAddrReach(tt.addr); got != tt.want {
				t.Errorf("ListenAddrReach(%q) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}

// data-hub owns alt-db writes for every other service. tlsutil.OptionsFromEnv
// fails open twice (no MTLS_CLIENT_AUTH means the certificate is not verified;
// no MTLS_ALLOWED_PEERS means any CA-issued certificate is accepted), so
// data-hub reads its own required config instead and refuses to start without it.
func TestLoadDataHubConfig(t *testing.T) {
	const (
		cert  = "/etc/alt/tls/tls.crt"
		key   = "/etc/alt/tls/tls.key"
		ca    = "/etc/alt/tls/ca.crt"
		addr  = "0.0.0.0:9443"
		peers = "pre-processor, rag-orchestrator"
	)

	setAll := func(t *testing.T) {
		t.Helper()
		t.Setenv("DATAHUB_LISTEN_ADDR", addr)
		t.Setenv("DATAHUB_TLS_CERT_FILE", cert)
		t.Setenv("DATAHUB_TLS_KEY_FILE", key)
		t.Setenv("DATAHUB_TLS_CA_FILE", ca)
		t.Setenv("DATAHUB_ALLOWED_PEERS", peers)
	}

	t.Run("loads every required value", func(t *testing.T) {
		setAll(t)
		got, err := LoadDataHubConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ListenAddr != addr {
			t.Errorf("ListenAddr = %q, want %q", got.ListenAddr, addr)
		}
		if got.CertFile != cert || got.KeyFile != key || got.CAFile != ca {
			t.Errorf("TLS material = %q/%q/%q, want %q/%q/%q", got.CertFile, got.KeyFile, got.CAFile, cert, key, ca)
		}
		want := []string{"pre-processor", "rag-orchestrator"}
		if len(got.AllowedPeers) != len(want) {
			t.Fatalf("AllowedPeers = %v, want %v", got.AllowedPeers, want)
		}
		for i := range want {
			if got.AllowedPeers[i] != want[i] {
				t.Fatalf("AllowedPeers = %v, want %v", got.AllowedPeers, want)
			}
		}
	})

	missing := []struct {
		name string
		key  string
	}{
		{name: "listen addr", key: "DATAHUB_LISTEN_ADDR"},
		{name: "cert file", key: "DATAHUB_TLS_CERT_FILE"},
		{name: "key file", key: "DATAHUB_TLS_KEY_FILE"},
		{name: "ca file", key: "DATAHUB_TLS_CA_FILE"},
		{name: "peer allowlist", key: "DATAHUB_ALLOWED_PEERS"},
	}
	for _, tt := range missing {
		t.Run("missing "+tt.name+" fails startup", func(t *testing.T) {
			setAll(t)
			t.Setenv(tt.key, "")
			if _, err := LoadDataHubConfig(); err == nil {
				t.Fatalf("expected an error when %s is unset", tt.key)
			}
		})
	}

	t.Run("blank-only peer allowlist fails startup", func(t *testing.T) {
		setAll(t)
		t.Setenv("DATAHUB_ALLOWED_PEERS", " , ,")
		if _, err := LoadDataHubConfig(); err == nil {
			t.Fatal("expected an error for an allowlist with no usable entries")
		}
	})
}

// Rule 9: each binary states the upstreams it actually calls. A harvester
// missing SOVEREIGN_URL marks outbox rows PROCESSED while the knowledge events
// they carry evaporate, which is why the check is not keyed on APP_ENV the way
// the backend's is.
func TestValidateBinaryConfig(t *testing.T) {
	tests := []struct {
		name     string
		validate func(*Config) error
		mutate   func(*Config)
		wantErr  string
	}{
		{name: "backend accepts a complete config", validate: ValidateBackendConfig, mutate: func(*Config) {}},
		{name: "backend needs the search indexer", validate: ValidateBackendConfig,
			mutate: func(c *Config) { c.SearchIndexer.ConnectURL = "" }, wantErr: "SEARCH_INDEXER_CONNECT_URL"},
		{name: "backend needs the rag connect url", validate: ValidateBackendConfig,
			mutate: func(c *Config) { c.Rag.OrchestratorConnectURL = "" }, wantErr: "RAG_ORCHESTRATOR_CONNECT_URL"},
		{name: "backend needs the pre-processor", validate: ValidateBackendConfig,
			mutate: func(c *Config) { c.PreProcessor.ConnectURL = "" }, wantErr: "PRE_PROCESSOR_CONNECT_URL"},

		{name: "harvester accepts a complete config", validate: ValidateHarvesterConfig, mutate: func(*Config) {}},
		{name: "harvester needs sovereign in every environment", validate: ValidateHarvesterConfig,
			mutate: func(c *Config) { c.Sovereign.URL = "" }, wantErr: "SOVEREIGN_URL"},
		{name: "harvester needs the rag orchestrator", validate: ValidateHarvesterConfig,
			mutate: func(c *Config) { c.Rag.OrchestratorURL = "" }, wantErr: "RAG_ORCHESTRATOR_URL"},

		{name: "datahub accepts a complete config", validate: ValidateDataHubConfig, mutate: func(*Config) {}},
		{name: "datahub needs auth-hub", validate: ValidateDataHubConfig,
			mutate: func(c *Config) { c.AuthHub.URL = "" }, wantErr: "AUTH_HUB_URL"},
		{name: "datahub needs the backend token secret", validate: ValidateDataHubConfig,
			mutate: func(c *Config) { c.Auth.BackendTokenSecret = "" }, wantErr: "BACKEND_TOKEN_SECRET"},
		{name: "datahub needs sovereign in every environment", validate: ValidateDataHubConfig,
			mutate: func(c *Config) { c.Sovereign.URL = "" }, wantErr: "SOVEREIGN_URL"},
		{name: "datahub needs mq-hub", validate: ValidateDataHubConfig,
			mutate: func(c *Config) { c.MQHub.ConnectURL = "" }, wantErr: "MQHUB_CONNECT_URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.mutate(cfg)
			err := tt.validate(cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error mentioning %s", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want it to name %s", err, tt.wantErr)
			}
		})
	}
}

// MQHUB_ENABLED=false leaves every article RPC returning 200 while publishing
// nothing — the silent-fallback shape of ADR-000928. Outside development it is
// a startup failure; inside it, an explicit opt-out.
func TestValidateDataHubConfig_EventPublishingDisabled(t *testing.T) {
	tests := []struct {
		name    string
		appEnv  string
		wantErr bool
	}{
		{name: "development tolerates it", appEnv: "development"},
		{name: "staging refuses it", appEnv: "staging", wantErr: true},
		{name: "production refuses it", appEnv: "production", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			cfg.AppEnv = tt.appEnv
			cfg.MQHub.Enabled = false

			err := ValidateDataHubConfig(cfg)
			if tt.wantErr && err == nil {
				t.Fatal("expected an error for MQHUB_ENABLED=false")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// HARVESTER_HEALTH_ADDR and DATAHUB_HEALTH_ADDR gave two of the three binaries
// their own private probe port (:9103 / :9104) while compose, Prometheus and
// the Hurl suites all addressed one shared :9110. Both are gone; OPS_LISTEN is
// the single knob. Nothing may quietly resurrect them, because a container
// whose probe port disagrees with its healthcheck is a container that never
// reports healthy.
func TestRetiredPerBinaryHealthAddrEnvIsGone(t *testing.T) {
	for _, name := range []string{"HARVESTER_HEALTH_ADDR", "DATAHUB_HEALTH_ADDR"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "127.0.0.1:9999")
			got, err := LoadOpsListenAddr()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != "127.0.0.1:9110" {
				t.Errorf("LoadOpsListenAddr() = %q with %s set, want the OPS_LISTEN default %q",
					got, name, "127.0.0.1:9110")
			}
		})
	}
}

// cmd/datahub's healthcheck subcommand dials the mTLS listener as well as
// GETting the ops surface, and it must do that without the certificate paths
// LoadDataHubConfig requires — the probe runs before anything is loaded.
func TestLoadDataHubListenAddr(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		want    string
		wantErr bool
	}{
		{name: "compose value", env: ":9443", want: ":9443"},
		{name: "explicit host", env: "0.0.0.0:9443", want: "0.0.0.0:9443"},
		{name: "unset is a startup failure", env: "", wantErr: true},
		{name: "missing port", env: "0.0.0.0", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATAHUB_LISTEN_ADDR", tt.env)
			got, err := LoadDataHubListenAddr()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("LoadDataHubListenAddr() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("LoadDataHubListenAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}
