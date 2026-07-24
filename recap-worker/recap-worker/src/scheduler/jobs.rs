use std::sync::Arc;

use anyhow::{Context, Result, anyhow};
use tokio::sync::Semaphore;
use tracing::warn;
use uuid::Uuid;

use crate::{
    clients::SubworkerClient,
    clients::subworker::evaluation::EvaluateRequest,
    config::Config,
    pipeline::{PipelineOrchestrator, morning::MorningPipeline, persist::PersistResult},
    store::dao::{JobStatus, RecapDao},
};

/// Result of evaluating whether a job succeeded or failed based on PersistResult.
enum JobOutcome {
    /// Job succeeded (at least some genres were stored, or no genres to process)
    Success,
    /// Job failed (no genres stored despite having genres to process)
    Failed(String),
}

/// Outcome of `Scheduler::retry_most_recent_failed_job` (`POST
/// /admin/jobs/retry`). Every variant is an explicit, distinguishable
/// answer — there is no variant that means "did nothing but call it success"
/// (CLAUDE.md rule 8).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum RetryOutcome {
    /// A fresh pipeline run was spawned, reusing the window of
    /// `retried_failed_job_id`. Runs fire-and-forget in the background, same
    /// as the manual `/v1/generate/recaps/*` triggers.
    Started {
        job_id: Uuid,
        retried_failed_job_id: Uuid,
    },
    /// No `failed` row exists in `recap_jobs` — nothing to retry.
    NoFailedJob,
    /// Another recap pipeline run is already in flight (`run_lock`
    /// contention); retrying now would run two pipelines concurrently.
    AlreadyRunning,
}

/// Looks up the most recent `failed` job and reports what a retry should
/// target: the failed job's id (for logging) and the window_days to reuse
/// for the fresh run. Kept separate from `Scheduler` so it is testable with
/// `MockRecapDao` without constructing the full pipeline (mirrors
/// `boot_time_resumable_target`'s free-function split below).
async fn find_retry_target(recap_dao: &dyn RecapDao) -> Result<Option<(Uuid, u32)>> {
    let target = recap_dao.find_most_recent_failed_job().await?;
    Ok(target.map(|(job_id, _status, _last_stage, window_days)| (job_id, window_days)))
}

#[derive(Debug, Clone)]
pub(crate) struct JobContext {
    pub(crate) job_id: Uuid,
    pub(crate) genres: Vec<String>,
    pub(crate) current_stage: Option<String>,
    pub(crate) window_days: u32,
    /// discriminator for `recap_jobs.trigger_source`:
    ///   - `"system"`  (default; JST 02:00 batch daemon 由来)
    ///   - `"morning"` (`POST /v1/morning/letters/regenerate` 由来)
    ///   - `"user"`    (manual `/v1/generate/recaps/*`)
    pub(crate) trigger_source: &'static str,
    /// Owning user, when the recap is per-user (morning_update / manual
    /// `/v1/generate/recaps/*`). Stays `None` for system batches that don't
    /// produce a single-user knowledge_events row. Persist-stage
    /// `recap.topic_snapshotted.v1` emit (Knowledge Loop Completion Phase 1
    /// §2) requires both `user_id` and `tenant_id` to be set; the publish
    /// helper skips emit when either is `None`.
    pub(crate) user_id: Option<Uuid>,
    /// Tenant scope for the knowledge_events emit. Resolved alongside
    /// `user_id`; missing tenant means we cannot place the event in the
    /// right multi-tenant projection scope and must skip emit.
    pub(crate) tenant_id: Option<Uuid>,
}

impl JobContext {
    pub(crate) fn new(job_id: Uuid, genres: Vec<String>) -> Self {
        Self {
            job_id,
            genres,
            current_stage: None,
            window_days: 7, // Default to 7 days for backward compatibility
            trigger_source: "system",
            user_id: None,
            tenant_id: None,
        }
    }

    pub(crate) fn new_with_window(job_id: Uuid, genres: Vec<String>, window_days: u32) -> Self {
        Self {
            job_id,
            genres,
            current_stage: None,
            window_days,
            trigger_source: "system",
            user_id: None,
            tenant_id: None,
        }
    }

    /// 手動 `/v1/generate/recaps/*` 由来の Job 用 constructor。
    /// `trigger_source = "user"` がセットされるため、クラッシュ後の boot で
    /// `find_resumable_job`（`trigger_source='system'` フィルタ）に
    /// バッチ Recap として誤 resume されない。
    pub(crate) fn new_manual(job_id: Uuid, genres: Vec<String>, window_days: u32) -> Self {
        Self {
            job_id,
            genres,
            current_stage: None,
            window_days,
            trigger_source: "user",
            user_id: None,
            tenant_id: None,
        }
    }

