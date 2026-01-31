//! Dispatch stage for ML/LLM processing.
//!
//! This module coordinates clustering and summarization for all genres.

mod clustering;
mod memory;
mod summarization;
mod types;

use std::{collections::HashMap, sync::Arc};

use anyhow::Result;
use async_trait::async_trait;
use tokio::sync::Semaphore;
use tracing::info;

use crate::{
    clients::{subworker::SubworkerClient, NewsCreatorClient},
    config::Config,
    scheduler::JobContext,
    store::dao::RecapDao,
};

use super::evidence::EvidenceBundle;

// Re-exports
pub(crate) use clustering::ClusteringOps;
pub(crate) use memory::read_process_memory_kb;
pub(crate) use summarization::SummarizationOps;
pub(crate) use types::{DispatchResult, DispatchResultLightweight, GenreResult};

/// ディスパッチステージトレイト。
#[async_trait]
pub(crate) trait DispatchStage: Send + Sync {
    async fn dispatch(&self, job: &JobContext, evidence: EvidenceBundle) -> Result<DispatchResult>;
}

/// SubworkerとNews-Creatorを連携させるディスパッチステージ。
///
/// 各ジャンルごとに：
/// 1. SubworkerでML処理（クラスタリング）
/// 2. News-CreatorでLLM処理（日本語要約生成）
#[derive(Clone)]
pub(crate) struct MlLlmDispatchStage {
    subworker_client: Arc<SubworkerClient>,
    news_creator_client: Arc<NewsCreatorClient>,
    dao: Arc<dyn RecapDao>,
    concurrency_semaphore: Arc<Semaphore>,
    config: Arc<Config>,
}

impl MlLlmDispatchStage {
    pub(crate) fn new(
        subworker_client: Arc<SubworkerClient>,
        news_creator_client: Arc<NewsCreatorClient>,
        dao: Arc<dyn RecapDao>,
        max_concurrency: usize,
        config: Arc<Config>,
    ) -> Self {
        Self {
            subworker_client,
            news_creator_client,
            dao,
            concurrency_semaphore: Arc::new(Semaphore::new(max_concurrency.max(1))),
            config,
        }
    }

