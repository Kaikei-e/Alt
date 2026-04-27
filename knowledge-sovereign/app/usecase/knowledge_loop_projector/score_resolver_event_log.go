package knowledge_loop_projector

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"knowledge-sovereign/driver/sovereign_db"
)

// uuidPattern is the canonical UUID v1-v5 form. We reject anything else from
// upstream payloads before forwarding it into URL formatters, since a
// malformed `recap_topic_snapshot_id` is the obvious open-redirect /
// path-traversal vector when the projector builds /recap/topic/<id>.
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

// scoreWindow is the lookback applied when summing v2 evidence. The
// canonical contract §6.4.1 / §11 documents this as "within
// event.occurred_at - 7d". Bound to event time, never wall-clock.
const scoreWindow = 7 * 24 * time.Hour

// EventLogLookup is the dependency the resolver needs to fetch evidence
// events. It is intentionally narrow — implementations only need to return
// events scoped by user, time window, and a small allowlist of event types.
//
// All implementations MUST physically bind the supplied userID into the SQL
// (or RPC) layer. F-001 mitigation lives at this seam: a resolver that
// returns rows for a different user is a critical violation and bumps
// crossUserIsolationViolationTotal.
type EventLogLookup interface {
	ListKnowledgeEventsForUserInWindow(
		ctx context.Context,
		userID uuid.UUID,
		eventTypes []string,
		since, until time.Time,
		limit int,
	) ([]sovereign_db.KnowledgeEvent, error)
}

// EventLogSurfaceScoreResolver computes SurfaceScoreInputs from the
// knowledge_events log without ever consulting mutable views or latest
// state. The lookup is bounded by [event.occurred_at - 7d, event.occurred_at)
// and a small allowlist of event types feeding Surface Planner v2:
//
//   - SummaryVersionCreated / SummarySuperseded → version_drift_count
//   - HomeItemOpened → has_open_interaction
//   - augur.conversation_linked.v1 → has_augur_link (entry-keyed)
//   - recap.topic_snapshotted.v1 → topic_overlap_count (term overlap)
//   - tag_set_versions emissions ride on SummaryVersionCreated payload
//     (tag_set_version_id) so we count those toward tag_overlap_count.
//
// Pure aggregation — replaying the same event log produces the same
// SurfaceScoreInputs bit-for-bit. Reproject-safe.
type EventLogSurfaceScoreResolver struct {
	lookup EventLogLookup
	limit  int
}

// NewEventLogSurfaceScoreResolver wires a lookup into the resolver. The
// limit caps how many evidence events are scanned per resolution to keep
// per-event tail latency bounded; a typical 7-day window for a single user
// holds far fewer events than this in practice.
func NewEventLogSurfaceScoreResolver(lookup EventLogLookup) *EventLogSurfaceScoreResolver {
	return &EventLogSurfaceScoreResolver{lookup: lookup, limit: 256}
}

// resolverEventTypes is the allowlist passed to the lookup. Keep this in
// sync with the canonical contract §6.4.1 (upstream snapshot events) and
// §11 (Why-kind mapping) — adding a new event type here without updating
// the contract is an incident.
var resolverEventTypes = []string{
	EventSummaryVersionCreated,
	EventSummarySuperseded,
	EventHomeItemOpened,
	EventRecapTopicSnapshotted,
	EventAugurConversationLinked,
}

