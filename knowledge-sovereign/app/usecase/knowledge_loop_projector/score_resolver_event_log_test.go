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