    fn clustering_ops(&self) -> ClusteringOps<'_> {
        ClusteringOps {
            subworker_client: &self.subworker_client,
            dao: &self.dao,
            config: &self.config,
            concurrency_semaphore: &self.concurrency_semaphore,
        }
    }

    fn summarization_ops(&self) -> SummarizationOps<'_> {
        SummarizationOps {
            news_creator_client: &self.news_creator_client,
            dao: &self.dao,
            config: &self.config,
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

        // 設定された全ジャンルを取得
        let all_genres = self.config.recap_genres().to_vec();

        if genre_count == 0 {
            return Ok(DispatchResult {
                job_id: job.job_id,
                genre_results: HashMap::new(),
                success_count: 0,
                failure_count: 0,
                all_genres,
            });
        }

        // Wrap evidence in Arc once to share across tasks without deep cloning
        let evidence_arc = Arc::new(evidence);

        // Phase 1: 全ジャンルを並列でクラスタリング
        let clustering_results = self
            .clustering_ops()
            .cluster_all_genres(job, &genres, evidence_arc.clone())
            .await;

        // Phase 2: サマリー生成はバッチ API で実行（1回の HTTP 呼び出し）
        let mut genre_results = self
            .summarization_ops()
            .generate_summaries_with_batch(job, clustering_results)
            .await;

        // 証拠がないジャンル（設定されているが処理されていない）を追加
        let processed_genres: std::collections::HashSet<String> =
            genre_results.keys().cloned().collect();
        for genre in &all_genres {
            if !processed_genres.contains(genre) {
                // 証拠がないジャンルを結果に追加（エラーとして記録）
                genre_results.insert(
                    genre.clone(),
                    GenreResult {
                        genre: genre.clone(),
                        clustering_response: None,
                        summary_response_id: None,
                        summary_response: None,
                        error: Some("no evidence (no articles assigned)".to_string()),
                    },
                );
            }
        }

        let success_count = genre_results
            .values()
            .filter(|result| result.error.is_none())
            .count();
        let failure_count = genre_results.len() - success_count;

        let dispatch_result = DispatchResult {
            job_id: job.job_id,
            genre_results,
            success_count,
            failure_count,
            all_genres,
        };

        if let Some((rss_kb, peak_kb)) = read_process_memory_kb() {
            info!(
                job_id = %dispatch_result.job_id,
                success_count = dispatch_result.success_count,
                failure_count = dispatch_result.failure_count,
                genre_count = dispatch_result.genre_results.len(),
                memory_rss_kb = rss_kb,
                memory_peak_kb = peak_kb,
                "completed ML/LLM dispatch"
            );
        } else {
            info!(
                job_id = %dispatch_result.job_id,
                success_count = dispatch_result.success_count,
                failure_count = dispatch_result.failure_count,
                genre_count = dispatch_result.genre_results.len(),
                "completed ML/LLM dispatch"
            );
        }

        Ok(dispatch_result)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::clients::news_creator::models::{
        ClusterInput, RepresentativeSentence, SummaryRequest,
    };
    use uuid::Uuid;

    #[test]
    fn genre_result_tracks_success_and_failure() {
        let success = GenreResult {
            genre: "ai".to_string(),
            clustering_response: None,
            summary_response_id: None,
            summary_response: None,
            error: None,
        };

        assert!(success.error.is_none());

        let failure = GenreResult {
            genre: "tech".to_string(),
            clustering_response: None,
            summary_response_id: None,
            summary_response: None,
            error: Some("Failed".to_string()),
        };

        assert!(failure.error.is_some());
    }

    /// Test: Clusters with empty representative_sentences should be filtered out
    #[test]
    fn test_filter_empty_representative_sentences() {
        let mut request = SummaryRequest {
            job_id: Uuid::new_v4(),
            genre: "tech".to_string(),
            clusters: vec![
                ClusterInput {
                    cluster_id: 0,
                    representative_sentences: vec![RepresentativeSentence {
                        text: "Valid sentence".to_string(),
                        published_at: None,
                        source_url: None,
                        article_id: Some("article-1".to_string()),
                        is_centroid: false,
                    }],
                    top_terms: None,
                },
                ClusterInput {
                    cluster_id: 1,
                    representative_sentences: vec![], // Empty - should be filtered
                    top_terms: None,
                },
                ClusterInput {
                    cluster_id: 2,
                    representative_sentences: vec![RepresentativeSentence {
                        text: "Another valid sentence".to_string(),
                        published_at: None,
                        source_url: None,
                        article_id: Some("article-2".to_string()),
                        is_centroid: false,
                    }],
                    top_terms: None,
                },
            ],
            genre_highlights: None,
            options: None,
        };

        let original_count = request.clusters.len();
        request
            .clusters
            .retain(|cluster| !cluster.representative_sentences.is_empty());

        assert_eq!(original_count, 3);
        assert_eq!(request.clusters.len(), 2);
        assert_eq!(request.clusters[0].cluster_id, 0);
        assert_eq!(request.clusters[1].cluster_id, 2);
    }

    /// Test: All clusters empty should result in empty clusters vec
    #[test]
    fn test_all_clusters_empty_representative_sentences() {
        let mut request = SummaryRequest {
            job_id: Uuid::new_v4(),
            genre: "tech".to_string(),
            clusters: vec![
                ClusterInput {
                    cluster_id: 0,
                    representative_sentences: vec![], // Empty
                    top_terms: None,
                },
                ClusterInput {
                    cluster_id: 1,
                    representative_sentences: vec![], // Empty
                    top_terms: None,
                },
            ],
            genre_highlights: None,
            options: None,
        };

        request
            .clusters
            .retain(|cluster| !cluster.representative_sentences.is_empty());

        assert!(
            request.clusters.is_empty(),
            "All empty clusters should be filtered out"
        );
    }
}
