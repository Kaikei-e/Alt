//! GenreLearningDao trait - Genre learning operations

use std::future::Future;

use anyhow::Result;

use crate::store::models::{GenreLearningRecord, GraphEdgeRecord};

/// GenreLearningDao - ジャンル学習のためのデータアクセス層
#[allow(dead_code)]
pub trait GenreLearningDao: Send + Sync {
    /// タグラベルグラフを読み込む
    fn load_tag_label_graph(
        &self,
        window_label: &str,
    ) -> impl Future<Output = Result<Vec<GraphEdgeRecord>>> + Send;

    /// ジャンル学習レコードを挿入/更新する
    fn upsert_genre_learning_record(
        &self,
        record: &GenreLearningRecord,
    ) -> impl Future<Output = Result<()>> + Send;

    /// ジャンル学習レコードを一括挿入/更新する
    fn upsert_genre_learning_records_bulk(
        &self,
        records: &[GenreLearningRecord],
    ) -> impl Future<Output = Result<()>> + Send;
}
