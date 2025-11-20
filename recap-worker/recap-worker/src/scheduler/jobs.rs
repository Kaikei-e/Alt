use std::sync::Arc;

use anyhow::Result;
use uuid::Uuid;

use crate::{
    config::Config,
    pipeline::{PipelineOrchestrator, morning::MorningPipeline},
};

#[derive(Debug, Clone)]
pub(crate) struct JobContext {
    pub(crate) job_id: Uuid,
    pub(crate) genres: Vec<String>,
}

impl JobContext {
    pub(crate) fn new(job_id: Uuid, genres: Vec<String>) -> Self {
        Self { job_id, genres }
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
}

impl Scheduler {
    pub(crate) fn new(
        pipeline: Arc<PipelineOrchestrator>,
        morning_pipeline: Arc<MorningPipeline>,
        config: Arc<Config>,
    ) -> Self {
        Self {
            pipeline,
            morning_pipeline,
            config,
        }
    }

    pub(crate) async fn run_job(&self, context: JobContext) -> Result<()> {
        tracing::info!(
            job_id = %context.job_id,
            prompt_version = %self.config.llm_prompt_version(),
            genres = context.genres().len(),
            "running recap job"
        );
        self.pipeline.execute(&context).await.map(|_| ())
    }

    pub(crate) async fn run_morning_update(&self, context: JobContext) -> Result<()> {
        tracing::info!("running morning update job");
        self.morning_pipeline.execute_update(&context).await
    }
}
