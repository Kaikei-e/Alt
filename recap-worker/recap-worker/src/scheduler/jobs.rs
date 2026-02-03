use std::sync::Arc;

use anyhow::{Context, Result, anyhow};
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

#[derive(Debug, Clone)]
pub(crate) struct JobContext {
    pub(crate) job_id: Uuid,
    pub(crate) genres: Vec<String>,
    pub(crate) current_stage: Option<String>,
    pub(crate) window_days: u32,
}

impl JobContext {
    pub(crate) fn new(job_id: Uuid, genres: Vec<String>) -> Self {
        Self {
            job_id,
            genres,
            current_stage: None,
            window_days: 7, // Default to 7 days for backward compatibility
        }
    }

    pub(crate) fn new_with_window(job_id: Uuid, genres: Vec<String>, window_days: u32) -> Self {
        Self {
            job_id,
            genres,
            current_stage: None,
            window_days,
        }
    }

    pub(crate) fn with_stage(mut self, stage: String) -> Self {
        self.current_stage = Some(stage);
        self
    }

    pub(crate) fn genres(&self) -> &[String] {
        &self.genres
    }

    pub(crate) fn window_days(&self) -> u32 {
        self.window_days
    }
}

#[derive(Clone)]
pub struct Scheduler {
    pipeline: Arc<PipelineOrchestrator>,
    morning_pipeline: Arc<MorningPipeline>,
    config: Arc<Config>,
    recap_dao: Arc<dyn RecapDao>,
    subworker_client: Arc<SubworkerClient>,
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
        }
    }

    pub(crate) async fn run_job(&self, context: JobContext) -> Result<()> {
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

        // Fallback: if we get here, something unexpected happened
        JobOutcome::Success
    }

    pub(crate) async fn run_morning_update(&self, context: JobContext) -> Result<()> {
        tracing::info!("running morning update job");
        self.morning_pipeline.execute_update(&context).await
    }

    pub(crate) async fn find_resumable_job(
        &self,
    ) -> Result<Option<(Uuid, JobStatus, Option<String>)>> {
        self.recap_dao.find_resumable_job().await
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

#[cfg(test)]
mod tests {
    use super::*;

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
}
