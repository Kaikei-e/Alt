//! MorningDao trait - Morning article group and Morning Letter operations

use std::future::Future;

use anyhow::Result;
use chrono::{DateTime, NaiveDate, Utc};
use uuid::Uuid;

use crate::store::models::{MorningLetter, MorningLetterSource};

/// MorningDao - モーニング記事グループ・Morning Letter のためのデータアクセス層
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

    /// Morning Letter を保存する (UPSERT on target_date + edition_timezone)。
    /// DB 上の実際の id を返す (conflict 時は既存の id を維持するため、
    /// メモリ上の `letter.id` とは一致しないことがある)。
    fn save_morning_letter(
        &self,
        letter: &MorningLetter,
    ) -> impl Future<Output = Result<Uuid>> + Send;

    /// Morning Letter のソース (provenance) を保存する
    fn save_morning_letter_sources(
        &self,
        sources: &[MorningLetterSource],
    ) -> impl Future<Output = Result<()>> + Send;

    /// 指定日の Morning Letter を取得する
    fn get_morning_letter_by_date(
        &self,
        date: NaiveDate,
    ) -> impl Future<Output = Result<Option<MorningLetter>>> + Send;

    /// 最新の Morning Letter を取得する
    fn get_latest_morning_letter(
        &self,
    ) -> impl Future<Output = Result<Option<MorningLetter>>> + Send;

    /// Morning Letter のソース (provenance) を取得する
    fn get_morning_letter_sources(
        &self,
        letter_id: Uuid,
    ) -> impl Future<Output = Result<Vec<MorningLetterSource>>> + Send;

    /// `before` 以前で最も新しい同一 timezone の Morning Letter を取得する。
    /// Since-yesterday band のリンク元に使う。
    fn get_previous_morning_letter(
        &self,
        edition_timezone: &str,
        before: NaiveDate,
    ) -> impl Future<Output = Result<Option<MorningLetter>>> + Send;
}
