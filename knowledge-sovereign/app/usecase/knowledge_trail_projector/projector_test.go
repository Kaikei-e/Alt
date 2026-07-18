package knowledge_trail_projector

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/usecase/trail_planner"
)

// fakeRepo is an in-memory stand-in for the sovereign repository. It records
// upserts keyed by footprint_key / branch_key so the test can assert reproject
// determinism and the untyped-branch rejection.
type fakeRepo struct {
	events     []sovereign_db.KnowledgeEvent
	checkpoint int64
	upserts    map[string]sovereign_db.TrailFootprint
	branches   map[string]sovereign_db.TrailBranch
	states     map[string]string
	outcomes   map[string]sovereign_db.TrailActOutcome
}

func newFakeRepo(events []sovereign_db.KnowledgeEvent) *fakeRepo {
	return &fakeRepo{
		events:   events,
		upserts:  map[string]sovereign_db.TrailFootprint{},
		branches: map[string]sovereign_db.TrailBranch{},
		states:   map[string]string{},
		outcomes: map[string]sovereign_db.TrailActOutcome{},
	}
}

func (f *fakeRepo) InsertTrailActOutcome(_ context.Context, o sovereign_db.TrailActOutcome, _ int) error {
	// Insert-only, first write wins — mirrors ON CONFLICT DO NOTHING.
	if _, exists := f.outcomes[o.OutcomeKey]; !exists {
		f.outcomes[o.OutcomeKey] = o
	}
	return nil
}

func (f *fakeRepo) UpsertTrailBranch(_ context.Context, _, _ uuid.UUID, b sovereign_db.TrailBranch, _ time.Time, _ int) error {
	f.branches[b.BranchKey] = b
	f.states[b.BranchKey] = "open"
	return nil
}

func (f *fakeRepo) SetTrailBranchState(_ context.Context, _ uuid.UUID, branchKey, state string) error {
	f.states[branchKey] = state
	return nil
}

func resolvedEvent(seq int64, payload trail_planner.BranchResolvedPayload, user *uuid.UUID) sovereign_db.KnowledgeEvent {
	body, _ := json.Marshal(payload)
	return sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      seq,
		OccurredAt:    time.Now().UTC(),
		TenantID:      uuid.New(),
		UserID:        user,
		EventType:     trail_planner.EventTrailBranchResolved,
		AggregateType: "trail_branch",
		AggregateID:   payload.BranchKey,
		Payload:       body,
	}
}

func branchEvent(seq int64, payload trail_planner.BranchProposedPayload, user *uuid.UUID) sovereign_db.KnowledgeEvent {
	body, _ := json.Marshal(payload)
	return sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      seq,
		OccurredAt:    time.Now().UTC(),
		TenantID:      uuid.New(),
		UserID:        user,
		EventType:     trail_planner.EventTrailBranchProposed,
		AggregateType: "trail_branch",
		AggregateID:   payload.BranchKey,
		Payload:       body,
	}
}

func (f *fakeRepo) GetProjectionCheckpoint(_ context.Context, _ string) (int64, error) {
	return f.checkpoint, nil
}
func (f *fakeRepo) UpdateProjectionCheckpoint(_ context.Context, _ string, lastSeq int64) error {
	f.checkpoint = lastSeq
	return nil
}
func (f *fakeRepo) ListKnowledgeEventsSince(_ context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error) {
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
func (f *fakeRepo) UpsertTrailFootprint(_ context.Context, fp sovereign_db.TrailFootprint, _ int) error {
	f.upserts[fp.FootprintKey] = fp
	return nil
}

func userPtr() *uuid.UUID { u := uuid.New(); return &u }

func actEvent(seq int64, eventType, itemKey, dedupe string, at time.Time, user *uuid.UUID) sovereign_db.KnowledgeEvent {
	return sovereign_db.KnowledgeEvent{
		EventID:     uuid.New(),
		EventSeq:    seq,
		OccurredAt:  at,
		TenantID:    uuid.New(),
		UserID:      user,
		EventType:   eventType,
		AggregateID: itemKey,
		DedupeKey:   dedupe,
	}
}

func TestProjector_FoldsActEventsToFootprints(t *testing.T) {
	user := userPtr()
	base := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	events := []sovereign_db.KnowledgeEvent{
		actEvent(1, "HomeItemOpened", "article:a", "open:a", base, user),
		actEvent(2, "HomeItemAsked", "article:a", "ask:a", base.Add(time.Minute), user),
		actEvent(3, "SummaryVersionCreated", "article:a", "sv:a", base.Add(2*time.Minute), user), // non-act → skipped
		actEvent(4, "HomeItemListened", "article:b", "listen:b", base.Add(3*time.Minute), user),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{BatchSize: 500, MaxBatchesPerTick: 4})

	require.NoError(t, p.RunBatch(context.Background()))

	assert.Len(t, repo.upserts, 3, "3 act events become footprints; SummaryVersionCreated is skipped")
	assert.Equal(t, "read", repo.upserts["open:a"].Verb)
	assert.Equal(t, "asked", repo.upserts["ask:a"].Verb)
	assert.Equal(t, "listened", repo.upserts["listen:b"].Verb)
	assert.Equal(t, int64(4), repo.checkpoint, "checkpoint advances to the max seq in the batch")
}

func TestProjector_SkipsSystemEventsWithoutUser(t *testing.T) {
	events := []sovereign_db.KnowledgeEvent{
		actEvent(1, "HomeItemOpened", "article:a", "open:a", time.Now().UTC(), nil), // nil user → skipped
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))
	assert.Empty(t, repo.upserts, "events without a user_id do not produce footprints")
	assert.Equal(t, int64(1), repo.checkpoint, "checkpoint still advances past skipped events")
}

