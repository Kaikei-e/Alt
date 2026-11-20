use anyhow::Result;
use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use std::time::Duration;
use tokio::time::sleep;
use tokio_util::sync::CancellationToken;
use tracing::{error, info, warn};
use uuid::Uuid;

use super::store::QueueStore;
use super::types::{QueuedJob, QueuedJobId, QueuedJobStatus};
use crate::clients::subworker::{
    ClassificationResult as SubworkerClassificationResult, SubworkerAuthError, SubworkerClient,
};

pub(crate) type AuthFailureMap = Arc<Mutex<HashMap<Uuid, (usize, SubworkerAuthError)>>>;

const DEFAULT_POLL_INTERVAL: Duration = Duration::from_millis(100);
const MAX_POLL_INTERVAL: Duration = Duration::from_secs(2);

enum PollOutcome {
    Job(QueuedJob),
    Continue,
    Break,
}

enum JobErrorKind {
    Auth(SubworkerAuthError),
    Retryable,
    Exhausted,
}

fn next_idle_backoff(current: Duration) -> Duration {
    current.saturating_mul(2).min(MAX_POLL_INTERVAL)
}

/// Background worker that processes queued classification jobs sequentially.
///
/// Concurrency is controlled by spawning one worker task per slot in
/// `ClassificationJobQueue::new`; a per-worker semaphore is unnecessary.
pub(crate) struct QueueWorker {
    store: Arc<QueueStore>,
    client: Arc<SubworkerClient>,
    retry_delay_ms: u64,
    auth_failures: AuthFailureMap,
}

impl QueueWorker {
    pub(crate) fn new(
        store: Arc<QueueStore>,
        client: Arc<SubworkerClient>,
        retry_delay_ms: u64,
        auth_failures: AuthFailureMap,
    ) -> Self {
        Self {
            store,
            client,
            retry_delay_ms,
            auth_failures,
        }
    }

    /// Run the worker loop until `cancel` fires. Cancellation is checked
    /// before each pick and raced against the idle/backoff sleeps, so a
    /// shutdown signal stops the loop from picking up new jobs promptly
    /// instead of relying on the caller aborting the task mid-flight.
    pub(crate) async fn run(&self, cancel: CancellationToken) -> Result<()> {
        info!(
            retry_delay_ms = self.retry_delay_ms,
            "starting classification queue worker"
        );

        let mut poll_interval = DEFAULT_POLL_INTERVAL;

        loop {
            if cancel.is_cancelled() {
                info!("shutdown requested, stopping classification queue worker");
                break;
            }

            let job = match self.poll_next_job(&mut poll_interval, &cancel).await {
                PollOutcome::Job(job) => job,
                PollOutcome::Continue => continue,
                PollOutcome::Break => break,
            };

            self.run_single_job(job).await;
        }

        Ok(())
    }

    async fn run_single_job(&self, job: QueuedJob) {
        // Process one job at a time in this worker slot.
        if let Err(e) = Self::process_job(
            self.store.clone(),
            self.client.clone(),
            job,
            self.retry_delay_ms,
            self.auth_failures.clone(),
        )
        .await
        {
            error!(error = %e, "job processing failed");
        }
    }

    async fn poll_next_job(
        &self,
        poll_interval: &mut Duration,
        cancel: &CancellationToken,
    ) -> PollOutcome {
        // Pick next job from queue (SELECT FOR UPDATE SKIP LOCKED)
        match self.store.pick_next_job().await {
            Ok(Some(j)) => {
                *poll_interval = DEFAULT_POLL_INTERVAL;
                PollOutcome::Job(j)
            }
            Ok(None) => {
                // No jobs available, wait with exponential backoff before retrying
                let sleep_duration = *poll_interval;
                *poll_interval = next_idle_backoff(*poll_interval);
                Self::sleep_or_cancel(sleep_duration, cancel).await
            }
            Err(e) => {
                error!(error = %e, "failed to pick next job");
                Self::sleep_or_cancel(Duration::from_secs(1), cancel).await
            }
        }
    }

    async fn sleep_or_cancel(delay: Duration, cancel: &CancellationToken) -> PollOutcome {
        tokio::select! {
            () = sleep(delay) => PollOutcome::Continue,
            () = cancel.cancelled() => {
                info!("shutdown requested, stopping classification queue worker");
                PollOutcome::Break
            }
        }
    }

