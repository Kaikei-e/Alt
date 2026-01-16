//! StageDao trait - Stage management operations

use anyhow::Result;
use async_trait::async_trait;
use serde_json::Value;
use uuid::Uuid;

/// StageDao - ステージ管理のためのデータアクセス層
#[allow(dead_code)]
#[async_trait]
pub trait StageDao: Send + Sync {
    /// ステージの実行ログを記録する
    async fn insert_stage_log(
        &self,
        job_id: Uuid,
        stage: &str,
        status: &str,
        message: Option<&str>,
    ) -> Result<()>;

    /// ステージの状態（チェックポイント）を保存する
    async fn save_stage_state(
        &self,
        job_id: Uuid,
        stage: &str,
        state_data: &Value,
    ) -> Result<()>;

    /// ステージの状態（チェックポイント）を読み込む
    async fn load_stage_state(&self, job_id: Uuid, stage: &str) -> Result<Option<Value>>;

    /// 失敗したタスクを記録する
    async fn insert_failed_task(
        &self,
        job_id: Uuid,
        stage: &str,
        payload: Option<&Value>,
        error: Option<&str>,
    ) -> Result<()>;
}
