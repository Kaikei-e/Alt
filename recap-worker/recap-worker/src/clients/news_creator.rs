use std::time::Duration;

use anyhow::{Context, Result, anyhow};
use reqwest::{Client, Url};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use tracing::{debug, warn};
use uuid::Uuid;

use crate::clients::subworker::ClusteringResponse;
use crate::schema::{news_creator::SUMMARY_RESPONSE_SCHEMA, validate_json};

#[derive(Debug, Clone)]
pub(crate) struct NewsCreatorClient {
    client: Client,
    base_url: Url,
}

impl NewsCreatorClient {
    pub(crate) fn new(base_url: impl Into<String>) -> Result<Self> {
        let client = Client::builder()
            .timeout(Duration::from_secs(5))
            .build()
            .context("failed to build news-creator client")?;

        let base_url = Url::parse(&base_url.into()).context("invalid news-creator base URL")?;

        Ok(Self { client, base_url })
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
            .timeout(Duration::from_secs(120)) // LLM処理のため長めのタイムアウト
            .send()
            .await
            .context("summary generation request failed")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(anyhow!(
                "summary generation endpoint returned error status {}: {}",
                status,
                body
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
    ///
    /// # Returns
    /// 要約リクエスト
    pub(crate) fn build_summary_request(
        job_id: Uuid,
        clustering: &ClusteringResponse,
        max_sentences_per_cluster: usize,
    ) -> SummaryRequest {
        let clusters = clustering
            .clusters
            .iter()
            .filter(|cluster| cluster.cluster_id >= 0)
            .map(|cluster| {
                // 各クラスターから代表的な文を選択（最大N文）
                let representative_sentences: Vec<String> = cluster
                    .representatives
                    .iter()
                    .take(max_sentences_per_cluster)
                    .filter_map(|rep| {
                        let text = rep.text.trim();
                        if text.is_empty() {
                            None
                        } else {
                            Some(text.to_string())
                        }
                    })
                    .collect();

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
                max_bullets: Some(5),
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
                "genre tie-break endpoint returned error status {}: {}",
                status,
                body
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

/// クラスター入力データ。
#[derive(Debug, Clone, Serialize)]
pub(crate) struct ClusterInput {
    pub(crate) cluster_id: i32,
    pub(crate) representative_sentences: Vec<String>,
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

        let client = NewsCreatorClient::new(server.uri()).expect("client should build");

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

        let client = NewsCreatorClient::new(server.uri()).expect("client should build");

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

        let client = NewsCreatorClient::new(server.uri()).expect("client should build");
        let summary = client
            .summarize(&serde_json::json!({"job_id": "00000000-0000-0000-0000-000000000000"}))
            .await
            .expect("summarize succeeds");

        assert_eq!(summary.response_id, "resp-123");
    }
}
