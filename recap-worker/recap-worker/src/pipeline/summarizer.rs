/// Summarizer - クラスタリング結果からLLM入力を最適化し、結果を保存するモジュール。
///
/// このモジュールは以下を担当：
/// 1. クラスタリング結果から重要な文を選択
/// 2. LLM入力の最適化（トークン数制限、冗長性削減）
/// 3. 生成された要約をDBに保存
use std::sync::Arc;

use anyhow::{Context, Result};
use tracing::{debug, info};
use uuid::Uuid;

use crate::{
    clients::{
        news_creator::{NewsCreatorClient, SummaryRequest, SummaryResponse},
        subworker::ClusteringResponse,
    },
    store::{dao::RecapDao, models::RecapFinalSection},
};

/// Summarizer - クラスタリング結果から要約を生成し、保存する。
#[derive(Debug, Clone)]
pub(crate) struct Summarizer {
    dao: Arc<RecapDao>,
    max_sentences_per_cluster: usize,
    max_clusters_per_genre: usize,
}

impl Summarizer {
    /// 新しいSummarizerを作成する。
    ///
    /// # Arguments
    /// * `dao` - データベースアクセスオブジェクト
    /// * `max_sentences_per_cluster` - クラスターごとの最大文数
    /// * `max_clusters_per_genre` - ジャンルごとの最大クラスター数
    pub(crate) fn new(
        dao: Arc<RecapDao>,
        max_sentences_per_cluster: usize,
        max_clusters_per_genre: usize,
    ) -> Self {
        Self {
            dao,
            max_sentences_per_cluster,
            max_clusters_per_genre,
        }
    }

    /// デフォルトパラメータで作成する。
    pub(crate) fn with_defaults(dao: Arc<RecapDao>) -> Self {
        Self::new(dao, 5, 20)
    }

    /// クラスタリング結果から要約リクエストを構築する。
    ///
    /// # Arguments
    /// * `job_id` - ジョブID
    /// * `clustering` - クラスタリング結果
    ///
    /// # Returns
    /// 最適化された要約リクエスト
    pub(crate) fn build_summary_request(
        &self,
        job_id: Uuid,
        clustering: &ClusteringResponse,
    ) -> SummaryRequest {
        // クラスターを重要度順にソート（コヒーレンススコアや文数で）
        let mut clusters = clustering.clusters.clone();
        clusters.sort_by(|a, b| {
            // コヒーレンススコアがある場合はそれで、なければ文数で
            match (a.coherence_score, b.coherence_score) {
                (Some(score_a), Some(score_b)) => score_b
                    .partial_cmp(&score_a)
                    .unwrap_or(std::cmp::Ordering::Equal),
                _ => b.sentences.len().cmp(&a.sentences.len()),
            }
        });

        // 上位N個のクラスターのみ使用
        let selected_clusters: Vec<_> = clusters
            .into_iter()
            .take(self.max_clusters_per_genre)
            .collect();

        debug!(
            job_id = %job_id,
            genre = %clustering.genre,
            selected_cluster_count = selected_clusters.len(),
            "selected top clusters for summarization"
        );

        NewsCreatorClient::build_summary_request(
            job_id,
            &ClusteringResponse {
                job_id: clustering.job_id,
                genre: clustering.genre.clone(),
                clusters: selected_clusters,
                metadata: clustering.metadata.clone(),
            },
            self.max_sentences_per_cluster,
        )
    }

    /// 要約レスポンスをDBに保存する。
    ///
    /// # Arguments
    /// * `job_id` - ジョブID
    /// * `summary` - 要約レスポンス
    ///
    /// # Returns
    /// 保存された行ID
    pub(crate) async fn save_summary(
        &self,
        job_id: Uuid,
        summary: &SummaryResponse,
    ) -> Result<i64> {
        info!(
            job_id = %job_id,
            genre = %summary.genre,
            bullet_count = summary.summary.bullets.len(),
            "saving summary to database"
        );

        let section = RecapFinalSection::new(
            job_id,
            summary.genre.clone(),
            summary.summary.title.clone(),
            summary.summary.bullets.clone(),
            summary.metadata.model.clone(),
        );

        let row_id = self
            .dao
            .save_final_section(&section)
            .await
            .context("failed to save final section")?;

        debug!(
            job_id = %job_id,
            genre = %summary.genre,
            row_id = row_id,
            "summary saved successfully"
        );

        Ok(row_id)
    }
}

