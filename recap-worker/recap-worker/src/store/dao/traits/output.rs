//! OutputDao trait - Recap output operations

use std::collections::HashMap;
use std::future::Future;

use anyhow::Result;
use serde_json::Value;
use uuid::Uuid;

use crate::store::models::{
    ClusterWithEvidence, GenreWithSummary, RecapFinalSection, RecapJob, RecapOutput,
};

/// OutputDao - リキャップ出力のためのデータアクセス層
#[allow(dead_code)]
pub trait OutputDao: Send + Sync {
    /// 最終セクションを保存する
    fn save_final_section(
        &self,
        section: &RecapFinalSection,
    ) -> impl Future<Output = Result<i64>> + Send;

    /// リキャップ出力を挿入/更新する
    fn upsert_recap_output(&self, output: &RecapOutput) -> impl Future<Output = Result<()>> + Send;

    /// リキャップ出力のボディJSONを取得する
    fn get_recap_output_body_json(
        &self,
        job_id: Uuid,
        genre: &str,
    ) -> impl Future<Output = Result<Option<Value>>> + Send;

    /// 最新の完了済みジョブを取得する
    fn get_latest_completed_job(
        &self,
        window_days: i32,
    ) -> impl Future<Output = Result<Option<RecapJob>>> + Send;

    /// ジョブごとのジャンルを取得する
    fn get_genres_by_job(
        &self,
        job_id: Uuid,
    ) -> impl Future<Output = Result<Vec<GenreWithSummary>>> + Send;

    /// ジョブごとのクラスタを取得する
    fn get_clusters_by_job(
        &self,
        job_id: Uuid,
    ) -> impl Future<Output = Result<HashMap<String, Vec<ClusterWithEvidence>>>> + Send;
}
