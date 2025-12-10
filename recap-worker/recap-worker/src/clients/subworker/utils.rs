use anyhow::Result;
use std::time::Duration;

use super::types::{
    EXTRACTION_TIMEOUT_SECS, ExtractRequest, ExtractResponse, MAX_ERROR_MESSAGE_LENGTH,
};
use crate::clients::subworker::SubworkerClient;

/// エラーメッセージを要約して切り詰める。
pub(crate) fn truncate_error_message(msg: &str) -> String {
    let char_count = msg.chars().count();
    if char_count <= MAX_ERROR_MESSAGE_LENGTH {
        return msg.to_string();
    }
    let truncated: String = msg.chars().take(MAX_ERROR_MESSAGE_LENGTH).collect();
    format!("{truncated}... (truncated, {char_count} chars)")
}

/// バリデーションエラーのリストを要約する。
pub(crate) fn summarize_validation_errors(errors: &[String]) -> Vec<String> {
    errors.iter().map(|e| truncate_error_message(e)).collect()
}

impl SubworkerClient {
    /// Health check endpoint
    pub(crate) async fn ping(&self) -> Result<()> {
        let url = self
            .base_url
            .join("health")
            .context("failed to build subworker health URL")?;

        self.client
            .get(url)
            .send()
            .await
            .context("subworker health request failed")?
            .error_for_status()
            .context("subworker health endpoint returned error status")?;

        Ok(())
    }

    /// Extract content from HTML
    pub(crate) async fn extract_content(&self, html: &str) -> Result<String> {
        let url = self
            .base_url
            .join("v1/extract")
            .context("failed to build extract URL")?;

        let request = ExtractRequest {
            html,
            include_comments: false,
        };

        let response = self
            .client
            .post(url)
            .json(&request)
            .timeout(Duration::from_secs(EXTRACTION_TIMEOUT_SECS))
            .send()
            .await
            .context("extract request failed")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(anyhow::anyhow!(
                "extract endpoint error {}: {}",
                status,
                body
            ));
        }

        let body: ExtractResponse = response
            .json()
            .await
            .context("failed to parse extract response")?;
        Ok(body.text)
    }
}

use anyhow::Context;

#[cfg(test)]
mod tests {
    use super::*;
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    #[tokio::test]
    async fn ping_succeeds_for_ok_response() {
        let server = MockServer::start().await;
        Mock::given(method("GET"))
            .and(path("/health"))
            .respond_with(ResponseTemplate::new(204))
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri(), 10).expect("client should build");

        client.ping().await.expect("ping should succeed");
    }

    #[tokio::test]
    async fn ping_fails_on_error_status() {
        let server = MockServer::start().await;
        Mock::given(method("GET"))
            .and(path("/health"))
            .respond_with(ResponseTemplate::new(500))
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri(), 10).expect("client should build");

        let error = client.ping().await.expect_err("ping should fail");
        assert!(error.to_string().contains("error status"));
    }
}
