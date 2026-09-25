package knowledge_home_projector

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

// ── fakeRepo ──

// fakeRepo is an in-memory stand-in for the sovereign repository, mirroring
// knowledge_trail_projector's fakeRepo pattern. Each mutation method decodes
// the json.RawMessage into its wire-capture type and records it by key, so
// tests can assert on fold outcomes without a live database. Error injection
// fields let tests exercise the non-fatal side-effect paths (today_digest /
// recall_candidate / clear_supersede failures must not fail the batch — see
// alt-backend/app/job/knowledge_projector.go's "// Non-fatal" comments).
type fakeRepo struct {
	events     []sovereign_db.KnowledgeEvent
	checkpoint int64
	// checkpointAt is the stored row's updated_at — the witness the real
	// repository's compare-and-set matches on. Modelling it here is what makes
	// "the projector handed back the token it actually read" checkable: a
	// fabricated token has the wrong witness and is refused, exactly as
	// Postgres would refuse it.
	checkpointAt     time.Time
	checkpointExists bool
	// advances records every compare-and-set attempt, so a test can assert both
	// the value passed and that a rejected advance is not retried in a loop.
	advances []fakeAdvance
	// advanceRejected simulates another writer (an operator's rebuild, the
	// reproject swap RPC) having moved the row since the batch read it.
	advanceRejected bool
	listCalls       int

	homeItems        map[string]capturedHomeItem
	dismissed        map[string]capturedDismiss
	clearedSupersede map[string]int
	digests          map[string]capturedDigest
	recallCandidates map[string]capturedRecallCandidate
	urlPatches       map[string]capturedURLPatch
	snoozed          map[string]capturedSnooze
	recallDismissed  map[string]capturedRecallDismiss

	// activeProjectionVersion mirrors knowledge_projection_versions'
	// status='active' row. Defaults to version 1 so existing tests that
	// don't care about versioning keep working unchanged; set to nil to
	// simulate no active version row (must fail the batch loudly).
	activeProjectionVersion *sovereign_db.ProjectionVersion
	activeVersionErr        error

	// frontiers are handed out one per ReadSequenceGapFrontier call, the last
	// one repeating; each one's HoleOpen is filled in from events rather than
	// scripted. The default stands for a write transaction that never ends:
	// its id sits below the ceiling of the first sighting, so a hole in the
	// sequence is never mistaken for a burned one unless a test says so.
	frontiers []sovereign_db.SequenceGapFrontier
	// beforeGapFrontier runs inside the gap frontier read, standing for a
	// writer that commits after the batch was read and before the verdict.
	beforeGapFrontier func()

	// dismissMissingKeys mirrors the real repository's zero-rows-updated
	// outcome: item_keys listed here have no knowledge_home_items row at the
	// projector's version, so DismissKnowledgeHomeItem reports
	// sovereign_db.ErrDismissTargetNotFound exactly as Postgres would (see
	// driver/sovereign_db/repository.go's RowsAffected() == 0 branch).
	dismissMissingKeys map[string]bool

	todayDigestErr    error
	recallCandErr     error
	clearSupersedeErr error
	snoozeRecallErr   error
	dismissRecallErr  error
	dismissHomeErr    error
}

// fakeAdvance is one recorded compare-and-set attempt.
type fakeAdvance struct {
	From  sovereign_db.ProjectionCheckpoint
	ToSeq int64
}

// fakeCheckpointAt is the fake's initial stored updated_at. Fixed, never wall
// clock: it is compared for equality, not for recency.
var fakeCheckpointAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

var _ Repository = (*fakeRepo)(nil)

func newFakeRepo(events []sovereign_db.KnowledgeEvent) *fakeRepo {
	return &fakeRepo{
		events:                  events,
		checkpointAt:            fakeCheckpointAt,
		checkpointExists:        true,
		homeItems:               map[string]capturedHomeItem{},
		dismissed:               map[string]capturedDismiss{},
		clearedSupersede:        map[string]int{},
		digests:                 map[string]capturedDigest{},
		recallCandidates:        map[string]capturedRecallCandidate{},
		urlPatches:              map[string]capturedURLPatch{},
		snoozed:                 map[string]capturedSnooze{},
		recallDismissed:         map[string]capturedRecallDismiss{},
		dismissMissingKeys:      map[string]bool{},
		activeProjectionVersion: &sovereign_db.ProjectionVersion{Version: 1},
		frontiers:               []sovereign_db.SequenceGapFrontier{{Ceiling: 101, Xmin: 100}},
	}
}

