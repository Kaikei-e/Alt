package alt_db

import (
	"alt/domain"
	"alt/utils"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// readStatusUpsertByWebsiteURLQuery marks the feed at a website_url as read
// for one user, creating the row if the user has not opened the feed before.
//
// One statement, shared by both read-status writes (capability catalog §4-5).
// The feed lookup is the INSERT's SELECT rather than a query of its own, which
// is what makes "no such feed" observable as RowsAffected() == 0 and removes
// the window in which a feed could be deleted between a resolve and a write —
// in that window the upsert wrote nothing and the caller was told it
// succeeded.
//
// It is a package constant rather than a string in each driver so that the two
// procedures cannot drift apart again: a change here changes both, and the
// driver tests assert against this same constant.
const readStatusUpsertByWebsiteURLQuery = `
	INSERT INTO read_status (feed_id, user_id, is_read, read_at, created_at)
	SELECT f.id, $2, TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
	FROM feeds f WHERE f.website_url = $1
	ON CONFLICT (feed_id, user_id) DO UPDATE
	SET is_read = TRUE, read_at = CURRENT_TIMESTAMP
`

// UpdateFeedStatus marks the feed at feedURL as read for one user.
//
// The transaction stays here rather than moving to the caller (ADR-000954 D3).
// It now wraps a single statement, so at the database level it commits nothing
// the statement would not have committed alone; it is kept because collapsing
// a transaction boundary is a change independent of collapsing the round
// trips, and one commit doing both would be ambiguous in a bisect.
func (r *FeedRepository) UpdateFeedStatus(ctx context.Context, feedURL url.URL, userID uuid.UUID) error {
	normalizedInputURL, err := utils.NormalizeURL(feedURL.String())
	if err != nil {
		logger.SafeErrorContext(ctx, "Error normalizing input URL", "error", err, "feedURL", feedURL.String())
		return fmt.Errorf("normalize feed url: %w", err)
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		logger.SafeErrorContext(ctx, "Error beginning transaction", "error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Unconditional rollback on a context detached from the request's: a
	// cancelled request must still release the transaction, and a Rollback
	// after a successful Commit is the no-op pgx.ErrTxClosed.
	defer func() {
		if rbErr := tx.Rollback(context.WithoutCancel(ctx)); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			logger.SafeWarnContext(ctx, "Error rolling back transaction", "error", rbErr)
		}
	}()

	tag, err := tx.Exec(ctx, readStatusUpsertByWebsiteURLQuery, normalizedInputURL, userID)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error updating feed status",
			"error", err,
			"user_id", userID,
			"normalizedURL", normalizedInputURL)
		return fmt.Errorf("failed to update feed status: %w", err)
	}

	if tag.RowsAffected() == 0 {
		logger.SafeErrorContext(ctx, "Feed not found",
			"normalizedURL", normalizedInputURL,
			"originalURL", feedURL.String(),
			"user_id", userID)
		return domain.ErrFeedNotFound
	}

	if err = tx.Commit(ctx); err != nil {
		logger.SafeErrorContext(ctx, "Error committing transaction", "error", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.SafeInfoContext(ctx, "feed status updated successfully",
		"user_id", userID,
		"normalizedURL", normalizedInputURL,
		"is_read", true)

	return nil
}
