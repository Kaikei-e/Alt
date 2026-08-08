package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"pre-processor/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dbExecutor is the subset of *pgxpool.Pool this repository uses. Scoping it
// down to an interface lets tests exercise the SQL this repository sends
// (predicates, argument binding) against pgxmock instead of only reaching
// the nil-db early return — a raw *pgxpool.Pool field cannot be swapped for
// a test double.
type dbExecutor interface {
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
}

// summarizeJobRepository implementation.
type summarizeJobRepository struct {
	db     dbExecutor
	logger *slog.Logger
	// clock supplies the completion instant written to both summarize_job_queue
	// and the notification_outbox row derived from it. It is a field rather
	// than a direct wall-clock read so a test can pin the two to one value and
	// prove the outbox carries the job's completion time, not a later read.
	clock func() time.Time
}

const getPendingJobsQuery = `
		SELECT id, job_id, article_id, status, summary, error_message,
		       retry_count, max_retries, created_at, started_at, completed_at
		FROM summarize_job_queue
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT $1
	`

// NewSummarizeJobRepository creates a new summarize job repository.
//
// db is taken as the concrete *pgxpool.Pool (not dbExecutor) and only
// assigned into the interface-typed field when non-nil: assigning a nil
// *pgxpool.Pool directly into an interface field produces a non-nil
// interface wrapping a nil pointer, which would make every `r.db == nil`
// guard below false and defeat the nil-database error path.
func NewSummarizeJobRepository(db *pgxpool.Pool, logger *slog.Logger) SummarizeJobRepository {
	repo := &summarizeJobRepository{logger: logger, clock: time.Now}
	if db != nil {
		repo.db = db
	}
	return repo
}

// CreateJob creates a new summarization job in the queue.
func (r *summarizeJobRepository) CreateJob(ctx context.Context, articleID string) (string, error) {
	if articleID == "" {
		r.logger.ErrorContext(ctx, "article ID cannot be empty")
		return "", fmt.Errorf("article ID cannot be empty")
	}

	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return "", fmt.Errorf("database connection is nil")
	}

	r.logger.InfoContext(ctx, "creating summarization job", "article_id", articleID)

	query := `
		INSERT INTO summarize_job_queue (article_id, status)
		SELECT $1, 'pending'
		WHERE NOT EXISTS (
			SELECT 1 FROM summarize_job_queue
			WHERE article_id = $1
			  AND status IN ('pending', 'running')
		)
		RETURNING job_id
	`

	// WHERE NOT EXISTS alone doesn't prevent duplicate jobs under concurrent
	// execution: two transactions can both evaluate the subquery as empty
	// before either commits its INSERT. Serialize CreateJob calls for the
	// same article_id with a transaction-scoped advisory lock — no schema
	// change (partial unique index) required.
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to begin transaction", "error", err)
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1)::bigint)", articleID); err != nil {
		r.logger.ErrorContext(ctx, "failed to acquire job creation lock", "error", err, "article_id", articleID)
		return "", fmt.Errorf("failed to acquire job creation lock: %w", err)
	}

	var jobID uuid.UUID
	err = tx.QueryRow(ctx, query, articleID).Scan(&jobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.InfoContext(ctx, "skipping duplicate job, pending/running job already exists", "article_id", articleID)
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return "", fmt.Errorf("failed to commit transaction: %w", commitErr)
			}
			return "", nil
		}
		r.logger.ErrorContext(ctx, "failed to create summarization job", "error", err, "article_id", articleID)
		return "", fmt.Errorf("failed to create summarization job: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		r.logger.ErrorContext(ctx, "failed to commit transaction", "error", err)
		return "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	r.logger.InfoContext(ctx, "summarization job created successfully", "job_id", jobID, "article_id", articleID)
	return jobID.String(), nil
}

