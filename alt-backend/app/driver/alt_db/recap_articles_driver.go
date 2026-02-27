package alt_db

import (
	"alt/domain"
	"alt/utils/constants"
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
    a.created_at AS published_at,
    NULL::text AS lang_hint
FROM articles a
WHERE a.created_at BETWEEN $1 AND $2 AND a.deleted_at IS NULL
ORDER BY a.created_at DESC, a.id DESC
OFFSET $3
LIMIT $4`

// FetchRecapArticles retrieves recap-ready articles with deterministic ordering.
func (r *AltDBRepository) FetchRecapArticles(ctx context.Context, query domain.RecapArticlesQuery) (*domain.RecapArticlesPage, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database connection not available")
	}
	if query.Page <= 0 || query.PageSize <= 0 {
		return nil, errors.New("page and page_size must be positive")
	}
	// Prevent DoS attack via excessive memory allocation (CWE-770)
	if query.PageSize > constants.MaxRecapPageSize {
		return nil, fmt.Errorf("page_size exceeds maximum allowed value of %d", constants.MaxRecapPageSize)
	}

	offset := (query.Page - 1) * query.PageSize

	rows, err := r.pool.Query(ctx, recapArticlesQuery, query.From, query.To, offset, query.PageSize)
	if err != nil {
		logger.SafeErrorContext(ctx, "recap articles query failed", "error", err, "from", query.From, "to", query.To)
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
			logger.SafeErrorContext(ctx, "recap articles scan failed", "error", err)
			return nil, fmt.Errorf("scan recap articles: %w", err)
		}

		totalCount = rowTotal

		article := domain.RecapArticle{
			ID:          articleID,
			Title:       nullStringPtr(title),
			FullText:    clampFullText(ctx, articleID, fulltext),
			SourceURL:   nullStringPtr(sourceURL),
			LangHint:    nullLowerTrimStringPtr(langHint),
			PublishedAt: nullTimePtr(published),
		}

		articles = append(articles, article)
	}

	if err := rows.Err(); err != nil {
		logger.SafeErrorContext(ctx, "iteration over recap articles failed", "error", err)
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

	logger.SafeInfoContext(ctx, "fetched recap articles",
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

func clampFullText(ctx context.Context, articleID uuid.UUID, body string) string {
	if len(body) <= constants.MaxRecapArticleBytes {
		return body
	}
	logger.SafeWarnContext(ctx, "recap article truncated to max bytes",
		"article_id", articleID.String(),
		"original_bytes", len(body),
		"max_bytes", constants.MaxRecapArticleBytes,
	)
	return body[:constants.MaxRecapArticleBytes]
}
