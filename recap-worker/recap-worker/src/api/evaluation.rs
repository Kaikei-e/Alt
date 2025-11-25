//! ジャンル分類の評価APIエンドポイント
//!
//! Golden Datasetを使用して、ジャンル分類器の精度を評価します。
//! Precision、Recall、F1-Scoreを計算し、混同行列を出力します。

use std::collections::HashMap;
use std::fs;
use std::path::PathBuf;

use anyhow::Context;
use axum::{Json, extract::State, http::StatusCode, response::IntoResponse};
use serde::{Deserialize, Serialize};
use tracing::{error, info};

use crate::app::AppState;
use crate::classification::{ClassificationLanguage, GenreClassifier};
use crate::store::models::{GenreEvaluationMetric, GenreEvaluationRun};

#[derive(Deserialize, Debug)]
pub(crate) struct EvaluateRequest {
    /// Path to golden dataset JSON file (optional, defaults to /app/data/golden_classification.json)
    #[serde(default)]
    data_path: Option<String>,
}

#[derive(Serialize, Debug)]
pub(crate) struct EvaluateResponse {
    run_id: uuid::Uuid,
    created_at: chrono::DateTime<chrono::Utc>,
}

#[derive(Deserialize, Debug)]
#[serde(untagged)]
enum GoldenItem {
    /// New format: single content field
    Simple {
        id: String,
        content: String,
        expected_genres: Vec<String>,
    },
    /// Legacy format: title and body fields
    #[allow(dead_code)]
    Legacy {
        id: String,
        #[serde(default)]
        title: Option<String>,
        body: String,
        #[serde(default)]
        lang: Option<String>,
        expected_genres: Vec<String>,
    },
}

impl GoldenItem {
    #[allow(clippy::match_same_arms)]
    fn id(&self) -> &str {
        match self {
            Self::Simple { id, .. } | Self::Legacy { id, .. } => id,
        }
    }

    fn content(&self) -> String {
        match self {
            Self::Simple { content, .. } => content.clone(),
            Self::Legacy { title, body, .. } => {
                if let Some(title) = title {
                    format!("{} {}", title, body)
                } else {
                    body.clone()
                }
            }
        }
    }

    #[allow(clippy::match_same_arms)]
    fn expected_genres(&self) -> &[String] {
        match self {
            Self::Simple {
                expected_genres, ..
            }
            | Self::Legacy {
                expected_genres, ..
            } => expected_genres,
        }
    }
}

#[derive(Debug, Default)]
struct ConfusionMatrix {
    /// True Positives per genre
    tp: HashMap<String, usize>,
    /// False Positives per genre
    fp: HashMap<String, usize>,
    /// False Negatives per genre
    fn_count: HashMap<String, usize>,
}

impl ConfusionMatrix {
    fn new() -> Self {
        Self::default()
    }

    fn add(&mut self, expected: &[String], predicted: &[String]) {
        let expected_set: std::collections::HashSet<String> =
            expected.iter().map(|s| s.to_lowercase()).collect();
        let predicted_set: std::collections::HashSet<String> =
            predicted.iter().map(|s| s.to_lowercase()).collect();

        // Count TP, FP, FN for each genre
        let all_genres: std::collections::HashSet<String> =
            expected_set.union(&predicted_set).cloned().collect();

        for genre in all_genres {
            let expected_has = expected_set.contains(&genre);
            let predicted_has = predicted_set.contains(&genre);

            if expected_has && predicted_has {
                *self.tp.entry(genre.clone()).or_insert(0) += 1;
            } else if !expected_has && predicted_has {
                *self.fp.entry(genre.clone()).or_insert(0) += 1;
            } else if expected_has && !predicted_has {
                *self.fn_count.entry(genre.clone()).or_insert(0) += 1;
            }
        }
    }

    fn precision(&self, genre: &str) -> f64 {
        let tp = self.tp.get(genre).copied().unwrap_or(0) as f64;
        let fp = self.fp.get(genre).copied().unwrap_or(0) as f64;
        let denominator = tp + fp;
        if denominator == 0.0 {
            0.0
        } else {
            tp / denominator
        }
    }

    fn recall(&self, genre: &str) -> f64 {
        let tp = self.tp.get(genre).copied().unwrap_or(0) as f64;
        let fn_val = self.fn_count.get(genre).copied().unwrap_or(0) as f64;
        let denominator = tp + fn_val;
        if denominator == 0.0 {
            0.0
        } else {
            tp / denominator
        }
    }

    fn f1_score(&self, genre: &str) -> f64 {
        let precision = self.precision(genre);
        let recall = self.recall(genre);
        let denominator = precision + recall;
        if denominator == 0.0 {
            0.0
        } else {
            2.0 * (precision * recall) / denominator
        }
    }

