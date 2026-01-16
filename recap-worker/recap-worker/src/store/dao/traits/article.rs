//! ArticleDao trait - Article management operations

use anyhow::Result;
use async_trait::async_trait;
use chrono::{DateTime, Utc};
use std::collections::HashMap;
use uuid::Uuid;

use crate::store::dao::article::FetchedArticleData;
use crate::store::models::RawArticle;

/// ArticleDao - 記事管理のためのデータアクセス層
#[allow(dead_code)]
#[async_trait]
pub trait ArticleDao: Send + Sync {
    /// 生の記事データをバックアップする
    async fn backup_raw_articles(&self, job_id: Uuid, articles: &[RawArticle]) -> Result<()>;

    /// 記事のメタデータを取得する
    async fn get_article_metadata(
        &self,
        job_id: Uuid,
        article_ids: &[String],
    ) -> Result<HashMap<String, (Option<DateTime<Utc>>, Option<String>)>>;

    /// 記事IDから記事を取得する
    async fn get_articles_by_ids(
        &self,
        job_id: Uuid,
        article_ids: &[String],
    ) -> Result<Vec<FetchedArticleData>>;
}
