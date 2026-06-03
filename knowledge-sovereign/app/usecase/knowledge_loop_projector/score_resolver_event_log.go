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
	// Phase 2 semantic Continue signal. Only events with continue_flag=true
	// are tallied (gate inside the resolver loop). Snooze / Save (continue
	// false) deliberately do not feed Continue placement.
	EventKnowledgeLoopActed,
	// ADR-000908 §Δ1: knowledge_loop.act_outcome.v1 events drive the
	// ActOutcomeSignal aggregation (engaged=+1, deep_engagement=+2,
	// accepted_change=+1, stale_save=-1, no_engagement=-2). Entry-keyed
	// match prevents cross-entry signal leakage.
	EventKnowledgeLoopActOutcome,
	// Article URL pin sources. These do not feed bucket placement; they
	// only let the resolver carry the article's canonical http(s) URL onto
	// SurfaceScoreInputs.SourceURL so seedActTargets can keep
	// act_targets[0].source_url stable when a non-article event re-seeds
	// the entry. Order inside this list is irrelevant — the resolver scans
	// by event_seq and validates the URL scheme via isHTTPSourceURL.
	EventArticleCreated,
	EventArticleUpdated,
	EventArticleUrlBackfilled,
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

	// Pin the article_id for the entry so seedActTargets can keep emitting a
	// stable article act_target across non-article events (notably
	// augur.conversation_linked.v1 whose payload only names entry_key +
	// conversation_id). The pin is conservative: only set when the current
	// event names it directly, otherwise filled from a prior event on the
	// same entry_key inside the score window. Reproject-safe because the
	// event log is the only source.
	out.ArticleID = thisArticleID

	// StalenessScore is purely event-bound: gap between the event's
	// occurrence time and the source's observed time (article published_at /
	// observed_at on the payload). Reproject-safe — no time.Now() involved.
	out.StalenessScore = pureStalenessBucket(ev.OccurredAt, readSourceObservedAt(ev.Payload))

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
			// Fill the article_id pin from a prior SummaryVersionCreated on
			// the same entry_key when the projecting event omitted it (the
			// augur.conversation_linked.v1 case). entry_key is a stable
			// natural key for the entry, so the first match wins.
			if out.ArticleID == "" && eArticleID != "" {
				eEntryKey := readEntryKey(e.Payload)
				if eEntryKey != "" && eEntryKey == thisEntryKey {
					out.ArticleID = eArticleID
				}
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
				// ContradictionCount is v1-defined as "count of summary
				// supersedes targeting this article in 7d"; the same event
				// drives both signals so a future SummaryContradicted event
				// can split them without breaking replay.
				out.ContradictionCount++
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
				out.QuestionContinuationScore++
			}
		case EventRecapTopicSnapshotted:
			eTerms := readTopicTerms(e.Payload)
			if len(eTerms) > 0 && len(thisTags) > 0 {
				if hasIntersection(thisTags, eTerms) {
					out.TopicOverlapCount++
					// RecapClusterMomentum echoes TopicOverlapCount until a
					// future resolver can distinguish "hot cluster" from
					// "any term overlap" by counting unique cluster_ids.
					out.RecapClusterMomentum++
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
		case EventKnowledgeLoopActed:
			// Entry-keyed match first (aggregate_id == entry_key for Loop
			// transitions) so cross-entry behaviour cannot bleed.
			eEntryKey := readEntryKey(e.Payload)
			if eEntryKey == "" || eEntryKey != thisEntryKey {
				continue
			}
			// Phase 2 semantic Continue signal. Gated on continue_flag=true so
			// Snooze (false) does not promote Continue.
			if readPayloadBool(e.Payload, "continue_flag") {
				out.RecentContinueActionCount++
			}
			// ADR-000938: a compare act is the user inspecting the redline of a
			// changed entry. It carries continue_flag=false, so it must be
			// counted independently of RecentContinueActionCount. It drives the
			// Contradiction relation's ADVANCING state.
			if isCompareIntent(readPayloadString(e.Payload, "acted_intent")) {
				out.CompareActionCount++
			}
		case EventKnowledgeLoopActOutcome:
			// ADR-000908 §Δ1: aggregate the cumulative outcome signal on
			// this entry. Entry-keyed match — cross-entry outcomes do not
			// leak into the current entry's ActOutcomeSignal. Unknown
			// outcome labels contribute 0 (defensive against payload schema
			// drift; new outcomes go through a coordinated release).
			eEntryKey := readEntryKey(e.Payload)
			if eEntryKey == "" || eEntryKey != thisEntryKey {
				continue
			}
			outcome := readPayloadString(e.Payload, "outcome")
			out.ActOutcomeSignal += actOutcomeDelta(outcome)
			// ADR-000938: accepted_change ("compare → dismiss = reconciled")
			// drives the Contradiction relation's RESOLVED state — the visible
			// close of the loop.
			if isAcceptedChangeOutcome(outcome) {
				out.AcceptedChangeCount++
			}
		}
	}

	// Second pass: pin SourceURL from prior article events on the same
	// article_id. Runs after pass 1 so out.ArticleID is already filled from
	// either the projecting event payload or a prior SummaryVersionCreated.
	// Without this pass, non-article events (notably augur.conversation_linked
	// .v1 / knowledge_loop.surface_plan_recomputed.v1 / TagSetVersionCreated)
	// trigger seedActTargets with no url on payload, which rewrites
	// act_targets[0].source_url to "" on every re-seed (systemic 8319-entry
	// drop documented at 2026-05-27).
	if out.ArticleID != "" {
		var pinSeq int64
		for _, e := range events {
			switch e.EventType {
			case EventArticleCreated, EventArticleUpdated, EventArticleUrlBackfilled:
			default:
				continue
			}
			eArticleID := readArticleID(e.Payload)
			if eArticleID == "" || eArticleID != out.ArticleID {
				continue
			}
			if e.EventSeq < pinSeq {
				continue
			}
			raw := readPayloadString(e.Payload, "url", "link")
			validated, ok := isHTTPSourceURL(raw)
			if !ok {
				continue
			}
			out.SourceURL = validated
			pinSeq = e.EventSeq
		}
	}

	return out
}

