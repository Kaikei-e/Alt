package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Wire-form contract: ArticleCreatedPayload marshals with the canonical
// "url" key, never the legacy "link" key. This guards against the class
// of bug PM-2026-041 found — multiple producers re-declaring local payload
// shapes with the wrong tag, leaving the projection silently empty.
//
// Asserts on the JSON bytes directly rather than round-tripping through the
// same struct, because consumer-struct round-trip cannot detect a tag drift
// against itself (PM-2026-041 false-confidence pattern).
func TestArticleCreatedPayload_MarshalsCanonicalUrlKey(t *testing.T) {
	t.Parallel()

	p := ArticleCreatedPayload{
		ArticleID:   "11111111-1111-4111-8111-111111111111",
		Title:       "Hello",
		PublishedAt: "2026-04-28T12:00:00Z",
		TenantID:    "22222222-2222-4222-8222-222222222222",
		URL:         "https://example.com/x",
	}

	raw, err := json.Marshal(p)
	require.NoError(t, err)

	var asMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &asMap))
	assert.Contains(t, asMap, "url", "canonical wire key is `url`")
	assert.NotContains(t, asMap, "link", "legacy `link` key must not appear in marshalled bytes")
	assert.Equal(t, "https://example.com/x", asMap["url"])
}

// Wire-form contract: ArticleUrlBackfilledPayload is the corrective event
// emitted to repair historical ArticleCreated events whose payload was
// written with the legacy "link" key (or no key at all). It MUST use the
// canonical "url" key so projector replay and reproject converge.
func TestArticleUrlBackfilledPayload_MarshalsCanonicalUrlKey(t *testing.T) {
	t.Parallel()

	p := ArticleUrlBackfilledPayload{
		ArticleID: "11111111-1111-4111-8111-111111111111",
		URL:       "https://example.com/x",
	}

	raw, err := json.Marshal(p)
	require.NoError(t, err)

	var asMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &asMap))
	assert.Equal(t, []string{"article_id", "url"}, sortedKeys(asMap),
		"corrective payload carries only article_id + url; no other fields permitted")
	assert.Equal(t, "https://example.com/x", asMap["url"])
}

// Round-trip: a producer-emitted bytestream with "url" can be read back
// into the consumer struct. Pinned so a future tag rename on either side
// fails this test loudly instead of silently zeroing the URL field.
func TestArticleCreatedPayload_RoundTripPreservesURL(t *testing.T) {
	t.Parallel()

	wire := []byte(`{"article_id":"a","title":"t","published_at":"","tenant_id":"u","url":"https://x"}`)
	var p ArticleCreatedPayload
	require.NoError(t, json.Unmarshal(wire, &p))
	assert.Equal(t, "https://x", p.URL)
}

// Defense-in-depth: payloads written with the legacy "link" key (12,252
// historical events as of 2026-04-28) deliberately do NOT round-trip
// into ArticleCreatedPayload.URL. This is the explicit semantic that
// drove the corrective ArticleUrlBackfilled event design — recovery
// happens via append-first new events, NOT via consumer-side dual-key
// fallback.
func TestArticleCreatedPayload_LegacyLinkKeyDoesNotPopulateURL(t *testing.T) {
	t.Parallel()

	wire := []byte(`{"article_id":"a","title":"t","published_at":"","tenant_id":"u","link":"https://x"}`)
	var p ArticleCreatedPayload
	require.NoError(t, json.Unmarshal(wire, &p))
	assert.Empty(t, p.URL,
		"legacy `link` key MUST NOT silently fall back into URL — recovery happens via ArticleUrlBackfilled corrective event")
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Manual insertion sort to keep the test self-contained without sort import.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}
