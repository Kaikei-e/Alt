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
