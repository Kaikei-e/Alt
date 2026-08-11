//go:build integration

package alt_db

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"alt/domain"
	"alt/test_utils/pgtest"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// The fan-out is an INSERT ... SELECT, and a parameter in a sub-SELECT target
// list gets no type context from the column it lands in — unlike the same
// parameter in a VALUES list. PostgreSQL infers `text`, pgx then encodes
// Payload's []byte as a bytea hex escape, and the assignment cast into JSONB
// rejects it. pgxmock cannot reach any of that: it replays the arguments it was
// handed and never asks a server what they mean. So this file exists for the
// properties that only a real server decides — parameter typing, the CHECK on
// state, and the partial unique index that supersedes an unsent digest.

func subscriptionFor(t *testing.T, repo *PushRepository, userID string, prefs domain.NotificationPreferences) string {
	t.Helper()

	// A documented example host, matching the fixtures in the mocked tests and
	// the pact. An endpoint is a capability URL, and a literal here that named
	// a real push service is a template someone completes with a genuine one.
	endpoint := "https://push.example/endpoint/" + uuid.NewString()
	created, err := repo.UpsertPushSubscription(context.Background(), domain.PushSubscription{
		UserID:              userID,
		Endpoint:            endpoint,
		P256dh:              "p256dh-" + uuid.NewString(),
		Auth:                "auth-" + uuid.NewString(),
		Preferences:         prefs,
		VAPIDKeyFingerprint: "pgtest",
	})
	require.NoError(t, err)
	require.True(t, created)
	return endpoint
}

func allKindsOn() domain.NotificationPreferences {
	return domain.NotificationPreferences{
		SummaryReady:       true,
		AcolyteReportReady: true,
		RecapReady:         true,
		TodayEntranceReady: true,
	}
}

// enqueueFor builds an enqueue that is due now and not yet expired.
//
// Both properties are relative to the run rather than to a calendar date, and
// that is the point: expires_at is a deadline the claim query enforces, so a
// fixed date makes every claiming test pass until that instant and fail for
// good afterwards. This fixture was written with a date one day out and the
// suite went red on its own, hours after the last commit that touched it.
//
// Truncated to Postgres's microsecond resolution so the round-trip assertions
// compare a value the column can actually hold.
func enqueueFor(userID, kind string, payload []byte) domain.NotificationEnqueue {
	occurred := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	return domain.NotificationEnqueue{
		DedupeKey:  kind + ":" + uuid.NewString(),
		UserID:     userID,
		Kind:       kind,
		Payload:    payload,
		OccurredAt: occurred,
		ExpiresAt:  occurred.Add(24 * time.Hour),
	}
}

