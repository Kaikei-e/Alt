use std::collections::HashMap;
use std::sync::Arc;

use anyhow::{Context, Result};
use async_trait::async_trait;
use tracing::{debug, info, warn};
use uuid::Uuid;

use crate::{
    clients::{
        NewsCreatorClient,
        subworker::{ClusteringResponse, SubworkerClient},
    },
    scheduler::JobContext,
};

use super::evidence::EvidenceBundle;

/// ディスパッチ結果。
#[derive(Debug, Clone)]
pub(crate) struct DispatchResult {
    pub(crate) job_id: Uuid,
    pub(crate) genre_results: HashMap<String, GenreResult>,
    pub(crate) success_count: usize,
    pub(crate) failure_count: usize,
}

/// ジャンル別の処理結果。
#[derive(Debug, Clone)]
pub(crate) struct GenreResult {
    pub(crate) genre: String,
    pub(crate) clustering_response: Option<ClusteringResponse>,
    pub(crate) summary_response_id: Option<String>,
    pub(crate) error: Option<String>,
}

#[async_trait]
pub(crate) trait DispatchStage: Send + Sync {
    async fn dispatch(&self, job: &JobContext, evidence: EvidenceBundle) -> Result<DispatchResult>;
}

/// SubworkerとNews-Creatorを連携させるディスパッチステージ。
///
/// 各ジャンルごとに：
/// 1. SubworkerでML処理（クラスタリング）
/// 2. News-CreatorでLLM処理（日本語要約生成）
#[derive(Debug, Clone)]
pub(crate) struct MlLlmDispatchStage {
    subworker_client: Arc<SubworkerClient>,
    news_creator_client: Arc<NewsCreatorClient>,
}

impl MlLlmDispatchStage {
    pub(crate) fn new(
        subworker_client: Arc<SubworkerClient>,
        news_creator_client: Arc<NewsCreatorClient>,
    ) -> Self {
        Self {
            subworker_client,
            news_creator_client,
        }
    }

    /// 単一ジャンルの処理を実行する。
    async fn process_genre(
        &self,
        job_id: Uuid,
        genre: &str,
        evidence: &super::evidence::EvidenceCorpus,
    ) -> GenreResult {
        debug!(
            job_id = %job_id,
            genre = %genre,
            article_count = evidence.articles.len(),
            "processing genre"
        );

        // Step 1: Subworkerでクラスタリング
        let clustering_result = self.subworker_client.cluster_corpus(job_id, evidence).await;

        let clustering_response = match clustering_result {
            Ok(response) => {
                info!(
                    job_id = %job_id,
                    genre = %genre,
                    cluster_count = response.clusters.len(),
                    "clustering completed successfully"
                );
                Some(response)
            }
            Err(e) => {
                warn!(
                    job_id = %job_id,
                    genre = %genre,
                    error = ?e,
                    "clustering failed"
                );
                return GenreResult {
                    genre: genre.to_string(),
                    clustering_response: None,
                    summary_response_id: None,
                    error: Some(format!("Clustering failed: {}", e)),
                };
            }
        };

        // Step 2: News-Creatorで日本語要約生成
        let clustering_response_for_summary = clustering_response.clone().unwrap();

        let summary_request = NewsCreatorClient::build_summary_request(
            job_id,
            &clustering_response_for_summary,
            5, // 最大5文/クラスター
        );

        let summary_result = self
            .news_creator_client
            .generate_summary(&summary_request)
            .await;

        match summary_result {
            Ok(summary_response) => {
                info!(
                    job_id = %job_id,
                    genre = %genre,
                    bullet_count = summary_response.summary.bullets.len(),
                    "summary generation completed successfully"
                );
                GenreResult {
                    genre: genre.to_string(),
                    clustering_response,
                    summary_response_id: Some(format!(
                        "{}-{}",
                        summary_response.job_id, summary_response.genre
                    )),
                    error: None,
                }
            }
            Err(e) => {
                warn!(
                    job_id = %job_id,
                    genre = %genre,
                    error = ?e,
                    "summary generation failed"
                );
                GenreResult {
                    genre: genre.to_string(),
                    clustering_response,
                    summary_response_id: None,
                    error: Some(format!("Summary generation failed: {}", e)),
                }
            }
        }
    }
}

#[async_trait]
impl DispatchStage for MlLlmDispatchStage {
    async fn dispatch(&self, job: &JobContext, evidence: EvidenceBundle) -> Result<DispatchResult> {
        let genres = evidence.genres();
        let genre_count = genres.len();

        info!(
            job_id = %job.job_id,
            genre_count = genre_count,
            "starting ML/LLM dispatch for all genres"
        );

        if genre_count == 0 {
            return Ok(DispatchResult {
                job_id: job.job_id,
                genre_results: HashMap::new(),
                success_count: 0,
                failure_count: 0,
            });
        }

        // 各ジャンルを並列処理
        let mut tasks = Vec::new();

        for genre in &genres {
            let corpus = evidence
                .get_corpus(genre)
                .context(format!("corpus not found for genre: {}", genre))?
                .clone();

            let self_clone = self.clone();
            let job_id = job.job_id;
            let genre_clone = genre.clone();

            let task = tokio::spawn(async move {
                let result = self_clone
                    .process_genre(job_id, &genre_clone, &corpus)
                    .await;
                (genre_clone, result)
            });

            tasks.push(task);
        }

        // すべてのタスクを待機
        let results = futures::future::join_all(tasks).await;

        let mut genre_results = HashMap::new();
        let mut success_count = 0;
        let mut failure_count = 0;

        for result in results {
            match result {
                Ok((genre, genre_result)) => {
                    debug!(
                        job_id = %job.job_id,
                        genre = %genre_result.genre,
                        has_clustering = genre_result.clustering_response.is_some(),
                        has_summary = genre_result.summary_response_id.is_some(),
                        has_error = genre_result.error.is_some(),
                        "processed genre result"
                    );

                    if genre_result.error.is_none() {
                        success_count += 1;
                    } else {
                        failure_count += 1;
                    }
                    genre_results.insert(genre, genre_result);
                }
                Err(e) => {
                    warn!(error = ?e, "genre processing task failed");
                    failure_count += 1;
                }
            }
        }

        let dispatch_result = DispatchResult {
            job_id: job.job_id,
            genre_results,
            success_count,
            failure_count,
        };

        info!(
            job_id = %dispatch_result.job_id,
            success_count = dispatch_result.success_count,
            failure_count = dispatch_result.failure_count,
            genre_count = dispatch_result.genre_results.len(),
            "completed ML/LLM dispatch"
        );

        Ok(dispatch_result)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn genre_result_tracks_success_and_failure() {
        let success = GenreResult {
            genre: "ai".to_string(),
            clustering_response: None,
            summary_response_id: None,
            error: None,
        };

        assert!(success.error.is_none());

        let failure = GenreResult {
            genre: "tech".to_string(),
            clustering_response: None,
            summary_response_id: None,
            error: Some("Failed".to_string()),
        };

        assert!(failure.error.is_some());
    }
}
