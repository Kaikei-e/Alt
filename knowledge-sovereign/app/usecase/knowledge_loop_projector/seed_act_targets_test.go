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

// TestSeedActTargets_AugurEvent_InputsArticleID_PreservesArticleTarget pins
// the Ask-then-Open fix: when augur.conversation_linked.v1 lands, the event
// payload carries no `article_id` of its own — only `entry_key`. Without
// SurfaceScoreInputs.ArticleID the projector overwrites act_targets with an
// empty slice and the FE's `sourceUrl()` returns null, leaving Article and
// Summary clicks dead. The resolver must pin ArticleID from the entry's
// prior event chain so seedActTargets keeps the article target stable across
// the OODA lifecycle. Reproject-safe: derived from event payload only.
func TestSeedActTargets_AugurEvent_InputsArticleID_PreservesArticleTarget(t *testing.T) {
	t.Parallel()
	userID := uuid.New()
	ev := &sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      6,
		OccurredAt:    time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		UserID:        &userID,
		AggregateType: "augur_conversation",
		AggregateID:   "conv-1",
		Payload:       json.RawMessage(`{"entry_key":"entry:art-pinned","conversation_id":"conv-1"}`),
	}

	out := seedActTargets(ev, SurfaceScoreInputs{ArticleID: "art-pinned"})

	require.NotNil(t, out, "augur event with pinned ArticleID must seed an article target")
	var raw []map[string]string
	require.NoError(t, json.Unmarshal(out, &raw))
	require.Len(t, raw, 1)
	require.Equal(t, "article", raw[0]["target_type"])
	require.Equal(t, "art-pinned", raw[0]["target_ref"])
	require.Equal(t, "/articles/art-pinned", raw[0]["route"])
}

// TestSeedActTargets_InputsArticleID_OverridesEmptyEvent — without an
// ArticleID pin from the resolver the legacy path (event payload extraction)
// stays the only source, so a payload-empty event still produces no article
// target. The Inputs path is opt-in, not a silent rewriter of valid events.
func TestSeedActTargets_InputsArticleID_DoesNotOverrideExplicitArticleEvent(t *testing.T) {
	t.Parallel()
	userID := uuid.New()
	ev := &sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      7,
		OccurredAt:    time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		UserID:        &userID,
		AggregateType: "article",
		AggregateID:   "art-from-event",
		Payload:       json.RawMessage(`{"article_id":"art-from-event"}`),
	}

	out := seedActTargets(ev, SurfaceScoreInputs{ArticleID: "art-from-input-ignored"})

	var raw []map[string]string
	require.NoError(t, json.Unmarshal(out, &raw))
	require.Len(t, raw, 1)
	// Event-derived article_id wins over the input pin so reproject-safety is
	// not weakened: the event log remains the canonical source.
	require.Equal(t, "art-from-event", raw[0]["target_ref"])
}

// TestSeedActTargets_NonArticleEvent_InputsSourceURL_PreservesSourceURL pins
// the SourceURL fallback contract: when an augur.conversation_linked.v1 event
// (or any non-article event) triggers a re-seed and inputs.SourceURL carries
// the article URL from the resolver pin, seedActTargets must emit
// act_targets[0].source_url so the FE's resolveLoopSourceUrl stays non-null
// and "Open Article" remains a single-click action. Without this fallback the
// projector rewrites source_url to "" on every non-article event, which is
// exactly the systemic 8319-entry drop documented at 2026-05-26.
func TestSeedActTargets_NonArticleEvent_InputsSourceURL_PreservesSourceURL(t *testing.T) {
	t.Parallel()
	userID := uuid.New()
	ev := &sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      10,
		OccurredAt:    time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		UserID:        &userID,
		AggregateType: "augur_conversation",
		AggregateID:   "conv-1",
		Payload:       json.RawMessage(`{"entry_key":"entry:art-pinned","conversation_id":"conv-1"}`),
	}

	out := seedActTargets(ev, SurfaceScoreInputs{
		ArticleID: "art-pinned",
		SourceURL: "https://example.com/post",
	})

	var raw []map[string]string
	require.NoError(t, json.Unmarshal(out, &raw))
	require.Len(t, raw, 1)
	require.Equal(t, "article", raw[0]["target_type"])
	require.Equal(t, "art-pinned", raw[0]["target_ref"])
	require.Equal(t, "/articles/art-pinned", raw[0]["route"])
	require.Equal(t, "https://example.com/post", raw[0]["source_url"],
		"inputs.SourceURL must seed source_url when the event payload omits url")
}

// TestSeedActTargets_ArticleEventURL_Overrides_InputsSourceURL — the event-
// derived URL must win over the input pin so reproject-safety is preserved:
// the event log remains the canonical source of the URL when both are
// available. Mirrors the ArticleID priority semantics.
func TestSeedActTargets_ArticleEventURL_Overrides_InputsSourceURL(t *testing.T) {
	t.Parallel()
	userID := uuid.New()
	ev := &sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      11,
		OccurredAt:    time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		UserID:        &userID,
		AggregateType: "article",
		AggregateID:   "art-with-url",
		Payload:       json.RawMessage(`{"article_id":"art-with-url","url":"https://example.com/from-event"}`),
	}

	out := seedActTargets(ev, SurfaceScoreInputs{
		ArticleID: "art-ignored",
		SourceURL: "https://example.com/from-input-ignored",
	})

	var raw []map[string]string
	require.NoError(t, json.Unmarshal(out, &raw))
	require.Len(t, raw, 1)
	require.Equal(t, "https://example.com/from-event", raw[0]["source_url"],
		"event-derived URL must win over inputs.SourceURL")
}

// TestSeedActTargets_NonArticleEvent_NoInputsSourceURL_OmitsKey — when neither
// the event payload nor inputs.SourceURL provides a URL, the source_url JSON
// key must be absent (not present as ""), so legacy entries round-trip
// cleanly through the `omitempty` JSON tag.
func TestSeedActTargets_NonArticleEvent_NoInputsSourceURL_OmitsKey(t *testing.T) {
	t.Parallel()
	userID := uuid.New()
	ev := &sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      12,
		OccurredAt:    time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		UserID:        &userID,
		AggregateType: "augur_conversation",
		AggregateID:   "conv-1",
		Payload:       json.RawMessage(`{"entry_key":"entry:art-no-url","conversation_id":"conv-1"}`),
	}

	out := seedActTargets(ev, SurfaceScoreInputs{ArticleID: "art-no-url"})

	var raw []map[string]any
	require.NoError(t, json.Unmarshal(out, &raw))
	require.Len(t, raw, 1)
	require.Equal(t, "article", raw[0]["target_type"])
	require.Equal(t, "/articles/art-no-url", raw[0]["route"])
	_, hasSourceURL := raw[0]["source_url"]
	require.False(t, hasSourceURL, "source_url key must be absent when neither event nor inputs provides one")
}
