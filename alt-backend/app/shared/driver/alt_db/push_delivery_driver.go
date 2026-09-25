package alt_db

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
)

// supersedePendingDigestQuery expires an unsent daily digest for every device
// of one user, so the next day's enqueue replaces it rather than stacking
// behind it.
//
// It is a separate statement from the insert below rather than an ON CONFLICT
// clause because the insert already has a conflict target — the (dedupe_key,
// subscription_id) idempotency key — and one INSERT cannot resolve two
// different unique indexes two different ways. Both statements run in one
// transaction, so the RPC is still one transaction boundary.
// The incoming key is excluded, and that exclusion is load-bearing rather than
// defensive. The daily job ticks every ten minutes and gates on the UTC hour,
// so six firings share one date and therefore one dedupe key. Without the
// exclusion the second firing expires the rows the first created and then
// inserts nothing, because the fan-out's ON CONFLICT DO NOTHING sees the same
// key — the digest destroys itself, and does so precisely when delivery is
// slow enough for the rows to still be pending, which is when it was most
// needed. A retryable send failure puts a row back in 'pending' too, so this
// is reachable well beyond the trigger hour.
const supersedePendingDigestQuery = `
	UPDATE push_deliveries d
	SET state = 'expired',
	    finalized_at = clock_timestamp(),
	    last_error = 'superseded by a newer daily digest'
	WHERE d.user_id = $1
	  AND d.kind = 'today_entrance_ready'
	  AND d.state = 'pending'
	  AND d.dedupe_key <> $2
`

// fanOutNotificationQuery creates one row per device that still wants this
// kind.
//
// The preference is read here, at fan-out, rather than at send time: a device
// that has the kind switched off should not have work queued against it at
// all, and evaluating it later would mean a claim that hands the dispatcher
// rows it is going to discard.
//
// ON CONFLICT DO NOTHING is scoped to the idempotency key. A relayed retry of
// the same enqueue therefore adds nothing, while a genuinely new notification
// of the same kind is a new row.
const fanOutNotificationQuery = `
	INSERT INTO push_deliveries (
		dedupe_key, subscription_id, user_id, kind, payload, occurred_at, expires_at
	)
	SELECT $1, s.id, s.user_id, $3, $4, $5, $6
	FROM push_subscriptions s
	WHERE s.user_id = $2
	  AND CASE $3
	        WHEN 'summary_ready'        THEN s.summary_ready
	        WHEN 'acolyte_report_ready' THEN s.acolyte_report_ready
	        WHEN 'recap_ready'          THEN s.recap_ready
	        WHEN 'today_entrance_ready' THEN s.today_entrance_ready
	        ELSE FALSE
	      END
	ON CONFLICT (dedupe_key, subscription_id) DO NOTHING
`

// EnqueueNotification fans one notification out to a user's devices and
// reports how many rows it created and how many older digests it superseded.
func (r *PushRepository) EnqueueNotification(ctx context.Context, in domain.NotificationEnqueue) (int, int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("begin enqueue notification tx: %w", err)
	}
	// Rollback after a successful Commit is a no-op that returns
	// pgx.ErrTxClosed, so the error is discarded rather than checked — there is
	// no outcome it could change. Written as a closure because errcheck cannot
	// tell that apart from a rollback whose failure matters.
	defer func() { _ = tx.Rollback(ctx) }()

	var superseded int64
	if in.Kind == domain.NotificationKindTodayEntranceReady {
		tag, err := tx.Exec(ctx, supersedePendingDigestQuery, in.UserID, in.DedupeKey)
		if err != nil {
			return 0, 0, fmt.Errorf("supersede pending digest: %w", err)
		}
		superseded = tag.RowsAffected()
	}

	// Payload goes as a string so pgx encodes it as text and the JSONB column
	// parses it. The pool runs simple protocol for PgBouncer transaction
	// pooling (init.go), which interpolates arguments client-side: a []byte
	// there is a bytea, and `\x7b22...` is not JSON. Same reason as
	// scraping_domain_driver.go and saveOutboxEventWithTx.
	tag, err := tx.Exec(ctx, fanOutNotificationQuery,
		in.DedupeKey, in.UserID, in.Kind, string(in.Payload), in.OccurredAt, in.ExpiresAt)
	if err != nil {
		return 0, 0, fmt.Errorf("fan out notification %s: %w", in.DedupeKey, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("commit enqueue notification: %w", err)
	}
	return int(tag.RowsAffected()), int(superseded), nil
}