    /// Morning-update 由来の Job 用 constructor。`trigger_source = "morning"`
    /// がセットされるため、プロセスクラッシュで残った行が boot 時に
    /// `find_resumable_job` から拾われず batch Recap として誤 resume されない。
    pub(crate) fn new_morning_update(job_id: Uuid) -> Self {
        Self {
            job_id,
            genres: Vec::new(),
            current_stage: None,
            // morning_update は overnight 1 日の窓で dedup + group する
            window_days: 1,
            trigger_source: "morning",
            user_id: None,
            tenant_id: None,
        }
    }

    pub(crate) fn with_stage(mut self, stage: String) -> Self {
        self.current_stage = Some(stage);
        self
    }

    /// Bind the owning user + tenant to the job. Used by per-user job
    /// constructors (morning_update, manual recap requests) so the
    /// persist-stage `recap.topic_snapshotted.v1` emit has the scope it
    /// needs. System batches keep both `None` and the publish helper
    /// gracefully skips emit.
    pub(crate) fn with_user_scope(mut self, user_id: Uuid, tenant_id: Uuid) -> Self {
        self.user_id = Some(user_id);
        self.tenant_id = Some(tenant_id);
        self
    }

    pub(crate) fn genres(&self) -> &[String] {
        &self.genres
    }

    pub(crate) fn window_days(&self) -> u32 {
        self.window_days
    }

    pub(crate) fn trigger_source(&self) -> &'static str {
        self.trigger_source
    }

    #[allow(dead_code)]
    pub(crate) fn user_id(&self) -> Option<Uuid> {
        self.user_id
    }

    #[allow(dead_code)]
    pub(crate) fn tenant_id(&self) -> Option<Uuid> {
        self.tenant_id
    }
}

#[derive(Clone)]
pub struct Scheduler {
    pipeline: Arc<PipelineOrchestrator>,
    morning_pipeline: Arc<MorningPipeline>,
    config: Arc<Config>,
    recap_dao: Arc<dyn RecapDao>,
    subworker_client: Arc<SubworkerClient>,
    /// Caps concurrent `run_job` executions at 1. `run_job` is the single
    /// choke point for the nightly batch daemon, the admin trigger, and the
    /// manual `/v1/generate/recaps/*` endpoints — without this, a manual
    /// re-trigger racing the nightly batch (or repeated manual triggers)
    /// could run the full LLM/clustering pipeline multiple times at once.
    run_lock: Arc<Semaphore>,
}

impl Scheduler {
    pub(crate) fn new(
        pipeline: Arc<PipelineOrchestrator>,
        morning_pipeline: Arc<MorningPipeline>,
        config: Arc<Config>,
        recap_dao: Arc<dyn RecapDao>,
        subworker_client: Arc<SubworkerClient>,
    ) -> Self {
        Self {
            pipeline,
            morning_pipeline,
            config,
            recap_dao,
            subworker_client,
            run_lock: Arc::new(Semaphore::new(1)),
        }
    }

    /// Gracefully stop the classification job queue's background worker
    /// tasks. Called from `main.rs`'s SIGTERM/SIGINT path so in-flight
    /// classification chunks are drained (or aborted) deliberately instead
    /// of being killed by process exit.
    pub async fn shutdown(&self) {
        self.pipeline.classification_queue().shutdown().await;
    }

    pub(crate) async fn run_job(&self, context: JobContext) -> Result<()> {
        // Non-blocking: a concurrent trigger must fail loudly and
        // immediately rather than queue up behind an in-flight pipeline run
        // (or worse, run alongside it).
        let _run_permit = Arc::clone(&self.run_lock)
            .try_acquire_owned()
            .map_err(|_| {
                anyhow!(
                    "job {} rejected: another recap pipeline run is already in progress",
                    context.job_id
                )
            })?;

        self.run_job_locked(context).await
    }

