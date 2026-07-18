use anyhow::{Context, Result};
use std::sync::{Arc, Mutex};
use std::time::Duration;
use tokio::task::JoinHandle;
use tokio_util::sync::CancellationToken;
use tracing::{debug, info};
use uuid::Uuid;

use crate::clients::subworker::SubworkerClient;

mod store;
mod types;
mod worker;

pub(crate) use store::QueueStore;
pub(crate) use types::{ClassificationResult, NewQueuedJob, QueuedJobId, QueuedJobStatus};
use worker::QueueWorker;

/// Queue for managing classification jobs with persistent storage and retry logic
pub(crate) struct ClassificationJobQueue {
    store: Arc<QueueStore>,
    #[allow(dead_code)]
    client: Arc<SubworkerClient>,
    workers: Arc<Mutex<Vec<JoinHandle<Result<()>>>>>,
    chunk_size: usize,
    max_retries: i32,
    shutdown: CancellationToken,
}

impl ClassificationJobQueue {
    /// Create a new classification job queue
    pub(crate) fn new(
        store: QueueStore,
        client: SubworkerClient,
        concurrency: usize,
        chunk_size: usize,
        max_retries: i32,
        retry_delay_ms: u64,
    ) -> Self {
        let store = Arc::new(store);
        let client = Arc::new(client);
        let shutdown = CancellationToken::new();

        // Start worker tasks (one per concurrency slot)
        let mut workers = Vec::new();
        for i in 0..concurrency {
            let store_clone = store.clone();
            let client_clone = client.clone();
            let worker_shutdown = shutdown.clone();
            let worker = QueueWorker::new(store_clone, client_clone, retry_delay_ms);
            let handle = tokio::spawn(async move {
                info!(worker_id = i, "starting queue worker");
                worker.run(worker_shutdown).await
            });
            workers.push(handle);
        }

        info!(
            concurrency,
            chunk_size, max_retries, retry_delay_ms, "classification job queue initialized"
        );

        Self {
            store,
            client,
            workers: Arc::new(Mutex::new(workers)),
            chunk_size,
            max_retries,
            shutdown,
        }
    }

    /// Enqueue classification texts, splitting into chunks
    pub(crate) async fn enqueue_classification(
        &self,
        job_id: Uuid,
        texts: Vec<String>,
    ) -> Result<Vec<QueuedJobId>> {
        let total_texts = texts.len();

        // If texts count is small, process in a single chunk
        if total_texts <= self.chunk_size {
            let job = NewQueuedJob {
                recap_job_id: job_id,
                chunk_idx: 0,
                texts,
                max_retries: self.max_retries,
            };
            let job_id = self.store.enqueue(job).await?;
            return Ok(vec![job_id]);
        }

        // Split into chunks
        let chunks: Vec<(usize, Vec<String>)> = texts
            .chunks(self.chunk_size)
            .enumerate()
            .map(|(idx, chunk)| (idx, chunk.to_vec()))
            .collect();

        info!(
            job_id = %job_id,
            total_texts,
            chunk_count = chunks.len(),
            chunk_size = self.chunk_size,
            "enqueueing classification chunks"
        );

        // Enqueue all chunks
        let mut job_ids = Vec::new();
        for (chunk_idx, chunk_texts) in chunks {
            let job = NewQueuedJob {
                recap_job_id: job_id,
                chunk_idx,
                texts: chunk_texts,
                max_retries: self.max_retries,
            };
            let queued_job_id = self.store.enqueue(job).await?;
            job_ids.push(queued_job_id);
        }

        debug!(
            job_id = %job_id,
            queued_job_count = job_ids.len(),
            "all chunks enqueued"
        );

        Ok(job_ids)
    }

    /// Wait for all jobs to complete
    pub(crate) async fn wait_for_completion(
        &self,
        recap_job_id: Uuid,
        timeout: Duration,
    ) -> Result<Vec<ClassificationResult>> {
        let start = std::time::Instant::now();
        let poll_interval = Duration::from_millis(500);

        loop {
            // Check if all jobs are completed
            let all_completed = self
                .store
                .all_jobs_completed(recap_job_id)
                .await
                .context("failed to check job completion")?;

            if all_completed {
                // `all_jobs_completed` treats 'failed' as "no longer pending",
                // so completion alone does not mean every chunk succeeded.
                // Check for failed chunks explicitly and fail closed instead
                // of silently returning a result set that is missing their
                // articles.
                let jobs = self
                    .store
                    .get_jobs_by_recap_job(recap_job_id)
                    .await
                    .context("failed to get jobs for completion check")?;

                let failed_chunks: Vec<usize> = jobs
                    .iter()
                    .filter(|j| j.status == QueuedJobStatus::Failed)
                    .map(|j| j.chunk_idx)
                    .collect();

                if !failed_chunks.is_empty() {
                    return Err(anyhow::anyhow!(
                        "classification job {} completed with {} failed chunk(s) out of {}: chunk_idx {:?}; articles in these chunks are missing from the result set",
                        recap_job_id,
                        failed_chunks.len(),
                        jobs.len(),
                        failed_chunks
                    ));
                }

                // Get all results
                let results = self
                    .store
                    .get_completed_results(recap_job_id)
                    .await
                    .context("failed to get completed results")?;

                info!(
                    job_id = %recap_job_id,
                    result_count = results.len(),
                    elapsed_seconds = start.elapsed().as_secs(),
                    "all classification jobs completed"
                );

                return Ok(results);
            }

            // Check timeout
            if start.elapsed() >= timeout {
                // Get jobs to check for failures
                let jobs = self
                    .store
                    .get_jobs_by_recap_job(recap_job_id)
                    .await
                    .context("failed to get jobs")?;

                let failed_count = jobs
                    .iter()
                    .filter(|j| j.status == QueuedJobStatus::Failed)
                    .count();
                let pending_count = jobs
                    .iter()
                    .filter(|j| {
                        j.status == QueuedJobStatus::Pending || j.status == QueuedJobStatus::Running
                    })
                    .count();

                if failed_count > 0 {
                    return Err(anyhow::anyhow!(
                        "classification jobs timed out: {} failed, {} still pending",
                        failed_count,
                        pending_count
                    ));
                }

                return Err(anyhow::anyhow!(
                    "classification jobs timed out: {} still pending",
                    pending_count
                ));
            }

            // Wait before next poll
            tokio::time::sleep(poll_interval).await;
        }
    }

