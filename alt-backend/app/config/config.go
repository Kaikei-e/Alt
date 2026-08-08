package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

type Config struct {
	Server        ServerConfig        `json:"server"`
	Database      DatabaseConfig      `json:"database"`
	RateLimit     RateLimitConfig     `json:"rate_limit"`
	Cache         CacheConfig         `json:"cache"`
	Logging       LoggingConfig       `json:"logging"`
	HTTP          HTTPConfig          `json:"http"`
	PreProcessor  PreProcessorConfig  `json:"pre_processor"`
	SearchIndexer SearchIndexerConfig `json:"search_indexer"`
	Recap         RecapConfig         `json:"recap"`
	Auth          AuthConfig          `json:"auth"`
	Rag           RAGConfig           `json:"rag"`
	AuthHub       AuthHubConfig       `json:"auth_hub"`
	MQHub         MQHubConfig         `json:"mq_hub"`
	InternalAPI   InternalAPIConfig   `json:"internal_api"`
	ImageProxy    ImageProxyConfig    `json:"image_proxy"`
	KnowledgeHome KnowledgeHomeConfig `json:"knowledge_home"`
	AdminMonitor  AdminMonitorConfig  `json:"admin_monitor"`
	Sovereign     SovereignConfig     `json:"sovereign"`
	Meilisearch   MeilisearchConfig   `json:"meilisearch"`
	WebPush       WebPushConfig       `json:"web_push"`

	// AppEnv drives the remaining environment-keyed checks (e.g. Knowledge
	// Sovereign wiring, BACKEND_TOKEN_SECRET length). It must be one of
	// appEnvValues: an unrecognised value is rejected rather than degrading to
	// the permissive development behaviour, because APP_ENV is optional and a
	// "prod" typo would otherwise be indistinguishable from "unset".
	// Guards that protect a deployment regardless of environment (see
	// validateImageProxyConfig) must NOT be keyed on this field.
	AppEnv string `json:"app_env" env:"APP_ENV" default:"development"`

	// Legacy fields for backward compatibility
	Port               int           `json:"port"`
	DatabaseURL        string        `json:"database_url"`
	LogLevel           string        `json:"log_level"`
	MeilisearchURL     string        `json:"meilisearch_url"`
	RateLimitInterval  time.Duration `json:"rate_limit_interval"`
	MaxPaginationLimit int           `json:"max_pagination_limit"`
}

// SovereignConfig holds configuration for the Knowledge Sovereign Connect-RPC
// service. URL has no default: an empty value means the client stays
// disabled (see di/knowledge_module.go logSovereignWiringState).
type SovereignConfig struct {
	URL        string `json:"url" env:"SOVEREIGN_URL" default:""`
	MetricsURL string `json:"metrics_url" env:"SOVEREIGN_METRICS_URL" default:"http://knowledge-sovereign:9501"`
}

// MeilisearchConfig holds configuration for the Meilisearch health-check target.
type MeilisearchConfig struct {
	Host string `json:"host" env:"MEILISEARCH_HOST" default:"http://meilisearch:7700"`
}

type PreProcessorConfig struct {
	Enabled    bool   `json:"enabled" env:"PRE_PROCESSOR_ENABLED" default:"true"`
	URL        string `json:"url" env:"PRE_PROCESSOR_URL" default:"http://pre-processor:9200"`
	ConnectURL string `json:"connect_url" env:"PRE_PROCESSOR_CONNECT_URL" default:"http://pre-processor:9202"`
}

type SearchIndexerConfig struct {
	ConnectURL string `json:"connect_url" env:"SEARCH_INDEXER_CONNECT_URL" default:"http://search-indexer:9301"`
}

type RecapConfig struct {
	DefaultPageSize  int    `json:"default_page_size" env:"RECAP_DEFAULT_PAGE_SIZE" default:"500"`
	MaxPageSize      int    `json:"max_page_size" env:"RECAP_MAX_PAGE_SIZE" default:"2000"`
	MaxRangeDays     int    `json:"max_range_days" env:"RECAP_MAX_RANGE_DAYS" default:"8"`
	RateLimitRPS     int    `json:"rate_limit_rps" env:"RECAP_RATE_LIMIT_RPS" default:"4"`
	RateLimitBurst   int    `json:"rate_limit_burst" env:"RECAP_RATE_LIMIT_BURST" default:"8"`
	MaxArticleBytes  int    `json:"max_article_bytes" env:"RECAP_MAX_ARTICLE_BYTES" default:"2097152"`
	ClusterDraftPath string `json:"cluster_draft_path" env:"RECAP_CLUSTER_DRAFT_PATH" default:"docs/genre-reorg-draft.json"`
	WorkerURL        string `json:"worker_url" env:"RECAP_WORKER_URL" default:"http://recap-worker:9005"`
}

