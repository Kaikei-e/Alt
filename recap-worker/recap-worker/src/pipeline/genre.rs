use async_trait::async_trait;
use uuid::Uuid;

use crate::scheduler::JobContext;

use super::dedup::DeduplicatedCorpus;
use super::preprocess::PreprocessedArticle;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct GenreAssignment {
    pub(crate) genre: String,
    pub(crate) article: PreprocessedArticle,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct GenreBundle {
    pub(crate) job_id: Uuid,
    pub(crate) assignments: Vec<GenreAssignment>,
}

#[async_trait]
pub(crate) trait GenreStage: Send + Sync {
    async fn assign(
        &self,
        job: &JobContext,
        corpus: DeduplicatedCorpus,
    ) -> anyhow::Result<GenreBundle>;
}

pub(crate) struct UnimplementedGenreStage;

#[async_trait]
impl GenreStage for UnimplementedGenreStage {
    async fn assign(
        &self,
        job: &JobContext,
        _corpus: DeduplicatedCorpus,
    ) -> anyhow::Result<GenreBundle> {
        anyhow::bail!(
            "genre stage not implemented for job {job}",
            job = job.job_id
        );
    }
}
