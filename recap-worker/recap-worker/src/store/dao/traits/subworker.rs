//! SubworkerDao trait - Subworker run and cluster operations

use std::future::Future;

use anyhow::Result;

use crate::store::models::{
    DiagnosticEntry, NewSubworkerRun, PersistedCluster, PersistedGenre, SubworkerRunStatus,
};

/// SubworkerDao - サブワーカー実行・クラスタのためのデータアクセス層
#[allow(dead_code)]
pub trait SubworkerDao: Send + Sync {
    /// サブワーカー実行を挿入する
    fn insert_subworker_run(
        &self,
        run: &NewSubworkerRun,
    ) -> impl Future<Output = Result<i64>> + Send;

    /// サブワーカー実行を成功としてマークする
    fn mark_subworker_run_success(
        &self,
        run_id: i64,
        cluster_count: i32,
        response_payload: &serde_json::Value,
    ) -> impl Future<Output = Result<()>> + Send;

    /// サブワーカー実行を失敗としてマークする
    fn mark_subworker_run_failure(
        &self,
        run_id: i64,
        status: SubworkerRunStatus,
        error_message: &str,
    ) -> impl Future<Output = Result<()>> + Send;

    /// クラスタを挿入する
    fn insert_clusters(
        &self,
        run_id: i64,
        clusters: &[PersistedCluster],
    ) -> impl Future<Output = Result<()>> + Send;

    /// 診断情報を挿入/更新する
    fn upsert_diagnostics(
        &self,
        run_id: i64,
        diagnostics: &[DiagnosticEntry],
    ) -> impl Future<Output = Result<()>> + Send;

    /// ジャンルを挿入/更新する
    fn upsert_genre(&self, genre: &PersistedGenre) -> impl Future<Output = Result<()>> + Send;
}
