//! ArticleDao trait - Article management operations

use std::collections::HashMap;
use std::future::Future;

use anyhow::Result;
use chrono::{DateTime, Utc};
use uuid::Uuid;

use crate::store::dao::article::FetchedArticleData;
use crate::store::models::RawArticle;

/// ArticleDao - 記事管理のためのデータアクセス層
#[allow(dead_code, clippy::type_complexity)]
pub trait ArticleDao: Send + Sync {
    /// 生の記事データをバックアップする
    fn backup_raw_articles(
        &self,
        job_id: Uuid,
        articles: &[RawArticle],
    ) -> impl Future<Output = Result<()>> + Send;

    /// 記事のメタデータを取得する
    fn get_article_metadata(
        &self,
        job_id: Uuid,
        article_ids: &[String],
    ) -> impl Future<Output = Result<HashMap<String, (Option<DateTime<Utc>>, Option<String>)>>> + Send;

    /// 記事IDから記事を取得する
    fn get_articles_by_ids(
        &self,
        job_id: Uuid,
        article_ids: &[String],
    ) -> impl Future<Output = Result<Vec<FetchedArticleData>>> + Send;
}