// HasRecentSuccessfulJob reports whether the article has a recently completed job
// with a non-empty summary payload.
func (r *summarizeJobRepository) HasRecentSuccessfulJob(ctx context.Context, articleID string, since time.Time) (bool, error) {
	if articleID == "" {
		r.logger.ErrorContext(ctx, "article ID cannot be empty")
		return false, fmt.Errorf("article ID cannot be empty")
	}

	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return false, fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT EXISTS (
			SELECT 1
			FROM summarize_job_queue
			WHERE article_id = $1
			  AND status = 'completed'
			  AND completed_at >= $2
			  AND summary IS NOT NULL
			  AND summary <> ''
		)
	`

	var exists bool
	if err := r.db.QueryRow(ctx, query, articleID, since).Scan(&exists); err != nil {
		r.logger.ErrorContext(ctx, "failed to check recent successful job", "error", err, "article_id", articleID)
		return false, fmt.Errorf("failed to check recent successful job: %w", err)
	}

	return exists, nil
}

// HasInFlightJob reports whether the article has a pending/running job that is
// fresher than the given cutoff. Stale rows (started_at or created_at older
// than the cutoff) are ignored so a crashed worker cannot block re-enqueue
// forever; RecoverStuckJobs will reset them independently.
func (r *summarizeJobRepository) HasInFlightJob(ctx context.Context, articleID string, since time.Time) (bool, error) {
	if articleID == "" {
		r.logger.ErrorContext(ctx, "article ID cannot be empty")
		return false, fmt.Errorf("article ID cannot be empty")
	}

	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return false, fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT EXISTS (
			SELECT 1
			FROM summarize_job_queue
			WHERE article_id = $1
			  AND status IN ('pending', 'running')
			  AND COALESCE(started_at, created_at) >= $2
		)
	`

	var exists bool
	if err := r.db.QueryRow(ctx, query, articleID, since).Scan(&exists); err != nil {
		r.logger.ErrorContext(ctx, "failed to check in-flight job", "error", err, "article_id", articleID)
		return false, fmt.Errorf("check in-flight job: %w", err)
	}

	return exists, nil
}

// HasDeadLetterJob reports whether the article's most recent job row is in
// the terminal dead_letter status. dead_letter is written only for the
// explicit domain.ErrContentNotProcessable classification (see
// UpdateJobStatus's SummarizeJobStatusDeadLetter case) — content the model
// has declared it can never summarize — so this has no time cutoff.
//
// The check looks at the LATEST row for the article, not "any row ever":
// article_id is not unique, and a stale dead_letter row from an earlier
// attempt must not out-vote a later row (e.g. a subsequent completed job,
// or one invalidated by the quality-checker's compensating transaction —
// see InvalidateCompletedJobSummary) that shows the article is no longer
// permanently stuck.
func (r *summarizeJobRepository) HasDeadLetterJob(ctx context.Context, articleID string) (bool, error) {
	if articleID == "" {
		r.logger.ErrorContext(ctx, "article ID cannot be empty")
		return false, fmt.Errorf("article ID cannot be empty")
	}

	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return false, fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT status
		FROM summarize_job_queue
		WHERE article_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`

	var status string
	err := r.db.QueryRow(ctx, query, articleID).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		r.logger.ErrorContext(ctx, "failed to check dead-lettered job", "error", err, "article_id", articleID)
		return false, fmt.Errorf("check dead-lettered job: %w", err)
	}

	return status == string(domain.SummarizeJobStatusDeadLetter), nil
}

// HasRecentFailedJob reports whether the article has a job that exhausted
// its retries (status = 'failed') within the cooldown window. UpdateJobStatus
// writes 'failed' for retry exhaustion on ANY error — including transient
// infrastructure failures (DB errors, network timeouts, upstream 5xxs) — so
// unlike HasDeadLetterJob this block is time-bounded, not permanent: once
// the window elapses, a fresh attempt is allowed in case the underlying
// failure has since cleared.
func (r *summarizeJobRepository) HasRecentFailedJob(ctx context.Context, articleID string, since time.Time) (bool, error) {
	if articleID == "" {
		r.logger.ErrorContext(ctx, "article ID cannot be empty")
		return false, fmt.Errorf("article ID cannot be empty")
	}

	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return false, fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT EXISTS (
			SELECT 1
			FROM summarize_job_queue
			WHERE article_id = $1
			  AND status = 'failed'
			  AND completed_at >= $2
		)
	`

	var exists bool
	if err := r.db.QueryRow(ctx, query, articleID, since).Scan(&exists); err != nil {
		r.logger.ErrorContext(ctx, "failed to check recently failed job", "error", err, "article_id", articleID)
		return false, fmt.Errorf("check recently failed job: %w", err)
	}

	return exists, nil
}

