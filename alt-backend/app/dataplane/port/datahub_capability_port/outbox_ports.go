package datahub_capability_port

import (
	"context"
	"time"

	"alt/domain"
)

// OutboxPort is the transactional outbox's state machine (catalog §2.A).
//
// Claim is a capability rather than a read: the lock and the status change
// belong to one transaction, and the interface says so by having no separate
// "mark processing" method for a caller to forget.
type OutboxPort interface {
	// ClaimBatch selects up to limit PENDING rows FOR UPDATE SKIP LOCKED and
	// marks them PROCESSING in the same transaction.
	ClaimBatch(ctx context.Context, limit int) ([]domain.OutboxEvent, error)
	// MarkProcessed records a terminal status and stamps processed_at.
	MarkProcessed(ctx context.Context, id string, status domain.OutboxEventStatus, errorMessage string) error
	// Release returns a claimed row to PENDING.
	Release(ctx context.Context, id string) error
	// Prune deletes PROCESSED rows older than the window and reports how many.
	Prune(ctx context.Context, olderThan time.Duration) (int64, error)
}
