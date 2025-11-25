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

/// Sanitize title and summary text by removing markdown code blocks
fn sanitize_title(text: &str) -> String {
    text.replace("```json", "")
        .replace("```", "")
        .trim()
        .to_string()
}

/// 永続化結果。
#[derive(Debug, Clone)]
pub(crate) struct PersistResult {
    pub(crate) job_id: uuid::Uuid,
    pub(crate) genres_stored: usize,
    pub(crate) genres_failed: usize,
    /// 証拠不足でスキップされたジャンル数（記事数が閾値未満）
    pub(crate) genres_skipped: usize,
    /// 記事が1件も割り当てられなかったジャンル数
    pub(crate) genres_no_evidence: usize,
    /// 設定された全ジャンル数
    pub(crate) total_genres: usize,
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
#[allow(clippy::too_many_lines)]
impl PersistStage for FinalSectionPersistStage {
    async fn persist(&self, job: &JobContext, result: DispatchResult) -> Result<PersistResult> {
        info!(
            job_id = %job.job_id,
            genre_count = result.genre_results.len(),
            "persisting final sections to database"
        );

        let mut genres_stored = 0;
        let mut genres_failed = 0;
        let mut genres_skipped = 0;
        let mut genres_no_evidence = 0;
        let total_genres = result.all_genres.len();

        for (genre, genre_result) in &result.genre_results {
            // エラーがある場合は分類
            if let Some(error_msg) = &genre_result.error {
                // エラーメッセージから分類
                if error_msg.contains("no evidence") || error_msg.contains("no articles assigned") {
                    // 記事が1件も割り当てられなかった
                    genres_no_evidence += 1;
                } else if error_msg.contains("insufficient documents")
                    || error_msg.contains("expected >=")
                {
                    // 証拠不足でスキップ（記事数が閾値未満）
                    genres_skipped += 1;
                } else {
                    // その他のエラー（クラスタリング失敗、サマリー生成失敗など）
                    warn!(
                        job_id = %job.job_id,
                        genre = %genre,
                        error = ?genre_result.error,
                        "skipping genre with error"
                    );
                    genres_failed += 1;
                }
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

            // Sanitize title to remove markdown code blocks
            let sanitized_title = sanitize_title(&summary_response.summary.title);
            let sanitized_summary = sanitize_title(&summary_text);

            let output = RecapOutput::new(
                job.job_id,
                genre.as_str(),
                summary_id.clone(),
                sanitized_title,
                sanitized_summary,
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
            genres_skipped,
            genres_no_evidence,
            total_genres,
        };

        info!(
            job_id = %persist_result.job_id,
            total_genres = persist_result.total_genres,
            genres_stored = persist_result.genres_stored,
            genres_failed = persist_result.genres_failed,
            genres_skipped = persist_result.genres_skipped,
            genres_no_evidence = persist_result.genres_no_evidence,
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
            genres_skipped: 1,
            genres_no_evidence: 1,
            total_genres: 9,
        };

        assert_eq!(result.genres_stored, 5);
        assert_eq!(result.genres_failed, 2);
        assert_eq!(result.genres_skipped, 1);
        assert_eq!(result.genres_no_evidence, 1);
        assert_eq!(result.total_genres, 9);
    }
}
