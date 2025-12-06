//! ジャンル分類の評価APIエンドポイント
//!
//! Golden Datasetを使用して、ジャンル分類器の精度を評価します。
//! Precision、Recall、F1-Scoreを計算し、混同行列を出力します。

use std::collections::HashMap;
use std::fs;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;

use anyhow::Context;
use axum::{Json, extract::State, http::StatusCode, response::IntoResponse};
use serde::{Deserialize, Serialize};
use tracing::{error, info};

use crate::app::AppState;
use crate::classification::{ClassificationLanguage, GenreClassifier};
use crate::pipeline::dedup::DeduplicatedArticle;
use crate::pipeline::genre::GenreCandidate;
use crate::pipeline::genre_refine::{
    DbTagLabelGraphSource, DefaultRefineEngine, RefineConfig, RefineEngine, TagFallbackMode,
    TagProfile,
};
use crate::scheduler::JobContext;
use crate::store::models::{GenreEvaluationMetric, GenreEvaluationRun};
use uuid::Uuid;

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

#[derive(Serialize, Debug)]
pub(crate) struct DatasetQualityReport {
    pub total_samples: usize,
    pub genre_count: usize,
    pub min_samples_per_genre: usize,
    pub max_samples_per_genre: usize,
    pub avg_samples_per_genre: f64,
    pub genres_below_threshold: Vec<String>,
    pub warnings: Vec<String>,
}

fn analyze_golden_dataset(items: &[GoldenItem]) -> DatasetQualityReport {
    let mut genre_counts: HashMap<String, usize> = HashMap::new();
    for item in items {
        for genre in item.expected_genres() {
            *genre_counts.entry(genre.to_lowercase()).or_insert(0) += 1;
        }
    }

    let genre_count = genre_counts.len();
    let total_samples = items.len();

    let samples_per_genre: Vec<usize> = genre_counts.values().copied().collect();
    let min_samples = samples_per_genre.iter().min().copied().unwrap_or(0);
    let max_samples = samples_per_genre.iter().max().copied().unwrap_or(0);
    let avg_samples = if genre_count > 0 {
        samples_per_genre.iter().sum::<usize>() as f64 / genre_count as f64
    } else {
        0.0
    };

    const THRESHOLD: usize = 5;
    let genres_below_threshold: Vec<String> = genre_counts
        .iter()
        .filter(|(_, count)| **count < THRESHOLD)
        .map(|(genre, _)| genre.clone())
        .collect();

    let mut warnings = Vec::new();
    if min_samples < THRESHOLD {
        warnings.push(format!(
            "{} genres have fewer than {} samples (statistically unstable)",
            genres_below_threshold.len(),
            THRESHOLD
        ));
    }
    if genre_count == 0 {
        warnings.push("No genres found in dataset".to_string());
    }
    if total_samples < 30 {
        warnings.push(format!(
            "Total samples ({}) is below recommended minimum (30)",
            total_samples
        ));
    }

    DatasetQualityReport {
        total_samples,
        genre_count,
        min_samples_per_genre: min_samples,
        max_samples_per_genre: max_samples,
        avg_samples_per_genre: avg_samples,
        genres_below_threshold,
        warnings,
    }
}

/// Golden Datasetのトップレベル構造
#[derive(Debug, Deserialize)]
#[allow(dead_code)]
struct GoldenDatasetRoot {
    #[serde(default)]
    schema_version: Option<String>,
    #[serde(default)]
    taxonomy_version: Option<String>,
    #[serde(default)]
    genres: Vec<String>,
    #[serde(default, rename = "facets_suggestion")]
    facets_suggestion: Vec<String>,
    items: Vec<GoldenItem>,
}

