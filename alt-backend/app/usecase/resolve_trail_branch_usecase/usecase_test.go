package resolve_trail_branch_usecase

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

const goodUUIDv7 = "01938e82-7c00-7a7b-9b10-0123456789ab"

func TestExecute_AppendsResolvedEvent(t *testing.T) {
	port := &fakeAppendPort{}
	uc := NewResolveTrailBranchUsecase(port)

	require.NoError(t, uc.Execute(context.Background(), uuid.New(), uuid.New(), "cluster:u:article:z", "taken", goodUUIDv7))

	require.Len(t, port.events, 1)
	e := port.events[0]
	assert.Equal(t, EventTrailBranchResolved, e.EventType)
	assert.Equal(t, EventTrailBranchResolved+":"+goodUUIDv7, e.DedupeKey, "dedupe key pins the client UUIDv7 for idempotency")
	assert.Equal(t, "trail_branch", e.AggregateType)
	var payload map[string]string
	require.NoError(t, json.Unmarshal(e.Payload, &payload))
	assert.Equal(t, "cluster:u:article:z", payload["branch_key"])
	assert.Equal(t, "taken", payload["resolution"])
}

func TestExecute_RejectsInvalidResolution(t *testing.T) {
	uc := NewResolveTrailBranchUsecase(&fakeAppendPort{})
	err := uc.Execute(context.Background(), uuid.New(), uuid.New(), "b", "maybe", goodUUIDv7)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestExecute_RejectsNonUUIDv7(t *testing.T) {
	uc := NewResolveTrailBranchUsecase(&fakeAppendPort{})
	err := uc.Execute(context.Background(), uuid.New(), uuid.New(), "b", "taken", "not-a-uuid")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestExecute_RejectsEmptyBranchKey(t *testing.T) {
	uc := NewResolveTrailBranchUsecase(&fakeAppendPort{})
	err := uc.Execute(context.Background(), uuid.New(), uuid.New(), "  ", "dismissed", goodUUIDv7)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestExecute_PropagatesAppendError(t *testing.T) {
	uc := NewResolveTrailBranchUsecase(&fakeAppendPort{err: errors.New("sovereign down")})
	err := uc.Execute(context.Background(), uuid.New(), uuid.New(), "b", "taken", goodUUIDv7)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrInvalidRequest, "an append failure is not a client validation error")
}
