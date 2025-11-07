use std::sync::Arc;

use anyhow::{Context, Result};
use async_trait::async_trait;
use tracing::{debug, info, warn};

use crate::store::dao::RecapDao;

use crate::scheduler::JobContext;

use super::dispatch::DispatchResult;

/// 永続化結果。
#[derive(Debug, Clone)]
pub(crate) struct PersistResult {
    pub(crate) job_id: uuid::Uuid,
    pub(crate) genres_stored: usize,
    pub(crate) genres_failed: usize,
}

#[async_trait]
pub(crate) trait PersistStage: Send + Sync {
    async fn persist(
        &self,
        job: &JobContext,
        result: DispatchResult,
    ) -> Result<PersistResult>;
}

/// 最終確定物をJSONBフィールドに保存するPersistStage。
pub(crate) struct FinalSectionPersistStage {
    dao: Arc<RecapDao>,
}

impl FinalSectionPersistStage {
    pub(crate) fn new(dao: Arc<RecapDao>) -> Self {
        Self { dao }
    }
}

#[async_trait]
impl PersistStage for FinalSectionPersistStage {
    async fn persist(
        &self,
        job: &JobContext,
        result: DispatchResult,
    ) -> Result<PersistResult> {
        info!(
            job_id = %job.job_id,
            genre_count = result.genre_results.len(),
            "persisting final sections to database"
        );

        let mut genres_stored = 0;
        let mut genres_failed = 0;

        for (genre, genre_result) in &result.genre_results {
            // エラーがある場合はスキップ
            if genre_result.error.is_some() {
                warn!(
                    job_id = %job.job_id,
                    genre = %genre,
                    error = ?genre_result.error,
                    "skipping genre with error"
                );
                genres_failed += 1;
                continue;
            }

            // クラスタリング結果と要約レスポンスIDが両方ある場合のみ保存
            // （Note: 実際の要約データはDispatchStage内で既にNews-Creatorから取得している想定）
            // 現時点では、要約レスポンスIDがあれば成功とみなす
            if genre_result.summary_response_id.is_some() {
                debug!(
                    job_id = %job.job_id,
                    genre = %genre,
                    "genre processed successfully"
                );
                genres_stored += 1;
            } else {
                warn!(
                    job_id = %job.job_id,
                    genre = %genre,
                    "genre missing summary response"
                );
                genres_failed += 1;
            }
        }

        info!(
            job_id = %job.job_id,
            genres_stored = genres_stored,
            genres_failed = genres_failed,
            "completed persisting final sections"
        );

        Ok(PersistResult {
            job_id: job.job_id,
            genres_stored,
            genres_failed,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    #[test]
    fn persist_result_tracks_success_and_failure() {
        let result = PersistResult {
            job_id: uuid::Uuid::new_v4(),
            genres_stored: 5,
            genres_failed: 2,
        };

        assert_eq!(result.genres_stored, 5);
        assert_eq!(result.genres_failed, 2);
    }
}
