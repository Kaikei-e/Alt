package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Research-backed defaults for RAG retrieval parameters.
// Sources:
// - EMNLP 2024: "Searching for Best Practices in RAG"
// - Microsoft RAG Techniques Guide
// - Databricks Long Context RAG Performance
const (
	defaultRAGSearchLimit   = 50   // Standard for pre-ranking pool
	defaultRAGQuotaOriginal = 5    // Within 5-10 optimal range
	defaultRAGQuotaExpanded = 5    // Within 5-10 optimal range
	defaultRAGRRFK          = 60.0 // Standard RRF constant
)

// Application-specific defaults for temporal boost.
// These values should be tuned via A/B testing.
const (
	defaultTemporalBoost6h  = float32(1.3)  // 30% boost for 0-6 hours
	defaultTemporalBoost12h = float32(1.15) // 15% boost for 6-12 hours
	defaultTemporalBoost18h = float32(1.05) // 5% boost for 12-18 hours
)

// Re-ranking defaults (research-backed).
// Sources:
// - Pinecone: +15-30% NDCG@10 improvement with cross-encoder
// - ZeroEntropy: -35% LLM hallucinations
const (
	defaultRerankEnabled = true                      // Enabled by default per user preference
	defaultRerankModel   = "BAAI/bge-reranker-v2-m3" // BAAI multilingual model
	defaultRerankTopK    = 10                        // Rerank 50 -> 10
	defaultRerankTimeout = 10                        // Seconds (M4 reranker: 2-5s typical for 10 chunks)
)

// Hybrid search defaults (research-backed).
// Sources:
// - EMNLP 2024: Alpha=0.3 optimal
// - Weaviate/LlamaIndex: RRF fusion with k=60
const (
	defaultHybridSearchEnabled = true // Enabled by default per user preference
	defaultHybridAlpha         = 0.3  // EMNLP 2024 optimal (slightly BM25-heavy)
	defaultHybridBM25Limit     = 50   // Match vector search limit
)

// Dynamic language allocation default.
// When enabled, selects top N chunks by score regardless of language (JA/EN).
const (
	defaultDynamicLanguageAllocationEnabled = true // Dynamic score-based allocation by default
)

// DB pool defaults.
const (
	defaultDBMaxConns = 20
	defaultDBMinConns = 5
)

// Cache defaults.
const (
	defaultCacheSize = 256
	defaultCacheTTL  = 10 // minutes
)

// ServerConfig holds server-related settings.
type ServerConfig struct {
	Port        string
	ConnectPort string
}

// DBConfig holds database connection settings.
type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	MaxConns int
	MinConns int
}

// DSN returns the PostgreSQL connection string.
func (c DBConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		c.User, c.Password, c.Host, c.Port, c.Name)
}

// EmbedderConfig holds embedder (Ollama) settings.
type EmbedderConfig struct {
	URL     string
	Model   string
	Timeout int // Seconds
}

// AugurConfig holds Knowledge Augur (LLM generator) settings.
type AugurConfig struct {
	URL     string
	Model   string
	Timeout int // Seconds
}

// SearchConfig holds search indexer settings.
type SearchConfig struct {
	IndexerURL string
	Timeout    int // Seconds
}

// QueryExpansionConfig holds query expansion settings.
type QueryExpansionConfig struct {
	URL     string
	Timeout int // Seconds
}

// RAGConfig holds RAG retrieval parameters.
type RAGConfig struct {
	SearchLimit   int
	QuotaOriginal int
	QuotaExpanded int
	RRFK          float64
	MaxChunks     int
	MaxTokens     int
	MorningLetterMaxTokens int
	MaxPromptTokens int
	Locale        string
	PromptVersion string
	DynamicLanguageAllocationEnabled bool
}

// RerankConfig holds cross-encoder reranking settings.
type RerankConfig struct {
	Enabled bool
	URL     string
	Model   string
	TopK    int
	Timeout int // Seconds
}

// HybridConfig holds hybrid search (BM25+vector) settings.
type HybridConfig struct {
	Enabled   bool
	Alpha     float64
	BM25Limit int
}

// TemporalConfig holds temporal boost settings.
type TemporalConfig struct {
	Boost6h  float32
	Boost12h float32
	Boost18h float32
}

// BackendConfig holds alt-backend connection settings.
type BackendConfig struct {
	URL     string
	Timeout int // Seconds
}

// CacheConfig holds answer cache settings.
type CacheConfig struct {
	Size int // Max entries
	TTL  int // Minutes
}

// Config is the top-level configuration, organized by concern.
type Config struct {
	Env            string
	Server         ServerConfig
	DB             DBConfig
	Embedder       EmbedderConfig
	Augur          AugurConfig
	Search         SearchConfig
	QueryExpansion QueryExpansionConfig
	RAG            RAGConfig
	Rerank         RerankConfig
	Hybrid         HybridConfig
	Temporal       TemporalConfig
	Backend        BackendConfig
	Cache          CacheConfig
}