#[derive(Deserialize, Debug)]
#[serde(untagged)]
enum GoldenItem {
    /// Bilingual format: content_ja and content_en
    Bilingual {
        id: String,
        #[serde(default)]
        content_ja: Option<String>,
        #[serde(default)]
        content_en: Option<String>,
        #[serde(default)]
        content: Option<String>, // レガシー対応
        expected_genres: Vec<String>,
    },
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
            Self::Bilingual { id, .. } | Self::Simple { id, .. } | Self::Legacy { id, .. } => id,
        }
    }

    fn content(&self) -> String {
        match self {
            Self::Bilingual {
                content_en,
                content_ja,
                content,
                ..
            } => {
                // content_en優先、次にcontent_ja、最後にcontent
                if let Some(en) = content_en {
                    if !en.trim().is_empty() {
                        return en.clone();
                    }
                }
                if let Some(ja) = content_ja {
                    if !ja.trim().is_empty() {
                        return ja.clone();
                    }
                }
                if let Some(c) = content {
                    return c.clone();
                }
                String::new()
            }
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
            Self::Bilingual {
                expected_genres, ..
            }
            | Self::Simple {
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

        let sum: f64 = all_genres.iter().map(|g| self.f1_score(g)).sum();
        sum / all_genres.len() as f64
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

    /// Get all genres that appear in the confusion matrix
    fn all_genres(&self) -> std::collections::HashSet<String> {
        self.tp
            .keys()
            .chain(self.fp.keys())
            .chain(self.fn_count.keys())
            .cloned()
            .collect()
    }

    /// Macro F1 excluding genres with no support (TP + FN = 0)
    /// Returns (macro_f1_valid, valid_genre_count, undefined_genre_count)
    fn macro_f1_excluding_undefined(&self) -> (f64, usize, usize) {
        let all_genres = self.all_genres();
        let valid_genres: Vec<&String> = all_genres
            .iter()
            .filter(|g| {
                let support = self.tp.get(*g).copied().unwrap_or(0)
                    + self.fn_count.get(*g).copied().unwrap_or(0);
                support > 0
            })
            .collect();

        let undefined_count = all_genres.len() - valid_genres.len();
        if valid_genres.is_empty() {
            return (0.0, 0, undefined_count);
        }

        let sum: f64 = valid_genres.iter().map(|g| self.f1_score(g)).sum();
        (
            sum / valid_genres.len() as f64,
            valid_genres.len(),
            undefined_count,
        )
    }

    /// Micro-averaged precision: total TP / (total TP + total FP)
    fn micro_precision(&self) -> f64 {
        let total_tp = self.total_tp() as f64;
        let total_false_positives = self.total_fp() as f64;
        if total_tp + total_false_positives == 0.0 {
            0.0
        } else {
            total_tp / (total_tp + total_false_positives)
        }
    }

    /// Micro-averaged recall: total TP / (total TP + total FN)
    fn micro_recall(&self) -> f64 {
        let total_tp = self.total_tp() as f64;
        let total_fn = self.total_fn() as f64;
        if total_tp + total_fn == 0.0 {
            0.0
        } else {
            total_tp / (total_tp + total_fn)
        }
    }

    /// Micro-averaged F1: harmonic mean of micro precision and micro recall
    fn micro_f1(&self) -> f64 {
        let precision = self.micro_precision();
        let recall = self.micro_recall();
        if precision + recall == 0.0 {
            0.0
        } else {
            2.0 * precision * recall / (precision + recall)
        }
    }

    /// Weighted F1: F1 scores weighted by support (TP + FN) for each genre
    fn weighted_f1(&self) -> f64 {
        let mut weighted_sum = 0.0;
        let mut total_support = 0usize;

        for genre in self.all_genres() {
            let support = self.tp.get(&genre).copied().unwrap_or(0)
                + self.fn_count.get(&genre).copied().unwrap_or(0);
            if support > 0 {
                weighted_sum += self.f1_score(&genre) * support as f64;
                total_support += support;
            }
        }

        if total_support == 0 {
            0.0
        } else {
            weighted_sum / total_support as f64
        }
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

    // 新しいスキーマ（items配列あり）とレガシースキーマ（直接配列）の両方に対応
    let items: Vec<GoldenItem> =
        if let Ok(root) = serde_json::from_str::<GoldenDatasetRoot>(&content) {
            root.items
        } else {
            // レガシー形式：直接配列としてパース
            serde_json::from_str(&content).with_context(|| {
                format!(
                    "Failed to parse golden dataset JSON from {}. Content preview: {}",
                    path.display(),
                    content.chars().take(200).collect::<String>()
                )
            })?
        };

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

async fn run_evaluation(
    state: &AppState,
    golden_data: &[GoldenItem],
) -> anyhow::Result<ConfusionMatrix> {
    let classifier = GenreClassifier::new_default();
    let mut confusion_matrix = ConfusionMatrix::new();

    // Create a RefineEngine for evaluation (uses actual pipeline logic)
    let refine_config = RefineConfig::new(false); // require_tags = false for evaluation
    let graph_source: Arc<dyn crate::pipeline::genre_refine::TagLabelGraphSource> =
        Arc::new(DbTagLabelGraphSource::new(
            state.dao(),
            "7d",                      // Use 7-day window
            Duration::from_secs(3600), // 1 hour TTL
        ));
    let refine_engine: Arc<dyn RefineEngine> =
        Arc::new(DefaultRefineEngine::new(refine_config, graph_source));

    let job_id = Uuid::new_v4();
    let job_context = JobContext::new(job_id, vec![]);

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

        // Use classifier to get initial candidates
        let classification = match classifier.predict(title, body, language) {
            Ok(result) => result,
            Err(e) => {
                error!(error = %e, item_id = %item.id(), "Failed to classify item");
                continue;
            }
        };

        // Create a DeduplicatedArticle for the evaluation item
        let sentences: Vec<String> = body
            .split_terminator(&['。', '.'][..])
            .filter(|s| !s.trim().is_empty())
            .map(|s| s.trim().to_string())
            .collect();

        // Generate sentence hashes (simplified for evaluation)
        let sentence_hashes: Vec<u64> = sentences
            .iter()
            .map(|s| {
                use std::collections::hash_map::DefaultHasher;
                use std::hash::{Hash, Hasher};
                let mut hasher = DefaultHasher::new();
                s.hash(&mut hasher);
                hasher.finish()
            })
            .collect();

        let article = DeduplicatedArticle {
            id: item.id().to_string(),
            title: Some(title.to_string()),
            sentences,
            sentence_hashes,
            language: "unknown".to_string(),
            tags: vec![],       // No tags in evaluation dataset
            duplicates: vec![], // No duplicates in evaluation
        };

        // Create GenreCandidates from classification result
        let candidates: Vec<GenreCandidate> = classification
            .top_genres
            .iter()
            .take(3)
            .map(|genre| {
                let score = classification.scores.get(genre).copied().unwrap_or(0.0);
                let keyword_support = classification.keyword_hits.get(genre).copied().unwrap_or(0);
                let classifier_confidence =
                    classification.scores.get(genre).copied().unwrap_or(0.0);
                GenreCandidate {
                    name: genre.clone(),
                    score: score as f32,
                    keyword_support,
                    classifier_confidence: classifier_confidence as f32,
                }
            })
            .collect();

        // Apply RefineEngine (same as actual pipeline)
        let tag_profile = TagProfile::default(); // No tags in evaluation
        let refine_input = crate::pipeline::genre_refine::RefineInput {
            job: &job_context,
            article: &article,
            candidates: &candidates,
            tag_profile: &tag_profile,
            fallback: TagFallbackMode::CoarseOnly, // No tags, so use CoarseOnly
        };

        let refine_outcome = refine_engine.refine(refine_input).await?;
        let predicted_genres = vec![refine_outcome.final_genre];

        confusion_matrix.add(item.expected_genres(), &predicted_genres);
    }

    Ok(confusion_matrix)
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

/// Load and validate golden dataset
fn load_and_validate_dataset(
    path: &PathBuf,
) -> Result<Vec<GoldenItem>, (StatusCode, Json<serde_json::Value>)> {
    info!("Loading golden dataset from: {}", path.display());

    let golden_data = load_golden_dataset(path).map_err(|e| {
        error!(
            error = %e,
            path = %path.display(),
            "Failed to load golden dataset"
        );
        (
            StatusCode::BAD_REQUEST,
            Json(serde_json::json!({
                "error": format!("Failed to load golden dataset: {}", e),
                "path": path.display().to_string(),
                "hint": "Check if the file exists and contains valid JSON array of items with 'id', 'content', and 'expected_genres' fields"
            })),
        )
    })?;

    info!("Loaded {} items", golden_data.len());

    // Analyze dataset quality
    let quality_report = analyze_golden_dataset(&golden_data);
    info!(
        total_samples = quality_report.total_samples,
        genre_count = quality_report.genre_count,
        min_samples_per_genre = quality_report.min_samples_per_genre,
        max_samples_per_genre = quality_report.max_samples_per_genre,
        avg_samples_per_genre = quality_report.avg_samples_per_genre,
        genres_below_threshold = ?quality_report.genres_below_threshold,
        warnings = ?quality_report.warnings,
        "Golden dataset quality analysis"
    );

    Ok(golden_data)
}

/// Calculate extended metrics from confusion matrix
fn calculate_extended_metrics(
    confusion_matrix: &ConfusionMatrix,
) -> (f64, f64, f64, f64, f64, usize, usize) {
    let (macro_f1_valid, valid_genre_count, undefined_genre_count) =
        confusion_matrix.macro_f1_excluding_undefined();
    let micro_precision = confusion_matrix.micro_precision();
    let micro_recall = confusion_matrix.micro_recall();
    let micro_f1 = confusion_matrix.micro_f1();
    let weighted_f1 = confusion_matrix.weighted_f1();

    (
        micro_precision,
        micro_recall,
        micro_f1,
        weighted_f1,
        macro_f1_valid,
        valid_genre_count,
        undefined_genre_count,
    )
}

/// Create evaluation run with all metrics
fn create_evaluation_run(
    dataset_path: String,
    total_items: usize,
    confusion_matrix: &ConfusionMatrix,
    extended_metrics: (f64, f64, f64, f64, f64, usize, usize),
) -> GenreEvaluationRun {
    let (
        micro_precision,
        micro_recall,
        micro_f1,
        weighted_f1,
        macro_f1_valid,
        valid_genre_count,
        undefined_genre_count,
    ) = extended_metrics;

    GenreEvaluationRun::new(
        dataset_path,
        total_items,
        confusion_matrix.macro_precision(),
        confusion_matrix.macro_recall(),
        confusion_matrix.macro_f1(),
        confusion_matrix.total_tp(),
        confusion_matrix.total_fp(),
        confusion_matrix.total_fn(),
    )
    .with_extended_metrics(
        micro_precision,
        micro_recall,
        micro_f1,
        weighted_f1,
        macro_f1_valid,
        valid_genre_count,
        undefined_genre_count,
    )
}

/// Log evaluation results
fn log_evaluation_results(
    evaluation_run: &GenreEvaluationRun,
    confusion_matrix: &ConfusionMatrix,
    extended_metrics: (f64, f64, f64, f64, f64, usize, usize),
) {
    let (
        micro_precision,
        micro_recall,
        micro_f1,
        weighted_f1,
        macro_f1_valid,
        valid_genre_count,
        undefined_genre_count,
    ) = extended_metrics;

    info!(
        run_id = %evaluation_run.run_id,
        total_items = evaluation_run.total_items,
        macro_precision = confusion_matrix.macro_precision(),
        macro_recall = confusion_matrix.macro_recall(),
        macro_f1 = confusion_matrix.macro_f1(),
        macro_f1_valid = macro_f1_valid,
        micro_precision = micro_precision,
        micro_recall = micro_recall,
        micro_f1 = micro_f1,
        weighted_f1 = weighted_f1,
        valid_genre_count = valid_genre_count,
        undefined_genre_count = undefined_genre_count,
        summary_tp = confusion_matrix.total_tp(),
        summary_fp = confusion_matrix.total_fp(),
        summary_fn = confusion_matrix.total_fn(),
        "Evaluation completed"
    );
}

/// POST /v1/evaluation/genres
/// Golden Datasetを使用してジャンル分類の精度を評価する
/// 評価結果はDBに保存され、run_idが返される
pub(crate) async fn evaluate_genres(
    State(state): State<AppState>,
    Json(request): Json<EvaluateRequest>,
) -> impl IntoResponse {
    let path = determine_dataset_path(&request);

    let golden_data = match load_and_validate_dataset(&path) {
        Ok(data) => data,
        Err(err) => return err.into_response(),
    };

    let confusion_matrix = match run_evaluation(&state, &golden_data).await {
        Ok(matrix) => matrix,
        Err(e) => {
            error!(error = %e, "Failed to run evaluation");
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(serde_json::json!({
                    "error": format!("Failed to run evaluation: {}", e),
                })),
            )
                .into_response();
        }
    };

    let per_genre_metrics = build_evaluation_metrics(&confusion_matrix);
    let extended_metrics = calculate_extended_metrics(&confusion_matrix);

    let evaluation_run = create_evaluation_run(
        path.display().to_string(),
        golden_data.len(),
        &confusion_matrix,
        extended_metrics,
    );

    log_evaluation_results(&evaluation_run, &confusion_matrix, extended_metrics);

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
    #[serde(skip_serializing_if = "Option::is_none")]
    quality_report: Option<DatasetQualityReport>,
}

/// GET /v1/evaluation/genres/{run_id}
/// 指定されたrun_idの評価結果を取得する
pub(crate) async fn get_evaluation_result(
    State(state): State<AppState>,
    axum::extract::Path(run_id): axum::extract::Path<uuid::Uuid>,
) -> impl IntoResponse {
    match state.dao().get_genre_evaluation(run_id).await {
        Ok(Some((run, metrics))) => {
            let response = EvaluationResultResponse {
                run,
                metrics,
                quality_report: None,
            };
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
            let response = EvaluationResultResponse {
                run,
                metrics,
                quality_report: None,
            };
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