type RAGConfig struct {
	OrchestratorURL        string `json:"orchestrator_url" env:"RAG_ORCHESTRATOR_URL" default:"http://rag-orchestrator:9010"`
	OrchestratorConnectURL string `json:"orchestrator_connect_url" env:"RAG_ORCHESTRATOR_CONNECT_URL" default:"http://rag-orchestrator:9011"`
}

type AuthHubConfig struct {
	URL string `json:"url" env:"AUTH_HUB_URL" default:"http://auth-hub:8888"`
}

// MQHubConfig holds configuration for mq-hub event broker.
type MQHubConfig struct {
	// Enabled determines if event publishing via mq-hub is active.
	Enabled bool `json:"enabled" env:"MQHUB_ENABLED" default:"false"`
	// ConnectURL is the Connect-RPC URL for mq-hub service.
	ConnectURL string `json:"connect_url" env:"MQHUB_CONNECT_URL" default:"http://mq-hub:9500"`
}

// ImageProxyConfig holds configuration for the OGP image proxy.
// Enabled defaults to true, so a deployment that supplies no secret is a
// misconfiguration, not an opt-out: set IMAGE_PROXY_ENABLED=false to disable
// the proxy explicitly (see validateImageProxyConfig).
type ImageProxyConfig struct {
	Enabled     bool   `json:"enabled" env:"IMAGE_PROXY_ENABLED" default:"true"`
	Secret      string `json:"secret" env:"IMAGE_PROXY_SECRET"`
	SecretFile  string `json:"-" env:"IMAGE_PROXY_SECRET_FILE"`
	CacheTTLMin int    `json:"cache_ttl_min" env:"IMAGE_PROXY_CACHE_TTL_MINUTES" default:"10080"`
	MaxWidth    int    `json:"max_width" env:"IMAGE_PROXY_MAX_WIDTH" default:"600"`
	WebPQuality int    `json:"webp_quality" env:"IMAGE_PROXY_WEBP_QUALITY" default:"80"`
}

// WebPushConfig holds the VAPID identity.
//
// The two halves are read by different binaries and neither reads both.
// cmd/backend needs only PublicKey, to hand to the browser; it sends no pushes,
// and giving it the signing credential would put one in the process with the
// largest attack surface for a capability it never exercises. cmd/notifier
// needs PrivateKey and Subject, and its container is the only one the secret is
// mounted into. ValidateBackendConfig and ValidateNotifierConfig each require
// only their own half, so neither binary can be started with a credential it
// has no use for.
//
// PublicKey is served rather than baked into the frontend bundle so that a key
// change does not require a frontend rebuild, and so a client can compare the
// served key against the one its live subscription was created under —
// rotating the keypair invalidates every existing subscription, and that
// comparison is how a browser notices.
//
// There is no Enabled flag and no default. Web Push either has an identity or
// it does not, and config.ValidateBackendConfig rejects an empty one at
// startup rather than letting GetPushConfig answer with an empty string that
// the browser would pass to pushManager.subscribe and fail on
// (CLAUDE.md rule 9).
type WebPushConfig struct {
	PublicKey     string `json:"vapid_public_key" env:"VAPID_PUBLIC_KEY"`
	PublicKeyFile string `json:"-" env:"VAPID_PUBLIC_KEY_FILE"`
	// PrivateKey never appears in JSON: this struct is reachable from a config
	// dump, and a signing key that leaks into a log is a key that has to be
	// rotated — which invalidates every existing browser subscription.
	PrivateKey     string `json:"-" env:"VAPID_PRIVATE_KEY"`
	PrivateKeyFile string `json:"-" env:"VAPID_PRIVATE_KEY_FILE"`
	// Subject is the RFC 8292 `sub` claim: a contact URI a push service can use
	// to reach the operator. Apple rejects anything that is not a mailto: or
	// https: URI, and only Apple does — so a malformed value here presents as
	// "iOS stopped receiving notifications" rather than as a broken deploy.
	Subject string `json:"vapid_subject" env:"VAPID_SUBJECT"`
}

