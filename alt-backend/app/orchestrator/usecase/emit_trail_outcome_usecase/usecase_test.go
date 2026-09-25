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

func TestValidateEmitRequest(t *testing.T) {
	tests := []struct {
		name      string
		branchKey string
		itemKey   string
		dwellMs   int64
		wantErr   bool
	}{
		{name: "valid request", branchKey: "b1", itemKey: "i1", dwellMs: 500, wantErr: false},
		{name: "empty branch", branchKey: "", itemKey: "i1", dwellMs: 500, wantErr: true},
		{name: "empty item", branchKey: "b1", itemKey: "", dwellMs: 500, wantErr: true},
		{name: "negative dwell", branchKey: "b1", itemKey: "i1", dwellMs: -10, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEmitRequest(tt.branchKey, tt.itemKey, tt.dwellMs)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestClampDwellMs(t *testing.T) {
	tests := []struct {
		name    string
		dwellMs int64
		want    int64
	}{
		{name: "under max", dwellMs: 5000, want: 5000},
		{name: "exact max", dwellMs: MaxDwellMs, want: MaxDwellMs},
		{name: "over max", dwellMs: MaxDwellMs + 1000, want: MaxDwellMs},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, clampDwellMs(tt.dwellMs))
		})
	}
}

func TestBuildOutcomePayload(t *testing.T) {
	payload := buildOutcomePayload("branch-1", "item-1", 1234)
	var parsed map[string]any
	err := json.Unmarshal(payload, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "branch-1", parsed["branch_key"])
	assert.Equal(t, "item-1", parsed["item_key"])
	assert.Equal(t, float64(1234), parsed["dwell_ms"])
}
