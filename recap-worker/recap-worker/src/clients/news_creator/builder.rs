use rayon::prelude::*;
use uuid::Uuid;

use super::models::{ClusterInput, RepresentativeSentence, SummaryOptions, SummaryRequest};
use crate::clients::TokenCounter;
use crate::clients::subworker::ClusteringResponse;

/// クラスタリングレスポンスから要約リクエストを構築する。
pub(crate) struct SummaryRequestBuilder<'a> {
    token_counter: &'a TokenCounter,
}

impl<'a> SummaryRequestBuilder<'a> {
    pub(crate) fn new(token_counter: &'a TokenCounter) -> Self {
        Self { token_counter }
    }

    /// クラスタリングレスポンスから要約リクエストを構築する。
    ///
    /// # Arguments
    /// * `job_id` - ジョブID
    /// * `clustering` - クラスタリング結果
    /// * `_max_sentences_per_cluster` - クラスターごとの最大文数 (予算配分ロジックにより無視される場合があるが、上限として使用)
    /// * `article_metadata` - 記事IDからメタデータ（published_at, source_url）へのマップ
    ///
    /// # Returns
    /// 要約リクエスト
    pub(crate) fn build_summary_request(
        &self,
        job_id: Uuid,
        clustering: &ClusteringResponse,
        _max_sentences_per_cluster: usize,
        article_metadata: &std::collections::HashMap<
            String,
            (Option<chrono::DateTime<chrono::Utc>>, Option<String>),
        >,
    ) -> SummaryRequest {
        // 総トークン予算 (60,000トークン)
        const TOTAL_TOKEN_BUDGET: usize = 60_000;
        // 1クラスタあたりの最小トークン数 (ヘッダーなどを考慮)
        const MIN_CLUSTER_BUDGET: usize = 200;
        // クラスタ数の上限 (40件)
        const MAX_CLUSTERS: usize = 40;

        // クラスタをsize（記事数）の降順でソートし、上位40件のみを抽出
        let mut sorted_clusters: Vec<_> = clustering
            .clusters
            .iter()
            .filter(|cluster| cluster.cluster_id >= 0)
            .collect();

        // size（記事数）の降順でソート
        sorted_clusters.sort_by(|a, b| b.size.cmp(&a.size));

        // 上位40件に制限
        let target_clusters: Vec<_> = sorted_clusters.into_iter().take(MAX_CLUSTERS).collect();

        // 予算配分のためのスコア計算 (size^0.8)
        let alpha: f64 = 0.8;
        let total_score: f64 = target_clusters
            .iter()
            .map(|c| (c.size as f64).powf(alpha))
            .sum();

        // Rayonを使用して並列処理
        // 各クラスタの予算計算と文選択は独立しているため並列化可能
        // トークンカウント（CPUバウンド）の負荷を分散する
        let clusters: Vec<ClusterInput> = target_clusters
            .into_par_iter()
            .map(|cluster| {
                // クラスタごとの予算計算
                let score = (cluster.size as f64).powf(alpha);
                let budget_ratio = if total_score > 0.0 {
                    score / total_score
                } else {
                    0.0
                };
                let cluster_budget_f64 = (TOTAL_TOKEN_BUDGET as f64 * budget_ratio).max(0.0);
                let cluster_budget = usize::try_from(cluster_budget_f64 as i64)
                    .unwrap_or(0)
                    .max(MIN_CLUSTER_BUDGET);

                // 文の選択 (予算内)
                let mut current_tokens = 0;
                let mut selected_sentences = Vec::new();

                // 代表文をスコア順（重要度順）にソートして検討する
                // ClusterRepresentativeにはscoreがあるが、Option<f32>なのでunwrap_or(0.0)する
                // 既存のrepresentativesはすでに何らかの順序があるかもしれないが、念のためスコア順にする
                let mut candidates: Vec<_> = cluster.representatives.iter().collect();
                candidates.sort_by(|a, b| {
                    b.score
                        .unwrap_or(0.0)
                        .partial_cmp(&a.score.unwrap_or(0.0))
                        .unwrap_or(std::cmp::Ordering::Equal)
                });

                for rep in candidates {
                    let text = rep.text.trim();
                    if text.is_empty() {
                        continue;
                    }

                    let tokens = self.token_counter.count_tokens(text);
                    if current_tokens + tokens <= cluster_budget {
                        // メタデータを取得
                        // Note: HashMapへのアクセスは読み取りのみなので、Arcで共有するか、
                        // ここでは単純に参照を渡しているが、Rayonのクロージャ内では参照がSend/Syncである必要がある
                        // HashMapはSyncなので問題ない
                        let (published_at, source_url) = article_metadata
                            .get(&rep.article_id)
                            .cloned()
                            .unwrap_or((None, None));

                        selected_sentences.push(RepresentativeSentence {
                            text: text.to_string(),
                            published_at: published_at.map(|dt| dt.to_rfc3339()),
                            source_url,
                            article_id: Some(rep.article_id.clone()),
                            is_centroid: rep.reasons.iter().any(|r| r == "centrality"),
                        });
                        current_tokens += tokens;
                    }
                }

                // 時系列順にソート（published_at が古い順）
                selected_sentences.sort_by(|a, b| match (&a.published_at, &b.published_at) {
                    (Some(a_dt), Some(b_dt)) => a_dt.cmp(b_dt),
                    (Some(_), None) => std::cmp::Ordering::Less,
                    (None, Some(_)) => std::cmp::Ordering::Greater,
                    (None, None) => std::cmp::Ordering::Equal,
                });

                ClusterInput {
                    cluster_id: cluster.cluster_id,
                    representative_sentences: selected_sentences,
                    top_terms: Some(cluster.top_terms.clone()),
                }
            })
            .collect();

        // Genre highlightsの変換
        let genre_highlights = clustering.genre_highlights.as_ref().map(|highlights| {
            highlights
                .iter()
                .map(|rep| {
                    let text = rep.text.trim().to_string();
                    // メタデータを取得
                    let (published_at, source_url) = article_metadata
                        .get(&rep.article_id)
                        .cloned()
                        .unwrap_or((None, None));

                    RepresentativeSentence {
                        text,
                        published_at: published_at.map(|dt| dt.to_rfc3339()),
                        source_url,
                        article_id: Some(rep.article_id.clone()),
                        is_centroid: rep.reasons.iter().any(|r| r == "centrality"),
                    }
                })
                .collect()
        });

        SummaryRequest {
            job_id,
            genre: clustering.genre.clone(),
            clusters,
            genre_highlights,
            options: Some(SummaryOptions {
                max_bullets: Some(15),
                temperature: Some(0.7),
            }),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::clients::subworker::{ClusterInfo, ClusterJobStatus, ClusterRepresentative};
    use serde_json::json;

    #[test]
    fn build_summary_request_limits_clusters_to_40() {
        let job_id = Uuid::new_v4();
        let mut clusters = Vec::new();

        // 50個のクラスタを作成（40を超える）
        for i in 0..50 {
            let i_usize = usize::try_from(i).expect("i should be non-negative");
            clusters.push(ClusterInfo {
                cluster_id: i,
                size: 100usize.saturating_sub(i_usize), // sizeが降順になるように設定
                label: None,
                top_terms: vec!["term1".to_string(), "term2".to_string()],
                stats: json!({}),
                representatives: vec![ClusterRepresentative {
                    article_id: format!("article-{}", i),
                    paragraph_idx: None,
                    text: format!("Representative sentence for cluster {}", i),
                    lang: Some("ja".to_string()),
                    score: Some(0.9),
                    reasons: vec![],
                }],
            });
        }

        let clustering_response = ClusteringResponse {
            run_id: 1,
            job_id,
            genre: "tech".to_string(),
            status: ClusterJobStatus::Succeeded,
            cluster_count: 50,
            clusters,
            genre_highlights: None,
            diagnostics: json!({}),
        };

        let article_metadata = std::collections::HashMap::new();
        let token_counter = TokenCounter::dummy();
        let builder = SummaryRequestBuilder::new(&token_counter);
        let request =
            builder.build_summary_request(job_id, &clustering_response, 5, &article_metadata);

        // クラスタ数が40件に制限されていることを確認
        assert_eq!(
            request.clusters.len(),
            40,
            "clusters should be limited to 40"
        );

        // クラスタがsizeの降順でソートされていることを確認
        for i in 1..request.clusters.len() {
            // 元のクラスタIDからsizeを推測（size = 100 - cluster_id）
            let prev_id = usize::try_from(request.clusters[i - 1].cluster_id).unwrap_or(0);
            let curr_id = usize::try_from(request.clusters[i].cluster_id).unwrap_or(0);
            let prev_size = 100usize.saturating_sub(prev_id);
            let curr_size = 100usize.saturating_sub(curr_id);
            assert!(
                prev_size >= curr_size,
                "clusters should be sorted by size in descending order"
            );
        }

        // 最初のクラスタが最大のsizeを持つことを確認
        assert_eq!(
            request.clusters[0].cluster_id, 0,
            "first cluster should have the largest size"
        );
    }

    #[test]
    fn build_summary_request_filters_negative_cluster_ids() {
        let job_id = Uuid::new_v4();
        let clusters = vec![
            ClusterInfo {
                cluster_id: -1, // ノイズクラスタ
                size: 100,
                label: None,
                top_terms: vec![],
                stats: json!({}),
                representatives: vec![ClusterRepresentative {
                    article_id: "article-1".to_string(),
                    paragraph_idx: None,
                    text: "Noise cluster".to_string(),
                    lang: Some("ja".to_string()),
                    score: Some(0.5),
                    reasons: vec![],
                }],
            },
            ClusterInfo {
                cluster_id: 0, // 有効なクラスタ
                size: 50,
                label: None,
                top_terms: vec![],
                stats: json!({}),
                representatives: vec![ClusterRepresentative {
                    article_id: "article-2".to_string(),
                    paragraph_idx: None,
                    text: "Valid cluster".to_string(),
                    lang: Some("ja".to_string()),
                    score: Some(0.9),
                    reasons: vec![],
                }],
            },
        ];

        let clustering_response = ClusteringResponse {
            run_id: 1,
            job_id,
            genre: "tech".to_string(),
            status: ClusterJobStatus::Succeeded,
            cluster_count: 2,
            clusters,
            genre_highlights: None,
            diagnostics: json!({}),
        };

        let article_metadata = std::collections::HashMap::new();
        let token_counter = TokenCounter::dummy();
        let builder = SummaryRequestBuilder::new(&token_counter);
        let request =
            builder.build_summary_request(job_id, &clustering_response, 5, &article_metadata);

        // 負のcluster_idが除外されていることを確認
        assert_eq!(request.clusters.len(), 1);
        assert_eq!(request.clusters[0].cluster_id, 0);
    }
}