func Load() *Config {
	return &Config{
		Env: getEnv("ENV", "development"),
		Server: ServerConfig{
			Port:        getEnv("PORT", "9010"),
			ConnectPort: getEnv("CONNECT_PORT", "9011"),
		},
		DB: DBConfig{
			Host:     getEnv("DB_HOST", "rag-db"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "rag_user"),
			Password: getSecret("DB_PASSWORD", "DB_PASSWORD_FILE", "rag_password"),
			Name:     getEnv("DB_NAME", "rag_db"),
			MaxConns: getEnvInt("DB_MAX_CONNS", defaultDBMaxConns),
			MinConns: getEnvInt("DB_MIN_CONNS", defaultDBMinConns),
		},
		Embedder: EmbedderConfig{
			URL:     getEnvWithAlt("EMBEDDER_EXTERNAL", "EMBEDDER_EXTERNAL_URL", "http://embedder-external:11436"),
			Model:   getEnv("EMBEDDING_MODEL", "embeddinggemma"),
			Timeout: getEnvInt("EMBEDDER_TIMEOUT", 30),
		},
		Augur: AugurConfig{
			URL:     getEnvWithAlt("AUGUR_EXTERNAL", "AUGUR_EXTERNAL_URL", "http://augur-external:11435"),
			Model:   getEnv("AUGUR_KNOWLEDGE_MODEL", "gemma3-12b-rag"),
			Timeout: getEnvInt("OLLAMA_TIMEOUT", 300),
		},
		Search: SearchConfig{
			IndexerURL: getEnv("SEARCH_INDEXER_URL", "http://search-indexer:8080"),
			Timeout:    getEnvInt("SEARCH_INDEXER_TIMEOUT", 10),
		},
		QueryExpansion: QueryExpansionConfig{
			URL:     getEnv("QUERY_EXPANSION_URL", "http://news-creator:11434"),
			Timeout: getEnvInt("QUERY_EXPANSION_TIMEOUT", 3),
		},
		RAG: RAGConfig{
			SearchLimit:   getEnvInt("RAG_SEARCH_LIMIT", defaultRAGSearchLimit),
			QuotaOriginal: getEnvInt("RAG_QUOTA_ORIGINAL", defaultRAGQuotaOriginal),
			QuotaExpanded: getEnvInt("RAG_QUOTA_EXPANDED", defaultRAGQuotaExpanded),
			RRFK:          getEnvFloat64("RAG_RRF_K", defaultRAGRRFK),
			MaxChunks:     getEnvInt("RAG_DEFAULT_MAX_CHUNKS", 7),
			MaxTokens:     getEnvInt("RAG_DEFAULT_MAX_TOKENS", 6144),
			MorningLetterMaxTokens: getEnvInt("MORNING_LETTER_MAX_TOKENS", 4096),
			MaxPromptTokens: getEnvInt("RAG_MAX_PROMPT_TOKENS", 6000),
			Locale:        getEnv("RAG_DEFAULT_LOCALE", "ja"),
			PromptVersion: getEnv("RAG_PROMPT_VERSION", "alpha-v1"),
			DynamicLanguageAllocationEnabled: getEnvBool("RAG_DYNAMIC_LANGUAGE_ALLOCATION", defaultDynamicLanguageAllocationEnabled),
		},
		Rerank: RerankConfig{
			Enabled: getEnvBool("RERANK_ENABLED", defaultRerankEnabled),
			URL:     getEnv("RERANK_URL", "http://news-creator:11434"),
			Model:   getEnv("RERANK_MODEL", defaultRerankModel),
			TopK:    getEnvInt("RERANK_TOP_K", defaultRerankTopK),
			Timeout: getEnvInt("RERANK_TIMEOUT", defaultRerankTimeout),
		},
		Hybrid: HybridConfig{
			Enabled:   getEnvBool("HYBRID_SEARCH_ENABLED", defaultHybridSearchEnabled),
			Alpha:     getEnvFloat64("HYBRID_ALPHA", defaultHybridAlpha),
			BM25Limit: getEnvInt("HYBRID_BM25_LIMIT", defaultHybridBM25Limit),
		},
		Temporal: TemporalConfig{
			Boost6h:  getEnvFloat32("TEMPORAL_BOOST_6H", defaultTemporalBoost6h),
			Boost12h: getEnvFloat32("TEMPORAL_BOOST_12H", defaultTemporalBoost12h),
			Boost18h: getEnvFloat32("TEMPORAL_BOOST_18H", defaultTemporalBoost18h),
		},
		Backend: BackendConfig{
			URL:     getEnv("ALT_BACKEND_URL", "http://alt-backend:9000"),
			Timeout: getEnvInt("ALT_BACKEND_TIMEOUT", 30),
		},
		Cache: CacheConfig{
			Size: getEnvInt("RAG_CACHE_SIZE", defaultCacheSize),
			TTL:  getEnvInt("RAG_CACHE_TTL_MINUTES", defaultCacheTTL),
		},
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getSecret(envKey, fileEnvKey, fallback string) string {
	// 1. Try direct environment variable
	if value, ok := os.LookupEnv(envKey); ok {
		return value
	}

	// 2. Try reading from file specified by fileEnvKey
	if filePath, ok := os.LookupEnv(fileEnvKey); ok {
		content, err := os.ReadFile(filePath) //nolint:gosec // G304: path from trusted env var
		if err == nil {
			return strings.TrimSpace(string(content))
		}
	}

	return fallback
}

func getEnvWithAlt(key, altKey, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	if value, ok := os.LookupEnv(altKey); ok {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func getEnvFloat64(key string, fallback float64) float64 {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}
	}
	return fallback
}

func getEnvFloat32(key string, fallback float32) float32 {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.ParseFloat(value, 32); err == nil {
			return float32(parsed)
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return fallback
}
