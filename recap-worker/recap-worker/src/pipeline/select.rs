use async_trait::async_trait;
use uuid::Uuid;

use crate::scheduler::JobContext;

use super::genre::{GenreAssignment, GenreBundle};

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct SelectedSummary {
    pub(crate) job_id: Uuid,
    pub(crate) assignments: Vec<GenreAssignment>,
}

#[async_trait]
pub(crate) trait SelectStage: Send + Sync {
    async fn select(
        &self,
        job: &JobContext,
        bundle: GenreBundle,
    ) -> anyhow::Result<SelectedSummary>;
}

pub(crate) struct UnimplementedSelectStage;

#[async_trait]
impl SelectStage for UnimplementedSelectStage {
    async fn select(
        &self,
        job: &JobContext,
        _bundle: GenreBundle,
    ) -> anyhow::Result<SelectedSummary> {
        anyhow::bail!(
            "select stage not implemented for job {job}",
            job = job.job_id
        );
    }
}
