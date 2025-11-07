package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const recapArticlesQuery = `
SELECT
    COUNT(*) OVER() AS total_count,
    a.id,
    a.title,
    COALESCE(NULLIF(a.content, ''), '') AS fulltext,
    a.url,
    COALESCE(a.published_at, a.created_at) AS published_at,
    LOWER(f.language) AS lang_hint
FROM articles a
LEFT JOIN feeds f ON f.id = a.feed_id
WHERE COALESCE(a.published_at, a.created_at) BETWEEN $1 AND $2
  AND ($3::text IS NULL OR LOWER(f.language) = $3::text)
ORDER BY COALESCE(a.published_at, a.created_at) DESC, a.id DESC
OFFSET $4
LIMIT $5`

const maxRecapArticleBytes = 2 * 1024 * 1024 // 2MB safeguard per PLAN5

// FetchRecapArticles retrieves recap-ready articles with deterministic ordering.
func (r *AltDBRepository) FetchRecapArticles(ctx context.Context, query domain.RecapArticlesQuery) (*domain.RecapArticlesPage, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database connection not available")
	}
	if query.Page <= 0 || query.PageSize <= 0 {
		return nil, errors.New("page and page_size must be positive")
	}

	offset := (query.Page - 1) * query.PageSize

	var langParam interface{}
	if query.LangHint != nil {
		langParam = *query.LangHint
	}

	rows, err := r.pool.Query(ctx, recapArticlesQuery, query.From, query.To, langParam, offset, query.PageSize)
	if err != nil {
		logger.SafeError("recap articles query failed", "error", err, "from", query.From, "to", query.To)
		return nil, fmt.Errorf("fetch recap articles: %w", err)
	}
	defer rows.Close()

	articles := make([]domain.RecapArticle, 0, query.PageSize)
	totalCount := 0

	for rows.Next() {
		var (
			rowTotal  int
			articleID uuid.UUID
			title     sql.NullString
			fulltext  string
			sourceURL sql.NullString
			published sql.NullTime
			langHint  sql.NullString
		)

		if err := rows.Scan(&rowTotal, &articleID, &title, &fulltext, &sourceURL, &published, &langHint); err != nil {
			logger.SafeError("recap articles scan failed", "error", err)
			return nil, fmt.Errorf("scan recap articles: %w", err)
		}

		totalCount = rowTotal

		article := domain.RecapArticle{
			ID:          articleID,
			Title:       nullStringPtr(title),
			FullText:    clampFullText(articleID, fulltext),
			SourceURL:   nullStringPtr(sourceURL),
			LangHint:    nullLowerTrimStringPtr(langHint),
			PublishedAt: nullTimePtr(published),
		}

		articles = append(articles, article)
	}

	if err := rows.Err(); err != nil {
		logger.SafeError("iteration over recap articles failed", "error", err)
		return nil, fmt.Errorf("iterate recap articles: %w", err)
	}

	hasMore := totalCount > 0 && offset+len(articles) < totalCount

	result := &domain.RecapArticlesPage{
		Total:    totalCount,
		Page:     query.Page,
		PageSize: query.PageSize,
		HasMore:  hasMore,
		Articles: articles,
	}

	logger.SafeInfo("fetched recap articles",
		"count", len(articles),
		"total", totalCount,
		"page", query.Page,
		"page_size", query.PageSize,
		"lang", query.LangHint,
	)

	return result, nil
}

func nullStringPtr(value sql.NullString) *string {
	if value.Valid {
		result := value.String
		return &result
	}
	return nil
}

func nullLowerTrimStringPtr(value sql.NullString) *string {
	if value.Valid {
		trimmed := strings.TrimSpace(value.String)
		if trimmed == "" {
			return nil
		}
		lowered := strings.ToLower(trimmed)
		return &lowered
	}
	return nil
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if value.Valid {
		t := value.Time
		return &t
	}
	return nil
}

func clampFullText(articleID uuid.UUID, body string) string {
	if len(body) <= maxRecapArticleBytes {
		return body
	}
	logger.SafeWarn("recap article truncated to max bytes",
		"article_id", articleID.String(),
		"original_bytes", len(body),
		"max_bytes", maxRecapArticleBytes,
	)
	return body[:maxRecapArticleBytes]
}
