//! Production adapters implementing theme pipeline ports.

use anyhow::Result;
use chrono::{DateTime, Utc};
use std::sync::Arc;
use uuid::Uuid;

use super::ports::{ThemeArticleSource, ThemeGenerator, ThemeKnowledgePort};
use crate::clients::datahub::{DataHubArticle, DataHubClient};
use crate::clients::knowledge_sovereign::publish::{ThemePublishOutcome, publish_theme_versions};
use crate::clients::knowledge_sovereign::{KnowledgeSovereignClient, ReadingSourceEntry};
use crate::clients::news_creator::NewsCreatorClient;
use crate::clients::news_creator::models::{ThemeGenerateOutcome, ThemeGenerateRequest};
use crate::store::dao::themes::UnpublishedThemeVersion;

pub use crate::pipeline::cards::adapters::{SubworkerCardVerifier, SubworkerEmbedCluster};

#[derive(Clone)]
pub struct SovereignThemeKnowledgePort {
    client: Arc<KnowledgeSovereignClient>,
}

impl SovereignThemeKnowledgePort {
    pub(crate) fn new(client: Arc<KnowledgeSovereignClient>) -> Self {
        Self { client }
    }
}

#[allow(private_interfaces)]
#[async_trait::async_trait]
impl ThemeKnowledgePort for SovereignThemeKnowledgePort {
    async fn list_reading_sources_in_window(
        &self,
        user_id: Uuid,
        from: DateTime<Utc>,
        to: DateTime<Utc>,
    ) -> Result<Vec<ReadingSourceEntry>> {
        self.client
            .list_reading_sources_in_window(user_id, from, to)
            .await
    }

    async fn publish_theme_versions(
        &self,
        user_id: Uuid,
        tenant_id: Uuid,
        versions: &[UnpublishedThemeVersion],
    ) -> ThemePublishOutcome {
        publish_theme_versions(&self.client, user_id, tenant_id, versions).await
    }
}

#[derive(Clone)]
pub struct DataHubThemeArticleSource {
    client: Arc<DataHubClient>,
}

impl DataHubThemeArticleSource {
    pub(crate) fn new(client: Arc<DataHubClient>) -> Self {
        Self { client }
    }
}

#[allow(private_interfaces)]
#[async_trait::async_trait]
impl ThemeArticleSource for DataHubThemeArticleSource {
    async fn batch_get_articles_by_ids(&self, article_ids: &[Uuid]) -> Result<Vec<DataHubArticle>> {
        self.client.batch_get_articles_by_ids(article_ids).await
    }
}

#[derive(Clone)]
pub struct NewsCreatorThemeGenerator {
    client: Arc<NewsCreatorClient>,
}

impl NewsCreatorThemeGenerator {
    pub(crate) fn new(client: Arc<NewsCreatorClient>) -> Self {
        Self { client }
    }
}

#[allow(private_interfaces)]
#[async_trait::async_trait]
impl ThemeGenerator for NewsCreatorThemeGenerator {
    async fn generate_theme(&self, req: &ThemeGenerateRequest) -> Result<ThemeGenerateOutcome> {
        self.client
            .generate_theme(req)
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))
    }
}
