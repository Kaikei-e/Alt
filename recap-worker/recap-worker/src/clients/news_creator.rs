use std::time::Duration;

use anyhow::{Context, Result};
use reqwest::{Client, Url};
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone)]
pub(crate) struct NewsCreatorClient {
    client: Client,
    base_url: Url,
}

impl NewsCreatorClient {
    pub(crate) fn new(base_url: impl Into<String>) -> Result<Self> {
        let client = Client::builder()
            .http2_prior_knowledge()
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
}

#[derive(Debug, Deserialize)]
pub(crate) struct NewsCreatorSummary {
    pub(crate) response_id: String,
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
