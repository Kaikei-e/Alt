use anyhow::Result;
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::Semaphore;
use tokio::time::sleep;
use tokio_util::sync::CancellationToken;
use tracing::{error, info, warn};

use super::store::QueueStore;
use super::types::QueuedJob;
use crate::clients::subworker::SubworkerClient;

/// Background worker that processes queued classification jobs
pub(crate) struct QueueWorker {
    store: Arc<QueueStore>,
    client: Arc<SubworkerClient>,
    semaphore: Arc<Semaphore>,
    retry_delay_ms: u64,
}

impl QueueWorker {
    pub(crate) fn new(
        store: Arc<QueueStore>,
        client: Arc<SubworkerClient>,
        concurrency: usize,
        retry_delay_ms: u64,
    ) -> Self {
        Self {
            store,
            client,
            semaphore: Arc::new(Semaphore::new(concurrency)),
            retry_delay_ms,
        }
    }

    /// Run the worker loop until `cancel` fires. Cancellation is checked
    /// before each pick and raced against the idle/backoff sleeps, so a
    /// shutdown signal stops the loop from picking up new jobs promptly
    /// instead of relying on the caller aborting the task mid-flight.
    pub(crate) async fn run(&self, cancel: CancellationToken) -> Result<()> {
        info!(
            concurrency = self.semaphore.available_permits(),
            retry_delay_ms = self.retry_delay_ms,
            "starting classification queue worker"
        );

        loop {
            if cancel.is_cancelled() {
                info!("shutdown requested, stopping classification queue worker");
                break;
            }

            // Acquire semaphore permit (limits to 8 concurrent jobs)
            let permit = tokio::select! {
                permit = Arc::clone(&self.semaphore).acquire_owned() => permit,
                () = cancel.cancelled() => {
                    info!("shutdown requested, stopping classification queue worker");
                    break;
                }
            };
            let Ok(permit) = permit else {
                // Semaphore was closed
                break;
            };

            // Pick next job from queue (SELECT FOR UPDATE SKIP LOCKED)
            let job = match self.store.pick_next_job().await {
                Ok(Some(j)) => j,
                Ok(None) => {
                    // No jobs available, wait a bit before retrying
                    drop(permit);
                    tokio::select! {
                        () = sleep(Duration::from_millis(100)) => {}
                        () = cancel.cancelled() => {
                            info!("shutdown requested, stopping classification queue worker");
                            break;
                        }
                    }
                    continue;
                }
                Err(e) => {
                    error!(error = %e, "failed to pick next job");
                    drop(permit);
                    tokio::select! {
                        () = sleep(Duration::from_secs(1)) => {}
                        () = cancel.cancelled() => {
                            info!("shutdown requested, stopping classification queue worker");
                            break;
                        }
                    }
                    continue;
                }
            };

            // Process job in background (don't block the loop)
            let store = self.store.clone();
            let client = self.client.clone();
            let retry_delay_ms = self.retry_delay_ms;
            tokio::spawn(async move {
                // Hold the permit for the full lifetime of the spawned task.
                let _permit = permit;
                if let Err(e) = Self::process_job(store, client, job, retry_delay_ms).await {
                    error!(error = %e, "job processing failed");
                }
            });
        }

        Ok(())
    }

    /// Process a single job
    async fn process_job(
        store: Arc<QueueStore>,
        client: Arc<SubworkerClient>,
        job: QueuedJob,
        retry_delay_ms: u64,
    ) -> Result<()> {
        let job_id = job.id;
        let recap_job_id = job.recap_job_id;
        let chunk_idx = job.chunk_idx;

        info!(
            job_id,
            recap_job_id = %recap_job_id,
            chunk_idx,
            text_count = job.texts.len(),
            retry_count = job.retry_count,
            "processing queued classification job"
        );

        // `store.pick_next_job()` already transitioned this row to 'running'
        // atomically as part of the pick itself (see queue/store.rs), so no
        // separate mark-running step is needed here.

        // Send to subworker
        let result = client
            .classify_chunk(recap_job_id, job.texts.clone(), chunk_idx)
            .await;

        match result {
            Ok(results) => {
                // Convert from subworker::ClassificationResult to queue::ClassificationResult
                let queue_results: Vec<super::types::ClassificationResult> = results
                    .into_iter()
                    .map(|r| super::types::ClassificationResult {
                        top_genre: r.top_genre,
                        confidence: r.confidence,
                        scores: r.scores,
                    })
                    .collect();

                // Mark as completed
                if let Err(e) = store.mark_completed(job_id, queue_results).await {
                    error!(
                        job_id,
                        error = %e,
                        "failed to mark job as completed"
                    );
                    return Err(e);
                }

                info!(
                    job_id,
                    recap_job_id = %recap_job_id,
                    chunk_idx,
                    "classification job completed successfully"
                );
                Ok(())
            }
            Err(e) => {
                let error_str = e.to_string();
                let should_retry = job.retry_count < job.max_retries;

                if should_retry {
                    // Exponential backoff, enforced by `next_retry_at` in the
                    // store rather than a `sleep()` here: sleeping in this
                    // task would hold the semaphore permit idle for the
                    // whole backoff window while the row is already
                    // re-pickable, wasting concurrency without actually
                    // delaying the retry.
                    let delay_ms = retry_delay_ms * (1_u64 << job.retry_count.min(3));
                    warn!(
                        job_id,
                        recap_job_id = %recap_job_id,
                        chunk_idx,
                        retry_count = job.retry_count + 1,
                        max_retries = job.max_retries,
                        delay_ms,
                        error = %error_str,
                        "classification job failed, will retry"
                    );

                    if let Err(store_err) = store.mark_retrying(job_id, &error_str, delay_ms).await
                    {
                        error!(
                            job_id,
                            error = %store_err,
                            "failed to mark job as retrying"
                        );
                        return Err(store_err);
                    }
                } else {
                    // Mark as failed (max retries exceeded)
                    error!(
                        job_id,
                        recap_job_id = %recap_job_id,
                        chunk_idx,
                        retry_count = job.retry_count,
                        max_retries = job.max_retries,
                        error = %error_str,
                        "classification job failed after max retries"
                    );

                    if let Err(store_err) = store.mark_failed(job_id, &error_str).await {
                        error!(
                            job_id,
                            error = %store_err,
                            "failed to mark job as failed"
                        );
                        return Err(store_err);
                    }
                }

                Err(e)
            }
        }
    }
}