func (f *fakeRepo) ReadProjectionCheckpointForAdvance(_ context.Context, _ string) (sovereign_db.ProjectionCheckpoint, error) {
	if !f.checkpointExists {
		return sovereign_db.ProjectionCheckpoint{}, nil
	}
	return sovereign_db.ProjectionCheckpoint{
		LastEventSeq: f.checkpoint,
		UpdatedAt:    f.checkpointAt,
		Exists:       true,
	}, nil
}

// AdvanceProjectionCheckpointIfUnchanged mirrors the driver's guarded UPDATE:
// it applies only when the token still describes the stored row, and every
// applied advance moves the witness, so a token read before it is dead.
func (f *fakeRepo) AdvanceProjectionCheckpointIfUnchanged(
	_ context.Context, _ string, from sovereign_db.ProjectionCheckpoint, toSeq int64,
) (bool, error) {
	f.advances = append(f.advances, fakeAdvance{From: from, ToSeq: toSeq})
	if f.advanceRejected {
		return false, nil
	}
	if from.Exists != f.checkpointExists || from.LastEventSeq != f.checkpoint || !from.UpdatedAt.Equal(f.checkpointAt) {
		return false, nil
	}
	f.checkpoint = toSeq
	f.checkpointAt = f.checkpointAt.Add(time.Second)
	f.checkpointExists = true
	return true, nil
}

