//! Port traits for the themes pipeline.

use anyhow::Result;
use chrono::{DateTime, Utc};
use uuid::Uuid;

use crate::clients::datahub::DataHubArticle;
use crate::clients::knowledge_sovereign::ReadingSourceEntry;
use crate::clients::knowledge_sovereign::publish::ThemePublishOutcome;
use crate::clients::news_creator::models::{ThemeGenerateOutcome, ThemeGenerateRequest};
use crate::store::dao::themes::UnpublishedThemeVersion;

pub use crate::pipeline::cards::ports::{CardVerifier, EmbedCluster};

/// Port for reading sources from and publishing theme events to knowledge-sovereign.
#[allow(private_interfaces)]
#[async_trait::async_trait]
pub trait ThemeKnowledgePort: Send + Sync {
    async fn list_reading_sources_in_window(
        &self,
        user_id: Uuid,
        from: DateTime<Utc>,
        to: DateTime<Utc>,
    ) -> Result<Vec<ReadingSourceEntry>>;

    async fn publish_theme_versions(
        &self,
        user_id: Uuid,
        tenant_id: Uuid,
        versions: &[UnpublishedThemeVersion],
    ) -> ThemePublishOutcome;
}

/// Port for fetching article bodies by ID from the data plane.
#[allow(private_interfaces)]
#[async_trait::async_trait]
pub trait ThemeArticleSource: Send + Sync {
    async fn batch_get_articles_by_ids(&self, article_ids: &[Uuid]) -> Result<Vec<DataHubArticle>>;
}

/// Port for generating theme page content via news-creator.
#[allow(private_interfaces)]
#[async_trait::async_trait]
pub trait ThemeGenerator: Send + Sync {
    async fn generate_theme(&self, req: &ThemeGenerateRequest) -> Result<ThemeGenerateOutcome>;
}

/// Runner abstraction for scheduled batch runs and manual HTTP triggers.
#[async_trait::async_trait]
pub trait ThemesJobRunner: Send + Sync {
    async fn run_themes(&self, run_id: Uuid, run_time: DateTime<Utc>) -> Result<()>;
}
