use anyhow::{Context, Result};
use chrono::Utc;
use serde_json::{Value, json};
use sqlx::{PgPool, Row};
use uuid::Uuid;

use super::types::{ClassificationResult, NewQueuedJob, QueuedJob, QueuedJobId, QueuedJobStatus};

#[derive(Debug, Clone)]
pub(crate) struct QueueStore {
    pool: PgPool,
}

impl QueueStore {
    pub(crate) fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    /// Insert a new job into the queue
    pub(crate) async fn enqueue(&self, job: NewQueuedJob) -> Result<QueuedJobId> {
        let texts_json = json!(job.texts);

        let row = sqlx::query(
            r"
            INSERT INTO classification_job_queue
                (recap_job_id, chunk_idx, texts, max_retries, status)
            VALUES ($1, $2, $3, $4, 'pending')
            ON CONFLICT (recap_job_id, chunk_idx) DO UPDATE
            SET texts = EXCLUDED.texts,
                status = CASE
                    WHEN classification_job_queue.status = 'failed' THEN 'pending'
                    ELSE classification_job_queue.status
                END,
                retry_count = 0,
                error_message = NULL
            RETURNING id
            ",
        )
        .bind(job.recap_job_id)
        .bind(job.chunk_idx as i32)
        .bind(texts_json)
        .bind(job.max_retries)
        .fetch_one(&self.pool)
        .await
        .context("failed to insert queued job")?;

        let id: i32 = row.try_get("id").context("failed to get job id")?;
        Ok(id)
    }

    /// Pick the next pending or retrying job (SELECT FOR UPDATE SKIP LOCKED)
    /// Returns None if no job is available
    pub(crate) async fn pick_next_job(&self) -> Result<Option<QueuedJob>> {
        let row = sqlx::query(
            r"
            SELECT id, recap_job_id, chunk_idx, status, texts, result,
                   error_message, retry_count, max_retries,
                   created_at, started_at, completed_at
            FROM classification_job_queue
            WHERE status IN ('pending', 'retrying')
            ORDER BY created_at ASC
            FOR UPDATE SKIP LOCKED
            LIMIT 1
            ",
        )
        .fetch_optional(&self.pool)
        .await
        .context("failed to pick next job")?;

        let Some(row) = row else {
            return Ok(None);
        };

        let job = Self::row_to_job(row)?;
        Ok(Some(job))
    }

    /// Mark a job as running
    pub(crate) async fn mark_running(&self, job_id: QueuedJobId) -> Result<()> {
        sqlx::query(
            r"
            UPDATE classification_job_queue
            SET status = 'running',
                started_at = COALESCE(started_at, NOW())
            WHERE id = $1
            ",
        )
        .bind(job_id)
        .execute(&self.pool)
        .await
        .context("failed to mark job as running")?;

        Ok(())
    }

    /// Mark a job as completed with results
    pub(crate) async fn mark_completed(
        &self,
        job_id: QueuedJobId,
        results: Vec<ClassificationResult>,
    ) -> Result<()> {
        let results_json = json!(
            results
                .iter()
                .map(|r| json!({
                    "top_genre": r.top_genre,
                    "confidence": r.confidence,
                    "scores": r.scores,
                }))
                .collect::<Vec<_>>()
        );

        sqlx::query(
            r"
            UPDATE classification_job_queue
            SET status = 'completed',
                result = $2,
                completed_at = NOW()
            WHERE id = $1
            ",
        )
        .bind(job_id)
        .bind(results_json)
        .execute(&self.pool)
        .await
        .context("failed to mark job as completed")?;

        Ok(())
    }

    /// Mark a job as failed
    pub(crate) async fn mark_failed(&self, job_id: QueuedJobId, error: &str) -> Result<()> {
        sqlx::query(
            r"
            UPDATE classification_job_queue
            SET status = 'failed',
                error_message = $2,
                completed_at = NOW()
            WHERE id = $1
            ",
        )
        .bind(job_id)
        .bind(error)
        .execute(&self.pool)
        .await
        .context("failed to mark job as failed")?;

        Ok(())
    }

    /// Mark a job as retrying (increment retry_count)
    pub(crate) async fn mark_retrying(&self, job_id: QueuedJobId, error: &str) -> Result<()> {
        sqlx::query(
            r"
            UPDATE classification_job_queue
            SET status = 'retrying',
                error_message = $2,
                retry_count = retry_count + 1,
                started_at = NULL
            WHERE id = $1
            ",
        )
        .bind(job_id)
        .bind(error)
        .execute(&self.pool)
        .await
        .context("failed to mark job as retrying")?;

        Ok(())
    }

