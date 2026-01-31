//! EvaluationDao trait - Genre evaluation operations

use std::future::Future;

use anyhow::Result;
use uuid::Uuid;

use crate::store::models::{GenreEvaluationMetric, GenreEvaluationRun};

/// EvaluationDao - ジャンル評価のためのデータアクセス層
#[allow(dead_code)]
pub trait EvaluationDao: Send + Sync {
    /// ジャンル評価を保存する
    fn save_genre_evaluation(
        &self,
        run: &GenreEvaluationRun,
        metrics: &[GenreEvaluationMetric],
    ) -> impl Future<Output = Result<()>> + Send;

    /// ジャンル評価を取得する
    fn get_genre_evaluation(
        &self,
        run_id: Uuid,
    ) -> impl Future<Output = Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>>> + Send;

    /// 最新のジャンル評価を取得する
    fn get_latest_genre_evaluation(
        &self,
    ) -> impl Future<Output = Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>>> + Send;
}