// GetJob retrieves a summarization job by job ID.
func (r *summarizeJobRepository) GetJob(ctx context.Context, jobID string) (*domain.SummarizeJob, error) {
	if jobID == "" {
		r.logger.ErrorContext(ctx, "job ID cannot be empty")
		return nil, fmt.Errorf("job ID cannot be empty")
	}

	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return nil, fmt.Errorf("database connection is nil")
	}

	startTime := time.Now()
	r.logger.DebugContext(ctx, "getting summarization job", "job_id", jobID)

	// Read with Read Committed isolation level to ensure we see latest committed data
	// This helps prevent stale reads when checking job status immediately after updates
	query := `
		SELECT id, job_id, article_id, status, summary, error_message,
		       retry_count, max_retries, created_at, started_at, completed_at
		FROM summarize_job_queue
		WHERE job_id = $1
	`

	var job domain.SummarizeJob
	var jobIDUUID uuid.UUID
	var summary sql.NullString
	var errorMessage sql.NullString
	err := r.db.QueryRow(ctx, query, jobID).Scan(
		&job.ID,
		&jobIDUUID,
		&job.ArticleID,
		&job.Status,
		&summary,
		&errorMessage,
		&job.RetryCount,
		&job.MaxRetries,
		&job.CreatedAt,
		&job.StartedAt,
		&job.CompletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.WarnContext(ctx, "summarization job not found", "job_id", jobID)
			return nil, fmt.Errorf("summarization job not found: %w", err)
		}
		r.logger.ErrorContext(ctx, "failed to get summarization job", "error", err, "job_id", jobID)
		return nil, fmt.Errorf("failed to get summarization job: %w", err)
	}

	job.JobID = jobIDUUID
	if summary.Valid {
		job.Summary = &summary.String
	}
	if errorMessage.Valid {
		job.ErrorMessage = &errorMessage.String
	}

	// Log timing information for debugging latency issues
	queryDuration := time.Since(startTime)
	r.logger.DebugContext(ctx, "summarization job retrieved successfully",
		"job_id", jobID,
		"status", job.Status,
		"query_duration_ms", queryDuration.Milliseconds(),
		"completed_at", job.CompletedAt)
	return &job, nil
}

// UpdateJobStatus updates the status of a summarization job.
func (r *summarizeJobRepository) UpdateJobStatus(ctx context.Context, jobID string, status domain.SummarizeJobStatus, summary string, errorMessage string) error {
	if jobID == "" {
		r.logger.ErrorContext(ctx, "job ID cannot be empty")
		return fmt.Errorf("job ID cannot be empty")
	}

	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return fmt.Errorf("database connection is nil")
	}

	r.logger.InfoContext(ctx, "updating summarization job status", "job_id", jobID, "status", status)

	now := time.Now()
	var query string
	var args []interface{}

	switch status {
	case domain.SummarizeJobStatusRunning:
		query = `
			UPDATE summarize_job_queue
			SET status = $1, started_at = $2
			WHERE job_id = $3
		`
		args = []interface{}{string(status), now, jobID}
	case domain.SummarizeJobStatusCompleted:
		query = completeJobQuery
		args = []interface{}{summary, now, errorMessage, jobID}
	case domain.SummarizeJobStatusFailed:
		// When a job fails:
		// - If retry_count + 1 >= max_retries: move to failed (retry-exhausted
		//   terminal state — HasRecentFailedJob applies a bounded cooldown, not
		//   a permanent block, because this branch fires for any error
		//   including transient ones). dead_letter is reserved for the
		//   explicit domain.ErrContentNotProcessable classification, set
		//   directly via the SummarizeJobStatusDeadLetter case below.
		// - Otherwise: set status to pending (will be retried)
		query = `
			UPDATE summarize_job_queue
			SET
				status = CASE
					WHEN retry_count + 1 >= max_retries THEN 'failed'
					ELSE 'pending'
				END,
				error_message = $1,
				completed_at = CASE
					WHEN retry_count + 1 >= max_retries THEN $2
					ELSE completed_at
				END,
				retry_count = retry_count + 1
			WHERE job_id = $3
		`
		args = []interface{}{errorMessage, now, jobID}
	case domain.SummarizeJobStatusDeadLetter:
		query = `
			UPDATE summarize_job_queue
			SET status = $1, error_message = $2, completed_at = $3
			WHERE job_id = $4
		`
		args = []interface{}{string(status), errorMessage, now, jobID}
	default:
		query = `
			UPDATE summarize_job_queue
			SET status = $1
			WHERE job_id = $2
		`
		args = []interface{}{string(status), jobID}
	}

	result, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to update summarization job status", "error", err, "job_id", jobID)
		return fmt.Errorf("failed to update summarization job status: %w", err)
	}

	if result.RowsAffected() == 0 {
		r.logger.WarnContext(ctx, "no rows affected when updating job status", "job_id", jobID)
		return fmt.Errorf("summarization job not found")
	}

	r.logger.InfoContext(ctx, "summarization job status updated successfully",
		"job_id", jobID,
		"status", status,
		"timestamp", now.UnixNano(),
		"committed_at", time.Now().UnixNano())
	return nil
}

