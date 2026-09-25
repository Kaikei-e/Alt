package datahub_capability_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
	"alt/shared/driver/alt_db"
)

// ---------------------------------------------------------------------------
// §2.A Outbox
// ---------------------------------------------------------------------------

// outboxDriver is the slice of alt_db the outbox capabilities use.
type outboxDriver interface {
	FetchAndLockPendingOutboxEvents(ctx context.Context, limit int) ([]alt_db.OutboxEvent, error)
	UpdateOutboxEventStatus(ctx context.Context, id string, status string, errorMessage *string) error
	PruneOutboxEvents(ctx context.Context, olderThan time.Duration) (int64, error)
}

// OutboxGateway implements datahub_capability_port.OutboxPort.
type OutboxGateway struct {
	db outboxDriver
}

func NewOutboxGateway(db *alt_db.AltDBRepository) *OutboxGateway {
	return &OutboxGateway{db: db}
}

func (g *OutboxGateway) ClaimBatch(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	rows, err := g.db.FetchAndLockPendingOutboxEvents(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("claim outbox batch: %w", err)
	}

	return outboxEventsFromDriver(rows), nil
}

func (g *OutboxGateway) MarkProcessed(ctx context.Context, id string, status domain.OutboxEventStatus, errorMessage string) error {
	errPtr := outboxErrorPtr(errorMessage)
	if err := g.db.UpdateOutboxEventStatus(ctx, id, string(status), errPtr); err != nil {
		return fmt.Errorf("mark outbox event %s as %s: %w", id, status, err)
	}
	return nil
}

func (g *OutboxGateway) Release(ctx context.Context, id string) error {
	if err := g.db.UpdateOutboxEventStatus(ctx, id, string(domain.OutboxPending), nil); err != nil {
		return fmt.Errorf("release outbox event %s: %w", id, err)
	}
	return nil
}

func (g *OutboxGateway) Prune(ctx context.Context, olderThan time.Duration) (int64, error) {
	pruned, err := g.db.PruneOutboxEvents(ctx, olderThan)
	if err != nil {
		return 0, fmt.Errorf("prune outbox events: %w", err)
	}
	return pruned, nil
}

func outboxEventsFromDriver(rows []alt_db.OutboxEvent) []domain.OutboxEvent {
	events := make([]domain.OutboxEvent, 0, len(rows))
	for _, r := range rows {
		events = append(events, domain.OutboxEvent{
			ID:        r.ID,
			EventType: r.EventType,
			Payload:   r.Payload,
			// The driver reports the pre-claim status it selected on. The
			// rows are PROCESSING by the time the transaction commits, and
			// that is what the caller must see: a response saying PENDING
			// would describe a row nobody else can take.
			Status:    domain.OutboxProcessing,
			CreatedAt: r.CreatedAt,
		})
	}
	return events
}

func outboxErrorPtr(errorMessage string) *string {
	if errorMessage != "" {
		return &errorMessage
	}
	return nil
}
