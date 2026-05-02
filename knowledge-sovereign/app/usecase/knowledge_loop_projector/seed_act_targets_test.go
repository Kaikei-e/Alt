package knowledge_loop_projector

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

// seedActTargets materialises the JSON bytes the projector writes into
// knowledge_loop_entries.act_targets. Downstream chain:
//
//	JSONB column → alt-backend decodeActTargets → loopv1.ActTarget
//	                                            → FE mapActTargetTypeFromProto
//	                                            → "recap" string in actTargets[]
//
// The entire chain is deterministic, so testing the JSON shape end-to-end is
// sufficient — we don't need to spin up a DB to verify the projector's
// behaviour.

func TestSeedActTargets_NoRecapID_ReturnsNil(t *testing.T) {
	t.Parallel()
	out := seedActTargets(nil, SurfaceScoreInputs{})
	require.Nil(t, out, "no act_targets should be seeded when no recap snapshot resolved")
}

func TestSeedActTargets_RecapID_WritesRecapTarget(t *testing.T) {
	t.Parallel()
	out := seedActTargets(nil, SurfaceScoreInputs{
		RecapTopicSnapshotID: "11111111-1111-4111-8111-111111111111",
	})
	require.NotEmpty(t, out)

	var raw []map[string]string
	require.NoError(t, json.Unmarshal(out, &raw))
	require.Len(t, raw, 1)
	require.Equal(t, "recap", raw[0]["target_type"])
	require.Equal(t, "11111111-1111-4111-8111-111111111111", raw[0]["target_ref"])
	require.Equal(t, "/recap/topic/11111111-1111-4111-8111-111111111111", raw[0]["route"])
}

// TestSeedActTargets_RouteIsAbsolutePath documents the contract the FE relies
// on: the route MUST start with "/" and MUST NOT contain ":". Together with
// the FE-side allowlist this prevents javascript: schemes / open-redirect
// vectors even if a future resolver leaks an attacker-controlled string into
// RecapTopicSnapshotID. The resolver itself UUID-validates the input; this
// test is the projector's belt-and-suspenders.
func TestSeedActTargets_RouteIsAbsolutePath(t *testing.T) {
	t.Parallel()
	out := seedActTargets(nil, SurfaceScoreInputs{
		RecapTopicSnapshotID: "22222222-2222-4222-8222-222222222222",
	})
	var raw []map[string]string
	require.NoError(t, json.Unmarshal(out, &raw))
	require.Len(t, raw, 1)
	route := raw[0]["route"]
	require.True(t, strings.HasPrefix(route, "/"), "route must be a server-relative path")
	require.False(t, strings.Contains(route, ":"), "route must not contain a scheme separator")
}

func TestSeedActTargets_ArticleEvent_WritesArticleTarget(t *testing.T) {
	t.Parallel()
	userID := uuid.New()
	ev := &sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      1,
		OccurredAt:    time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		UserID:        &userID,
		AggregateType: "article",
		AggregateID:   "art-article-target",
		Payload:       json.RawMessage(`{"article_id":"art-article-target"}`),
	}

	out := seedActTargets(ev, SurfaceScoreInputs{})

	var raw []map[string]string
	require.NoError(t, json.Unmarshal(out, &raw))
	require.Len(t, raw, 1)
	require.Equal(t, "article", raw[0]["target_type"])
	require.Equal(t, "art-article-target", raw[0]["target_ref"])
	require.Equal(t, "/articles/art-article-target", raw[0]["route"])
}

// TestSeedActTargets_ArticleEvent_WithURL_WritesSourceURL pins the contract
// the FE relies on: when the event payload carries the article's external
// HTTPS source URL, the projector copies it into act_targets[].source_url so
// the SPA reader can use it as `?url=` (route stays the internal SPA path).
//
// Reproject-safe: the URL is read from the event payload only, never from
// the latest article state.
func TestSeedActTargets_ArticleEvent_WithURL_WritesSourceURL(t *testing.T) {
	t.Parallel()
	userID := uuid.New()
	ev := &sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      2,
		OccurredAt:    time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		UserID:        &userID,
		AggregateType: "article",
		AggregateID:   "art-with-url",
		Payload:       json.RawMessage(`{"article_id":"art-with-url","url":"https://example.com/post"}`),
	}

	out := seedActTargets(ev, SurfaceScoreInputs{})

	var raw []map[string]string
	require.NoError(t, json.Unmarshal(out, &raw))
	require.Len(t, raw, 1)
	require.Equal(t, "article", raw[0]["target_type"])
	require.Equal(t, "art-with-url", raw[0]["target_ref"])
	require.Equal(t, "/articles/art-with-url", raw[0]["route"])
	require.Equal(t, "https://example.com/post", raw[0]["source_url"])
}

