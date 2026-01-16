//! EvaluationDao trait - Genre evaluation operations

use anyhow::Result;
use async_trait::async_trait;
use uuid::Uuid;

use crate::store::models::{GenreEvaluationMetric, GenreEvaluationRun};

/// EvaluationDao - ジャンル評価のためのデータアクセス層
#[allow(dead_code)]
#[async_trait]
pub trait EvaluationDao: Send + Sync {
    /// ジャンル評価を保存する
    async fn save_genre_evaluation(
        &self,
        run: &GenreEvaluationRun,
        metrics: &[GenreEvaluationMetric],
    ) -> Result<()>;

    /// ジャンル評価を取得する
    async fn get_genre_evaluation(
        &self,
        run_id: Uuid,
    ) -> Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>>;

    /// 最新のジャンル評価を取得する
    async fn get_latest_genre_evaluation(
        &self,
    ) -> Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>>;
}
