package recall_snooze_usecase

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

func (m *mockCandidatePort) SnoozeRecallCandidate(_ context.Context, _ uuid.UUID, _ string, _ time.Time, _ time.Time) error {
	m.rec.calls = append(m.rec.calls, "snooze")
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

// SnoozeRecallCandidate must succeed before AppendKnowledgeEvent so a bogus or
// unassociated itemKey is rejected at the read-model gate without appending a
// poison-pill event to knowledge_events.
func TestRecallSnoozeUsecase_Execute_WritesReadModelBeforeAppendingEvent(t *testing.T) {
	rec := &recorder{}
	candidatePort := &mockCandidatePort{rec: rec}
	eventPort := &mockEventPort{rec: rec, eventSeq: 42}

	err := NewRecallSnoozeUsecase(candidatePort, eventPort).
		Execute(context.Background(), uuid.New(), uuid.New(), "article:1", 12)

	require.NoError(t, err)
	assert.Equal(t, []string{"snooze", "append"}, rec.calls)
	require.Len(t, eventPort.events, 1)
	assert.Equal(t, domain.EventRecallSnoozed, eventPort.events[0].EventType)
}

func TestRecallSnoozeUsecase_Execute_DoesNotAppendEventWhenCandidateSnoozeFails(t *testing.T) {
	rec := &recorder{}
	candidatePort := &mockCandidatePort{rec: rec, err: errors.New("recall home association denied or missing")}
	eventPort := &mockEventPort{rec: rec, eventSeq: 42}

	err := NewRecallSnoozeUsecase(candidatePort, eventPort).
		Execute(context.Background(), uuid.New(), uuid.New(), "article:bogus", 12)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "recall home association denied or missing")
	assert.Equal(t, []string{"snooze"}, rec.calls)
	assert.Empty(t, eventPort.events, "must not append an event when candidate snooze fails")
}

func TestRecallSnoozeUsecase_Execute_FailsWhenAppendFails(t *testing.T) {
	rec := &recorder{}
	candidatePort := &mockCandidatePort{rec: rec}
	eventPort := &mockEventPort{rec: rec, err: errors.New("sovereign unavailable")}

	err := NewRecallSnoozeUsecase(candidatePort, eventPort).
		Execute(context.Background(), uuid.New(), uuid.New(), "article:1", 12)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "sovereign unavailable")
	assert.Equal(t, []string{"snooze", "append"}, rec.calls)
}
