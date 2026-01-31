//! Dispatch result types.

use std::collections::HashMap;

use serde::{Deserialize, Serialize};
use uuid::Uuid;

use crate::clients::subworker::ClusteringResponse;

/// ディスパッチ結果。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct DispatchResult {
    pub(crate) job_id: Uuid,
    pub(crate) genre_results: HashMap<String, GenreResult>,
    pub(crate) success_count: usize,
    pub(crate) failure_count: usize,
    /// 設定された全ジャンルリスト（証拠がないジャンルも含む）
    pub(crate) all_genres: Vec<String>,
}

/// ジャンル別の処理結果。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct GenreResult {
    /// The genre name.
    pub(crate) genre: String,
    /// Clustering response from subworker (used in persist and pulse stages).
    pub(crate) clustering_response: Option<ClusteringResponse>,
    pub(crate) summary_response_id: Option<String>,
    pub(crate) summary_response: Option<crate::clients::news_creator::SummaryResponse>,
    pub(crate) error: Option<String>,
}

/// ステージ状態保存用の軽量版ディスパッチ結果。
/// clustering_responseとsummary_responseを除外してサイズを削減。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct DispatchResultLightweight {
    pub(crate) job_id: Uuid,
    pub(crate) genre_results: HashMap<String, GenreResultLightweight>,
    pub(crate) success_count: usize,
    pub(crate) failure_count: usize,
    /// 設定された全ジャンルリスト（証拠がないジャンルも含む）
    pub(crate) all_genres: Vec<String>,
}

/// ステージ状態保存用の軽量版ジャンル結果。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct GenreResultLightweight {
    pub(crate) genre: String,
    /// クラスタリングのrun_id（データベースから再取得可能）
    pub(crate) clustering_run_id: Option<i64>,
    pub(crate) summary_response_id: Option<String>,
    pub(crate) error: Option<String>,
}

impl DispatchResult {
    /// 軽量版に変換（大きなデータを除外）
    pub(crate) fn to_lightweight(&self) -> DispatchResultLightweight {
        let genre_results: HashMap<String, GenreResultLightweight> = self
            .genre_results
            .iter()
            .map(|(genre, result)| {
                let clustering_run_id = result.clustering_response.as_ref().map(|cr| cr.run_id);
                (
                    genre.clone(),
                    GenreResultLightweight {
                        genre: result.genre.clone(),
                        clustering_run_id,
                        summary_response_id: result.summary_response_id.clone(),
                        error: result.error.clone(),
                    },
                )
            })
            .collect();

        DispatchResultLightweight {
            job_id: self.job_id,
            genre_results,
            success_count: self.success_count,
            failure_count: self.failure_count,
            all_genres: self.all_genres.clone(),
        }
    }
}