func TestProjector_ReprojectIsDeterministic(t *testing.T) {
	user := userPtr()
	base := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	proposed := validBranchPayload()
	// The log spans the full Trail vocabulary: footprints (incl. a historical
	// loop.acted event), a branch proposal, and a branch resolution. Reproject
	// safety must hold across all three read models, not just the spine.
	events := []sovereign_db.KnowledgeEvent{
		actEvent(1, "HomeItemOpened", "article:a", "open:a", base, user),
		actEvent(2, "knowledge_loop.acted.v1", "article:c", "acted:c", base.Add(time.Minute), user),
		branchEvent(3, proposed, user),
		resolvedEvent(4, trail_planner.BranchResolvedPayload{BranchKey: proposed.BranchKey, Resolution: "taken"}, user),
	}

	first := newFakeRepo(events)
	require.NoError(t, NewProjector(first, nil, Config{}).RunBatch(context.Background()))

	// Re-run from a fresh checkpoint over the same log: identical read models.
	second := newFakeRepo(events)
	require.NoError(t, NewProjector(second, nil, Config{}).RunBatch(context.Background()))

	assert.Equal(t, first.upserts, second.upserts,
		"replaying the same event log must reproduce an identical spine (reproject-safe)")
	assert.Equal(t, first.branches, second.branches,
		"branches must reproject identically from the same log")
	assert.Equal(t, first.states, second.states,
		"branch resolution states must reproject identically")
	assert.Equal(t, "read", first.upserts["acted:c"].Verb,
		"historical knowledge_loop.acted.v1 projects as a read footprint")
}

func validBranchPayload() trail_planner.BranchProposedPayload {
	return trail_planner.BranchProposedPayload{
		BranchKey:     "cluster:u:article:z",
		AnchorItemKey: "article:a",
		RelationKind:  "cluster",
		Why:           "Joins a topic you follow — shares rust.",
		EvidenceRefs:  []trail_planner.EvidenceRef{{RefID: "rust", Label: "rust", Kind: "tag"}},
		Confidence:    "plausible",
		TargetItemKey: "article:z",
		TargetTitle:   "Async Rust",
	}
}

func TestProjector_FoldsValidBranch(t *testing.T) {
	user := userPtr()
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{branchEvent(1, validBranchPayload(), user)})
	require.NoError(t, NewProjector(repo, nil, Config{}).RunBatch(context.Background()))

	require.Len(t, repo.branches, 1)
	b := repo.branches["cluster:u:article:z"]
	assert.Equal(t, "cluster", b.RelationKind)
	assert.NotEmpty(t, b.Why)
	assert.Len(t, b.EvidenceRefs, 1)
	assert.Equal(t, "plausible", b.Confidence)
}

func TestProjector_FoldsBranchResolution(t *testing.T) {
	user := userPtr()
	proposed := validBranchPayload()
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		branchEvent(1, proposed, user),
		resolvedEvent(2, trail_planner.BranchResolvedPayload{BranchKey: proposed.BranchKey, Resolution: "taken"}, user),
	})
	require.NoError(t, NewProjector(repo, nil, Config{}).RunBatch(context.Background()))

	assert.Equal(t, "taken", repo.states[proposed.BranchKey],
		"branch_resolved transitions the branch out of the open set (trail closure)")
}

func TestProjector_RejectsInvalidResolution(t *testing.T) {
	user := userPtr()
	proposed := validBranchPayload()
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		branchEvent(1, proposed, user),
		resolvedEvent(2, trail_planner.BranchResolvedPayload{BranchKey: proposed.BranchKey, Resolution: "wat"}, user),
	})
	require.NoError(t, NewProjector(repo, nil, Config{}).RunBatch(context.Background()))

	assert.Equal(t, "open", repo.states[proposed.BranchKey],
		"an invalid resolution must not transition the branch")
}

