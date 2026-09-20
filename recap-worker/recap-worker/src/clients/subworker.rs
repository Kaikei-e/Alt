use anyhow::{Context, Result};
use reqwest::{Client, Url};
use std::time::Duration;

mod admin;
mod classification;
mod clustering;
pub(crate) mod evaluation;
mod types;
mod utils;

pub(crate) use types::*;

use types::{DEFAULT_COARSE_CLASSIFY_TIMEOUT_SECS, SUBWORKER_TIMEOUT_SECS};

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
    /// Bearer token sent on `/admin/*` and `/v1/runs` requests to
    /// recap-subworker. `None` exactly when `RECAP_ADMIN_AUTH=disabled` was
    /// set explicitly — see `with_admin_token`.
    admin_token: Option<String>,
}

impl SubworkerClient {
    pub(crate) fn new(endpoint: impl Into<String>, min_documents_per_genre: usize) -> Result<Self> {
        let client = Client::builder()
            .timeout(Duration::from_secs(SUBWORKER_TIMEOUT_SECS))
            .build()
            .context("failed to build subworker client")?;
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
            admin_token: None,
        })
    }

    /// Overrides the per-call `classify_coarse` timeout. Used by `app.rs` to
    /// wire in `RECAP_SUBWORKER_COARSE_CLASSIFY_TIMEOUT_SECS`.
    #[must_use]
    pub(crate) fn with_coarse_classify_timeout(mut self, timeout: Duration) -> Self {
        self.coarse_classify_timeout = timeout;
        self
    }

    /// Sets the bearer token sent on `/admin/*` and `/v1/runs` requests to
    /// recap-subworker. Used by `app.rs` to wire in
    /// `Config::admin_auth_token` — the same secret recap-worker requires
    /// on its own `/admin/*` routes (see the `AdminAuth` doc comment in
    /// `config.rs`). `None` sends no `Authorization` header, matching
    /// `RECAP_ADMIN_AUTH=disabled`.
    #[must_use]
    pub(crate) fn with_admin_token(mut self, token: Option<String>) -> Self {
        self.admin_token = token;
        self
    }

    /// Applies the `Authorization: Bearer <token>` header when an admin
    /// token is configured, so every `/admin/*` and `/v1/runs` call site
    /// stays in sync with `with_admin_token` instead of reimplementing the
    /// header format.
    pub(crate) fn authorize(&self, builder: reqwest::RequestBuilder) -> reqwest::RequestBuilder {
        match &self.admin_token {
            Some(token) => builder.bearer_auth(token),
            None => builder,
        }
    }
}
