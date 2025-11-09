use std::sync::Arc;

use anyhow::Result;
use uuid::Uuid;

use crate::{config::Config, pipeline::PipelineOrchestrator};

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
    config: Arc<Config>,
}

impl Scheduler {
    pub(crate) fn new(pipeline: Arc<PipelineOrchestrator>, config: Arc<Config>) -> Self {
        Self { pipeline, config }
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
}
