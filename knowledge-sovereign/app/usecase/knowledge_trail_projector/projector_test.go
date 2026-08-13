package knowledge_trail_projector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/usecase/trail_planner"
)

// recordingHandler is a minimal slog.Handler that captures every record for
// assertion — used to pin the Wave 10 branch-KPI log lines without wiring a
// real log sink.
type recordingHandler struct {
	records []recordedLog
}

type recordedLog struct {
	Level   slog.Level
	Message string
	Attrs   map[string]any
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := map[string]any{}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	h.records = append(h.records, recordedLog{Level: r.Level, Message: r.Message, Attrs: attrs})
	return nil
}

func (h *recordingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *recordingHandler) find(message string) (recordedLog, bool) {
	for _, r := range h.records {
		if r.Message == message {
			return r, true
		}
	}
	return recordedLog{}, false
}

// fakeRepo is an in-memory stand-in for the sovereign repository. It records
// upserts keyed by footprint_key / branch_key so the test can assert reproject
// determinism and the untyped-branch rejection.
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
	// frontiers are handed out one per ReadSequenceGapFrontier call, the last
	// one repeating; each one's HoleOpen is filled in from events rather than
	// scripted. The default stands for a write transaction that never ends:
	// its id sits below the ceiling of the first sighting, so a hole in the
	// sequence is never mistaken for a burned one unless a test says so.
	frontiers []sovereign_db.SequenceGapFrontier
	// beforeGapFrontier runs inside the gap frontier read, standing for a
	// writer that commits after the batch was read and before the verdict.
	beforeGapFrontier func()
	upserts           map[string]sovereign_db.TrailFootprint
	branches          map[string]sovereign_db.TrailBranch
	states            map[string]string
	outcomes          map[string]sovereign_db.TrailActOutcome
}

// fakeAdvance is one recorded compare-and-set attempt.
type fakeAdvance struct {
	From  sovereign_db.ProjectionCheckpoint
	ToSeq int64
}

// fakeCheckpointAt is the fake's initial stored updated_at. Fixed, never wall
// clock: it is compared for equality, not for recency.
var fakeCheckpointAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newFakeRepo(events []sovereign_db.KnowledgeEvent) *fakeRepo {
	return &fakeRepo{
		events:           events,
		checkpointAt:     fakeCheckpointAt,
		checkpointExists: true,
		frontiers:        []sovereign_db.SequenceGapFrontier{{Ceiling: 101, Xmin: 100}},
		upserts:          map[string]sovereign_db.TrailFootprint{},
		branches:         map[string]sovereign_db.TrailBranch{},
		states:           map[string]string{},
		outcomes:         map[string]sovereign_db.TrailActOutcome{},
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

// TestProjector_HighDensityReplayHasNoSilentTruncation replays a
// production-density log (thousands of events, small batches) twice and
// checks exact coverage. A 2-row seed cannot catch batch-boundary truncation —
// the silent-truncation failure class only shows at density.
func TestProjector_HighDensityReplayIsExactAtBatchBoundaries(t *testing.T) {
	user := userPtr()
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	var events []sovereign_db.KnowledgeEvent
	seq := int64(0)
	next := func() int64 { seq++; return seq }
	const items = 1200
	for i := range items {
		item := fmt.Sprintf("article:%04d", i)
		at := base.Add(time.Duration(i) * time.Minute)
		events = append(events,
			actEvent(next(), "HomeItemOpened", item, "open:"+item, at, user))
		if i%3 == 0 {
			events = append(events,
				outcomeEvent(next(), "trail.act_outcome.v1", "b:"+item, "trail.act_outcome.v1:b:"+item,
					map[string]any{"branch_key": "b:" + item, "item_key": item, "dwell_ms": 31000}, user))
		}
		if i%5 == 0 {
			events = append(events,
				outcomeEvent(next(), "knowledge_loop.act_outcome.v1", "entry:"+item,
					"knowledge_loop.act_outcome.v1:entry:"+item+":default",
					map[string]any{"entry_key": item, "outcome": "no_engagement"}, user))
		}
	}
	wantOutcomes := items/3 + items/5
	cfg := Config{BatchSize: 256, MaxBatchesPerTick: 1}

	run := func() *fakeRepo {
		repo := newFakeRepo(events)
		p := NewProjector(repo, nil, cfg)
		// Drive ticks until the checkpoint stops moving, like the runtime ticker.
		for {
			before := repo.checkpoint
			require.NoError(t, p.RunBatch(context.Background()))
			if repo.checkpoint == before {
				break
			}
		}
		return repo
	}

	first := run()
	assert.Equal(t, seq, first.checkpoint, "checkpoint must reach the end of the log")
	assert.Len(t, first.upserts, items, "every act event must become a footprint — no silent truncation")
	assert.Len(t, first.outcomes, wantOutcomes, "every outcome event must land in the side table — no silent truncation")

	second := run()
	assert.Equal(t, first.upserts, second.upserts)
	assert.Equal(t, first.outcomes, second.outcomes)
}

// TestProjector_DismissReasonPayloadDoesNotBreakFold pins Wave 10 (D28(d)):
// the sovereign projector must stay payload-only and must not choke on a
// branch_resolved event carrying the new optional dismiss_reason field — no
// new column, no new event vocabulary, the resolution still folds.
func TestProjector_DismissReasonPayloadDoesNotBreakFold(t *testing.T) {
	user := userPtr()
	proposed := validBranchPayload()
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		branchEvent(1, proposed, user),
		resolvedEvent(2, trail_planner.BranchResolvedPayload{
			BranchKey: proposed.BranchKey, Resolution: "dismissed", DismissReason: "not_following_topic",
		}, user),
	})
	require.NoError(t, NewProjector(repo, nil, Config{}).RunBatch(context.Background()))

	assert.Equal(t, "dismissed", repo.states[proposed.BranchKey],
		"a dismiss_reason on the payload must not prevent the resolution from folding")
}

