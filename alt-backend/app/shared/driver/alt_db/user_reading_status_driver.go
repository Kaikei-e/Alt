package alt_db

import (
	"alt/domain"
	"alt/utils"
	"alt/utils/logger"
	"context"
	"fmt"
	"net/url"

	"github.com/google/uuid"
)

// MarkArticleAsRead marks the feed an article's URL belongs to as read.
//
// The name is the caller's vocabulary and the table is feeds — the browser
// marks an article read, but not every feed has an articles row, so the read
// state has always been kept per feed.
//
// userID is an argument rather than something read from the request context.
// alt-data-hub serves this over Connect, where the peer certificate identifies
// alt-backend and not the person whose read state is being written; a
// context-scoped user would resolve to the service account for every caller.
//
// The missing-feed answer is RowsAffected() == 0, the same as UpdateFeedStatus
// since capability catalog §4-5 collapsed the two implementations onto one
// statement.
func (r *SubscriptionRepository) MarkArticleAsRead(ctx context.Context, articleURL url.URL, userID uuid.UUID) error {
	// Zero-trust: normalise before matching (strips UTM parameters, trailing
	// slashes) so two spellings of one feed do not become two read_status rows.
	originalURL := articleURL.String()
	normalizedURL, err := utils.NormalizeURL(originalURL)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error normalizing feed URL", "error", err, "feedURL", originalURL)
		return fmt.Errorf("normalize feed url: %w", err)
	}

	tag, err := r.pool.Exec(ctx, readStatusUpsertByWebsiteURLQuery, normalizedURL, userID)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error updating feed read status",
			"error", err,
			"user_id", userID,
			"normalizedURL", normalizedURL)
		return fmt.Errorf("failed to update feed read status: %w", err)
	}

	if tag.RowsAffected() == 0 {
		logger.SafeErrorContext(ctx, "Feed not found",
			"normalizedURL", normalizedURL,
			"originalURL", originalURL,
			"user_id", userID)
		return domain.ErrFeedNotFound
	}

	logger.SafeInfoContext(ctx, "feed read status updated successfully",
		"user_id", userID,
		"normalizedURL", normalizedURL,
		"is_read", true)

	return nil
}
