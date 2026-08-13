package recall_dismiss_usecase

import (
	"alt/domain"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recorder struct {
	calls []string
}

type mockCandidatePort struct {
	rec *recorder
	err error
}

func (m *mockCandidatePort) DismissRecallCandidate(_ context.Context, _ uuid.UUID, _ string, _ time.Time) error {
	m.rec.calls = append(m.rec.calls, "dismiss")
	return m.err
}

type mockEventPort struct {
	rec      *recorder
	events   []domain.KnowledgeEvent
	err      error
	eventSeq int64
}

func (m *mockEventPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) (int64, error) {
	m.rec.calls = append(m.rec.calls, "append")
	if m.err != nil {
		return 0, m.err
	}
	m.events = append(m.events, event)
	return m.eventSeq, nil
}

// recall_candidate_view is TRUNCATEd on reproject and rebuilt from the event
// log, so a dismiss that only reached the read model resurrects. The append is
// the durable write and must come first.
func TestRecallDismissUsecase_Execute_AppendsBeforeReadModelWrite(t *testing.T) {
	rec := &recorder{}
	candidatePort := &mockCandidatePort{rec: rec}
	eventPort := &mockEventPort{rec: rec, eventSeq: 42}

	err := NewRecallDismissUsecase(candidatePort, eventPort).
		Execute(context.Background(), uuid.New(), uuid.New(), "article:1")

	require.NoError(t, err)
	assert.Equal(t, []string{"append", "dismiss"}, rec.calls)
	require.Len(t, eventPort.events, 1)
	assert.Equal(t, domain.EventRecallDismissed, eventPort.events[0].EventType)
}

func TestRecallDismissUsecase_Execute_FailsWhenAppendFails(t *testing.T) {
	rec := &recorder{}
	candidatePort := &mockCandidatePort{rec: rec}
	eventPort := &mockEventPort{rec: rec, err: errors.New("sovereign unavailable")}

	err := NewRecallDismissUsecase(candidatePort, eventPort).
		Execute(context.Background(), uuid.New(), uuid.New(), "article:1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "sovereign unavailable")
	assert.Equal(t, []string{"append"}, rec.calls)
}
