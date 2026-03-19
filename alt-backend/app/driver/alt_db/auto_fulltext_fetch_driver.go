package alt_db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (r *AltDBRepository) ListSubscribedUserIDsByFeedLinkID(ctx context.Context, feedLinkID string) ([]string, error) {
	if r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	rows, err := r.pool.Query(ctx,
		`SELECT user_id::text FROM user_feed_subscriptions WHERE feed_link_id = $1 ORDER BY user_id`,
		feedLinkID,
	)
	if err != nil {
		return nil, fmt.Errorf("list subscribed user ids by feed link id: %w", err)
	}
	defer rows.Close()

	var userIDs []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, fmt.Errorf("scan subscribed user id: %w", err)
		}
		userIDs = append(userIDs, userID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscribed user ids: %w", err)
	}

	return userIDs, nil
}

func (r *AltDBRepository) CheckArticleExistsByURLForUser(ctx context.Context, url string, userID string) (bool, string, error) {
	if r.pool == nil {
		return false, "", errors.New("database connection not available")
	}

	var articleID string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM articles WHERE url = $1 AND user_id = $2 AND deleted_at IS NULL LIMIT 1`,
		url,
		userID,
	).Scan(&articleID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("check article exists by url for user: %w", err)
	}

	return true, articleID, nil
}
