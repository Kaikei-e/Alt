package qualitychecker

import (
	"context"

	"pre-processor/driver"
	logger "pre-processor/utils/logger"

	"github.com/jackc/pgx/v5/pgxpool"
)

// This is a compatibility alias to the driver.ArticleWithSummary type.
type ArticleWithScore = driver.ArticleWithSummary

// This function is kept for backward compatibility but will be removed in the future.
func FetchArticleAndSummaries(ctx context.Context, dbPool *pgxpool.Pool, offset int, offsetStep int) ([]ArticleWithScore, error) {
	logger.Logger.WarnContext(ctx, "FetchArticleAndSummaries is deprecated, please use driver.GetArticlesWithSummaries with cursor-based pagination")

	// For backward compatibility, we'll simulate the old behavior using the new driver function
	// This is not efficient but maintains compatibility
	articles, _, _, err := driver.GetArticlesWithSummaries(ctx, dbPool, nil, "", offsetStep)
	if err != nil {
		return nil, err
	}

	// Since we can't efficiently implement offset with cursor pagination,
	// we'll just return the first batch for now
	if len(articles) == 0 {
		logger.Logger.InfoContext(ctx, "No articles found for quality check")
		return nil, nil
	}

	return articles, nil
}
