package outbox_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"alt/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubOutboxPort struct {
	claimed     []domain.OutboxEvent
	claimLimit  int
	claimErr    error
	markCalls   []markCall
	markErr     error
	releaseIDs  []string
	releaseErr  error
	prunedCount int64
	prunedOlder time.Duration
	pruneErr    error
}

type markCall struct {
	id      string
	status  domain.OutboxEventStatus
	message string
}

func (s *stubOutboxPort) ClaimBatch(_ context.Context, limit int) ([]domain.OutboxEvent, error) {
	s.claimLimit = limit
	return s.claimed, s.claimErr
}

func (s *stubOutboxPort) MarkProcessed(_ context.Context, id string, status domain.OutboxEventStatus, message string) error {
	s.markCalls = append(s.markCalls, markCall{id: id, status: status, message: message})
	return s.markErr
}

func (s *stubOutboxPort) Release(_ context.Context, id string) error {
	s.releaseIDs = append(s.releaseIDs, id)
	return s.releaseErr
}

func (s *stubOutboxPort) Prune(_ context.Context, olderThan time.Duration) (int64, error) {
	s.prunedOlder = olderThan
	return s.prunedCount, s.pruneErr
}

func TestNewUsecaseRejectsNilPort(t *testing.T) {
	assert.Panics(t, func() { NewOutboxUsecase(nil) },
		"a nil port would make every claim answer 'no pending events', which is indistinguishable from a drained outbox")
}

// TestClaimBatchClampsLimit: the batch size arrives from the wire, and an
// unbounded one turns a single request into a table-sized lock held across the
// whole transaction.
func TestClaimBatchClampsLimit(t *testing.T) {
	tests := []struct {
		name      string
		requested int
		want      int
	}{
		{name: "zero uses the default", requested: 0, want: DefaultClaimLimit},
		{name: "negative uses the default", requested: -5, want: DefaultClaimLimit},
		{name: "within range is honoured", requested: 25, want: 25},
		{name: "above the cap is clamped", requested: 5000, want: MaxClaimLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := &stubOutboxPort{}
			uc := NewOutboxUsecase(port)

			_, err := uc.ClaimBatch(context.Background(), tt.requested)
			require.NoError(t, err)
			assert.Equal(t, tt.want, port.claimLimit)
		})
	}
}

func TestClaimBatchReturnsClaimedEvents(t *testing.T) {
	port := &stubOutboxPort{claimed: []domain.OutboxEvent{
		{ID: "e1", EventType: "ARTICLE_UPSERT", Status: domain.OutboxProcessing},
	}}
	uc := NewOutboxUsecase(port)

	events, err := uc.ClaimBatch(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, domain.OutboxProcessing, events[0].Status)
}

func TestClaimBatchWrapsPortError(t *testing.T) {
	sentinel := errors.New("pool exhausted")
	uc := NewOutboxUsecase(&stubOutboxPort{claimErr: sentinel})

	_, err := uc.ClaimBatch(context.Background(), 10)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// TestMarkProcessedRejectsNonTerminalStatus is the state machine as a test.
// PENDING and PROCESSING each have their own procedure, and accepting them
// here would let a caller drive the outbox into a state no procedure owns —
// the failure the bare `status string` driver made possible.
func TestMarkProcessedRejectsNonTerminalStatus(t *testing.T) {
	tests := []struct {
		name   string
		status domain.OutboxEventStatus
	}{
		{name: "pending belongs to Release", status: domain.OutboxPending},
		{name: "processing belongs to ClaimBatch", status: domain.OutboxProcessing},
		{name: "unspecified is not a transition", status: domain.OutboxEventStatus("")},
		{name: "unknown value", status: domain.OutboxEventStatus("RETRYING")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := &stubOutboxPort{}
			uc := NewOutboxUsecase(port)

			err := uc.MarkProcessed(context.Background(), "e1", tt.status, "")
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrNotTerminalStatus)
			assert.Empty(t, port.markCalls, "the write must not reach the database")
		})
	}
}

func TestMarkProcessedAcceptsTerminalStatuses(t *testing.T) {
	tests := []struct {
		name    string
		status  domain.OutboxEventStatus
		message string
	}{
		{name: "processed", status: domain.OutboxProcessed},
		{name: "failed carries the reason", status: domain.OutboxFailed, message: "rag upsert refused: 503"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := &stubOutboxPort{}
			uc := NewOutboxUsecase(port)

			require.NoError(t, uc.MarkProcessed(context.Background(), "e1", tt.status, tt.message))
			require.Len(t, port.markCalls, 1)
			assert.Equal(t, tt.status, port.markCalls[0].status)
			assert.Equal(t, tt.message, port.markCalls[0].message)
		})
	}
}

func TestMarkProcessedRequiresID(t *testing.T) {
	port := &stubOutboxPort{}
	uc := NewOutboxUsecase(port)

	err := uc.MarkProcessed(context.Background(), "", domain.OutboxProcessed, "")
	require.Error(t, err)
	assert.Empty(t, port.markCalls)
}

func TestReleaseReturnsEventToPending(t *testing.T) {
	port := &stubOutboxPort{}
	uc := NewOutboxUsecase(port)

	require.NoError(t, uc.Release(context.Background(), "e1"))
	assert.Equal(t, []string{"e1"}, port.releaseIDs)
}

func TestReleaseRequiresID(t *testing.T) {
	port := &stubOutboxPort{}
	uc := NewOutboxUsecase(port)

	require.Error(t, uc.Release(context.Background(), ""))
	assert.Empty(t, port.releaseIDs)
}

// TestPruneRejectsNonPositiveRetention: a zero or negative window would delete
// every PROCESSED row, including ones written seconds ago. The retention
// window arrives from the wire, so an omitted field must not read as "delete
// everything".
func TestPruneRejectsNonPositiveRetention(t *testing.T) {
	tests := []struct {
		name      string
		retention time.Duration
	}{
		{name: "zero", retention: 0},
		{name: "negative", retention: -time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := &stubOutboxPort{}
			uc := NewOutboxUsecase(port)

			_, err := uc.Prune(context.Background(), tt.retention)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidRetention)
			assert.Zero(t, port.prunedOlder, "the delete must not reach the database")
		})
	}
}

func TestPruneReportsDeletedRows(t *testing.T) {
	port := &stubOutboxPort{prunedCount: 12}
	uc := NewOutboxUsecase(port)

	pruned, err := uc.Prune(context.Background(), 7*24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, int64(12), pruned)
	assert.Equal(t, 7*24*time.Hour, port.prunedOlder)
}
