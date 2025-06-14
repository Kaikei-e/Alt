package qualitychecker

import (
	"context"
	"pre-processor/logger"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ArticleWithScore struct {
	ArticleID       string `db:"article_id"`
	Content         string `db:"content"`
	SummaryJapanese string `db:"summary_japanese"`
	Score           int    `db:"score"`
}

func FetchArticleAndSummaries(ctx context.Context, dbPool *pgxpool.Pool, offset int, offsetStep int) ([]ArticleWithScore, error) {
	logger.Logger.Info("Fetching article and summary for quality check", "offset", offset, "limit", offsetStep)

	query := `
		SELECT
			a_s.article_id,
			a.content as content,
			a_s.summary_japanese
		FROM article_summaries a_s
		JOIN articles a ON a_s.article_id = a.id
		ORDER BY a_s.created_at ASC
		LIMIT $1 OFFSET $2
	`

	rows, err := dbPool.Query(ctx, query, offsetStep, offset)
	if err != nil {
		logger.Logger.Error("Failed to fetch article and summary", "error", err)
		return nil, err
	}
	defer rows.Close()

	articleWithScores := []ArticleWithScore{}
	for rows.Next() {
		var articleWithScore ArticleWithScore
		err := rows.Scan(&articleWithScore.ArticleID, &articleWithScore.Content, &articleWithScore.SummaryJapanese)
		if err != nil {
			logger.Logger.Error("Failed to scan article and summary", "error", err)
			return nil, err
		}
		articleWithScores = append(articleWithScores, articleWithScore)
	}

	if len(articleWithScores) == 0 {
		logger.Logger.Info("No articles found for quality check", "offset", offset)
		return nil, nil
	}

	logger.Logger.Info("Found articles for quality check", "count", len(articleWithScores), "offset", offset)
	return articleWithScores, nil
}