// KnowledgeHomeConfig holds configuration for Knowledge Home feature flags.
type KnowledgeHomeConfig struct {
	EnableHomePage     bool   `json:"enable_home_page" env:"KNOWLEDGE_HOME_ENABLE_PAGE" default:"false"`
	EnableTracking     bool   `json:"enable_tracking" env:"KNOWLEDGE_HOME_ENABLE_TRACKING" default:"false"`
	EnableProjectionV2 bool   `json:"enable_projection_v2" env:"KNOWLEDGE_HOME_ENABLE_PROJECTION_V2" default:"false"`
	RolloutPercentage  int    `json:"rollout_percentage" env:"KNOWLEDGE_HOME_ROLLOUT_PERCENTAGE" default:"0"`
	AllowedUserIDs     string `json:"allowed_user_ids" env:"KNOWLEDGE_HOME_ALLOWED_USER_IDS" default:""`
	// RecallWeightSet pins which weights map the recall projector uses.
	// ADR-000913 §D-9 (Twitter Heavy-Ranker grounding). "v1_fixed" is the
	// pre-Heavy-Ranker default; "v2_heavy_ranker" introduces explicit
	// negative weights for recently_dismissed / low_summary_confidence.
	RecallWeightSet        string        `json:"recall_weight_set" env:"KNOWLEDGE_HOME_RECALL_WEIGHT_SET" default:"v1_fixed"`
	EnableLens             bool          `json:"enable_lens" env:"KNOWLEDGE_HOME_ENABLE_LENS" default:"false"`
	EnableStreamUpdates    bool          `json:"enable_stream_updates" env:"KNOWLEDGE_HOME_ENABLE_STREAM_UPDATES" default:"false"`
	EnableSupersedeUX      bool          `json:"enable_supersede_ux" env:"KNOWLEDGE_HOME_ENABLE_SUPERSEDE_UX" default:"false"`
	ProjectorPollInterval  time.Duration `json:"projector_poll_interval" env:"KNOWLEDGE_HOME_PROJECTOR_POLL_INTERVAL" default:"5s"`
	ProjectorTimeout       time.Duration `json:"projector_timeout" env:"KNOWLEDGE_HOME_PROJECTOR_TIMEOUT" default:"25s"`
	ProjectorBatchSize     int           `json:"projector_batch_size" env:"KNOWLEDGE_HOME_PROJECTOR_BATCH_SIZE" default:"500"`
	ProjectorNotifyChannel string        `json:"projector_notify_channel" env:"KNOWLEDGE_HOME_PROJECTOR_NOTIFY_CHANNEL" default:"knowledge_projector"`
	ProjectorDirectDBHost  string        `json:"projector_direct_db_host" env:"KNOWLEDGE_HOME_PROJECTOR_DIRECT_DB_HOST" default:""`
	ProjectorDirectDBPort  string        `json:"projector_direct_db_port" env:"KNOWLEDGE_HOME_PROJECTOR_DIRECT_DB_PORT" default:""`
}

// AdminMonitorConfig controls the admin-facing Prometheus observability endpoint.
// Queries are allowlisted server-side and run through a gated gateway with
// cache, singleflight, and rate limit.
type AdminMonitorConfig struct {
	Enabled        bool          `json:"enabled" env:"ADMIN_MONITOR_ENABLED" default:"false"`
	PrometheusURL  string        `json:"prometheus_url" env:"ADMIN_MONITOR_PROMETHEUS_URL" default:"http://prometheus:9090"`
	QueryTimeout   time.Duration `json:"query_timeout" env:"ADMIN_MONITOR_QUERY_TIMEOUT" default:"3s"`
	CacheTTL       time.Duration `json:"cache_ttl" env:"ADMIN_MONITOR_CACHE_TTL" default:"10s"`
	RateLimitRPS   int           `json:"rate_limit_rps" env:"ADMIN_MONITOR_RATE_LIMIT_RPS" default:"5"`
	RateLimitBurst int           `json:"rate_limit_burst" env:"ADMIN_MONITOR_RATE_LIMIT_BURST" default:"10"`
	StreamInterval time.Duration `json:"stream_interval" env:"ADMIN_MONITOR_STREAM_INTERVAL" default:"5s"`
}

