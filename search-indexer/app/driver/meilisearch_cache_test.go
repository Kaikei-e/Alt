package driver

import (
	"testing"
	"time"
)

func TestSearchCache_MissThenHit(t *testing.T) {
	cache, err := newSearchCache(8, 5*time.Minute)
	if err != nil {
		t.Fatalf("newSearchCache: %v", err)
	}

	key := cacheKey{Query: "rust", UserID: "u1", Limit: 5}
	if _, ok := cache.get(key); ok {
		t.Fatalf("unexpected hit on empty cache")
	}

	entry := cacheEntry{
		Docs:           []SearchDocumentDriver{{ID: "1", Title: "ok"}},
		EstimatedTotal: 1,
		ProcessingMs:   42,
	}
	cache.put(key, entry)

	got, ok := cache.get(key)
	if !ok {
		t.Fatalf("expected hit after put")
	}
	if len(got.Docs) != 1 || got.Docs[0].ID != "1" {
		t.Errorf("got Docs=%v, want [{ID:1}]", got.Docs)
	}
	if got.EstimatedTotal != 1 || got.ProcessingMs != 42 {
		t.Errorf("got EstimatedTotal=%d ProcessingMs=%d, want 1/42", got.EstimatedTotal, got.ProcessingMs)
	}
}

func TestSearchCache_TTL_Expires(t *testing.T) {
	cache, err := newSearchCache(8, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("newSearchCache: %v", err)
	}

	key := cacheKey{Query: "rust", UserID: "u1", Limit: 5}
	cache.put(key, cacheEntry{EstimatedTotal: 1})

	time.Sleep(25 * time.Millisecond)
	if _, ok := cache.get(key); ok {
		t.Fatalf("expected expired entry to miss")
	}
}

func TestSearchCache_TenantIsolation(t *testing.T) {
	cache, err := newSearchCache(8, 5*time.Minute)
	if err != nil {
		t.Fatalf("newSearchCache: %v", err)
	}

	keyA := cacheKey{Query: "rust", UserID: "tenant-a", Limit: 5}
	keyB := cacheKey{Query: "rust", UserID: "tenant-b", Limit: 5}

	cache.put(keyA, cacheEntry{Docs: []SearchDocumentDriver{{ID: "doc-a"}}})

	if _, ok := cache.get(keyB); ok {
		t.Fatalf("tenant B must not see tenant A entry")
	}
}

func TestSearchCache_HybridConfigSeparatesKeys(t *testing.T) {
	cache, err := newSearchCache(8, 5*time.Minute)
	if err != nil {
		t.Fatalf("newSearchCache: %v", err)
	}

	bm25 := cacheKey{Query: "rust", UserID: "u1", Limit: 5, Embedder: "", SemanticRatio: 0}
	hybrid := cacheKey{Query: "rust", UserID: "u1", Limit: 5, Embedder: "qwen3", SemanticRatio: 0.7}

	cache.put(bm25, cacheEntry{EstimatedTotal: 10})

	if _, ok := cache.get(hybrid); ok {
		t.Fatalf("hybrid query must not share cache with BM25")
	}
}

func TestNormalizeCacheKeyQuery_Idempotent_AndLowercasesLatin(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"UAE", "uae"},
		{"  Rust   Lang  ", "rust lang"},
		{"日本語", "日本語"}, // CJK unchanged
		{"Tokyo日本", "tokyo日本"},
	}
	for _, c := range cases {
		got := normalizeCacheKeyQuery(c.in)
		if got != c.want {
			t.Errorf("normalizeCacheKeyQuery(%q) = %q, want %q", c.in, got, c.want)
		}
		// idempotence
		if g2 := normalizeCacheKeyQuery(got); g2 != got {
			t.Errorf("not idempotent: normalize(%q) = %q vs %q", got, g2, got)
		}
	}
}
