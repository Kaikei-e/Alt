package alt_db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"alt/domain"

	"github.com/jackc/pgx/v5"
)

// upsertPushSubscriptionQuery replaces the key material and preferences of an
// endpoint already registered.
//
// The ON CONFLICT clause is scoped to the owning user. A browser endpoint is
// globally unique in practice, but "in practice" is not a constraint: without
// the WHERE, anyone able to guess or replay another person's endpoint could
// point it at their own user_id and start receiving that person's
// notifications. With it, the write affects no row and the caller learns
// nothing.
//
// `created` is derived from xmax rather than from a preceding SELECT: xmax is
// zero exactly for a tuple this statement inserted, so one round trip answers
// both "store it" and "was this device new".
const upsertPushSubscriptionQuery = `
	INSERT INTO push_subscriptions (
		user_id, endpoint, p256dh, auth,
		summary_ready, acolyte_report_ready, recap_ready, today_entrance_ready,
		vapid_key_fingerprint
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	ON CONFLICT (endpoint) DO UPDATE SET
		p256dh                = EXCLUDED.p256dh,
		auth                  = EXCLUDED.auth,
		summary_ready         = EXCLUDED.summary_ready,
		acolyte_report_ready  = EXCLUDED.acolyte_report_ready,
		recap_ready           = EXCLUDED.recap_ready,
		today_entrance_ready  = EXCLUDED.today_entrance_ready,
		vapid_key_fingerprint = EXCLUDED.vapid_key_fingerprint,
		updated_at            = clock_timestamp()
	WHERE push_subscriptions.user_id = EXCLUDED.user_id
	RETURNING (xmax = 0) AS created
`

// UpsertPushSubscription stores a subscription and reports whether the row was
// newly inserted.
func (r *PushRepository) UpsertPushSubscription(ctx context.Context, sub domain.PushSubscription) (bool, error) {
	var created bool
	err := r.pool.QueryRow(ctx, upsertPushSubscriptionQuery,
		sub.UserID, sub.Endpoint, sub.P256dh, sub.Auth,
		sub.Preferences.SummaryReady, sub.Preferences.AcolyteReportReady,
		sub.Preferences.RecapReady, sub.Preferences.TodayEntranceReady,
		sub.VAPIDKeyFingerprint,
	).Scan(&created)
	if errors.Is(err, pgx.ErrNoRows) {
		// The ON CONFLICT ... WHERE excluded the row: this endpoint belongs to
		// a different user. Reported as a plain conflict, without echoing the
		// endpoint or naming the owner.
		return false, domain.ErrPushSubscriptionOwnedByAnotherUser
	}
	if err != nil {
		return false, fmt.Errorf("upsert push subscription: %w", err)
	}
	return created, nil
}

const pushSubscriptionColumns = `
	user_id, endpoint, p256dh, auth,
	summary_ready, acolyte_report_ready, recap_ready, today_entrance_ready,
	vapid_key_fingerprint, created_at, updated_at, last_success_at, last_failure_at
`

// GetPushSubscription returns nil without error when this user has no
// subscription at that endpoint.
func (r *PushRepository) GetPushSubscription(ctx context.Context, userID, endpoint string) (*domain.PushSubscription, error) {
	query := `SELECT ` + pushSubscriptionColumns + `
		FROM push_subscriptions
		WHERE endpoint = $1 AND user_id = $2`

	sub, err := scanPushSubscription(r.pool.QueryRow(ctx, query, endpoint, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get push subscription: %w", err)
	}
	return &sub, nil
}

const updatePushSubscriptionPreferencesQuery = `
	UPDATE push_subscriptions
	SET summary_ready        = $3,
	    acolyte_report_ready = $4,
	    recap_ready          = $5,
	    today_entrance_ready = $6,
	    updated_at           = clock_timestamp()
	WHERE endpoint = $1 AND user_id = $2
`

// UpdatePushSubscriptionPreferences writes all four booleans and reports
// whether a row matched.
func (r *PushRepository) UpdatePushSubscriptionPreferences(ctx context.Context, userID, endpoint string, prefs domain.NotificationPreferences) (bool, error) {
	tag, err := r.pool.Exec(ctx, updatePushSubscriptionPreferencesQuery,
		endpoint, userID,
		prefs.SummaryReady, prefs.AcolyteReportReady, prefs.RecapReady, prefs.TodayEntranceReady)
	if err != nil {
		return false, fmt.Errorf("update push subscription preferences: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// DeletePushSubscription removes one device and reports whether there was one.
func (r *PushRepository) DeletePushSubscription(ctx context.Context, userID, endpoint string) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM push_subscriptions WHERE endpoint = $1 AND user_id = $2`, endpoint, userID)
	if err != nil {
		return false, fmt.Errorf("delete push subscription: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ListPushSubscriptionsForUser returns every device of one user, oldest first
// so the fan-out order is stable across calls.
func (r *PushRepository) ListPushSubscriptionsForUser(ctx context.Context, userID string) ([]domain.PushSubscription, error) {
	query := `SELECT ` + pushSubscriptionColumns + `
		FROM push_subscriptions
		WHERE user_id = $1
		ORDER BY created_at ASC, endpoint ASC`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list push subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []domain.PushSubscription
	for rows.Next() {
		sub, err := scanPushSubscription(rows)
		if err != nil {
			return nil, fmt.Errorf("scan push subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate push subscriptions: %w", err)
	}
	return subs, nil
}

func scanPushSubscription(row rowScanner) (domain.PushSubscription, error) {
	var (
		sub           domain.PushSubscription
		lastSuccessAt *time.Time
		lastFailureAt *time.Time
	)
	err := row.Scan(
		&sub.UserID, &sub.Endpoint, &sub.P256dh, &sub.Auth,
		&sub.Preferences.SummaryReady, &sub.Preferences.AcolyteReportReady,
		&sub.Preferences.RecapReady, &sub.Preferences.TodayEntranceReady,
		&sub.VAPIDKeyFingerprint, &sub.CreatedAt, &sub.UpdatedAt,
		&lastSuccessAt, &lastFailureAt,
	)
	if err != nil {
		return domain.PushSubscription{}, err
	}
	if lastSuccessAt != nil {
		sub.LastSuccessAt = *lastSuccessAt
	}
	if lastFailureAt != nil {
		sub.LastFailureAt = *lastFailureAt
	}
	return sub, nil
}
