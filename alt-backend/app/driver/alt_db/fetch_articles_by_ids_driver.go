package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"

	"github.com/google/uuid"
)

// FetchArticlesByIDs retrieves articles by their IDs with tags
// Returns articles in the same order as the input IDs (where found)
func (r *AltDBRepository) FetchArticlesByIDs(ctx context.Context, articleIDs []uuid.UUID) ([]*domain.Article, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	if len(articleIDs) == 0 {
		logger.Logger.InfoContext(ctx, "No article IDs provided for fetch")
		return []*domain.Article{}, nil
	}

	logger.Logger.InfoContext(ctx, "Fetching articles by IDs", "article_count", len(articleIDs))

	query := `
		SELECT
			a.id,
			a.feed_id,
			a.title,
			a.content,
			a.url,
			a.created_at,
			COALESCE(ARRAY_AGG(ft.tag_name) FILTER (WHERE ft.tag_name IS NOT NULL), '{}') as tags
		FROM articles a
		LEFT JOIN article_tags at ON a.id = at.article_id
		LEFT JOIN feed_tags ft ON at.feed_tag_id = ft.id
		WHERE a.id = ANY($1) AND a.deleted_at IS NULL
		GROUP BY a.id
	`

	rows, err := r.pool.Query(ctx, query, articleIDs)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to query articles by IDs", "error", err, "article_count", len(articleIDs))
		return nil, errors.New("error fetching articles by IDs")
	}
	defer rows.Close()

	// Create a map for quick lookup
	articleMap := make(map[uuid.UUID]*domain.Article)
	for rows.Next() {
		var a domain.Article
		var tags []string

		err := rows.Scan(
			&a.ID, &a.FeedID, &a.Title, &a.Content, &a.URL, &a.CreatedAt, &tags,
		)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to scan article row", "error", err)
			return nil, errors.New("error scanning article")
		}

		// Set defaults for missing fields
		a.PublishedAt = a.CreatedAt
		a.UpdatedAt = a.CreatedAt
		a.Tags = tags
		// Summary, Author, Language, TenantID remain as zero values (empty string/UUID)

		articleMap[a.ID] = &a
	}

	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "Error iterating article rows", "error", err)
		return nil, errors.New("error iterating article rows")
	}

	// Return articles in the order of input IDs (preserve order)
	var articles []*domain.Article
	missingCount := 0
	for _, id := range articleIDs {
		if article, ok := articleMap[id]; ok {
			articles = append(articles, article)
		} else {
			missingCount++
			logger.Logger.WarnContext(ctx, "Article not found in database", "article_id", id)
		}
	}

	if missingCount > 0 {
		logger.Logger.WarnContext(ctx, "Some articles were not found", "missing_count", missingCount, "total_requested", len(articleIDs))
	}

	logger.Logger.InfoContext(ctx, "Successfully fetched articles by IDs", "found_count", len(articles), "requested_count", len(articleIDs))
	return articles, nil
}