// TestProjector_LogsBranchResolvedKPI pins the Wave 10 observability bullet:
// every branch_resolved fold emits a trail.branch_resolved log carrying
// resolution and whether a dismiss reason was supplied — the KPI is
// taken→engaged dwell, not CTR, but resolution+reason presence is the raw
// signal the ClickHouse pipeline aggregates.
func TestProjector_LogsBranchResolvedKPI(t *testing.T) {
	user := userPtr()
	proposed := validBranchPayload()
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		branchEvent(1, proposed, user),
		resolvedEvent(2, trail_planner.BranchResolvedPayload{
			BranchKey: proposed.BranchKey, Resolution: "dismissed", DismissReason: "wrong_relation",
		}, user),
	})
	rec := &recordingHandler{}
	p := NewProjector(repo, slog.New(rec), Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	log, ok := rec.find("trail.branch_resolved")
	require.True(t, ok, "a trail.branch_resolved KPI log must be emitted")
	assert.Equal(t, "dismissed", log.Attrs["resolution"])
	assert.Equal(t, true, log.Attrs["has_reason"])
}

// TestProjector_LogsBranchResolvedKPI_NoReason pins has_reason=false when no
// dismiss reason was supplied (including "taken", which never carries one).
func TestProjector_LogsBranchResolvedKPI_NoReason(t *testing.T) {
	user := userPtr()
	proposed := validBranchPayload()
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		branchEvent(1, proposed, user),
		resolvedEvent(2, trail_planner.BranchResolvedPayload{BranchKey: proposed.BranchKey, Resolution: "taken"}, user),
	})
	rec := &recordingHandler{}
	p := NewProjector(repo, slog.New(rec), Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	log, ok := rec.find("trail.branch_resolved")
	require.True(t, ok)
	assert.Equal(t, "taken", log.Attrs["resolution"])
	assert.Equal(t, false, log.Attrs["has_reason"])
}

// TestProjector_LogsActOutcomeObservedKPI pins the dwell-side observability
// bullet: every trail.act_outcome.v1 fold logs the raw dwell_ms and whether
// it crosses the engaged threshold, referencing sovereign_db.EngagedDwellMs
// rather than duplicating the literal.
func TestProjector_LogsActOutcomeObservedKPI(t *testing.T) {
	user := userPtr()
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		outcomeEvent(1, "trail.act_outcome.v1", "cluster:u:article:z",
			"trail.act_outcome.v1:cluster:u:article:z",
			map[string]any{"branch_key": "cluster:u:article:z", "item_key": "article:z", "dwell_ms": sovereign_db.EngagedDwellMs}, user),
	})
	rec := &recordingHandler{}
	p := NewProjector(repo, slog.New(rec), Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	log, ok := rec.find("trail.act_outcome.observed")
	require.True(t, ok, "a trail.act_outcome.observed KPI log must be emitted")
	assert.Equal(t, sovereign_db.EngagedDwellMs, log.Attrs["dwell_ms"])
	assert.Equal(t, true, log.Attrs["engaged"], "dwell at the threshold must read as engaged")
}

