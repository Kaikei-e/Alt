use anyhow::Result;
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::Semaphore;
use tokio::time::sleep;
use tracing::{debug, error, info, warn};

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

    /// Run the worker loop
    pub(crate) async fn run(&self) -> Result<()> {
        info!(
            concurrency = self.semaphore.available_permits(),
            retry_delay_ms = self.retry_delay_ms,
            "starting classification queue worker"
        );

        loop {
            // Acquire semaphore permit (limits to 8 concurrent jobs)
            let permit = self.semaphore.acquire().await;
            if permit.is_err() {
                // Semaphore was closed
                break;
            }
            let permit = permit.unwrap();

            // Pick next job from queue (SELECT FOR UPDATE SKIP LOCKED)
            let job = match self.store.pick_next_job().await {
                Ok(Some(j)) => j,
                Ok(None) => {
                    // No jobs available, wait a bit before retrying
                    drop(permit);
                    sleep(Duration::from_millis(100)).await;
                    continue;
                }
                Err(e) => {
                    error!(error = %e, "failed to pick next job");
                    drop(permit);
                    sleep(Duration::from_millis(1000)).await;
                    continue;
                }
            };

            // Process job in background (don't block the loop)
            let store = self.store.clone();
            let client = self.client.clone();
            let retry_delay_ms = self.retry_delay_ms;
            tokio::spawn(async move {
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

        // Mark as running
        if let Err(e) = store.mark_running(job_id).await {
            error!(
                job_id,
                error = %e,
                "failed to mark job as running"
            );
            return Err(e);
        }

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
                    // Mark as retrying
                    warn!(
                        job_id,
                        recap_job_id = %recap_job_id,
                        chunk_idx,
                        retry_count = job.retry_count + 1,
                        max_retries = job.max_retries,
                        error = %error_str,
                        "classification job failed, will retry"
                    );

                    if let Err(store_err) = store.mark_retrying(job_id, &error_str).await {
                        error!(
                            job_id,
                            error = %store_err,
                            "failed to mark job as retrying"
                        );
                        return Err(store_err);
                    }

                    // Wait before retry (exponential backoff)
                    let delay_ms = retry_delay_ms * (1_u64 << job.retry_count.min(3));
                    debug!(job_id, delay_ms, "waiting before retry");
                    sleep(Duration::from_millis(delay_ms)).await;
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
