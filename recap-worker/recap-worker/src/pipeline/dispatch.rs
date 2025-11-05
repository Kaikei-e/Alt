use async_trait::async_trait;
use uuid::Uuid;

use crate::scheduler::JobContext;

use super::select::SelectedSummary;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct DispatchResult {
    pub(crate) job_id: Uuid,
    pub(crate) response_id: Option<String>,
}

#[async_trait]
pub(crate) trait DispatchStage: Send + Sync {
    async fn dispatch(
        &self,
        job: &JobContext,
        summary: SelectedSummary,
    ) -> anyhow::Result<DispatchResult>;
}

pub(crate) struct UnimplementedDispatchStage;

#[async_trait]
impl DispatchStage for UnimplementedDispatchStage {
    async fn dispatch(
        &self,
        job: &JobContext,
        _summary: SelectedSummary,
    ) -> anyhow::Result<DispatchResult> {
        anyhow::bail!(
            "dispatch stage not implemented for job {job}",
            job = job.job_id
        );
    }
}