// TestProjector_LogsActOutcomeObservedKPI_BelowThreshold pins engaged=false
// for a dwell below sovereign_db.EngagedDwellMs.
func TestProjector_LogsActOutcomeObservedKPI_BelowThreshold(t *testing.T) {
	user := userPtr()
	below := sovereign_db.EngagedDwellMs - 1
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		outcomeEvent(1, "trail.act_outcome.v1", "cluster:u:article:z",
			"trail.act_outcome.v1:cluster:u:article:z",
			map[string]any{"branch_key": "cluster:u:article:z", "item_key": "article:z", "dwell_ms": below}, user),
	})
	rec := &recordingHandler{}
	p := NewProjector(repo, slog.New(rec), Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	log, ok := rec.find("trail.act_outcome.observed")
	require.True(t, ok)
	assert.Equal(t, false, log.Attrs["engaged"])
}

// ── checkpoint advance ──

// The checkpoint must be advanced with the state the batch actually read, not
// with a value assembled at write time. The fake refuses any other token, the
// same way the guarded UPDATE refuses a row that has moved.
func TestProjector_AdvancesTheCheckpointWithTheStateItReadAtTheStartOfTheBatch(t *testing.T) {
	user := userPtr()
	at := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		actEvent(11, "HomeItemOpened", "article:a", "d-11", at, user),
		actEvent(12, "HomeItemOpened", "article:b", "d-12", at, user),
	})
	repo.checkpoint = 10

	require.NoError(t, NewProjector(repo, nil, Config{}).RunBatch(context.Background()))

	require.Len(t, repo.advances, 1)
	assert.EqualValues(t, 10, repo.advances[0].From.LastEventSeq, "the token must carry the sequence the batch folded from")
	assert.True(t, repo.advances[0].From.UpdatedAt.Equal(fakeCheckpointAt), "the token must carry the witness that was read")
	assert.True(t, repo.advances[0].From.Exists)
	assert.EqualValues(t, 12, repo.advances[0].ToSeq)
	assert.EqualValues(t, 12, repo.checkpoint)
}

// PM-2026-010's ending, at unit level: something else wrote the checkpoint
// while this batch was folding. The advance is refused, and the projector must
// not carry on as if it had landed — it stops the tick, says so loudly, and
// lets the next tick re-read whatever the other writer decided.
func TestProjector_RejectedCheckpointAdvanceStopsTheTickAndIsReportedLoudly(t *testing.T) {
	user := userPtr()
	at := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		actEvent(1, "HomeItemOpened", "article:a", "d-1", at, user),
		actEvent(2, "HomeItemOpened", "article:b", "d-2", at, user),
		actEvent(3, "HomeItemOpened", "article:c", "d-3", at, user),
		actEvent(4, "HomeItemOpened", "article:d", "d-4", at, user),
	})
	repo.advanceRejected = true

	logs := &recordingHandler{}
	// Batches of two, four allowed per tick: without an explicit stop the loop
	// would fold the same events again and again behind an unmoving checkpoint.
	p := NewProjector(repo, slog.New(logs), Config{BatchSize: 2, MaxBatchesPerTick: 4})

	require.NoError(t, p.RunBatch(context.Background()),
		"losing the checkpoint race is a recoverable outcome, not a batch failure")

	assert.Zero(t, repo.checkpoint, "a refused advance must leave the stored checkpoint exactly as the other writer left it")
	assert.Len(t, repo.advances, 1, "a refused advance must not be retried in a loop")
	assert.Equal(t, 1, repo.listCalls, "the tick must stop instead of folding the next batch on a checkpoint it could not move")

	rec, ok := logs.find("trail.checkpoint_advance_rejected")
	require.True(t, ok, "a refused advance must be reported, never swallowed")
	assert.Equal(t, slog.LevelError, rec.Level, "the batch's work was abandoned; that is error-level")
	assert.EqualValues(t, 0, rec.Attrs["expected_seq"])
	assert.EqualValues(t, 2, rec.Attrs["attempted_seq"])
	assert.Equal(t, projectorName, rec.Attrs["projector"])
}

