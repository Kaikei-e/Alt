package knowledge_loop_projector

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

func makeEnrichEvent(t *testing.T, eventType string, seq int64, payload map[string]any) sovereign_db.KnowledgeEvent {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	userID := uuid.New()
	return sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      seq,
		OccurredAt:    time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		UserID:        &userID,
		EventType:     eventType,
		AggregateType: "article",
		AggregateID:   "article:42",
		Payload:       body,
	}
}

func TestEnrichWhyFromEvent_SummaryVersionCreated_SourceWhyWithTitle(t *testing.T) {
	ev := makeEnrichEvent(t, EventSummaryVersionCreated, 100, map[string]any{
		"summary_version_id": "sv-123",
		"article_id":         "article:42",
		"article_title":      "How Event Sourcing Changes Everything",
	})
	why := EnrichWhyFromEvent(&ev)

	require.Equal(t, sovereignv1.WhyKind_WHY_KIND_SOURCE, why.Kind)
	require.NotEmpty(t, why.Text)
	require.LessOrEqual(t, len(why.Text), 512)
	require.Contains(t, why.Text, "How Event Sourcing Changes Everything",
		"Article title must appear in why_text so the user gets real context")
	require.Contains(t, strings.ToLower(why.Text), "summary")

	require.Len(t, why.EvidenceRefs, 2)
	require.Equal(t, "sv-123", why.EvidenceRefs[0].RefId)
	require.Equal(t, "article:42", why.EvidenceRefs[1].RefId)
}

func TestEnrichWhyFromEvent_SummaryVersionCreated_FallbackWithoutTitle(t *testing.T) {
	ev := makeEnrichEvent(t, EventSummaryVersionCreated, 101, map[string]any{
		"summary_version_id": "sv-456",
	})
	why := EnrichWhyFromEvent(&ev)
	require.Equal(t, sovereignv1.WhyKind_WHY_KIND_SOURCE, why.Kind)
	require.NotEmpty(t, why.Text)
	require.NotContains(t, why.Text, "New summary",
		"the placeholder string must not survive the rewrite")
}

func TestEnrichWhyFromEvent_HomeItemAsked_KeepsConversationRef(t *testing.T) {
	ev := makeEnrichEvent(t, EventHomeItemAsked, 102, map[string]any{
		"conversation_id": "conv-xyz",
		"article_id":      "article:42",
	})
	why := EnrichWhyFromEvent(&ev)
	require.Equal(t, sovereignv1.WhyKind_WHY_KIND_SOURCE, why.Kind)
	require.NotEmpty(t, why.Text)
	var hasConv bool
	for _, r := range why.EvidenceRefs {
		if r.RefId == "conv-xyz" {
			hasConv = true
			break
		}
	}
	require.True(t, hasConv, "ask evidence must carry conversation_id for deep-link")
}

func TestEnrichWhyFromEvent_HomeItemSuperseded_ChangeWhyKeepsBothVersions(t *testing.T) {
	ev := makeEnrichEvent(t, EventHomeItemSuperseded, 103, map[string]any{
		"entry_key":                "article:42",
		"new_entry_key":            "article:43",
		"previous_summary_version": "sv-old",
		"summary_version_id":       "sv-new",
	})
	why := EnrichWhyFromEvent(&ev)
	require.Equal(t, sovereignv1.WhyKind_WHY_KIND_CHANGE, why.Kind)
	require.Contains(t, strings.ToLower(why.Text), "version")
	refs := make(map[string]bool, len(why.EvidenceRefs))
	for _, r := range why.EvidenceRefs {
		refs[r.RefId] = true
	}
	require.True(t, refs["sv-old"])
	require.True(t, refs["sv-new"])
}

func TestEnrichWhyFromEvent_HomeItemOpened_RecallWhy(t *testing.T) {
	ev := makeEnrichEvent(t, EventHomeItemOpened, 104, map[string]any{
		"entry_key":  "article:42",
		"article_id": "article:42",
	})
	why := EnrichWhyFromEvent(&ev)
	require.Equal(t, sovereignv1.WhyKind_WHY_KIND_RECALL, why.Kind)
	require.NotEmpty(t, why.Text)
	var hasEventRef bool
	for _, r := range why.EvidenceRefs {
		if r.RefId == ev.EventID.String() {
			hasEventRef = true
			break
		}
	}
	require.True(t, hasEventRef, "open recall evidence must reference the open event id")
}

