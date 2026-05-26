package knowledge_loop_projector

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// fakeEventLogLookup lets a test inject a fixed slice of evidence events.
// It also remembers the last query parameters so we can assert the
// resolver bound user_id physically.
type fakeEventLogLookup struct {
	events         []sovereign_db.KnowledgeEvent
	lastUserID     uuid.UUID
	lastEventTypes []string
	lastSince      time.Time
	lastUntil      time.Time
}

func (f *fakeEventLogLookup) ListKnowledgeEventsForUserInWindow(
	_ context.Context,
	userID uuid.UUID,
	eventTypes []string,
	since, until time.Time,
	_ int,
) ([]sovereign_db.KnowledgeEvent, error) {
	f.lastUserID = userID
	f.lastEventTypes = append([]string(nil), eventTypes...)
	f.lastSince = since
	f.lastUntil = until
	return f.events, nil
}

func ev(eventType string, occurredAt time.Time, userID uuid.UUID, payload map[string]any) sovereign_db.KnowledgeEvent {
	raw, _ := json.Marshal(payload)
	uid := userID
	return sovereign_db.KnowledgeEvent{
		EventType:  eventType,
		OccurredAt: occurredAt,
		UserID:     &uid,
		Payload:    raw,
	}
}

func TestEventLogResolver_BindsUserAndWindowFromEvent(t *testing.T) {
	t.Parallel()

	uid := uuid.New()
	occurred := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	lookup := &fakeEventLogLookup{}
	r := NewEventLogSurfaceScoreResolver(lookup)
	target := ev(EventSummaryVersionCreated, occurred, uid, map[string]any{
		"article_id": "art-1",
	})
	r.Resolve(context.Background(), &target)

	if lookup.lastUserID != uid {
		t.Errorf("user_id not bound: got %v want %v", lookup.lastUserID, uid)
	}
	if !lookup.lastUntil.Equal(occurred) {
		t.Errorf("until not bound to event.occurred_at: got %v want %v",
			lookup.lastUntil, occurred)
	}
	if !lookup.lastSince.Equal(occurred.Add(-scoreWindow)) {
		t.Errorf("since not bound to event.occurred_at - 7d: got %v",
			lookup.lastSince)
	}
}

func TestEventLogResolver_VersionDriftFromSummaryEvents(t *testing.T) {
	t.Parallel()

	uid := uuid.New()
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	lookup := &fakeEventLogLookup{events: []sovereign_db.KnowledgeEvent{
		ev(EventSummaryVersionCreated, now.Add(-3*time.Hour), uid, map[string]any{"article_id": "art-1"}),
		ev(EventSummarySuperseded, now.Add(-1*time.Hour), uid, map[string]any{"article_id": "art-1"}),
		ev(EventSummaryVersionCreated, now.Add(-2*time.Hour), uid, map[string]any{"article_id": "art-2"}),
	}}
	r := NewEventLogSurfaceScoreResolver(lookup)
	target := ev(EventSummaryVersionCreated, now, uid, map[string]any{"article_id": "art-1"})
	out := r.Resolve(context.Background(), &target)

	if out.VersionDriftCount != 2 {
		t.Errorf("VersionDriftCount = %d; want 2", out.VersionDriftCount)
	}
}

func TestEventLogResolver_AugurLinkPromotesContinue(t *testing.T) {
	t.Parallel()

	uid := uuid.New()
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	lookup := &fakeEventLogLookup{events: []sovereign_db.KnowledgeEvent{
		ev(EventAugurConversationLinked, now.Add(-1*time.Hour), uid, map[string]any{
			"entry_key":       "entry:art-1",
			"conversation_id": "conv-1",
		}),
	}}
	r := NewEventLogSurfaceScoreResolver(lookup)
	target := ev(EventSummaryVersionCreated, now, uid, map[string]any{
		"article_id": "art-1",
		"entry_key":  "entry:art-1",
	})
	out := r.Resolve(context.Background(), &target)

	if !out.HasAugurLink {
		t.Error("HasAugurLink = false; want true")
	}
	got := decideBucketV2(out)
	if got != sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE {
		t.Errorf("decideBucketV2 = %v; want CONTINUE", got)
	}
}