// The regression this file was written for. Every enqueue in production failed
// with SQLSTATE 22P02 while the mocked tests stayed green.
func TestEnqueueNotification_WritesJSONPayloadToRealPostgres(t *testing.T) {
	pool := pgtest.NewDB(t)
	repo := NewPushRepository(pool)
	ctx := context.Background()

	userID := uuid.NewString()
	subscriptionFor(t, repo, userID, allKindsOn())

	payload := []byte(`{"url":"/articles/abc","title":"summary ready"}`)
	in := enqueueFor(userID, domain.NotificationKindSummaryReady, payload)

	fanned, superseded, err := repo.EnqueueNotification(ctx, in)
	require.NoError(t, err)
	require.Equal(t, 1, fanned, "the one subscribed device should get a row")
	require.Equal(t, 0, superseded)

	var stored []byte
	var occurred, expires time.Time
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT payload, occurred_at, expires_at FROM push_deliveries WHERE dedupe_key = $1`,
		in.DedupeKey).Scan(&stored, &occurred, &expires))

	require.JSONEq(t, string(payload), string(stored),
		"the payload has to survive as JSON, not as a bytea escape")
	require.True(t, occurred.Equal(in.OccurredAt), "occurred_at is business time and must round-trip")
	require.True(t, expires.Equal(in.ExpiresAt))
}

// The same statement carries every kind, so a cast that fixes one and not the
// others would pass the test above and still lose three quarters of the
// product's notifications.
func TestEnqueueNotification_WritesEveryKind(t *testing.T) {
	pool := pgtest.NewDB(t)
	repo := NewPushRepository(pool)
	ctx := context.Background()

	userID := uuid.NewString()
	subscriptionFor(t, repo, userID, allKindsOn())

	for _, kind := range []string{
		domain.NotificationKindSummaryReady,
		domain.NotificationKindAcolyteReportReady,
		domain.NotificationKindRecapReady,
		domain.NotificationKindTodayEntranceReady,
	} {
		t.Run(kind, func(t *testing.T) {
			in := enqueueFor(userID, kind, []byte(`{"kind":"`+kind+`"}`))
			fanned, _, err := repo.EnqueueNotification(ctx, in)
			require.NoError(t, err)
			require.Equal(t, 1, fanned)
		})
	}
}

// A payload holding a JSON scalar or an array is still valid JSON. Casting via
// text would accept the object case and reject nothing else, so the narrow fix
// has to be exercised beyond the shape the producers happen to send today.
func TestEnqueueNotification_AcceptsAnyValidJSONPayload(t *testing.T) {
	pool := pgtest.NewDB(t)
	repo := NewPushRepository(pool)
	ctx := context.Background()

	userID := uuid.NewString()
	subscriptionFor(t, repo, userID, allKindsOn())

	for name, payload := range map[string][]byte{
		"object":         []byte(`{"a":1}`),
		"array":          []byte(`[1,2,3]`),
		"string scalar":  []byte(`"plain"`),
		"unicode escape": []byte(`{"title":"日本語"}`),
		"nested":         []byte(`{"a":{"b":[{"c":null}]}}`),
	} {
		t.Run(name, func(t *testing.T) {
			in := enqueueFor(userID, domain.NotificationKindSummaryReady, payload)
			fanned, _, err := repo.EnqueueNotification(ctx, in)
			require.NoError(t, err)
			require.Equal(t, 1, fanned)

			var stored []byte
			require.NoError(t, pool.QueryRow(ctx,
				`SELECT payload FROM push_deliveries WHERE dedupe_key = $1`,
				in.DedupeKey).Scan(&stored))
			require.JSONEq(t, string(payload), string(stored))
		})
	}
}

// Fanning out to one device per subscription, and only to the devices that
// still want the kind. The preference is read at fan-out, so a device with the
// kind switched off must produce no row at all rather than a row the dispatcher
// later discards.
func TestEnqueueNotification_HonoursPerDevicePreferences(t *testing.T) {
	pool := pgtest.NewDB(t)
	repo := NewPushRepository(pool)
	ctx := context.Background()

	userID := uuid.NewString()
	subscriptionFor(t, repo, userID, allKindsOn())
	subscriptionFor(t, repo, userID, allKindsOn())
	subscriptionFor(t, repo, userID, domain.NotificationPreferences{
		SummaryReady:       false,
		AcolyteReportReady: true,
		RecapReady:         true,
		TodayEntranceReady: true,
	})

	in := enqueueFor(userID, domain.NotificationKindSummaryReady, []byte(`{"url":"/a"}`))
	fanned, _, err := repo.EnqueueNotification(ctx, in)
	require.NoError(t, err)
	require.Equal(t, 2, fanned, "the device with summary_ready off gets nothing")
}

// Zero matching devices is an ordinary answer, not an error — the relay marks
// the outbox row forwarded either way. Asserting it here keeps a future fix to
// the fan-out from turning "nobody subscribed" into a retry loop.
func TestEnqueueNotification_NoSubscribersIsNotAnError(t *testing.T) {
	pool := pgtest.NewDB(t)
	repo := NewPushRepository(pool)
	ctx := context.Background()

	in := enqueueFor(uuid.NewString(), domain.NotificationKindSummaryReady, []byte(`{"url":"/a"}`))
	fanned, superseded, err := repo.EnqueueNotification(ctx, in)
	require.NoError(t, err)
	require.Equal(t, 0, fanned)
	require.Equal(t, 0, superseded)
}

// A relayed retry of the same enqueue must add nothing. The relay retries on
// any transport failure, so this is the ordinary path rather than an edge case.
func TestEnqueueNotification_RetryOfSameDedupeKeyAddsNothing(t *testing.T) {
	pool := pgtest.NewDB(t)
	repo := NewPushRepository(pool)
	ctx := context.Background()

	userID := uuid.NewString()
	subscriptionFor(t, repo, userID, allKindsOn())

	in := enqueueFor(userID, domain.NotificationKindSummaryReady, []byte(`{"url":"/a"}`))

	first, _, err := repo.EnqueueNotification(ctx, in)
	require.NoError(t, err)
	require.Equal(t, 1, first)

	second, _, err := repo.EnqueueNotification(ctx, in)
	require.NoError(t, err)
	require.Equal(t, 0, second, "ON CONFLICT DO NOTHING collapses the retry")

	var rows int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM push_deliveries WHERE dedupe_key = $1`, in.DedupeKey).Scan(&rows))
	require.Equal(t, 1, rows)
}

