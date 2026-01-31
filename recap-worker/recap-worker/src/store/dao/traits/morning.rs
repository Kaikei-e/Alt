//! MorningDao trait - Morning article group operations

use std::future::Future;

use anyhow::Result;
use chrono::{DateTime, Utc};
use uuid::Uuid;

/// MorningDao - モーニング記事グループのためのデータアクセス層
#[allow(dead_code, clippy::type_complexity)]
pub trait MorningDao: Send + Sync {
    /// モーニング記事グループを保存する
    fn save_morning_article_groups(
        &self,
        groups: &[(Uuid, Uuid, bool)],
    ) -> impl Future<Output = Result<()>> + Send;

    /// モーニング記事グループを取得する
    fn get_morning_article_groups(
        &self,
        since: DateTime<Utc>,
    ) -> impl Future<Output = Result<Vec<(Uuid, Uuid, bool, DateTime<Utc>)>>> + Send;
}