func TestEventLogResolver_TopicOverlapFromRecap(t *testing.T) {
	t.Parallel()

	uid := uuid.New()
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	lookup := &fakeEventLogLookup{events: []sovereign_db.KnowledgeEvent{
		ev(EventRecapTopicSnapshotted, now.Add(-2*time.Hour), uid, map[string]any{
			"top_terms": []any{"finance", "energy", "markets"},
		}),
	}}
	r := NewEventLogSurfaceScoreResolver(lookup)
	target := ev(EventSummaryVersionCreated, now, uid, map[string]any{
		"article_id": "art-1",
		"tags":       []any{"Energy", "earnings"},
	})
	out := r.Resolve(context.Background(), &target)

	if out.TopicOverlapCount != 1 {
		t.Errorf("TopicOverlapCount = %d; want 1 (energy overlap)", out.TopicOverlapCount)
	}
}

// Recap CTA wiring: when the resolver sees a RecapTopicSnapshotted event whose
// top_terms overlap with the entry's tags, it must remember the snapshot id of
// the most-recent matching event so the projector can seed an act_target with
// target_type=recap pointing at /recap/topic/<id>.
func TestEventLogResolver_RecordsMostRecentMatchingRecapTopicSnapshotID(t *testing.T) {
	t.Parallel()

	uid := uuid.New()
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	lookup := &fakeEventLogLookup{events: []sovereign_db.KnowledgeEvent{
		// Older snapshot — overlap, but should be shadowed by the newer one.
		ev(EventRecapTopicSnapshotted, now.Add(-3*time.Hour), uid, map[string]any{
			"recap_topic_snapshot_id": "11111111-1111-4111-8111-111111111111",
			"top_terms":               []any{"energy"},
		}),
		// Newer snapshot — also overlap; this is the one we should expose.
		ev(EventRecapTopicSnapshotted, now.Add(-1*time.Hour), uid, map[string]any{
			"recap_topic_snapshot_id": "22222222-2222-4222-8222-222222222222",
			"top_terms":               []any{"energy", "markets"},
		}),
		// Newest snapshot — but no overlap; must NOT be picked.
		ev(EventRecapTopicSnapshotted, now.Add(-30*time.Minute), uid, map[string]any{
			"recap_topic_snapshot_id": "33333333-3333-4333-8333-333333333333",
			"top_terms":               []any{"medicine", "biotech"},
		}),
	}}
	r := NewEventLogSurfaceScoreResolver(lookup)
	target := ev(EventSummaryVersionCreated, now, uid, map[string]any{
		"article_id": "art-1",
		"tags":       []any{"energy", "earnings"},
	})
	out := r.Resolve(context.Background(), &target)

	if out.TopicOverlapCount != 2 {
		t.Errorf("TopicOverlapCount = %d; want 2", out.TopicOverlapCount)
	}
	if out.RecapTopicSnapshotID != "22222222-2222-4222-8222-222222222222" {
		t.Errorf("RecapTopicSnapshotID = %q; want the most-recent overlapping snapshot",
			out.RecapTopicSnapshotID)
	}
}

func TestEventLogResolver_RejectsNonUuidRecapTopicSnapshotID(t *testing.T) {
	t.Parallel()

	uid := uuid.New()
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	lookup := &fakeEventLogLookup{events: []sovereign_db.KnowledgeEvent{
		ev(EventRecapTopicSnapshotted, now.Add(-1*time.Hour), uid, map[string]any{
			// Not a UUID — open-redirect / path-traversal vector if forwarded
			// to /recap/topic/<id>. The resolver must drop it.
			"recap_topic_snapshot_id": "../etc/passwd",
			"top_terms":               []any{"energy"},
		}),
	}}
	r := NewEventLogSurfaceScoreResolver(lookup)
	target := ev(EventSummaryVersionCreated, now, uid, map[string]any{
		"article_id": "art-1",
		"tags":       []any{"energy"},
	})
	out := r.Resolve(context.Background(), &target)

	if out.RecapTopicSnapshotID != "" {
		t.Errorf("RecapTopicSnapshotID = %q; non-UUID input must be rejected", out.RecapTopicSnapshotID)
	}
	// Overlap should still count; only the snapshot id is dropped.
	if out.TopicOverlapCount != 1 {
		t.Errorf("TopicOverlapCount = %d; want 1", out.TopicOverlapCount)
	}
}

