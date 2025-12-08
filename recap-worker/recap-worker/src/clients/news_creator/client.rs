use std::time::Duration;

use anyhow::{Context, Result, anyhow};
use reqwest::{Client, Url};
use serde::Serialize;
use serde_json::Value;
use tracing::{debug, warn};
use uuid::Uuid;

use crate::clients::TokenCounter;
use crate::clients::subworker::ClusteringResponse;
use crate::schema::{news_creator::SUMMARY_RESPONSE_SCHEMA, validate_json};

use super::builder::SummaryRequestBuilder;
use super::models::{
    GenreTieBreakRequest, GenreTieBreakResponse, NewsCreatorSummary, SummaryRequest,
    SummaryResponse, truncate_error_message,
};

#[derive(Debug, Clone)]
pub(crate) struct NewsCreatorClient {
    client: Client,
    base_url: Url,
    summary_timeout: Duration,
    token_counter: TokenCounter,
}

impl NewsCreatorClient {
    pub(crate) fn new(base_url: impl Into<String>, summary_timeout: Duration) -> Result<Self> {
        let client = Client::builder()
            .timeout(Duration::from_secs(5))
            .build()
            .context("failed to build news-creator client")?;

        let base_url = Url::parse(&base_url.into()).context("invalid news-creator base URL")?;

        // Try to initialize TokenCounter, fallback to dummy if it fails (e.g., in test environments)
        let token_counter = TokenCounter::new().unwrap_or_else(|e| {
            tracing::warn!(
                "Failed to initialize TokenCounter: {}. Using dummy tokenizer (character count only).",
                e
            );
            TokenCounter::dummy()
        });

        Ok(Self {
            client,
            base_url,
            summary_timeout,
            token_counter,
        })
    }

    #[cfg(test)]
    #[allow(dead_code)]
    pub(crate) fn new_dummy() -> Self {
        Self::new_for_test("http://localhost")
    }