// TestEvidenceRef_LabelDoesNotLeakKindStrings pins the 2026-05-27 regression
// where EvidenceRef.Label held the projector's internal kind discriminator
// ("summary", "article", "tags", ...) instead of a human-readable string.
// The downstream FE rendered the kind string next to the raw refID UUID, which
// looked like "8ddee42d-... — summary" to the user. Label must be either a
// human-readable excerpt/title (when payload carries one) or empty — never one
// of the discriminator strings the projector uses internally.
func TestEvidenceRef_LabelDoesNotLeakKindStrings(t *testing.T) {
	forbidden := map[string]struct{}{
		"summary":          {},
		"previous_summary": {},
		"new_summary":      {},
		"what_changed":     {},
		"article":          {},
		"tags":             {},
		"conversation":     {},
		"open_event":       {},
		"dismiss_event":    {},
		"entry":            {},
		"previous_entry":   {},
		"new_entry":        {},
	}

	cases := []struct {
		name      string
		eventType string
		payload   map[string]any
	}{
		{
			name:      "SummaryVersionCreated",
			eventType: EventSummaryVersionCreated,
			payload: map[string]any{
				"summary_version_id": "sv-A",
				"article_id":         "article:A",
				"tag_set_version_id": "tsv-A",
				"article_title":      "Why SDD Breaks Down in Microservices",
			},
		},
		{
			name:      "HomeItemsSeen",
			eventType: EventHomeItemsSeen,
			payload: map[string]any{
				"summary_version_id": "sv-B",
				"tag_set_version_id": "tsv-B",
				"article_id":         "article:B",
			},
		},
		{
			name:      "HomeItemAsked",
			eventType: EventHomeItemAsked,
			payload: map[string]any{
				"conversation_id": "conv-C",
				"article_id":      "article:C",
			},
		},
		{
			name:      "HomeItemOpened",
			eventType: EventHomeItemOpened,
			payload: map[string]any{
				"article_id": "article:D",
				"entry_key":  "article:D",
			},
		},
		{
			name:      "HomeItemSuperseded",
			eventType: EventHomeItemSuperseded,
			payload: map[string]any{
				"entry_key":                "article:E",
				"new_entry_key":            "article:E2",
				"previous_summary_version": "sv-E-old",
				"summary_version_id":       "sv-E-new",
			},
		},
		{
			name:      "HomeItemDismissed",
			eventType: EventHomeItemDismissed,
			payload: map[string]any{
				"entry_key": "article:F",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ev := makeEnrichEvent(t, tc.eventType, 200, tc.payload)
			why := EnrichWhyFromEvent(&ev)
			for _, ref := range why.EvidenceRefs {
				if _, bad := forbidden[ref.Label]; bad {
					t.Fatalf("EvidenceRef.Label leaked kind discriminator %q (refID=%q kind=%v)", ref.Label, ref.RefId, ref.Kind)
				}
			}
		})
	}
}

// TestEvidenceRef_LabelHydratedFromArticleTitle confirms that when the event
// payload carries article_title, the ARTICLE-kind evidence ref's Label is the
// title (truncated and sanitized), not empty. This is the human-readable path
// the FE renders next to "Article ·".
func TestEvidenceRef_LabelHydratedFromArticleTitle(t *testing.T) {
	ev := makeEnrichEvent(t, EventSummaryVersionCreated, 201, map[string]any{
		"summary_version_id": "sv-T",
		"article_id":         "article:T",
		"article_title":      "Why SDD Breaks Down in Microservices",
	})
	why := EnrichWhyFromEvent(&ev)

	var articleRef *sovereignv1.KnowledgeLoopEvidenceRef
	for _, r := range why.EvidenceRefs {
		if r.Kind == sovereignv1.EvidenceKind_EVIDENCE_KIND_ARTICLE {
			articleRef = r
			break
		}
	}
	require.NotNil(t, articleRef, "expected an ARTICLE-kind evidence ref")
	require.NotEmpty(t, articleRef.Label, "Label must be the article title, not empty")
	require.Contains(t, articleRef.Label, "Why SDD Breaks Down")
}

func TestEnrichWhyFromEvent_UnknownEventFallsBack(t *testing.T) {
	ev := makeEnrichEvent(t, "UnknownEventType", 105, map[string]any{"entry_key": "article:42"})
	why := EnrichWhyFromEvent(&ev)
	require.Equal(t, sovereignv1.WhyKind_WHY_KIND_SOURCE, why.Kind)
	require.NotEmpty(t, why.Text)
}

func TestEnrichWhyFromEvent_TextIsPlainAndBounded(t *testing.T) {
	ev := makeEnrichEvent(t, EventSummaryVersionCreated, 106, map[string]any{
		"article_title": "<script>alert(1)</script>" + strings.Repeat("x", 600),
	})
	why := EnrichWhyFromEvent(&ev)
	require.GreaterOrEqual(t, len(why.Text), 1)
	require.LessOrEqual(t, len(why.Text), 512)
	require.NotContains(t, why.Text, "<script")
}

