//! JobStatusDao trait - Job status dashboard operations

use anyhow::Result;
use async_trait::async_trait;
use uuid::Uuid;

use crate::store::models::{ExtendedRecapJob, JobStats};

/// JobStatusDao - ジョブステータスダッシュボードのためのデータアクセス層
#[allow(dead_code)]
#[async_trait]
pub trait JobStatusDao: Send + Sync {
    /// 拡張ジョブ一覧を取得する
    async fn get_extended_jobs(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> Result<Vec<ExtendedRecapJob>>;

    /// ユーザーのジョブ一覧を取得する
    async fn get_user_jobs(
        &self,
        user_id: Uuid,
        window_seconds: i64,
        limit: i64,
    ) -> Result<Vec<ExtendedRecapJob>>;

    /// 実行中のジョブを取得する
    async fn get_running_job(&self) -> Result<Option<ExtendedRecapJob>>;

    /// ジョブ統計を取得する
    async fn get_job_stats(&self) -> Result<JobStats>;

    /// ジョブの特定ユーザーの記事数を取得する
    async fn get_user_article_count_for_job(&self, job_id: Uuid, user_id: Uuid) -> Result<i32>;

    /// ジョブの全記事数を取得する
    async fn get_total_article_count_for_job(&self, job_id: Uuid) -> Result<i32>;

    /// ジャンルの進捗を取得する
    async fn get_genre_progress(&self, job_id: Uuid) -> Result<Vec<(String, String, Option<i32>)>>;

    /// 完了したステージを取得する
    async fn get_completed_stages(&self, job_id: Uuid) -> Result<Vec<String>>;

    /// ユーザートリガーのジョブを作成する
    async fn create_user_triggered_job(
        &self,
        job_id: Uuid,
        user_id: Uuid,
        note: Option<&str>,
    ) -> Result<()>;

    /// ユーザーのジョブ数を取得する
    async fn get_user_jobs_count(&self, user_id: Uuid, window_seconds: i64) -> Result<i32>;
}
