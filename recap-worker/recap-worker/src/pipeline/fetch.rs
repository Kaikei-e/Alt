use std::sync::Arc;

use anyhow::{Context, Result, bail};
use async_trait::async_trait;
use chrono::{DateTime, Duration, Utc};
use tracing::{debug, info, warn};
use uuid::Uuid;

use crate::{
    clients::{
        alt_backend::{AltBackendArticle, AltBackendClient},
        tag_generator::TagGeneratorClient,
    },
    scheduler::JobContext,
    store::{dao::RecapDao, models::RawArticle},
    util::retry::{RetryConfig, is_retryable_error},
};

use super::tag_signal::TagSignal;
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub(crate) struct FetchedArticle {
    pub(crate) id: String,
    pub(crate) title: Option<String>,
    pub(crate) body: String,
    pub(crate) language: Option<String>,
    pub(crate) published_at: Option<DateTime<Utc>>,
    pub(crate) source_url: Option<String>,
    pub(crate) tags: Vec<TagSignal>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub(crate) struct FetchedCorpus {
    pub(crate) job_id: Uuid,
    pub(crate) articles: Vec<FetchedArticle>,
}

#[async_trait]
pub(crate) trait FetchStage: Send + Sync {
    async fn fetch(&self, job: &JobContext) -> anyhow::Result<FetchedCorpus>;
}

/// alt-backendから記事を取得し、Raw記事をDBにバックアップするステージ。
pub(crate) struct AltBackendFetchStage {
    client: Arc<AltBackendClient>,
    tag_generator_client: Option<Arc<TagGeneratorClient>>,
    dao: Arc<RecapDao>,
    retry_config: RetryConfig,
    window_days: u32,
}

impl AltBackendFetchStage {
    pub(crate) fn new(
        client: Arc<AltBackendClient>,
        tag_generator_client: Option<Arc<TagGeneratorClient>>,
        dao: Arc<RecapDao>,
        retry_config: RetryConfig,
        window_days: u32,
    ) -> Self {
        Self {
            client,
            tag_generator_client,
            dao,
            retry_config,
            window_days,
        }
    }

    /// 再試行付きで記事を取得する。
    async fn fetch_with_retry(
        &self,
        from: DateTime<Utc>,
        to: DateTime<Utc>,
    ) -> Result<Vec<AltBackendArticle>> {
        let mut attempt = 0;

        loop {
            info!(attempt, %from, %to, "fetching articles batch...");
            let start = std::time::Instant::now();
            match self.client.fetch_articles(from, to).await {
                Ok(articles) => {
                    let elapsed = start.elapsed();
                    info!(attempt, count = articles.len(), elapsed_ms = elapsed.as_millis(), "fetch call succeeded");
                    if attempt > 0 {
                        info!(attempt, "fetch succeeded after retry");
                    }
                    return Ok(articles);
                }
                Err(err) => {
                    let elapsed = start.elapsed();
                    warn!(attempt, elapsed_ms = elapsed.as_millis(), ?err, "fetch call failed");
                    attempt += 1;

                    if !self.retry_config.can_retry(attempt) {
                        warn!(
                            attempt,
                            max_attempts = self.retry_config.max_attempts,
                            "fetch failed after all retries"
                        );
                        return Err(err);
                    }

                    // reqwest::Errorでない場合は再試行不可
                    let is_retryable = err
                        .downcast_ref::<reqwest::Error>()
                        .is_some_and(is_retryable_error);

                    if !is_retryable {
                        warn!(?err, "error is not retryable");
                        return Err(err);
                    }

                    let delay = self.retry_config.delay_for_attempt(attempt);
                    warn!(
                        attempt,
                        delay_ms = delay.as_millis(),
                        "fetch failed, retrying after delay"
                    );

                    tokio::time::sleep(delay).await;
                }
            }
        }
    }
}

#[async_trait]
impl FetchStage for AltBackendFetchStage {
    async fn fetch(&self, job: &JobContext) -> Result<FetchedCorpus> {
        let lock_result = self
            .dao
            .create_job_with_lock(job.job_id, None)
            .await
            .map_err(|err| {
                tracing::error!(job_id = %job.job_id, error = ?err, "failed to create recap job record");
                err
            })
            .context("failed to create recap job record")?;

        if lock_result.is_none() {
            bail!("recap job {} is already being processed", job.job_id);
        }

        // 取得期間を計算（現在時刻からwindow_days日前まで）
        let to = Utc::now();
        let from = to - Duration::days(i64::from(self.window_days));

        info!(
            job_id = %job.job_id,
            from = %from.to_rfc3339(),
            to = %to.to_rfc3339(),
            window_days = self.window_days,
            "fetching articles from alt-backend"
        );

        // 再試行付きで記事を取得
        let articles = self.fetch_with_retry(from, to).await?;

        info!(
            job_id = %job.job_id,
            count = articles.len(),
            "fetched articles from alt-backend"
        );

        // Raw記事をバックアップ
        let raw_articles = convert_to_raw_articles(&articles);
        self.dao
            .backup_raw_articles(job.job_id, &raw_articles)
            .await
            .context("failed to backup raw articles")?;

        debug!(
            job_id = %job.job_id,
            count = raw_articles.len(),
            "backed up raw articles to database"
        );

        // tag-generatorからタグを取得（オプショナル）
        let mut tags_by_article = std::collections::HashMap::new();
        if let Some(ref tag_client) = self.tag_generator_client {
            let article_ids: Vec<String> = articles.iter().map(|a| a.article_id.clone()).collect();
            match tag_client.fetch_tags_batch(&article_ids).await {
                Ok(tags) => {
                    tags_by_article = tags;
                    info!(
                        job_id = %job.job_id,
                        articles_with_tags = tags_by_article.len(),
                        "fetched tags from tag-generator"
                    );
                }
                Err(err) => {
                    warn!(
                        job_id = %job.job_id,
                        error = ?err,
                        "failed to fetch tags from tag-generator, continuing without tags"
                    );
                }
            }
        }

        // パイプライン用のデータ構造に変換
        Ok(FetchedCorpus {
            job_id: job.job_id,
            articles: articles
                .into_iter()
                .map(|article| {
                    // alt-backendから取得したタグとtag-generatorから取得したタグをマージ
                    let mut tags: Vec<TagSignal> = article
                        .tags
                        .into_iter()
                        .map(|tag| {
                            TagSignal::new(
                                tag.label,
                                tag.confidence.unwrap_or(0.0),
                                tag.source,
                                tag.updated_at,
                            )
                        })
                        .collect();

                    // tag-generatorから取得したタグを追加（重複チェックなし、後で必要に応じて追加）
                    if let Some(tag_generator_tags) = tags_by_article.get(&article.article_id) {
                        tags.extend_from_slice(tag_generator_tags);
                    }

                    FetchedArticle {
                        id: article.article_id,
                        title: article.title,
                        body: article.fulltext,
                        language: article.lang_hint,
                        published_at: article.published_at,
                        source_url: article.source_url,
                        tags,
                    }
                })
                .collect(),
        })
    }
}

/// AltBackendArticleをRawArticleに変換する。
fn convert_to_raw_articles(articles: &[AltBackendArticle]) -> Vec<RawArticle> {
    articles
        .iter()
        .map(|article| {
            // 正規化ハッシュを計算（現時点では単純なハッシュ、後でXXH3に置き換え）
            let normalized_hash = format!("{:x}", md5::compute(&article.fulltext));

            RawArticle::new(
                article.article_id.clone(),
                article.title.clone(),
                article.fulltext.clone(),
                article.published_at,
                article.source_url.clone(),
                article.lang_hint.clone(),
                normalized_hash,
            )
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn convert_to_raw_articles_creates_valid_entries() {
        let articles = vec![AltBackendArticle {
            article_id: "art-1".to_string(),
            title: Some("Title".to_string()),
            fulltext: "Content".to_string(),
            published_at: None,
            source_url: Some("https://example.com".to_string()),
            lang_hint: Some("en".to_string()),
            tags: Vec::new(),
        }];

        let raw = convert_to_raw_articles(&articles);

        assert_eq!(raw.len(), 1);
        assert_eq!(raw[0].article_id, "art-1");
        assert_eq!(raw[0].title.as_deref(), Some("Title"));
        assert_eq!(raw[0].fulltext_html, "Content");
        assert_eq!(raw[0].lang_hint.as_deref(), Some("en"));
        assert!(!raw[0].normalized_hash.is_empty());
    }

    #[test]
    fn fetched_article_stores_all_fields() {
        let article = FetchedArticle {
            id: "art-1".to_string(),
            title: Some("Title".to_string()),
            body: "Body".to_string(),
            language: Some("ja".to_string()),
            published_at: None,
            source_url: Some("https://example.com".to_string()),
            tags: Vec::new(),
        };

        assert_eq!(article.id, "art-1");
        assert_eq!(article.title.as_deref(), Some("Title"));
        assert_eq!(article.body, "Body");
        assert_eq!(article.language.as_deref(), Some("ja"));
        assert_eq!(article.source_url.as_deref(), Some("https://example.com"));
    }
}