func (f *fakeRepo) ListKnowledgeEventsSince(_ context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error) {
	f.listCalls++
	var out []sovereign_db.KnowledgeEvent
	for _, e := range f.events {
		if e.EventSeq > afterSeq {
			out = append(out, e)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (f *fakeRepo) ReadSequenceGapFrontier(_ context.Context, firstSeq, lastSeq int64) (sovereign_db.SequenceGapFrontier, error) {
	if f.beforeGapFrontier != nil {
		f.beforeGapFrontier()
	}
	next := f.frontiers[0]
	if len(f.frontiers) > 1 {
		f.frontiers = f.frontiers[1:]
	}
	// The real query answers both halves from one snapshot, so the fake reads
	// the run out of the same events the batch read came from — a scripted
	// "still empty" that the events contradict is a state the server cannot
	// produce.
	next.HoleOpen = true
	for _, evt := range f.events {
		if evt.EventSeq >= firstSeq && evt.EventSeq <= lastSeq {
			next.HoleOpen = false
		}
	}
	return next, nil
}

func (f *fakeRepo) GetActiveProjectionVersion(_ context.Context) (*sovereign_db.ProjectionVersion, error) {
	if f.activeVersionErr != nil {
		return nil, f.activeVersionErr
	}
	return f.activeProjectionVersion, nil
}

func (f *fakeRepo) UpsertKnowledgeHomeItem(_ context.Context, payload json.RawMessage) error {
	var w capturedHomeItem
	if err := json.Unmarshal(payload, &w); err != nil {
		return fmt.Errorf("fakeRepo.UpsertKnowledgeHomeItem: %w", err)
	}
	// Mirror sovereign_db.UpsertKnowledgeHomeItem's merge-safe COALESCE for
	// string fields: empty incoming title/url/summary must not wipe values
	// already folded from an earlier event (SummaryVersionCreated → delayed
	// ArticleCreated is the Trail blank-title failure mode).
	if existing, ok := f.homeItems[w.ItemKey]; ok {
		if w.Title == "" {
			w.Title = existing.Title
		}
		if w.URL == "" {
			w.URL = existing.URL
		}
		if w.SummaryExcerpt == "" {
			w.SummaryExcerpt = existing.SummaryExcerpt
		}
		// Mirror GREATEST latch: '' < missing < pending < ready (alphabetical).
		if existing.SummaryState > w.SummaryState {
			w.SummaryState = existing.SummaryState
		}
		if len(w.Tags) == 0 {
			w.Tags = existing.Tags
		}
	}
	f.homeItems[w.ItemKey] = w
	return nil
}

func (f *fakeRepo) DismissKnowledgeHomeItem(_ context.Context, payload json.RawMessage) error {
	if f.dismissHomeErr != nil {
		return f.dismissHomeErr
	}
	var w capturedDismiss
	if err := json.Unmarshal(payload, &w); err != nil {
		return fmt.Errorf("fakeRepo.DismissKnowledgeHomeItem: %w", err)
	}
	if f.dismissMissingKeys[w.ItemKey] {
		return sovereign_db.ErrDismissTargetNotFound
	}
	f.dismissed[w.ItemKey] = w
	return nil
}

func (f *fakeRepo) ClearSupersedeState(_ context.Context, payload json.RawMessage) error {
	if f.clearSupersedeErr != nil {
		return f.clearSupersedeErr
	}
	var w capturedClearSupersede
	if err := json.Unmarshal(payload, &w); err != nil {
		return fmt.Errorf("fakeRepo.ClearSupersedeState: %w", err)
	}
	f.clearedSupersede[w.ItemKey]++
	return nil
}

func (f *fakeRepo) UpsertTodayDigest(_ context.Context, payload json.RawMessage) error {
	var w capturedDigest
	if err := json.Unmarshal(payload, &w); err != nil {
		return fmt.Errorf("fakeRepo.UpsertTodayDigest: %w", err)
	}
	// Mirror sovereign_db.UpsertTodayDigest's producer-wiring guard: the
	// merge-safe UPSERT gates its additive counters on last_event_seq, so a
	// payload that omits the key — or sends the 0 that a BIGSERIAL never
	// issues — is refused before anything reaches the row rather than
	// silently falling back to the wall clock. A fake that accepted what the
	// driver refuses is how a fold that writes no last_event_seq stayed green
	// here while today_digest_view stopped being written in production.
	if w.LastEventSeq == nil {
		return fmt.Errorf("fakeRepo.UpsertTodayDigest: last_event_seq is required")
	}
	if *w.LastEventSeq <= 0 {
		return fmt.Errorf("fakeRepo.UpsertTodayDigest: last_event_seq must be a positive event_seq, got %d", *w.LastEventSeq)
	}
	if f.todayDigestErr != nil {
		return f.todayDigestErr
	}
	f.digests[w.UserID.String()] = w
	return nil
}

func (f *fakeRepo) UpsertRecallCandidate(_ context.Context, payload json.RawMessage) error {
	if f.recallCandErr != nil {
		return f.recallCandErr
	}
	var w capturedRecallCandidate
	if err := json.Unmarshal(payload, &w); err != nil {
		return fmt.Errorf("fakeRepo.UpsertRecallCandidate: %w", err)
	}
	f.recallCandidates[w.ItemKey] = w
	return nil
}

func (f *fakeRepo) PatchKnowledgeHomeItemURL(_ context.Context, payload json.RawMessage) error {
	var w capturedURLPatch
	if err := json.Unmarshal(payload, &w); err != nil {
		return fmt.Errorf("fakeRepo.PatchKnowledgeHomeItemURL: %w", err)
	}
	f.urlPatches[w.ItemKey] = w
	return nil
}

func (f *fakeRepo) SnoozeRecallCandidate(_ context.Context, payload json.RawMessage) error {
	if f.snoozeRecallErr != nil {
		return f.snoozeRecallErr
	}
	var w capturedSnooze
	if err := json.Unmarshal(payload, &w); err != nil {
		return fmt.Errorf("fakeRepo.SnoozeRecallCandidate: %w", err)
	}
	f.snoozed[w.ItemKey] = w
	return nil
}

func (f *fakeRepo) DismissRecallCandidate(_ context.Context, payload json.RawMessage) error {
	if f.dismissRecallErr != nil {
		return f.dismissRecallErr
	}
	var w capturedRecallDismiss
	if err := json.Unmarshal(payload, &w); err != nil {
		return fmt.Errorf("fakeRepo.DismissRecallCandidate: %w", err)
	}
	f.recallDismissed[w.ItemKey] = w
	return nil
}

// ── event builders ──

func userPtr() *uuid.UUID { u := uuid.New(); return &u }

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func homeEvent(seq int64, eventType, aggregateID string, occurredAt time.Time, tenantID uuid.UUID, userID *uuid.UUID, payload json.RawMessage) sovereign_db.KnowledgeEvent {
	return sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      seq,
		OccurredAt:    occurredAt,
		TenantID:      tenantID,
		UserID:        userID,
		EventType:     eventType,
		AggregateType: "article",
		AggregateID:   aggregateID,
		DedupeKey:     fmt.Sprintf("%s:%d", eventType, seq),
		Payload:       payload,
	}
}