    #[cfg(test)]
    pub(crate) fn new_for_test(base_url: impl Into<String>) -> Self {
        Self {
            client: Client::new(),
            base_url: Url::parse(&base_url.into()).unwrap(),
            summary_timeout: Duration::from_secs(60),
            token_counter: TokenCounter::dummy(),
        }
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
    /// * `_max_sentences_per_cluster` - クラスターごとの最大文数 (予算配分ロジックにより無視される場合があるが、上限として使用)
    /// * `article_metadata` - 記事IDからメタデータ（published_at, source_url）へのマップ
    ///
    /// # Returns
    /// 要約リクエスト
    pub(crate) fn build_summary_request(
        &self,
        job_id: Uuid,
        clustering: &ClusteringResponse,
        max_sentences_per_cluster: usize,
        article_metadata: &std::collections::HashMap<
            String,
            (Option<chrono::DateTime<chrono::Utc>>, Option<String>),
        >,
    ) -> SummaryRequest {
        let builder = SummaryRequestBuilder::new(&self.token_counter);
        builder.build_summary_request(
            job_id,
            clustering,
            max_sentences_per_cluster,
            article_metadata,
        )
    }

    /// Mapフェーズ：単一クラスタの要約を生成する。
    #[allow(dead_code)]
    pub(crate) async fn map_summarize(
        &self,
        job_id: Uuid,
        genre: &str,
        cluster: crate::clients::news_creator::models::ClusterInput,
    ) -> Result<SummaryResponse> {
        let request = SummaryRequest {
            job_id,
            genre: genre.to_string(),
            clusters: vec![cluster],
            genre_highlights: None,
            options: Some(crate::clients::news_creator::models::SummaryOptions {
                max_bullets: Some(5), // 中間要約は短めに
                temperature: Some(0.7),
            }),
        };
        self.generate_summary(&request).await
    }

    /// Reduceフェーズ：複数の中間要約から最終要約を生成する。
    #[allow(dead_code)]
    pub(crate) async fn reduce_summarize(
        &self,
        job_id: Uuid,
        genre: &str,
        summaries: Vec<crate::clients::news_creator::models::Summary>,
    ) -> Result<SummaryResponse> {
        // 中間要約の各行を代表文として扱う
        let mut representative_sentences = Vec::new();
        for summary in summaries {
            for bullet in summary.bullets {
                representative_sentences.push(
                    crate::clients::news_creator::models::RepresentativeSentence {
                        text: bullet,
                        published_at: None, // 中間要約には日付がない
                        source_url: None,
                        article_id: None,
                        is_centroid: false,
                    },
                );
            }
        }

        let cluster = crate::clients::news_creator::models::ClusterInput {
            cluster_id: 0, // 仮想的な単一クラスタ
            representative_sentences,
            top_terms: None,
        };

        let request = SummaryRequest {
            job_id,
            genre: genre.to_string(),
            clusters: vec![cluster],
            genre_highlights: None,
            options: Some(crate::clients::news_creator::models::SummaryOptions {
                max_bullets: Some(15), // 最終要約
                temperature: Some(0.7),
            }),
        };
        self.generate_summary(&request).await
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

        let client = NewsCreatorClient::new_for_test(server.uri());

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

        let client = NewsCreatorClient::new_for_test(server.uri());

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

        let client = NewsCreatorClient::new_for_test(server.uri());
        let summary = client
            .summarize(&serde_json::json!({"job_id": "00000000-0000-0000-0000-000000000000"}))
            .await
            .expect("summarize succeeds");

        assert_eq!(summary.response_id, "resp-123");
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

        let client = NewsCreatorClient::new_for_test(server.uri());

        let request = SummaryRequest {
            job_id: Uuid::new_v4(),
            genre: "tech".to_string(),
            clusters: vec![crate::clients::news_creator::models::ClusterInput {
                cluster_id: 0,
                representative_sentences: vec![
                    crate::clients::news_creator::models::RepresentativeSentence {
                        text: "Test sentence".to_string(),
                        published_at: None,
                        source_url: None,
                        article_id: None,
                        is_centroid: false,
                    },
                ],
                top_terms: None,
            }],
            genre_highlights: None,
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

#[cfg(test)]
mod tests_map_reduce {
    use super::*;
    use uuid::Uuid;
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    #[tokio::test]
    async fn map_summarize_constructs_correct_request() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/v1/summary/generate"))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
                "job_id": "00000000-0000-0000-0000-000000000000",
                "genre": "tech",
                "summary": {
                    "title": "Map Summary",
                    "bullets": ["Bullet 1", "Bullet 2"],
                    "language": "ja"
                },
                "metadata": {
                    "model": "gpt-4",
                    "temperature": 0.7
                }
            })))
            .mount(&server)
            .await;

        let client = NewsCreatorClient::new_for_test(server.uri());
        let cluster = crate::clients::news_creator::models::ClusterInput {
            cluster_id: 1,
            representative_sentences: vec![],
            top_terms: None,
        };

        let response = client
            .map_summarize(Uuid::new_v4(), "tech", cluster)
            .await
            .expect("map_summarize succeeds");

        assert_eq!(response.summary.title, "Map Summary");
        assert_eq!(response.summary.bullets.len(), 2);
    }

    #[tokio::test]
    async fn reduce_summarize_constructs_correct_request() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/v1/summary/generate"))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
                "job_id": "00000000-0000-0000-0000-000000000000",
                "genre": "tech",
                "summary": {
                    "title": "Reduce Summary",
                    "bullets": ["Final Bullet 1"],
                    "language": "ja"
                },
                "metadata": {
                    "model": "gpt-4",
                    "temperature": 0.7
                }
            })))
            .mount(&server)
            .await;

        let client = NewsCreatorClient::new_for_test(server.uri());
        let summaries = vec![
            crate::clients::news_creator::models::Summary {
                title: "Summary 1".to_string(),
                bullets: vec!["Bullet A".to_string()],
                language: "ja".to_string(),
            },
            crate::clients::news_creator::models::Summary {
                title: "Summary 2".to_string(),
                bullets: vec!["Bullet B".to_string()],
                language: "ja".to_string(),
            },
        ];

        let response = client
            .reduce_summarize(Uuid::new_v4(), "tech", summaries)
            .await
            .expect("reduce_summarize succeeds");

        assert_eq!(response.summary.title, "Reduce Summary");
        assert_eq!(response.summary.bullets.len(), 1);
    }
}
