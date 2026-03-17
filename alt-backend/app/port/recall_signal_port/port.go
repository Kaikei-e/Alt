package recall_signal_port

import (
	"alt/domain"
	"context"

	"github.com/google/uuid"
)

// AppendRecallSignalPort writes a recall signal.
type AppendRecallSignalPort interface {
	AppendRecallSignal(ctx context.Context, signal domain.RecallSignal) error
}

// ListRecallSignalsByUserPort reads recall signals for a user within a time window.
type ListRecallSignalsByUserPort interface {
	ListRecallSignalsByUser(ctx context.Context, userID uuid.UUID, sinceDays int) ([]domain.RecallSignal, error)
}
