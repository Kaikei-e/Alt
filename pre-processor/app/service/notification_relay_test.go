package service

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pre-processor/domain"
	"pre-processor/metrics"
)

var relayNow = time.Date(2026, 8, 8, 11, 0, 0, 0, time.UTC)

type failedAttempt struct {
	id      string
	retryIn time.Duration
	reason  string
}

type fakeOutbox struct {
	claimed   []domain.NotificationOutboxRow
	claimErr  error
	oldestAge time.Duration
	ageErr    error

	claimCalls int
	forwarded  []string
	failed     []failedAttempt
	dead       []string
	// afterClaim runs when ClaimBatch returns, so a test can observe what the
	// relay had already done by the time it owned rows.
	afterClaim func()
}

func (f *fakeOutbox) ClaimBatch(_ context.Context, _ string, _ int, _ time.Duration) ([]domain.NotificationOutboxRow, error) {
	f.claimCalls++
	if f.afterClaim != nil {
		f.afterClaim()
	}
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	return f.claimed, nil
}

func (f *fakeOutbox) MarkForwarded(_ context.Context, id string) error {
	f.forwarded = append(f.forwarded, id)
	return nil
}

func (f *fakeOutbox) MarkAttemptFailed(_ context.Context, id string, retryIn time.Duration, reason string) error {
	f.failed = append(f.failed, failedAttempt{id: id, retryIn: retryIn, reason: reason})
	return nil
}

func (f *fakeOutbox) MarkDead(_ context.Context, id string, _ string) error {
	f.dead = append(f.dead, id)
	return nil
}

func (f *fakeOutbox) OldestPendingAge(context.Context) (time.Duration, error) {
	return f.oldestAge, f.ageErr
}

type fakeForwarder struct {
	err     error
	rows    []domain.NotificationOutboxRow
	expires []time.Time
}

func (f *fakeForwarder) EnqueueNotification(_ context.Context, row domain.NotificationOutboxRow, expiresAt time.Time) error {
	f.rows = append(f.rows, row)
	f.expires = append(f.expires, expiresAt)
	return f.err
}

func relayTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newTestRelay(t *testing.T, outbox *fakeOutbox, fwd NotificationForwarder) (*NotificationRelay, *metrics.OutboxRelayMetrics) {
	t.Helper()
	m := metrics.NewOutboxRelayMetrics()
	relay, err := NewNotificationRelay(outbox, fwd, m, "relay-test", relayTestLogger())
	require.NoError(t, err)
	relay.clock = func() time.Time { return relayNow }
	// Zero jitter makes the scheduled retry instant assertable; the jitter
	// range itself is covered by TestNotificationBackoffBound below.
	relay.jitter = func(time.Duration) time.Duration { return 0 }
	return relay, m
}

func outboxRow(id string, attempts int) domain.NotificationOutboxRow {
	return domain.NotificationOutboxRow{
		ID:         id,
		DedupeKey:  "summary:" + id,
		UserID:     "11111111-2222-4333-8444-555555555555",
		Kind:       string(domain.NotificationKindSummaryReady),
		Payload:    []byte(`{"kind":"summary_ready","url":"/articles/art-001"}`),
		OccurredAt: relayNow.Add(-time.Minute),
		Attempts:   attempts,
	}
}

func TestNotificationRelay_ForwardsAndMarksForwarded(t *testing.T) {
	outbox := &fakeOutbox{claimed: []domain.NotificationOutboxRow{outboxRow("row-1", 1)}, oldestAge: 90 * time.Second}
	fwd := &fakeForwarder{}
	relay, _ := newTestRelay(t, outbox, fwd)

	require.NoError(t, relay.Tick(context.Background()))

	require.Len(t, fwd.rows, 1)
	assert.Equal(t, "summary:row-1", fwd.rows[0].DedupeKey)
	assert.Equal(t, relayNow.Add(-time.Minute).Add(notificationTTL), fwd.expires[0],
		"expiry is measured from when the thing happened, not from when the relay got to it")
	assert.Equal(t, []string{"row-1"}, outbox.forwarded)
	assert.Empty(t, outbox.failed)
	assert.Empty(t, outbox.dead)
}

// TestNotificationRelay_ClaimIsCommittedBeforeForward asserts the relay does
// not interleave the claim and the RPC: by the time it owns rows, it has made
// no outbound call, and every call it does make happens afterwards.
func TestNotificationRelay_ClaimIsCommittedBeforeForward(t *testing.T) {
	fwd := &fakeForwarder{}
	outbox := &fakeOutbox{claimed: []domain.NotificationOutboxRow{outboxRow("row-1", 1)}}
	outbox.afterClaim = func() {
		assert.Empty(t, fwd.rows, "no notification may be forwarded before the claim has been taken and committed")
	}
	relay, _ := newTestRelay(t, outbox, fwd)

	require.NoError(t, relay.Tick(context.Background()))
	assert.Len(t, fwd.rows, 1)
}

func TestNotificationRelay_SchedulesBackoffOnForwardFailure(t *testing.T) {
	outbox := &fakeOutbox{claimed: []domain.NotificationOutboxRow{outboxRow("row-1", 2)}}
	fwd := &fakeForwarder{err: errors.New("data hub unavailable")}
	relay, _ := newTestRelay(t, outbox, fwd)

	require.NoError(t, relay.Tick(context.Background()))

	require.Len(t, outbox.failed, 1)
	assert.Equal(t, "row-1", outbox.failed[0].id)
	assert.Equal(t, time.Duration(0), outbox.failed[0].retryIn, "zero jitter schedules the retry immediately")
	assert.Contains(t, outbox.failed[0].reason, "data hub unavailable")
	assert.Empty(t, outbox.forwarded)
	assert.Empty(t, outbox.dead)
}