// claimPushDeliveryBatchQuery is the whole delivery lease, in one statement.
//
// It covers fresh work and crash reclaim together: the inner SELECT matches
// `state IN ('pending','sending')`, so a row whose dispatcher died mid-attempt
// is picked up by the same query that picks up new work, once its lease has
// elapsed. Making the lease *be* next_attempt_at is what buys that — there is
// no separate reclaim sweeper, and therefore none to forget to schedule.
//
// FOR UPDATE SKIP LOCKED keeps two dispatchers off the same row without either
// of them waiting. The expiry sweep is a WHERE branch of the same statement
// rather than a second query: a row past expires_at is finalised as `expired`
// instead of being handed out, so a backlog that built up while the push
// service was down drains rather than being delivered days late.
//
// The join is what makes a claimed row sendable. Without it the dispatcher
// would need one lookup per delivery, inside the lease window, to learn where
// to send.
const claimPushDeliveryBatchQuery = `
	WITH due AS (
		SELECT id
		FROM push_deliveries
		WHERE state IN ('pending', 'sending')
		  AND next_attempt_at <= clock_timestamp()
		ORDER BY next_attempt_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT $2
	), claimed AS (
		UPDATE push_deliveries d
		SET state = CASE WHEN d.expires_at <= clock_timestamp() THEN 'expired' ELSE 'sending' END,
		    attempts = d.attempts + CASE WHEN d.expires_at <= clock_timestamp() THEN 0 ELSE 1 END,
		    locked_by = $1,
		    next_attempt_at = clock_timestamp() + make_interval(secs => $3::double precision),
		    finalized_at = CASE WHEN d.expires_at <= clock_timestamp() THEN clock_timestamp() ELSE NULL END
		FROM due
		WHERE d.id = due.id
		RETURNING d.id, d.dedupe_key, d.subscription_id, d.user_id, d.kind, d.payload,
		          d.occurred_at, d.state, d.attempts, d.next_attempt_at, d.expires_at
	)
	SELECT c.id::text, c.dedupe_key, c.subscription_id::text, c.user_id::text, c.kind,
	       c.payload, c.occurred_at, c.state, c.attempts, c.next_attempt_at, c.expires_at,
	       s.endpoint, s.p256dh, s.auth
	FROM claimed c
	JOIN push_subscriptions s ON s.id = c.subscription_id
	WHERE c.state = 'sending'
`

// ClaimPushDeliveryBatch takes the lease on up to limit due rows.
func (r *PushRepository) ClaimPushDeliveryBatch(ctx context.Context, lockedBy string, limit int, lease time.Duration) ([]domain.PushDelivery, error) {
	rows, err := r.pool.Query(ctx, claimPushDeliveryBatchQuery, lockedBy, limit, lease.Seconds())
	if err != nil {
		return nil, fmt.Errorf("claim push delivery batch: %w", err)
	}
	defer rows.Close()

	var deliveries []domain.PushDelivery
	for rows.Next() {
		var (
			d     domain.PushDelivery
			state string
		)
		if err := rows.Scan(
			&d.ID, &d.DedupeKey, &d.SubscriptionID, &d.UserID, &d.Kind, &d.Payload,
			&d.OccurredAt, &state, &d.Attempts, &d.NextAttemptAt, &d.ExpiresAt,
			&d.Endpoint, &d.P256dh, &d.Auth,
		); err != nil {
			return nil, fmt.Errorf("scan claimed push delivery: %w", err)
		}
		d.State = domain.NotificationState(state)
		deliveries = append(deliveries, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed push deliveries: %w", err)
	}
	return deliveries, nil
}

// MarkPushDeliverySent finalises a claimed row as delivered.
//
// The WHERE pins state = 'sending' so a row whose lease expired and was
// re-claimed by another dispatcher is not finalised by the loser of that race:
// the late writer affects no row rather than overwriting the winner's outcome.
func (r *PushRepository) MarkPushDeliverySent(ctx context.Context, id string, statusCode int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE push_deliveries
		SET state = 'sent', last_status = $2, last_error = NULL, finalized_at = clock_timestamp()
		WHERE id = $1 AND state = 'sending'`, id, nullableInt(statusCode))
	if err != nil {
		return fmt.Errorf("mark push delivery %s sent: %w", id, err)
	}
	return nil
}

// ReleasePushDelivery returns a claimed row to PENDING at the caller's next
// attempt time.
func (r *PushRepository) ReleasePushDelivery(ctx context.Context, id string, nextAttemptAt time.Time, errorMessage string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE push_deliveries
		SET state = 'pending', next_attempt_at = $2, last_error = $3, locked_by = NULL
		WHERE id = $1 AND state = 'sending'`, id, nextAttemptAt, nullableText(errorMessage))
	if err != nil {
		return fmt.Errorf("release push delivery %s: %w", id, err)
	}
	return nil
}

