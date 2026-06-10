package knowledge_loop_projector

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"

	"knowledge-sovereign/driver/sovereign_db"
)

// Knowledge Loop evidence accumulator (ADR-000939).
//
// This replaces EventLogSurfaceScoreResolver's per-entry 7-day window re-scan
// (which read ORDER BY event_seq ASC LIMIT 256 over a broad multi-article
// window and so, at production log density, read only the OLDEST 256 events and
// missed every recent fact — relations=[] for every entry). The accumulator
// folds each event's evidence into knowledge_loop_evidence in O(1), and the
// entry's SurfaceScoreInputs are a pure derivation over that table.
//
// Co-projection contract (ADR-000939 §2): the projector calls
// deriveSurfaceScoreInputs (reads the accumulator state of the prefix seq < N)
// BEFORE applyEvidence (folds event N's fact), so an entry's relations stay a
// deterministic pure function of the event-log prefix and reproject reproduces
// them bit-for-bit. The accumulator is disposable and TRUNCATEd + rebuilt on
// every reproject.

// scoreWindow is the lookback applied when summing windowed evidence. Bound to
// event.occurred_at, never wall-clock. Matches the canonical contract §6.4.1 /
// §11 "within event.occurred_at - 7d".
const scoreWindow = 7 * 24 * time.Hour

// failLoudError marks an error that must ABORT the projector batch rather than
// be logged-and-skipped like a malformed-payload event. ADR-000939 §4 /
// CLAUDE.md #8: an accumulator read/write failure must stop the batch (so the
// checkpoint does not advance and the events re-project once the dependency
// recovers) — degrading to empty inputs would silently empty the Orient
// surface, the exact PM-2026-045 failure mode this rework exists to kill.
type failLoudError struct{ err error }

func (e failLoudError) Error() string { return e.err.Error() }
func (e failLoudError) Unwrap() error { return e.err }

func failLoud(err error) error {
	if err == nil {
		return nil
	}
	return failLoudError{err: err}
}

// Accumulator scope kinds (knowledge_loop_evidence.scope_kind). Kept in sync
// with the CHECK constraint in migration 00025.
const (
	evScopeEntry     = "entry"
	evScopeArticle   = "article"
	evScopeTag       = "tag"
	evScopeTopicTerm = "topic_term"
)

// Accumulator signal kinds (knowledge_loop_evidence.signal_kind). Each maps a
// SurfaceScoreInputs field; kept in sync with the CHECK constraint in 00025.
const (
	evSigSummaryVersion   = "summary_version"
	evSigSummarySupersede = "summary_supersede"
	evSigOpenInteraction  = "open_interaction"
	evSigContinueAct      = "continue_act"
	evSigCompareAct       = "compare_act"
	evSigActOutcome       = "act_outcome"
	evSigAugurLink        = "augur_link"
	evSigTagActivity      = "tag_activity"
	evSigTopicSnapshot    = "topic_snapshot"
	evSigTagSetCurrent    = "tag_set_current"
	evSigURLPin           = "url_pin"
	evSigArticlePin       = "article_pin"
)

// applyEvidence folds the event's fact(s) into the accumulator. Called AFTER
// the entry is derived (ADR-000939 §2b) for every event that carries evidence,
// so future entries see it. Fail-loud: an accumulator write error is returned,
// never swallowed — a silent fallback here would be exactly the [[000928]] /
// PM-2026-045 failure mode the rework exists to kill.
func (p *Projector) applyEvidence(ctx context.Context, ev *sovereign_db.KnowledgeEvent) error {
	if ev.UserID == nil {
		return nil
	}
	for _, w := range evidenceWritesForEvent(ev) {
		if err := p.repo.UpsertKnowledgeLoopEvidence(ctx, w); err != nil {
			return failLoud(fmt.Errorf("applyEvidence %s/%s/%s: %w", w.ScopeKind, w.ScopeRef, w.SignalKind, err))
		}
	}
	return nil
}

