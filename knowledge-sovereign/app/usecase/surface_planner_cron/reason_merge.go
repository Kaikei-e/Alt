package surface_planner_cron

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"knowledge-sovereign/driver/sovereign_db"
)

// Why-code reconciliation for Surface Planner v2 (ADR-000913 §D-6 /
// canonical contract §11). When the planner cron recomputes an entry's
// surface placement and the surrounding why-code set changes, we emit a
// `knowledge_loop.reason_merged.v1` system event in the same logical batch
// so the projector can patch the entry's why narrative without a manual
// rerun.
//
// The event is reproject-safe by construction: the dedupe_key incorporates
// the batch_max_seq, so replays of the same input event log produce the
// same emission set (or none, when a partial replay reaches the boundary
// and the dedupe key already lives in the dedupe table).

// EventReasonMerged is the canonical event type for why-code reconciliation.
const EventReasonMerged = "knowledge_loop.reason_merged.v1"

// DiffWhyCodes returns the (added, removed) why-code deltas between two
// slices. Both inputs are treated as sets — duplicates are folded — and
// the output is deterministic (sorted ascending) so reproject yields the
// same payload.
func DiffWhyCodes(prev, next []string) (added, removed []string) {
	prevSet := make(map[string]struct{}, len(prev))
	for _, c := range prev {
		if c == "" {
			continue
		}
		prevSet[c] = struct{}{}
	}
	nextSet := make(map[string]struct{}, len(next))
	for _, c := range next {
		if c == "" {
			continue
		}
		nextSet[c] = struct{}{}
	}
	for c := range nextSet {
		if _, ok := prevSet[c]; !ok {
			added = append(added, c)
		}
	}
	for c := range prevSet {
		if _, ok := nextSet[c]; !ok {
			removed = append(removed, c)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

// BuildReasonMergedEvent assembles a system-emitted `ReasonMerged` event
// for the given entry. Returns (zero, false) when there is no delta — the
// caller should skip the emit rather than write a no-op event.
//
// The dedupe_key shape `ReasonMerged:<itemKey>:<batchMaxSeq>` makes reruns
// of the same batch idempotent at the AppendKnowledgeEvent layer.
func BuildReasonMergedEvent(
	anchor sovereign_db.KnowledgeEvent,
	articleID, itemKey string,
	addedCodes, removedCodes []string,
	batchMaxSeq int64,
) (sovereign_db.KnowledgeEvent, bool, error) {
	if len(addedCodes) == 0 && len(removedCodes) == 0 {
		return sovereign_db.KnowledgeEvent{}, false, nil
	}
	if anchor.UserID == nil {
		return sovereign_db.KnowledgeEvent{}, false, fmt.Errorf("reason_merged: anchor event has no user_id")
	}
	body := map[string]any{
		"article_id":        articleID,
		"item_key":          itemKey,
		"added_why_codes":   addedCodes,
		"removed_why_codes": removedCodes,
		"batch_max_seq":     batchMaxSeq,
		"reason_merged_at":  anchor.OccurredAt.UTC().Format(time.RFC3339Nano),
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return sovereign_db.KnowledgeEvent{}, false, err
	}
	dedupeKey := fmt.Sprintf("%s:%s:%d", EventReasonMerged, itemKey, batchMaxSeq)
	uid := *anchor.UserID
	return sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    anchor.OccurredAt,
		TenantID:      anchor.TenantID,
		UserID:        &uid,
		ActorType:     "system",
		ActorID:       "surface_planner_v2",
		EventType:     EventReasonMerged,
		AggregateType: "article",
		AggregateID:   articleID,
		DedupeKey:     dedupeKey,
		Payload:       payload,
	}, true, nil
}
