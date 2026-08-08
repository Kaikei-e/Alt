package job

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"alt/domain"

	"github.com/google/uuid"
)

type countCall struct {
	userID uuid.UUID
	date   time.Time
}

type fakeDigestReader struct {
	users      []uuid.UUID
	counts     map[uuid.UUID]int
	countErrs  map[uuid.UUID]error
	listErr    error
	listCalls  int
	countCalls []countCall
}

func (f *fakeDigestReader) ListDistinctUserIDs(context.Context) ([]uuid.UUID, error) {
	f.listCalls++
	return f.users, f.listErr
}

func (f *fakeDigestReader) CountNeedToKnowItems(_ context.Context, userID uuid.UUID, date time.Time) (int, error) {
	f.countCalls = append(f.countCalls, countCall{userID: userID, date: date})
	if err, ok := f.countErrs[userID]; ok {
		return 0, err
	}
	return f.counts[userID], nil
}

type fakeNotificationEnqueuer struct {
	enqueued []domain.NotificationEnqueue
	err      error
}

func (f *fakeNotificationEnqueuer) Enqueue(_ context.Context, in domain.NotificationEnqueue) (int, int, error) {
	f.enqueued = append(f.enqueued, in)
	if f.err != nil {
		return 0, 0, f.err
	}
	return 1, 0, nil
}

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func mustUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", s, err)
	}
	return id
}

// The digest counts a UTC calendar day, so the hour that may fire is a UTC
// hour and nothing else. Firing at 00:00 UTC would count a day that has just
// begun and report ~0 items; 23:00 UTC counts a nearly complete day. There is
// no per-user timezone in this system, so the gate reads the clock in UTC
// rather than in whatever zone the container happens to carry.
func TestTodayEntranceNotificationJob_OnlyFiresInTheTriggerHourUTC(t *testing.T) {
	tokyo := time.FixedZone("JST", 9*60*60)

	tests := []struct {
		name     string
		now      time.Time
		wantFire bool
	}{
		{name: "midnight UTC", now: time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)},
		{name: "22:59 UTC", now: time.Date(2026, 8, 8, 22, 59, 59, 0, time.UTC)},
		{name: "23:00 UTC", now: time.Date(2026, 8, 8, 23, 0, 0, 0, time.UTC), wantFire: true},
		{name: "23:59 UTC", now: time.Date(2026, 8, 8, 23, 59, 59, 0, time.UTC), wantFire: true},
		{
			// 08:00 the next morning in the operator's zone is the same
			// instant as 23:00 UTC, so the zone of the reading must not
			// change the answer.
			name:     "23:00 UTC expressed in +09:00",
			now:      time.Date(2026, 8, 9, 8, 0, 0, 0, tokyo),
			wantFire: true,
		},
		{
			// 23:00 local in +09:00 is 14:00 UTC — mid-afternoon of a day the
			// digest is still filling up. A job that read the local hour would
			// fire here.
			name: "23:00 local in +09:00",
			now:  time.Date(2026, 8, 8, 23, 0, 0, 0, tokyo),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := mustUUID(t, "3f2504e0-4f89-11d3-9a0c-0305e82c3301")
			digests := &fakeDigestReader{users: []uuid.UUID{userID}, counts: map[uuid.UUID]int{userID: 7}}
			pushes := &fakeNotificationEnqueuer{}

			fn := todayEntranceNotificationJob(digests, pushes, fixedClock(tt.now))
			if err := fn(context.Background()); err != nil {
				t.Fatalf("job returned %v", err)
			}

			if tt.wantFire {
				if digests.listCalls != 1 {
					t.Errorf("audience should be read once in the trigger hour, got %d reads", digests.listCalls)
				}
				if len(pushes.enqueued) != 1 {
					t.Errorf("expected 1 enqueue in the trigger hour, got %d", len(pushes.enqueued))
				}
				return
			}
			if digests.listCalls != 0 {
				t.Errorf("outside the trigger hour the job must not read the audience, got %d reads", digests.listCalls)
			}
			if len(pushes.enqueued) != 0 {
				t.Errorf("outside the trigger hour the job must enqueue nothing, got %d", len(pushes.enqueued))
			}
		})
	}
}

