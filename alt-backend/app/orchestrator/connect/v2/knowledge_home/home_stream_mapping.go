package knowledge_home

import (
	"time"

	"github.com/google/uuid"

	"alt/domain"
	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
)

// coalesceStreamEvents deduplicates events by aggregate_id within a batch,
// keeping only the latest event for each aggregate. This reduces wire traffic
// when multiple updates arrive for the same item within a 5-second window.
func coalesceStreamEvents(events []domain.KnowledgeEvent) []domain.KnowledgeEvent {
	if len(events) <= 1 {
		return events
	}

	seen := make(map[string]int, len(events))
	result := make([]domain.KnowledgeEvent, 0, len(events))

	for _, event := range events {
		key := event.AggregateType + ":" + event.AggregateID
		if idx, exists := seen[key]; exists {
			// Replace with later event (higher seq)
			result[idx] = event
		} else {
			seen[key] = len(result)
			result = append(result, event)
		}
	}

	return result
}

// mapToCanonicalStreamType converts a domain event type to a canonical stream event type.
// See ADR-434 Phase 0 canonical contract for the mapping.
func mapToCanonicalStreamType(eventType string) string {
	switch eventType {
	case domain.EventArticleCreated:
		return "item_added"
	case domain.EventRecallSnoozed,
		domain.EventRecallDismissed:
		return "recall_changed"
	case domain.EventSummaryVersionCreated,
		domain.EventTagSetVersionCreated,
		domain.EventSummarySuperseded,
		domain.EventTagSetSuperseded,
		domain.EventHomeItemSuperseded,
		domain.EventReasonMerged:
		return "item_updated"
	default:
		// System/user interaction events trigger digest re-fetch only
		return "digest_changed"
	}
}

// buildStreamResponse creates a StreamKnowledgeHomeUpdatesResponse from a domain event.
// For item_added/item_updated, it includes a minimal KnowledgeHomeItem with item_key only.
// For digest_changed, no payload is included (frontend re-fetches via unary).
// For recall_changed, a minimal RecallChange is included and may be enriched later.
func buildStreamResponse(event domain.KnowledgeEvent) *knowledgehomev1.StreamKnowledgeHomeUpdatesResponse {
	canonicalType := mapToCanonicalStreamType(event.EventType)
	resp := &knowledgehomev1.StreamKnowledgeHomeUpdatesResponse{
		EventType:  canonicalType,
		OccurredAt: event.OccurredAt.Format(time.RFC3339),
	}
	switch canonicalType {
	case "item_added", "item_updated":
		itemKey := event.AggregateType + ":" + event.AggregateID
		resp.Item = &knowledgehomev1.KnowledgeHomeItem{ItemKey: itemKey}
	case "recall_changed":
		itemKey := event.AggregateType + ":" + event.AggregateID
		resp.RecallChange = &knowledgehomev1.RecallCandidate{ItemKey: itemKey}
	}
	return resp
}

func dropArticleAggregates(events []domain.KnowledgeEvent) []domain.KnowledgeEvent {
	out := make([]domain.KnowledgeEvent, 0, len(events))
	for _, e := range events {
		if e.AggregateType == domain.AggregateArticle {
			continue
		}
		out = append(out, e)
	}
	return out
}

// applyLensVisibility filters article events according to visibility map.
func applyLensVisibility(events []domain.KnowledgeEvent, visibility map[uuid.UUID]bool) []domain.KnowledgeEvent {
	out := make([]domain.KnowledgeEvent, 0, len(events))
	for _, e := range events {
		if e.AggregateType != domain.AggregateArticle {
			out = append(out, e)
			continue
		}
		id, parseErr := uuid.Parse(e.AggregateID)
		if parseErr != nil {
			continue
		}
		if visibility[id] {
			out = append(out, e)
		}
	}
	return out
}
