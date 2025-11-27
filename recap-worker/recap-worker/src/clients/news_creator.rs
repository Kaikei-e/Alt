use std::time::Duration;

use anyhow::{Context, Result, anyhow};
use reqwest::{Client, Url};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use tracing::{debug, warn};
use uuid::Uuid;

use crate::clients::subworker::ClusteringResponse;
use crate::schema::{news_creator::SUMMARY_RESPONSE_SCHEMA, validate_json};

const MAX_ERROR_MESSAGE_LENGTH: usize = 500;

/// エラーメッセージを要約して切り詰める。
fn truncate_error_message(msg: &str) -> String {
    let char_count = msg.chars().count();
    if char_count <= MAX_ERROR_MESSAGE_LENGTH {
        return msg.to_string();
    }
    let truncated: String = msg.chars().take(MAX_ERROR_MESSAGE_LENGTH).collect();
    format!("{truncated}... (truncated, {char_count} chars)")
}

#[derive(Debug, Clone)]
pub(crate) struct NewsCreatorClient {
    client: Client,
    base_url: Url,
    summary_timeout: Duration,
}

impl NewsCreatorClient {
    pub(crate) fn new(base_url: impl Into<String>, summary_timeout: Duration) -> Result<Self> {
        let client = Client::builder()
            .timeout(Duration::from_secs(5))
            .build()
            .context("failed to build news-creator client")?;

        let base_url = Url::parse(&base_url.into()).context("invalid news-creator base URL")?;

        Ok(Self {
            client,
            base_url,
            summary_timeout,
        })
    }

    pub(crate) async fn health_check(&self) -> Result<()> {
        let url = self
            .base_url
            .join("health")
            .context("failed to build news-creator health URL")?;

        self.client
            .get(url)
            .send()
            .await
            .context("news-creator health request failed")?
            .error_for_status()
            .context("news-creator health endpoint returned error status")?;

        Ok(())
    }

    /// 旧バージョンの要約メソッド。
    #[allow(dead_code)]
    pub(crate) async fn summarize(&self, payload: impl Serialize) -> Result<NewsCreatorSummary> {
        let url = self
            .base_url
            .join("v1/recap/summarize")
            .context("failed to build news-creator summarize URL")?;

        let response = self
            .client
            .post(url)
            .json(&payload)
            .timeout(Duration::from_secs(60))
            .send()
            .await
            .context("news-creator summarize request failed")?
            .error_for_status()
            .context("news-creator summarize endpoint returned error status")?;

        response
            .json::<NewsCreatorSummary>()
            .await
            .context("failed to deserialize news-creator response")
    }

    /// クラスタリング結果から日本語要約を生成する。
    ///
    /// # Arguments
    /// * `request` - 要約リクエスト
    ///
    /// # Returns
    /// 日本語要約レスポンス（JSON Schema検証済み）
    pub(crate) async fn generate_summary(
        &self,
        request: &SummaryRequest,
    ) -> Result<SummaryResponse> {
        let url = self
            .base_url
            .join("v1/summary/generate")
            .context("failed to build summary generation URL")?;

        debug!(
            job_id = %request.job_id,
            genre = %request.genre,
            cluster_count = request.clusters.len(),
            "sending summary generation request to news-creator"
        );

        let response = self
            .client
            .post(url)
            .json(request)
            .header("X-Job-ID", request.job_id.to_string())
            .timeout(self.summary_timeout)
            .send()
            .await
            .context("summary generation request failed")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            let truncated_body = truncate_error_message(&body);
            return Err(anyhow!(
                "summary generation endpoint returned error status {status}: {truncated_body}"
            ));
        }

        // レスポンスをJSONとして取得
        let response_json: Value = response
            .json()
            .await
            .context("failed to deserialize summary response as JSON")?;

        // JSON Schemaで検証
        let validation = validate_json(&SUMMARY_RESPONSE_SCHEMA, &response_json);
        if !validation.valid {
            warn!(
                job_id = %request.job_id,
                genre = %request.genre,
                errors = ?validation.errors,
                "summary response failed JSON Schema validation"
            );
            return Err(anyhow!(
                "summary response validation failed: {:?}",
                validation.errors
            ));
        }

