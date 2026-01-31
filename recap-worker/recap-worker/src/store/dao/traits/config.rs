//! ConfigDao trait - Worker configuration operations

use std::future::Future;

use anyhow::Result;
use serde_json::Value;

/// ConfigDao - ワーカー設定のためのデータアクセス層
#[allow(dead_code)]
pub trait ConfigDao: Send + Sync {
    /// 最新のワーカー設定を取得する
    fn get_latest_worker_config(
        &self,
        config_type: &str,
    ) -> impl Future<Output = Result<Option<Value>>> + Send;

    /// ワーカー設定を挿入する
    fn insert_worker_config(
        &self,
        config_type: &str,
        config_payload: &Value,
        source: &str,
        metadata: Option<&Value>,
    ) -> impl Future<Output = Result<()>> + Send;
}
