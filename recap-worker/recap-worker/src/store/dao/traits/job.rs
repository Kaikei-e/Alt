//! JobDao trait - Job management operations

use std::future::Future;

use anyhow::Result;
use chrono::{DateTime, Utc};
use uuid::Uuid;

use crate::store::dao::types::{JobStatus, JobStatusTransition, StatusTransitionActor};

/// JobDao - ジョブ管理のためのデータアクセス層
#[allow(dead_code, clippy::type_complexity)]
pub trait JobDao: Send + Sync {
    /// アドバイザリロックを取得し、新しいジョブを作成する（デフォルト7日間）
    fn create_job_with_lock(
        &self,
        job_id: Uuid,
        note: Option<&str>,
    ) -> impl Future<Output = Result<Option<Uuid>>> + Send;

    /// アドバイザリロックを取得し、新しいジョブを作成する（ウィンドウ日数指定あり）
    fn create_job_with_lock_and_window(
        &self,
        job_id: Uuid,
        note: Option<&str>,
        window_days: u32,
    ) -> impl Future<Output = Result<Option<Uuid>>> + Send;

    /// 指定されたjob_idのジョブが存在するかチェックする
    fn job_exists(&self, job_id: Uuid) -> impl Future<Output = Result<bool>> + Send;

    /// 再開可能なジョブを探す
    fn find_resumable_job(
        &self,
    ) -> impl Future<Output = Result<Option<(Uuid, JobStatus, Option<String>)>>> + Send;

    /// ジョブのステータスと最終ステージを更新する
    fn update_job_status(
        &self,
        job_id: Uuid,
        status: JobStatus,
        last_stage: Option<&str>,
    ) -> impl Future<Output = Result<()>> + Send;

    /// ダッシュボード用に全ジョブを取得する
    fn get_recap_jobs(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> impl Future<
        Output = Result<Vec<(Uuid, String, Option<String>, DateTime<Utc>, DateTime<Utc>)>>,
    > + Send;

    /// 指定された保持期間より古いジョブを削除する
    fn delete_old_jobs(&self, retention_days: i64) -> impl Future<Output = Result<u64>> + Send;

    /// ステータス遷移をイミュータブルな履歴テーブルに記録する
    fn record_status_transition(
        &self,
        job_id: Uuid,
        status: JobStatus,
        stage: Option<&str>,
        reason: Option<&str>,
        actor: StatusTransitionActor,
    ) -> impl Future<Output = Result<i64>> + Send;

    /// ジョブのステータスを更新し、同時に履歴テーブルにも記録する（アトミック）
    fn update_job_status_with_history(
        &self,
        job_id: Uuid,
        status: JobStatus,
        last_stage: Option<&str>,
        reason: Option<&str>,
    ) -> impl Future<Output = Result<()>> + Send;

    /// 指定されたジョブのステータス履歴を取得する
    fn get_status_history(
        &self,
        job_id: Uuid,
    ) -> impl Future<Output = Result<Vec<JobStatusTransition>>> + Send;
}
