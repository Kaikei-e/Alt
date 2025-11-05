use async_trait::async_trait;
use uuid::Uuid;

use crate::scheduler::JobContext;

use super::preprocess::{PreprocessedArticle, PreprocessedCorpus};

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct DeduplicatedCorpus {
    pub(crate) job_id: Uuid,
    pub(crate) articles: Vec<PreprocessedArticle>,
}

#[async_trait]
pub(crate) trait DedupStage: Send + Sync {
    async fn deduplicate(
        &self,
        job: &JobContext,
        _corpus: PreprocessedCorpus,
    ) -> anyhow::Result<DeduplicatedCorpus>;
}

pub(crate) struct UnimplementedDedupStage;

#[async_trait]
impl DedupStage for UnimplementedDedupStage {
    async fn deduplicate(
        &self,
        job: &JobContext,
        _corpus: PreprocessedCorpus,
    ) -> anyhow::Result<DeduplicatedCorpus> {
        anyhow::bail!(
            "dedup stage not implemented for job {job}",
            job = job.job_id
        );
    }
}