// evidenceWritesForEvent is the pure event → accumulator-update mapping. Each
// returned write is either a fact append (NewFact) or a pin (PinnedText /
// PinnedPayload). Reproject-safe: every field is read from the event payload.
func evidenceWritesForEvent(ev *sovereign_db.KnowledgeEvent) []sovereign_db.KnowledgeLoopEvidenceWrite {
	var ws []sovereign_db.KnowledgeLoopEvidenceWrite

	switch ev.EventType {
	case EventSummaryVersionCreated:
		if aid := evidenceArticleID(ev); aid != "" {
			ws = append(ws, factWrite(ev, evScopeArticle, aid, evSigSummaryVersion,
				extractStringField(ev.Payload, "summary_version_id")))
		}

	case EventSummarySuperseded, EventHomeItemSuperseded:
		if aid := evidenceArticleID(ev); aid != "" {
			ws = append(ws, factWrite(ev, evScopeArticle, aid, evSigSummarySupersede, ""))
		}

	case EventArticleCreated, EventArticleUpdated, EventArticleUrlBackfilled:
		if aid := evidenceArticleID(ev); aid != "" {
			if validated, ok := isHTTPSourceURL(extractStringField(ev.Payload, "url", "link")); ok {
				ws = append(ws, pinTextWrite(ev, evScopeArticle, aid, evSigURLPin, validated))
			}
		}

	case EventTagSetVersionCreated:
		aid := evidenceArticleID(ev)
		tags := normalizeTokens(readPayloadStringSlice(ev.Payload, "tags", "article_tags"))
		if aid != "" && len(tags) > 0 {
			if payload, err := json.Marshal(map[string][]string{"tags": tags}); err == nil {
				ws = append(ws, pinPayloadWrite(ev, evScopeArticle, aid, evSigTagSetCurrent, payload))
			}
			for _, t := range tags {
				ws = append(ws, factWrite(ev, evScopeTag, t, evSigTagActivity, aid))
			}
		}

	case EventHomeItemOpened:
		if ek := evidenceEntryKey(ev); ek != "" {
			ws = append(ws, factWrite(ev, evScopeEntry, ek, evSigOpenInteraction, ""))
			if aid := evidenceArticleID(ev); aid != "" {
				ws = append(ws, pinTextWrite(ev, evScopeEntry, ek, evSigArticlePin, aid))
			}
		}

	case EventKnowledgeLoopActed:
		if ek := evidenceEntryKey(ev); ek != "" {
			if readPayloadBool(ev.Payload, "continue_flag") {
				ws = append(ws, factWrite(ev, evScopeEntry, ek, evSigContinueAct, ""))
			}
			if isCompareIntent(extractStringField(ev.Payload, "acted_intent")) {
				ws = append(ws, factWrite(ev, evScopeEntry, ek, evSigCompareAct, ""))
			}
		}

	case EventKnowledgeLoopActOutcome:
		if ek := evidenceEntryKey(ev); ek != "" {
			ws = append(ws, factWrite(ev, evScopeEntry, ek, evSigActOutcome,
				extractStringField(ev.Payload, "outcome")))
		}

	case EventAugurConversationLinked:
		if ek := evidenceEntryKey(ev); ek != "" {
			ws = append(ws, factWrite(ev, evScopeEntry, ek, evSigAugurLink, ""))
			if aid := evidenceArticleID(ev); aid != "" {
				ws = append(ws, pinTextWrite(ev, evScopeEntry, ek, evSigArticlePin, aid))
			}
		}

	case EventRecapTopicSnapshotted:
		sid := extractStringField(ev.Payload, "recap_topic_snapshot_id")
		terms := normalizeTokens(readPayloadStringSlice(ev.Payload, "top_terms", "topic_terms"))
		if sid != "" && isUUID(sid) {
			for _, t := range terms {
				ws = append(ws, factWrite(ev, evScopeTopicTerm, t, evSigTopicSnapshot, sid))
			}
		}
	}

	return ws
}

func factWrite(ev *sovereign_db.KnowledgeEvent, scopeKind, scopeRef, signalKind, v string) sovereign_db.KnowledgeLoopEvidenceWrite {
	return sovereign_db.KnowledgeLoopEvidenceWrite{
		UserID:     *ev.UserID,
		TenantID:   ev.TenantID,
		ScopeKind:  scopeKind,
		ScopeRef:   scopeRef,
		SignalKind: signalKind,
		NewFact:    &sovereign_db.KnowledgeLoopEvidenceFact{OccurredAt: ev.OccurredAt, EventSeq: ev.EventSeq, V: v},
		OccurredAt: ev.OccurredAt,
		EventSeq:   ev.EventSeq,
	}
}

