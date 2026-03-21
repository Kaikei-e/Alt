package repository

import (
	"context"
	"fmt"
)

const countStalePendingJobsQuery = `
		SELECT COUNT(*)
		FROM summarize_job_queue q
		WHERE q.status = 'pending'
		  AND EXISTS (
			SELECT 1
			FROM article_summaries s
			WHERE s.article_id = q.article_id
		  )
	`

const deleteStalePendingJobsQuery = `
		DELETE FROM summarize_job_queue
		WHERE id IN (
			SELECT candidate.id
			FROM summarize_job_queue candidate
			WHERE candidate.status = 'pending'
			  AND EXISTS (
				SELECT 1
				FROM article_summaries s
				WHERE s.article_id = candidate.article_id
			  )
			ORDER BY candidate.created_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id
	`

func (r *summarizeJobRepository) CountStalePendingJobs(ctx context.Context) (int64, error) {
	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return 0, fmt.Errorf("database connection is nil")
	}

	var count int64
	if err := r.db.QueryRow(ctx, countStalePendingJobsQuery).Scan(&count); err != nil {
		r.logger.ErrorContext(ctx, "failed to count stale pending jobs", "error", err)
		return 0, fmt.Errorf("failed to count stale pending jobs: %w", err)
	}

	return count, nil
}

func (r *summarizeJobRepository) DeleteStalePendingJobs(ctx context.Context, limit int) (int64, error) {
	if limit <= 0 {
		r.logger.ErrorContext(ctx, "limit must be positive", "limit", limit)
		return 0, fmt.Errorf("limit must be positive")
	}

	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return 0, fmt.Errorf("database connection is nil")
	}

	rows, err := r.db.Query(ctx, deleteStalePendingJobsQuery, limit)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to delete stale pending jobs", "error", err)
		return 0, fmt.Errorf("failed to delete stale pending jobs: %w", err)
	}
	defer rows.Close()

	var deleted int64
	for rows.Next() {
		deleted++
	}
	if err := rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "failed iterating deleted stale pending jobs", "error", err)
		return 0, fmt.Errorf("failed iterating deleted stale pending jobs: %w", err)
	}

	return deleted, nil
}
