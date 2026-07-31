package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// userIDFromContext is the bridge between the in-process callers, which carry
// the signed-in user in the request context, and the *ForUser driver methods
// alt-data-hub calls with the owner as an argument.
//
// It returns nil rather than an error for an absent user: the reads that use
// it have an anonymous variant, and collapsing "no user" into a failure would
// break the unauthenticated article lookup that has always worked.
func userIDFromContext(ctx context.Context) *uuid.UUID {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil
	}
	id := user.UserID
	return &id
}

const fetchArticleByURLQuery = `
	SELECT id, title, content, url, COALESCE(feed_id::text, '') AS feed_id
	FROM articles
	WHERE url = $1 AND deleted_at IS NULL
`

const fetchArticleByURLWithUserQuery = `
	SELECT id, title, content, url, COALESCE(feed_id::text, '') AS feed_id
	FROM articles
	WHERE url = $1 AND user_id = $2 AND deleted_at IS NULL
`

// FetchArticleByURL retrieves article content from database by URL.
// Scopes to the authenticated user when user context is available.
func (r *ArticleRepository) FetchArticleByURL(ctx context.Context, articleURL string) (*domain.ArticleContent, error) {
	return r.FetchArticleByURLForUser(ctx, articleURL, userIDFromContext(ctx))
}

// FetchArticleByURLForUser is the same read with the owner named explicitly.
//
// A nil userID keeps the unscoped query the anonymous path already used; the
// two are different SQL statements and the caller picks one, rather than the
// scope depending on whether a context value happened to be present
// (ADR-000954 Wave 3, catalog §2.C).
func (r *ArticleRepository) FetchArticleByURLForUser(ctx context.Context, articleURL string, userID *uuid.UUID) (*domain.ArticleContent, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	cleanURL := strings.TrimSpace(articleURL)
	if cleanURL == "" {
		return nil, errors.New("article url cannot be empty")
	}

	var article domain.ArticleContent
	var err error

	if userID != nil {
		err = r.pool.QueryRow(ctx, fetchArticleByURLWithUserQuery, cleanURL, *userID).Scan(
			&article.ID,
			&article.Title,
			&article.Content,
			&article.URL,
			&article.FeedID,
		)
	} else {
		err = r.pool.QueryRow(ctx, fetchArticleByURLQuery, cleanURL).Scan(
			&article.ID,
			&article.Title,
			&article.Content,
			&article.URL,
			&article.FeedID,
		)
	}

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.SafeInfoContext(ctx, "article not found in database", "url", cleanURL)
			return nil, nil // Return nil without error to indicate not found
		}
		err = fmt.Errorf("fetch article by url: %w", err)
		logger.SafeErrorContext(ctx, "failed to fetch article", "url", cleanURL, "error", err)
		return nil, err
	}

	logger.SafeInfoContext(ctx, "article retrieved from database", "url", cleanURL, "article_id", article.ID)
	return &article, nil
}

const fetchArticlesByURLsQuery = `
	SELECT id, title, content, url, COALESCE(feed_id::text, '') AS feed_id
	FROM articles
	WHERE url = ANY($1) AND deleted_at IS NULL
`

const fetchArticlesByURLsWithUserQuery = `
	SELECT id, title, content, url, COALESCE(feed_id::text, '') AS feed_id
	FROM articles
	WHERE url = ANY($1) AND user_id = $2 AND deleted_at IS NULL
`

// FetchArticlesByURLs retrieves article content for a batch of URLs in a
// single query (replacing the N+1 pattern of calling FetchArticleByURL once
// per URL). Missing URLs are simply absent from the returned map. Scopes to
// the authenticated user when user context is available, mirroring
// FetchArticleByURL.
func (r *ArticleRepository) FetchArticlesByURLs(ctx context.Context, urls []string) (map[string]*domain.ArticleContent, error) {
	return r.FetchArticlesByURLsForUser(ctx, urls, userIDFromContext(ctx))
}

// FetchArticlesByURLsForUser is the batch read with the owner named
// explicitly. See FetchArticleByURLForUser for why the scope is an argument.
func (r *ArticleRepository) FetchArticlesByURLsForUser(ctx context.Context, urls []string, userID *uuid.UUID) (map[string]*domain.ArticleContent, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database connection not available")
	}
	if len(urls) == 0 {
		return map[string]*domain.ArticleContent{}, nil
	}

	var rows pgx.Rows
	var err error
	if userID != nil {
		rows, err = r.pool.Query(ctx, fetchArticlesByURLsWithUserQuery, urls, *userID)
	} else {
		rows, err = r.pool.Query(ctx, fetchArticlesByURLsQuery, urls)
	}
	if err != nil {
		return nil, fmt.Errorf("fetch articles by urls: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*domain.ArticleContent, len(urls))
	for rows.Next() {
		var article domain.ArticleContent
		if err := rows.Scan(&article.ID, &article.Title, &article.Content, &article.URL, &article.FeedID); err != nil {
			return nil, fmt.Errorf("scan article row: %w", err)
		}
		result[article.URL] = &article
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate article rows: %w", err)
	}

	return result, nil
}

const fetchArticleByIDQuery = `
	SELECT id, title, content, url, COALESCE(feed_id::text, '') AS feed_id
	FROM articles
	WHERE id = $1 AND deleted_at IS NULL
`

// FetchArticleByID retrieves article content from database by ID
func (r *ArticleRepository) FetchArticleByID(ctx context.Context, articleID string) (*domain.ArticleContent, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	cleanID := strings.TrimSpace(articleID)
	if cleanID == "" {
		return nil, errors.New("article id cannot be empty")
	}

	var article domain.ArticleContent
	err := r.pool.QueryRow(ctx, fetchArticleByIDQuery, cleanID).Scan(
		&article.ID,
		&article.Title,
		&article.Content,
		&article.URL,
		&article.FeedID,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.SafeInfoContext(ctx, "article not found in database", "id", cleanID)
			return nil, nil // Return nil without error to indicate not found
		}
		err = fmt.Errorf("fetch article by id: %w", err)
		logger.SafeErrorContext(ctx, "failed to fetch article", "id", cleanID, "error", err)
		return nil, err
	}

	logger.SafeInfoContext(ctx, "article retrieved from database", "id", cleanID)
	return &article, nil
}