func pinTextWrite(ev *sovereign_db.KnowledgeEvent, scopeKind, scopeRef, signalKind, text string) sovereign_db.KnowledgeLoopEvidenceWrite {
	return sovereign_db.KnowledgeLoopEvidenceWrite{
		UserID:     *ev.UserID,
		TenantID:   ev.TenantID,
		ScopeKind:  scopeKind,
		ScopeRef:   scopeRef,
		SignalKind: signalKind,
		PinnedText: text,
		OccurredAt: ev.OccurredAt,
		EventSeq:   ev.EventSeq,
	}
}

func pinPayloadWrite(ev *sovereign_db.KnowledgeEvent, scopeKind, scopeRef, signalKind string, payload []byte) sovereign_db.KnowledgeLoopEvidenceWrite {
	return sovereign_db.KnowledgeLoopEvidenceWrite{
		UserID:        *ev.UserID,
		TenantID:      ev.TenantID,
		ScopeKind:     scopeKind,
		ScopeRef:      scopeRef,
		SignalKind:    signalKind,
		PinnedPayload: payload,
		OccurredAt:    ev.OccurredAt,
		EventSeq:      ev.EventSeq,
	}
}

// deriveSurfaceScoreInputs builds the SurfaceScoreInputs for the entry the
// event projects, purely from the accumulator state of the event-log prefix
// (seq < N for entry-creating events, since applyEvidence runs after this; seq
// ≤ N for the late-fuel re-derivation, which applies its fact first because the
// new evidence is the whole point of the re-derivation). decideBucketV2 /
// extractRelations / hasV2Signal consume the result unchanged.
//
// Fail-loud: a read error is returned, never degraded to empty inputs. An empty
// result is a legitimate "no fuel" state; a read error is a bug and must stop
// the batch (ADR-000939 §4).
func (p *Projector) deriveSurfaceScoreInputs(ctx context.Context, ev *sovereign_db.KnowledgeEvent) (SurfaceScoreInputs, error) {
	out := SurfaceScoreInputs{FreshnessAt: ev.OccurredAt, EventType: ev.EventType}
	if ev.UserID == nil {
		return out, nil
	}

	entryKey := evidenceEntryKey(ev)
	articleID := evidenceArticleID(ev)

	// Event-payload-derived signals (not accumulator): staleness from the gap
	// between event time and the source's observed time; confidence ladder from
	// the recap persist stage when the projecting event carries it.
	out.StalenessScore = pureStalenessBucket(ev.OccurredAt, readPayloadTimestamp(ev.Payload,
		"source_observed_at", "published_at", "observed_at"))
	out.ConfidenceLadder = readConfidenceLadder(ev.Payload)

	scopes := make([]sovereign_db.KnowledgeLoopEvidenceScope, 0, 2)
	if entryKey != "" {
		scopes = append(scopes, sovereign_db.KnowledgeLoopEvidenceScope{ScopeKind: evScopeEntry, ScopeRef: entryKey})
	}
	if articleID != "" {
		scopes = append(scopes, sovereign_db.KnowledgeLoopEvidenceScope{ScopeKind: evScopeArticle, ScopeRef: articleID})
	}

	since := ev.OccurredAt.Add(-scoreWindow)
	until := ev.OccurredAt

	var articleTags []string
	if len(scopes) > 0 {
		states, err := p.repo.GetKnowledgeLoopEvidenceForScopes(ctx, *ev.UserID, scopes)
		if err != nil {
			return out, failLoud(fmt.Errorf("deriveSurfaceScoreInputs: read entry/article evidence: %w", err))
		}
		for _, st := range states {
			switch st.ScopeKind {
			case evScopeEntry:
				applyEntrySignal(&out, st, since, until, &articleID)
			case evScopeArticle:
				applyArticleSignal(&out, st, since, until, &articleTags)
			}
		}
	}
	out.ArticleID = articleID

	// Cluster overlap: this entry's article tags against the user's other tag
	// activity (excluding self) and the recap topic terms in the window. Reads a
	// second batch keyed by the article's current tags.
	if len(articleTags) > 0 {
		tagScopes := make([]sovereign_db.KnowledgeLoopEvidenceScope, 0, 2*len(articleTags))
		for _, t := range articleTags {
			tagScopes = append(tagScopes,
				sovereign_db.KnowledgeLoopEvidenceScope{ScopeKind: evScopeTag, ScopeRef: t},
				sovereign_db.KnowledgeLoopEvidenceScope{ScopeKind: evScopeTopicTerm, ScopeRef: t},
			)
		}
		clusterStates, err := p.repo.GetKnowledgeLoopEvidenceForScopes(ctx, *ev.UserID, tagScopes)
		if err != nil {
			return out, failLoud(fmt.Errorf("deriveSurfaceScoreInputs: read cluster evidence: %w", err))
		}
		var latestSnapAt time.Time
		for _, st := range clusterStates {
			switch st.SignalKind {
			case evSigTagActivity:
				for _, f := range st.Facts {
					// Exclude the entry's own article so a tag only this article
					// carries is not mistaken for a tracked topic.
					if factInWindow(f, since, until) && f.V != articleID {
						out.TagOverlapCount++
					}
				}
			case evSigTopicSnapshot:
				for _, f := range st.Facts {
					if !factInWindow(f, since, until) {
						continue
					}
					out.TopicOverlapCount++
					out.RecapClusterMomentum++
					if f.OccurredAt.After(latestSnapAt) && isUUID(f.V) {
						out.RecapTopicSnapshotID = f.V
						latestSnapAt = f.OccurredAt
					}
				}
			}
		}
	}

	return out, nil
}