    /// Process a single job
    async fn process_job(
        store: Arc<QueueStore>,
        client: Arc<SubworkerClient>,
        job: QueuedJob,
        retry_delay_ms: u64,
        auth_failures: AuthFailureMap,
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
                Self::complete_job(&store, job_id, recap_job_id, chunk_idx, results).await
            }
            Err(e) => Self::handle_job_error(&store, &job, retry_delay_ms, &auth_failures, e).await,
        }
    }

    async fn complete_job(
        store: &QueueStore,
        job_id: QueuedJobId,
        recap_job_id: Uuid,
        chunk_idx: usize,
        results: Vec<SubworkerClassificationResult>,
    ) -> Result<()> {
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
            return Err(e.into());
        }

        info!(
            job_id,
            recap_job_id = %recap_job_id,
            chunk_idx,
            "classification job completed successfully"
        );
        Ok(())
    }

    async fn handle_job_error(
        store: &QueueStore,
        job: &QueuedJob,
        retry_delay_ms: u64,
        auth_failures: &AuthFailureMap,
        e: anyhow::Error,
    ) -> Result<()> {
        let error_str = format!("{e:#}");
        let kind = if let Some(auth_err) = e.downcast_ref::<SubworkerAuthError>().cloned() {
            JobErrorKind::Auth(auth_err)
        } else if job.retry_count < job.max_retries {
            JobErrorKind::Retryable
        } else {
            JobErrorKind::Exhausted
        };

        match kind {
            JobErrorKind::Auth(auth_err) => {
                Self::handle_auth_failure(store, job, auth_failures, auth_err, &error_str).await?;
            }
            JobErrorKind::Retryable => {
                Self::schedule_retry(store, job, retry_delay_ms, &error_str).await?;
            }
            JobErrorKind::Exhausted => {
                Self::mark_exhausted_failed(store, job, &error_str).await?;
            }
        }

        Err(e)
    }

    async fn handle_auth_failure(
        store: &QueueStore,
        job: &QueuedJob,
        auth_failures: &AuthFailureMap,
        auth_err: SubworkerAuthError,
        error_str: &str,
    ) -> Result<()> {
        let job_id = job.id;
        let recap_job_id = job.recap_job_id;
        let chunk_idx = job.chunk_idx;

        error!(
            job_id,
            recap_job_id = %recap_job_id,
            chunk_idx,
            error = %error_str,
            "classification job failed with subworker authentication error"
        );
        let is_running = match store.get_job(job_id).await {
            Ok(Some(current)) => current.status == QueuedJobStatus::Running,
            Ok(None) => false,
            Err(store_err) => {
                error!(
                    job_id,
                    recap_job_id = %recap_job_id,
                    error = %store_err,
                    "failed to inspect chunk status before recording auth failure"
                );
                return Err(store_err.into());
            }
        };
        if is_running {
            auth_failures
                .lock()
                .unwrap_or_else(std::sync::PoisonError::into_inner)
                .entry(recap_job_id)
                .or_insert((chunk_idx, auth_err));
            if let Err(store_err) = store.mark_recap_job_failed(recap_job_id, error_str).await {
                auth_failures
                    .lock()
                    .unwrap_or_else(std::sync::PoisonError::into_inner)
                    .remove(&recap_job_id);
                error!(
                    job_id,
                    recap_job_id = %recap_job_id,
                    error = %store_err,
                    "failed to mark recap job chunks as failed"
                );
                return Err(store_err.into());
            }
        }
        Ok(())
    }

    async fn schedule_retry(
        store: &QueueStore,
        job: &QueuedJob,
        retry_delay_ms: u64,
        error_str: &str,
    ) -> Result<()> {
        // Exponential backoff, enforced by `next_retry_at` in the
        // store rather than a `sleep()` here: sleeping would
        // block this worker slot for the whole backoff window
        // while the row is already re-pickable.
        let delay_ms = retry_delay_ms * (1_u64 << job.retry_count.min(3));
        warn!(
            job_id = job.id,
            recap_job_id = %job.recap_job_id,
            chunk_idx = job.chunk_idx,
            retry_count = job.retry_count + 1,
            max_retries = job.max_retries,
            delay_ms,
            error = %error_str,
            "classification job failed, will retry"
        );

        if let Err(store_err) = store.mark_retrying(job.id, error_str, delay_ms).await {
            error!(
                job_id = job.id,
                error = %store_err,
                "failed to mark job as retrying"
            );
            return Err(store_err.into());
        }
        Ok(())
    }

    async fn mark_exhausted_failed(
        store: &QueueStore,
        job: &QueuedJob,
        error_str: &str,
    ) -> Result<()> {
        // Mark as failed (max retries exceeded)
        error!(
            job_id = job.id,
            recap_job_id = %job.recap_job_id,
            chunk_idx = job.chunk_idx,
            retry_count = job.retry_count,
            max_retries = job.max_retries,
            error = %error_str,
            "classification job failed after max retries"
        );

        if let Err(store_err) = store.mark_failed(job.id, error_str).await {
            error!(
                job_id = job.id,
                error = %store_err,
                "failed to mark job as failed"
            );
            return Err(store_err.into());
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn next_idle_backoff_doubles_and_caps_at_max() {
        let mut backoff = DEFAULT_POLL_INTERVAL;
        assert_eq!(backoff, Duration::from_millis(100));

        backoff = next_idle_backoff(backoff);
        assert_eq!(backoff, Duration::from_millis(200));

        backoff = next_idle_backoff(backoff);
        assert_eq!(backoff, Duration::from_millis(400));

        backoff = next_idle_backoff(backoff);
        assert_eq!(backoff, Duration::from_millis(800));

        backoff = next_idle_backoff(backoff);
        assert_eq!(backoff, Duration::from_millis(1600));

        backoff = next_idle_backoff(backoff);
        assert_eq!(backoff, MAX_POLL_INTERVAL);
        assert_eq!(backoff, Duration::from_secs(2));

        // Stays capped at MAX_POLL_INTERVAL
        backoff = next_idle_backoff(backoff);
        assert_eq!(backoff, MAX_POLL_INTERVAL);
    }
}
