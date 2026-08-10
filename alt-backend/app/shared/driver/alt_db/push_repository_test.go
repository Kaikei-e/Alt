package alt_db

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"alt/domain"

	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/require"
)

// normalizePushSQL collapses a statement's formatting to single spaces so an
// assertion can name a whole clause without also pinning its indentation.
// Re-indenting the SQL must not fail these tests; changing what it says must.
func normalizePushSQL(query string) string {
	return strings.Join(strings.Fields(query), " ")
}

// TestPushDeliveryBacklogAgeQueryCountsSendingRows pins the predicate rather
// than the plan.
//
// This is the reason the procedure exists. A row orphaned by a dispatcher that
// died mid-attempt stays in 'sending' until its lease expires; a query that
// matched only 'pending' would report an age of 0 while the queue was stuck,
// and the alert built on this metric would stay green through exactly the
// outage it was written to catch. Asserting on the SQL text is the only place
// that can be caught without a live database, because the wrong query returns
// a perfectly well-formed answer.
func TestPushDeliveryBacklogAgeQueryCountsSendingRows(t *testing.T) {
	require.Contains(t, pushDeliveryBacklogAgeQuery, "'sending'",
		"the backlog age must include rows a crashed dispatcher left in 'sending', "+
			"or a stuck queue reads as an empty one")
	require.Contains(t, pushDeliveryBacklogAgeQuery, "'pending'")

	// now() is the transaction start time, so inside a long transaction it
	// would understate the age by however long that transaction had been open.
	require.Contains(t, pushDeliveryBacklogAgeQuery, "clock_timestamp()")
	require.False(t, strings.Contains(pushDeliveryBacklogAgeQuery, " now()"),
		"the age must be read from clock_timestamp(), not the transaction clock")

	// An empty queue makes MIN() NULL; without COALESCE the scan fails and the
	// caller has a reason to skip the gauge instead of a value to publish.
	require.Contains(t, pushDeliveryBacklogAgeQuery, "COALESCE")
}