// One notification a day per user, carrying a trigger and not content: the
// count is the only datum allowed through, and the dedupe key is what makes a
// second tick inside the same hour a no-op provider-side.
func TestTodayEntranceNotificationJob_EnqueuesOneTriggerPerUserWithItems(t *testing.T) {
	alice := mustUUID(t, "3f2504e0-4f89-11d3-9a0c-0305e82c3301")
	bob := mustUUID(t, "1b4e28ba-2fa1-4d3b-a3f5-ccee1bf27e11")
	carol := mustUUID(t, "9c858901-8a57-4791-81fe-4c455b099bc9")

	now := time.Date(2026, 8, 8, 23, 12, 0, 0, time.UTC)
	digests := &fakeDigestReader{
		users:  []uuid.UUID{alice, bob, carol},
		counts: map[uuid.UUID]int{alice: 5, bob: 0, carol: 2},
	}
	pushes := &fakeNotificationEnqueuer{}

	fn := todayEntranceNotificationJob(digests, pushes, fixedClock(now))
	if err := fn(context.Background()); err != nil {
		t.Fatalf("job returned %v", err)
	}

	// bob's count is zero. "0 new" is worse than silence and would burn the
	// one daily notification the user agreed to.
	if len(pushes.enqueued) != 2 {
		t.Fatalf("expected 2 enqueues (the zero-count user is skipped), got %d", len(pushes.enqueued))
	}

	got := pushes.enqueued[0]
	if want := "digest:" + alice.String() + ":2026-08-08"; got.DedupeKey != want {
		t.Errorf("dedupe key = %q, want %q", got.DedupeKey, want)
	}
	if got.UserID != alice.String() {
		t.Errorf("user id = %q, want %q", got.UserID, alice)
	}
	if got.Kind != domain.NotificationKindTodayEntranceReady {
		t.Errorf("kind = %q, want %q", got.Kind, domain.NotificationKindTodayEntranceReady)
	}
	if !got.OccurredAt.Equal(now) {
		t.Errorf("occurred at = %v, want %v", got.OccurredAt, now)
	}
	if want := now.Add(todayEntranceTTL); !got.ExpiresAt.Equal(want) {
		t.Errorf("expires at = %v, want %v", got.ExpiresAt, want)
	}

	// Exactly these three keys. A title or an excerpt here would turn the push
	// into a delivery channel and put article text on a lock screen.
	var payload map[string]any
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	want := map[string]any{"kind": "today_entrance_ready", "url": "/home", "count": float64(5)}
	if len(payload) != len(want) {
		t.Fatalf("payload = %v, want exactly %v", payload, want)
	}
	for k, v := range want {
		if payload[k] != v {
			t.Errorf("payload[%q] = %v, want %v", k, payload[k], v)
		}
	}

	if payload := pushes.enqueued[1].Payload; !json.Valid(payload) {
		t.Fatalf("second payload is not JSON: %s", payload)
	}
	if want := "digest:" + carol.String() + ":2026-08-08"; pushes.enqueued[1].DedupeKey != want {
		t.Errorf("second dedupe key = %q, want %q", pushes.enqueued[1].DedupeKey, want)
	}

	// The count is windowed on the UTC calendar day the reading falls in, so
	// the date handed to the counter has to be the UTC one.
	if len(digests.countCalls) != 3 {
		t.Fatalf("expected one count per user, got %d", len(digests.countCalls))
	}
	for _, call := range digests.countCalls {
		if call.date.Location() != time.UTC {
			t.Errorf("count for %s asked about %v, which is not a UTC reading", call.userID, call.date)
		}
		if y, m, d := call.date.Date(); y != 2026 || m != time.August || d != 8 {
			t.Errorf("count for %s asked about %v, want the 2026-08-08 UTC day", call.userID, call.date)
		}
	}
}

