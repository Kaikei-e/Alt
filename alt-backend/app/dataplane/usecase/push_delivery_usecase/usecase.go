// Package push_delivery_usecase owns the Web Push delivery queue's state
// machine on the provider side of services.datahub.v1.
//
// It exists for the reason outbox_usecase does and no other: there is a state
// machine here spread across several driver calls, and once the queue is
// behind an RPC the callers are on the far side of a network, so "no caller
// happens to ask for an illegal transition" stops being a property of the code
// and becomes a property of the deployment. The rules live on the side that
// owns the table.
//
// The sibling push_subscriptions capability deliberately has no usecase: every
// one of its operations is a single statement whose invariants are already in
// the SQL, and a usecase there would only forward.
package push_delivery_usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"alt/dataplane/port/datahub_capability_port"
	"alt/domain"
)

// Claim sizing and lease bounds. A requested value of zero means "the caller
// did not say" — an omitted protobuf field and an explicit zero are the same
// bytes — and an unbounded one would turn a single request into a table-sized
// lock, or into a lease long enough that a crashed dispatcher's rows sit
// undeliverable for hours.
const (
	DefaultClaimLimit = 50
	MaxClaimLimit     = 500
	DefaultLease      = 60 * time.Second
	MaxLease          = 15 * time.Minute
)

var (
	// ErrMissingID is returned when a per-row operation arrives with no id.
	ErrMissingID = errors.New("push delivery: id is required")
	// ErrMissingLockedBy is returned for a claim with no owner. locked_by is
	// the only forensic link from a stuck row to the process holding it, and a
	// claim that left it empty would produce rows nobody can be shown to own.
	ErrMissingLockedBy = errors.New("push delivery: locked_by is required")
	// ErrInvalidEnqueue covers every malformed fan-out request.
	ErrInvalidEnqueue = errors.New("push delivery: invalid enqueue")
	// ErrUnknownKind is returned for a kind outside the four the product
	// sends. It is not pedantry: the partial unique index that supersedes an
	// unsent daily digest names 'today_entrance_ready' literally, and so does
	// the preference column the fan-out reads, so a misspelled kind would
	// silently fan out to nobody — or stack digests — and nothing downstream
	// would look wrong.
	ErrUnknownKind = errors.New("push delivery: unknown kind")
	// ErrMissingNextAttempt is returned for a release with no schedule.
	// Releasing without one returns the row to a claim that fires immediately,
	// which is a spin rather than a backoff.
	ErrMissingNextAttempt = errors.New("push delivery: next_attempt_at is required")
)

// PushDeliveryUsecase serves the five delivery-queue procedures.
type PushDeliveryUsecase struct {
	port datahub_capability_port.PushDeliveryPort
}

// NewPushDeliveryUsecase panics on a nil port rather than storing it.
//
// A nil port would make ClaimBatch answer "nothing due", which is exactly what
// a healthy drained queue looks like: the dispatcher would tick, log success,
// and deliver no notification, forever (CLAUDE.md rule 8 / ADR-000928).
func NewPushDeliveryUsecase(port datahub_capability_port.PushDeliveryPort) *PushDeliveryUsecase {
	if port == nil {
		panic("push_delivery_usecase: PushDeliveryPort is required — a nil port makes " +
			"an unwired delivery queue indistinguishable from an empty one " +
			"(see .claude/rules/di-wiring.md)")
	}
	return &PushDeliveryUsecase{port: port}
}

// Enqueue fans one notification out to a user's devices.
//
// occurred_at and expires_at are both required and neither is defaulted here.
// occurred_at is business time — when the fact happened — and substituting a
// reading of this process's clock would record when the row was written, which
// is a different fact that happens to look plausible. expires_at is the
// producer's judgement about how long delivery stays worth attempting, and a
// table-wide default would apply a job-finished ping's shelf life to a daily
// digest.
//
// A zero delivery count is an ordinary answer, not an error: the user may have
// no device, may have turned the kind off, or the enqueue may have been
// relayed twice.
func (u *PushDeliveryUsecase) Enqueue(ctx context.Context, in domain.NotificationEnqueue) (int, int, error) {
	if err := validateEnqueue(in); err != nil {
		return 0, 0, err
	}

	delivered, superseded, err := u.port.Enqueue(ctx, in)
	if err != nil {
		return 0, 0, fmt.Errorf("enqueue notification %s: %w", in.DedupeKey, err)
	}
	return delivered, superseded, nil
}