// Resolve queries the event log and aggregates the v2 evidence. It returns
// the same SurfaceScoreInputs shape the NullSurfaceScoreResolver does, but
// with non-zero counts when relevant evidence exists. Errors fall back to
// the same shape as Null — the projector never fails the batch because the
// resolver couldn't fetch evidence; placement degrades to v1 mapping.
func (r *EventLogSurfaceScoreResolver) Resolve(
	ctx context.Context,
	ev *sovereign_db.KnowledgeEvent,
) SurfaceScoreInputs {
	out := SurfaceScoreInputs{
		FreshnessAt: ev.OccurredAt,
		EventType:   ev.EventType,
	}
	if ev.UserID == nil {
		return out
	}
	since := ev.OccurredAt.Add(-scoreWindow)
	until := ev.OccurredAt

	events, err := r.lookup.ListKnowledgeEventsForUserInWindow(
		ctx,
		*ev.UserID,
		resolverEventTypes,
		since,
		until,
		r.limit,
	)
	if err != nil {
		return out
	}

	// F-001 enforcement at the data boundary. The lookup is supposed to bind
	// user_id physically; double-check defensively. A mismatch is a
	// critical violation, not a silent data-quality blip.
	for _, e := range events {
		if e.UserID == nil || *e.UserID != *ev.UserID {
			crossUserIsolationViolationTotal.Inc()
			return out
		}
	}

	thisArticleID := readArticleID(ev.Payload)
	thisTags := readArticleTagSet(ev.Payload)
	thisEntryKey := readEntryKey(ev.Payload)

	// Track the most-recent RecapTopicSnapshotted whose top_terms overlap with
	// the entry's tags. The projector uses this to seed a Recap act_target so
	// the UI can render an "Open Recap" CTA on the entry without us having to
	// extend the projector to treat the snapshot itself as an entry-creating
	// event (canonical contract §6.4.1: snapshot events feed Surface Planner
	// inputs only).
	var matchedRecapAt time.Time
	for _, e := range events {
		switch e.EventType {
		case EventSummaryVersionCreated:
			eArticleID := readArticleID(e.Payload)
			if eArticleID != "" && eArticleID == thisArticleID {
				out.VersionDriftCount++
			}
			eTags := readArticleTagSet(e.Payload)
			if len(thisTags) > 0 && len(eTags) > 0 {
				if hasIntersection(thisTags, eTags) {
					out.TagOverlapCount++
				}
			}
		case EventSummarySuperseded:
			eArticleID := readArticleID(e.Payload)
			if eArticleID != "" && eArticleID == thisArticleID {
				out.VersionDriftCount++
			}
		case EventHomeItemOpened:
			eEntryKey := readEntryKey(e.Payload)
			if eEntryKey != "" && eEntryKey == thisEntryKey {
				out.HasOpenInteraction = true
			}
		case EventAugurConversationLinked:
			eEntryKey := readEntryKey(e.Payload)
			if eEntryKey != "" && eEntryKey == thisEntryKey {
				out.HasAugurLink = true
			}
		case EventRecapTopicSnapshotted:
			eTerms := readTopicTerms(e.Payload)
			if len(eTerms) > 0 && len(thisTags) > 0 {
				if hasIntersection(thisTags, eTerms) {
					out.TopicOverlapCount++
					// Pin the most-recent matching snapshot id (security:
					// validate as UUID before forwarding to the URL formatter
					// so a malformed payload can't smuggle a path-traversal
					// or javascript: scheme into /recap/topic/<id>).
					if e.OccurredAt.After(matchedRecapAt) {
						sid := readPayloadString(e.Payload, "recap_topic_snapshot_id")
						if sid != "" && isUUID(sid) {
							out.RecapTopicSnapshotID = sid
							matchedRecapAt = e.OccurredAt
						}
					}
				}
			}
		}
	}

	return out
}

// isUUID rejects any string that is not a canonical 36-char hyphenated UUID.
// Cheap regex check so the resolver does not depend on uuid.Parse being free
// of allocation in hot paths.
func isUUID(s string) bool {
	return uuidPattern.MatchString(s)
}

// --- payload field readers (pure, stateless) -------------------------------

func readArticleID(raw json.RawMessage) string {
	return readPayloadString(raw, "article_id")
}

func readEntryKey(raw json.RawMessage) string {
	return readPayloadString(raw, "entry_key", "item_key")
}

func readArticleTagSet(raw json.RawMessage) []string {
	return readPayloadStringSlice(raw, "tags", "article_tags")
}

func readTopicTerms(raw json.RawMessage) []string {
	return readPayloadStringSlice(raw, "top_terms", "topic_terms")
}

func readPayloadString(raw json.RawMessage, keys ...string) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
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

func readPayloadStringSlice(raw json.RawMessage, keys ...string) []string {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		arr, ok := v.([]any)
		if !ok {
			continue
		}
		out := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

// hasIntersection returns true if any string in `a` appears in `b`,
// matching case-insensitively against trimmed forms. Used for both tag and
// topic overlap checks so the semantic ("articles share a tag/term") is the
// same regardless of casing or accidental whitespace.
func hasIntersection(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(b))
	for _, s := range b {
		k := strings.ToLower(strings.TrimSpace(s))
		if k == "" {
			continue
		}
		set[k] = struct{}{}
	}
	for _, s := range a {
		k := strings.ToLower(strings.TrimSpace(s))
		if k == "" {
			continue
		}
		if _, ok := set[k]; ok {
			return true
		}
	}
	return false
}