        debug!(
            job_id = %request.job_id,
            genre = %request.genre,
            "summary response passed JSON Schema validation"
        );

        // 検証済みのJSONを構造体にデシリアライズ
        serde_json::from_value(response_json)
            .context("failed to deserialize validated summary response")
    }

    /// クラスタリングレスポンスから要約リクエストを構築する。
    ///
    /// # Arguments
    /// * `job_id` - ジョブID
    /// * `clustering` - クラスタリング結果
    /// * `max_sentences_per_cluster` - クラスターごとの最大文数
    /// * `article_metadata` - 記事IDからメタデータ（published_at, source_url）へのマップ
    ///
    /// # Returns
    /// 要約リクエスト
    ///
    /// # Note
    /// クラスタ数は上位40件に制限されます。これは以下の技術的根拠に基づきます：
    /// - 4BパラメータLLM（Context 8k程度）のコンテキストウィンドウ制約
    /// - 1クラスタ平均5文（約200トークン）× 40件 = 8,000トークン
    /// - システムプロンプトや出力分を含め、8k-12kコンテキストで安全に処理できる限界値
    /// - トピック分布のZipf則（ロングテール）により、下位クラスタはノイズである可能性が高い
    pub(crate) fn build_summary_request(
        job_id: Uuid,
        clustering: &ClusteringResponse,
        max_sentences_per_cluster: usize,
        article_metadata: &std::collections::HashMap<
            String,
            (Option<chrono::DateTime<chrono::Utc>>, Option<String>),
        >,
    ) -> SummaryRequest {
        // クラスタをsize（記事数）の降順でソートし、上位40件のみを抽出
        const MAX_CLUSTERS: usize = 40;
        let mut sorted_clusters: Vec<_> = clustering
            .clusters
            .iter()
            .filter(|cluster| cluster.cluster_id >= 0)
            .collect();

        // size（記事数）の降順でソート
        sorted_clusters.sort_by(|a, b| b.size.cmp(&a.size));

        // 上位40件に制限
        let clusters: Vec<ClusterInput> = sorted_clusters
            .into_iter()
            .take(MAX_CLUSTERS)
            .map(|cluster| {
                // 各クラスターから代表的な文を選択（最大N文）
                let mut representative_sentences: Vec<RepresentativeSentence> = cluster
                    .representatives
                    .iter()
                    .take(max_sentences_per_cluster)
                    .filter_map(|rep| {
                        let text = rep.text.trim();
                        if text.is_empty() {
                            None
                        } else {
                            // メタデータを取得
                            let (published_at, source_url) = article_metadata
                                .get(&rep.article_id)
                                .cloned()
                                .unwrap_or((None, None));

                            Some(RepresentativeSentence {
                                text: text.to_string(),
                                published_at: published_at.map(|dt| dt.to_rfc3339()),
                                source_url,
                                article_id: Some(rep.article_id.clone()),
                            })
                        }
                    })
                    .collect();

                // 時系列順にソート（published_at が古い順）
                representative_sentences.sort_by(|a, b| {
                    match (&a.published_at, &b.published_at) {
                        (Some(a_dt), Some(b_dt)) => {
                            // RFC3339形式の文字列を比較
                            a_dt.cmp(b_dt)
                        }
                        (Some(_), None) => std::cmp::Ordering::Less,
                        (None, Some(_)) => std::cmp::Ordering::Greater,
                        (None, None) => std::cmp::Ordering::Equal,
                    }
                });

                ClusterInput {
                    cluster_id: cluster.cluster_id,
                    representative_sentences,
                    top_terms: Some(cluster.top_terms.clone()),
                }
            })
            .collect();

        SummaryRequest {
            job_id,
            genre: clustering.genre.clone(),
            clusters,
            options: Some(SummaryOptions {
                max_bullets: Some(15),
                temperature: Some(0.7),
            }),
        }
    }

    /// ジャンルタイブレーク用のLLM推論を実行する（後方互換性のため保持）。
    #[allow(dead_code)]
    pub(crate) async fn tie_break_genre(
        &self,
        request: &GenreTieBreakRequest,
    ) -> Result<GenreTieBreakResponse> {
        let url = self
            .base_url
            .join("v1/genre/tie-break")
            .context("failed to build genre tie-break URL")?;

        debug!(
            job_id = %request.job_id,
            article_id = %request.article_id,
            candidate_count = request.candidates.len(),
            "sending genre tie-break request"
        );

        let response = self
            .client
            .post(url)
            .json(request)
            .timeout(Duration::from_secs(30))
            .send()
            .await
            .context("genre tie-break request failed")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(anyhow!(
                "genre tie-break endpoint returned error status {status}: {body}"
            ));
        }

        response
            .json::<GenreTieBreakResponse>()
            .await
            .context("failed to deserialize genre tie-break response")
    }
}