// completeJobQuery is the single definition of what completing a job means,
// shared by UpdateJobStatus and by CompleteJobWithNotification so the outbox
// row can never be attached to a different notion of "completed".
const completeJobQuery = `
		UPDATE summarize_job_queue
		SET status = 'completed', summary = $1, completed_at = $2, error_message = $3
		WHERE job_id = $4
	`

// enqueueNotificationQuery inserts the outbox row that a relay later forwards
// to alt-data-hub.
//
// ON CONFLICT DO NOTHING because dedupe_key is derived from the job id: a
// second completion of the same job — a redelivered event, a retried call —
// must be a no-op on the outbox rather than an error that rolls the whole
// completion back.
const enqueueNotificationQuery = `
		INSERT INTO notification_outbox (dedupe_key, user_id, kind, payload, occurred_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (dedupe_key) DO NOTHING
	`

// summaryReadyDedupeKey derives the outbox key from the business fact. Every
// retry of the same completion produces the same key, which is what lets the
// at-least-once relay and the downstream fan-out collapse duplicates.
func summaryReadyDedupeKey(jobID string) string {
	return "summary:" + jobID
}

// summaryReadyPayload builds the notification body: a discriminator and where
// to go. No title, no summary text, nothing about the article's content — the
// notification is a trigger, and the device fetches what it needs behind the
// user's own session.
func summaryReadyPayload(articleID string) ([]byte, error) {
	payload := struct {
		Kind string `json:"kind"`
		URL  string `json:"url"`
	}{
		Kind: string(domain.NotificationKindSummaryReady),
		URL:  "/articles/" + articleID,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode summary_ready payload: %w", err)
	}
	return encoded, nil
}