    /// Handles `POST /admin/jobs/retry`: finds the most recent `failed`
    /// recap job and re-triggers a fresh pipeline run for its window,
    /// fire-and-forget (same async-spawn pattern as the manual
    /// `/v1/generate/recaps/*` triggers in `api::generate`). The permit is
    /// acquired synchronously up front so an in-flight run is reported as
    /// `AlreadyRunning` immediately, instead of racing the spawned pipeline
    /// run or silently queuing behind it.
    ///
    /// Never fakes success (CLAUDE.md rule 8): the old implementation built
    /// an empty-`genres` `JobContext` unconditionally and reported success
    /// as soon as `run_job` returned `Ok`, regardless of whether there was
    /// anything to retry. This either genuinely reruns the pipeline for a
    /// real failed job, or reports explicitly why it did not
    /// (`NoFailedJob` / `AlreadyRunning`).
    pub(crate) async fn retry_most_recent_failed_job(&self) -> Result<RetryOutcome> {
        let Ok(permit) = Arc::clone(&self.run_lock).try_acquire_owned() else {
            return Ok(RetryOutcome::AlreadyRunning);
        };

        let Some((retried_failed_job_id, window_days)) =
            find_retry_target(self.recap_dao.as_ref()).await?
        else {
            return Ok(RetryOutcome::NoFailedJob);
        };

        let job_id = Uuid::new_v4();
        let job = JobContext::new_manual(job_id, Vec::new(), window_days);
        let scheduler = self.clone();

        tokio::spawn(async move {
            // Held for the lifetime of the spawned task so no other run can
            // start until this retry finishes.
            let _permit = permit;
            if let Err(error) = scheduler.run_job_locked(job).await {
                tracing::error!(
                    %job_id,
                    %retried_failed_job_id,
                    error = ?error,
                    "admin retry pipeline run failed"
                );
            } else {
                tracing::info!(
                    %job_id,
                    %retried_failed_job_id,
                    "admin retry pipeline run completed"
                );
            }
        });

        Ok(RetryOutcome::Started {
            job_id,
            retried_failed_job_id,
        })
    }

    /// Core pipeline-run body. Callers must already hold a `run_lock`
    /// permit: `run_job` acquires one itself before calling this;
    /// `retry_most_recent_failed_job` acquires its own up front (before the
    /// DAO lookup) so it can report `AlreadyRunning` synchronously instead
    /// of double-acquiring here.
    #[allow(clippy::too_many_lines)]
    async fn run_job_locked(&self, context: JobContext) -> Result<()> {
        tracing::info!(
            job_id = %context.job_id,
            prompt_version = %self.config.llm_prompt_version(),
            genres = context.genres().len(),
            "running recap job"
        );

        match self.pipeline.execute(&context).await {
            Ok(persist_result) => {
                // Check if the job actually succeeded based on PersistResult contents
                let job_outcome = Self::evaluate_job_outcome(&persist_result);

                match job_outcome {
                    JobOutcome::Success => {
                        self.recap_dao
                            .update_job_status_with_history(
                                context.job_id,
                                JobStatus::Completed,
                                None,
                                None,
                            )
                            .await?;

                        // Run classification evaluation after successful job completion
                        if self.config.classification_eval_enabled() {
                            if let Err(e) = self.run_classification_evaluation(context.job_id).await
                            {
                                warn!(
                                    job_id = %context.job_id,
                                    error = %e,
                                    "failed to run classification evaluation (job still marked as completed)"
                                );
                            }
                        }

                        Ok(())
                    }
                    JobOutcome::Failed(reason) => {
                        // Pipeline completed but no genres were stored - this is a failure
                        tracing::error!(
                            job_id = %context.job_id,
                            genres_stored = persist_result.genres_stored,
                            genres_failed = persist_result.genres_failed,
                            genres_skipped = persist_result.genres_skipped,
                            genres_no_evidence = persist_result.genres_no_evidence,
                            total_genres = persist_result.total_genres,
                            "job completed but no genres were stored - marking as failed"
                        );

                        if let Err(dao_err) = self
                            .recap_dao
                            .update_job_status_with_history(
                                context.job_id,
                                JobStatus::Failed,
                                None,
                                Some(&reason),
                            )
                            .await
                        {
                            tracing::error!(job_id = %context.job_id, error = %dao_err, "failed to update job status to failed");
                        }

                        // Log failed task details
                        if let Err(log_err) = self
                            .recap_dao
                            .insert_failed_task(context.job_id, "persist", None, Some(&reason))
                            .await
                        {
                            tracing::error!(job_id = %context.job_id, error = %log_err, "failed to insert failed task log");
                        }

                        Err(anyhow!(reason))
                    }
                }
            }
            Err(e) => {
                tracing::error!(job_id = %context.job_id, error = %e, "job execution failed");
                // Attempt to record failure status with error reason, but preserve original error
                let error_reason = format!("{:#}", e);
                if let Err(dao_err) = self
                    .recap_dao
                    .update_job_status_with_history(
                        context.job_id,
                        JobStatus::Failed,
                        None,
                        Some(&error_reason),
                    )
                    .await
                {
                    tracing::error!(job_id = %context.job_id, error = %dao_err, "failed to update job status to failed");
                }

                // Log failed task details with full error chain
                let stage = context
                    .current_stage
                    .as_deref()
                    .unwrap_or("pipeline_execution");
                // Use {:#} format to include full error chain (Caused by: ...)
                let error_msg = format!("{:#}", e);
                if let Err(log_err) = self
                    .recap_dao
                    .insert_failed_task(context.job_id, stage, None, Some(&error_msg))
                    .await
                {
                    tracing::error!(job_id = %context.job_id, error = %log_err, "failed to insert failed task log");
                }

                Err(e)
            }
        }
    }