// InternalAPIConfig holds configuration for the internal service-to-service API.
// Authentication is established at the TLS transport layer (mTLS); the struct
// is retained for forward-compatible field access and currently empty.
type InternalAPIConfig struct{}

type AuthConfig struct {
	BackendTokenSecret     string `json:"backend_token_secret" env:"BACKEND_TOKEN_SECRET"`
	BackendTokenSecretFile string `json:"-" env:"BACKEND_TOKEN_SECRET_FILE"`
	BackendTokenIssuer     string `json:"backend_token_issuer" env:"BACKEND_TOKEN_ISSUER"`
	BackendTokenAudience   string `json:"backend_token_audience" env:"BACKEND_TOKEN_AUDIENCE"`
}

type ServerConfig struct {
	Port        int `json:"port" env:"SERVER_PORT" default:"9000"`
	ConnectPort int `json:"connect_port" env:"CONNECT_PORT" default:"9101"`
	// There is no InternalPort. The listener it named became cmd/backend's
	// operator listener, whose bind address comes from OPERATOR_LISTEN_ADDR
	// via config.LoadOperatorListenAddr — a host:port, not a bare port, because
	// how far it binds is the whole access control for the admin services.
	// Keeping a parsed-but-unread INTERNAL_PORT here would have made a compose
	// line look like it moved a listener that it could not move (rule 8).
	ReadTimeout        time.Duration `json:"read_timeout" env:"SERVER_READ_TIMEOUT" default:"300s"` // Extended for LLM processing (nginx timeout 240s + margin)
	WriteTimeout       time.Duration `json:"write_timeout" env:"SERVER_WRITE_TIMEOUT" default:"300s"`
	IdleTimeout        time.Duration `json:"idle_timeout" env:"SERVER_IDLE_TIMEOUT" default:"120s"`
	SSEInterval        time.Duration `json:"sse_interval" env:"SERVER_SSE_INTERVAL" default:"5s"`
	CORSAllowedOrigins []string      `json:"cors_allowed_origins" env:"CORS_ALLOWED_ORIGINS" default:"http://localhost:3000,http://localhost:80,http://localhost:4173,https://curionoah.com"`
}

type RateLimitConfig struct {
	ExternalAPIInterval time.Duration `json:"external_api_interval" env:"RATE_LIMIT_EXTERNAL_API_INTERVAL" default:"10s"`
	ExternalAPIBurst    int           `json:"external_api_burst" env:"RATE_LIMIT_EXTERNAL_API_BURST" default:"3"`
	FeedFetchLimit      int           `json:"feed_fetch_limit" env:"RATE_LIMIT_FEED_FETCH_LIMIT" default:"100"`

	// CoordinationRedisURL points at the arbiter that makes the per-host
	// interval hold across processes. ADR-000954 put two internet-facing
	// binaries in the deployment — cmd/backend fetches synchronously for the
	// user, cmd/harvester fetches on a schedule — and a process-local token
	// bucket lets the same publisher see two requests per interval, which is
	// the thing CLAUDE.md rule 2 promises will not happen.
	//
	// It has no default on purpose. Empty is the explicit "local mode": the
	// composition roots log host_rate_limiter.mode=local at startup and state
	// that the guarantee is per process, so an unset value can never be
	// mistaken for wiring that silently went missing (rules 8 and 9).
	CoordinationRedisURL string `json:"coordination_redis_url" env:"HOST_RATE_LIMITER_REDIS_URL" default:""`

	// InteractiveSlotWait bounds how long a request a user is waiting on may
	// queue for its turn at a host before giving the turn up and telling the
	// client when to come back. Without it, a Visual Preview swipe queues
	// behind the harvester's collector on the same publisher and spends its
	// whole deadline waiting to fail — a background job blocking an
	// interactive one.
	//
	// This never widens the interval above. Giving the turn up early can only
	// make a publisher see fewer requests, so CLAUDE.md rule 2 is untouched:
	// the fix is to give up faster, never to fetch more often.
	//
	// Zero is the explicit "queue like a background job" setting, logged at
	// startup so it cannot be confused with wiring that went missing.
	InteractiveSlotWait time.Duration `json:"interactive_slot_wait" env:"RATE_LIMIT_INTERACTIVE_SLOT_WAIT" default:"2s"`

	// DOS Protection Configuration
	DOSProtection DOSProtectionConfig `json:"dos_protection"`
}

