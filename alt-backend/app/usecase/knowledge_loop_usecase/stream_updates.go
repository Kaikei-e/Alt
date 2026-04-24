package knowledge_loop_usecase

import (
	"alt/domain"
	"encoding/json"
)

// StreamUpdateKind enumerates the stream frame kinds we emit for Knowledge Loop
// subscribers. Each kind maps to one variant of the proto
// StreamKnowledgeLoopUpdatesResponse.update oneof.
type StreamUpdateKind int

const (
	StreamUpdateKindNone StreamUpdateKind = iota
	StreamUpdateKindAppended
	StreamUpdateKindRevised
	StreamUpdateKindSuperseded
	StreamUpdateKindWithdrawn
	StreamUpdateKindSurfaceRebalanced
)

// StreamUpdateFrame carries the classification result for a single event.
// The handler converts this into the proto oneof on send.
type StreamUpdateFrame struct {
	Kind        StreamUpdateKind
	EntryKey    string
	NewEntryKey string // populated for Superseded
	Revision    int64  // event_seq; client compares to its local projection_revision
}

// ClassifyLoopStreamUpdate maps a knowledge_events row to a Knowledge Loop stream
// frame. Returns ok=false for events that do not produce a client-visible frame
// (unknown event types, high-frequency observed events, system events without
// user context).
//
// The classification is deterministic and reproject-safe: a replay of the same
// event log produces the same frame sequence.
func ClassifyLoopStreamUpdate(ev *domain.KnowledgeEvent) (StreamUpdateFrame, bool) {
	if ev == nil || ev.UserID == nil {
		return StreamUpdateFrame{}, false
	}

	entryKey := extractEntryKeyForStream(ev)

	switch ev.EventType {
	case domain.EventSummaryVersionCreated,
		domain.EventHomeItemsSeen,
		domain.EventHomeItemAsked:
		return StreamUpdateFrame{
			Kind:     StreamUpdateKindAppended,
			EntryKey: entryKey,
			Revision: ev.EventSeq,
		}, true

	case domain.EventHomeItemOpened,
		domain.EventHomeItemListened,
		domain.EventHomeItemTagClicked:
		return StreamUpdateFrame{
			Kind:     StreamUpdateKindRevised,
			EntryKey: entryKey,
			Revision: ev.EventSeq,
		}, true

	case domain.EventHomeItemSuperseded, domain.EventSummarySuperseded, domain.EventTagSetSuperseded:
		newKey := extractStringFromPayload(ev.Payload, "new_entry_key", "superseded_by_entry_key")
		return StreamUpdateFrame{
			Kind:        StreamUpdateKindSuperseded,
			EntryKey:    entryKey,
			NewEntryKey: newKey,
			Revision:    ev.EventSeq,
		}, true

	case domain.EventHomeItemDismissed:
		return StreamUpdateFrame{
			Kind:     StreamUpdateKindWithdrawn,
			EntryKey: entryKey,
			Revision: ev.EventSeq,
		}, true

	case domain.EventKnowledgeLoopObserved:
		// High-frequency; suppressed per canonical contract §9 (EntryRevised
		// is for silent projection changes; observed alone is too noisy).
		return StreamUpdateFrame{}, false

	case domain.EventKnowledgeLoopOriented,
		domain.EventKnowledgeLoopDecisionPresented,
		domain.EventKnowledgeLoopActed,
		domain.EventKnowledgeLoopReturned,
		domain.EventKnowledgeLoopDeferred,
		domain.EventKnowledgeLoopSessionReset,
		domain.EventKnowledgeLoopLensModeSwitched:
		// Session-level transitions can reshuffle foreground/bucket composition;
		// emit a SurfaceRebalanced hint so the client refetches selectively.
		return StreamUpdateFrame{
			Kind:     StreamUpdateKindSurfaceRebalanced,
			EntryKey: entryKey,
			Revision: ev.EventSeq,
		}, true
	}

	return StreamUpdateFrame{}, false
}

// extractEntryKeyForStream prefers the payload's entry_key field, falling back
// to the event's AggregateID. Either is safe: both come directly from the event
// payload written at append time, so the classification is replay-deterministic.
func extractEntryKeyForStream(ev *domain.KnowledgeEvent) string {
	if key := extractStringFromPayload(ev.Payload, "entry_key", "item_key"); key != "" {
		return key
	}
	return ev.AggregateID
}

// extractStringFromPayload is a minimal payload field reader used by the stream
// classifier. Duplicates the projector's helper so this package stays free of
// a cross-package import on job/*.
func extractStringFromPayload(payload json.RawMessage, keys ...string) string {
	if len(payload) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}
