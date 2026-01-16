//! SubworkerDao trait - Subworker run and cluster operations

use anyhow::Result;
use async_trait::async_trait;

use crate::store::models::{
    DiagnosticEntry, NewSubworkerRun, PersistedCluster, PersistedGenre, SubworkerRunStatus,
};

/// SubworkerDao - サブワーカー実行・クラスタのためのデータアクセス層
#[allow(dead_code)]
#[async_trait]
pub trait SubworkerDao: Send + Sync {
    /// サブワーカー実行を挿入する
    async fn insert_subworker_run(&self, run: &NewSubworkerRun) -> Result<i64>;

    /// サブワーカー実行を成功としてマークする
    async fn mark_subworker_run_success(
        &self,
        run_id: i64,
        cluster_count: i32,
        response_payload: &serde_json::Value,
    ) -> Result<()>;

    /// サブワーカー実行を失敗としてマークする
    async fn mark_subworker_run_failure(
        &self,
        run_id: i64,
        status: SubworkerRunStatus,
        error_message: &str,
    ) -> Result<()>;

    /// クラスタを挿入する
    async fn insert_clusters(&self, run_id: i64, clusters: &[PersistedCluster]) -> Result<()>;

    /// 診断情報を挿入/更新する
    async fn upsert_diagnostics(&self, run_id: i64, diagnostics: &[DiagnosticEntry]) -> Result<()>;

    /// ジャンルを挿入/更新する
    async fn upsert_genre(&self, genre: &PersistedGenre) -> Result<()>;
}
