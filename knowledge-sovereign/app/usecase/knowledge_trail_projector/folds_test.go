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

var testFixedTime = time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)

func validBranchPayload() trail_planner.BranchProposedPayload {
	return trail_planner.BranchProposedPayload{
		BranchKey: "cluster:u:article:z", AnchorItemKey: "article:a", RelationKind: "cluster",
		Why: "Joins a topic you follow — shares rust.", Confidence: "plausible",
		EvidenceRefs:  []trail_planner.EvidenceRef{{RefID: "rust", Label: "rust", Kind: "tag"}},
		TargetItemKey: "article:z", TargetTitle: "Async Rust",
	}
}

func branchEvent(seq int64, payload trail_planner.BranchProposedPayload, user *uuid.UUID) sovereign_db.KnowledgeEvent {
	body, _ := json.Marshal(payload)
	return sovereign_db.KnowledgeEvent{
		EventID: uuid.New(), EventSeq: seq, OccurredAt: testFixedTime, TenantID: uuid.New(), UserID: user,
		EventType: trail_planner.EventTrailBranchProposed, AggregateType: "trail_branch", AggregateID: payload.BranchKey, Payload: body,
	}
}

func resolvedEvent(seq int64, payload trail_planner.BranchResolvedPayload, user *uuid.UUID) sovereign_db.KnowledgeEvent {
	body, _ := json.Marshal(payload)
	return sovereign_db.KnowledgeEvent{
		EventID: uuid.New(), EventSeq: seq, OccurredAt: testFixedTime, TenantID: uuid.New(), UserID: user,
		EventType: trail_planner.EventTrailBranchResolved, AggregateType: "trail_branch", AggregateID: payload.BranchKey, Payload: body,
	}
}

func outcomeEvent(seq int64, eventType, aggregateID, dedupe string, payload map[string]any, user *uuid.UUID) sovereign_db.KnowledgeEvent {
	body, _ := json.Marshal(payload)
	return sovereign_db.KnowledgeEvent{
		EventID: uuid.New(), EventSeq: seq, OccurredAt: time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC),
		TenantID: uuid.New(), UserID: user, EventType: eventType, AggregateType: "trail_branch", AggregateID: aggregateID, DedupeKey: dedupe, Payload: body,
	}
}

// ── Pure table tests ──

func TestBuildBranchFromEvent(t *testing.T) {
	valid := validBranchPayload()
	validBytes, _ := json.Marshal(valid)

	untyped := valid
	untyped.Why = ""
	untypedBytes, _ := json.Marshal(untyped)

	tests := []struct {
		name      string
		evt       sovereign_db.KnowledgeEvent
		wantOK    bool
		wantErr   bool
		checkKind string
	}{
		{"valid branch", sovereign_db.KnowledgeEvent{Payload: validBytes}, true, false, "cluster"},
		{"untyped branch", sovereign_db.KnowledgeEvent{Payload: untypedBytes}, false, false, ""},
		{"invalid json", sovereign_db.KnowledgeEvent{Payload: []byte("not-json")}, false, true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			branch, payload, ok, err := buildBranchFromEvent(tt.evt)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				require.NotNil(t, branch)
				assert.Equal(t, tt.checkKind, branch.RelationKind)
				assert.Equal(t, valid.BranchKey, payload.BranchKey)
			}
		})
	}
}

func TestBuildBranchResolutionFromEvent(t *testing.T) {
	user := userPtr()
	validBytes, _ := json.Marshal(trail_planner.BranchResolvedPayload{BranchKey: "b1", Resolution: "taken"})
	invalidBytes, _ := json.Marshal(trail_planner.BranchResolvedPayload{BranchKey: "b1", Resolution: "unknown"})

	tests := []struct {
		name    string
		evt     sovereign_db.KnowledgeEvent
		wantOK  bool
		wantErr bool
	}{
		{"valid taken", sovereign_db.KnowledgeEvent{UserID: user, Payload: validBytes}, true, false},
		{"invalid resolution string", sovereign_db.KnowledgeEvent{UserID: user, Payload: invalidBytes}, false, false},
		{"missing user", sovereign_db.KnowledgeEvent{UserID: nil, Payload: validBytes}, false, false},
		{"corrupted json", sovereign_db.KnowledgeEvent{UserID: user, Payload: []byte("{corrupt")}, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, ok, err := buildBranchResolutionFromEvent(tt.evt)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, "taken", res.Resolution)
			}
		})
	}
}