func TestProjector_RejectsUntypedBranch(t *testing.T) {
	user := userPtr()
	// Each of these is missing one leg of the four-tuple → must NOT be folded.
	noKind := validBranchPayload()
	noKind.BranchKey = "b:nokind"
	noKind.RelationKind = ""
	noWhy := validBranchPayload()
	noWhy.BranchKey = "b:nowhy"
	noWhy.Why = ""
	noEvidence := validBranchPayload()
	noEvidence.BranchKey = "b:noev"
	noEvidence.EvidenceRefs = nil
	noConf := validBranchPayload()
	noConf.BranchKey = "b:noconf"
	noConf.Confidence = ""

	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		branchEvent(1, noKind, user),
		branchEvent(2, noWhy, user),
		branchEvent(3, noEvidence, user),
		branchEvent(4, noConf, user),
	})
	require.NoError(t, NewProjector(repo, nil, Config{}).RunBatch(context.Background()))

	assert.Empty(t, repo.branches,
		"a branch missing any of relation_kind/why/evidence/confidence must never be surfaced (no untyped branch)")
	assert.Equal(t, int64(4), repo.checkpoint, "checkpoint still advances past rejected branches")
}

func outcomeEvent(seq int64, eventType, aggregateID, dedupe string, payload map[string]any, user *uuid.UUID) sovereign_db.KnowledgeEvent {
	body, _ := json.Marshal(payload)
	return sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      seq,
		OccurredAt:    time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		UserID:        user,
		EventType:     eventType,
		AggregateType: "trail_branch",
		AggregateID:   aggregateID,
		DedupeKey:     dedupe,
		Payload:       body,
	}
}

func TestProjector_FoldsTrailActOutcome(t *testing.T) {
	user := userPtr()
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		outcomeEvent(1, "trail.act_outcome.v1", "cluster:u:article:z",
			"trail.act_outcome.v1:cluster:u:article:z",
			map[string]any{"branch_key": "cluster:u:article:z", "item_key": "article:z", "dwell_ms": 42000}, user),
	})
	require.NoError(t, NewProjector(repo, nil, Config{}).RunBatch(context.Background()))

	require.Len(t, repo.outcomes, 1, "trail.act_outcome.v1 must project into the outcomes side table")
	o := repo.outcomes["trail.act_outcome.v1:cluster:u:article:z"]
	assert.Equal(t, "cluster:u:article:z", o.BranchKey)
	assert.Equal(t, "article:z", o.ItemKey)
	require.NotNil(t, o.DwellMs, "trail outcomes carry the raw dwell")
	assert.Equal(t, int64(42000), *o.DwellMs)
	assert.Empty(t, o.LegacyOutcome)
	assert.Empty(t, repo.upserts, "an act outcome must never add a row to the spine (D20)")
	assert.Equal(t, int64(1), repo.checkpoint)
}

func TestProjector_FoldsLegacyLoopActOutcomeVerbatim(t *testing.T) {
	user := userPtr()
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		outcomeEvent(1, "knowledge_loop.act_outcome.v1", "entry:x",
			"knowledge_loop.act_outcome.v1:entry:x:default",
			map[string]any{
				"acted_event_id": uuid.New().String(),
				"entry_key":      "article:x",
				"lens_mode_id":   "default",
				"outcome":        "engaged",
				"observed_at":    "2026-05-20T10:00:00Z",
			}, user),
	})
	require.NoError(t, NewProjector(repo, nil, Config{}).RunBatch(context.Background()))

	require.Len(t, repo.outcomes, 1, "historical loop outcomes must keep feeding wear after the retire")
	o := repo.outcomes["knowledge_loop.act_outcome.v1:entry:x:default"]
	assert.Equal(t, "article:x", o.ItemKey, "item key comes from the payload entry_key, not the aggregate id")
	assert.Nil(t, o.DwellMs, "legacy classified outcomes are never faked into milliseconds (D18)")
	assert.Equal(t, "engaged", o.LegacyOutcome)
	assert.Empty(t, o.BranchKey)
	assert.Empty(t, repo.upserts, "legacy outcomes do not add spine rows either")
}

func TestProjector_ActOutcomeReplayIsDeterministic(t *testing.T) {
	user := userPtr()
	events := []sovereign_db.KnowledgeEvent{
		outcomeEvent(1, "trail.act_outcome.v1", "b1", "trail.act_outcome.v1:b1",
			map[string]any{"branch_key": "b1", "item_key": "article:a", "dwell_ms": 1000}, user),
		outcomeEvent(2, "knowledge_loop.act_outcome.v1", "entry:x", "knowledge_loop.act_outcome.v1:entry:x:default",
			map[string]any{"entry_key": "article:x", "outcome": "no_engagement"}, user),
	}
	first := newFakeRepo(events)
	require.NoError(t, NewProjector(first, nil, Config{}).RunBatch(context.Background()))
	second := newFakeRepo(events)
	require.NoError(t, NewProjector(second, nil, Config{}).RunBatch(context.Background()))

	assert.Equal(t, first.outcomes, second.outcomes, "outcome projection must be a deterministic fold of the log")
}