func validateEnqueue(in domain.NotificationEnqueue) error {
	switch {
	case in.DedupeKey == "":
		return fmt.Errorf("%w: dedupe_key is required and must be derived from the business fact, "+
			"so a relayed retry produces the same key", ErrInvalidEnqueue)
	case in.UserID == "":
		return fmt.Errorf("%w: user_id is required", ErrInvalidEnqueue)
	case in.OccurredAt.IsZero():
		return fmt.Errorf("%w: occurred_at is required; it is business time and the provider "+
			"does not substitute its own clock", ErrInvalidEnqueue)
	case in.ExpiresAt.IsZero():
		return fmt.Errorf("%w: expires_at is required", ErrInvalidEnqueue)
	case !in.ExpiresAt.After(in.OccurredAt):
		return fmt.Errorf("%w: expires_at %s is not after occurred_at %s, so every row would be "+
			"expired by the first claim that saw it", ErrInvalidEnqueue, in.ExpiresAt, in.OccurredAt)
	}
	if !isKnownKind(in.Kind) {
		return fmt.Errorf("%w: %q", ErrUnknownKind, in.Kind)
	}
	return nil
}

func isKnownKind(kind string) bool {
	switch kind {
	case domain.NotificationKindSummaryReady,
		domain.NotificationKindAcolyteReportReady,
		domain.NotificationKindRecapReady,
		domain.NotificationKindTodayEntranceReady:
		return true
	default:
		return false
	}
}

// ClaimBatch takes the lease on due rows, clamping the requested size and
// lease window.
func (u *PushDeliveryUsecase) ClaimBatch(ctx context.Context, lockedBy string, limit int, lease time.Duration) ([]domain.PushDelivery, error) {
	if lockedBy == "" {
		return nil, ErrMissingLockedBy
	}

	deliveries, err := u.port.ClaimBatch(ctx, lockedBy, clampClaimLimit(limit), clampLease(lease))
	if err != nil {
		return nil, fmt.Errorf("claim push delivery batch: %w", err)
	}
	return deliveries, nil
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

func clampLease(lease time.Duration) time.Duration {
	switch {
	case lease <= 0:
		return DefaultLease
	case lease > MaxLease:
		return MaxLease
	default:
		return lease
	}
}

// MarkSent records delivery for a claimed row.
func (u *PushDeliveryUsecase) MarkSent(ctx context.Context, id string, statusCode int) error {
	if id == "" {
		return ErrMissingID
	}
	if err := u.port.MarkSent(ctx, id, statusCode); err != nil {
		return fmt.Errorf("mark push delivery %s sent: %w", id, err)
	}
	return nil
}

// Release returns a claimed row to PENDING at the caller's next attempt time.
func (u *PushDeliveryUsecase) Release(ctx context.Context, id string, nextAttemptAt time.Time, errorMessage string) error {
	if id == "" {
		return ErrMissingID
	}
	if nextAttemptAt.IsZero() {
		return ErrMissingNextAttempt
	}
	if err := u.port.Release(ctx, id, nextAttemptAt, errorMessage); err != nil {
		return fmt.Errorf("release push delivery %s: %w", id, err)
	}
	return nil
}

// BacklogAge reports how stale the queue is and how many non-terminal rows it
// holds.
//
// There is nothing to validate and nothing to clamp: it takes no argument, and
// both numbers are readings. It lives here rather than beside the subscription
// reads because it is a question about this state machine — the answer is only
// correct if it counts the `sending` rows a crashed dispatcher left behind, and
// that rule belongs with the rest of the queue's rules.
func (u *PushDeliveryUsecase) BacklogAge(ctx context.Context) (time.Duration, int64, error) {
	oldest, pending, err := u.port.BacklogAge(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("read push delivery backlog age: %w", err)
	}
	return oldest, pending, nil
}

// MarkDead ends delivery for a failure that will not improve.
func (u *PushDeliveryUsecase) MarkDead(ctx context.Context, id string, statusCode int, errorMessage string) error {
	if id == "" {
		return ErrMissingID
	}
	if err := u.port.MarkDead(ctx, id, statusCode, errorMessage); err != nil {
		return fmt.Errorf("mark push delivery %s dead: %w", id, err)
	}
	return nil
}