func TestBuildActOutcomeFromEvent(t *testing.T) {
	user := userPtr()
	dwell := int64(15000)
	trailBytes, _ := json.Marshal(map[string]any{"branch_key": "b1", "item_key": "a1", "dwell_ms": dwell})
	incompleteBytes, _ := json.Marshal(map[string]any{"branch_key": "b1"})
	legacyBytes, _ := json.Marshal(map[string]any{"entry_key": "a1", "outcome": "engaged"})

	tests := []struct {
		name       string
		evt        sovereign_db.KnowledgeEvent
		wantOK     bool
		wantErr    bool
		wantItem   string
		wantLegacy string
	}{
		{"valid trail dwell", sovereign_db.KnowledgeEvent{UserID: user, EventType: eventTrailActOutcome, Payload: trailBytes}, true, false, "a1", ""},
		{"incomplete trail dwell", sovereign_db.KnowledgeEvent{UserID: user, EventType: eventTrailActOutcome, Payload: incompleteBytes}, false, false, "", ""},
		{"invalid json", sovereign_db.KnowledgeEvent{UserID: user, EventType: eventTrailActOutcome, Payload: []byte("{invalid")}, false, true, "", ""},
		{"valid legacy outcome", sovereign_db.KnowledgeEvent{UserID: user, EventType: eventLegacyActOutcome, Payload: legacyBytes}, true, false, "a1", "engaged"},
		{"missing user", sovereign_db.KnowledgeEvent{UserID: nil, EventType: eventTrailActOutcome, Payload: trailBytes}, false, false, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome, ok, err := buildActOutcomeFromEvent(tt.evt)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				require.NotNil(t, outcome)
				assert.Equal(t, tt.wantItem, outcome.ItemKey)
				assert.Equal(t, tt.wantLegacy, outcome.LegacyOutcome)
			}
		})
	}
}

func TestFootprintFromEvent_Pure(t *testing.T) {
	user := userPtr()
	tests := []struct {
		eventType, agg string
		user           *uuid.UUID
		wantOK         bool
		wantVerb       string
	}{
		{"HomeItemOpened", "item:1", user, true, "read"},
		{"HomeItemAsked", "item:1", user, true, "asked"},
		{"HomeItemListened", "item:1", user, true, "listened"},
		{"HomeItemDismissed", "item:1", user, true, "dismissed"},
		{"knowledge_loop.acted.v1", "item:1", user, true, "read"},
		{"SummaryCreated", "item:1", user, false, ""},
		{"HomeItemOpened", "item:1", nil, false, ""},
		{"HomeItemOpened", "", user, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			fp, ok := footprintFromEvent(sovereign_db.KnowledgeEvent{UserID: tt.user, EventType: tt.eventType, AggregateID: tt.agg})
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantVerb, fp.Verb)
				assert.Equal(t, "item:1", fp.ItemKey)
			}
		})
	}
}

// ── Fold behavior tests ──

func TestProjector_FoldsActEventsToFootprints(t *testing.T) {
	user := userPtr()
	base := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	events := []sovereign_db.KnowledgeEvent{
		actEvent(1, "HomeItemOpened", "article:a", "open:a", base, user),
		actEvent(2, "HomeItemAsked", "article:a", "ask:a", base.Add(time.Minute), user),
		actEvent(3, "SummaryVersionCreated", "article:a", "sv:a", base.Add(2*time.Minute), user),
		actEvent(4, "HomeItemListened", "article:b", "listen:b", base.Add(3*time.Minute), user),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{BatchSize: 500, MaxBatchesPerTick: 4})

	require.NoError(t, p.RunBatch(context.Background()))
	assert.Len(t, repo.upserts, 3)
	assert.Equal(t, "read", repo.upserts["open:a"].Verb)
	assert.Equal(t, "asked", repo.upserts["ask:a"].Verb)
	assert.Equal(t, "listened", repo.upserts["listen:b"].Verb)
	assert.Equal(t, int64(4), repo.checkpoint)
}

func TestProjector_SkipsSystemEventsWithoutUser(t *testing.T) {
	events := []sovereign_db.KnowledgeEvent{
		actEvent(1, "HomeItemOpened", "article:a", "open:a", testFixedTime, nil),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))
	assert.Empty(t, repo.upserts)
	assert.Equal(t, int64(1), repo.checkpoint)
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
	tests := []struct {
		name, res, reason, wantState string
	}{
		{"valid taken", "taken", "", "taken"},
		{"invalid resolution ignored", "wat", "", "open"},
		{"dismissed with reason", "dismissed", "not_following_topic", "dismissed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
				branchEvent(1, proposed, user),
				resolvedEvent(2, trail_planner.BranchResolvedPayload{
					BranchKey: proposed.BranchKey, Resolution: tt.res, DismissReason: tt.reason,
				}, user),
			})
			require.NoError(t, NewProjector(repo, nil, Config{}).RunBatch(context.Background()))
			assert.Equal(t, tt.wantState, repo.states[proposed.BranchKey])
		})
	}
}