// TestSeedActTargets_ArticleEvent_NoURL_OmitsSourceURL — payload without a
// URL must produce an act_target whose source_url key is absent (not present
// as empty string), so legacy events round-trip cleanly through the
// `omitempty` JSON tag.
func TestSeedActTargets_ArticleEvent_NoURL_OmitsSourceURL(t *testing.T) {
	t.Parallel()
	userID := uuid.New()
	ev := &sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      3,
		OccurredAt:    time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		UserID:        &userID,
		AggregateType: "article",
		AggregateID:   "art-no-url",
		Payload:       json.RawMessage(`{"article_id":"art-no-url"}`),
	}

	out := seedActTargets(ev, SurfaceScoreInputs{})

	var raw []map[string]any
	require.NoError(t, json.Unmarshal(out, &raw))
	require.Len(t, raw, 1)
	require.Equal(t, "article", raw[0]["target_type"])
	require.Equal(t, "/articles/art-no-url", raw[0]["route"])
	_, hasSourceURL := raw[0]["source_url"]
	require.False(t, hasSourceURL, "source_url key must be absent for legacy payloads")
}

// TestSeedActTargets_ArticleEvent_LegacyLinkKey_FallsBack — wire compatibility
// with legacy events that used `"link"` instead of `"url"` (the postmortem
// PM-2026-041 left a small population of historical events with this key).
func TestSeedActTargets_ArticleEvent_LegacyLinkKey_FallsBack(t *testing.T) {
	t.Parallel()
	userID := uuid.New()
	ev := &sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      4,
		OccurredAt:    time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		UserID:        &userID,
		AggregateType: "article",
		AggregateID:   "art-legacy-link",
		Payload:       json.RawMessage(`{"article_id":"art-legacy-link","link":"https://example.com/legacy"}`),
	}

	out := seedActTargets(ev, SurfaceScoreInputs{})

	var raw []map[string]string
	require.NoError(t, json.Unmarshal(out, &raw))
	require.Len(t, raw, 1)
	require.Equal(t, "https://example.com/legacy", raw[0]["source_url"])
}

// TestSeedActTargets_ArticleEvent_NonHTTPURL_Rejected — defense-in-depth
// against a `javascript:` (or other non-HTTP) scheme leaking from a malformed
// event payload into the projection. The FE's safeArticleHref provides a
// second layer; the projector must not be the weak link.
func TestSeedActTargets_ArticleEvent_NonHTTPURL_Rejected(t *testing.T) {
	t.Parallel()
	userID := uuid.New()
	for _, badURL := range []string{
		"javascript:alert(1)",
		"data:text/html;base64,PHNjcmlwdD4=",
		"file:///etc/passwd",
		"not-a-url-at-all",
	} {
		t.Run(badURL, func(t *testing.T) {
			t.Parallel()
			payload := []byte(`{"article_id":"art-bad","url":` + mustJSONString(badURL) + `}`)
			ev := &sovereign_db.KnowledgeEvent{
				EventID:       uuid.New(),
				EventSeq:      5,
				OccurredAt:    time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
				TenantID:      uuid.New(),
				UserID:        &userID,
				AggregateType: "article",
				AggregateID:   "art-bad",
				Payload:       json.RawMessage(payload),
			}

			out := seedActTargets(ev, SurfaceScoreInputs{})

			var raw []map[string]any
			require.NoError(t, json.Unmarshal(out, &raw))
			require.Len(t, raw, 1)
			_, hasSourceURL := raw[0]["source_url"]
			require.False(t, hasSourceURL, "non-http(s) URL %q must not leak into source_url", badURL)
		})
	}
}

// mustJSONString returns a JSON-encoded string literal for embedding inside
// a hand-written JSON template. Used to keep the table-driven test readable
// without escaping every embedded value by hand.
func mustJSONString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
