// Package repository: articles_language_repo.go implements the DB driver for
// the language backfill job. It exposes two operations — cursor-paginated
// fetch of articles whose language is still "und" and an idempotent bulk
// update. Both queries defend against concurrent writers with
// `language = 'und'` predicates so that new ingestion paths cannot be
// overwritten by a trailing backfill run.
package repository

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ArticleForDetect is a minimal projection of the articles table consumed by
// the language detector. Only the fields the detector needs are selected so
// that the backfill keeps memory pressure bounded on a 48k-row scan.
type ArticleForDetect struct {
	ID      string
	Title   string
	Content string
}

// LanguageUpdate is a single (id, detected language) pair.
type LanguageUpdate struct {
	ID       string
	Language string
}

// ArticlesLanguageRepo is the interface the backfill service depends on.
type ArticlesLanguageRepo interface {
	FetchUndArticles(ctx context.Context, afterID string, limit int) ([]ArticleForDetect, error)
	UpdateLanguageBulk(ctx context.Context, updates []LanguageUpdate) (int, error)
}

type articlesLanguageRepo struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

// NewArticlesLanguageRepo constructs a pgx-backed implementation.
func NewArticlesLanguageRepo(db *pgxpool.Pool, logger *slog.Logger) ArticlesLanguageRepo {
	if logger == nil {
		logger = slog.Default()
	}
	return &articlesLanguageRepo{db: db, logger: logger}
}

const fetchUndQueryAll = `
	SELECT id::text, title, content
	FROM articles
	WHERE language = 'und'
	  AND deleted_at IS NULL
	ORDER BY id
	LIMIT $1
`

const fetchUndQueryAfter = `
	SELECT id::text, title, content
	FROM articles
	WHERE language = 'und'
	  AND deleted_at IS NULL
	  AND id > $1::uuid
	ORDER BY id
	LIMIT $2
`

// FetchUndArticles returns the next batch of still-undetermined articles,
// cursor-paginated by id for deterministic resume semantics. An empty
// afterID skips the cursor predicate so the first batch starts at the
// lowest id.
func (r *articlesLanguageRepo) FetchUndArticles(ctx context.Context, afterID string, limit int) ([]ArticleForDetect, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database pool is nil")
	}
	if limit <= 0 {
		return nil, nil
	}

	var rows pgx.Rows
	var err error
	if afterID == "" {
		rows, err = r.db.Query(ctx, fetchUndQueryAll, limit)
	} else {
		rows, err = r.db.Query(ctx, fetchUndQueryAfter, afterID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("fetch und articles: %w", err)
	}
	defer rows.Close()

	out := make([]ArticleForDetect, 0, limit)
	for rows.Next() {
		var a ArticleForDetect
		if err := rows.Scan(&a.ID, &a.Title, &a.Content); err != nil {
			return nil, fmt.Errorf("scan und article: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate und articles: %w", err)
	}
	return out, nil
}

// UpdateLanguageBulk applies a per-row mapping via a single UPDATE ... CASE
// statement. The `language = 'und'` predicate in the WHERE clause guarantees
// the update cannot clobber confirmed rows written by a concurrent ingestion
// path.
func (r *articlesLanguageRepo) UpdateLanguageBulk(ctx context.Context, updates []LanguageUpdate) (int, error) {
	if len(updates) == 0 {
		return 0, nil
	}
	if r.db == nil {
		return 0, fmt.Errorf("database pool is nil")
	}

	args := make([]any, 0, len(updates)*2)
	var caseBuilder strings.Builder
	var inBuilder strings.Builder
	caseBuilder.WriteString("CASE id")
	for i, u := range updates {
		idPh := fmt.Sprintf("$%d::uuid", i*2+1)
		langPh := fmt.Sprintf("$%d", i*2+2)
		fmt.Fprintf(&caseBuilder, " WHEN %s THEN %s", idPh, langPh)
		if i > 0 {
			inBuilder.WriteString(", ")
		}
		inBuilder.WriteString(idPh)
		args = append(args, u.ID, u.Language)
	}
	caseBuilder.WriteString(" END")

	query := fmt.Sprintf(
		"UPDATE articles SET language = %s WHERE id IN (%s) AND language = 'und'",
		caseBuilder.String(),
		inBuilder.String(),
	)

	tag, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("update language bulk: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
