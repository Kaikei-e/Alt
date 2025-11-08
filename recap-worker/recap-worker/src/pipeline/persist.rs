use std::sync::Arc;

use anyhow::{Context, Result};
use async_trait::async_trait;
use tracing::{debug, info, warn};

use crate::scheduler::JobContext;
use crate::store::{
    dao::RecapDao,
    models::{PersistedGenre, RecapOutput},
};

use super::dispatch::DispatchResult;
use serde_json::json;

/// 永続化結果。
#[derive(Debug, Clone)]
pub(crate) struct PersistResult {
    pub(crate) job_id: uuid::Uuid,
    pub(crate) genres_stored: usize,
    pub(crate) genres_failed: usize,
}

#[async_trait]
pub(crate) trait PersistStage: Send + Sync {
    async fn persist(&self, job: &JobContext, result: DispatchResult) -> Result<PersistResult>;
}

/// 最終確定物をJSONBフィールドに保存するPersistStage。
#[allow(dead_code)]
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
    async fn persist(&self, job: &JobContext, result: DispatchResult) -> Result<PersistResult> {
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

            let summary_response = match (
                &genre_result.summary_response_id,
                &genre_result.summary_response,
            ) {
                (Some(_), Some(response)) => response,
                (None, _) => {
                    warn!(
                        job_id = %job.job_id,
                        genre = %genre,
                        "genre missing summary response id"
                    );
                    genres_failed += 1;
                    continue;
                }
                (_, None) => {
                    warn!(
                        job_id = %job.job_id,
                        genre = %genre,
                        "genre missing summary payload"
                    );
                    genres_failed += 1;
                    continue;
                }
            };

            let summary_id = genre_result
                .summary_response_id
                .as_ref()
                .expect("checked above")
                .clone();

            let bullet_values = summary_response
                .summary
                .bullets
                .iter()
                .map(|bullet| json!({ "text": bullet, "sources": [] }))
                .collect::<Vec<_>>();
            let bullets_json = serde_json::Value::Array(bullet_values);

            let summary_text = summary_response.summary.bullets.join("\n");
            let body_json = serde_json::to_value(summary_response)
                .context("failed to convert summary response to JSON")?;

            let output = RecapOutput::new(
                job.job_id,
                genre.as_str(),
                summary_id.clone(),
                summary_response.summary.title.clone(),
                summary_text,
                bullets_json,
                body_json,
            );

            let mut persisted_successfully = true;

            if let Err(err) = self.dao.upsert_recap_output(&output).await {
                warn!(
                    job_id = %job.job_id,
                    genre = %genre,
                    error = ?err,
                    "failed to persist recap output"
                );
                persisted_successfully = false;
            }

            let persisted_genre =
                PersistedGenre::new(job.job_id, genre.as_str()).with_response_id(Some(summary_id));
            if let Err(err) = self.dao.upsert_genre(&persisted_genre).await {
                warn!(
                    job_id = %job.job_id,
                    genre = %genre,
                    error = ?err,
                    "failed to persist recap section pointer"
                );
                persisted_successfully = false;
            }

            if persisted_successfully {
                debug!(
                    job_id = %job.job_id,
                    genre = %genre,
                    "genre processed successfully"
                );
                genres_stored += 1;
            } else {
                genres_failed += 1;
            }
        }

        let persist_result = PersistResult {
            job_id: job.job_id,
            genres_stored,
            genres_failed,
        };

        info!(
            job_id = %persist_result.job_id,
            genres_stored = persist_result.genres_stored,
            genres_failed = persist_result.genres_failed,
            "completed persisting final sections"
        );

        Ok(persist_result)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

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
