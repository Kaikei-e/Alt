use std::sync::Arc;

use anyhow::{Context, Result};
use async_trait::async_trait;
use chrono::{DateTime, Duration, Utc};
use tracing::{debug, info, warn};
use uuid::Uuid;

use crate::{
    clients::alt_backend::{AltBackendArticle, AltBackendClient},
    store::{dao::RecapDao, models::RawArticle},
    util::retry::{is_retryable_error, RetryConfig},
    scheduler::JobContext,
};

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct FetchedArticle {
    pub(crate) id: String,
    pub(crate) title: Option<String>,
    pub(crate) body: String,
    pub(crate) language: Option<String>,
    pub(crate) published_at: Option<DateTime<Utc>>,
    pub(crate) source_url: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
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
    dao: Arc<RecapDao>,
    retry_config: RetryConfig,
    window_days: u32,
}

impl AltBackendFetchStage {
    pub(crate) fn new(
        client: Arc<AltBackendClient>,
        dao: Arc<RecapDao>,
        retry_config: RetryConfig,
        window_days: u32,
    ) -> Self {
        Self {
            client,
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
            match self.client.fetch_articles(from, to).await {
                Ok(articles) => {
                    if attempt > 0 {
                        info!(attempt, "fetch succeeded after retry");
                    }
                    return Ok(articles);
                }
                Err(err) => {
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
                        .map_or(false, is_retryable_error);

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

        // パイプライン用のデータ構造に変換
        Ok(FetchedCorpus {
            job_id: job.job_id,
            articles: articles
                .into_iter()
                .map(|article| FetchedArticle {
                    id: article.id,
                    title: article.title,
                    body: article.content,
                    language: article.lang,
                    published_at: article.published_at,
                    source_url: article.source_url,
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
            let normalized_hash = format!("{:x}", md5::compute(&article.content));

            RawArticle::new(
                article.id.clone(),
                article.title.clone(),
                article.content.clone(),
                article.published_at,
                article.source_url.clone(),
                article.lang.clone(),
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
            id: "art-1".to_string(),
            title: Some("Title".to_string()),
            content: "Content".to_string(),
            published_at: None,
            source_url: Some("https://example.com".to_string()),
            lang: Some("en".to_string()),
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
        };

        assert_eq!(article.id, "art-1");
        assert_eq!(article.title.as_deref(), Some("Title"));
        assert_eq!(article.body, "Body");
        assert_eq!(article.language.as_deref(), Some("ja"));
        assert_eq!(article.source_url.as_deref(), Some("https://example.com"));
    }
}