func TestProjector_RejectsUntypedBranch(t *testing.T) {
	user := userPtr()
	mk := func(mutate func(*trail_planner.BranchProposedPayload)) trail_planner.BranchProposedPayload {
		b := validBranchPayload()
		mutate(&b)
		return b
	}
	cases := []trail_planner.BranchProposedPayload{
		mk(func(b *trail_planner.BranchProposedPayload) { b.RelationKind = "" }),
		mk(func(b *trail_planner.BranchProposedPayload) { b.Why = "" }),
		mk(func(b *trail_planner.BranchProposedPayload) { b.EvidenceRefs = nil }),
		mk(func(b *trail_planner.BranchProposedPayload) { b.Confidence = "" }),
	}
	var evts []sovereign_db.KnowledgeEvent
	for i, c := range cases {
		evts = append(evts, branchEvent(int64(i+1), c, user))
	}
	repo := newFakeRepo(evts)
	require.NoError(t, NewProjector(repo, nil, Config{}).RunBatch(context.Background()))
	assert.Empty(t, repo.branches)
	assert.Equal(t, int64(4), repo.checkpoint)
}

func TestProjector_FoldsActOutcomes(t *testing.T) {
	user := userPtr()
	t.Run("trail outcome", func(t *testing.T) {
		repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
			outcomeEvent(1, "trail.act_outcome.v1", "cluster:u:article:z", "trail.act_outcome.v1:cluster:u:article:z",
				map[string]any{"branch_key": "cluster:u:article:z", "item_key": "article:z", "dwell_ms": 42000}, user),
		})
		require.NoError(t, NewProjector(repo, nil, Config{}).RunBatch(context.Background()))
		require.Len(t, repo.outcomes, 1)
		o := repo.outcomes["trail.act_outcome.v1:cluster:u:article:z"]
		assert.Equal(t, "cluster:u:article:z", o.BranchKey)
		assert.Equal(t, "article:z", o.ItemKey)
		require.NotNil(t, o.DwellMs)
		assert.Equal(t, int64(42000), *o.DwellMs)
		assert.Empty(t, o.LegacyOutcome)
		assert.Empty(t, repo.upserts, "an act outcome never adds a footprint to the spine")
		assert.Equal(t, int64(1), repo.checkpoint)
	})
	t.Run("legacy outcome verbatim", func(t *testing.T) {
		repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
			outcomeEvent(1, "knowledge_loop.act_outcome.v1", "entry:x", "knowledge_loop.act_outcome.v1:entry:x:default",
				map[string]any{"entry_key": "article:x", "outcome": "engaged"}, user),
		})
		require.NoError(t, NewProjector(repo, nil, Config{}).RunBatch(context.Background()))
		require.Len(t, repo.outcomes, 1)
		o := repo.outcomes["knowledge_loop.act_outcome.v1:entry:x:default"]
		assert.Equal(t, "article:x", o.ItemKey)
		assert.Nil(t, o.DwellMs)
		assert.Equal(t, "engaged", o.LegacyOutcome)
		assert.Empty(t, o.BranchKey)
		assert.Empty(t, repo.upserts, "a legacy act outcome never adds a footprint to the spine")
	})
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
	assert.Equal(t, first.outcomes, second.outcomes)
}

func TestProjector_LogsBranchResolvedKPI(t *testing.T) {
	for _, tt := range []struct {
		res, reason string
		wantReason  bool
	}{
		{"dismissed", "wrong_relation", true},
		{"taken", "", false},
	} {
		t.Run(tt.res, func(t *testing.T) {
			user, proposed := userPtr(), validBranchPayload()
			repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
				branchEvent(1, proposed, user),
				resolvedEvent(2, trail_planner.BranchResolvedPayload{
					BranchKey: proposed.BranchKey, Resolution: tt.res, DismissReason: tt.reason,
				}, user),
			})
			rec := &recordingHandler{}
			require.NoError(t, NewProjector(repo, slog.New(rec), Config{}).RunBatch(context.Background()))

			log, ok := rec.find("trail.branch_resolved")
			require.True(t, ok)
			assert.Equal(t, tt.res, log.Attrs["resolution"])
			assert.Equal(t, tt.wantReason, log.Attrs["has_reason"])
		})
	}
}

func TestProjector_LogsActOutcomeObservedKPI(t *testing.T) {
	for _, tt := range []struct {
		dwell       int64
		wantEngaged bool
	}{
		{sovereign_db.EngagedDwellMs, true},
		{sovereign_db.EngagedDwellMs - 1, false},
	} {
		t.Run(fmt.Sprintf("dwell_%d", tt.dwell), func(t *testing.T) {
			user := userPtr()
			repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
				outcomeEvent(1, "trail.act_outcome.v1", "cluster:u:article:z", "trail.act_outcome.v1:cluster:u:article:z",
					map[string]any{"branch_key": "cluster:u:article:z", "item_key": "article:z", "dwell_ms": tt.dwell}, user),
			})
			rec := &recordingHandler{}
			require.NoError(t, NewProjector(repo, slog.New(rec), Config{}).RunBatch(context.Background()))

			log, ok := rec.find("trail.act_outcome.observed")
			require.True(t, ok)
			assert.Equal(t, tt.dwell, log.Attrs["dwell_ms"])
			assert.Equal(t, tt.wantEngaged, log.Attrs["engaged"])
		})
	}
}
