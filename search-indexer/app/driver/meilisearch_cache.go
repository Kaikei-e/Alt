// Package driver: meilisearch_cache.go adds a small in-memory LRU cache in
// front of the Meilisearch search path. The cache key includes the hybrid
// config snapshot ({embedder, semanticRatio}) so a config flip in env never
// serves stale results from a different ranking regime, and it includes the
// user_id so cross-tenant leakage is structurally impossible.
//
// Invalidation is TTL-only: IndexDocuments / DeleteDocuments do NOT flush the
// cache. The 5-minute default TTL trades freshness (recently indexed articles
// may take up to 5 minutes to appear in repeat queries) for hit rate. This is
// acceptable because Meilisearch's own indexing latency for hybrid embeddings
// is already in the minute range, and global search is interactive — repeat
// queries from the same user during scrolling / refinement get a hot path.
package driver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/text/unicode/norm"
)

// cacheKey uniquely identifies a Meilisearch search request. user_id is part
// of the key so tenant isolation is enforced structurally — a hit only happens
// when the same caller asks the same question. Hybrid params are included so a
// config change between request and lookup cannot serve stale ranking output.
type cacheKey struct {
	Query         string
	UserID        string
	Filter        string
	Offset        int64
	Limit         int64
	Embedder      string
	SemanticRatio float64
}

// String produces a stable identifier for singleflight grouping. Query and
// Filter are attacker/user-controlled and may contain any byte including a
// pipe, so a plain delimited join could collide two different requests
// (e.g. one user's query bleeding a pipe-separated UserID into another's)
// onto the same singleflight key and leak cross-tenant results. Hashing the
// whole key removes that structurally instead of relying on the inputs
// never containing the delimiter.
func (k cacheKey) String() string {
	h := sha256.Sum256(fmt.Appendf(nil, "%s|%s|%s|%d|%d|%s|%g",
		k.Query, k.UserID, k.Filter, k.Offset, k.Limit, k.Embedder, k.SemanticRatio))
	return hex.EncodeToString(h[:])
}

// cacheEntry stores both the result and the engine-reported processingMs so
// downstream layers can keep observability parity on cache hits.
type cacheEntry struct {
	Docs           []SearchDocumentDriver
	EstimatedTotal int64
	ProcessingMs   int64
	storedAt       time.Time
}

// searchCache is a thin wrapper around hashicorp/golang-lru/v2 that adds a
// soft TTL by stamping each entry with storedAt and treating older reads as
// misses. We keep the LRU eviction (size cap) untouched so memory stays
// bounded even when traffic exceeds the TTL window.
type searchCache struct {
	lru *lru.Cache[cacheKey, cacheEntry]
	ttl time.Duration
}

func newSearchCache(size int, ttl time.Duration) (*searchCache, error) {
	if size <= 0 {
		size = 1024
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	c, err := lru.New[cacheKey, cacheEntry](size)
	if err != nil {
		return nil, err
	}
	return &searchCache{lru: c, ttl: ttl}, nil
}

func (c *searchCache) get(k cacheKey) (cacheEntry, bool) {
	if c == nil || c.lru == nil {
		return cacheEntry{}, false
	}
	e, ok := c.lru.Get(k)
	if !ok {
		return cacheEntry{}, false
	}
	if time.Since(e.storedAt) > c.ttl {
		c.lru.Remove(k)
		return cacheEntry{}, false
	}
	return e, true
}

func (c *searchCache) put(k cacheKey, e cacheEntry) {
	if c == nil || c.lru == nil {
		return
	}
	e.storedAt = time.Now()
	c.lru.Add(k, e)
}

// normalizeCacheKeyQuery folds Unicode and case so trivially equivalent
// queries hit the same cache entry. The driver already receives queries that
// have passed the usecase-layer sanitizer (NFC + zero-width strip + whitespace
// fold), but this function is idempotent so it is safe to apply twice.
//
// strings.ToLower is intentional: Latin "UAE" / "uae" share a key. CJK and
// other non-cased scripts are unaffected by ToLower per Unicode spec.
func normalizeCacheKeyQuery(q string) string {
	q = norm.NFC.String(q)
	q = strings.TrimSpace(q)
	// collapse internal runs of whitespace into single spaces
	if strings.ContainsAny(q, " \t\n") {
		fields := strings.Fields(q)
		q = strings.Join(fields, " ")
	}
	return strings.ToLower(q)
}
