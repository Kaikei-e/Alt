package alt_db

import (
	"context"
	"log/slog"

	"alt/driver/models"
)

func (a *AltDBRepository) GetFeedURLsByArticleIDs(ctx context.Context, articleIDs []string) ([]models.FeedAndArticle, error) {
	if len(articleIDs) == 0 {
		slog.Info("no article IDs provided, returning empty map")
		return nil, nil
	}

	queryString := `
		SELECT f.id as feed_id, a.id as article_id, f.link as url, f.title as feed_title, a.title as article_title
		FROM articles a
		INNER JOIN feeds f ON a.url = f.link
		WHERE a.id = ANY($1)
	`

	slog.Info("querying feed URLs by article IDs",
		"article_count", len(articleIDs))

	rows, err := a.pool.Query(ctx, queryString, articleIDs)
	if err != nil {
		slog.Error("failed to query feed URLs by article IDs",
			"error", err,
			"article_count", len(articleIDs))
		return nil, err
	}
	defer rows.Close()

	feedAndArticles := []models.FeedAndArticle{}

	for rows.Next() {
		var feedAndArticle models.FeedAndArticle
		err := rows.Scan(&feedAndArticle.FeedID, &feedAndArticle.ArticleID, &feedAndArticle.URL, &feedAndArticle.FeedTitle, &feedAndArticle.ArticleTitle)
		if err != nil {
			slog.Error("failed to scan feed URL row",
				"error", err)
			return nil, err
		}
		feedAndArticles = append(feedAndArticles, feedAndArticle)
	}

	if err := rows.Err(); err != nil {
		slog.Error("error iterating feed URL rows",
			"error", err)
		return nil, err
	}

	return feedAndArticles, nil
}
