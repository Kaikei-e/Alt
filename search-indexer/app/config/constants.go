package config

import (
	"os"
	"strconv"
	"time"
)

// Service constants with env var override support.
var (
	IndexInterval       = durationEnv("INDEX_INTERVAL", 5*time.Minute)
	IndexBatchSize      = intEnv("INDEX_BATCH_SIZE", 200)
	IndexRetryInterval  = durationEnv("INDEX_RETRY_INTERVAL", 1*time.Minute)
	HTTPAddr            = stringEnv("HTTP_ADDR", ":9300")
	ConnectAddr         = stringEnv("CONNECT_ADDR", ":9301")
	DBTimeout           = durationEnv("DB_TIMEOUT", 10*time.Second)
	MeiliTimeout        = durationEnv("MEILI_TIMEOUT", 15*time.Second)
	RecapWorkerURL      = stringEnv("RECAP_WORKER_URL", "")
	RecapIndexInterval  = durationEnv("RECAP_INDEX_INTERVAL", 5*time.Minute)
	RecapIndexBatchSize = intEnv("RECAP_INDEX_BATCH_SIZE", 200)
	// MeiliHybridEmbedder names the embedder Meilisearch uses for hybrid search.
	// Empty disables hybrid mode (BM25 only). When set, the driver attaches
	// the embedder name + semantic ratio to every SearchRequest.
	MeiliHybridEmbedder      = stringEnv("MEILI_HYBRID_EMBEDDER", "")
	MeiliHybridSemanticRatio = floatEnv("MEILI_HYBRID_SEMANTIC_RATIO", 0.5)
	// MeiliSearchCutoffMs bounds Meilisearch processing time per query at the
	// engine level. When a query exceeds this budget Meilisearch returns the
	// hits it has accumulated so far and marks estimatedTotalHits as a lower
	// bound — strictly better than letting hybrid embedder calls run unbounded
	// against the 10s Connect-RPC section timeout. Zero disables the cap.
	MeiliSearchCutoffMs = intEnv("MEILI_SEARCH_CUTOFF_MS", 1500)
	// MeiliSearchCacheSize caps the in-memory LRU that absorbs repeat search
	// queries. Each entry is bounded by max limit (100 hits) so 1024 entries
	// stays well under 50MB even with cropped content. Zero disables the
	// cache (useful for relevance-eval reruns).
	MeiliSearchCacheSize = intEnv("MEILI_SEARCH_CACHE_SIZE", 1024)
	// MeiliSearchCacheTTL bounds the staleness window for cached results.
	// Keep this in the minute range so newly-indexed articles still surface
	// in repeat queries within a reasonable window without forcing per-write
	// cache flushes.
	MeiliSearchCacheTTL = durationEnv("MEILI_SEARCH_CACHE_TTL", 5*time.Minute)
)

func floatEnv(key string, defaultVal float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func stringEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func intEnv(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}

func durationEnv(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultVal
}
