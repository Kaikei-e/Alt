//! ConfigDao trait - Worker configuration operations

use anyhow::Result;
use async_trait::async_trait;
use serde_json::Value;

/// ConfigDao - ワーカー設定のためのデータアクセス層
#[allow(dead_code)]
#[async_trait]
pub trait ConfigDao: Send + Sync {
    /// 最新のワーカー設定を取得する
    async fn get_latest_worker_config(&self, config_type: &str) -> Result<Option<Value>>;

    /// ワーカー設定を挿入する
    async fn insert_worker_config(
        &self,
        config_type: &str,
        config_payload: &Value,
        source: &str,
        metadata: Option<&Value>,
    ) -> Result<()>;
}