// The daily digest supersedes its own unsent predecessor through a partial
// unique index naming the kind literally. Both halves of that statement bind
// parameters the same way the fan-out does.
func TestEnqueueNotification_TodayEntranceSupersedesUnsentDigest(t *testing.T) {
	pool := pgtest.NewDB(t)
	repo := NewPushRepository(pool)
	ctx := context.Background()

	userID := uuid.NewString()
	subscriptionFor(t, repo, userID, allKindsOn())

	first := enqueueFor(userID, domain.NotificationKindTodayEntranceReady, []byte(`{"day":"2026-08-09"}`))
	fanned, superseded, err := repo.EnqueueNotification(ctx, first)
	require.NoError(t, err)
	require.Equal(t, 1, fanned)
	require.Equal(t, 0, superseded, "there was no earlier digest to supersede")

	second := enqueueFor(userID, domain.NotificationKindTodayEntranceReady, []byte(`{"day":"2026-08-10"}`))
	fanned, superseded, err = repo.EnqueueNotification(ctx, second)
	require.NoError(t, err)
	require.Equal(t, 1, fanned)
	require.Equal(t, 1, superseded, "yesterday's unsent digest is expired by today's")

	var state string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT state FROM push_deliveries WHERE dedupe_key = $1`, first.DedupeKey).Scan(&state))
	require.Equal(t, string(domain.NotificationExpired), state)
}

// The whole enqueue is one transaction. A supersede that lands while the
// fan-out fails would leave the user with no digest at all, so the rollback is
// the invariant rather than an implementation detail.
func TestEnqueueNotification_FanOutFailureRollsBackTheSupersede(t *testing.T) {
	pool := pgtest.NewDB(t)
	repo := NewPushRepository(pool)
	ctx := context.Background()

	userID := uuid.NewString()
	subscriptionFor(t, repo, userID, allKindsOn())

	first := enqueueFor(userID, domain.NotificationKindTodayEntranceReady, []byte(`{"day":"2026-08-09"}`))
	_, _, err := repo.EnqueueNotification(ctx, first)
	require.NoError(t, err)

	// kind is NOT NULL and the CHECK on state is unrelated; the reachable
	// server-side rejection here is a payload that is not JSON at all, which is
	// exactly what production was sending by accident.
	broken := enqueueFor(userID, domain.NotificationKindTodayEntranceReady, []byte(`not json`))
	_, _, err = repo.EnqueueNotification(ctx, broken)
	require.Error(t, err)

	var state string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT state FROM push_deliveries WHERE dedupe_key = $1`, first.DedupeKey).Scan(&state))
	require.Equal(t, string(domain.NotificationPending), state,
		"the supersede must not survive a failed fan-out")
}

// Round-tripping through the claim proves the row the dispatcher receives is
// the row the producer wrote — the step that never ran in production because
// push_deliveries was empty for the whole life of the service.
func TestEnqueueNotification_ClaimReturnsThePayloadAsWritten(t *testing.T) {
	pool := pgtest.NewDB(t)
	repo := NewPushRepository(pool)
	ctx := context.Background()

	userID := uuid.NewString()
	endpoint := subscriptionFor(t, repo, userID, allKindsOn())

	payload, err := json.Marshal(map[string]any{"url": "/articles/abc", "title": "日本語"})
	require.NoError(t, err)

	in := enqueueFor(userID, domain.NotificationKindSummaryReady, payload)
	_, _, err = repo.EnqueueNotification(ctx, in)
	require.NoError(t, err)

	claimed, err := repo.ClaimPushDeliveryBatch(ctx, "pgtest-dispatcher", 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.JSONEq(t, string(payload), string(claimed[0].Payload))
	require.Equal(t, endpoint, claimed[0].Endpoint)
	require.Equal(t, domain.NotificationKindSummaryReady, claimed[0].Kind)
}

// The backlog read is the dispatcher's whole view of the system, and the
// device population is part of it: an empty queue means "idle" when devices
// exist and "nobody can receive anything" when they do not.
func TestPushDeliveryBacklogAge_CountsDevicesAlongsideTheQueue(t *testing.T) {
	pool := pgtest.NewDB(t)
	repo := NewPushRepository(pool)
	ctx := context.Background()

	age, pending, subscriptions, err := repo.PushDeliveryBacklogAge(ctx)
	require.NoError(t, err)
	require.Zero(t, age)
	require.Zero(t, pending)
	require.Zero(t, subscriptions, "an unsubscribed deployment reports zero devices, not an error")

	userID := uuid.NewString()
	subscriptionFor(t, repo, userID, allKindsOn())
	subscriptionFor(t, repo, userID, allKindsOn())

	_, _, subscriptions, err = repo.PushDeliveryBacklogAge(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), subscriptions, "one row per device, not per user")

	in := enqueueFor(userID, domain.NotificationKindSummaryReady, []byte(`{"url":"/a"}`))
	_, _, err = repo.EnqueueNotification(ctx, in)
	require.NoError(t, err)

	age, pending, subscriptions, err = repo.PushDeliveryBacklogAge(ctx)
	require.NoError(t, err)
	require.Positive(t, age, "occurred_at is in the past, so the queue has an age")
	require.Equal(t, int64(2), pending)
	require.Equal(t, int64(2), subscriptions)
}

// A device count that only moved when the queue did would report the population
// of an hour ago for as long as nothing was enqueued.
func TestPushDeliveryBacklogAge_TracksDeviceRemoval(t *testing.T) {
	pool := pgtest.NewDB(t)
	repo := NewPushRepository(pool)
	ctx := context.Background()

	userID := uuid.NewString()
	endpoint := subscriptionFor(t, repo, userID, allKindsOn())

	_, _, subscriptions, err := repo.PushDeliveryBacklogAge(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), subscriptions)

	deleted, err := repo.DeletePushSubscription(ctx, userID, endpoint)
	require.NoError(t, err)
	require.True(t, deleted)

	_, _, subscriptions, err = repo.PushDeliveryBacklogAge(ctx)
	require.NoError(t, err)
	require.Zero(t, subscriptions, "the gauge has to fall when a device unsubscribes")
}
