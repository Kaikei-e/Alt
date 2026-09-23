use anyhow::Result;
use chrono::{DateTime, Utc};
use std::collections::HashMap;
use std::sync::Arc;
use uuid::Uuid;

use super::ports::{CardGenerator, CardVerifier, EmbedCluster, FeedSource, GenreTagger};
use crate::clients::alt_backend::{AltBackendClient, AltBackendFeed};
use crate::clients::news_creator::NewsCreatorClient;
use crate::clients::news_creator::models::{CardGenerateOutcome, CardGenerateRequest};
use crate::clients::subworker::SubworkerClient;
use crate::clients::subworker::cards::{
    ClusterStoriesRequest, ClusterStoriesResponse, SubworkerCardsClient, VerifyCardRequest,
    VerifyCardResponse,
};

/// Adapter wrapping `AltBackendClient` as a `FeedSource`.
#[derive(Clone)]
pub struct AltBackendFeedSource {
    client: Arc<AltBackendClient>,
}

impl AltBackendFeedSource {
    pub(crate) fn new(client: Arc<AltBackendClient>) -> Self {
        Self { client }
    }
}

#[async_trait::async_trait]
impl FeedSource for AltBackendFeedSource {
    async fn fetch_all_feeds_in_window(
        &self,
        from: DateTime<Utc>,
        to: DateTime<Utc>,
    ) -> Result<Vec<AltBackendFeed>> {
        self.client.fetch_all_feeds_in_window(from, to).await
    }

    async fn get_all_read_feed_ids(
        &self,
        user_id: Uuid,
        since: Option<DateTime<Utc>>,
    ) -> Result<Vec<Uuid>> {
        self.client.get_all_read_feed_ids(user_id, since).await
    }
}

/// Adapter wrapping `SubworkerCardsClient` as an `EmbedCluster`.
#[derive(Clone)]
pub struct SubworkerEmbedCluster {
    client: Arc<SubworkerCardsClient>,
    expected_model: String,
    expected_dim: usize,
}

impl SubworkerEmbedCluster {
    pub fn new(client: Arc<SubworkerCardsClient>) -> Self {
        Self::with_expected(client, "bge-m3", 1024)
    }

    pub fn with_expected(
        client: Arc<SubworkerCardsClient>,
        expected_model: impl Into<String>,
        expected_dim: usize,
    ) -> Self {
        Self {
            client,
            expected_model: expected_model.into(),
            expected_dim,
        }
    }
}

#[async_trait::async_trait]
impl EmbedCluster for SubworkerEmbedCluster {
    async fn embed(&self, texts: &[String]) -> Result<Vec<Vec<f32>>> {
        let resp = self
            .client
            .embed(texts, true)
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))?;

        if resp.model != self.expected_model || resp.dim != self.expected_dim {
            anyhow::bail!(
                "subworker embedding identity mismatch: expected model '{}' with dim {}, got '{}' with dim {}",
                self.expected_model,
                self.expected_dim,
                resp.model,
                resp.dim
            );
        }

        Ok(resp.embeddings)
    }

    async fn cluster_stories(&self, req: &ClusterStoriesRequest) -> Result<ClusterStoriesResponse> {
        self.client
            .cluster_stories(req)
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))
    }
}

/// Adapter wrapping `SubworkerClient` as a `GenreTagger`.
#[derive(Clone)]
pub struct SubworkerGenreTagger {
    client: Arc<SubworkerClient>,
}

impl SubworkerGenreTagger {
    pub(crate) fn new(client: Arc<SubworkerClient>) -> Self {
        Self { client }
    }
}

#[async_trait::async_trait]
impl GenreTagger for SubworkerGenreTagger {
    async fn tag_genre(&self, text: &str) -> Result<HashMap<String, f32>> {
        self.client.classify_coarse(text).await
    }

    async fn tag_genres(
        &self,
        texts: &[String],
        concurrency: usize,
    ) -> Result<Vec<HashMap<String, f32>>> {
        use futures::stream::{self, StreamExt, TryStreamExt};
        let concurrency = concurrency.max(1);
        let client = Arc::clone(&self.client);
        stream::iter(texts.iter().cloned())
            .map(move |t| {
                let client = Arc::clone(&client);
                async move { client.classify_coarse(&t).await }
            })
            .buffered(concurrency)
            .try_collect()
            .await
    }
}

/// Adapter wrapping `NewsCreatorClient` as a `CardGenerator`.
#[allow(private_interfaces)]
#[derive(Clone)]
pub struct NewsCreatorCardGenerator {
    client: Arc<NewsCreatorClient>,
}

#[allow(private_interfaces)]
impl NewsCreatorCardGenerator {
    pub fn new(client: Arc<NewsCreatorClient>) -> Self {
        Self { client }
    }
}

#[allow(private_interfaces)]
#[async_trait::async_trait]
impl CardGenerator for NewsCreatorCardGenerator {
    async fn generate_card(&self, req: &CardGenerateRequest) -> Result<CardGenerateOutcome> {
        self.client
            .generate_card(req)
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))
    }
}

/// Adapter wrapping `SubworkerCardsClient` as a `CardVerifier`.
#[derive(Clone)]
pub struct SubworkerCardVerifier {
    client: Arc<SubworkerCardsClient>,
}

impl SubworkerCardVerifier {
    pub fn new(client: Arc<SubworkerCardsClient>) -> Self {
        Self { client }
    }
}

#[async_trait::async_trait]
impl CardVerifier for SubworkerCardVerifier {
    async fn verify_card(&self, req: &VerifyCardRequest) -> Result<VerifyCardResponse> {
        self.client
            .verify_card(req)
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))
    }
}