// event_seq is handed out when a transaction inserts, not when it commits, so
// a transaction holding seq 1 can still be in flight while a later
// transaction's seq 2 is already visible. Folding whatever the query returned
// and advancing to its maximum moves the checkpoint to 2, and seq 1 — asked
// for as "event_seq > 2" from then on — is never folded: the footprint is lost
// from the spine permanently, with nothing left behind that says so.
func TestProjector_StopsAtASequenceGapLeftByAnUncommittedTransaction(t *testing.T) {
	user := userPtr()
	at := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		actEvent(2, "HomeItemOpened", "article:b", "d-2", at, user),
	})
	p := NewProjector(repo, nil, Config{BatchSize: 500, MaxBatchesPerTick: 4})

	require.NoError(t, p.RunBatch(context.Background()))

	assert.Zero(t, repo.checkpoint, "the checkpoint must not step over a sequence that is missing rather than absent")
	assert.Empty(t, repo.upserts, "an event beyond the gap must wait for the gap to resolve, so folds stay in sequence order")

	// The transaction holding seq 1 commits.
	repo.events = append([]sovereign_db.KnowledgeEvent{
		actEvent(1, "HomeItemOpened", "article:a", "d-1", at, user),
	}, repo.events...)

	require.NoError(t, p.RunBatch(context.Background()))

	assert.EqualValues(t, 2, repo.checkpoint, "once the gap is filled the batch folds through to the tip")
	assert.Len(t, repo.upserts, 2, "both footprints must reach the spine")
}

// A rolled-back transaction burns its sequence value: the hole it leaves is
// never filled, and waiting for it forever would wedge the spine at that
// sequence for good. Once every transaction that had written when the hole was
// first seen has finished, no live writer can still hold the value, and the
// projection steps over it — loudly, because a burned sequence is a
// producer-side rollback worth seeing.
func TestProjector_StepsPastASequenceBurnedByARolledBackTransaction(t *testing.T) {
	user := userPtr()
	at := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		actEvent(2, "HomeItemOpened", "article:b", "d-2", at, user),
	})
	repo.frontiers = []sovereign_db.SequenceGapFrontier{
		{Ceiling: 101, Xmin: 100}, // the writer that took seq 1 wrote below this ceiling and is still in flight
		{Ceiling: 140, Xmin: 101}, // xmin has reached that ceiling: it finished, and seq 1 never arrived
	}
	logs := &recordingHandler{}
	p := NewProjector(repo, slog.New(logs), Config{})

	require.NoError(t, p.RunBatch(context.Background()))
	assert.Zero(t, repo.checkpoint, "the hole is still fillable on this tick")

	require.NoError(t, p.RunBatch(context.Background()))

	assert.EqualValues(t, 2, repo.checkpoint, "a sequence no live transaction can still commit must not wedge the projection")
	assert.Len(t, repo.upserts, 1, "the events past the burned sequence must be folded")

	rec, ok := logs.find("trail.sequence_gap_abandoned")
	require.True(t, ok, "abandoning a sequence must be reported, never silent")
	assert.Equal(t, slog.LevelWarn, rec.Level)
	assert.EqualValues(t, 1, rec.Attrs["gap_seq"])
}

// The mirror of the case above: while the transaction that took the missing
// sequence is still running, no number of ticks may step over it.
func TestProjector_WaitsWhileTheTransactionHoldingTheMissingSequenceIsInFlight(t *testing.T) {
	user := userPtr()
	at := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		actEvent(2, "HomeItemOpened", "article:b", "d-2", at, user),
	})
	p := NewProjector(repo, nil, Config{})

	for i := 0; i < 3; i++ {
		require.NoError(t, p.RunBatch(context.Background()))
	}

	assert.Zero(t, repo.checkpoint, "an in-flight writer still owns seq 1; the checkpoint must stay behind it")
	assert.Empty(t, repo.upserts)
}

