//! StageDao trait - Stage management operations

use std::future::Future;

use anyhow::Result;
use serde_json::Value;
use uuid::Uuid;

/// StageDao - ステージ管理のためのデータアクセス層
#[allow(dead_code)]
pub trait StageDao: Send + Sync {
    /// ステージの実行ログを記録する
    fn insert_stage_log(
        &self,
        job_id: Uuid,
        stage: &str,
        status: &str,
        message: Option<&str>,
    ) -> impl Future<Output = Result<()>> + Send;

    /// ステージの状態（チェックポイント）を保存する
    fn save_stage_state(
        &self,
        job_id: Uuid,
        stage: &str,
        state_data: &Value,
    ) -> impl Future<Output = Result<()>> + Send;

    /// ステージの状態（チェックポイント）を読み込む
    fn load_stage_state(
        &self,
        job_id: Uuid,
        stage: &str,
    ) -> impl Future<Output = Result<Option<Value>>> + Send;

    /// 失敗したタスクを記録する
    fn insert_failed_task(
        &self,
        job_id: Uuid,
        stage: &str,
        payload: Option<&Value>,
        error: Option<&str>,
    ) -> impl Future<Output = Result<()>> + Send;
}