// isCompareIntent reports whether an acted_intent payload value is the compare
// intent. Matches both the canonical enum form and the bare label so a producer
// or test using either spelling is counted (mirrors actOutcomeDelta).
func isCompareIntent(intent string) bool {
	switch intent {
	case "compare", "DECISION_INTENT_COMPARE":
		return true
	default:
		return false
	}
}

// isAcceptedChangeOutcome reports whether an outcome payload value is
// accepted_change, in either the canonical enum form or the bare label.
func isAcceptedChangeOutcome(outcome string) bool {
	switch outcome {
	case "accepted_change", "ACT_OUTCOME_KIND_ACCEPTED_CHANGE":
		return true
	default:
		return false
	}
}

// actOutcomeDelta maps an ActOutcomeKind enum string onto the
// ActOutcomeSignal contribution (ADR-000908 §Δ1). Unknown / unspecified
// outcomes contribute 0 so payload schema drift does not silently move
// entries between buckets.
func actOutcomeDelta(outcome string) int32 {
	switch outcome {
	case "engaged", "ACT_OUTCOME_KIND_ENGAGED":
		return 1
	case "deep_engagement", "ACT_OUTCOME_KIND_DEEP_ENGAGEMENT":
		return 2
	case "accepted_change", "ACT_OUTCOME_KIND_ACCEPTED_CHANGE":
		return 1
	case "stale_save", "ACT_OUTCOME_KIND_STALE_SAVE":
		return -1
	case "no_engagement", "ACT_OUTCOME_KIND_NO_ENGAGEMENT":
		return -2
	default:
		return 0
	}
}

// readPayloadBool returns the bool value of `key` on a JSON payload. Returns
// false when the key is missing or not a bool.
func readPayloadBool(raw json.RawMessage, key string) bool {
	if len(raw) == 0 {
		return false
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
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

// readSourceObservedAt extracts the source observation time (article
// published_at, recap snapshot start, etc.) from a knowledge_event payload.
// Returns the zero value when no recognised field is present, which
// pureStalenessBucket interprets as "treat as fresh".
func readSourceObservedAt(raw json.RawMessage) time.Time {
	s := readPayloadString(raw, "source_observed_at", "published_at", "observed_at")
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
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
