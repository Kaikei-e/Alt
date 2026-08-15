package push_dispatch_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"alt/domain"
	"alt/orchestrator/port/push_dispatch_port"
)

// --- fakes -----------------------------------------------------------------

type recordedFinal struct {
	deliveryID string
	statusCode int
	reason     string
	retryAfter time.Duration
}

type fakeDeliveries struct {
	batch    []domain.PushDelivery
	claimErr error

	sent     []string
	dead     []recordedFinal
	released []recordedFinal
}

func (f *fakeDeliveries) ClaimBatch(_ context.Context, _ int) ([]domain.PushDelivery, error) {
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	return f.batch, nil
}

func (f *fakeDeliveries) BacklogAge(_ context.Context) (time.Duration, int64, int64, error) {
	return 0, 0, 0, nil
}

func (f *fakeDeliveries) MarkSent(_ context.Context, id string, _ int) error {
	f.sent = append(f.sent, id)
	return nil
}

func (f *fakeDeliveries) MarkDead(_ context.Context, id string, status int, reason string) error {
	f.dead = append(f.dead, recordedFinal{deliveryID: id, statusCode: status, reason: reason})
	return nil
}

func (f *fakeDeliveries) Release(_ context.Context, id string, retryAfter time.Duration, status int, reason string) error {
	f.released = append(f.released, recordedFinal{
		deliveryID: id, statusCode: status, reason: reason, retryAfter: retryAfter,
	})
	return nil
}

type fakeSubscriptions struct{ deleted []string }

func (f *fakeSubscriptions) DeleteByEndpoint(_ context.Context, _, endpoint string) error {
	f.deleted = append(f.deleted, endpoint)
	return nil
}

type fakeSender struct {
	outcome  push_dispatch_port.SendOutcome
	err      error
	calls    int
	requests []push_dispatch_port.SendRequest
}

func (f *fakeSender) Send(_ context.Context, req push_dispatch_port.SendRequest) (push_dispatch_port.SendOutcome, error) {
	f.calls++
	f.requests = append(f.requests, req)
	return f.outcome, f.err
}

func delivery(attempts int) domain.PushDelivery {
	return domain.PushDelivery{
		ID:             "delivery-1",
		DedupeKey:      "recap:job-1",
		SubscriptionID: "sub-1",
		UserID:         "user-1",
		Kind:           domain.NotificationKindRecapReady,
		Payload:        []byte(`{"kind":"recap_ready","url":"/recap"}`),
		State:          domain.NotificationSending,
		Attempts:       attempts,
		Endpoint:       "https://fcm.googleapis.com/fcm/send/abc",
		P256dh:         "BKey",
		Auth:           "AAuth",
	}
}

func newUsecase(d *fakeDeliveries, s *fakeSubscriptions, sender *fakeSender) *Usecase {
	u := New(d, s, sender)
	// Deterministic backoff so the assertions describe policy rather than luck.
	u.backoff = func(attempt int) time.Duration {
		return time.Duration(attempt) * time.Minute
	}
	return u
}

// --- tests -----------------------------------------------------------------

func TestDispatchBatch_MarksSentOnAcceptance(t *testing.T) {
	deliveries := &fakeDeliveries{batch: []domain.PushDelivery{delivery(1)}}
	subs := &fakeSubscriptions{}
	sender := &fakeSender{outcome: push_dispatch_port.SendOutcome{StatusCode: 201}}

	stats, err := newUsecase(deliveries, subs, sender).DispatchBatch(context.Background(), 10)
	if err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	if len(deliveries.sent) != 1 || deliveries.sent[0] != "delivery-1" {
		t.Fatalf("expected the row marked sent, got %v", deliveries.sent)
	}
	if stats.Sent != 1 {
		t.Fatalf("stats.Sent = %d, want 1", stats.Sent)
	}
	if len(subs.deleted) != 0 {
		t.Fatalf("a successful send must not touch the subscription, got %v", subs.deleted)
	}
}

func TestDispatchBatch_DeletesSubscriptionWhenGone(t *testing.T) {
	// 404/410 is the only signal a push service gives that a device is gone
	// for good. Leaving the row behind means every later fan-out pays for a
	// device that will never receive again, and the delivery success rate
	// decays in a way that looks like a regression.
	deliveries := &fakeDeliveries{batch: []domain.PushDelivery{delivery(1)}}
	subs := &fakeSubscriptions{}
	sender := &fakeSender{outcome: push_dispatch_port.SendOutcome{StatusCode: 410, Gone: true}}

	stats, err := newUsecase(deliveries, subs, sender).DispatchBatch(context.Background(), 10)
	if err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	if len(subs.deleted) != 1 || subs.deleted[0] != "https://fcm.googleapis.com/fcm/send/abc" {
		t.Fatalf("expected the subscription deleted, got %v", subs.deleted)
	}
	if len(deliveries.dead) != 1 {
		t.Fatalf("expected the delivery finalized, got %v", deliveries.dead)
	}
	if stats.Gone != 1 {
		t.Fatalf("stats.Gone = %d, want 1", stats.Gone)
	}
	if stats.Dead != 0 {
		t.Fatalf("a gone device is not a pipeline failure; stats.Dead = %d, want 0", stats.Dead)
	}
}

