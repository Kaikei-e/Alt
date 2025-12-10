use anyhow::{Context, Result};
use std::sync::{Arc, Mutex};
use std::time::Duration;
use tokio::task::JoinHandle;
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

        // Start worker tasks (one per concurrency slot)
        let mut workers = Vec::new();
        for i in 0..concurrency {
            let store_clone = store.clone();
            let client_clone = client.clone();
            let worker = QueueWorker::new(store_clone, client_clone, 1, retry_delay_ms);
            let handle = tokio::spawn(async move {
                info!(worker_id = i, "starting queue worker");
                worker.run().await
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

    /// Shutdown all workers
    #[allow(dead_code)]
    pub(crate) async fn shutdown(&self) {
        info!("shutting down classification job queue");
        // Take ownership of workers to await them
        let workers = {
            let mut workers_guard = self.workers.lock().unwrap();
            std::mem::take(&mut *workers_guard)
        };

        // Abort all worker tasks
        for worker in &workers {
            worker.abort();
        }

        // Wait for all workers to finish
        for worker in workers {
            let _ = worker.await;
        }

        info!("all classification queue workers stopped");
    }
}