// MarkPushDeliveryDead finalises a claimed row as undeliverable.
func (r *PushRepository) MarkPushDeliveryDead(ctx context.Context, id string, statusCode int, errorMessage string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE push_deliveries
		SET state = 'dead', last_status = $2, last_error = $3, finalized_at = clock_timestamp()
		WHERE id = $1 AND state = 'sending'`, id, nullableInt(statusCode), nullableText(errorMessage))
	if err != nil {
		return fmt.Errorf("mark push delivery %s dead: %w", id, err)
	}
	return nil
}

// pushDeliveryBacklogAgeQuery reports the queue's staleness and its depth in
// one row.
//
// `state IN ('pending', 'sending')` is the whole point of the statement. The
// lease *is* next_attempt_at, so a row abandoned by a dispatcher that died
// mid-attempt sits in 'sending' until that lease elapses; a query matching only
// 'pending' would answer 0 for a queue nothing is draining, and the alert built
// on this number would stay green through precisely the outage it exists to
// notice. It is the same predicate the claim statement above uses, and the
// partial claim index covers it.
//
// The age is measured from occurred_at — business time — so it answers "how
// stale is the oldest thing a user has not been told about" rather than "how
// long has a row sat in this table", which keeps relay lag inside the number
// instead of behind it.
//
// clock_timestamp() rather than now(): now() is the transaction's start time,
// so inside a long-running transaction it would understate the age by however
// long that transaction had been open.
//
// COALESCE turns the NULL that MIN() over no rows produces into 0, so the
// caller always has a value to publish rather than a reason to skip the gauge —
// and a gauge that stops being set keeps reporting its last value.
//
// The device count is a scalar subquery over a second table rather than a
// join: joining would multiply the queue rows by the devices and turn both
// aggregates into the wrong number, and two round trips would sample the two
// facts at different instants, which is exactly the comparison the caller
// needs to be able to make.
const pushDeliveryBacklogAgeQuery = `
	SELECT COALESCE(EXTRACT(EPOCH FROM (clock_timestamp() - MIN(occurred_at))), 0),
	       COUNT(*),
	       (SELECT COUNT(*) FROM push_subscriptions)
	FROM push_deliveries
	WHERE state IN ('pending', 'sending')
`

// PushDeliveryBacklogAge returns the age of the oldest non-terminal row, how
// many non-terminal rows there are, and how many devices are registered to
// receive anything. An empty queue on an unsubscribed deployment is (0, 0, 0)
// and not an error.
func (r *PushRepository) PushDeliveryBacklogAge(ctx context.Context) (time.Duration, int64, int64, error) {
	var (
		seconds       float64
		pending       int64
		subscriptions int64
	)
	if err := r.pool.QueryRow(ctx, pushDeliveryBacklogAgeQuery).Scan(&seconds, &pending, &subscriptions); err != nil {
		return 0, 0, 0, fmt.Errorf("read push delivery backlog age: %w", err)
	}
	// occurred_at is the producer's business time and nothing constrains it to
	// the past, so a post-dated fact or a skewed producer clock would otherwise
	// publish a negative age.
	if seconds < 0 {
		seconds = 0
	}
	return time.Duration(seconds * float64(time.Second)), pending, subscriptions, nil
}

// nullableText keeps an empty message out of last_error, so "no reason
// recorded" reads as NULL rather than as an empty reason.
func nullableText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// nullableInt keeps a zero status out of last_status: 0 is not an HTTP status,
// it is "the attempt never reached a response".
func nullableInt(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}
