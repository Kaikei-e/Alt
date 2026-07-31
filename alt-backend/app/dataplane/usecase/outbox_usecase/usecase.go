// Package outbox_usecase owns the outbox state machine on the provider side
// of alt.datahub.v1 (ADR-000954 Wave 3, catalog §2.A).
//
// The transitions used to be enforced nowhere. The driver took a
// `status string` and wrote whatever it was given; the only reason a row never
// went from PROCESSED back to PENDING was that no caller happened to ask. With
// the outbox behind an RPC the callers are on the other side of a network, so
// "no caller happens to ask" stops being a property of the code and becomes a
// property of the deployment. The rules live here, on the side that owns the
// table (ADR-000954 D4).
package outbox_usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"alt/dataplane/port/datahub_capability_port"
	"alt/domain"
)

// Claim batch sizing. The requested size arrives from the wire: zero means
// "caller did not say", and an unbounded value would turn one request into a
// table-sized lock held for the length of the transaction.
const (
	DefaultClaimLimit = 10
	MaxClaimLimit     = 500
)

var (
	// ErrNotTerminalStatus is returned when MarkProcessed is asked to write a
	// status that is not PROCESSED or FAILED. PENDING is ReleaseOutboxEvent's
	// job and PROCESSING is ClaimOutboxBatch's; each transition has exactly
	// one procedure.
	ErrNotTerminalStatus = errors.New("outbox: status is not terminal")
	// ErrInvalidRetention is returned for a zero or negative retention window.
	// Zero would delete every PROCESSED row including ones written seconds
	// ago, and an omitted protobuf field is indistinguishable from an explicit
	// zero — so it must not mean "delete everything".
	ErrInvalidRetention = errors.New("outbox: retention window must be positive")
	// ErrMissingID is returned when a per-row operation arrives with no id.
	ErrMissingID = errors.New("outbox: event id is required")
)

// OutboxUsecase serves the four outbox procedures.
type OutboxUsecase struct {
	port datahub_capability_port.OutboxPort
}

// NewOutboxUsecase panics on a nil port rather than storing it.
//
// A nil port here would make ClaimBatch answer "no pending events", which is
// exactly what a healthy drained outbox looks like: the harvester would tick
// every five seconds, log success, and deliver nothing, forever
// (CLAUDE.md rule 8 / ADR-000928).
func NewOutboxUsecase(port datahub_capability_port.OutboxPort) *OutboxUsecase {
	if port == nil {
		panic("outbox_usecase: OutboxPort is required — a nil port makes an unwired outbox " +
			"indistinguishable from an empty one (see .claude/rules/di-wiring.md)")
	}
	return &OutboxUsecase{port: port}
}

// ClaimBatch takes ownership of pending events, clamping the requested size.
func (u *OutboxUsecase) ClaimBatch(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	events, err := u.port.ClaimBatch(ctx, clampClaimLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("claim outbox batch: %w", err)
	}
	return events, nil
}

func clampClaimLimit(limit int) int {
	switch {
	case limit <= 0:
		return DefaultClaimLimit
	case limit > MaxClaimLimit:
		return MaxClaimLimit
	default:
		return limit
	}
}

// MarkProcessed records a terminal outcome for a claimed event.
func (u *OutboxUsecase) MarkProcessed(ctx context.Context, id string, status domain.OutboxEventStatus, errorMessage string) error {
	if id == "" {
		return ErrMissingID
	}
	if !status.IsTerminal() {
		return fmt.Errorf("%w: %q (PENDING is ReleaseOutboxEvent, PROCESSING is ClaimOutboxBatch)",
			ErrNotTerminalStatus, status)
	}

	if err := u.port.MarkProcessed(ctx, id, status, errorMessage); err != nil {
		return fmt.Errorf("mark outbox event %s as %s: %w", id, status, err)
	}
	return nil
}

// Release returns a claimed-but-unattempted event to PENDING.
func (u *OutboxUsecase) Release(ctx context.Context, id string) error {
	if id == "" {
		return ErrMissingID
	}
	if err := u.port.Release(ctx, id); err != nil {
		return fmt.Errorf("release outbox event %s: %w", id, err)
	}
	return nil
}

// Prune deletes PROCESSED events older than the caller's retention window.
func (u *OutboxUsecase) Prune(ctx context.Context, olderThan time.Duration) (int64, error) {
	if olderThan <= 0 {
		return 0, fmt.Errorf("%w: got %s", ErrInvalidRetention, olderThan)
	}

	pruned, err := u.port.Prune(ctx, olderThan)
	if err != nil {
		return 0, fmt.Errorf("prune outbox events: %w", err)
	}
	return pruned, nil
}