// applyEntrySignal folds one entry-scoped accumulator cell into the inputs.
func applyEntrySignal(out *SurfaceScoreInputs, st sovereign_db.KnowledgeLoopEvidenceState, since, until time.Time, articleID *string) {
	switch st.SignalKind {
	case evSigOpenInteraction:
		if windowCount(st.Facts, since, until) > 0 {
			out.HasOpenInteraction = true
		}
	case evSigContinueAct:
		out.RecentContinueActionCount += uint32(windowCount(st.Facts, since, until))
	case evSigCompareAct:
		out.CompareActionCount += uint32(windowCount(st.Facts, since, until))
	case evSigAugurLink:
		c := windowCount(st.Facts, since, until)
		if c > 0 {
			out.HasAugurLink = true
		}
		out.QuestionContinuationScore += uint32(c)
	case evSigActOutcome:
		for _, f := range st.Facts {
			if !factInWindow(f, since, until) {
				continue
			}
			out.ActOutcomeSignal += actOutcomeDelta(f.V)
			if isAcceptedChangeOutcome(f.V) {
				out.AcceptedChangeCount++
			}
		}
	case evSigArticlePin:
		if *articleID == "" && st.PinnedText != "" {
			*articleID = st.PinnedText
		}
	}
}

// applyArticleSignal folds one article-scoped accumulator cell into the inputs.
func applyArticleSignal(out *SurfaceScoreInputs, st sovereign_db.KnowledgeLoopEvidenceState, since, until time.Time, articleTags *[]string) {
	switch st.SignalKind {
	case evSigSummaryVersion:
		// Prior re-summaries of this article are version drift.
		out.VersionDriftCount += uint32(windowCount(st.Facts, since, until))
	case evSigSummarySupersede:
		// A supersede is both drift and a contradiction (a newer version
		// replaced what the user read) — mirrors the v1 resolver semantics.
		c := uint32(windowCount(st.Facts, since, until))
		out.VersionDriftCount += c
		out.ContradictionCount += c
	case evSigURLPin:
		out.SourceURL = st.PinnedText
	case evSigTagSetCurrent:
		*articleTags = normalizeTokens(readTagsFromPinnedPayload(st.PinnedPayload))
	}
}

// windowCount counts facts whose occurred_at is inside [since, until].
func windowCount(facts []sovereign_db.KnowledgeLoopEvidenceFact, since, until time.Time) int {
	n := 0
	for _, f := range facts {
		if factInWindow(f, since, until) {
			n++
		}
	}
	return n
}

func factInWindow(f sovereign_db.KnowledgeLoopEvidenceFact, since, until time.Time) bool {
	return !f.OccurredAt.Before(since) && !f.OccurredAt.After(until)
}

// --- entry-key / article-id resolution (pure, event-payload only) -----------

// evidenceEntryKey resolves the entry_key an event's evidence is scoped to. It
// delegates to deriveEntryKey — the SAME function the projector uses to key the
// entry row — so the accumulator's entry scope and the projected entry_key can
// never diverge (e.g. HomeItemOpened's home_session aggregate, augur's payload
// entry_key, and SummaryVersionCreated's article aggregate all resolve the same
// way on both sides). The "event:<uuid>" fallback deriveEntryKey returns for an
// unkeyable event simply yields a scope no real entry shares, so it is a safe
// no-op rather than a mis-scope.
func evidenceEntryKey(ev *sovereign_db.KnowledgeEvent) string {
	k, _ := deriveEntryKey(ev)
	return k
}