    /// Get a job by ID
    #[allow(dead_code)]
    pub(crate) async fn get_job(&self, job_id: QueuedJobId) -> Result<Option<QueuedJob>> {
        let row = sqlx::query(
            r"
            SELECT id, recap_job_id, chunk_idx, status, texts, result,
                   error_message, retry_count, max_retries,
                   created_at, started_at, completed_at
            FROM classification_job_queue
            WHERE id = $1
            ",
        )
        .bind(job_id)
        .fetch_optional(&self.pool)
        .await
        .context("failed to get job")?;

        let Some(row) = row else {
            return Ok(None);
        };

        let job = Self::row_to_job(row)?;
        Ok(Some(job))
    }

    /// Get all jobs for a recap job ID
    pub(crate) async fn get_jobs_by_recap_job(&self, recap_job_id: Uuid) -> Result<Vec<QueuedJob>> {
        let rows = sqlx::query(
            r"
            SELECT id, recap_job_id, chunk_idx, status, texts, result,
                   error_message, retry_count, max_retries,
                   created_at, started_at, completed_at
            FROM classification_job_queue
            WHERE recap_job_id = $1
            ORDER BY chunk_idx ASC
            ",
        )
        .bind(recap_job_id)
        .fetch_all(&self.pool)
        .await
        .context("failed to get jobs by recap job id")?;

        let mut jobs = Vec::new();
        for row in rows {
            jobs.push(Self::row_to_job(row)?);
        }
        Ok(jobs)
    }

    /// Check if all jobs for a recap job are completed
    pub(crate) async fn all_jobs_completed(&self, recap_job_id: Uuid) -> Result<bool> {
        let row = sqlx::query(
            r"
            SELECT COUNT(*) FILTER (WHERE status NOT IN ('completed', 'failed')) as pending_count
            FROM classification_job_queue
            WHERE recap_job_id = $1
            ",
        )
        .bind(recap_job_id)
        .fetch_one(&self.pool)
        .await
        .context("failed to check job completion")?;

        let pending_count: i64 = row.try_get("pending_count").unwrap_or(0);
        Ok(pending_count == 0)
    }

    /// Get all completed results for a recap job
    pub(crate) async fn get_completed_results(
        &self,
        recap_job_id: Uuid,
    ) -> Result<Vec<ClassificationResult>> {
        let rows = sqlx::query(
            r"
            SELECT result
            FROM classification_job_queue
            WHERE recap_job_id = $1
              AND status = 'completed'
              AND result IS NOT NULL
            ORDER BY chunk_idx ASC
            ",
        )
        .bind(recap_job_id)
        .fetch_all(&self.pool)
        .await
        .context("failed to get completed results")?;

        let mut all_results = Vec::new();
        for row in rows {
            let result_json: Value = row.try_get("result").context("failed to get result")?;
            let results: Vec<ClassificationResult> =
                serde_json::from_value(result_json).context("failed to deserialize results")?;
            all_results.extend(results);
        }
        Ok(all_results)
    }

    /// Convert a database row to QueuedJob
    fn row_to_job(row: sqlx::postgres::PgRow) -> Result<QueuedJob> {
        let id: i32 = row.try_get("id").context("failed to get id")?;
        let recap_job_id: Uuid = row
            .try_get("recap_job_id")
            .context("failed to get recap_job_id")?;
        let chunk_idx: i32 = row
            .try_get("chunk_idx")
            .context("failed to get chunk_idx")?;
        let status_str: String = row.try_get("status").context("failed to get status")?;
        let texts_json: Value = row.try_get("texts").context("failed to get texts")?;
        let result_json: Option<Value> = row.try_get("result").ok();
        let error_message: Option<String> = row.try_get("error_message").ok();
        let retry_count: i32 = row.try_get("retry_count").unwrap_or(0);
        let max_retries: i32 = row.try_get("max_retries").unwrap_or(3);
        let created_at: chrono::DateTime<Utc> = row
            .try_get("created_at")
            .context("failed to get created_at")?;
        let started_at: Option<chrono::DateTime<Utc>> = row.try_get("started_at").ok();
        let completed_at: Option<chrono::DateTime<Utc>> = row.try_get("completed_at").ok();

        let status = QueuedJobStatus::from_str(&status_str)
            .context(format!("invalid status: {}", status_str))?;

        let texts: Vec<String> =
            serde_json::from_value(texts_json).context("failed to deserialize texts")?;

        let result = if let Some(result_json) = result_json {
            Some(serde_json::from_value(result_json).context("failed to deserialize result")?)
        } else {
            None
        };

        Ok(QueuedJob {
            id,
            recap_job_id,
            chunk_idx: usize::try_from(chunk_idx.max(0)).unwrap_or(0),
            status,
            texts,
            result,
            error_message,
            retry_count,
            max_retries,
            created_at,
            started_at,
            completed_at,
        })
    }
}