func TestEventLogResolver_PopulatesContradictionCountFromSupersededChain(t *testing.T) {
	t.Parallel()
	uid := uuid.New()
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	lookup := &fakeEventLogLookup{events: []sovereign_db.KnowledgeEvent{
		ev(EventSummarySuperseded, now.Add(-2*time.Hour), uid, map[string]any{"article_id": "art-1"}),
		ev(EventSummarySuperseded, now.Add(-1*time.Hour), uid, map[string]any{"article_id": "art-1"}),
		ev(EventSummarySuperseded, now.Add(-30*time.Minute), uid, map[string]any{"article_id": "art-other"}),
	}}
	r := NewEventLogSurfaceScoreResolver(lookup)
	target := ev(EventSummaryVersionCreated, now, uid, map[string]any{"article_id": "art-1"})
	out := r.Resolve(context.Background(), &target)
	if out.ContradictionCount != 2 {
		t.Errorf("ContradictionCount = %d; want 2 (only art-1 supersedes count)", out.ContradictionCount)
	}
	if out.VersionDriftCount != 2 {
		t.Errorf("VersionDriftCount = %d; want 2", out.VersionDriftCount)
	}
}

func TestEventLogResolver_PopulatesQuestionContinuationFromAugur(t *testing.T) {
	t.Parallel()
	uid := uuid.New()
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	lookup := &fakeEventLogLookup{events: []sovereign_db.KnowledgeEvent{
		ev(EventAugurConversationLinked, now.Add(-2*time.Hour), uid, map[string]any{"entry_key": "entry:art-1"}),
		ev(EventAugurConversationLinked, now.Add(-1*time.Hour), uid, map[string]any{"entry_key": "entry:art-1"}),
	}}
	r := NewEventLogSurfaceScoreResolver(lookup)
	target := ev(EventSummaryVersionCreated, now, uid, map[string]any{
		"article_id": "art-1",
		"entry_key":  "entry:art-1",
	})
	out := r.Resolve(context.Background(), &target)
	if !out.HasAugurLink {
		t.Error("HasAugurLink = false; want true")
	}
	if out.QuestionContinuationScore != 2 {
		t.Errorf("QuestionContinuationScore = %d; want 2", out.QuestionContinuationScore)
	}
}

func TestEventLogResolver_PopulatesRecapClusterMomentumAlongsideTopicOverlap(t *testing.T) {
	t.Parallel()
	uid := uuid.New()
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	lookup := &fakeEventLogLookup{events: []sovereign_db.KnowledgeEvent{
		ev(EventRecapTopicSnapshotted, now.Add(-2*time.Hour), uid, map[string]any{
			"top_terms": []any{"finance", "energy"},
		}),
		ev(EventRecapTopicSnapshotted, now.Add(-1*time.Hour), uid, map[string]any{
			"top_terms": []any{"energy", "markets"},
		}),
	}}
	r := NewEventLogSurfaceScoreResolver(lookup)
	target := ev(EventSummaryVersionCreated, now, uid, map[string]any{
		"article_id": "art-1",
		"tags":       []any{"energy"},
	})
	out := r.Resolve(context.Background(), &target)
	if out.TopicOverlapCount != 2 || out.RecapClusterMomentum != 2 {
		t.Errorf("expected TopicOverlap=RecapMomentum=2, got %d / %d",
			out.TopicOverlapCount, out.RecapClusterMomentum)
	}
}

func TestEventLogResolver_PopulatesStalenessScoreFromSourceObservedAt(t *testing.T) {
	t.Parallel()
	uid := uuid.New()
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	target := ev(EventSummaryVersionCreated, now, uid, map[string]any{
		"article_id":   "art-1",
		"published_at": now.Add(-10 * 24 * time.Hour).Format(time.RFC3339),
	})
	r := NewEventLogSurfaceScoreResolver(&fakeEventLogLookup{})
	out := r.Resolve(context.Background(), &target)
	// 10d ago → bucket 3 (aging, 7d ≤ gap < 30d)
	if out.StalenessScore != 3 {
		t.Errorf("StalenessScore = %d; want 3 for 10d-old source", out.StalenessScore)
	}
}

func TestEventLogResolver_StalenessScoreZeroWhenSourceObservedAtMissing(t *testing.T) {
	t.Parallel()
	uid := uuid.New()
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	target := ev(EventSummaryVersionCreated, now, uid, map[string]any{"article_id": "art-1"})
	r := NewEventLogSurfaceScoreResolver(&fakeEventLogLookup{})
	out := r.Resolve(context.Background(), &target)
	if out.StalenessScore != 0 {
		t.Errorf("StalenessScore = %d; want 0 when no source_observed_at on payload", out.StalenessScore)
	}
}

