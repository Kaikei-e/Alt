package backfill

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// defaultScanPageSize bounds how many source rows are held in flight. The
	// scan pages by keyset rather than streaming one cursor for hours, so a
	// dropped connection costs one page, not the whole enqueue.
	defaultScanPageSize = 500

	articleScanColumns = `SELECT id, title, content, url, user_id, created_at FROM articles
		WHERE content IS NOT NULL AND content <> '' AND deleted_at IS NULL`
	articleScanFirstPage = articleScanColumns + ` ORDER BY created_at DESC, id DESC LIMIT $1`
	articleScanNextPage  = articleScanColumns + ` AND (created_at, id) < ($1, $2)
		ORDER BY created_at DESC, id DESC LIMIT $3`
)

// SQLArticleSource streams indexable articles from the source database.
type SQLArticleSource struct {
	db       *sql.DB
	pageSize int
}

// NewSQLArticleSource creates a source over an open source-database handle.
func NewSQLArticleSource(db *sql.DB, pageSize int) (*SQLArticleSource, error) {
	if db == nil {
		return nil, fmt.Errorf("article source: database handle is required")
	}
	if pageSize <= 0 {
		pageSize = defaultScanPageSize
	}
	return &SQLArticleSource{db: db, pageSize: pageSize}, nil
}

// ScanArticles walks every article in (created_at, id) descending order.
func (s *SQLArticleSource) ScanArticles(ctx context.Context, fn func(Article) error) error {
	var lastCreatedAt time.Time
	var lastID string
	first := true

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		var rows *sql.Rows
		var err error
		if first {
			rows, err = s.db.QueryContext(ctx, articleScanFirstPage, s.pageSize)
		} else {
			rows, err = s.db.QueryContext(ctx, articleScanNextPage, lastCreatedAt, lastID, s.pageSize)
		}
		if err != nil {
			return fmt.Errorf("query articles: %w", err)
		}

		page, err := scanArticlePage(rows, fn)
		if err != nil {
			return err
		}

		if page.count == 0 {
			return nil
		}

		lastCreatedAt, lastID = page.lastCreatedAt, page.lastID
		first = false

		if page.count < s.pageSize {
			return nil
		}
	}
}

type articlePage struct {
	count         int
	lastCreatedAt time.Time
	lastID        string
}

func scanArticlePage(rows *sql.Rows, fn func(Article) error) (articlePage, error) {
	defer func() { _ = rows.Close() }()

	var page articlePage
	for rows.Next() {
		var a Article
		if err := rows.Scan(&a.ID, &a.Title, &a.Body, &a.URL, &a.UserID, &a.CreatedAt); err != nil {
			return page, fmt.Errorf("scan article: %w", err)
		}
		page.count++
		page.lastCreatedAt, page.lastID = a.CreatedAt, a.ID

		if err := fn(a); err != nil {
			return page, err
		}
	}
	if err := rows.Err(); err != nil {
		return page, fmt.Errorf("iterate articles: %w", err)
	}
	return page, nil
}

// PgxVersionStateReader reads current-version derivation state from rag-db.
// It reuses the pool the repositories already hold rather than opening a
// second one.
type PgxVersionStateReader struct {
	pool *pgxpool.Pool
}

// NewPgxVersionStateReader creates a state reader over an open rag-db pool.
func NewPgxVersionStateReader(pool *pgxpool.Pool) (*PgxVersionStateReader, error) {
	if pool == nil {
		return nil, fmt.Errorf("version state reader: database pool is required")
	}
	return &PgxVersionStateReader{pool: pool}, nil
}

const currentStateQuery = `
	SELECT v.chunker_version, v.embedder_version
	FROM rag_documents d
	JOIN rag_document_versions v ON v.id = d.current_version_id
	WHERE d.article_id = $1
`

// CurrentState returns the derivation recorded on articleID's current version.
func (r *PgxVersionStateReader) CurrentState(ctx context.Context, articleID string) (VersionState, bool, error) {
	var state VersionState
	err := r.pool.QueryRow(ctx, currentStateQuery, articleID).
		Scan(&state.ChunkerVersion, &state.EmbedderVersion)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return VersionState{}, false, nil
		}
		return VersionState{}, false, fmt.Errorf("query current version state: %w", err)
	}
	return state, true, nil
}

const articleIDsAtTargetQuery = `
	SELECT d.article_id
	FROM rag_documents d
	JOIN rag_document_versions v ON v.id = d.current_version_id
	WHERE v.chunker_version = $1 AND v.embedder_version = $2
`

// ArticleIDsAt returns every article whose current version already matches
// target, so an enqueue can skip work a previous run finished.
func (r *PgxVersionStateReader) ArticleIDsAt(ctx context.Context, target RebuildTarget) (map[string]struct{}, error) {
	rows, err := r.pool.Query(ctx, articleIDsAtTargetQuery, target.ChunkerVersion, target.EmbedderVersion)
	if err != nil {
		return nil, fmt.Errorf("query articles at target: %w", err)
	}
	defer rows.Close()

	out := make(map[string]struct{})
	for rows.Next() {
		var articleID string
		if err := rows.Scan(&articleID); err != nil {
			return nil, fmt.Errorf("scan article id: %w", err)
		}
		out[articleID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate articles at target: %w", err)
	}
	return out, nil
}

const pendingJobCountQuery = `
	SELECT count(*) FROM rag_jobs
	WHERE job_type = $1 AND status IN ('new', 'processing')
`

// CountPendingJobs returns the queue depth a rebuild is about to drain. It is
// what turns the progress log's ETA from a guess into a number; a rebuild with
// an unknown total still runs, it just reports no ETA.
func CountPendingJobs(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	if pool == nil {
		return 0, fmt.Errorf("count pending jobs: database pool is required")
	}
	var count int64
	if err := pool.QueryRow(ctx, pendingJobCountQuery, rebuildJobType).Scan(&count); err != nil {
		return 0, fmt.Errorf("count pending jobs: %w", err)
	}
	return count, nil
}

var (
	_ ArticleSource      = (*SQLArticleSource)(nil)
	_ VersionStateReader = (*PgxVersionStateReader)(nil)
)