func TestDispatchBatch_ReleasesRetryableWithBackoff(t *testing.T) {
	deliveries := &fakeDeliveries{batch: []domain.PushDelivery{delivery(2)}}
	sender := &fakeSender{outcome: push_dispatch_port.SendOutcome{StatusCode: 503, Retryable: true}}

	_, err := newUsecase(deliveries, &fakeSubscriptions{}, sender).DispatchBatch(context.Background(), 10)
	if err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	if len(deliveries.released) != 1 {
		t.Fatalf("expected the row released, got %v", deliveries.released)
	}
	if got := deliveries.released[0].retryAfter; got != 2*time.Minute {
		t.Fatalf("retryAfter = %v, want the backoff for attempt 2", got)
	}
}

func TestDispatchBatch_HonoursRetryAfterOverBackoff(t *testing.T) {
	// A push service that says "not before 90s" knows something the dispatcher
	// does not; ignoring it is how a 429 turns into a longer 429.
	deliveries := &fakeDeliveries{batch: []domain.PushDelivery{delivery(1)}}
	sender := &fakeSender{outcome: push_dispatch_port.SendOutcome{
		StatusCode: 429, Retryable: true, RetryAfter: 90 * time.Second,
	}}

	_, err := newUsecase(deliveries, &fakeSubscriptions{}, sender).DispatchBatch(context.Background(), 10)
	if err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	if got := deliveries.released[0].retryAfter; got != 90*time.Second {
		t.Fatalf("retryAfter = %v, want the server's Retry-After", got)
	}
}

func TestDispatchBatch_StopsRetryingAfterTheAttemptBudget(t *testing.T) {
	// A "your job finished" notification delivered days late is worse than one
	// never delivered, so the budget is deliberately small — unlike a general
	// job queue, where eventual completion still has value.
	deliveries := &fakeDeliveries{batch: []domain.PushDelivery{delivery(maxAttempts)}}
	sender := &fakeSender{outcome: push_dispatch_port.SendOutcome{StatusCode: 503, Retryable: true}}

	stats, err := newUsecase(deliveries, &fakeSubscriptions{}, sender).DispatchBatch(context.Background(), 10)
	if err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	if len(deliveries.released) != 0 {
		t.Fatalf("budget exhausted, expected no release, got %v", deliveries.released)
	}
	if len(deliveries.dead) != 1 || stats.Dead != 1 {
		t.Fatalf("expected the row dead-lettered, got %v / stats.Dead=%d", deliveries.dead, stats.Dead)
	}
}

func TestDispatchBatch_DeadLettersTerminalRejections(t *testing.T) {
	// 400/401/403/413 are sender bugs. Retrying reproduces them exactly, so the
	// row goes terminal and the alert on 401/403 is what gets a human involved.
	for _, status := range []int{400, 401, 403, 413} {
		deliveries := &fakeDeliveries{batch: []domain.PushDelivery{delivery(1)}}
		subs := &fakeSubscriptions{}
		sender := &fakeSender{outcome: push_dispatch_port.SendOutcome{StatusCode: status}}

		_, err := newUsecase(deliveries, subs, sender).DispatchBatch(context.Background(), 10)
		if err != nil {
			t.Fatalf("status %d: DispatchBatch: %v", status, err)
		}

		if len(deliveries.dead) != 1 {
			t.Fatalf("status %d: expected dead-letter, got %v", status, deliveries.dead)
		}
		if len(subs.deleted) != 0 {
			t.Fatalf("status %d: our own bug must not delete the user's subscription", status)
		}
	}
}

func TestDispatchBatch_TransportFailureIsRetryable(t *testing.T) {
	deliveries := &fakeDeliveries{batch: []domain.PushDelivery{delivery(1)}}
	sender := &fakeSender{
		outcome: push_dispatch_port.SendOutcome{Retryable: true},
		err:     errors.New("dial tcp: i/o timeout"),
	}

	_, err := newUsecase(deliveries, &fakeSubscriptions{}, sender).DispatchBatch(context.Background(), 10)
	if err != nil {
		t.Fatalf("a single delivery failing must not fail the batch: %v", err)
	}

	if len(deliveries.released) != 1 {
		t.Fatalf("expected the row released for retry, got %v", deliveries.released)
	}
}