    /// Get job status
    #[allow(dead_code)]
    pub(crate) async fn get_job_status(
        &self,
        job_id: QueuedJobId,
    ) -> Result<Option<QueuedJobStatus>> {
        let job = self.store.get_job(job_id).await?;
        Ok(job.map(|j| j.status))
    }

    /// Shutdown all workers. Cancels the shared `CancellationToken` so each
    /// worker stops picking up new jobs at its next loop checkpoint and lets
    /// any job already in flight finish (rather than aborting the task
    /// mid-write, which would leave the row stuck in 'running').
    pub(crate) async fn shutdown(&self) {
        info!("shutting down classification job queue");
        self.shutdown.cancel();

        // Take ownership of workers to await them
        let workers = {
            let mut workers_guard = self.workers.lock().unwrap();
            std::mem::take(&mut *workers_guard)
        };

        // Wait for all workers to finish
        for worker in workers {
            let _ = worker.await;
        }

        info!("all classification queue workers stopped");
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
    use std::collections::HashMap;

    async fn setup_classification_queue_table(pool: &sqlx::PgPool) -> Result<()> {
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
        Ok(())
    }

    /// A failed chunk must surface as an error from `wait_for_completion`,
    /// not be silently dropped from the result set. Before this fix,
    /// `all_jobs_completed` (which treats 'failed' as "no longer pending")
    /// made the loop return `Ok` with whatever completed-chunk results
    /// existed, quietly losing the failed chunk's articles.
    #[tokio::test]
    async fn wait_for_completion_errors_when_a_chunk_failed() -> Result<()> {
        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return Ok(());
        };
        let pool = PgPoolOptions::new()
            .max_connections(5)
            .connect(&database_url)
            .await?;
        setup_classification_queue_table(&pool).await?;

        let recap_job_id = Uuid::new_v4();
        let store = QueueStore::new(pool.clone());

        let id0 = store
            .enqueue(NewQueuedJob {
                recap_job_id,
                chunk_idx: 0,
                texts: vec!["a".to_string()],
                max_retries: 3,
            })
            .await?;
        store
            .mark_completed(
                id0,
                vec![ClassificationResult {
                    top_genre: "tech".to_string(),
                    confidence: 0.9,
                    scores: HashMap::new(),
                }],
            )
            .await?;

        let id1 = store
            .enqueue(NewQueuedJob {
                recap_job_id,
                chunk_idx: 1,
                texts: vec!["b".to_string()],
                max_retries: 3,
            })
            .await?;
        store.mark_failed(id1, "subworker unavailable").await?;

        let id2 = store
            .enqueue(NewQueuedJob {
                recap_job_id,
                chunk_idx: 2,
                texts: vec!["c".to_string()],
                max_retries: 3,
            })
            .await?;
        store
            .mark_completed(
                id2,
                vec![ClassificationResult {
                    top_genre: "science".to_string(),
                    confidence: 0.8,
                    scores: HashMap::new(),
                }],
            )
            .await?;

        // concurrency=0: no background workers, so nothing else mutates
        // these rows while we assert on wait_for_completion.
        let client = SubworkerClient::new("http://localhost:8002", 10)?;
        let queue = ClassificationJobQueue::new(store, client, 0, 200, 3, 5000);

        let err = queue
            .wait_for_completion(recap_job_id, Duration::from_secs(5))
            .await
            .expect_err("a failed chunk must surface as an error, not a silently-shrunk result");

        let message = err.to_string();
        assert!(
            message.contains("failed"),
            "error should mention the failed chunk(s): {message}"
        );
        assert!(
            message.contains('1'),
            "error should reference the failed chunk_idx (1): {message}"
        );

        let _ = sqlx::query("DELETE FROM classification_job_queue WHERE recap_job_id = $1")
            .bind(recap_job_id)
            .execute(&pool)
            .await;
        Ok(())
    }
}