func TestEventLogResolver_OpenInteractionFromHomeItemOpened(t *testing.T) {
	t.Parallel()

	uid := uuid.New()
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	lookup := &fakeEventLogLookup{events: []sovereign_db.KnowledgeEvent{
		ev(EventHomeItemOpened, now.Add(-30*time.Minute), uid, map[string]any{
			"entry_key": "entry:art-1",
		}),
	}}
	r := NewEventLogSurfaceScoreResolver(lookup)
	target := ev(EventSummaryVersionCreated, now, uid, map[string]any{
		"article_id": "art-1",
		"entry_key":  "entry:art-1",
	})
	out := r.Resolve(context.Background(), &target)

	if !out.HasOpenInteraction {
		t.Error("HasOpenInteraction = false; want true")
	}
}

func TestEventLogResolver_F001CrossUserMismatchReturnsEmpty(t *testing.T) {
	t.Parallel()

	uid := uuid.New()
	otherUID := uuid.New()
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	other := otherUID
	lookup := &fakeEventLogLookup{events: []sovereign_db.KnowledgeEvent{
		// Lookup buggy: returns an event for a different user. Resolver MUST
		// detect this and return empty inputs (and bump the F-001 counter).
		{
			EventType:  EventAugurConversationLinked,
			OccurredAt: now.Add(-1 * time.Hour),
			UserID:     &other,
			Payload:    json.RawMessage(`{"entry_key":"entry:art-1"}`),
		},
	}}
	r := NewEventLogSurfaceScoreResolver(lookup)
	target := ev(EventSummaryVersionCreated, now, uid, map[string]any{
		"entry_key": "entry:art-1",
	})
	out := r.Resolve(context.Background(), &target)

	if out.HasAugurLink || out.TopicOverlapCount > 0 || out.VersionDriftCount > 0 {
		t.Errorf("cross-user evidence leaked: %+v", out)
	}
}

func TestEventLogResolver_NoUserIDOnEventReturnsEmpty(t *testing.T) {
	t.Parallel()

	lookup := &fakeEventLogLookup{}
	r := NewEventLogSurfaceScoreResolver(lookup)
	systemEvent := sovereign_db.KnowledgeEvent{
		EventType:  EventArticleCreated,
		OccurredAt: time.Now(),
		UserID:     nil,
	}
	out := r.Resolve(context.Background(), &systemEvent)

	if out.HasAugurLink || out.TopicOverlapCount > 0 || out.VersionDriftCount > 0 {
		t.Errorf("system event should resolve to empty inputs: %+v", out)
	}
}

func TestEventLogResolver_AllowlistMatchesContract(t *testing.T) {
	t.Parallel()

	want := []string{
		EventSummaryVersionCreated,
		EventSummarySuperseded,
		EventHomeItemOpened,
		EventRecapTopicSnapshotted,
		EventAugurConversationLinked,
		// Phase 2: knowledge_loop.acted.v1 with continue_flag=true is a
		// Continue signal — the resolver gates on continue_flag inside the
		// loop so Snooze (false) does not promote.
		EventKnowledgeLoopActed,
		// ADR-000908 §Δ1: knowledge_loop.act_outcome.v1 events feed the
		// ActOutcomeSignal aggregation. Immediate engaged / deep_engagement
		// signals come from alt-backend view trackers; the 7-day
		// no_engagement fallback is emitted by act_outcome_cron.
		EventKnowledgeLoopActOutcome,
	}
	if !reflect.DeepEqual(resolverEventTypes, want) {
		t.Errorf("resolverEventTypes drifted from contract §6.4.1 / ADR-000908: got %v want %v",
			resolverEventTypes, want)
	}
}