type DOSProtectionConfig struct {
	Enabled          bool                 `json:"enabled" env:"DOS_PROTECTION_ENABLED" default:"true"`
	RateLimit        int                  `json:"rate_limit" env:"DOS_PROTECTION_RATE_LIMIT" default:"100"`
	BurstLimit       int                  `json:"burst_limit" env:"DOS_PROTECTION_BURST_LIMIT" default:"200"`
	WindowSize       time.Duration        `json:"window_size" env:"DOS_PROTECTION_WINDOW_SIZE" default:"1m"`
	BlockDuration    time.Duration        `json:"block_duration" env:"DOS_PROTECTION_BLOCK_DURATION" default:"5m"`
	WhitelistedPaths []string             `json:"whitelisted_paths"`
	CircuitBreaker   CircuitBreakerConfig `json:"circuit_breaker"`
}

type CircuitBreakerConfig struct {
	Enabled          bool          `json:"enabled" env:"CIRCUIT_BREAKER_ENABLED" default:"true"`
	FailureThreshold int           `json:"failure_threshold" env:"CIRCUIT_BREAKER_FAILURE_THRESHOLD" default:"10"`
	TimeoutDuration  time.Duration `json:"timeout_duration" env:"CIRCUIT_BREAKER_TIMEOUT_DURATION" default:"30s"`
	RecoveryTimeout  time.Duration `json:"recovery_timeout" env:"CIRCUIT_BREAKER_RECOVERY_TIMEOUT" default:"60s"`
}

type DatabaseConfig struct {
	MaxConnections    int           `json:"max_connections" env:"DB_MAX_CONNECTIONS" default:"25"`
	ConnectionTimeout time.Duration `json:"connection_timeout" env:"DB_CONNECTION_TIMEOUT" default:"30s"`
}

type CacheConfig struct {
	FeedCacheExpiry   time.Duration `json:"feed_cache_expiry" env:"CACHE_FEED_EXPIRY" default:"300s"`
	SearchCacheExpiry time.Duration `json:"search_cache_expiry" env:"CACHE_SEARCH_EXPIRY" default:"900s"`
}

type LoggingConfig struct {
	Level  string `json:"level" env:"LOG_LEVEL" default:"info"`
	Format string `json:"format" env:"LOG_FORMAT" default:"json"`
}

type HTTPConfig struct {
	ClientTimeout       time.Duration `json:"client_timeout" env:"HTTP_CLIENT_TIMEOUT" default:"30s"`
	DialTimeout         time.Duration `json:"dial_timeout" env:"HTTP_DIAL_TIMEOUT" default:"10s"`
	TLSHandshakeTimeout time.Duration `json:"tls_handshake_timeout" env:"HTTP_TLS_HANDSHAKE_TIMEOUT" default:"10s"`
	IdleConnTimeout     time.Duration `json:"idle_conn_timeout" env:"HTTP_IDLE_CONN_TIMEOUT" default:"90s"`
}