/// LLMタイブレークに渡す候補（後方互換性のため保持）。
#[allow(dead_code)]
#[derive(Debug, Clone, Serialize)]
pub(crate) struct GenreTieBreakCandidate {
    pub(crate) name: String,
    pub(crate) score: f32,
    pub(crate) keyword_support: usize,
    pub(crate) classifier_confidence: f32,
}

/// LLMタイブレークリクエスト（後方互換性のため保持）。
#[allow(dead_code)]
#[derive(Debug, Clone, Serialize)]
pub(crate) struct GenreTieBreakRequest {
    pub(crate) job_id: Uuid,
    pub(crate) article_id: String,
    pub(crate) language: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) body_preview: Option<String>,
    pub(crate) candidates: Vec<GenreTieBreakCandidate>,
    pub(crate) tags: Vec<TagSignalPayload>,
}

/// LLMに渡すタグ要約（後方互換性のため保持）。
#[allow(dead_code)]
#[derive(Debug, Clone, Serialize)]
pub(crate) struct TagSignalPayload {
    pub(crate) label: String,
    pub(crate) confidence: f32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) source: Option<String>,
}

/// LLMタイブレーク応答（後方互換性のため保持）。
#[allow(dead_code)]
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct GenreTieBreakResponse {
    pub(crate) genre: String,
    pub(crate) confidence: f32,
    #[serde(default)]
    pub(crate) trace_id: Option<String>,
}

/// 旧バージョンの要約レスポンス。
#[allow(dead_code)]
#[derive(Debug, Deserialize)]
pub(crate) struct NewsCreatorSummary {
    pub(crate) response_id: String,
}

/// 日本語要約リクエスト。
#[derive(Debug, Clone, Serialize)]
pub(crate) struct SummaryRequest {
    pub(crate) job_id: Uuid,
    pub(crate) genre: String,
    pub(crate) clusters: Vec<ClusterInput>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) options: Option<SummaryOptions>,
}

/// 代表文のメタデータ。
#[derive(Debug, Clone, Serialize)]
pub(crate) struct RepresentativeSentence {
    pub(crate) text: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) published_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) source_url: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) article_id: Option<String>,
}

/// クラスター入力データ。
#[derive(Debug, Clone, Serialize)]
pub(crate) struct ClusterInput {
    pub(crate) cluster_id: i32,
    pub(crate) representative_sentences: Vec<RepresentativeSentence>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) top_terms: Option<Vec<String>>,
}

/// 要約生成オプション。
#[derive(Debug, Clone, Serialize)]
pub(crate) struct SummaryOptions {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) max_bullets: Option<usize>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) temperature: Option<f64>,
}

/// 日本語要約レスポンス。
#[allow(dead_code)]
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct SummaryResponse {
    pub(crate) job_id: Uuid,
    pub(crate) genre: String,
    pub(crate) summary: Summary,
    metadata: SummaryMetadata,
}

/// 要約内容。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct Summary {
    pub(crate) title: String,
    pub(crate) bullets: Vec<String>,
    pub(crate) language: String,
}

/// 要約メタデータ。
#[allow(dead_code)]
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct SummaryMetadata {
    pub(crate) model: String,
    #[serde(default)]
    temperature: Option<f64>,
    #[serde(default)]
    prompt_tokens: Option<usize>,
    #[serde(default)]
    completion_tokens: Option<usize>,
    #[serde(default)]
    processing_time_ms: Option<usize>,
}

