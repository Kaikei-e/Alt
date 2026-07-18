package emit_trail_outcome_usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAppendPort struct {
	events []domain.KnowledgeEvent
	err    error
}

func (f *fakeAppendPort) AppendKnowledgeEvent(_ context.Context, e domain.KnowledgeEvent) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.events = append(f.events, e)
	return int64(len(f.events)), nil
}

func TestExecute_AppendsActOutcomeEvent(t *testing.T) {
	port := &fakeAppendPort{}
	uc := NewEmitTrailOutcomeUsecase(port)

	require.NoError(t, uc.Execute(context.Background(), uuid.New(), uuid.New(), "cluster:u:article:z", "article:z", 42000))

	require.Len(t, port.events, 1)
	e := port.events[0]
	assert.Equal(t, EventTrailActOutcome, e.EventType)
	assert.Equal(t, "trail.act_outcome.v1", e.EventType, "event vocabulary pinned by D16 — never knowledge_loop.*")
	assert.Equal(t, EventTrailActOutcome+":cluster:u:article:z", e.DedupeKey,
		"dedupe key is the proposal ref: one outcome per taken branch, first write wins (D19)")
	assert.Equal(t, "trail_branch", e.AggregateType)
	assert.Equal(t, "cluster:u:article:z", e.AggregateID)
	assert.Equal(t, domain.ActorUser, e.ActorType)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(e.Payload, &payload))
	assert.Equal(t, "cluster:u:article:z", payload["branch_key"])
	assert.Equal(t, "article:z", payload["item_key"])
	assert.Equal(t, float64(42000), payload["dwell_ms"])
	// D18: the payload is the raw measurement only — no classification field.
	_, hasOutcome := payload["outcome"]
	assert.False(t, hasOutcome, "payload must not bake an engagement classification into the fact")
}

func TestExecute_RejectsEmptyBranchKey(t *testing.T) {
	uc := NewEmitTrailOutcomeUsecase(&fakeAppendPort{})
	err := uc.Execute(context.Background(), uuid.New(), uuid.New(), "  ", "article:z", 1000)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestExecute_RejectsEmptyItemKey(t *testing.T) {
	uc := NewEmitTrailOutcomeUsecase(&fakeAppendPort{})
	err := uc.Execute(context.Background(), uuid.New(), uuid.New(), "b", "", 1000)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestExecute_RejectsNegativeDwell(t *testing.T) {
	uc := NewEmitTrailOutcomeUsecase(&fakeAppendPort{})
	err := uc.Execute(context.Background(), uuid.New(), uuid.New(), "b", "article:z", -1)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestExecute_ClampsAbsurdDwell(t *testing.T) {
	port := &fakeAppendPort{}
	uc := NewEmitTrailOutcomeUsecase(port)

	require.NoError(t, uc.Execute(context.Background(), uuid.New(), uuid.New(), "b", "article:z", 48*60*60*1000))

	require.Len(t, port.events, 1)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(port.events[0].Payload, &payload))
	assert.Equal(t, float64(MaxDwellMs), payload["dwell_ms"],
		"a forgotten overnight tab must not mint absurd business facts")
}

func TestExecute_WrapsAppendError(t *testing.T) {
	boom := errors.New("append down")
	uc := NewEmitTrailOutcomeUsecase(&fakeAppendPort{err: boom})
	err := uc.Execute(context.Background(), uuid.New(), uuid.New(), "b", "article:z", 1000)
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, ErrInvalidRequest)
}
