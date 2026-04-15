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

use types::SUBWORKER_TIMEOUT_SECS;

#[derive(Debug, Clone)]
pub(crate) struct SubworkerClient {
    pub(crate) client: Client,
    pub(crate) base_url: Url,
    pub(crate) min_documents_per_genre: usize,
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
        })
    }
}
