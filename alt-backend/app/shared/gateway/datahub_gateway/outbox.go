package datahub_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
	datahubv1 "alt/gen/proto/alt/datahub/v1"
	"alt/gen/proto/alt/datahub/v1/datahubv1connect"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
)

// OutboxGateway is alt-harvester's view of the transactional outbox
// (catalog §2.A).
//
// The claim is the reason this is a capability and not a pair of reads and
// writes: ClaimOutboxBatch holds FOR UPDATE SKIP LOCKED across the select and
// the status update, inside one transaction on the provider. Splitting it
// would put a network round trip inside the lock window and let two harvesters
// take the same event.
type OutboxGateway struct {
	client datahubv1connect.DataHubServiceClient
}

// NewOutboxGateway wires the gateway to the shared DataHubService client.
func NewOutboxGateway(client datahubv1connect.DataHubServiceClient) *OutboxGateway {
	if client == nil {
		panic("datahub_gateway: OutboxGateway requires a DataHubService client — " +
			"a nil client would make every outbox tick fail identically to an empty outbox " +
			"(see .claude/rules/di-wiring.md)")
	}
	return &OutboxGateway{client: client}
}

// ClaimBatch takes ownership of up to limit PENDING events, marking them
// PROCESSING in the same transaction that selects them.
func (g *OutboxGateway) ClaimBatch(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	resp, err := g.client.ClaimOutboxBatch(ctx, connect.NewRequest(&datahubv1.ClaimOutboxBatchRequest{
		Limit: safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, fmt.Errorf("claim outbox batch: %w", err)
	}

	events := make([]domain.OutboxEvent, 0, len(resp.Msg.GetEvents()))
	for _, e := range resp.Msg.GetEvents() {
		events = append(events, outboxEventFromProto(e))
	}
	return events, nil
}

// MarkProcessed records a terminal outcome for a claimed event.
//
// The status is checked here rather than only on the provider so a caller that
// passes PENDING gets an error naming Release instead of a Connect
// InvalidArgument from three layers away.
func (g *OutboxGateway) MarkProcessed(ctx context.Context, id string, status domain.OutboxEventStatus, errorMessage string) error {
	if !status.IsTerminal() {
		return fmt.Errorf("mark outbox event %s processed: status %q is not terminal "+
			"(use Release to return a claimed event to PENDING)", id, status)
	}

	_, err := g.client.MarkOutboxProcessed(ctx, connect.NewRequest(&datahubv1.MarkOutboxProcessedRequest{
		Id:           id,
		Status:       outboxStatusToProto(status),
		ErrorMessage: errorMessage,
	}))
	if err != nil {
		return fmt.Errorf("mark outbox event %s as %s: %w", id, status, err)
	}
	return nil
}

// Release returns a claimed-but-unattempted event to PENDING.
//
// Callers reach this on a context detached from the cancelled one. The row is
// only reachable through this call: the claim query reads PENDING only, so a
// PROCESSING row nobody releases is never fetched again by anyone.
func (g *OutboxGateway) Release(ctx context.Context, id string) error {
	_, err := g.client.ReleaseOutboxEvent(ctx, connect.NewRequest(&datahubv1.ReleaseOutboxEventRequest{
		Id: id,
	}))
	if err != nil {
		return fmt.Errorf("release outbox event %s to PENDING: %w", id, err)
	}
	return nil
}

// Prune deletes PROCESSED events older than the retention window and returns
// how many rows went.
//
// The window is the caller's: how long a delivered event is worth keeping for
// incident forensics is an operational policy of the pruning job, not an
// invariant of the table.
func (g *OutboxGateway) Prune(ctx context.Context, olderThan time.Duration) (int64, error) {
	resp, err := g.client.PruneOutboxEvents(ctx, connect.NewRequest(&datahubv1.PruneOutboxEventsRequest{
		OlderThanSeconds: int64(olderThan.Seconds()),
	}))
	if err != nil {
		return 0, fmt.Errorf("prune outbox events older than %s: %w", olderThan, err)
	}
	return resp.Msg.GetPrunedCount(), nil
}