    /// Evaluates whether a job should be considered successful or failed based on PersistResult.
    ///
    /// Decision logic:
    /// - genres_stored > 0: Success (partial success is still success)
    /// - genres_stored == 0 && (genres_failed > 0 || genres_skipped > 0): Failed
    /// - genres_stored == 0 && genres_no_evidence > 0 only: Success (no articles is a valid state)
    /// - genres_stored == 0 && total_genres == 0: Success (empty job is valid)
    fn evaluate_job_outcome(persist_result: &PersistResult) -> JobOutcome {
        // If any genres were stored, the job succeeded (partial success is success)
        if persist_result.genres_stored > 0 {
            return JobOutcome::Success;
        }

        // If no genres to process, that's a valid completion
        if persist_result.total_genres == 0 {
            return JobOutcome::Success;
        }

        // If genres_stored == 0 but we have failures or skips, that's a failure
        if persist_result.genres_failed > 0 || persist_result.genres_skipped > 0 {
            let reason = format!(
                "No genres were stored: failed={}, skipped={}, no_evidence={}",
                persist_result.genres_failed,
                persist_result.genres_skipped,
                persist_result.genres_no_evidence
            );
            return JobOutcome::Failed(reason);
        }

        // If genres_stored == 0 but only genres_no_evidence > 0, that's valid
        // (all genres had no articles assigned, which is a legitimate state)
        if persist_result.genres_no_evidence > 0 {
            return JobOutcome::Success;
        }

        // Fallback: total_genres > 0 but all counters are 0
        // → classification completely failed (e.g. subworker down)
        JobOutcome::Failed(format!(
            "No genres produced results: total_genres={}, all counters zero (classification may have failed entirely)",
            persist_result.total_genres
        ))
    }

    pub(crate) async fn run_morning_update(&self, context: JobContext) -> Result<()> {
        tracing::info!(job_id = %context.job_id, "running morning update job");
        // `execute_update`'s fetch stage creates the `recap_jobs` row with the
        // schema default `status='pending'`. Unlike the batch pipeline, the
        // morning pipeline never advances that row — so close it out here, or it
        // leaks one orphaned `pending` row every 30-min tick.
        let outcome = self.morning_pipeline.execute_update(&context).await;
        finalize_morning_job(self.recap_dao.as_ref(), context.job_id, outcome).await
    }

    /// Boot-time hygiene (CLAUDE.md rule 8): resolve the one resumable job
    /// (if any) and seal every other orphaned pending/running row as
    /// `failed`. See [`boot_time_resumable_target`] for the testable core.
    pub(crate) async fn resolve_boot_time_resumable_target(
        &self,
    ) -> Option<(Uuid, JobStatus, Option<String>, u32)> {
        let max_age = self.config.resumable_max_age_hours();
        boot_time_resumable_target(self.recap_dao.as_ref(), max_age).await
    }

    /// 保持期間より古いジョブを削除する。
    ///
    /// CASCADEにより、関連するrecap_job_articles、recap_stage_state等も自動削除される。
    pub(crate) async fn cleanup_old_jobs(&self) -> Result<u64> {
        let retention_days = self.config.job_retention_days();
        let deleted_count = self.recap_dao.delete_old_jobs(retention_days).await?;
        if deleted_count > 0 {
            tracing::info!(retention_days, deleted_count, "cleaned up old recap jobs");
        }
        Ok(deleted_count)
    }

    /// 分類評価を実行し、結果をrecap_system_metricsに保存する
    async fn run_classification_evaluation(&self, job_id: Uuid) -> Result<()> {
        tracing::info!(job_id = %job_id, "running classification evaluation");

        let request = EvaluateRequest {
            golden_data_path: None, // Use default path
            weights_path: None,
            use_bootstrap: self.config.classification_eval_use_bootstrap(),
            n_bootstrap: self.config.classification_eval_n_bootstrap(),
            use_cross_validation: self.config.classification_eval_use_cv(),
            n_folds: 5,        // Default value
            save_to_db: false, // We'll save to system_metrics ourselves
        };

        let eval_response = self
            .subworker_client
            .evaluate_genres(&request)
            .await
            .context("failed to call evaluation API")?;

        // Convert evaluation response to system metrics format
        let genre_count = eval_response.per_genre_metrics.len();
        let mut metrics = serde_json::json!({
            "accuracy": eval_response.accuracy,
            "macro_f1": eval_response.macro_f1,
            "micro_f1": eval_response.micro_f1,
            "hamming_loss": 0.0, // Not provided by evaluation API, set to 0.0
        });

        // Add per-genre metrics
        let mut per_genre: serde_json::Map<String, serde_json::Value> = serde_json::Map::new();
        for (genre, metric) in eval_response.per_genre_metrics {
            per_genre.insert(
                genre.clone(),
                serde_json::json!({
                    "precision": metric.precision,
                    "recall": metric.recall,
                    "f1-score": metric.f1,
                    "support": metric.support,
                    "threshold": metric.threshold.unwrap_or(0.5),
                }),
            );
        }
        metrics["per_genre"] = serde_json::Value::Object(per_genre);

        // Save to system_metrics
        self.recap_dao
            .save_system_metrics(job_id, "classification", &metrics)
            .await
            .context("failed to save classification metrics")?;

        tracing::info!(
            job_id = %job_id,
            accuracy = eval_response.accuracy,
            macro_f1 = eval_response.macro_f1,
            genre_count,
            "classification evaluation completed and saved"
        );

        Ok(())
    }
}