    fn macro_precision(&self) -> f64 {
        let all_genres: std::collections::HashSet<String> = self
            .tp
            .keys()
            .chain(self.fp.keys())
            .chain(self.fn_count.keys())
            .cloned()
            .collect();

        if all_genres.is_empty() {
            return 0.0;
        }

        let sum: f64 = all_genres.iter().map(|g| self.precision(g)).sum();
        sum / all_genres.len() as f64
    }

    fn macro_recall(&self) -> f64 {
        let all_genres: std::collections::HashSet<String> = self
            .tp
            .keys()
            .chain(self.fp.keys())
            .chain(self.fn_count.keys())
            .cloned()
            .collect();

        if all_genres.is_empty() {
            return 0.0;
        }

        let sum: f64 = all_genres.iter().map(|g| self.recall(g)).sum();
        sum / all_genres.len() as f64
    }

    fn macro_f1(&self) -> f64 {
        let macro_precision = self.macro_precision();
        let macro_recall = self.macro_recall();
        let denominator = macro_precision + macro_recall;
        if denominator == 0.0 {
            0.0
        } else {
            2.0 * (macro_precision * macro_recall) / denominator
        }
    }

    fn total_tp(&self) -> usize {
        self.tp.values().sum()
    }

    fn total_fp(&self) -> usize {
        self.fp.values().sum()
    }

    fn total_fn(&self) -> usize {
        self.fn_count.values().sum()
    }
}

fn load_golden_dataset(path: &PathBuf) -> anyhow::Result<Vec<GoldenItem>> {
    // Check if file exists
    if !path.exists() {
        anyhow::bail!("Golden dataset file does not exist: {}", path.display());
    }

    let content = fs::read_to_string(path)
        .with_context(|| format!("Failed to read golden dataset from {}", path.display()))?;

    if content.trim().is_empty() {
        anyhow::bail!("Golden dataset file is empty: {}", path.display());
    }

    let items: Vec<GoldenItem> = serde_json::from_str(&content).with_context(|| {
        format!(
            "Failed to parse golden dataset JSON from {}. Content preview: {}",
            path.display(),
            content.chars().take(200).collect::<String>()
        )
    })?;

    if items.is_empty() {
        anyhow::bail!("Golden dataset contains no items: {}", path.display());
    }

    Ok(items)
}

#[allow(dead_code)]
fn determine_dataset_path(request: &EvaluateRequest) -> PathBuf {
    let data_path = request.data_path.clone().unwrap_or_else(|| {
        std::env::var("RECAP_GOLDEN_DATASET_PATH")
            .unwrap_or_else(|_| "/app/data/golden_classification.json".to_string())
    });
    PathBuf::from(data_path)
}

#[allow(dead_code)]
fn run_evaluation(golden_data: &[GoldenItem]) -> ConfusionMatrix {
    let classifier = GenreClassifier::new_default();
    let mut confusion_matrix = ConfusionMatrix::new();

    for item in golden_data {
        let content = item.content();
        // Split content into title and body (first sentence as title, rest as body)
        let content_str: &str = &content;
        let (title, body) = content_str
            .char_indices()
            .find(|(_, ch)| matches!(ch, '。' | '.'))
            .map_or_else(
                || ("", content_str),
                |(pos, ch)| {
                    // Calculate the byte position after the delimiter character
                    let delimiter_len = ch.len_utf8();
                    let (title_part, body_part) = content_str.split_at(pos + delimiter_len);
                    (title_part.trim(), body_part.trim())
                },
            );
        let language = ClassificationLanguage::Unknown; // Auto-detect

        match classifier.predict(title, body, language) {
            Ok(result) => {
                let predicted = &result.top_genres;
                confusion_matrix.add(item.expected_genres(), predicted);
            }
            Err(e) => {
                error!(error = %e, item_id = %item.id(), "Failed to classify item");
                // Continue with other items
            }
        }
    }

    confusion_matrix
}

#[allow(dead_code)]
fn build_evaluation_metrics(confusion_matrix: &ConfusionMatrix) -> Vec<GenreEvaluationMetric> {
    // Collect all genres
    let all_genres: std::collections::HashSet<String> = confusion_matrix
        .tp
        .keys()
        .chain(confusion_matrix.fp.keys())
        .chain(confusion_matrix.fn_count.keys())
        .cloned()
        .collect();

    let mut sorted_genres: Vec<String> = all_genres.into_iter().collect();
    sorted_genres.sort();

    // Build per-genre metrics for DB storage
    sorted_genres
        .iter()
        .map(|genre| {
            GenreEvaluationMetric::new(
                genre.clone(),
                confusion_matrix.tp.get(genre).copied().unwrap_or(0),
                confusion_matrix.fp.get(genre).copied().unwrap_or(0),
                confusion_matrix.fn_count.get(genre).copied().unwrap_or(0),
                confusion_matrix.precision(genre),
                confusion_matrix.recall(genre),
                confusion_matrix.f1_score(genre),
            )
        })
        .collect()
}

