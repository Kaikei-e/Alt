//! JobStatusDao trait - Job status dashboard operations

use std::future::Future;

use anyhow::Result;
use uuid::Uuid;

use crate::store::models::{ExtendedRecapJob, JobStats};

/// JobStatusDao - ジョブステータスダッシュボードのためのデータアクセス層
#[allow(dead_code)]
pub trait JobStatusDao: Send + Sync {
    /// 拡張ジョブ一覧を取得する
    fn get_extended_jobs(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> impl Future<Output = Result<Vec<ExtendedRecapJob>>> + Send;

    /// ユーザーのジョブ一覧を取得する
    fn get_user_jobs(
        &self,
        user_id: Uuid,
        window_seconds: i64,
        limit: i64,
    ) -> impl Future<Output = Result<Vec<ExtendedRecapJob>>> + Send;

    /// 実行中のジョブを取得する
    fn get_running_job(&self) -> impl Future<Output = Result<Option<ExtendedRecapJob>>> + Send;

    /// ジョブ統計を取得する
    fn get_job_stats(&self) -> impl Future<Output = Result<JobStats>> + Send;

    /// ジョブの特定ユーザーの記事数を取得する
    fn get_user_article_count_for_job(
        &self,
        job_id: Uuid,
        user_id: Uuid,
    ) -> impl Future<Output = Result<i32>> + Send;

    /// ジョブの全記事数を取得する
    fn get_total_article_count_for_job(
        &self,
        job_id: Uuid,
    ) -> impl Future<Output = Result<i32>> + Send;

    /// ジャンルの進捗を取得する
    fn get_genre_progress(
        &self,
        job_id: Uuid,
    ) -> impl Future<Output = Result<Vec<(String, String, Option<i32>)>>> + Send;

    /// 完了したステージを取得する
    fn get_completed_stages(
        &self,
        job_id: Uuid,
    ) -> impl Future<Output = Result<Vec<String>>> + Send;

    /// ユーザートリガーのジョブを作成する
    fn create_user_triggered_job(
        &self,
        job_id: Uuid,
        user_id: Uuid,
        note: Option<&str>,
    ) -> impl Future<Output = Result<()>> + Send;

    /// ユーザーのジョブ数を取得する
    fn get_user_jobs_count(
        &self,
        user_id: Uuid,
        window_seconds: i64,
    ) -> impl Future<Output = Result<i32>> + Send;
}