/// Seal the `recap_jobs` row a morning-update tick created, mirroring how
/// [`Scheduler::run_job`] seals batch jobs: success → [`JobStatus::MorningCompleted`],
/// failure → [`JobStatus::Failed`] (with the error chain as the reason). A DAO
/// write failure here is logged but never masks the pipeline's own result.
async fn finalize_morning_job(
    recap_dao: &dyn RecapDao,
    job_id: Uuid,
    outcome: Result<()>,
) -> Result<()> {
    match outcome {
        Ok(()) => {
            if let Err(dao_err) = recap_dao
                .update_job_status_with_history(
                    job_id,
                    JobStatus::MorningCompleted,
                    Some("persist"),
                    None,
                )
                .await
            {
                tracing::error!(%job_id, error = %dao_err, "failed to mark morning update job completed");
            }
            Ok(())
        }
        Err(e) => {
            let reason = format!("{:#}", e);
            tracing::error!(%job_id, error = %reason, "morning update job execution failed");
            if let Err(dao_err) = recap_dao
                .update_job_status_with_history(job_id, JobStatus::Failed, None, Some(&reason))
                .await
            {
                tracing::error!(%job_id, error = %dao_err, "failed to mark morning update job failed");
            }
            Err(e)
        }
    }
}

/// Testable core of [`Scheduler::resolve_boot_time_resumable_target`].
///
/// A DB error from `find_resumable_job` must never fold into "no resumable
/// job" (CLAUDE.md rule 8) — doing so would let the unconditional
/// `mark_abandoned_jobs(None)` sweep below seal an undetected resumable job
/// as `failed` too, exactly the boot-time hygiene invariant the caller
/// (`BatchDaemon::run`) documents. On error we log loudly and skip the
/// sweep entirely this cycle, leaving hygiene to retry on the next boot.
async fn boot_time_resumable_target(
    recap_dao: &dyn RecapDao,
    max_age_hours: i64,
) -> Option<(Uuid, JobStatus, Option<String>, u32)> {
    let resumable_target = match recap_dao.find_resumable_job(max_age_hours).await {
        Ok(target) => target,
        Err(err) => {
            tracing::error!(
                error = %err,
                "boot-time hygiene: find_resumable_job failed — skipping mark_abandoned_jobs sweep this cycle to avoid sealing an undetected resumable job as failed"
            );
            return None;
        }
    };

    let keep_job_id = resumable_target.as_ref().map(|(id, _, _, _)| *id);
    match recap_dao.mark_abandoned_jobs(keep_job_id).await {
        Ok(updated) if updated > 0 => tracing::info!(
            ?keep_job_id,
            marked_failed = updated,
            "sealed orphaned recap jobs as failed"
        ),
        Ok(_) => {}
        Err(err) => tracing::error!(error = %err, "boot-time hygiene: mark_abandoned_jobs failed"),
    }

    resumable_target
}

#[cfg(test)]
mod tests {
    use super::*;

    /// RED→GREEN regression (rule 8): when there is no `failed` job to
    /// retry, `find_retry_target` must report `None` — never fabricate a
    /// target. This is the core of the fix for the fake `/admin/jobs/retry`
    /// endpoint, which previously always kicked off a disconnected fresh
    /// run and reported success regardless of whether anything had failed.
    #[tokio::test]
    async fn find_retry_target_returns_none_when_no_failed_job() {
        use crate::store::dao::mock::MockRecapDao;

        let dao = MockRecapDao::new();
        dao.set_find_most_recent_failed_job_result(Ok(None));

        let target = find_retry_target(&dao).await.expect("dao call succeeds");

        assert!(target.is_none(), "no failed job means nothing to retry");
    }

