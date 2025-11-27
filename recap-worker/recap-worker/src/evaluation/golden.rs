//! ゴールデンセット評価ロジック。

use std::{collections::HashSet, fs, path::Path};

use anyhow::{Context, Result};
use serde::Deserialize;
use serde_json::Value;
use uuid::Uuid;

use crate::{
    evaluation::{
        metrics::{ClassificationMetrics, MetricsCalculator},
        rouge::{RougeScores, compute_rouge},
    },
    pipeline::genre_keywords::GenreKeywords,
};

/// ゴールデンセットの単一レコード。
#[derive(Debug, Deserialize)]
pub struct GoldenRun {
    pub job_id: Option<Uuid>,
    pub genre: Option<String>,
    #[serde(default)]
    pub status: String,
    #[serde(default)]
    pub cluster_count: i32,
    #[serde(default)]
    pub summary_text: Option<String>,
    #[serde(default)]
    pub reference_summary: Option<String>,
    #[serde(default)]
    pub diagnostics: Value,
    #[serde(default)]
    pub quality_score: Option<f32>,
}

/// ゴールデンセットの構造。
#[derive(Debug, Deserialize)]
pub struct GoldenDataset {
    #[serde(default)]
    pub generated_at: Option<String>,
    #[serde(default)]
    pub criteria: Value,
    #[serde(default)]
    pub total_candidates: Option<usize>,
    #[serde(default)]
    pub good: Vec<GoldenRun>,
    #[serde(default)]
    pub bad: Vec<GoldenRun>,
}

/// 評価サマリー。
#[derive(Debug, Clone)]
pub struct EvaluationSummary {
    pub total_samples: usize,
    pub good_samples: usize,
    pub bad_samples: usize,
    pub avg_quality_score: f32,
    pub avg_noise_ratio: f32,
    pub classification: ClassificationMetrics,
    pub rouge: RougeScores,
}

impl Default for EvaluationSummary {
    fn default() -> Self {
        Self {
            total_samples: 0,
            good_samples: 0,
            bad_samples: 0,
            avg_quality_score: 0.0,
            avg_noise_ratio: 0.0,
            classification: ClassificationMetrics::default(),
            rouge: RougeScores::default(),
        }
    }
}

/// ゴールデンセットをファイルから読み込む。
pub fn load_dataset(path: &Path) -> Result<GoldenDataset> {
    let data = fs::read_to_string(path)
        .with_context(|| format!("failed to read golden dataset: {}", path.display()))?;
    let dataset: GoldenDataset =
        serde_json::from_str(&data).context("failed to parse golden dataset JSON")?;
    Ok(dataset)
}

/// 既定パス（`recap-worker/resources/golden_runs.json`）から読み込む。
pub fn load_default_dataset() -> Result<GoldenDataset> {
    let path = Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("..")
        .join("resources")
        .join("golden_runs.json");
    load_dataset(&path)
}

/// ゴールデンセットを評価し、集計結果を返す。
pub fn evaluate_dataset(dataset: &GoldenDataset) -> EvaluationSummary {
    let mut summary = EvaluationSummary::default();
    let mut quality_sum = 0.0f32;
    let mut quality_count = 0usize;
    let mut noise_sum = 0.0f32;
    let mut noise_count = 0usize;

    let mut rouge_sum = RougeScores::default();
    let mut rouge_count = 0usize;

    let keywords = GenreKeywords::default_keywords();
    let mut metrics_calculator = MetricsCalculator::new(2);

    let all_samples = dataset
        .good
        .iter()
        .chain(dataset.bad.iter())
        .collect::<Vec<&GoldenRun>>();

    for run in &all_samples {
        if let Some(score) = run.quality_score {
            quality_sum += score;
            quality_count += 1;
        }

        if let Some(noise_ratio) = run
            .diagnostics
            .get("noise_ratio")
            .and_then(serde_json::Value::as_f64)
        {
            noise_sum += noise_ratio as f32;
            noise_count += 1;
        }

        if let (Some(summary_text), Some(reference_summary)) =
            (&run.summary_text, &run.reference_summary)
        {
            if !summary_text.trim().is_empty() && !reference_summary.trim().is_empty() {
                let rouge = compute_rouge(summary_text, reference_summary);
                rouge_sum.rouge1_precision += rouge.rouge1_precision;
                rouge_sum.rouge1_recall += rouge.rouge1_recall;
                rouge_sum.rouge1_f += rouge.rouge1_f;
                rouge_sum.rouge_l_precision += rouge.rouge_l_precision;
                rouge_sum.rouge_l_recall += rouge.rouge_l_recall;
                rouge_sum.rouge_l_f += rouge.rouge_l_f;
                rouge_count += 1;
            }
        }

        if let (Some(expected_genre), Some(summary_text)) = (&run.genre, &run.summary_text) {
            let expected_set = HashSet::from([expected_genre.clone()]);
            let predicted_list: Vec<String> = keywords
                .top_genres(summary_text, 3)
                .into_iter()
                .map(|(genre, _)| genre)
                .collect();
            let predicted_set = predicted_list.iter().cloned().collect::<HashSet<String>>();
            metrics_calculator.push(expected_set, predicted_set, Some(&predicted_list));
        }
    }

    summary.good_samples = dataset.good.len();
    summary.bad_samples = dataset.bad.len();
    summary.total_samples = summary.good_samples + summary.bad_samples;

    if quality_count > 0 {
        summary.avg_quality_score = quality_sum / quality_count as f32;
    }
    if noise_count > 0 {
        summary.avg_noise_ratio = noise_sum / noise_count as f32;
    }
    if rouge_count > 0 {
        summary.rouge = RougeScores {
            rouge1_precision: rouge_sum.rouge1_precision / rouge_count as f32,
            rouge1_recall: rouge_sum.rouge1_recall / rouge_count as f32,
            rouge1_f: rouge_sum.rouge1_f / rouge_count as f32,
            rouge_l_precision: rouge_sum.rouge_l_precision / rouge_count as f32,
            rouge_l_recall: rouge_sum.rouge_l_recall / rouge_count as f32,
            rouge_l_f: rouge_sum.rouge_l_f / rouge_count as f32,
        };
    }

    summary.classification = metrics_calculator.finalize();

    summary
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn evaluate_dataset_handles_empty() {
        let dataset = GoldenDataset {
            generated_at: None,
            criteria: Value::Null,
            total_candidates: None,
            good: vec![],
            bad: vec![],
        };
        let summary = evaluate_dataset(&dataset);
        assert_eq!(summary.total_samples, 0);
        assert!((summary.avg_quality_score - 0.0).abs() < f32::EPSILON);
        assert!((summary.avg_noise_ratio - 0.0).abs() < f32::EPSILON);
    }
}
