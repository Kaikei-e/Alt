use anyhow::Result;
use chrono::{DateTime, Utc};
use std::collections::HashMap;
use uuid::Uuid;

use crate::clients::alt_backend::AltBackendFeed;
use crate::clients::news_creator::models::{CardGenerateOutcome, CardGenerateRequest};
use crate::clients::subworker::cards::{
    ClusterStoriesRequest, ClusterStoriesResponse, VerifyCardRequest, VerifyCardResponse,
};

/// Port for fetching raw feed items in a time window.
#[async_trait::async_trait]
pub trait FeedSource: Send + Sync {
    async fn fetch_all_feeds_in_window(
        &self,
        from: DateTime<Utc>,
        to: DateTime<Utc>,
    ) -> Result<Vec<AltBackendFeed>>;

    async fn get_all_read_feed_ids(
        &self,
        user_id: Uuid,
        since: Option<DateTime<Utc>>,
    ) -> Result<Vec<Uuid>>;
}

/// Port for ML embeddings and story clustering.
#[async_trait::async_trait]
pub trait EmbedCluster: Send + Sync {
    async fn embed(&self, texts: &[String]) -> Result<Vec<Vec<f32>>>;
    async fn cluster_stories(&self, req: &ClusterStoriesRequest) -> Result<ClusterStoriesResponse>;
}

/// Port for coarse genre tagging of normalized items.
#[async_trait::async_trait]
pub trait GenreTagger: Send + Sync {
    async fn tag_genre(&self, text: &str) -> Result<HashMap<String, f32>>;

    async fn tag_genres(
        &self,
        texts: &[String],
        concurrency: usize,
    ) -> Result<Vec<HashMap<String, f32>>> {
        use futures::stream::{self, StreamExt, TryStreamExt};
        let concurrency = concurrency.max(1);
        stream::iter(texts.iter().cloned())
            .map(|t| async move { self.tag_genre(&t).await })
            .buffered(concurrency)
            .try_collect()
            .await
    }
}

/// Port for generating topic cards from candidate items.
#[allow(private_interfaces)]
#[async_trait::async_trait]
pub trait CardGenerator: Send + Sync {
    async fn generate_card(&self, req: &CardGenerateRequest) -> Result<CardGenerateOutcome>;
}

/// Port for verifying topic cards against source items (attribution, filler, specificity).
#[async_trait::async_trait]
pub trait CardVerifier: Send + Sync {
    async fn verify_card(&self, req: &VerifyCardRequest) -> Result<VerifyCardResponse>;
}
