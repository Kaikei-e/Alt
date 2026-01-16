//! JobDao trait - Job management operations

use anyhow::Result;
use async_trait::async_trait;
use chrono::{DateTime, Utc};
use uuid::Uuid;

use crate::store::dao::types::{JobStatus, JobStatusTransition, StatusTransitionActor};

/// JobDao - ジョブ管理のためのデータアクセス層
#[allow(dead_code)]
#[async_trait]
pub trait JobDao: Send + Sync {
    /// アドバイザリロックを取得し、新しいジョブを作成する
    async fn create_job_with_lock(
        &self,
        job_id: Uuid,
        note: Option<&str>,
    ) -> Result<Option<Uuid>>;

    /// 指定されたjob_idのジョブが存在するかチェックする
    async fn job_exists(&self, job_id: Uuid) -> Result<bool>;

    /// 再開可能なジョブを探す
    async fn find_resumable_job(&self) -> Result<Option<(Uuid, JobStatus, Option<String>)>>;

    /// ジョブのステータスと最終ステージを更新する
    async fn update_job_status(
        &self,
        job_id: Uuid,
        status: JobStatus,
        last_stage: Option<&str>,
    ) -> Result<()>;

    /// ダッシュボード用に全ジョブを取得する
    async fn get_recap_jobs(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> Result<Vec<(Uuid, String, Option<String>, DateTime<Utc>, DateTime<Utc>)>>;

    /// 指定された保持期間より古いジョブを削除する
    async fn delete_old_jobs(&self, retention_days: i64) -> Result<u64>;

    /// ステータス遷移をイミュータブルな履歴テーブルに記録する
    async fn record_status_transition(
        &self,
        job_id: Uuid,
        status: JobStatus,
        stage: Option<&str>,
        reason: Option<&str>,
        actor: StatusTransitionActor,
    ) -> Result<i64>;

    /// ジョブのステータスを更新し、同時に履歴テーブルにも記録する（アトミック）
    async fn update_job_status_with_history(
        &self,
        job_id: Uuid,
        status: JobStatus,
        last_stage: Option<&str>,
        reason: Option<&str>,
    ) -> Result<()>;

    /// 指定されたジョブのステータス履歴を取得する
    async fn get_status_history(&self, job_id: Uuid) -> Result<Vec<JobStatusTransition>>;
}
