use std::sync::Arc;

use anyhow::{Context, Result};
use tracing::warn;
use uuid::Uuid;

use crate::{
    clients::SubworkerClient,
    clients::subworker::evaluation::EvaluateRequest,
    config::Config,
    pipeline::{PipelineOrchestrator, morning::MorningPipeline},
    store::dao::{JobStatus, RecapDao},
};

#[derive(Debug, Clone)]
pub(crate) struct JobContext {
    pub(crate) job_id: Uuid,
    pub(crate) genres: Vec<String>,
    pub(crate) current_stage: Option<String>,
}

impl JobContext {
    pub(crate) fn new(job_id: Uuid, genres: Vec<String>) -> Self {
        Self {
            job_id,
            genres,
            current_stage: None,
        }
    }

    pub(crate) fn with_stage(mut self, stage: String) -> Self {
        self.current_stage = Some(stage);
        self
    }

    pub(crate) fn genres(&self) -> &[String] {
        &self.genres
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
            Ok(_) => {
                self.recap_dao
                    .update_job_status(context.job_id, JobStatus::Completed, None)
                    .await?;

                // Run classification evaluation after successful job completion
                if self.config.classification_eval_enabled() {
                    if let Err(e) = self.run_classification_evaluation(context.job_id).await {
                        warn!(
                            job_id = %context.job_id,
                            error = %e,
                            "failed to run classification evaluation (job still marked as completed)"
                        );
                    }
                }

                Ok(())
            }
            Err(e) => {
                tracing::error!(job_id = %context.job_id, error = %e, "job execution failed");
                // Attempt to record failure status, but preserve original error
                if let Err(dao_err) = self
                    .recap_dao
                    .update_job_status(context.job_id, JobStatus::Failed, None)
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
