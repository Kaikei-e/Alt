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

    /// Atomically pick the next pending/retrying job and mark it as running.
    ///
    /// This is a single-statement CAS: the row selection (`FOR UPDATE SKIP
    /// LOCKED`) and the `status = 'running'` transition happen inside the
    /// same statement/transaction, so the row lock is held until the status
    /// change is durable. A plain `SELECT ... FOR UPDATE SKIP LOCKED`
    /// followed by a separate `UPDATE` would auto-commit (and release the
    /// lock) as soon as the SELECT completes, letting a concurrent caller
    /// (another worker, or this worker's own next loop iteration) observe
    /// the same row as still 'pending' and pick it again.
    /// Returns None if no job is available.
    pub(crate) async fn pick_next_job(&self) -> Result<Option<QueuedJob>> {
        let row = sqlx::query(
            r"
            UPDATE classification_job_queue
            SET status = 'running',
                started_at = COALESCE(started_at, NOW())
            WHERE id = (
                SELECT id
                FROM classification_job_queue
                WHERE status IN ('pending', 'retrying')
                ORDER BY created_at ASC
                FOR UPDATE SKIP LOCKED
                LIMIT 1
            )
            RETURNING id, recap_job_id, chunk_idx, status, texts, result,
                      error_message, retry_count, max_retries,
                      created_at, started_at, completed_at
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

#[cfg(test)]
mod tests {
    //! Integration tests requiring a DATABASE_URL environment variable.
    //! They no-op (return Ok) when it is unset, matching the convention in
    //! `src/store/dao/tests.rs`.

    use super::*;
    use sqlx::Executor;
    use sqlx::postgres::PgPoolOptions;
    use std::collections::HashSet;
    use std::sync::Arc;

    async fn setup_classification_queue_table(pool: &PgPool) -> Result<()> {
        pool.execute(
            r"
            CREATE TABLE IF NOT EXISTS classification_job_queue (
                id SERIAL PRIMARY KEY,
                recap_job_id UUID NOT NULL,
                chunk_idx INT NOT NULL,
                status VARCHAR(20) NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'running', 'completed', 'failed', 'retrying')),
                texts JSONB NOT NULL,
                result JSONB,
                error_message TEXT,
                retry_count INT DEFAULT 0,
                max_retries INT DEFAULT 3,
                created_at TIMESTAMPTZ DEFAULT NOW(),
                started_at TIMESTAMPTZ,
                completed_at TIMESTAMPTZ,
                UNIQUE(recap_job_id, chunk_idx)
            );
            ",
        )
        .await?;
        // `pick_next_job` is a global queue scan (by design: any worker may
        // claim any pending job, not just ones from its own recap_job_id),
        // so these tests need a table with no unrelated pending/running rows
        // left over from a previous run.
        pool.execute("TRUNCATE TABLE classification_job_queue;")
            .await?;
        Ok(())
    }

    async fn cleanup(pool: &PgPool, recap_job_id: Uuid) {
        let _ = sqlx::query("DELETE FROM classification_job_queue WHERE recap_job_id = $1")
            .bind(recap_job_id)
            .execute(pool)
            .await;
    }

    /// Reproduces the double-pick race directly: `pick_next_job` must be a
    /// single atomic CAS, so firing many more concurrent pickers than there
    /// are pending rows must yield each row to exactly one caller. The old
    /// implementation (a bare `SELECT ... FOR UPDATE SKIP LOCKED` that never
    /// itself changed `status`) would hand the same row to every concurrent
    /// caller, since nothing ever left the 'pending' set.
    #[tokio::test]
    async fn pick_next_job_never_double_picks_under_concurrency() -> Result<()> {
        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return Ok(());
        };
        let pool = PgPoolOptions::new()
            .max_connections(10)
            .connect(&database_url)
            .await?;
        setup_classification_queue_table(&pool).await?;

        let recap_job_id = Uuid::new_v4();
        let store = Arc::new(QueueStore::new(pool.clone()));

        const JOB_COUNT: usize = 15;
        for i in 0..JOB_COUNT {
            store
                .enqueue(NewQueuedJob {
                    recap_job_id,
                    chunk_idx: i,
                    texts: vec!["text".to_string()],
                    max_retries: 3,
                })
                .await?;
        }

        let mut handles = Vec::new();
        for _ in 0..(JOB_COUNT * 4) {
            let store = store.clone();
            handles.push(tokio::spawn(async move { store.pick_next_job().await }));
        }

        let mut picked_ids = Vec::new();
        for handle in handles {
            if let Some(job) = handle.await?? {
                picked_ids.push(job.id);
            }
        }

        let unique: HashSet<_> = picked_ids.iter().collect();
        assert_eq!(
            unique.len(),
            picked_ids.len(),
            "pick_next_job double-picked at least one job: {picked_ids:?}"
        );
        assert_eq!(
            picked_ids.len(),
            JOB_COUNT,
            "expected exactly one picker to win each of the {JOB_COUNT} pending jobs"
        );

        cleanup(&pool, recap_job_id).await;
        Ok(())
    }

    /// Verifies the CAS shape itself: picking a job must durably transition
    /// it to 'running' in the same statement, so a second pick sees nothing
    /// left to take.
    #[tokio::test]
    async fn pick_next_job_marks_row_running_and_is_not_repickable() -> Result<()> {
        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return Ok(());
        };
        let pool = PgPoolOptions::new()
            .max_connections(2)
            .connect(&database_url)
            .await?;
        setup_classification_queue_table(&pool).await?;

        let recap_job_id = Uuid::new_v4();
        let store = QueueStore::new(pool.clone());
        store
            .enqueue(NewQueuedJob {
                recap_job_id,
                chunk_idx: 0,
                texts: vec!["hello".to_string()],
                max_retries: 3,
            })
            .await?;

        let picked = store
            .pick_next_job()
            .await?
            .expect("the only pending job should be picked");
        assert_eq!(picked.status, QueuedJobStatus::Running);

        let row = sqlx::query("SELECT status FROM classification_job_queue WHERE id = $1")
            .bind(picked.id)
            .fetch_one(&pool)
            .await?;
        let status: String = row.try_get("status")?;
        assert_eq!(status, "running");

        let second = store.pick_next_job().await?;
        assert!(
            second.is_none(),
            "job was already claimed; nothing should be left to pick"
        );

        cleanup(&pool, recap_job_id).await;
        Ok(())
    }
}
