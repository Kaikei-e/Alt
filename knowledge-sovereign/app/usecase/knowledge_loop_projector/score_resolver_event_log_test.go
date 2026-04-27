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
	}
	if !reflect.DeepEqual(resolverEventTypes, want) {
		t.Errorf("resolverEventTypes drifted from contract §6.4.1: got %v want %v",
			resolverEventTypes, want)
	}
}