func TestPushRepository_PushDeliveryBacklogAge(t *testing.T) {
	tests := []struct {
		name          string
		seconds       float64
		pending       int64
		subscriptions int64
		wantOldest    time.Duration
		wantPending   int64
	}{
		{
			name:          "backlog reports age, depth and the device population",
			seconds:       123.5,
			pending:       7,
			subscriptions: 3,
			wantOldest:    123500 * time.Millisecond,
			wantPending:   7,
		},
		{
			name:          "empty queue is zero rather than absent",
			seconds:       0,
			pending:       0,
			subscriptions: 2,
			wantOldest:    0,
			wantPending:   0,
		},
		{
			// The reading that separates an idle deployment from a broken one.
			// Every other number here is identical in the two cases.
			name:          "an unsubscribed deployment reports zero devices",
			seconds:       0,
			pending:       0,
			subscriptions: 0,
			wantOldest:    0,
			wantPending:   0,
		},
		{
			// occurred_at is the producer's business time and nothing pins it
			// to the past, so a post-dated fact or a skewed producer clock
			// would otherwise publish a negative age.
			name:          "a future occurred_at clamps to zero",
			seconds:       -42,
			pending:       1,
			subscriptions: 1,
			wantOldest:    0,
			wantPending:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()

			mock.ExpectQuery(regexp.QuoteMeta(pushDeliveryBacklogAgeQuery)).
				WillReturnRows(pgxmock.NewRows([]string{
					"oldest_pending_age_seconds", "pending_count", "active_subscription_count",
				}).AddRow(tt.seconds, tt.pending, tt.subscriptions))

			repo := &PushRepository{pool: mock}
			oldest, pending, subscriptions, err := repo.PushDeliveryBacklogAge(context.Background())
			require.NoError(t, err)
			require.Equal(t, tt.wantOldest, oldest)
			require.Equal(t, tt.wantPending, pending)
			require.Equal(t, tt.subscriptions, subscriptions)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestPushRepository_PushDeliveryBacklogAge_QueryError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(regexp.QuoteMeta(pushDeliveryBacklogAgeQuery)).
		WillReturnError(errors.New("db failed"))

	repo := &PushRepository{pool: mock}
	oldest, pending, subscriptions, err := repo.PushDeliveryBacklogAge(context.Background())
	require.Error(t, err)
	require.ErrorContains(t, err, "read push delivery backlog age")
	require.Zero(t, oldest)
	require.Zero(t, pending)
	require.Zero(t, subscriptions)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// The claim: what the lease is made of
//
// Every assertion in this block is on the SQL text, and that is the only check
// available here rather than a shortcut. pgxmock never evaluates a predicate —
// it replays whatever rows the test hands it — so a claim with the wrong WHERE,
// the wrong lock mode, or no attempt increment returns a perfectly well-formed
// batch through the mock. Only a live database or the text can tell a correct
// claim from a wrong one, and the wrong one's symptom in production is
// "notifications stopped" with no error logged anywhere.
// ---------------------------------------------------------------------------

// TestClaimPushDeliveryBatchQueryReclaimsAbandonedSendingRows pins the claim's
// predicate.
//
// There is no reclaim sweeper anywhere in this service. The lease *is*
// next_attempt_at: a row whose dispatcher died mid-attempt stays in 'sending'
// with a lease that eventually elapses, and this statement is the only one that
// can ever pick it up again. Narrow the predicate to 'pending' and such a row is
// not reclaimed late — it is never reclaimed at all.
func TestClaimPushDeliveryBatchQueryReclaimsAbandonedSendingRows(t *testing.T) {
	query := normalizePushSQL(claimPushDeliveryBatchQuery)

	require.Contains(t, query, "WHERE state IN ('pending', 'sending')",
		"a claim that skips 'sending' can never reclaim a row abandoned by a crashed "+
			"dispatcher, and nothing else in this service will")

	// The lease is judged against clock_timestamp() rather than now(): now() is
	// the transaction's start time, so a dispatcher inside a longer transaction
	// would measure every lease against a stale reading and hand out rows whose
	// leases had not actually elapsed.
	require.Contains(t, query, "next_attempt_at <= clock_timestamp()")
	require.NotContains(t, query, " now()",
		"the lease must be judged against the statement clock, not the transaction clock")
}

// TestClaimPushDeliveryBatchQueryTakesTheRowLockWithSkipLocked pins what lets
// two dispatchers share one queue.
func TestClaimPushDeliveryBatchQueryTakesTheRowLockWithSkipLocked(t *testing.T) {
	query := normalizePushSQL(claimPushDeliveryBatchQuery)

	require.Contains(t, query, "FOR UPDATE SKIP LOCKED",
		"without SKIP LOCKED a second dispatcher blocks on the first one's rows; "+
			"without FOR UPDATE both claim the same row and the user is pushed twice")

	// The lock and the LIMIT belong to the same subquery, so the batch size cuts
	// the locked set down rather than the other way round: locking first and
	// limiting afterwards would leave one dispatcher holding rows it never
	// claims, invisible to the other for the length of the transaction.
	require.Contains(t, query, "ORDER BY next_attempt_at, id FOR UPDATE SKIP LOCKED LIMIT $2")
}

// TestClaimPushDeliveryBatchQueryWritesTheLeaseIntoNextAttemptAt pins the lease
// arithmetic itself.
func TestClaimPushDeliveryBatchQueryWritesTheLeaseIntoNextAttemptAt(t *testing.T) {
	query := normalizePushSQL(claimPushDeliveryBatchQuery)

	require.Contains(t, query,
		"next_attempt_at = clock_timestamp() + make_interval(secs => $3::double precision)",
		"next_attempt_at is the lease: a claim that does not push it forward leaves "+
			"the row immediately due again, and the next dispatcher pass delivers it a second time")

	// $3 is the third argument of ClaimPushDeliveryBatch's Query call, and it
	// arrives as a count of seconds. A lease bound to the wrong parameter would
	// silently use the batch limit as its duration.
	require.Contains(t, query, "LIMIT $2")
	require.Contains(t, query, "locked_by = $1")
}

// TestClaimPushDeliveryBatchQueryChargesAnAttemptAtClaimTime pins the retry
// budget's only decrement.
func TestClaimPushDeliveryBatchQueryChargesAnAttemptAtClaimTime(t *testing.T) {
	query := normalizePushSQL(claimPushDeliveryBatchQuery)

	require.Contains(t, query,
		"attempts = d.attempts + CASE WHEN d.expires_at <= clock_timestamp() THEN 0 ELSE 1 END",
		"the retry budget is spent at claim time, not at send time: a row that kills "+
			"its dispatcher between claim and send must still have cost an attempt, or it "+
			"is retried forever and the budget never exhausts")
}

// TestClaimPushDeliveryBatchQueryExpiresRatherThanDeliversStaleRows pins the
// expiry sweep that shares this statement.
//
// A backlog built up while the push service was down must drain rather than
// arrive days late, and the sweep is a branch of the claim rather than a second
// statement so there is no separate job to forget to schedule.
func TestClaimPushDeliveryBatchQueryExpiresRatherThanDeliversStaleRows(t *testing.T) {
	query := normalizePushSQL(claimPushDeliveryBatchQuery)

	require.Contains(t, query,
		"state = CASE WHEN d.expires_at <= clock_timestamp() THEN 'expired' ELSE 'sending' END")
	require.Contains(t, query,
		"finalized_at = CASE WHEN d.expires_at <= clock_timestamp() THEN clock_timestamp() ELSE NULL END")

	// A swept row must not reach the caller: this filter is what keeps an
	// expired row out of the returned batch even though the UPDATE touched it.
	require.Contains(t, query, "WHERE c.state = 'sending'")
}

// claimedPushDeliveryColumns is the claim's projection, in the order
// ClaimPushDeliveryBatch scans it.
var claimedPushDeliveryColumns = []string{
	"id", "dedupe_key", "subscription_id", "user_id", "kind", "payload",
	"occurred_at", "state", "attempts", "next_attempt_at", "expires_at",
	"endpoint", "p256dh", "auth",
}

func TestPushRepository_ClaimPushDeliveryBatch(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	occurredAt := time.Date(2026, 8, 8, 23, 0, 0, 0, time.UTC)
	nextAttemptAt := occurredAt.Add(30 * time.Second)
	expiresAt := occurredAt.Add(24 * time.Hour)

	mock.ExpectQuery(regexp.QuoteMeta(claimPushDeliveryBatchQuery)).
		// lockedBy, batch limit, and the lease as a count of seconds, in that
		// order. The lease reaches SQL as a double precision number of seconds,
		// so 30 * time.Second must arrive as 30 and not as its nanosecond count.
		WithArgs("dispatcher-a", 25, 30.0).
		WillReturnRows(pgxmock.NewRows(claimedPushDeliveryColumns).AddRow(
			"11111111-1111-1111-1111-111111111111", "recap:job-7",
			"22222222-2222-2222-2222-222222222222", "33333333-3333-3333-3333-333333333333",
			domain.NotificationKindRecapReady, []byte(`{"kind":"recap_ready"}`),
			occurredAt, "sending", 3, nextAttemptAt, expiresAt,
			"https://push.example/endpoint", "p256dh-key", "auth-secret",
		))

	repo := &PushRepository{pool: mock}
	deliveries, err := repo.ClaimPushDeliveryBatch(context.Background(), "dispatcher-a", 25, 30*time.Second)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)

	got := deliveries[0]
	require.Equal(t, domain.NotificationSending, got.State)
	// Attempts is what the dispatcher compares against its retry budget, so a
	// claim that dropped the column would make every row look brand new.
	require.Equal(t, 3, got.Attempts)
	require.Equal(t, nextAttemptAt, got.NextAttemptAt)
	require.Equal(t, expiresAt, got.ExpiresAt)
	require.Equal(t, occurredAt, got.OccurredAt)
	// The key material rides along with the claim: a batch without it would
	// force one lookup per delivery inside the lease window.
	require.Equal(t, "https://push.example/endpoint", got.Endpoint)
	require.Equal(t, "p256dh-key", got.P256dh)
	require.Equal(t, "auth-secret", got.Auth)
	require.JSONEq(t, `{"kind":"recap_ready"}`, string(got.Payload))

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestPushRepository_ClaimPushDeliveryBatch_NothingDue covers the pass that
// claims nothing, which is the overwhelmingly common one: the dispatcher ticks
// every couple of seconds and an empty queue must be an empty batch rather than
// an error the job would log as a failure.
func TestPushRepository_ClaimPushDeliveryBatch_NothingDue(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(regexp.QuoteMeta(claimPushDeliveryBatchQuery)).
		WithArgs("dispatcher-a", 10, 30.0).
		WillReturnRows(pgxmock.NewRows(claimedPushDeliveryColumns))

	repo := &PushRepository{pool: mock}
	deliveries, err := repo.ClaimPushDeliveryBatch(context.Background(), "dispatcher-a", 10, 30*time.Second)
	require.NoError(t, err)
	require.Empty(t, deliveries)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPushRepository_ClaimPushDeliveryBatch_QueryError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(regexp.QuoteMeta(claimPushDeliveryBatchQuery)).
		WithArgs("dispatcher-a", 10, 30.0).
		WillReturnError(errors.New("connection refused"))

	repo := &PushRepository{pool: mock}
	deliveries, err := repo.ClaimPushDeliveryBatch(context.Background(), "dispatcher-a", 10, 30*time.Second)
	require.ErrorContains(t, err, "claim push delivery batch")
	require.Nil(t, deliveries)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestPushRepository_ClaimPushDeliveryBatch_ScanErrorFailsTheBatch pins that a
// row the driver cannot decode aborts the pass instead of being skipped.
//
// The rows are already claimed by the time they are scanned — the UPDATE
// committed with the statement — so silently dropping one would leave it leased
// to a dispatcher that never saw it, and it would sit unsent for a whole lease.
func TestPushRepository_ClaimPushDeliveryBatch_ScanErrorFailsTheBatch(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	occurredAt := time.Date(2026, 8, 8, 23, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(claimPushDeliveryBatchQuery)).
		WithArgs("dispatcher-a", 10, 30.0).
		WillReturnRows(pgxmock.NewRows(claimedPushDeliveryColumns).AddRow(
			"11111111-1111-1111-1111-111111111111", "recap:job-7",
			"22222222-2222-2222-2222-222222222222", "33333333-3333-3333-3333-333333333333",
			domain.NotificationKindRecapReady, []byte(`{}`),
			occurredAt, "sending", 1, occurredAt, occurredAt,
			"https://push.example/endpoint", "p256dh-key", "auth-secret",
		).RowError(0, errors.New("bad column")))

	repo := &PushRepository{pool: mock}
	deliveries, err := repo.ClaimPushDeliveryBatch(context.Background(), "dispatcher-a", 10, 30*time.Second)
	require.ErrorContains(t, err, "scan claimed push delivery")
	require.Nil(t, deliveries)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestPushRepository_ClaimPushDeliveryBatch_IterationErrorFailsTheBatch covers
// the connection dropping part-way through the result set, which pgx reports
// only through Rows.Err(). Ignoring it would return a truncated batch that looks
// exactly like a short queue.
func TestPushRepository_ClaimPushDeliveryBatch_IterationErrorFailsTheBatch(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	occurredAt := time.Date(2026, 8, 8, 23, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(claimPushDeliveryBatchQuery)).
		WithArgs("dispatcher-a", 10, 30.0).
		WillReturnRows(pgxmock.NewRows(claimedPushDeliveryColumns).AddRow(
			"11111111-1111-1111-1111-111111111111", "recap:job-7",
			"22222222-2222-2222-2222-222222222222", "33333333-3333-3333-3333-333333333333",
			domain.NotificationKindRecapReady, []byte(`{}`),
			occurredAt, "sending", 1, occurredAt, occurredAt,
			"https://push.example/endpoint", "p256dh-key", "auth-secret",
		).CloseError(errors.New("connection lost")))

	repo := &PushRepository{pool: mock}
	deliveries, err := repo.ClaimPushDeliveryBatch(context.Background(), "dispatcher-a", 10, 30*time.Second)
	require.ErrorContains(t, err, "iterate claimed push deliveries")
	require.Nil(t, deliveries)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// The fan-out: who a notification reaches
// ---------------------------------------------------------------------------

// TestFanOutNotificationQueryFiltersOnThePerKindPreference pins each kind to the
// column that decides whether a device wants it.
//
// The preference is evaluated in SQL, so it is again only checkable as text: the
// mock never runs the CASE, and a wrong arm produces a syntactically valid
// statement whose only symptom is a notification arriving on a device that
// turned that kind off. Pairing the domain constant with the column also catches
// the reverse mistake — renaming a constant without editing the SQL leaves the
// arm unreachable, the kind falls through to ELSE, and nobody is notified at all.
func TestFanOutNotificationQueryFiltersOnThePerKindPreference(t *testing.T) {
	query := normalizePushSQL(fanOutNotificationQuery)

	kinds := []struct {
		kind   string
		column string
	}{
		{domain.NotificationKindSummaryReady, "s.summary_ready"},
		{domain.NotificationKindAcolyteReportReady, "s.acolyte_report_ready"},
		{domain.NotificationKindRecapReady, "s.recap_ready"},
		{domain.NotificationKindTodayEntranceReady, "s.today_entrance_ready"},
	}
	for _, tt := range kinds {
		t.Run(tt.kind, func(t *testing.T) {
			require.Contains(t, query, "WHEN '"+tt.kind+"' THEN "+tt.column,
				"this kind must be gated on its own preference column")
		})
	}

	require.Contains(t, query, "ELSE FALSE",
		"a kind this statement does not recognise must reach nobody rather than everybody")
	require.Contains(t, query, "WHERE s.user_id = $2",
		"the fan-out is one user's devices; without the scope it would enqueue against every subscription in the table")
}

// TestFanOutNotificationQueryIsIdempotentPerDevice pins the conflict target.
func TestFanOutNotificationQueryIsIdempotentPerDevice(t *testing.T) {
	query := normalizePushSQL(fanOutNotificationQuery)

	require.Contains(t, query, "ON CONFLICT (dedupe_key, subscription_id) DO NOTHING",
		"the idempotency key is per device: conflicting on dedupe_key alone would let "+
			"the first device's row suppress every other device of the same user, and dropping "+
			"the clause turns a relayed retry into a second push")
}

// TestSupersedePendingDigestQueryMatchesThePartialUniqueIndex pins the predicate
// that keeps the daily digest to one unsent copy.
//
// idx_push_deliveries_pending_digest is UNIQUE (subscription_id, kind) WHERE
// state = 'pending' AND kind = 'today_entrance_ready'. The insert that follows
// this UPDATE resolves conflicts on (dedupe_key, subscription_id) and cannot
// absorb a violation of that other index, so this statement has to clear exactly
// the rows the index covers or the next day's enqueue aborts the whole
// transaction on a unique violation.
func TestSupersedePendingDigestQueryMatchesThePartialUniqueIndex(t *testing.T) {
	query := normalizePushSQL(supersedePendingDigestQuery)

	require.Contains(t, query, "SET state = 'expired'")
	require.Contains(t, query, "d.kind = '"+domain.NotificationKindTodayEntranceReady+"'",
		"only the daily digest supersedes; the other kinds stack because each one describes a different fact")
	require.Contains(t, query, "d.state = 'pending'")

	// Deliberately scoped to 'pending'. A digest already handed to a dispatcher
	// is in 'sending', outside the partial index, and finalising it here would
	// overwrite the outcome of a row another process holds the lease on.
	require.NotContains(t, query, "'sending'")
}

// TestSupersedePendingDigestQueryExcludesTheIncomingKey is the regression guard
// for a digest that destroyed itself.
//
// The daily job ticks every ten minutes and gates on the UTC hour, so six
// firings share one date and therefore one dedupe key. Without this exclusion
// the second firing expired the rows the first had created, and the fan-out
// that follows inserted nothing because ON CONFLICT DO NOTHING matched the same
// key — leaving the user with a digest marked "superseded by a newer daily
// digest" when no newer digest existed.
//
// It is reachable well outside the trigger hour: a retryable send failure
// releases a row back to 'pending', and any backoff longer than the tick
// interval puts it in the path of the next firing.
func TestSupersedePendingDigestQueryExcludesTheIncomingKey(t *testing.T) {
	query := normalizePushSQL(supersedePendingDigestQuery)

	require.Contains(t, query, "d.dedupe_key <> $2",
		"a re-enqueue of the same digest must be a no-op, not a self-destruct")
}

func TestPushRepository_EnqueueNotification_DigestSupersedesBeforeItFansOut(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	in := domain.NotificationEnqueue{
		DedupeKey:  "digest:33333333-3333-3333-3333-333333333333:2026-08-08",
		UserID:     "33333333-3333-3333-3333-333333333333",
		Kind:       domain.NotificationKindTodayEntranceReady,
		Payload:    []byte(`{"kind":"today_entrance_ready","url":"/home","count":5}`),
		OccurredAt: time.Date(2026, 8, 8, 23, 0, 0, 0, time.UTC),
		ExpiresAt:  time.Date(2026, 8, 9, 23, 0, 0, 0, time.UTC),
	}

	// Expectations match in order, so this also pins that the supersede runs
	// before the insert and inside the same transaction. Run the other way round
	// the UPDATE would expire the digest the INSERT had just created, and split
	// across two transactions a crash between them would leave the user with no
	// digest at all rather than with the older one.
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(supersedePendingDigestQuery)).
		WithArgs(in.UserID, in.DedupeKey).
		WillReturnResult(pgxmock.NewResult("UPDATE", 2))
	mock.ExpectExec(regexp.QuoteMeta(fanOutNotificationQuery)).
		WithArgs(in.DedupeKey, in.UserID, in.Kind, string(in.Payload), in.OccurredAt, in.ExpiresAt).
		WillReturnResult(pgxmock.NewResult("INSERT", 3))
	mock.ExpectCommit()

	repo := &PushRepository{pool: mock}
	delivered, superseded, err := repo.EnqueueNotification(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, 3, delivered)
	require.Equal(t, 2, superseded)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestPushRepository_EnqueueNotification_OtherKindsNeverSupersede pins that the
// supersede is the digest's alone.
//
// Two summaries, two reports or two recaps are two different facts and both are
// worth delivering; expiring the older one would drop a notification the user
// has no other way to learn about. The mock enforces this by matching the first
// Exec against the fan-out statement: an unconditional supersede would arrive
// first and fail to match.
func TestPushRepository_EnqueueNotification_OtherKindsNeverSupersede(t *testing.T) {
	kinds := []string{
		domain.NotificationKindSummaryReady,
		domain.NotificationKindAcolyteReportReady,
		domain.NotificationKindRecapReady,
	}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()

			in := domain.NotificationEnqueue{
				DedupeKey:  kind + ":job-7",
				UserID:     "33333333-3333-3333-3333-333333333333",
				Kind:       kind,
				Payload:    []byte(`{"url":"/home"}`),
				OccurredAt: time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
				ExpiresAt:  time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC),
			}

			mock.ExpectBegin()
			mock.ExpectExec(regexp.QuoteMeta(fanOutNotificationQuery)).
				WithArgs(in.DedupeKey, in.UserID, in.Kind, string(in.Payload), in.OccurredAt, in.ExpiresAt).
				WillReturnResult(pgxmock.NewResult("INSERT", 2))
			mock.ExpectCommit()

			repo := &PushRepository{pool: mock}
			delivered, superseded, err := repo.EnqueueNotification(context.Background(), in)
			require.NoError(t, err)
			require.Equal(t, 2, delivered)
			require.Zero(t, superseded)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestPushRepository_EnqueueNotification_ReportsWhatItExpiredWithoutInserting
// pins the return shape that is worth reading as a warning rather than as a
// success.
//
// (0 delivered, N superseded) is a legitimate outcome and the counts must
// survive it: a genuinely newer digest expires yesterday's rows and, if every
// device already holds a row for the new key, inserts nothing.
//
// It used to be reachable for a much worse reason. The UPDATE was unconditional
// for the digest kind while the insert resolved (dedupe_key, subscription_id)
// with DO NOTHING, so a repeat enqueue carrying the *same* key expired the rows
// it was about to recreate and then created none — the digest destroying itself,
// with last_error blaming a newer digest that did not exist. The exclusion in
// supersedePendingDigestQuery closes that, and
// TestSupersedePendingDigestQueryExcludesTheIncomingKey is what keeps it closed.
//
// The counts here come from the mock, so this test asserts the plumbing rather
// than the predicate.
func TestPushRepository_EnqueueNotification_ReportsWhatItExpiredWithoutInserting(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	in := domain.NotificationEnqueue{
		DedupeKey:  "digest:33333333-3333-3333-3333-333333333333:2026-08-08",
		UserID:     "33333333-3333-3333-3333-333333333333",
		Kind:       domain.NotificationKindTodayEntranceReady,
		Payload:    []byte(`{"kind":"today_entrance_ready","url":"/home","count":5}`),
		OccurredAt: time.Date(2026, 8, 8, 23, 10, 0, 0, time.UTC),
		ExpiresAt:  time.Date(2026, 8, 9, 23, 10, 0, 0, time.UTC),
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(supersedePendingDigestQuery)).
		WithArgs(in.UserID, in.DedupeKey).
		WillReturnResult(pgxmock.NewResult("UPDATE", 2))
	mock.ExpectExec(regexp.QuoteMeta(fanOutNotificationQuery)).
		WithArgs(in.DedupeKey, in.UserID, in.Kind, string(in.Payload), in.OccurredAt, in.ExpiresAt).
		WillReturnResult(pgxmock.NewResult("INSERT", 0))
	mock.ExpectCommit()

	repo := &PushRepository{pool: mock}
	delivered, superseded, err := repo.EnqueueNotification(context.Background(), in)
	require.NoError(t, err)
	require.Zero(t, delivered)
	require.Equal(t, 2, superseded,
		"the caller must be able to see that rows were expired even though none were created")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPushRepository_EnqueueNotification_BeginError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectBegin().WillReturnError(errors.New("pool exhausted"))

	repo := &PushRepository{pool: mock}
	delivered, superseded, err := repo.EnqueueNotification(context.Background(), domain.NotificationEnqueue{
		DedupeKey: "recap:job-7",
		UserID:    "33333333-3333-3333-3333-333333333333",
		Kind:      domain.NotificationKindRecapReady,
	})
	require.ErrorContains(t, err, "begin enqueue notification tx")
	require.Zero(t, delivered)
	require.Zero(t, superseded)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestPushRepository_EnqueueNotification_SupersedeErrorRollsBack pins that a
// failed supersede aborts the enqueue rather than falling through to the insert.
//
// Falling through is the dangerous direction: the insert would then hit
// idx_push_deliveries_pending_digest with the older digest still pending, and
// the ON CONFLICT clause names a different index, so the failure would surface
// as an opaque unique violation instead of as the error that actually happened.
func TestPushRepository_EnqueueNotification_SupersedeErrorRollsBack(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	in := domain.NotificationEnqueue{
		DedupeKey: "digest:33333333-3333-3333-3333-333333333333:2026-08-08",
		UserID:    "33333333-3333-3333-3333-333333333333",
		Kind:      domain.NotificationKindTodayEntranceReady,
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(supersedePendingDigestQuery)).
		WithArgs(in.UserID, in.DedupeKey).
		WillReturnError(errors.New("deadlock detected"))
	mock.ExpectRollback()

	repo := &PushRepository{pool: mock}
	delivered, superseded, err := repo.EnqueueNotification(context.Background(), in)
	require.ErrorContains(t, err, "supersede pending digest")
	require.Zero(t, delivered)
	require.Zero(t, superseded)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPushRepository_EnqueueNotification_FanOutErrorRollsBack(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	in := domain.NotificationEnqueue{
		DedupeKey:  "recap:job-7",
		UserID:     "33333333-3333-3333-3333-333333333333",
		Kind:       domain.NotificationKindRecapReady,
		Payload:    []byte(`{"url":"/home"}`),
		OccurredAt: time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
		ExpiresAt:  time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC),
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(fanOutNotificationQuery)).
		WithArgs(in.DedupeKey, in.UserID, in.Kind, string(in.Payload), in.OccurredAt, in.ExpiresAt).
		WillReturnError(errors.New("unique violation"))
	mock.ExpectRollback()

	repo := &PushRepository{pool: mock}
	delivered, superseded, err := repo.EnqueueNotification(context.Background(), in)
	// The dedupe key is the only handle a relay has on the enqueue it retried,
	// and unlike the endpoint it is safe to name.
	require.ErrorContains(t, err, "fan out notification recap:job-7")
	require.Zero(t, delivered)
	require.Zero(t, superseded)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestPushRepository_EnqueueNotification_CommitErrorReportsNothingEnqueued pins
// that a failed commit is not reported as a successful fan-out. The relay
// retries on the error, and the rolled-back rows are re-created by the retry;
// reporting the pre-commit row count would strand the notification instead.
func TestPushRepository_EnqueueNotification_CommitErrorReportsNothingEnqueued(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	in := domain.NotificationEnqueue{
		DedupeKey:  "digest:33333333-3333-3333-3333-333333333333:2026-08-08",
		UserID:     "33333333-3333-3333-3333-333333333333",
		Kind:       domain.NotificationKindTodayEntranceReady,
		Payload:    []byte(`{"kind":"today_entrance_ready","url":"/home","count":5}`),
		OccurredAt: time.Date(2026, 8, 8, 23, 0, 0, 0, time.UTC),
		ExpiresAt:  time.Date(2026, 8, 9, 23, 0, 0, 0, time.UTC),
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(supersedePendingDigestQuery)).
		WithArgs(in.UserID, in.DedupeKey).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(regexp.QuoteMeta(fanOutNotificationQuery)).
		WithArgs(in.DedupeKey, in.UserID, in.Kind, string(in.Payload), in.OccurredAt, in.ExpiresAt).
		WillReturnResult(pgxmock.NewResult("INSERT", 2))
	mock.ExpectCommit().WillReturnError(errors.New("server closed the connection"))

	repo := &PushRepository{pool: mock}
	delivered, superseded, err := repo.EnqueueNotification(context.Background(), in)
	require.ErrorContains(t, err, "commit enqueue notification")
	require.Zero(t, delivered)
	require.Zero(t, superseded)
	require.NoError(t, mock.ExpectationsWereMet())
}
