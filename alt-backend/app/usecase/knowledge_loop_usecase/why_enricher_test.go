package knowledge_loop_usecase

import (
	"alt/domain"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Helper: build a Knowledge event with a structured payload.
func makeEnrichEvent(t *testing.T, eventType string, seq int64, payload map[string]any) domain.KnowledgeEvent {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	userID := uuid.New()
	return domain.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      seq,
		OccurredAt:    time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		UserID:        &userID,
		EventType:     eventType,
		AggregateType: "article",
		AggregateID:   "article:42",
		Payload:       body,
	}
}

func TestEnrichWhyFromEvent_SummaryVersionCreated_SourceWhy(t *testing.T) {
	ev := makeEnrichEvent(t, domain.EventSummaryVersionCreated, 100, map[string]any{
		"summary_version_id": "sv-123",
		"article_id":         "article:42",
		"article_title":      "How Event Sourcing Changes Everything",
	})
	why := EnrichWhyFromEvent(&ev)

	require.Equal(t, domain.WhyKindSource, why.Kind)
	require.NotEmpty(t, why.Text)
	require.LessOrEqual(t, len(why.Text), 512)
	require.Contains(t, strings.ToLower(why.Text), "summary")

	// evidence_refs include both the summary version and the article, in that order
	require.Len(t, why.EvidenceRefs, 2)
	require.Equal(t, "sv-123", why.EvidenceRefs[0].RefID)
	require.Equal(t, "article:42", why.EvidenceRefs[1].RefID)
}

func TestEnrichWhyFromEvent_HomeItemAsked_SourceWhyWithConversation(t *testing.T) {
	ev := makeEnrichEvent(t, domain.EventHomeItemAsked, 101, map[string]any{
		"conversation_id": "conv-xyz",
		"article_id":      "article:42",
	})
	why := EnrichWhyFromEvent(&ev)

	require.Equal(t, domain.WhyKindSource, why.Kind)
	// Text should mention the action, in plain words.
	require.NotEmpty(t, why.Text)
	// The conversation ref must surface as evidence so the UI can deep-link back.
	var hasConv bool
	for _, r := range why.EvidenceRefs {
		if r.RefID == "conv-xyz" {
			hasConv = true
			break
		}
	}
	require.True(t, hasConv, "ask evidence must carry conversation_id for deep-link")
}

func TestEnrichWhyFromEvent_HomeItemSuperseded_ChangeWhyKeepsBothVersions(t *testing.T) {
	ev := makeEnrichEvent(t, domain.EventHomeItemSuperseded, 102, map[string]any{
		"entry_key":                "article:42",
		"new_entry_key":            "article:43",
		"previous_summary_version": "sv-old",
		"summary_version_id":       "sv-new",
	})
	why := EnrichWhyFromEvent(&ev)

	require.Equal(t, domain.WhyKindChange, why.Kind)
	require.Contains(t, strings.ToLower(why.Text), "version")
	// Both old and new summary versions must be in evidence_refs so the UI can
	// render a diff badge without fetching additional state.
	refs := make(map[string]bool, len(why.EvidenceRefs))
	for _, r := range why.EvidenceRefs {
		refs[r.RefID] = true
	}
	require.True(t, refs["sv-old"])
	require.True(t, refs["sv-new"])
}

func TestEnrichWhyFromEvent_HomeItemOpened_RecallWhy(t *testing.T) {
	ev := makeEnrichEvent(t, domain.EventHomeItemOpened, 103, map[string]any{
		"entry_key":  "article:42",
		"article_id": "article:42",
	})
	why := EnrichWhyFromEvent(&ev)

	require.Equal(t, domain.WhyKindRecall, why.Kind)
	require.NotEmpty(t, why.Text)
	// Open events identify the triggering event itself so the UI can anchor
	// "you opened this before" back to a specific event row.
	var hasEventRef bool
	for _, r := range why.EvidenceRefs {
		if r.RefID == ev.EventID.String() {
			hasEventRef = true
			break
		}
	}
	require.True(t, hasEventRef, "open recall evidence must reference the open event id")
}

func TestEnrichWhyFromEvent_UnknownEventFallsBack(t *testing.T) {
	ev := makeEnrichEvent(t, "UnknownEventType", 104, map[string]any{"entry_key": "article:42"})
	why := EnrichWhyFromEvent(&ev)

	// Unknown events fall back to a generic source why so the entry is still
	// renderable; the projector never produces an empty why_text.
	require.Equal(t, domain.WhyKindSource, why.Kind)
	require.NotEmpty(t, why.Text)
}

func TestEnrichWhyFromEvent_EvidenceRefsAreBoundedToEight(t *testing.T) {
	// Synthetic payload with many version-like fields. Even if future payloads
	// become richer, the enricher must cap evidence_refs at 8 (canonical
	// contract §3.10).
	ev := makeEnrichEvent(t, domain.EventSummaryVersionCreated, 105, map[string]any{
		"summary_version_id": "sv-1",
		"tag_set_version_id": "tsv-1",
		"lens_version_id":    "lv-1",
		"article_id":         "article:42",
		"article_title":      "A very long title that should be truncated to 512 chars in text but refs are strict",
	})
	why := EnrichWhyFromEvent(&ev)
	require.LessOrEqual(t, len(why.EvidenceRefs), 8)
}

func TestEnrichWhyFromEvent_TextIsPlainAndBounded(t *testing.T) {
	// Craft a title with HTML-like garbage to ensure the enricher does not
	// copy it verbatim (canonical contract §3.10: why text is plain, 1..512).
	ev := makeEnrichEvent(t, domain.EventSummaryVersionCreated, 106, map[string]any{
		"article_title": "<script>alert(1)</script>" + strings.Repeat("x", 600),
	})
	why := EnrichWhyFromEvent(&ev)
	require.GreaterOrEqual(t, len(why.Text), 1)
	require.LessOrEqual(t, len(why.Text), 512)
	require.NotContains(t, why.Text, "<script")
}

func TestWhyMappingVersion_BumpedForEnrichment(t *testing.T) {
	// Bumping the version is the projector's signal that a full reproject is
	// required to refresh the why_text fields across existing entries.
	require.GreaterOrEqual(t, WhyMappingVersion, 2,
		"Why enrichment must bump WhyMappingVersion so runbook triggers reproject")
}