func TestEnrichWhyFromEvent_EvidenceRefsAreBoundedToEight(t *testing.T) {
	ev := makeEnrichEvent(t, EventSummaryVersionCreated, 107, map[string]any{
		"summary_version_id": "sv-1",
		"tag_set_version_id": "tsv-1",
		"lens_version_id":    "lv-1",
		"article_id":         "article:42",
		"article_title":      "Some title",
	})
	why := EnrichWhyFromEvent(&ev)
	require.LessOrEqual(t, len(why.EvidenceRefs), 8)
}

func TestWhyMappingVersion_BumpedForOwnershipMove(t *testing.T) {
	require.GreaterOrEqual(t, WhyMappingVersion, 4,
		"WhyMappingVersion must signal the knowledge-sovereign ownership move so the runbook triggers reproject")
}

// WhyPayload v2 producer wiring (ADR-000908 §Δ4). Once the enricher integrates
// the pure helpers (boundCounterEvidence, confidenceLadderFromKind,
// whatWouldChangeFromKind), every emitted WhyPayload must carry the v2 fields
// so the wire format and the UI can rely on their presence.

func TestEnrichWhyFromEvent_SummaryVersionCreated_PopulatesV2Fields(t *testing.T) {
	ev := makeEnrichEvent(t, EventSummaryVersionCreated, 200, map[string]any{
		"summary_version_id": "sv-200",
		"article_id":         "article:200",
		"article_title":      "Article 200",
	})
	why := EnrichWhyFromEvent(&ev)
	require.NotNil(t, why.ConfidenceLadder,
		"confidence_ladder must be populated for every enriched WhyPayload")
	require.Equal(t, sovereignv1.ConfidenceLadder_CONFIDENCE_LADDER_SPECULATION, *why.ConfidenceLadder)
	require.NotNil(t, why.WhatWouldChangeMyMind,
		"what_would_change_my_mind must be populated for SOURCE kind")
	require.NotEmpty(t, *why.WhatWouldChangeMyMind)
}

func TestEnrichWhyFromEvent_HomeItemSuperseded_PopulatesCounterEvidence(t *testing.T) {
	ev := makeEnrichEvent(t, EventHomeItemSuperseded, 201, map[string]any{
		"previous_summary_version": "sv-prev",
		"summary_version_id":       "sv-new",
		"entry_key":                "entry-old",
		"new_entry_key":            "entry-new",
		"article_title":            "Updated article",
	})
	why := EnrichWhyFromEvent(&ev)
	require.Equal(t, sovereignv1.WhyKind_WHY_KIND_CHANGE, why.Kind)
	require.NotNil(t, why.ConfidenceLadder)
	require.Equal(t, sovereignv1.ConfidenceLadder_CONFIDENCE_LADDER_EVIDENCE, *why.ConfidenceLadder,
		"CHANGE kind must map to EVIDENCE ladder per canonical contract §11")
	require.NotEmpty(t, why.CounterEvidenceRefs,
		"superseded entries must surface the previous version as counter-evidence")
	require.Equal(t, "sv-prev", why.CounterEvidenceRefs[0].RefId)
	require.Equal(t, sovereignv1.EvidenceKind_EVIDENCE_KIND_SUMMARY, why.CounterEvidenceRefs[0].Kind,
		"counter-evidence on a superseded summary must route as SUMMARY")
	require.NotNil(t, why.WhatWouldChangeMyMind)
	require.Contains(t, *why.WhatWouldChangeMyMind, "newer")
}

func TestEnrichWhyFromEvent_HomeItemOpened_RecallLadderIsSpeculation(t *testing.T) {
	ev := makeEnrichEvent(t, EventHomeItemOpened, 202, map[string]any{
		"article_id": "article:202",
	})
	why := EnrichWhyFromEvent(&ev)
	require.Equal(t, sovereignv1.WhyKind_WHY_KIND_RECALL, why.Kind)
	require.NotNil(t, why.ConfidenceLadder)
	require.Equal(t, sovereignv1.ConfidenceLadder_CONFIDENCE_LADDER_SPECULATION, *why.ConfidenceLadder)
}

func TestWhyMappingVersion_IsAtLeastEleven(t *testing.T) {
	// WhyMappingVersion >= 11 means WhyPayload v2 producer wiring has landed
	// (ADR-000908 §Δ4). Later bumps cover review_reason / persist_stage etc.
	// — we keep this assertion permissive so a future bump does not need a
	// duplicate update here.
	require.GreaterOrEqual(t, WhyMappingVersion, 11,
		"WhyMappingVersion must be ≥ 11 once WhyPayload v2 producer wiring lands")
}
