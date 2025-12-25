package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RagJobRepository struct {
	db *pgxpool.Pool
}

func NewRagJobRepository(db *pgxpool.Pool) domain.RagJobRepository {
	return &RagJobRepository{db: db}
}

func (r *RagJobRepository) Enqueue(ctx context.Context, job *domain.RagJob) error {
	query := `
		INSERT INTO rag_jobs (id, job_type, payload, status, error_message, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	payloadBytes, err := json.Marshal(job.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	_, err = r.db.Exec(ctx, query,
		job.ID,
		job.JobType,
		payloadBytes,
		job.Status,
		job.ErrorMessage,
		job.CreatedAt,
		job.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to enqueue job: %w", err)
	}
	return nil
}

func (r *RagJobRepository) AcquireNextJob(ctx context.Context) (*domain.RagJob, error) {
	// This must be run within a transaction to hold the lock until the work is committed/updated?
	// Actually, usually "Acquire" updates the status to 'processing' immediately inside the same transaction
	// to prevent others from picking it up, then commits.
	// So this function should probably update the status to 'processing' atomically.

	// Let's modify the query to update and return.
	// CTE is useful here.

	cteQuery := `
		WITH next_job AS (
			SELECT id
			FROM rag_jobs
			WHERE status = 'new'
			ORDER BY created_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE rag_jobs
		SET status = 'processing', updated_at = $1
		FROM next_job
		WHERE rag_jobs.id = next_job.id
		RETURNING rag_jobs.id, rag_jobs.job_type, rag_jobs.payload, rag_jobs.status, rag_jobs.error_message, rag_jobs.created_at, rag_jobs.updated_at
	`

	var job domain.RagJob
	var payloadBytes []byte

	err := r.db.QueryRow(ctx, cteQuery, time.Now()).Scan(
		&job.ID,
		&job.JobType,
		&payloadBytes,
		&job.Status,
		&job.ErrorMessage,
		&job.CreatedAt,
		&job.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to acquire next job: %w", err)
	}

	if err := json.Unmarshal(payloadBytes, &job.Payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	return &job, nil
}

func (r *RagJobRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, errorMessage *string) error {
	query := `
		UPDATE rag_jobs
		SET status = $1, error_message = $2, updated_at = $3
		WHERE id = $4
	`
	_, err := r.db.Exec(ctx, query, status, errorMessage, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}
	return nil
}