// Correctness against repeats comes from the dedupe key, not from the
// schedule: the job ticks every ten minutes, so it fires several times inside
// the trigger hour and every one of those must land on the same key.
func TestTodayEntranceNotificationJob_RepeatsInsideTheHourReuseTheDedupeKey(t *testing.T) {
	userID := mustUUID(t, "3f2504e0-4f89-11d3-9a0c-0305e82c3301")
	digests := &fakeDigestReader{users: []uuid.UUID{userID}, counts: map[uuid.UUID]int{userID: 3}}
	pushes := &fakeNotificationEnqueuer{}

	for _, now := range []time.Time{
		time.Date(2026, 8, 8, 23, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 8, 23, 10, 0, 0, time.UTC),
		time.Date(2026, 8, 8, 23, 50, 0, 0, time.UTC),
	} {
		fn := todayEntranceNotificationJob(digests, pushes, fixedClock(now))
		if err := fn(context.Background()); err != nil {
			t.Fatalf("job returned %v", err)
		}
	}

	if len(pushes.enqueued) != 3 {
		t.Fatalf("expected 3 enqueue attempts, got %d", len(pushes.enqueued))
	}
	first := pushes.enqueued[0].DedupeKey
	for i, e := range pushes.enqueued {
		if e.DedupeKey != first {
			t.Errorf("enqueue %d dedupe key = %q, want %q — a restart mid-window must collapse into one delivery", i, e.DedupeKey, first)
		}
	}
}

// One user's failure must not cost every later user their notification, and it
// must not be swallowed either.
func TestTodayEntranceNotificationJob_ContinuesPastOneUserAndReportsTheFailure(t *testing.T) {
	alice := mustUUID(t, "3f2504e0-4f89-11d3-9a0c-0305e82c3301")
	bob := mustUUID(t, "1b4e28ba-2fa1-4d3b-a3f5-ccee1bf27e11")

	digests := &fakeDigestReader{
		users:     []uuid.UUID{alice, bob},
		counts:    map[uuid.UUID]int{bob: 4},
		countErrs: map[uuid.UUID]error{alice: errors.New("sovereign unreachable")},
	}
	pushes := &fakeNotificationEnqueuer{}

	fn := todayEntranceNotificationJob(digests, pushes, fixedClock(time.Date(2026, 8, 8, 23, 0, 0, 0, time.UTC)))
	err := fn(context.Background())

	if err == nil {
		t.Error("a failed count must be reported, not swallowed")
	}
	if len(pushes.enqueued) != 1 {
		t.Fatalf("expected the second user to still be notified, got %d enqueues", len(pushes.enqueued))
	}
	if pushes.enqueued[0].UserID != bob.String() {
		t.Errorf("notified %q, want %q", pushes.enqueued[0].UserID, bob)
	}
}

func TestTodayEntranceNotificationJob_AudienceReadFailureIsReported(t *testing.T) {
	digests := &fakeDigestReader{listErr: errors.New("sovereign unreachable")}
	pushes := &fakeNotificationEnqueuer{}

	fn := todayEntranceNotificationJob(digests, pushes, fixedClock(time.Date(2026, 8, 8, 23, 0, 0, 0, time.UTC)))
	if err := fn(context.Background()); err == nil {
		t.Fatal("a failed audience read must be reported")
	}
	if len(pushes.enqueued) != 0 {
		t.Errorf("nothing may be enqueued when the audience is unknown, got %d", len(pushes.enqueued))
	}
}

// Both dependencies are wired unconditionally at the composition root, so a
// nil one can only be a DI bug. It has to stop the process rather than tick a
// silent no-op every ten minutes forever (CLAUDE.md rule 8).
func TestTodayEntranceNotificationJob_NilDependenciesPanic(t *testing.T) {
	tests := []struct {
		name    string
		digests digestAudienceReader
		pushes  notificationEnqueuer
	}{
		{name: "nil digest reader", pushes: &fakeNotificationEnqueuer{}},
		{name: "nil enqueuer", digests: &fakeDigestReader{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected a panic rather than a job that ticks and does nothing")
				}
			}()
			TodayEntranceNotificationJob(tt.digests, tt.pushes)
		})
	}
}

// The exported constructor is what the registry calls, so its default clock
// has to be the real one.
func TestTodayEntranceNotificationJob_DefaultsToTheWallClock(t *testing.T) {
	digests := &fakeDigestReader{}
	pushes := &fakeNotificationEnqueuer{}

	fn := TodayEntranceNotificationJob(digests, pushes)
	if err := fn(context.Background()); err != nil {
		t.Fatalf("job returned %v", err)
	}

	// Outside the trigger hour nothing is read; inside it the empty audience
	// is read once. Either is fine — what must not happen is a panic from an
	// unset clock.
	if digests.listCalls > 1 {
		t.Errorf("audience read %d times in one pass", digests.listCalls)
	}
}