    /// When a failed job exists, its window_days must be reused for the
    /// retry — not silently dropped like the old empty-genres JobContext.
    #[tokio::test]
    async fn find_retry_target_reuses_window_days_of_failed_job() {
        use crate::store::dao::mock::MockRecapDao;

        let dao = MockRecapDao::new();
        let failed_job_id = Uuid::new_v4();
        dao.set_find_most_recent_failed_job_result(Ok(Some((
            failed_job_id,
            JobStatus::Failed,
            Some("dispatch".to_string()),
            3,
        ))));

        let target = find_retry_target(&dao).await.expect("dao call succeeds");

        assert_eq!(target, Some((failed_job_id, 3)));
    }

    /// A transient DB error must propagate, not fold into "no failed job"
    /// (which would silently report `NoFailedJob` — itself an honest
    /// answer, but for the wrong reason, masking a DB outage as "nothing to
    /// retry").
    #[tokio::test]
    async fn find_retry_target_propagates_dao_error() {
        use crate::error::RecapError;
        use crate::store::dao::mock::MockRecapDao;

        let dao = MockRecapDao::new();
        dao.set_find_most_recent_failed_job_result(Err(RecapError::Db(
            "connection reset by peer".to_string(),
        )));

        let result = find_retry_target(&dao).await;

        assert!(
            result.is_err(),
            "DB error must propagate, not degrade to None"
        );
    }

    /// Regression: `JobContext::new` and `new_with_window` must tag rows with
    /// `trigger_source = "system"` so the JST 02:00 batch daemon remains the
    /// only producer of auto-resumable rows.
    #[test]
    fn job_context_default_trigger_source_is_system() {
        let plain = JobContext::new(Uuid::new_v4(), vec!["ai".into()]);
        assert_eq!(plain.trigger_source(), "system");

        let windowed = JobContext::new_with_window(Uuid::new_v4(), vec!["ai".into()], 3);
        assert_eq!(windowed.trigger_source(), "system");
        assert_eq!(windowed.window_days(), 3);
    }

    /// Core of Phase 4 (ADR-000709): morning_update Jobs MUST set
    /// `trigger_source = "morning"` so `find_resumable_job` (which filters
    /// `trigger_source = 'system'`) never picks them up as a batch Recap
    /// candidate after a crash.
    #[test]
    fn job_context_new_morning_update_marks_trigger_source_morning() {
        let ctx = JobContext::new_morning_update(Uuid::new_v4());
        assert_eq!(ctx.trigger_source(), "morning");
        assert_eq!(
            ctx.window_days(),
            1,
            "morning_update operates on a single overnight window"
        );
        assert!(
            ctx.genres().is_empty(),
            "morning_update needs no genre fan-out; genres must be empty"
        );
        assert!(
            ctx.current_stage.is_none(),
            "fresh context has no resume stage"
        );
    }

    /// `with_stage` preserves the existing `trigger_source` tag. Without this
    /// invariant a resumed morning_update row could be silently relabelled
    /// as 'system' when reconstructing a JobContext from DB state.
    #[test]
    fn job_context_with_stage_preserves_trigger_source() {
        let ctx = JobContext::new_morning_update(Uuid::new_v4()).with_stage("dedup".into());
        assert_eq!(ctx.trigger_source(), "morning");
        assert_eq!(ctx.current_stage.as_deref(), Some("dedup"));
    }

    /// `JobStatus::MorningCompleted` must round-trip through the textual form
    /// stored in `recap_jobs.status` (and accepted by the history CHECK
    /// constraint added alongside this status).
    #[test]
    fn job_status_morning_completed_round_trips_via_db_string() {
        assert_eq!(JobStatus::MorningCompleted.as_ref(), "morning_completed");
        assert_eq!(
            JobStatus::from_db_str("morning_completed"),
            JobStatus::MorningCompleted
        );
        assert!(JobStatus::MorningCompleted.is_terminal());
        // Unknown / future values still degrade to Failed, as before.
        assert_eq!(JobStatus::from_db_str("???"), JobStatus::Failed);
    }

    /// A morning-update tick that finishes cleanly must seal its `recap_jobs`
    /// row as `morning_completed` — otherwise the row stays at the schema
    /// default `pending` forever and a fresh one leaks every 30 minutes.
    #[tokio::test]
    async fn finalize_morning_job_seals_completed_on_success() {
        use crate::store::dao::mock::MockRecapDao;

        let dao = MockRecapDao::new();
        let job_id = Uuid::new_v4();

        finalize_morning_job(&dao, job_id, Ok(())).await.unwrap();

        let transitions = dao.status_transitions();
        assert_eq!(transitions.len(), 1, "exactly one terminal transition");
        assert_eq!(transitions[0].job_id, job_id);
        assert_eq!(transitions[0].status, JobStatus::MorningCompleted);
        assert_eq!(transitions[0].last_stage.as_deref(), Some("persist"));
        assert!(transitions[0].reason.is_none());
    }

