package alt_db

import (
	"alt/domain"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// CreateBackfillJob inserts a new backfill job.
func (r *AltDBRepository) CreateBackfillJob(ctx context.Context, job domain.KnowledgeBackfillJob) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.CreateBackfillJob")
	defer span.End()

	query := `INSERT INTO knowledge_backfill_jobs
		(job_id, status, projection_version, cursor_user_id, cursor_date, cursor_article_id,
		 total_events, processed_events, error_message, created_at, started_at, completed_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	_, err := r.pool.Exec(ctx, query,
		job.JobID, job.Status, job.ProjectionVersion,
		job.CursorUserID, job.CursorDate, job.CursorArticleID,
		job.TotalEvents, job.ProcessedEvents, job.ErrorMessage,
		job.CreatedAt, job.StartedAt, job.CompletedAt, job.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("CreateBackfillJob: %w", err)
	}
	return nil
}

// GetBackfillJob retrieves a backfill job by ID.
func (r *AltDBRepository) GetBackfillJob(ctx context.Context, jobID uuid.UUID) (*domain.KnowledgeBackfillJob, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.GetBackfillJob")
	defer span.End()

	query := `SELECT job_id, status, projection_version, cursor_user_id, cursor_date, cursor_article_id,
		total_events, processed_events, error_message, created_at, started_at, completed_at, updated_at
		FROM knowledge_backfill_jobs WHERE job_id = $1`

	var job domain.KnowledgeBackfillJob
	err := r.pool.QueryRow(ctx, query, jobID).Scan(
		&job.JobID, &job.Status, &job.ProjectionVersion,
		&job.CursorUserID, &job.CursorDate, &job.CursorArticleID,
		&job.TotalEvents, &job.ProcessedEvents, &job.ErrorMessage,
		&job.CreatedAt, &job.StartedAt, &job.CompletedAt, &job.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetBackfillJob: %w", err)
	}
	return &job, nil
}

// UpdateBackfillJob updates an existing backfill job.
func (r *AltDBRepository) UpdateBackfillJob(ctx context.Context, job domain.KnowledgeBackfillJob) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.UpdateBackfillJob")
	defer span.End()

	query := `UPDATE knowledge_backfill_jobs SET
		status = $2, cursor_user_id = $3, cursor_date = $4, cursor_article_id = $5,
		total_events = $6, processed_events = $7, error_message = $8,
		started_at = $9, completed_at = $10, updated_at = now()
		WHERE job_id = $1`

	_, err := r.pool.Exec(ctx, query,
		job.JobID, job.Status, job.CursorUserID, job.CursorDate, job.CursorArticleID,
		job.TotalEvents, job.ProcessedEvents, job.ErrorMessage,
		job.StartedAt, job.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("UpdateBackfillJob: %w", err)
	}
	return nil
}

// ListBackfillJobs returns all backfill jobs ordered by creation time descending.
func (r *AltDBRepository) ListBackfillJobs(ctx context.Context) ([]domain.KnowledgeBackfillJob, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.ListBackfillJobs")
	defer span.End()

	query := `SELECT job_id, status, projection_version, cursor_user_id, cursor_date, cursor_article_id,
		total_events, processed_events, error_message, created_at, started_at, completed_at, updated_at
		FROM knowledge_backfill_jobs ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("ListBackfillJobs: %w", err)
	}
	defer rows.Close()

	var jobs []domain.KnowledgeBackfillJob
	for rows.Next() {
		var job domain.KnowledgeBackfillJob
		err := rows.Scan(
			&job.JobID, &job.Status, &job.ProjectionVersion,
			&job.CursorUserID, &job.CursorDate, &job.CursorArticleID,
			&job.TotalEvents, &job.ProcessedEvents, &job.ErrorMessage,
			&job.CreatedAt, &job.StartedAt, &job.CompletedAt, &job.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("ListBackfillJobs scan: %w", err)
		}
		jobs = append(jobs, job)
	}

	span.SetAttributes(attribute.Int("db.row_count", len(jobs)))
	return jobs, nil
}

// CountBackfillArticles returns the number of non-deleted articles available for replay.
func (r *AltDBRepository) CountBackfillArticles(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM articles WHERE deleted_at IS NULL`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("CountBackfillArticles: %w", err)
	}
	return count, nil
}

// ListBackfillArticles returns a batch of historical articles ordered by created_at ASC, id ASC.
func (r *AltDBRepository) ListBackfillArticles(ctx context.Context, lastCreatedAt *time.Time, lastArticleID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillArticle, error) {
	var (
		rows pgx.Rows
		err  error
	)

	if lastCreatedAt == nil || lastArticleID == nil {
		rows, err = r.pool.Query(ctx, `
			SELECT id, user_id, created_at, COALESCE(published_at, created_at) AS published_at, title
			FROM articles
			WHERE deleted_at IS NULL
			ORDER BY created_at ASC, id ASC
			LIMIT $1
		`, limit)
	} else {
		rows, err = r.pool.Query(ctx, `
			SELECT id, user_id, created_at, COALESCE(published_at, created_at) AS published_at, title
			FROM articles
			WHERE deleted_at IS NULL
			  AND (created_at, id) > ($1, $2)
			ORDER BY created_at ASC, id ASC
			LIMIT $3
		`, *lastCreatedAt, *lastArticleID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("ListBackfillArticles: %w", err)
	}
	defer rows.Close()

	articles := make([]domain.KnowledgeBackfillArticle, 0, limit)
	for rows.Next() {
		var article domain.KnowledgeBackfillArticle
		if err := rows.Scan(&article.ArticleID, &article.UserID, &article.CreatedAt, &article.PublishedAt, &article.Title); err != nil {
			return nil, fmt.Errorf("ListBackfillArticles scan: %w", err)
		}
		articles = append(articles, article)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListBackfillArticles rows: %w", err)
	}

	return articles, nil
}