#[cfg(test)]
mod tests {
    use super::*;
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    #[tokio::test]
    async fn health_check_succeeds_on_200() {
        let server = MockServer::start().await;
        Mock::given(method("GET"))
            .and(path("/health"))
            .respond_with(ResponseTemplate::new(200))
            .mount(&server)
            .await;

        let client = NewsCreatorClient::new(server.uri(), Duration::from_secs(600))
            .expect("client should build");

        client
            .health_check()
            .await
            .expect("health check should succeed");
    }

    #[tokio::test]
    async fn health_check_fails_on_error_status() {
        let server = MockServer::start().await;
        Mock::given(method("GET"))
            .and(path("/health"))
            .respond_with(ResponseTemplate::new(503))
            .mount(&server)
            .await;

        let client = NewsCreatorClient::new(server.uri(), Duration::from_secs(600))
            .expect("client should build");

        let error = client.health_check().await.expect_err("should fail");
        assert!(error.to_string().contains("error status"));
    }

    #[tokio::test]
    async fn summarize_parses_response() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/v1/recap/summarize"))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
                "response_id": "resp-123"
            })))
            .mount(&server)
            .await;

        let client = NewsCreatorClient::new(server.uri(), Duration::from_secs(600))
            .expect("client should build");
        let summary = client
            .summarize(&serde_json::json!({"job_id": "00000000-0000-0000-0000-000000000000"}))
            .await
            .expect("summarize succeeds");

        assert_eq!(summary.response_id, "resp-123");
    }

    #[test]
    fn build_summary_request_limits_clusters_to_40() {
        use crate::clients::subworker::{ClusterInfo, ClusterJobStatus, ClusterRepresentative};
        use serde_json::json;

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
            diagnostics: json!({}),
        };

        let article_metadata = std::collections::HashMap::new();
        let request = NewsCreatorClient::build_summary_request(
            job_id,
            &clustering_response,
            5,
            &article_metadata,
        );

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
        use crate::clients::subworker::{ClusterInfo, ClusterJobStatus, ClusterRepresentative};
        use serde_json::json;

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
            diagnostics: json!({}),
        };

        let article_metadata = std::collections::HashMap::new();
        let request = NewsCreatorClient::build_summary_request(
            job_id,
            &clustering_response,
            5,
            &article_metadata,
        );

        // 負のcluster_idが除外されていることを確認
        assert_eq!(request.clusters.len(), 1);
        assert_eq!(request.clusters[0].cluster_id, 0);
    }

    #[tokio::test]
    async fn generate_summary_truncates_large_error_messages() {
        let server = MockServer::start().await;
        // 巨大なエラーメッセージを返すモック
        let large_error_body = "x".repeat(10000);
        Mock::given(method("POST"))
            .and(path("/v1/summary/generate"))
            .respond_with(ResponseTemplate::new(400).set_body_string(large_error_body.clone()))
            .mount(&server)
            .await;

        let client = NewsCreatorClient::new(server.uri(), Duration::from_secs(600))
            .expect("client should build");

        let request = SummaryRequest {
            job_id: Uuid::new_v4(),
            genre: "tech".to_string(),
            clusters: vec![ClusterInput {
                cluster_id: 0,
                representative_sentences: vec![RepresentativeSentence {
                    text: "Test sentence".to_string(),
                    published_at: None,
                    source_url: None,
                    article_id: None,
                }],
                top_terms: None,
            }],
            options: None,
        };

        let error = client
            .generate_summary(&request)
            .await
            .expect_err("should fail with 400 status");

        let error_msg = error.to_string();
        // エラーメッセージがtruncateされていることを確認
        assert!(
            error_msg.len() < 1000,
            "error message should be truncated, got length: {}",
            error_msg.len()
        );
        // truncateされたことを示すメッセージが含まれていることを確認
        assert!(
            error_msg.contains("truncated"),
            "error message should indicate truncation: {}",
            error_msg
        );
    }
}
