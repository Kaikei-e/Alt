use std::sync::Arc;

use async_trait::async_trait;

use crate::store::{dao::RecapDao, models::PersistedGenre};

use crate::scheduler::JobContext;

use super::dispatch::DispatchResult;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct PersistResult {
    pub(crate) stored: bool,
}

#[async_trait]
pub(crate) trait PersistStage: Send + Sync {
    async fn persist(
        &self,
        job: &JobContext,
        result: DispatchResult,
    ) -> anyhow::Result<PersistResult>;
}

pub(crate) struct UnimplementedPersistStage;

#[async_trait]
impl PersistStage for UnimplementedPersistStage {
    async fn persist(
        &self,
        job: &JobContext,
        _result: DispatchResult,
    ) -> anyhow::Result<PersistResult> {
        anyhow::bail!(
            "persist stage not implemented for job {job}",
            job = job.job_id
        );
    }
}

pub(crate) struct LoggingPersistStage {
    dao: Arc<RecapDao>,
}

impl LoggingPersistStage {
    pub(crate) fn new(dao: Arc<RecapDao>) -> Self {
        Self { dao }
    }
}

#[async_trait]
impl PersistStage for LoggingPersistStage {
    async fn persist(
        &self,
        job: &JobContext,
        result: DispatchResult,
    ) -> anyhow::Result<PersistResult> {
        for genre in job.genres() {
            let record = PersistedGenre::new(job.job_id, genre.clone())
                .with_response_id(result.response_id.clone());
            self.dao.upsert_genre(&record).await?;
        }

        Ok(PersistResult {
            stored: result.response_id.is_some(),
        })
    }
}
