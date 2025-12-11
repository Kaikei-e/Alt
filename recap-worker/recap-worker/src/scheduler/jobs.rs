use std::sync::Arc;

use anyhow::Result;
use uuid::Uuid;

use crate::{
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
    recap_dao: Arc<RecapDao>,
}

impl Scheduler {
    pub(crate) fn new(
        pipeline: Arc<PipelineOrchestrator>,
        morning_pipeline: Arc<MorningPipeline>,
        config: Arc<Config>,
        recap_dao: Arc<RecapDao>,
    ) -> Self {
        Self {
            pipeline,
            morning_pipeline,
            config,
            recap_dao,
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

                // Log failed task details
                let stage = context
                    .current_stage
                    .as_deref()
                    .unwrap_or("pipeline_execution");
                let error_msg = e.to_string();
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
}