// NewConfig creates a new configuration by loading from environment variables
// with fallback to default values
func NewConfig() (*Config, error) {
	config := &Config{}

	if err := loadFromEnvironment(config); err != nil {
		return nil, err
	}

	if err := validateConfig(config); err != nil {
		return nil, err
	}

	// Load image proxy secret from file if configured (Docker Secrets support)
	if config.ImageProxy.SecretFile != "" {
		content, err := os.ReadFile(config.ImageProxy.SecretFile)
		if err == nil {
			config.ImageProxy.Secret = strings.TrimSpace(string(content))
		}
	}

	// Load the VAPID public key from file if configured (Docker Secrets
	// support). A read failure leaves the env value in place, and
	// ValidateBackendConfig rejects an empty result — so a mounted-but-broken
	// secret fails startup rather than serving an empty key.
	if config.WebPush.PublicKeyFile != "" {
		content, err := os.ReadFile(config.WebPush.PublicKeyFile)
		if err == nil {
			config.WebPush.PublicKey = strings.TrimSpace(string(content))
		}
	}

	// Same pattern for the signing key, read only by cmd/notifier.
	// ValidateNotifierConfig rejects an empty result, so a secret that is
	// mounted but unreadable fails startup instead of producing a dispatcher
	// that answers every send with a 401 nobody looks at.
	if config.WebPush.PrivateKeyFile != "" {
		content, err := os.ReadFile(config.WebPush.PrivateKeyFile)
		if err == nil {
			config.WebPush.PrivateKey = strings.TrimSpace(string(content))
		}
	}

	// Load backend token secret from file if configured (Docker Secrets support)
	if config.Auth.BackendTokenSecretFile != "" {
		content, err := os.ReadFile(config.Auth.BackendTokenSecretFile)
		if err == nil {
			config.Auth.BackendTokenSecret = strings.TrimSpace(string(content))
		}
		// If file read fails, we fall back to the env var value (if any) or keep it empty
	}

	// Set defaults for JWT issuer and audience if not provided
	if config.Auth.BackendTokenIssuer == "" {
		config.Auth.BackendTokenIssuer = "auth-hub"
	}
	if config.Auth.BackendTokenAudience == "" {
		config.Auth.BackendTokenAudience = "alt-backend"
	}

	if err := validateAppEnv(config.AppEnv); err != nil {
		return nil, fmt.Errorf("app env validation failed: %w", err)
	}
	slog.Info("app_env_resolved", "app_env", config.AppEnv)

	// Validate the image proxy after secrets are loaded. This guard is
	// deliberately environment-independent: keying it on APP_ENV made it dead
	// code, because APP_ENV is set in no compose file.
	if err := validateImageProxyConfig(&config.ImageProxy); err != nil {
		return nil, fmt.Errorf("image proxy config validation failed: %w", err)
	}

	// Validate auth configuration after secrets are loaded
	// This ensures fail-fast behavior for misconfigured production deployments
	if err := validateAuthConfig(&config.Auth, config.AppEnv); err != nil {
		return nil, fmt.Errorf("auth config validation failed: %w", err)
	}

	return config, nil
}

// appEnvValues enumerates the environments alt-backend recognises. Anything
// else is a typo, and a typo must not silently buy the development behaviour
// of the environment-keyed guards.
var appEnvValues = []string{"development", "staging", "production"}

func validateAppEnv(appEnv string) error {
	for _, valid := range appEnvValues {
		if appEnv == valid {
			return nil
		}
	}
	return fmt.Errorf("APP_ENV must be one of: %s, got %s",
		strings.Join(appEnvValues, ", "), appEnv)
}

// validateImageProxyConfig enforces CLAUDE.md rule 9 for the OGP image proxy:
// a missing secret is missing required config, so startup exits non-zero
// instead of limping along with the proxy silently unwired.
//
// IMAGE_PROXY_ENABLED defaults to true and compose/core.yaml mounts
// IMAGE_PROXY_SECRET_FILE=/run/secrets/image_proxy_secret, so an empty or
// whitespace-only secret file used to land in di/image_module.go's
// "enabled && !hasSecret" branch, which logged slog.Error and continued —
// leaving every warmer/backfill job no-oping forever. The old panic there was
// keyed on APP_ENV=production, which is set in no compose file and therefore
// never fired.
//
// The explicit opt-out is IMAGE_PROXY_ENABLED=false, which di/image_module.go
// reports as a loud image_proxy_disabled startup log.
func validateImageProxyConfig(config *ImageProxyConfig) error {
	if !config.Enabled {
		return nil
	}

	if strings.TrimSpace(config.Secret) == "" {
		return fmt.Errorf("IMAGE_PROXY_ENABLED=true requires a non-empty secret: " +
			"set IMAGE_PROXY_SECRET, point IMAGE_PROXY_SECRET_FILE at a non-empty file, " +
			"or set IMAGE_PROXY_ENABLED=false to disable the image proxy explicitly")
	}

	return nil
}

// Load is an alias for NewConfig for backward compatibility
func Load() (*Config, error) {
	return NewConfig()
}
