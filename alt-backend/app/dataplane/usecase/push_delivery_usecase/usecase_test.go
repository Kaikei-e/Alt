package push_delivery_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"alt/domain"

	"github.com/stretchr/testify/require"
)

// stubPort records what reached the port and answers whatever the test wants.
// A counter rather than a mock because the only question these tests ask of it
// is whether validation let the call through.
type stubPort struct {
	calls int
	in    domain.NotificationEnqueue
}

func (s *stubPort) Enqueue(_ context.Context, in domain.NotificationEnqueue) (int, int, error) {
	s.calls++
	s.in = in
	return 1, 0, nil
}

func (s *stubPort) ClaimBatch(context.Context, string, int, time.Duration) ([]domain.PushDelivery, error) {
	return nil, nil
}
func (s *stubPort) MarkSent(context.Context, string, int) error              { return nil }
func (s *stubPort) Release(context.Context, string, time.Time, string) error { return nil }
func (s *stubPort) MarkDead(context.Context, string, int, string) error      { return nil }
func (s *stubPort) BacklogAge(context.Context) (time.Duration, int64, int64, error) {
	return 0, 0, 0, nil
}

func validEnqueue() domain.NotificationEnqueue {
	occurred := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	return domain.NotificationEnqueue{
		DedupeKey:  "summary:11111111-1111-1111-1111-111111111111",
		UserID:     "22222222-2222-2222-2222-222222222222",
		Kind:       domain.NotificationKindSummaryReady,
		Payload:    []byte(`{"url":"/articles/abc","kind":"summary_ready"}`),
		OccurredAt: occurred,
		ExpiresAt:  occurred.Add(24 * time.Hour),
	}
}

func TestEnqueue_AcceptsAValidRequest(t *testing.T) {
	port := &stubPort{}
	uc := NewPushDeliveryUsecase(port)

	delivered, superseded, err := uc.Enqueue(context.Background(), validEnqueue())
	require.NoError(t, err)
	require.Equal(t, 1, delivered)
	require.Equal(t, 0, superseded)
	require.Equal(t, 1, port.calls)
}

// payload lands in a JSONB NOT NULL column. An absent one reaches PostgreSQL as
// the empty string, which is not JSON, and surfaces as an opaque 22P02 the
// relay then retries eight times before dead-lettering the notification. The
// producer's mistake has to be named at the boundary instead.
func TestEnqueue_RejectsAnEmptyPayload(t *testing.T) {
	for name, payload := range map[string][]byte{
		"nil":   nil,
		"empty": {},
	} {
		t.Run(name, func(t *testing.T) {
			port := &stubPort{}
			uc := NewPushDeliveryUsecase(port)

			in := validEnqueue()
			in.Payload = payload

			_, _, err := uc.Enqueue(context.Background(), in)
			require.ErrorIs(t, err, ErrInvalidEnqueue)
			require.Equal(t, 0, port.calls, "the port must not be asked to store a payload the column rejects")
		})
	}
}

// Anything that is not JSON fails the same way once it reaches the column, and
// the boundary is the only place that can tell the producer which of its
// notifications was malformed.
func TestEnqueue_RejectsAPayloadThatIsNotJSON(t *testing.T) {
	port := &stubPort{}
	uc := NewPushDeliveryUsecase(port)

	in := validEnqueue()
	in.Payload = []byte("not json")

	_, _, err := uc.Enqueue(context.Background(), in)
	require.ErrorIs(t, err, ErrInvalidEnqueue)
	require.Equal(t, 0, port.calls)
}

func TestEnqueue_RejectsMalformedRequests(t *testing.T) {
	cases := map[string]func(*domain.NotificationEnqueue){
		"no dedupe key":         func(in *domain.NotificationEnqueue) { in.DedupeKey = "" },
		"no user":               func(in *domain.NotificationEnqueue) { in.UserID = "" },
		"no occurred_at":        func(in *domain.NotificationEnqueue) { in.OccurredAt = time.Time{} },
		"no expires_at":         func(in *domain.NotificationEnqueue) { in.ExpiresAt = time.Time{} },
		"expires before occurs": func(in *domain.NotificationEnqueue) { in.ExpiresAt = in.OccurredAt.Add(-time.Hour) },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			port := &stubPort{}
			uc := NewPushDeliveryUsecase(port)

			in := validEnqueue()
			mutate(&in)

			_, _, err := uc.Enqueue(context.Background(), in)
			require.ErrorIs(t, err, ErrInvalidEnqueue)
			require.Equal(t, 0, port.calls)
		})
	}
}

func TestEnqueue_RejectsAnUnknownKind(t *testing.T) {
	port := &stubPort{}
	uc := NewPushDeliveryUsecase(port)

	in := validEnqueue()
	in.Kind = "summary_redy"

	_, _, err := uc.Enqueue(context.Background(), in)
	require.ErrorIs(t, err, ErrUnknownKind)
	require.Equal(t, 0, port.calls)
}

// The payload is opaque to this layer beyond being JSON: the producer
// serialised it next to the fact it describes and the dispatcher renders it.
// Validation must not start requiring a shape.
func TestEnqueue_AcceptsAnyJSONShape(t *testing.T) {
	for name, payload := range map[string][]byte{
		"array":  []byte(`[1,2,3]`),
		"scalar": []byte(`"ready"`),
		"nested": []byte(`{"a":{"b":[null]}}`),
	} {
		t.Run(name, func(t *testing.T) {
			port := &stubPort{}
			uc := NewPushDeliveryUsecase(port)

			in := validEnqueue()
			in.Payload = payload

			_, _, err := uc.Enqueue(context.Background(), in)
			require.NoError(t, err)
			require.Equal(t, 1, port.calls)
		})
	}
}

func TestNewPushDeliveryUsecase_PanicsOnNilPort(t *testing.T) {
	require.Panics(t, func() { NewPushDeliveryUsecase(nil) })
}

func TestEnqueue_WrapsAPortFailureWithTheDedupeKey(t *testing.T) {
	boom := errors.New("boom")
	uc := NewPushDeliveryUsecase(&failingPort{err: boom})

	in := validEnqueue()
	_, _, err := uc.Enqueue(context.Background(), in)
	require.ErrorIs(t, err, boom)
	require.Contains(t, err.Error(), in.DedupeKey)
}

type failingPort struct {
	stubPort
	err error
}

func (f *failingPort) Enqueue(context.Context, domain.NotificationEnqueue) (int, int, error) {
	return 0, 0, f.err
}
