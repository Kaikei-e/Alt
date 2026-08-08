package service

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"pre-processor/domain"
	"pre-processor/metrics"
	"pre-processor/repository"
)

const (
	// relayBatchSize bounds how many rows one tick claims.
	relayBatchSize = 50
	// relayInterval is the tick cadence of the relay loop.
	relayInterval = 15 * time.Second
	// relayLease is how long a claim holds a row. It has to comfortably exceed
	// the forward RPC's own timeout, or a slow-but-succeeding call would race a
	// second relay reclaiming the same row.
	relayLease = 2 * time.Minute
	// relayBackoffBase and relayBackoffCap bound the full-jitter retry window.
	relayBackoffBase = 10 * time.Second
	relayBackoffCap  = time.Hour
	// relayMaxAttempts is where a row stops being retried and becomes 'dead'.
	relayMaxAttempts = 8
	// notificationTTL is how long a summary_ready notification is worth
	// delivering. A "your summary is ready" ping the user never saw within a
	// day is noise by the time it arrives.
	notificationTTL = 24 * time.Hour
)

// notificationRelayEnabledLog is the startup line that states the relay's
// wiring state. It is always emitted, so "the relay is off" can never be
// inferred from silence (CLAUDE.md rule 8).
const notificationRelayEnabledLog = "notification_outbox_relay_enabled"

// NotificationForwarder sends one claimed outbox row to alt-data-hub.
type NotificationForwarder interface {
	EnqueueNotification(ctx context.Context, row domain.NotificationOutboxRow, expiresAt time.Time) error
}

// NotificationRelay drains notification_outbox to alt-data-hub.
//
// It is deliberately at-least-once. Claiming, forwarding and marking cannot be
// one atomic step across two databases and a network, so the relay optimises
// for never losing a notification and leans on the derived dedupe_key to make
// a repeat harmless downstream.
type NotificationRelay struct {
	outbox    repository.NotificationOutboxRepository
	forwarder NotificationForwarder
	metrics   *metrics.OutboxRelayMetrics
	logger    *slog.Logger
	name      string

	clock func() time.Time
	// jitter draws the actual sleep from [0, bound). Injected so a test can
	// pin the schedule while the envelope itself stays random in production.
	jitter func(bound time.Duration) time.Duration
}

// NewNotificationRelay wires the relay and fails loudly if any collaborator is
// missing, so a half-wired relay cannot start and then quietly forward nothing.
func NewNotificationRelay(
	outbox repository.NotificationOutboxRepository,
	forwarder NotificationForwarder,
	relayMetrics *metrics.OutboxRelayMetrics,
	name string,
	logger *slog.Logger,
) (*NotificationRelay, error) {
	if outbox == nil {
		return nil, fmt.Errorf("notification relay: outbox repository is not wired")
	}
	if forwarder == nil {
		return nil, fmt.Errorf("notification relay: data hub forwarder is not wired")
	}
	if relayMetrics == nil {
		return nil, fmt.Errorf("notification relay: metrics are not wired")
	}
	if name == "" {
		return nil, fmt.Errorf("notification relay: name is required for locked_by")
	}

	return &NotificationRelay{
		outbox:    outbox,
		forwarder: forwarder,
		metrics:   relayMetrics,
		logger:    logger,
		name:      name,
		clock:     time.Now,
		jitter:    randomBelow,
	}, nil
}

// LogStartup states the relay's configuration on one loud line.
func (r *NotificationRelay) LogStartup(ctx context.Context) {
	r.logger.InfoContext(ctx, notificationRelayEnabledLog,
		"relay", r.name,
		"interval", relayInterval,
		"batch_size", relayBatchSize,
		"lease", relayLease,
		"max_attempts", relayMaxAttempts)
}

// Interval is the tick cadence the job runner should use.
func (r *NotificationRelay) Interval() time.Duration { return relayInterval }

// Tick claims one batch and forwards it.
//
// Both gauges are published before any early return: a tick that failed still
// happened, and the freshness gauge is only useful if it says so.
func (r *NotificationRelay) Tick(ctx context.Context) error {
	defer r.publishGauges(ctx)

	claimed, err := r.outbox.ClaimBatch(ctx, r.name, relayBatchSize, relayLease)
	if err != nil {
		return fmt.Errorf("claim notification batch: %w", err)
	}
	if len(claimed) == 0 {
		return nil
	}

	// The claim transaction is committed by the time ClaimBatch returns, so
	// every call below runs outside it.
	for _, row := range claimed {
		r.forward(ctx, row)
	}

	return nil
}

func (r *NotificationRelay) forward(ctx context.Context, row domain.NotificationOutboxRow) {
	if r.forwarder == nil {
		panic("notification relay: forwarder is not wired but a claimed row reached the forward path")
	}

	err := r.forwarder.EnqueueNotification(ctx, row, row.OccurredAt.Add(notificationTTL))
	if err == nil {
		if markErr := r.outbox.MarkForwarded(ctx, row.ID); markErr != nil {
			// The row stays claimed and its lease will expire, so the next
			// relay re-forwards it. dedupe_key makes that harmless.
			r.logger.ErrorContext(ctx, "forwarded notification but failed to mark it",
				"error", markErr, "outbox_id", row.ID, "dedupe_key", row.DedupeKey)
		}
		return
	}

	if row.Attempts >= relayMaxAttempts {
		reason := fmt.Sprintf("gave up after %d attempts: %v", row.Attempts, err)
		r.logger.ErrorContext(ctx, "notification exhausted its retries, moving to dead",
			"error", err, "outbox_id", row.ID, "dedupe_key", row.DedupeKey, "attempts", row.Attempts)
		if markErr := r.outbox.MarkDead(ctx, row.ID, reason); markErr != nil {
			r.logger.ErrorContext(ctx, "failed to mark notification dead",
				"error", markErr, "outbox_id", row.ID)
		}
		return
	}

	retryIn := r.jitter(notificationBackoffBound(row.Attempts))
	r.logger.WarnContext(ctx, "failed to forward notification, scheduling retry",
		"error", err, "outbox_id", row.ID, "attempts", row.Attempts, "retry_in", retryIn)
	if markErr := r.outbox.MarkAttemptFailed(ctx, row.ID, retryIn, err.Error()); markErr != nil {
		r.logger.ErrorContext(ctx, "failed to record notification retry",
			"error", markErr, "outbox_id", row.ID)
	}
}

// publishGauges writes both series on every tick, including a backlog age of
// 0. A gauge that stops being written keeps reporting its last value, which is
// exactly how a wedged relay goes on looking healthy.
func (r *NotificationRelay) publishGauges(ctx context.Context) {
	age, err := r.outbox.OldestPendingAge(ctx)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to read notification outbox backlog age", "error", err)
		// The tick is still recorded below; only the backlog number is unknown.
		age = 0
	}
	r.metrics.ObserveTick(age, r.clock())
}

// notificationBackoffBound is the full-jitter envelope: min(cap, base*2^attempt).
func notificationBackoffBound(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	bound := relayBackoffBase
	for i := 0; i < attempt; i++ {
		bound *= 2
		if bound >= relayBackoffCap {
			return relayBackoffCap
		}
	}
	return bound
}

// randomBelow draws the actual sleep uniformly from [0, bound).
func randomBelow(bound time.Duration) time.Duration {
	if bound <= 0 {
		return 0
	}
	// #nosec G404 -- retry jitter spreads load; it is not a security decision.
	return time.Duration(rand.Int64N(int64(bound)))
}
