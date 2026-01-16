//! GenreLearningDao trait - Genre learning operations

use anyhow::Result;
use async_trait::async_trait;

use crate::store::models::{GenreLearningRecord, GraphEdgeRecord};

/// GenreLearningDao - ジャンル学習のためのデータアクセス層
#[allow(dead_code)]
#[async_trait]
pub trait GenreLearningDao: Send + Sync {
    /// タグラベルグラフを読み込む
    async fn load_tag_label_graph(&self, window_label: &str) -> Result<Vec<GraphEdgeRecord>>;

    /// ジャンル学習レコードを挿入/更新する
    async fn upsert_genre_learning_record(&self, record: &GenreLearningRecord) -> Result<()>;

    /// ジャンル学習レコードを一括挿入/更新する
    async fn upsert_genre_learning_records_bulk(&self, records: &[GenreLearningRecord])
        -> Result<()>;
}
