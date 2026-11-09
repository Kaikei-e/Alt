use anyhow::{Context, Result};
use reqwest::header::{AUTHORIZATION, HeaderMap, HeaderValue};
use reqwest::{Client, Url};
use std::time::Duration;

mod admin;
pub(crate) mod cards;
mod classification;
mod clustering;
pub(crate) mod evaluation;
mod types;
mod utils;

pub(crate) use types::*;

use types::{DEFAULT_COARSE_CLASSIFY_TIMEOUT_SECS, SUBWORKER_TIMEOUT_SECS};

pub(crate) fn build_default_headers(token: Option<&str>) -> Result<HeaderMap> {
    let mut headers = HeaderMap::new();
    if let Some(token) = token {
        let mut value = HeaderValue::from_str(&format!("Bearer {token}"))
            .context("invalid subworker admin auth token for Authorization header")?;
        value.set_sensitive(true);
        headers.insert(AUTHORIZATION, value);
    }
    Ok(headers)
}

fn build_http_client(timeout: Duration, token: Option<&str>) -> Result<Client> {
    let headers = build_default_headers(token)?;
    Client::builder()
        .timeout(timeout)
        .default_headers(headers)
        .build()
        .context("failed to build subworker client")
}

#[derive(Debug, Clone)]
pub(crate) struct SubworkerClient {
    pub(crate) client: Client,
    pub(crate) base_url: Url,
    pub(crate) min_documents_per_genre: usize,
    /// Per-call timeout override for `classify_coarse`. Without this the
    /// call inherits the client-wide `SUBWORKER_TIMEOUT_SECS` (1h), letting
    /// a slow-but-alive subworker stall the genre stage per article. Set via
    /// `with_coarse_classify_timeout`; defaults to
    /// `DEFAULT_COARSE_CLASSIFY_TIMEOUT_SECS` for the ~20 call sites that
    /// don't opt into a config-driven override.
    coarse_classify_timeout: Duration,
}

impl SubworkerClient {
    pub(crate) fn new(endpoint: impl Into<String>, min_documents_per_genre: usize) -> Result<Self> {
        let client = build_http_client(Duration::from_secs(SUBWORKER_TIMEOUT_SECS), None)?;
        Self::new_with_client(endpoint, min_documents_per_genre, client)
    }

    /// Construct with an externally-built `reqwest::Client`. Used by the
    /// mTLS wiring in `app.rs` to inject an identity-presenting client.
    pub(crate) fn new_with_client(
        endpoint: impl Into<String>,
        min_documents_per_genre: usize,
        client: Client,
    ) -> Result<Self> {
        let base_url = Url::parse(&endpoint.into()).context("invalid subworker base URL")?;
        Ok(Self {
            client,
            base_url,
            min_documents_per_genre,
            coarse_classify_timeout: Duration::from_secs(DEFAULT_COARSE_CLASSIFY_TIMEOUT_SECS),
        })
    }

    /// Overrides the per-call `classify_coarse` timeout. Used by `app.rs` to
    /// wire in `RECAP_SUBWORKER_COARSE_CLASSIFY_TIMEOUT_SECS`.
    #[must_use]
    pub(crate) fn with_coarse_classify_timeout(mut self, timeout: Duration) -> Self {
        self.coarse_classify_timeout = timeout;
        self
    }

    /// Rebuilds the underlying `reqwest::Client` with `ClientBuilder::default_headers`
    /// carrying `Authorization: Bearer <token>` marked sensitive when `token` is `Some`.
    /// When `token` is `None` (`RECAP_ADMIN_AUTH=disabled`), no default `Authorization`
    /// header is set.
    #[must_use]
    pub(crate) fn with_admin_token(mut self, token: Option<String>) -> Self {
        self.client = build_http_client(
            Duration::from_secs(SUBWORKER_TIMEOUT_SECS),
            token.as_deref(),
        )
        .expect("failed to build subworker client with default headers");
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use reqwest::header::HeaderName;
    use wiremock::matchers::{header, method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    #[test]
    fn build_default_headers_marks_authorization_sensitive() {
        let headers = build_default_headers(Some("secret-token-value")).unwrap();
        let auth = headers
            .get(AUTHORIZATION)
            .expect("Authorization header must be set");
        assert_eq!(auth.to_str().unwrap(), "Bearer secret-token-value");
        assert!(auth.is_sensitive());
    }

    #[test]
    fn build_default_headers_omits_authorization_when_none() {
        let headers = build_default_headers(None).unwrap();
        assert!(headers.get(AUTHORIZATION).is_none());
    }

    #[tokio::test]
    async fn injected_client_preserves_probe_header_and_sends_authorization() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/v1/extract"))
            .and(header("x-mtls-probe", "mtls-identity-present"))
            .and(header("authorization", "Bearer test-injected-token"))
            .respond_with(
                ResponseTemplate::new(200).set_body_string(r#"{"text":"extracted body"}"#),
            )
            .expect(1)
            .mount(&server)
            .await;

        let mut probe_headers = HeaderMap::new();
        probe_headers.insert(
            HeaderName::from_static("x-mtls-probe"),
            HeaderValue::from_static("mtls-identity-present"),
        );
        let injected = Client::builder()
            .default_headers(probe_headers)
            .build()
            .unwrap();

        let client = SubworkerClient::new_with_client(server.uri(), 10, injected)
            .unwrap()
            .with_admin_token(Some("test-injected-token".to_string()));

        let text = client
            .extract_content("<p>hello</p>")
            .await
            .expect("injected client must preserve probe header and send Authorization");
        assert_eq!(text, "extracted body");
    }
}
