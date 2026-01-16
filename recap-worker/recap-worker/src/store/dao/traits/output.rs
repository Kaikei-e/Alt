//! OutputDao trait - Recap output operations

use anyhow::Result;
use async_trait::async_trait;
use serde_json::Value;
use std::collections::HashMap;
use uuid::Uuid;

use crate::store::models::{ClusterWithEvidence, GenreWithSummary, RecapFinalSection, RecapJob, RecapOutput};

/// OutputDao - リキャップ出力のためのデータアクセス層
#[allow(dead_code)]
#[async_trait]
pub trait OutputDao: Send + Sync {
    /// 最終セクションを保存する
    async fn save_final_section(&self, section: &RecapFinalSection) -> Result<i64>;

    /// リキャップ出力を挿入/更新する
    async fn upsert_recap_output(&self, output: &RecapOutput) -> Result<()>;

    /// リキャップ出力のボディJSONを取得する
    async fn get_recap_output_body_json(
        &self,
        job_id: Uuid,
        genre: &str,
    ) -> Result<Option<Value>>;

    /// 最新の完了済みジョブを取得する
    async fn get_latest_completed_job(&self, window_days: i32) -> Result<Option<RecapJob>>;

    /// ジョブごとのジャンルを取得する
    async fn get_genres_by_job(&self, job_id: Uuid) -> Result<Vec<GenreWithSummary>>;

    /// ジョブごとのクラスタを取得する
    async fn get_clusters_by_job(
        &self,
        job_id: Uuid,
    ) -> Result<HashMap<String, Vec<ClusterWithEvidence>>>;
}
