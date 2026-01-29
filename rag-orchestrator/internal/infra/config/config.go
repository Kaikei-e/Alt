package config

import (
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
	defaultRerankTimeout = 30                        // Seconds
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

type Config struct {
	Env                   string
	Port                  string
	ConnectPort           string // Connect-RPC server port
	DBHost                string
	DBPort                string
	DBUser                string
	DBPassword            string
	DBName                string
	OllamaURL             string
	EmbeddingModel        string
	PromptVersion         string
	AnswerMaxChunks       int
	AnswerMaxTokens       int
	DefaultLocale         string
	KnowledgeAugurURL     string
	KnowledgeAugurModel   string
	OllamaTimeout         int // Seconds (deprecated, use EmbedderTimeout)
	EmbedderTimeout       int // Seconds - timeout for embedder-external API calls
	SearchIndexerURL      string
	SearchIndexerTimeout  int // Seconds
	AltBackendURL         string
	AltBackendTimeout     int    // Seconds
	QueryExpansionURL     string // news-creator URL for query expansion
	QueryExpansionTimeout int    // Seconds

	// RAG Retrieval Parameters (research-backed defaults)
	RAGSearchLimit   int     // Vector search initial fetch (default: 50)
	RAGQuotaOriginal int     // Chunks from original query (default: 5)
	RAGQuotaExpanded int     // Chunks from expanded queries (default: 5)
	RAGRRFK          float64 // RRF constant (default: 60.0)

	// Temporal Boost Parameters (application-specific)
	TemporalBoost6h  float32 // Boost for 0-6 hours (default: 1.3)
	TemporalBoost12h float32 // Boost for 6-12 hours (default: 1.15)
	TemporalBoost18h float32 // Boost for 12-18 hours (default: 1.05)

	// Re-ranking Parameters (research-backed)
	RerankEnabled bool   // Enable cross-encoder reranking (default: true)
	RerankURL     string // news-creator URL for reranking
	RerankModel   string // Cross-encoder model name (default: bge-reranker-v2-m3)
	RerankTopK    int    // Results after reranking (default: 10)
	RerankTimeout int    // Seconds (default: 30)

	// Hybrid Search Parameters (research-backed)
	HybridSearchEnabled bool    // Enable BM25+vector fusion (default: true)
	HybridAlpha         float64 // Weight: 0.0=BM25, 1.0=vector (default: 0.3)
	HybridBM25Limit     int     // BM25 candidates to fetch (default: 50)

	// Dynamic Language Allocation Parameter
	DynamicLanguageAllocationEnabled bool // Enable dynamic JA/EN allocation by score (default: true)
}

func Load() *Config {
	return &Config{
		Env:                   getEnv("ENV", "development"),
		Port:                  getEnv("PORT", "9010"),
		ConnectPort:           getEnv("CONNECT_PORT", "9011"),
		DBHost:                getEnv("DB_HOST", "rag-db"),
		DBPort:                getEnv("DB_PORT", "5432"),
		DBUser:                getEnv("DB_USER", "rag_user"),
		DBPassword:            getSecret("DB_PASSWORD", "DB_PASSWORD_FILE", "rag_password"),
		DBName:                getEnv("DB_NAME", "rag_db"),
		OllamaURL:             getEnvWithAlt("EMBEDDER_EXTERNAL", "EMBEDDER_EXTERNAL_URL", "http://embedder-external:11436"),
		EmbeddingModel:        getEnv("EMBEDDING_MODEL", "embeddinggemma"), // Default to gemma3:4b if not specified, assuming it supports embedding
		PromptVersion:         getEnv("RAG_PROMPT_VERSION", "alpha-v1"),
		AnswerMaxChunks:       getEnvInt("RAG_DEFAULT_MAX_CHUNKS", 10),
		AnswerMaxTokens:       getEnvInt("RAG_DEFAULT_MAX_TOKENS", 2560),
		DefaultLocale:         getEnv("RAG_DEFAULT_LOCALE", "ja"),
		KnowledgeAugurURL:     getEnvWithAlt("AUGUR_EXTERNAL", "AUGUR_EXTERNAL_URL", "http://augur-external:11435"),
		KnowledgeAugurModel:   getEnv("AUGUR_KNOWLEDGE_MODEL", "qwen3-14b-rag"),
		OllamaTimeout:         getEnvInt("OLLAMA_TIMEOUT", 300),
		EmbedderTimeout:       getEnvInt("EMBEDDER_TIMEOUT", 30), // 30s default to stay under Cloudflare 100s timeout
		SearchIndexerURL:      getEnv("SEARCH_INDEXER_URL", "http://search-indexer:8080"),
		SearchIndexerTimeout:  getEnvInt("SEARCH_INDEXER_TIMEOUT", 10),
		AltBackendURL:         getEnv("ALT_BACKEND_URL", "http://alt-backend:9000"),
		AltBackendTimeout:     getEnvInt("ALT_BACKEND_TIMEOUT", 30),
		QueryExpansionURL:     getEnv("QUERY_EXPANSION_URL", "http://news-creator:11434"),
		QueryExpansionTimeout: getEnvInt("QUERY_EXPANSION_TIMEOUT", 30),

		// RAG Retrieval Parameters
		RAGSearchLimit:   getEnvInt("RAG_SEARCH_LIMIT", defaultRAGSearchLimit),
		RAGQuotaOriginal: getEnvInt("RAG_QUOTA_ORIGINAL", defaultRAGQuotaOriginal),
		RAGQuotaExpanded: getEnvInt("RAG_QUOTA_EXPANDED", defaultRAGQuotaExpanded),
		RAGRRFK:          getEnvFloat64("RAG_RRF_K", defaultRAGRRFK),

		// Temporal Boost Parameters
		TemporalBoost6h:  getEnvFloat32("TEMPORAL_BOOST_6H", defaultTemporalBoost6h),
		TemporalBoost12h: getEnvFloat32("TEMPORAL_BOOST_12H", defaultTemporalBoost12h),
		TemporalBoost18h: getEnvFloat32("TEMPORAL_BOOST_18H", defaultTemporalBoost18h),

		// Re-ranking Parameters
		RerankEnabled: getEnvBool("RERANK_ENABLED", defaultRerankEnabled),
		RerankURL:     getEnv("RERANK_URL", "http://news-creator:11434"),
		RerankModel:   getEnv("RERANK_MODEL", defaultRerankModel),
		RerankTopK:    getEnvInt("RERANK_TOP_K", defaultRerankTopK),
		RerankTimeout: getEnvInt("RERANK_TIMEOUT", defaultRerankTimeout),

		// Hybrid Search Parameters
		HybridSearchEnabled: getEnvBool("HYBRID_SEARCH_ENABLED", defaultHybridSearchEnabled),
		HybridAlpha:         getEnvFloat64("HYBRID_ALPHA", defaultHybridAlpha),
		HybridBM25Limit:     getEnvInt("HYBRID_BM25_LIMIT", defaultHybridBM25Limit),

		// Dynamic Language Allocation Parameter
		DynamicLanguageAllocationEnabled: getEnvBool("RAG_DYNAMIC_LANGUAGE_ALLOCATION", defaultDynamicLanguageAllocationEnabled),
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
		// If file read fails, we could log but here we just fall through or return fallback?
		// For now, let's just proceed.
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