    /// A failed morning-update tick seals the row as `failed` with the error
    /// chain as the reason, and the original error still propagates so the
    /// daemon logs it.
    #[tokio::test]
    async fn finalize_morning_job_seals_failed_and_propagates_error() {
        use crate::store::dao::mock::MockRecapDao;

        let dao = MockRecapDao::new();
        let job_id = Uuid::new_v4();
        let err = anyhow!("news-creator unreachable");

        let result = finalize_morning_job(&dao, job_id, Err(err)).await;
        assert!(result.is_err(), "pipeline error must propagate");
        assert!(
            format!("{:#}", result.unwrap_err()).contains("news-creator unreachable"),
            "original error chain preserved"
        );

        let transitions = dao.status_transitions();
        assert_eq!(transitions.len(), 1);
        assert_eq!(transitions[0].status, JobStatus::Failed);
        assert_eq!(
            transitions[0].reason.as_deref(),
            Some("news-creator unreachable")
        );
    }

    /// RED→GREEN regression (rule 8): a transient DB error from
    /// `find_resumable_job` must NOT be folded into "no resumable job".
    /// Before the fix, `daemon.rs` used `.ok().flatten()`, which collapsed
    /// `Err` and `Ok(None)` into the same `None` and then called
    /// `mark_abandoned_jobs(None)` unconditionally — sealing every
    /// pending/running row (including one that may have been genuinely
    /// resumable) as `failed` on a one-off DB hiccup. The fix must skip the
    /// sweep entirely when `find_resumable_job` errors.
    #[tokio::test]
    async fn boot_time_resumable_target_skips_sweep_on_find_resumable_job_error() {
        use crate::error::RecapError;
        use crate::store::dao::mock::MockRecapDao;

        let dao = MockRecapDao::new();
        dao.set_find_resumable_job_result(Err(RecapError::Db(
            "connection reset by peer".to_string(),
        )));

        let target = boot_time_resumable_target(&dao, 12).await;

        assert!(
            target.is_none(),
            "no resumable target should be returned on DB error"
        );
        assert!(
            dao.mark_abandoned_jobs_calls().is_empty(),
            "mark_abandoned_jobs must not be called this cycle when find_resumable_job errored, \
             or a transient DB hiccup would seal every in-flight job as failed"
        );
    }

    /// When there genuinely is no resumable job (`Ok(None)`), the sweep must
    /// still run — this is the legitimate "everything in-flight is orphaned"
    /// path.
    #[tokio::test]
    async fn boot_time_resumable_target_sweeps_all_when_none_resumable() {
        use crate::store::dao::mock::MockRecapDao;

        let dao = MockRecapDao::new();
        dao.set_find_resumable_job_result(Ok(None));

        let target = boot_time_resumable_target(&dao, 12).await;

        assert!(target.is_none());
        assert_eq!(dao.mark_abandoned_jobs_calls(), vec![None]);
    }

    /// When a resumable job is found, the sweep must run keeping exactly
    /// that job's id, and the resumable target must be returned to the
    /// caller so `BatchDaemon::run` can resume it.
    #[tokio::test]
    async fn boot_time_resumable_target_keeps_resumable_job_out_of_sweep() {
        use crate::store::dao::mock::MockRecapDao;

        let dao = MockRecapDao::new();
        let job_id = Uuid::new_v4();
        dao.set_find_resumable_job_result(Ok(Some((
            job_id,
            JobStatus::Running,
            Some("select".to_string()),
            3,
        ))));

        let target = boot_time_resumable_target(&dao, 12).await;

        assert_eq!(
            target,
            Some((job_id, JobStatus::Running, Some("select".to_string()), 3))
        );
        assert_eq!(dao.mark_abandoned_jobs_calls(), vec![Some(job_id)]);
    }

    /// Test: Job should be marked as Failed when genres_stored=0 but genres_failed>0
    #[test]
    fn test_job_marked_failed_when_no_genres_stored_with_failures() {
        let persist_result = PersistResult {
            job_id: Uuid::new_v4(),
            genres_stored: 0,
            genres_failed: 55,
            genres_skipped: 0,
            genres_no_evidence: 5,
            total_genres: 60,
        };

        let outcome = Scheduler::evaluate_job_outcome(&persist_result);

        match outcome {
            JobOutcome::Failed(reason) => {
                assert!(reason.contains("No genres were stored"));
                assert!(reason.contains("failed=55"));
            }
            JobOutcome::Success => {
                panic!("Expected job to be marked as Failed, but got Success");
            }
        }
    }