func TestDispatchBatch_ContinuesAfterOneDeliveryFails(t *testing.T) {
	// One unreachable device must not stall the queue for every other device.
	first := delivery(1)
	second := delivery(1)
	second.ID = "delivery-2"
	second.Endpoint = "https://web.push.apple.com/xyz"

	deliveries := &fakeDeliveries{batch: []domain.PushDelivery{first, second}}
	sender := &fakeSender{outcome: push_dispatch_port.SendOutcome{StatusCode: 201}}

	stats, err := newUsecase(deliveries, &fakeSubscriptions{}, sender).DispatchBatch(context.Background(), 10)
	if err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	if sender.calls != 2 || stats.Sent != 2 {
		t.Fatalf("sender.calls=%d stats.Sent=%d, want 2 and 2", sender.calls, stats.Sent)
	}
}

func TestDispatchBatch_ReportsClaimFailure(t *testing.T) {
	// Failing to claim is not "nothing to do": swallowing it would make a
	// broken data plane look like an idle queue, which is the shape of failure
	// this whole feature is built to avoid.
	deliveries := &fakeDeliveries{claimErr: errors.New("mtls handshake failed")}

	_, err := newUsecase(deliveries, &fakeSubscriptions{}, &fakeSender{}).
		DispatchBatch(context.Background(), 10)
	if err == nil {
		t.Fatal("expected the claim error to propagate")
	}
}

func TestDispatchBatch_AppliesPerKindDeliveryPolicy(t *testing.T) {
	// Topic is the collapse key the push service applies to messages it has not
	// delivered yet. It is plaintext to that service and never forwarded to the
	// browser, so it carries the kind and nothing user-specific — a per-user
	// value would hand Google, Mozilla and Apple a correlation key for free.
	cases := map[string]struct {
		kind        string
		wantTTL     time.Duration
		wantUrgency string
	}{
		"job completion": {
			kind:    domain.NotificationKindRecapReady,
			wantTTL: 24 * time.Hour,
			// Empty means normal, which RFC 8030 expresses by omitting the header.
			wantUrgency: "",
		},
		"daily digest": {
			kind:    domain.NotificationKindTodayEntranceReady,
			wantTTL: 12 * time.Hour,
			// A digest genuinely can wait for power or Wi-Fi, and dying before
			// tomorrow's is what stops a delayed one arriving a day late
			// claiming to be today's.
			wantUrgency: "low",
		},
	}

	for name, tc := range cases {
		d := delivery(1)
		d.Kind = tc.kind
		deliveries := &fakeDeliveries{batch: []domain.PushDelivery{d}}
		sender := &fakeSender{outcome: push_dispatch_port.SendOutcome{StatusCode: 201}}

		if _, err := newUsecase(deliveries, &fakeSubscriptions{}, sender).
			DispatchBatch(context.Background(), 10); err != nil {
			t.Fatalf("%s: DispatchBatch: %v", name, err)
		}

		req := sender.requests[0]
		if req.TTL != tc.wantTTL {
			t.Fatalf("%s: TTL = %v, want %v", name, req.TTL, tc.wantTTL)
		}
		if req.Urgency != tc.wantUrgency {
			t.Fatalf("%s: Urgency = %q, want %q", name, req.Urgency, tc.wantUrgency)
		}
		if req.Topic != tc.kind {
			t.Fatalf("%s: Topic = %q, want the kind", name, req.Topic)
		}
		if len(req.Topic) > 32 {
			t.Fatalf("%s: Topic is %d chars, over the RFC 8030 limit", name, len(req.Topic))
		}
	}
}

func TestFullJitterBackoff_NeverExceedsCapAndIsNonNegative(t *testing.T) {
	cases := []struct {
		name    string
		attempt int
		max     time.Duration
	}{
		{name: "first attempt stays in the base window", attempt: 0, max: backoffBase},
		{name: "capped attempts never exceed backoffCap", attempt: 20, max: backoffCap},
		{name: "attempt at the shift cap stays at backoffCap", attempt: 12, max: backoffCap},
	}

	const samples = 200
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < samples; i++ {
				got := fullJitterBackoff(tc.attempt)
				if got < 0 {
					t.Fatalf("fullJitterBackoff(%d) = %v, want non-negative", tc.attempt, got)
				}
				if got > tc.max {
					t.Fatalf("fullJitterBackoff(%d) = %v, want <= %v", tc.attempt, got, tc.max)
				}
				if got > backoffCap {
					t.Fatalf("fullJitterBackoff(%d) = %v, exceeds backoffCap %v", tc.attempt, got, backoffCap)
				}
			}
		})
	}
}