// ADR-000908 §Δ1: ActOutcome events are aggregated into ActOutcomeSignal
// using a fixed scoring table. The match is entry-keyed (mirrors the
// RecentContinueActionCount pattern) so cross-entry outcomes do not bleed.
//
//	engaged          = +1
//	deep_engagement  = +2
//	accepted_change  = +1
//	stale_save       = -1
//	no_engagement    = -2
func TestEventLogResolver_AggregatesActOutcomeSignal(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	tenantID := uuid.New()
	entryKey := "article:42"
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

	mkOutcome := func(seq int64, entry, outcome string, when time.Time) sovereign_db.KnowledgeEvent {
		body, _ := json.Marshal(map[string]any{
			"entry_key": entry,
			"outcome":   outcome,
		})
		uid := userID
		return sovereign_db.KnowledgeEvent{
			EventSeq:      seq,
			OccurredAt:    when,
			TenantID:      tenantID,
			UserID:        &uid,
			EventType:     EventKnowledgeLoopActOutcome,
			AggregateType: "knowledge_loop_entry",
			AggregateID:   entry,
			Payload:       body,
		}
	}

	lookup := &fakeEventLogLookup{events: []sovereign_db.KnowledgeEvent{
		mkOutcome(1, entryKey, "engaged", now.Add(-3*time.Hour)),                   // +1
		mkOutcome(2, entryKey, "deep_engagement", now.Add(-2*time.Hour)),           // +2
		mkOutcome(3, entryKey, "no_engagement", now.Add(-1*time.Hour)),             // -2
		mkOutcome(4, "article:other", "deep_engagement", now.Add(-30*time.Minute)), // must NOT count
		mkOutcome(5, entryKey, "stale_save", now.Add(-15*time.Minute)),             // -1
	}}
	r := NewEventLogSurfaceScoreResolver(lookup)

	uid := userID
	target := sovereign_db.KnowledgeEvent{
		EventSeq:   100,
		OccurredAt: now,
		TenantID:   tenantID,
		UserID:     &uid,
		EventType:  EventSummaryVersionCreated,
		Payload:    mustJSON(t, map[string]any{"entry_key": entryKey, "article_id": "art-1"}),
	}
	out := r.Resolve(context.Background(), &target)

	// 1 + 2 - 2 - 1 = 0 (cross-entry deep_engagement excluded)
	if out.ActOutcomeSignal != 0 {
		t.Errorf("ActOutcomeSignal = %d; want 0 (1+2-2-1; cross-entry excluded)", out.ActOutcomeSignal)
	}
}

// Unknown outcome values must not crash the aggregator and must not affect
// the signal. Defense against payload schema drift.
func TestEventLogResolver_UnknownOutcomeIsZeroDelta(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	tenantID := uuid.New()
	entryKey := "article:42"
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

	uid := userID
	lookup := &fakeEventLogLookup{events: []sovereign_db.KnowledgeEvent{{
		EventSeq:      1,
		OccurredAt:    now.Add(-1 * time.Hour),
		TenantID:      tenantID,
		UserID:        &uid,
		EventType:     EventKnowledgeLoopActOutcome,
		AggregateType: "knowledge_loop_entry",
		AggregateID:   entryKey,
		Payload: mustJSON(t, map[string]any{
			"entry_key": entryKey,
			"outcome":   "totally_made_up",
		}),
	}}}
	r := NewEventLogSurfaceScoreResolver(lookup)

	uid2 := userID
	target := sovereign_db.KnowledgeEvent{
		EventSeq:   100,
		OccurredAt: now,
		TenantID:   tenantID,
		UserID:     &uid2,
		EventType:  EventSummaryVersionCreated,
		Payload:    mustJSON(t, map[string]any{"entry_key": entryKey}),
	}
	out := r.Resolve(context.Background(), &target)
	if out.ActOutcomeSignal != 0 {
		t.Errorf("unknown outcome must contribute 0; got %d", out.ActOutcomeSignal)
	}
}