// evidenceArticleID resolves the article_id an event is about: the payload
// article_id, else the "article:" prefix of the entry key.
func evidenceArticleID(ev *sovereign_db.KnowledgeEvent) string {
	if id := extractStringField(ev.Payload, "article_id"); id != "" {
		return id
	}
	if ev.AggregateType == "article" && ev.AggregateID != "" {
		return ev.AggregateID
	}
	ek := extractStringField(ev.Payload, "entry_key", "item_key")
	if aid, ok := strings.CutPrefix(ek, "article:"); ok {
		return aid
	}
	return ""
}

// --- payload readers + small pure helpers -----------------------------------

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

func isUUID(s string) bool { return uuidPattern.MatchString(s) }

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

// readTagsFromPinnedPayload extracts the tags array from a tag_set_current pin
// (shape {"tags": [...]}).
func readTagsFromPinnedPayload(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var m struct {
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m.Tags
}

// readConfidenceLadder reads persist_stage_confidence_ladder from an event
// payload (ADR-000913 §D-10). Absent → 0 (UNSPECIFIED, no demotion).
func readConfidenceLadder(raw json.RawMessage) int32 {
	if len(raw) == 0 {
		return 0
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return 0
	}
	if v, ok := m["persist_stage_confidence_ladder"].(float64); ok {
		return int32(v)
	}
	return 0
}

// normalizeTokens lower-cases and trims tag/term tokens and drops empties so
// overlap matching is case-insensitive and stable. De-dupes within the slice.
func normalizeTokens(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		k := strings.ToLower(strings.TrimSpace(s))
		if k == "" {
			continue
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isCompareIntent(intent string) bool {
	switch intent {
	case "compare", "DECISION_INTENT_COMPARE":
		return true
	default:
		return false
	}
}

func isAcceptedChangeOutcome(outcome string) bool {
	switch outcome {
	case "accepted_change", "ACT_OUTCOME_KIND_ACCEPTED_CHANGE":
		return true
	default:
		return false
	}
}

// actOutcomeDelta maps an ActOutcomeKind label onto its ActOutcomeSignal
// contribution (ADR-000908 §Δ1). Unknown labels contribute 0.
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

// --- bucket + planner-version derivation ------------------------------------

// resolveBucketAndInputs derives the SurfaceScoreInputs from the accumulator,
// then picks the bucket and the honest per-entry planner version. It returns an
// error (unlike the old resolver shim) so a read failure stops the batch
// instead of silently degrading to v1.
func (p *Projector) resolveBucketAndInputs(
	ctx context.Context,
	ev *sovereign_db.KnowledgeEvent,
) (sovereignv1.SurfaceBucket, SurfaceScoreInputs, sovereignv1.SurfacePlannerVersion, error) {
	in, err := p.deriveSurfaceScoreInputs(ctx, ev)
	if err != nil {
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_UNSPECIFIED, in,
			sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V1, err
	}
	bucket := decideBucketV2(in)
	plannerVersion := plannerVersionForInputs(in)
	observeSurfaceBucketAssigned(plannerVersionMetricLabel(plannerVersion), bucketMetricLabel(bucket))
	return bucket, in, plannerVersion, nil
}

// hasV2Signal reports whether any real cross-source evidence applied to the
// entry — the honest "did v2 evidence actually fire" predicate behind the
// per-entry planner version (ADR-000938 / ADR-000939). Covers every signal that
// can produce a relation or a non-fallback bucket so the label cannot read v2
// for a pure v1 event-type placement.
func hasV2Signal(in SurfaceScoreInputs) bool {
	return in.TopicOverlapCount > 0 ||
		in.TagOverlapCount > 0 ||
		in.RecapClusterMomentum > 0 ||
		in.VersionDriftCount > 0 ||
		in.ContradictionCount > 0 ||
		in.HasAugurLink ||
		in.QuestionContinuationScore > 0 ||
		in.HasOpenInteraction ||
		in.RecentContinueActionCount > 0 ||
		in.CompareActionCount > 0 ||
		in.AcceptedChangeCount > 0
}

func plannerVersionForInputs(in SurfaceScoreInputs) sovereignv1.SurfacePlannerVersion {
	if hasV2Signal(in) {
		return sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V2
	}
	return sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V1
}

func plannerVersionMetricLabel(v sovereignv1.SurfacePlannerVersion) string {
	if v == sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V2 {
		return "v2"
	}
	return "v1"
}