    /// Test: Job should be marked as Failed when genres_stored=0 but genres_skipped>0
    #[test]
    fn test_job_marked_failed_when_no_genres_stored_with_skipped() {
        let persist_result = PersistResult {
            job_id: Uuid::new_v4(),
            genres_stored: 0,
            genres_failed: 0,
            genres_skipped: 10,
            genres_no_evidence: 5,
            total_genres: 15,
        };

        let outcome = Scheduler::evaluate_job_outcome(&persist_result);

        match outcome {
            JobOutcome::Failed(reason) => {
                assert!(reason.contains("No genres were stored"));
                assert!(reason.contains("skipped=10"));
            }
            JobOutcome::Success => {
                panic!("Expected job to be marked as Failed, but got Success");
            }
        }
    }

    /// Test: Job should be marked as Completed when some genres are stored (partial success)
    #[test]
    fn test_job_marked_completed_when_some_genres_stored() {
        let persist_result = PersistResult {
            job_id: Uuid::new_v4(),
            genres_stored: 5,
            genres_failed: 10,
            genres_skipped: 2,
            genres_no_evidence: 3,
            total_genres: 20,
        };

        let outcome = Scheduler::evaluate_job_outcome(&persist_result);

        match outcome {
            JobOutcome::Success => {
                // Expected
            }
            JobOutcome::Failed(reason) => {
                panic!(
                    "Expected job to be marked as Success, but got Failed: {}",
                    reason
                );
            }
        }
    }

    /// Test: Job should be marked as Completed when all genres have no evidence
    /// (this is a valid state - no articles in the time window)
    #[test]
    fn test_job_marked_completed_when_only_no_evidence() {
        let persist_result = PersistResult {
            job_id: Uuid::new_v4(),
            genres_stored: 0,
            genres_failed: 0,
            genres_skipped: 0,
            genres_no_evidence: 10,
            total_genres: 10,
        };

        let outcome = Scheduler::evaluate_job_outcome(&persist_result);

        match outcome {
            JobOutcome::Success => {
                // Expected - no evidence is a valid completion state
            }
            JobOutcome::Failed(reason) => {
                panic!(
                    "Expected job to be marked as Success, but got Failed: {}",
                    reason
                );
            }
        }
    }

    /// Test: Job should be marked as Completed when total_genres=0 (empty job)
    #[test]
    fn test_job_marked_completed_when_empty_job() {
        let persist_result = PersistResult {
            job_id: Uuid::new_v4(),
            genres_stored: 0,
            genres_failed: 0,
            genres_skipped: 0,
            genres_no_evidence: 0,
            total_genres: 0,
        };

        let outcome = Scheduler::evaluate_job_outcome(&persist_result);

        match outcome {
            JobOutcome::Success => {
                // Expected - empty job is valid
            }
            JobOutcome::Failed(reason) => {
                panic!(
                    "Expected job to be marked as Success, but got Failed: {}",
                    reason
                );
            }
        }
    }

    /// Test: Mixed scenario - genres_stored=0 with both failures and no_evidence
    #[test]
    fn test_job_marked_failed_with_mixed_failures_and_no_evidence() {
        let persist_result = PersistResult {
            job_id: Uuid::new_v4(),
            genres_stored: 0,
            genres_failed: 30,
            genres_skipped: 5,
            genres_no_evidence: 25,
            total_genres: 60,
        };

        let outcome = Scheduler::evaluate_job_outcome(&persist_result);

        match outcome {
            JobOutcome::Failed(reason) => {
                assert!(reason.contains("No genres were stored"));
                assert!(reason.contains("failed=30"));
                assert!(reason.contains("skipped=5"));
            }
            JobOutcome::Success => {
                panic!("Expected job to be marked as Failed, but got Success");
            }
        }
    }

    /// Test: Job should be marked as Failed when total_genres > 0
    /// but all counters are zero (classification completely failed, e.g. subworker down)
    #[test]
    fn test_job_marked_failed_when_all_counters_zero_but_genres_exist() {
        let persist_result = PersistResult {
            job_id: Uuid::new_v4(),
            genres_stored: 0,
            genres_failed: 0,
            genres_skipped: 0,
            genres_no_evidence: 0,
            total_genres: 30,
        };

        let outcome = Scheduler::evaluate_job_outcome(&persist_result);

        match outcome {
            JobOutcome::Failed(reason) => {
                assert!(
                    reason.contains("classification"),
                    "Expected reason to mention classification, got: {reason}"
                );
            }
            JobOutcome::Success => {
                panic!("Expected Failed when all counters are zero but total_genres > 0");
            }
        }
    }
}