/// POST /v1/evaluation/genres
/// Golden Datasetを使用してジャンル分類の精度を評価する
/// 評価結果はDBに保存され、run_idが返される
pub(crate) async fn evaluate_genres(
    State(state): State<AppState>,
    Json(request): Json<EvaluateRequest>,
) -> impl IntoResponse {
    let path = determine_dataset_path(&request);
    info!("Loading golden dataset from: {}", path.display());

    let golden_data = match load_golden_dataset(&path) {
        Ok(data) => data,
        Err(e) => {
            error!(
                error = %e,
                path = %path.display(),
                "Failed to load golden dataset"
            );
            return (
                StatusCode::BAD_REQUEST,
                Json(serde_json::json!({
                    "error": format!("Failed to load golden dataset: {}", e),
                    "path": path.display().to_string(),
                    "hint": "Check if the file exists and contains valid JSON array of items with 'id', 'content', and 'expected_genres' fields"
                })),
            )
                .into_response();
        }
    };

    info!("Loaded {} items", golden_data.len());

    let confusion_matrix = run_evaluation(&golden_data);
    let per_genre_metrics = build_evaluation_metrics(&confusion_matrix);

    let evaluation_run = GenreEvaluationRun::new(
        path.display().to_string(),
        golden_data.len(),
        confusion_matrix.macro_precision(),
        confusion_matrix.macro_recall(),
        confusion_matrix.macro_f1(),
        confusion_matrix.total_tp(),
        confusion_matrix.total_fp(),
        confusion_matrix.total_fn(),
    );

    info!(
        run_id = %evaluation_run.run_id,
        total_items = golden_data.len(),
        macro_precision = confusion_matrix.macro_precision(),
        macro_recall = confusion_matrix.macro_recall(),
        macro_f1 = confusion_matrix.macro_f1(),
        summary_tp = confusion_matrix.total_tp(),
        summary_fp = confusion_matrix.total_fp(),
        summary_fn = confusion_matrix.total_fn(),
        "Evaluation completed"
    );

    match state
        .dao()
        .save_genre_evaluation(&evaluation_run, &per_genre_metrics)
        .await
    {
        Ok(()) => {
            info!(run_id = %evaluation_run.run_id, "Evaluation results saved to database");
        }
        Err(e) => {
            error!(
                error = %e,
                run_id = %evaluation_run.run_id,
                "Failed to save evaluation results to database"
            );
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(serde_json::json!({
                    "error": "Failed to save evaluation results",
                    "run_id": evaluation_run.run_id,
                })),
            )
                .into_response();
        }
    }

    let response = EvaluateResponse {
        run_id: evaluation_run.run_id,
        created_at: chrono::Utc::now(),
    };

    (StatusCode::OK, Json(response)).into_response()
}

#[derive(Serialize, Debug)]
pub(crate) struct EvaluationResultResponse {
    run: GenreEvaluationRun,
    metrics: Vec<GenreEvaluationMetric>,
}

/// GET /v1/evaluation/genres/{run_id}
/// 指定されたrun_idの評価結果を取得する
pub(crate) async fn get_evaluation_result(
    State(state): State<AppState>,
    axum::extract::Path(run_id): axum::extract::Path<uuid::Uuid>,
) -> impl IntoResponse {
    match state.dao().get_genre_evaluation(run_id).await {
        Ok(Some((run, metrics))) => {
            let response = EvaluationResultResponse { run, metrics };
            (StatusCode::OK, Json(response)).into_response()
        }
        Ok(None) => (
            StatusCode::NOT_FOUND,
            Json(serde_json::json!({
                "error": "Evaluation result not found",
                "run_id": run_id,
            })),
        )
            .into_response(),
        Err(e) => {
            error!(
                error = %e,
                run_id = %run_id,
                "Failed to fetch evaluation result"
            );
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(serde_json::json!({
                    "error": "Failed to fetch evaluation result",
                    "run_id": run_id,
                })),
            )
                .into_response()
        }
    }
}

/// GET /v1/evaluation/genres/latest
/// 最新の評価結果を取得する
pub(crate) async fn get_latest_evaluation_result(
    State(state): State<AppState>,
) -> impl IntoResponse {
    match state.dao().get_latest_genre_evaluation().await {
        Ok(Some((run, metrics))) => {
            let response = EvaluationResultResponse { run, metrics };
            (StatusCode::OK, Json(response)).into_response()
        }
        Ok(None) => (
            StatusCode::NOT_FOUND,
            Json(serde_json::json!({
                "error": "No evaluation results found",
            })),
        )
            .into_response(),
        Err(e) => {
            error!(error = %e, "Failed to fetch latest evaluation result");
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(serde_json::json!({
                    "error": "Failed to fetch latest evaluation result",
                })),
            )
                .into_response()
        }
    }
}
