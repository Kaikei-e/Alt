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
use worker::{AuthFailureMap, QueueWorker};

/// Queue for managing classification jobs with persistent storage and retry logic
pub(crate) struct ClassificationJobQueue {
    store: Arc<QueueStore>,
    #[allow(dead_code)]
    client: Arc<SubworkerClient>,
    workers: Arc<Mutex<Vec<JoinHandle<Result<()>>>>>,
    chunk_size: usize,
    max_retries: i32,
    shutdown: CancellationToken,
    auth_failures: AuthFailureMap,
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
        let auth_failures: AuthFailureMap = Arc::new(Mutex::new(std::collections::HashMap::new()));

        // Start worker tasks (one per concurrency slot)
        let mut workers = Vec::new();
        for i in 0..concurrency {
            let store_clone = store.clone();
            let client_clone = client.clone();
            let worker_shutdown = shutdown.clone();
            let worker_auth_failures = auth_failures.clone();
            let worker = QueueWorker::new(
                store_clone,
                client_clone,
                retry_delay_ms,
                worker_auth_failures,
            );
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
            auth_failures,
        }
    }

    /// Enqueue classification texts, splitting into chunks
    pub(crate) async fn enqueue_classification(
        &self,
        job_id: Uuid,
        texts: Vec<String>,
    ) -> Result<Vec<QueuedJobId>> {
        self.auth_failures
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .remove(&job_id);

        let total_texts = texts.len();

        // If texts count is small, process in a single chunk
        if total_texts <= self.chunk_size {
            let job = NewQueuedJob {
                recap_job_id: job_id,
                chunk_idx: 0,
                texts,
                max_retries: self.max_retries,
            };
            let queued_job_id = self.store.enqueue(job).await?;
            self.auth_failures
                .lock()
                .unwrap_or_else(std::sync::PoisonError::into_inner)
                .remove(&job_id);
            return Ok(vec![queued_job_id]);
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

        self.auth_failures
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .remove(&job_id);

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
                if let Some((chunk_idx, auth_err)) = self
                    .auth_failures
                    .lock()
                    .unwrap_or_else(std::sync::PoisonError::into_inner)
                    .remove(&recap_job_id)
                {
                    return Err(anyhow::Error::from(auth_err).context(format!(
                        "classify-runs request rejected for job {} chunk {}",
                        recap_job_id, chunk_idx
                    )));
                }

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
                if let Some((chunk_idx, auth_err)) = self
                    .auth_failures
                    .lock()
                    .unwrap_or_else(std::sync::PoisonError::into_inner)
                    .remove(&recap_job_id)
                {
                    return Err(anyhow::Error::from(auth_err).context(format!(
                        "classify-runs request rejected for job {} chunk {}",
                        recap_job_id, chunk_idx
                    )));
                }

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
                next_retry_at TIMESTAMPTZ,
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
        let client = SubworkerClient::new("http://localhost:8002", 10, None)?;
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

    #[tokio::test]
    async fn classify_texts_queued_fails_immediately_on_401_without_retrying() -> Result<()> {
        use wiremock::matchers::{method, path};
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return Ok(());
        };
        let schema = format!("test_queue_auth_{}", Uuid::new_v4().simple());
        let schema_for_conn = schema.clone();
        let pool = PgPoolOptions::new()
            .max_connections(5)
            .after_connect(move |conn, _meta| {
                let schema_sql = format!(
                    "CREATE SCHEMA IF NOT EXISTS {schema_for_conn}; SET search_path TO {schema_for_conn};"
                );
                Box::pin(async move {
                    conn.execute(sqlx::AssertSqlSafe(schema_sql)).await?;
                    Ok(())
                })
            })
            .connect(&database_url)
            .await?;
        setup_classification_queue_table(&pool).await?;

        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/v1/classify-runs"))
            .respond_with(
                ResponseTemplate::new(401).set_body_string(r#"{"detail":"unauthenticated peer"}"#),
            )
            .expect(1)
            .mount(&server)
            .await;

        let recap_job_id = Uuid::new_v4();
        let store = QueueStore::new(pool.clone());
        let client = SubworkerClient::new(server.uri(), 10, None)?;
        let staging_queue =
            ClassificationJobQueue::new(store.clone(), client.clone(), 0, 1, 3, 5000);
        staging_queue
            .enqueue_classification(
                recap_job_id,
                vec!["first article".to_string(), "second article".to_string()],
            )
            .await?;

        let queue = ClassificationJobQueue::new(store.clone(), client.clone(), 1, 1, 3, 5000);
        let err = queue
            .wait_for_completion(recap_job_id, Duration::from_secs(10))
            .await
            .expect_err("401 from /v1/classify-runs must fail wait_for_completion");

        queue.shutdown().await;

        let jobs = store.get_jobs_by_recap_job(recap_job_id).await?;
        let drop_sql = format!("DROP SCHEMA IF EXISTS {schema} CASCADE;");
        let _ = pool.execute(sqlx::AssertSqlSafe(drop_sql)).await;

        assert!(
            crate::clients::subworker::is_subworker_auth_error(&err),
            "expected SubworkerAuthError in cause chain, got: {err:#}"
        );
        let chain = format!("{err:#}");
        assert!(
            chain.contains("401 Unauthorized"),
            "expected '401 Unauthorized' in error chain, got: {chain}"
        );

        assert_eq!(jobs.len(), 2);
        for job in &jobs {
            assert_eq!(job.status, QueuedJobStatus::Failed);
            assert_eq!(job.retry_count, 0);
        }

        assert!(
            queue
                .auth_failures
                .lock()
                .unwrap_or_else(std::sync::PoisonError::into_inner)
                .is_empty(),
            "auth_failures map must not leak entries after wait_for_completion"
        );
        Ok(())
    }

    #[tokio::test]
    async fn late_in_flight_401_after_waiter_returns_does_not_reinsert_auth_failure() -> Result<()>
    {
        use wiremock::matchers::{body_string_contains, method, path};
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return Ok(());
        };
        let schema = format!("test_queue_auth_late_{}", Uuid::new_v4().simple());
        let schema_for_conn = schema.clone();
        let pool = PgPoolOptions::new()
            .max_connections(5)
            .after_connect(move |conn, _meta| {
                let schema_sql = format!(
                    "CREATE SCHEMA IF NOT EXISTS {schema_for_conn}; SET search_path TO {schema_for_conn};"
                );
                Box::pin(async move {
                    conn.execute(sqlx::AssertSqlSafe(schema_sql)).await?;
                    Ok(())
                })
            })
            .connect(&database_url)
            .await?;
        setup_classification_queue_table(&pool).await?;

        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/v1/classify-runs"))
            .and(body_string_contains("fast article"))
            .respond_with(
                ResponseTemplate::new(401)
                    .set_body_string(r#"{"detail":"unauthenticated peer"}"#)
                    .set_delay(Duration::from_millis(60)),
            )
            .expect(1)
            .mount(&server)
            .await;
        Mock::given(method("POST"))
            .and(path("/v1/classify-runs"))
            .and(body_string_contains("slow article"))
            .respond_with(
                ResponseTemplate::new(401)
                    .set_body_string(r#"{"detail":"unauthenticated peer"}"#)
                    .set_delay(Duration::from_millis(250)),
            )
            .expect(1)
            .mount(&server)
            .await;

        let recap_job_id = Uuid::new_v4();
        let store = QueueStore::new(pool.clone());
        let client = SubworkerClient::new(server.uri(), 10, None)?;
        let staging_queue =
            ClassificationJobQueue::new(store.clone(), client.clone(), 0, 1, 3, 5000);
        staging_queue
            .enqueue_classification(
                recap_job_id,
                vec!["fast article".to_string(), "slow article".to_string()],
            )
            .await?;

        let queue = ClassificationJobQueue::new(store.clone(), client.clone(), 2, 1, 3, 5000);
        tokio::time::sleep(Duration::from_millis(120)).await;

        let err = queue
            .wait_for_completion(recap_job_id, Duration::from_secs(5))
            .await
            .expect_err("401 from /v1/classify-runs must fail wait_for_completion");
        assert!(crate::clients::subworker::is_subworker_auth_error(&err));

        queue.shutdown().await;

        let drop_sql = format!("DROP SCHEMA IF EXISTS {schema} CASCADE;");
        let _ = pool.execute(sqlx::AssertSqlSafe(drop_sql)).await;

        assert!(
            queue
                .auth_failures
                .lock()
                .unwrap_or_else(std::sync::PoisonError::into_inner)
                .is_empty(),
            "late in-flight 401 on an already-failed chunk must not re-insert into auth_failures"
        );
        Ok(())
    }

    #[tokio::test]
    async fn wait_for_completion_timeout_returns_and_removes_auth_failure_entry() -> Result<()> {
        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return Ok(());
        };
        let schema = format!("test_queue_auth_timeout_{}", Uuid::new_v4().simple());
        let schema_for_conn = schema.clone();
        let pool = PgPoolOptions::new()
            .max_connections(5)
            .after_connect(move |conn, _meta| {
                let schema_sql = format!(
                    "CREATE SCHEMA IF NOT EXISTS {schema_for_conn}; SET search_path TO {schema_for_conn};"
                );
                Box::pin(async move {
                    conn.execute(sqlx::AssertSqlSafe(schema_sql)).await?;
                    Ok(())
                })
            })
            .connect(&database_url)
            .await?;
        setup_classification_queue_table(&pool).await?;

        let recap_job_id = Uuid::new_v4();
        let store = QueueStore::new(pool.clone());
        let client = SubworkerClient::new("http://localhost:8002", 10, None)?;
        let queue = ClassificationJobQueue::new(store, client, 0, 1, 3, 5000);
        queue
            .enqueue_classification(recap_job_id, vec!["article".to_string()])
            .await?;
        let auth_failures = queue.auth_failures.clone();
        let writer = tokio::spawn(async move {
            tokio::time::sleep(Duration::from_millis(5)).await;
            auth_failures
                .lock()
                .unwrap_or_else(std::sync::PoisonError::into_inner)
                .insert(
                    recap_job_id,
                    (
                        0,
                        crate::clients::subworker::SubworkerAuthError::new(
                            "classify-runs",
                            reqwest::StatusCode::UNAUTHORIZED,
                            "unauthorized",
                        ),
                    ),
                );
        });

        let err = queue
            .wait_for_completion(recap_job_id, Duration::from_millis(40))
            .await
            .expect_err("pending chunk must fail on timeout");
        writer.await.expect("writer task");

        let drop_sql = format!("DROP SCHEMA IF EXISTS {schema} CASCADE;");
        let _ = pool.execute(sqlx::AssertSqlSafe(drop_sql)).await;

        assert!(
            crate::clients::subworker::is_subworker_auth_error(&err),
            "timeout branch must return the recorded SubworkerAuthError when present, got: {err:#}"
        );
        assert!(
            queue
                .auth_failures
                .lock()
                .unwrap_or_else(std::sync::PoisonError::into_inner)
                .is_empty(),
            "timeout branch must drop any stale auth_failures entry for the job"
        );
        Ok(())
    }

    #[tokio::test]
    async fn stale_auth_failure_is_cleared_on_fresh_enqueue_and_completion_succeeds() -> Result<()>
    {
        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return Ok(());
        };
        let schema = format!("test_queue_auth_stale_{}", Uuid::new_v4().simple());
        let schema_for_conn = schema.clone();
        let pool = PgPoolOptions::new()
            .max_connections(5)
            .after_connect(move |conn, _meta| {
                let schema_sql = format!(
                    "CREATE SCHEMA IF NOT EXISTS {schema_for_conn}; SET search_path TO {schema_for_conn};"
                );
                Box::pin(async move {
                    conn.execute(sqlx::AssertSqlSafe(schema_sql)).await?;
                    Ok(())
                })
            })
            .connect(&database_url)
            .await?;
        setup_classification_queue_table(&pool).await?;

        let recap_job_id = Uuid::new_v4();
        let store = QueueStore::new(pool.clone());
        let client = SubworkerClient::new("http://localhost:8002", 10, None)?;
        let queue = ClassificationJobQueue::new(store.clone(), client, 0, 1, 3, 5000);

        queue
            .auth_failures
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .insert(
                recap_job_id,
                (
                    0,
                    crate::clients::subworker::SubworkerAuthError::new(
                        "classify-runs",
                        reqwest::StatusCode::UNAUTHORIZED,
                        "stale unauthorized from prior attempt",
                    ),
                ),
            );

        let chunk_ids = queue
            .enqueue_classification(recap_job_id, vec!["fresh article".to_string()])
            .await?;
        assert_eq!(chunk_ids.len(), 1);
        store
            .mark_completed(
                chunk_ids[0],
                vec![ClassificationResult {
                    top_genre: "ai".to_string(),
                    confidence: 0.95,
                    scores: HashMap::new(),
                }],
            )
            .await?;

        let results = queue
            .wait_for_completion(recap_job_id, Duration::from_secs(5))
            .await;

        let drop_sql = format!("DROP SCHEMA IF EXISTS {schema} CASCADE;");
        let _ = pool.execute(sqlx::AssertSqlSafe(drop_sql)).await;

        let results = results.expect("fresh enqueue must clear stale auth_failures and succeed");
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].top_genre, "ai");
        Ok(())
    }
}