// CompleteJobWithNotification transitions a job to completed and enqueues its
// summary_ready notification in the same local transaction.
//
// That single commit is the entire reason notification_outbox lives in
// pre-processor-db: the notification is enqueued if and only if the completion
// commits. Splitting the two — completing here and calling alt-data-hub
// separately — would be a dual write, and every failure of the second half
// would silently lose a user-visible notification.
func (r *summarizeJobRepository) CompleteJobWithNotification(ctx context.Context, jobID, summary, userID, articleID string) error {
	if jobID == "" {
		return fmt.Errorf("job ID cannot be empty")
	}
	if userID == "" {
		return fmt.Errorf("user ID cannot be empty: a notification with no recipient cannot be enqueued")
	}
	if articleID == "" {
		return fmt.Errorf("article ID cannot be empty: the notification has no navigate target")
	}
	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return fmt.Errorf("database connection is nil")
	}

	payload, err := summaryReadyPayload(articleID)
	if err != nil {
		return err
	}

	completedAt := r.clock()

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to begin completion transaction", "error", err, "job_id", jobID)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	result, err := tx.Exec(ctx, completeJobQuery, summary, completedAt, "", jobID)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to complete summarization job", "error", err, "job_id", jobID)
		return fmt.Errorf("failed to complete summarization job: %w", err)
	}
	if result.RowsAffected() == 0 {
		r.logger.WarnContext(ctx, "no rows affected when completing job", "job_id", jobID)
		return fmt.Errorf("summarization job not found")
	}

	// occurred_at is completedAt, not a second clock read: the notification
	// describes when the summary became ready, which is the instant recorded
	// on the job row one statement above.
	if _, err := tx.Exec(ctx, enqueueNotificationQuery,
		summaryReadyDedupeKey(jobID),
		userID,
		string(domain.NotificationKindSummaryReady),
		payload,
		completedAt,
	); err != nil {
		r.logger.ErrorContext(ctx, "failed to enqueue summary_ready notification", "error", err, "job_id", jobID)
		return fmt.Errorf("failed to enqueue summary_ready notification: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		r.logger.ErrorContext(ctx, "failed to commit job completion", "error", err, "job_id", jobID)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	r.logger.InfoContext(ctx, "summarization job completed and notification enqueued",
		"job_id", jobID,
		"article_id", articleID,
		"dedupe_key", summaryReadyDedupeKey(jobID))
	return nil
}

// GetPendingJobs retrieves pending jobs from the queue.
func (r *summarizeJobRepository) GetPendingJobs(ctx context.Context, limit int) ([]*domain.SummarizeJob, error) {
	if limit <= 0 {
		r.logger.ErrorContext(ctx, "limit must be positive", "limit", limit)
		return nil, fmt.Errorf("limit must be positive")
	}

	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return nil, fmt.Errorf("database connection is nil")
	}

	r.logger.DebugContext(ctx, "getting pending summarization jobs", "limit", limit)

	rows, err := r.db.Query(ctx, getPendingJobsQuery, limit)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to get pending jobs", "error", err)
		return nil, fmt.Errorf("failed to get pending jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]*domain.SummarizeJob, 0, limit)
	for rows.Next() {
		var job domain.SummarizeJob
		var jobIDUUID uuid.UUID
		var summary sql.NullString
		var errorMessage sql.NullString
		err := rows.Scan(
			&job.ID,
			&jobIDUUID,
			&job.ArticleID,
			&job.Status,
			&summary,
			&errorMessage,
			&job.RetryCount,
			&job.MaxRetries,
			&job.CreatedAt,
			&job.StartedAt,
			&job.CompletedAt,
		)
		if err != nil {
			r.logger.ErrorContext(ctx, "failed to scan job row", "error", err)
			return nil, fmt.Errorf("failed to scan job row: %w", err)
		}
		job.JobID = jobIDUUID
		if summary.Valid {
			job.Summary = &summary.String
		}
		if errorMessage.Valid {
			job.ErrorMessage = &errorMessage.String
		}
		jobs = append(jobs, &job)
	}

	if err := rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "error iterating job rows", "error", err)
		return nil, fmt.Errorf("error iterating job rows: %w", err)
	}

	if len(jobs) == 0 {
		r.logger.DebugContext(ctx, "retrieved pending summarization jobs", "count", 0)
	} else {
		r.logger.InfoContext(ctx, "retrieved pending summarization jobs", "count", len(jobs))
	}
	return jobs, nil
}