/// 最終セクションのモデル（DAOに追加）。
#[derive(Debug, Clone)]
pub(crate) struct RecapFinalSection {
    pub(crate) job_id: Uuid,
    pub(crate) genre: String,
    pub(crate) title_ja: String,
    pub(crate) bullets_ja: Vec<String>,
    pub(crate) model_name: String,
}

impl RecapFinalSection {
    pub(crate) fn new(
        job_id: Uuid,
        genre: String,
        title_ja: String,
        bullets_ja: Vec<String>,
        model_name: String,
    ) -> Self {
        Self {
            job_id,
            genre,
            title_ja,
            bullets_ja,
            model_name,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::clients::subworker::{Cluster, ClusterSentence, ClusteringMetadata};

    fn create_cluster(id: usize, sentence_count: usize, coherence: Option<f64>) -> Cluster {
        let sentences = (0..sentence_count)
            .map(|i| ClusterSentence {
                sentence_id: i,
                text: format!("Sentence {} in cluster {}", i, id),
                source_article_id: format!("art-{}", i),
                embedding: None,
            })
            .collect();

        Cluster {
            cluster_id: id,
            sentences,
            centroid: None,
            top_terms: vec!["term1".to_string(), "term2".to_string()],
            coherence_score: coherence,
        }
    }

    #[test]
    fn summarizer_selects_top_clusters_by_coherence() {
        let dao = Arc::new(RecapDao::new_mock()); // モックDAO（テスト用）
        let summarizer = Summarizer::new(dao, 5, 3);

        let job_id = Uuid::new_v4();
        let clustering = ClusteringResponse {
            job_id,
            genre: "ai".to_string(),
            clusters: vec![
                create_cluster(0, 10, Some(0.5)),
                create_cluster(1, 5, Some(0.9)),  // 最高スコア
                create_cluster(2, 8, Some(0.7)),  // 2番目
                create_cluster(3, 12, Some(0.6)), // 3番目
                create_cluster(4, 3, Some(0.4)),
            ],
            metadata: ClusteringMetadata {
                total_sentences: 38,
                cluster_count: 5,
                processing_time_ms: Some(1000),
            },
        };

        let request = summarizer.build_summary_request(job_id, &clustering);

        // 上位3つのクラスターが選択される
        assert_eq!(request.clusters.len(), 3);
        // 最高スコアのクラスターが最初に来る
        assert_eq!(request.clusters[0].cluster_id, 1);
    }

    #[test]
    fn summarizer_respects_max_clusters_limit() {
        let dao = Arc::new(RecapDao::new_mock());
        let summarizer = Summarizer::new(dao, 5, 2); // 最大2クラスター

        let job_id = Uuid::new_v4();
        let clustering = ClusteringResponse {
            job_id,
            genre: "tech".to_string(),
            clusters: vec![
                create_cluster(0, 10, Some(0.8)),
                create_cluster(1, 5, Some(0.9)),
                create_cluster(2, 8, Some(0.7)),
            ],
            metadata: ClusteringMetadata {
                total_sentences: 23,
                cluster_count: 3,
                processing_time_ms: Some(500),
            },
        };

        let request = summarizer.build_summary_request(job_id, &clustering);

        // 最大2クラスターまで
        assert_eq!(request.clusters.len(), 2);
    }

    #[test]
    fn summarizer_falls_back_to_sentence_count() {
        let dao = Arc::new(RecapDao::new_mock());
        let summarizer = Summarizer::new(dao, 5, 10);

        let job_id = Uuid::new_v4();
        let clustering = ClusteringResponse {
            job_id,
            genre: "science".to_string(),
            clusters: vec![
                create_cluster(0, 10, None), // コヒーレンススコアなし
                create_cluster(1, 15, None), // 最も多くの文
                create_cluster(2, 5, None),
            ],
            metadata: ClusteringMetadata {
                total_sentences: 30,
                cluster_count: 3,
                processing_time_ms: Some(800),
            },
        };

        let request = summarizer.build_summary_request(job_id, &clustering);

        // 文数でソートされ、最も多いクラスターが最初
        assert_eq!(request.clusters[0].cluster_id, 1);
    }
}