// TestEventLogResolver_KnowledgeLoopActedContinueFlagSemantics pins the
// Phase 2 Continue signal: only knowledge_loop.acted.v1 events with
// continue_flag=true tally toward RecentContinueActionCount, and the match
// is entry-keyed so cross-entry behaviour cannot bleed.
func TestEventLogResolver_KnowledgeLoopActedContinueFlagSemantics(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	tenantID := uuid.New()
	entryKey := "article:42"
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

	mkActed := func(seq int64, entry string, continueFlag bool, when time.Time) sovereign_db.KnowledgeEvent {
		body, _ := json.Marshal(map[string]any{
			"entry_key":     entry,
			"acted_intent":  "DECISION_INTENT_OPEN",
			"continue_flag": continueFlag,
		})
		uid := userID
		return sovereign_db.KnowledgeEvent{
			EventSeq:      seq,
			OccurredAt:    when,
			TenantID:      tenantID,
			UserID:        &uid,
			EventType:     EventKnowledgeLoopActed,
			AggregateType: "loop_session",
			AggregateID:   entry,
			Payload:       body,
		}
	}

	lookup := &fakeEventLogLookup{events: []sovereign_db.KnowledgeEvent{
		mkActed(1, entryKey, true, now.Add(-2*time.Hour)),
		mkActed(2, entryKey, false, now.Add(-1*time.Hour)),          // snooze — must NOT count
		mkActed(3, "article:other", true, now.Add(-30*time.Minute)), // other entry — must NOT count
		mkActed(4, entryKey, true, now.Add(-15*time.Minute)),
	}}
	r := NewEventLogSurfaceScoreResolver(lookup)

	uid := userID
	thisEvent := &sovereign_db.KnowledgeEvent{
		EventSeq:   100,
		OccurredAt: now,
		TenantID:   tenantID,
		UserID:     &uid,
		EventType:  EventKnowledgeLoopActed,
		Payload:    mustJSON(t, map[string]any{"entry_key": entryKey, "continue_flag": true}),
	}

	out := r.Resolve(context.Background(), thisEvent)
	if out.RecentContinueActionCount != 2 {
		t.Errorf("RecentContinueActionCount: want 2 (continue_flag=true on this entry), got %d", out.RecentContinueActionCount)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// TestEventLogResolver_PinsArticleIDForAugurEvent — when the projecting event
// is augur.conversation_linked.v1 (payload has entry_key but no article_id),
// the resolver must lift the article_id from a prior SummaryVersionCreated
// event on the same entry_key. seedActTargets reads this pin so the article
// act_target survives the augur event, keeping "Open Article" clickable after
// Ask. Reproject-safe: the event log is the only source.
func TestEventLogResolver_PinsArticleIDForAugurEvent(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	tenantID := uuid.New()
	articleID := "art-pinned-by-resolver"
	entryKey := "entry:" + articleID
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

	uid := userID
	lookup := &fakeEventLogLookup{events: []sovereign_db.KnowledgeEvent{
		// Prior SummaryVersionCreated names the article; happens minutes
		// before the augur link. The resolver must find this and pin it.
		{
			EventSeq:      1,
			OccurredAt:    now.Add(-2 * time.Hour),
			TenantID:      tenantID,
			UserID:        &uid,
			EventType:     EventSummaryVersionCreated,
			AggregateType: "article",
			AggregateID:   articleID,
			Payload: mustJSON(t, map[string]any{
				"article_id": articleID,
				"entry_key":  entryKey,
			}),
		},
	}}

	r := NewEventLogSurfaceScoreResolver(lookup)
	thisEvent := &sovereign_db.KnowledgeEvent{
		EventSeq:      100,
		OccurredAt:    now,
		TenantID:      tenantID,
		UserID:        &uid,
		EventType:     EventAugurConversationLinked,
		AggregateType: "augur_conversation",
		AggregateID:   "conv-1",
		Payload: mustJSON(t, map[string]any{
			"entry_key":       entryKey,
			"conversation_id": "conv-1",
		}),
	}

	out := r.Resolve(context.Background(), thisEvent)
	if out.ArticleID != articleID {
		t.Errorf("ArticleID: want %q (pinned from prior SummaryVersionCreated), got %q", articleID, out.ArticleID)
	}
}

// TestEventLogResolver_DoesNotPinArticleIDAcrossEntries — confirm the pin is
// entry-keyed: a SummaryVersionCreated on a *different* entry must not leak
// its article_id into this entry's resolution. F-001-style isolation.
func TestEventLogResolver_DoesNotPinArticleIDAcrossEntries(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	tenantID := uuid.New()
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

	uid := userID
	lookup := &fakeEventLogLookup{events: []sovereign_db.KnowledgeEvent{
		{
			EventSeq:      1,
			OccurredAt:    now.Add(-2 * time.Hour),
			TenantID:      tenantID,
			UserID:        &uid,
			EventType:     EventSummaryVersionCreated,
			AggregateType: "article",
			AggregateID:   "art-other",
			Payload: mustJSON(t, map[string]any{
				"article_id": "art-other",
				"entry_key":  "entry:art-other",
			}),
		},
	}}

	r := NewEventLogSurfaceScoreResolver(lookup)
	thisEvent := &sovereign_db.KnowledgeEvent{
		EventSeq:      100,
		OccurredAt:    now,
		TenantID:      tenantID,
		UserID:        &uid,
		EventType:     EventAugurConversationLinked,
		AggregateType: "augur_conversation",
		AggregateID:   "conv-1",
		Payload: mustJSON(t, map[string]any{
			"entry_key":       "entry:art-this",
			"conversation_id": "conv-1",
		}),
	}

	out := r.Resolve(context.Background(), thisEvent)
	if out.ArticleID != "" {
		t.Errorf("ArticleID: want \"\" (no match for this entry_key), got %q", out.ArticleID)
	}
}