// A hole in the middle of a batch cuts it there: everything below the hole is
// folded and the checkpoint stops one short of it, so the missing sequence is
// still the next thing asked for when its transaction commits.
func TestProjector_FoldsUpToAMidBatchSequenceGapAndResumesAcrossIt(t *testing.T) {
	user := userPtr()
	at := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		actEvent(1, "HomeItemOpened", "article:a", "d-1", at, user),
		actEvent(2, "HomeItemOpened", "article:b", "d-2", at, user),
		actEvent(4, "HomeItemOpened", "article:d", "d-4", at, user),
	})
	p := NewProjector(repo, nil, Config{})

	require.NoError(t, p.RunBatch(context.Background()))

	assert.EqualValues(t, 2, repo.checkpoint, "the checkpoint must stop one short of the hole")
	assert.Len(t, repo.upserts, 2)

	repo.events = append(repo.events[:2], append([]sovereign_db.KnowledgeEvent{
		actEvent(3, "HomeItemOpened", "article:c", "d-3", at, user),
	}, repo.events[2:]...)...)

	require.NoError(t, p.RunBatch(context.Background()))

	assert.EqualValues(t, 4, repo.checkpoint)
	assert.Len(t, repo.upserts, 4, "the event that was in flight must reach the spine")
}

// A run of sequences burned together — a batch of appends that all rolled back
// — is stepped over in one advance, not one sequence per tick: every value
// below the first visible event was taken before that event's, so the same
// frontier decides the whole run at once.
func TestProjector_StepsPastAWholeRunOfBurnedSequencesAtOnce(t *testing.T) {
	user := userPtr()
	at := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		actEvent(4, "HomeItemOpened", "article:d", "d-4", at, user),
	})
	repo.frontiers = []sovereign_db.SequenceGapFrontier{
		{Ceiling: 101, Xmin: 100},
		{Ceiling: 140, Xmin: 101},
	}
	logs := &recordingHandler{}
	p := NewProjector(repo, slog.New(logs), Config{})

	require.NoError(t, p.RunBatch(context.Background()))
	require.NoError(t, p.RunBatch(context.Background()))

	assert.EqualValues(t, 4, repo.checkpoint)
	assert.Len(t, repo.upserts, 1)

	rec, ok := logs.find("trail.sequence_gap_abandoned")
	require.True(t, ok)
	assert.EqualValues(t, 1, rec.Attrs["gap_seq"])
	assert.EqualValues(t, 3, rec.Attrs["gap_through"], "the whole burned run is reported, not just its head")
}

// The verdict is a second round trip, and Read Committed gives it its own
// snapshot: a writer that commits between the batch read and the verdict is
// missing from the batch while the ids already count it finished. Judging on
// that pair alone steps the checkpoint over an event that is committed and
// readable, and the footprint is lost from the spine with nothing left behind
// that says so. The frontier re-reads the run itself for exactly this case.
func TestProjector_NeverStepsOverASequenceThatCommittedAfterTheBatchWasRead(t *testing.T) {
	user := userPtr()
	at := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		actEvent(2, "HomeItemOpened", "article:b", "d-2", at, user),
	})
	repo.frontiers = []sovereign_db.SequenceGapFrontier{
		{Ceiling: 101, Xmin: 100},
		{Ceiling: 140, Xmin: 101}, // by the ids alone, every writer below the first ceiling has finished
	}
	p := NewProjector(repo, nil, Config{})

	require.NoError(t, p.RunBatch(context.Background()))
	require.Zero(t, repo.checkpoint, "the hole is still fillable on this tick")

	// The writer holding seq 1 commits after this tick's batch read and before
	// its verdict.
	repo.beforeGapFrontier = func() {
		repo.beforeGapFrontier = nil
		repo.events = append([]sovereign_db.KnowledgeEvent{
			actEvent(1, "HomeItemOpened", "article:a", "d-1", at, user),
		}, repo.events...)
	}

	require.NoError(t, p.RunBatch(context.Background()))
	assert.Zero(t, repo.checkpoint, "the sequence arrived; the checkpoint must not step over it")

	require.NoError(t, p.RunBatch(context.Background()))

	assert.EqualValues(t, 2, repo.checkpoint)
	assert.Len(t, repo.upserts, 2, "the event that committed mid-tick must still reach the spine")
}