func TestNotificationRelay_DeadLettersAfterMaxAttempts(t *testing.T) {
	outbox := &fakeOutbox{claimed: []domain.NotificationOutboxRow{outboxRow("row-1", relayMaxAttempts)}}
	fwd := &fakeForwarder{err: errors.New("data hub unavailable")}
	relay, _ := newTestRelay(t, outbox, fwd)

	require.NoError(t, relay.Tick(context.Background()))

	assert.Equal(t, []string{"row-1"}, outbox.dead)
	assert.Empty(t, outbox.failed)
}

func TestNotificationRelay_OneFailureDoesNotStopTheBatch(t *testing.T) {
	outbox := &fakeOutbox{claimed: []domain.NotificationOutboxRow{outboxRow("row-1", 1), outboxRow("row-2", 1)}}
	fwd := &failFirstForwarder{}
	relay, _ := newTestRelay(t, outbox, fwd)

	require.NoError(t, relay.Tick(context.Background()))

	assert.Equal(t, []string{"row-2"}, outbox.forwarded)
	require.Len(t, outbox.failed, 1)
	assert.Equal(t, "row-1", outbox.failed[0].id)
}

type failFirstForwarder struct{ calls int }

func (f *failFirstForwarder) EnqueueNotification(context.Context, domain.NotificationOutboxRow, time.Time) error {
	f.calls++
	if f.calls == 1 {
		return errors.New("first one fails")
	}
	return nil
}

// TestNotificationRelay_PublishesGaugesOnEveryTick covers the failure mode
// the gauge exists to catch: a relay that stops publishing keeps reporting its
// last value forever, so an empty backlog has to write a real 0.
func TestNotificationRelay_PublishesGaugesOnEveryTick(t *testing.T) {
	outbox := &fakeOutbox{oldestAge: 0}
	relay, m := newTestRelay(t, outbox, &fakeForwarder{})

	require.NoError(t, relay.Tick(context.Background()))

	exposition := m.Prometheus()
	assert.Contains(t, exposition, "notification_outbox_oldest_pending_age_seconds 0")
	assert.Contains(t, exposition, "notification_outbox_last_tick_timestamp_seconds")
	assert.NotContains(t, exposition, "NaN")
}

func TestNotificationRelay_PublishesGaugesEvenWhenTheClaimFails(t *testing.T) {
	outbox := &fakeOutbox{claimErr: errors.New("db down"), oldestAge: 5 * time.Minute}
	relay, m := newTestRelay(t, outbox, &fakeForwarder{})

	err := relay.Tick(context.Background())

	require.Error(t, err)
	assert.Contains(t, m.Prometheus(), "notification_outbox_last_tick_timestamp_seconds",
		"a tick that failed still happened, and the freshness gauge must say so")
}

// TestNewNotificationRelay_RefusesUnwiredDependencies keeps a half-wired relay
// from starting and then silently forwarding nothing (CLAUDE.md rule 8).
func TestNewNotificationRelay_RefusesUnwiredDependencies(t *testing.T) {
	m := metrics.NewOutboxRelayMetrics()

	_, err := NewNotificationRelay(nil, &fakeForwarder{}, m, "relay-test", relayTestLogger())
	require.Error(t, err)

	_, err = NewNotificationRelay(&fakeOutbox{}, nil, m, "relay-test", relayTestLogger())
	require.Error(t, err)

	_, err = NewNotificationRelay(&fakeOutbox{}, &fakeForwarder{}, nil, "relay-test", relayTestLogger())
	require.Error(t, err)
}

func TestNotificationRelay_PanicsWhenForwarderIsUnwiredAtUse(t *testing.T) {
	outbox := &fakeOutbox{claimed: []domain.NotificationOutboxRow{outboxRow("row-1", 1)}}
	relay, _ := newTestRelay(t, outbox, &fakeForwarder{})
	relay.forwarder = nil

	assert.Panics(t, func() {
		_ = relay.Tick(context.Background())
	}, "an unwired forwarder must be loud, not a no-op that drops every notification")
}

// TestNotificationBackoffBound pins the full-jitter envelope: the sleep is
// drawn from [0, bound) and the bound doubles per attempt up to the cap.
func TestNotificationBackoffBound(t *testing.T) {
	assert.Equal(t, 2*relayBackoffBase, notificationBackoffBound(1))
	assert.Equal(t, 4*relayBackoffBase, notificationBackoffBound(2))
	assert.Equal(t, relayBackoffCap, notificationBackoffBound(1000),
		"the bound is capped no matter how the attempt counter grows")

	for attempt := 0; attempt <= relayMaxAttempts; attempt++ {
		bound := notificationBackoffBound(attempt)
		assert.Positive(t, bound)
		assert.LessOrEqual(t, bound, relayBackoffCap)
	}
}

func TestNotificationRelayStartupLogIsExplicit(t *testing.T) {
	assert.True(t, strings.HasSuffix(notificationRelayEnabledLog, "_enabled"),
		"startup must state the relay's wiring state loudly (CLAUDE.md rule 8)")
}
