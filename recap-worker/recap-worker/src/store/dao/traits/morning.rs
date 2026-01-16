//! MorningDao trait - Morning article group operations

use anyhow::Result;
use async_trait::async_trait;
use chrono::{DateTime, Utc};
use uuid::Uuid;

/// MorningDao - モーニング記事グループのためのデータアクセス層
#[allow(dead_code)]
#[async_trait]
pub trait MorningDao: Send + Sync {
    /// モーニング記事グループを保存する
    async fn save_morning_article_groups(&self, groups: &[(Uuid, Uuid, bool)]) -> Result<()>;

    /// モーニング記事グループを取得する
    async fn get_morning_article_groups(
        &self,
        since: DateTime<Utc>,
    ) -> Result<Vec<(Uuid, Uuid, bool, DateTime<Utc>)>>;
}