// DequeueJobs atomically selects pending jobs and transitions them to running
// in a single transaction, preventing duplicate processing under concurrency.
// Uses FOR UPDATE SKIP LOCKED to avoid contention between concurrent workers.
func (r *summarizeJobRepository) DequeueJobs(ctx context.Context, limit int) ([]*domain.SummarizeJob, error) {
	if limit <= 0 {
		r.logger.ErrorContext(ctx, "limit must be positive", "limit", limit)
		return nil, fmt.Errorf("limit must be positive")
	}

	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return nil, fmt.Errorf("database connection is nil")
	}

	r.logger.DebugContext(ctx, "dequeuing summarization jobs", "limit", limit)

	now := time.Now()

	// Atomic dequeue: select pending jobs and set them to running in one statement.
	// FOR UPDATE SKIP LOCKED ensures concurrent workers don't pick the same rows.
	query := `
		UPDATE summarize_job_queue
		SET status = 'running', started_at = $1
		WHERE id IN (
			SELECT id FROM summarize_job_queue
			WHERE status = 'pending'
			ORDER BY created_at ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, job_id, article_id, status, summary, error_message,
		          retry_count, max_retries, created_at, started_at, completed_at
	`

	rows, err := r.db.Query(ctx, query, now, limit)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to dequeue jobs", "error", err)
		return nil, fmt.Errorf("failed to dequeue jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]*domain.SummarizeJob, 0, limit)
	for rows.Next() {
		var job domain.SummarizeJob
		var jobIDUUID uuid.UUID
		var summary sql.NullString
		var errorMessage sql.NullString
		err := rows.Scan(
			&job.ID,
			&jobIDUUID,
			&job.ArticleID,
			&job.Status,
			&summary,
			&errorMessage,
			&job.RetryCount,
			&job.MaxRetries,
			&job.CreatedAt,
			&job.StartedAt,
			&job.CompletedAt,
		)
		if err != nil {
			r.logger.ErrorContext(ctx, "failed to scan dequeued job row", "error", err)
			return nil, fmt.Errorf("failed to scan dequeued job row: %w", err)
		}
		job.JobID = jobIDUUID
		if summary.Valid {
			job.Summary = &summary.String
		}
		if errorMessage.Valid {
			job.ErrorMessage = &errorMessage.String
		}
		jobs = append(jobs, &job)
	}

	if err := rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "error iterating dequeued job rows", "error", err)
		return nil, fmt.Errorf("error iterating dequeued job rows: %w", err)
	}

	if len(jobs) == 0 {
		r.logger.DebugContext(ctx, "no pending jobs to dequeue")
	} else {
		r.logger.InfoContext(ctx, "dequeued summarization jobs", "count", len(jobs))
	}
	return jobs, nil
}

// RecoverStuckJobs resets jobs stuck in 'running' status for more than 10 minutes back to 'pending'.
func (r *summarizeJobRepository) RecoverStuckJobs(ctx context.Context) (int64, error) {
	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return 0, fmt.Errorf("database connection is nil")
	}

	query := `
		UPDATE summarize_job_queue
		SET status = 'pending', started_at = NULL
		WHERE status = 'running'
		  AND started_at < NOW() - interval '10 minutes'
	`

	result, err := r.db.Exec(ctx, query)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to recover stuck jobs", "error", err)
		return 0, fmt.Errorf("failed to recover stuck jobs: %w", err)
	}

	recovered := result.RowsAffected()
	if recovered > 0 {
		r.logger.WarnContext(ctx, "recovered stuck running jobs", "count", recovered)
	}
	return recovered, nil
}

// InvalidateCompletedJobSummary NULLs the summary column on completed jobs for
// the given article. This is a compensating transaction: when quality-check
// deletes a summary from article_summaries, this ensures HasRecentSuccessfulJob
// no longer blocks re-enqueue.
func (r *summarizeJobRepository) InvalidateCompletedJobSummary(ctx context.Context, articleID string) error {
	if articleID == "" {
		r.logger.ErrorContext(ctx, "article ID cannot be empty")
		return fmt.Errorf("article ID cannot be empty")
	}

	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return fmt.Errorf("database connection is nil")
	}

	query := `
		UPDATE summarize_job_queue
		SET summary = NULL
		WHERE article_id = $1
		  AND status = 'completed'
		  AND summary IS NOT NULL
	`

	result, err := r.db.Exec(ctx, query, articleID)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to invalidate completed job summary", "error", err, "article_id", articleID)
		return fmt.Errorf("failed to invalidate completed job summary: %w", err)
	}

	if affected := result.RowsAffected(); affected > 0 {
		r.logger.InfoContext(ctx, "invalidated completed job summary for re-enqueue",
			"article_id", articleID, "affected_rows", affected)
	}

	return nil
}
