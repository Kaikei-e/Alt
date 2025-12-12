//! ゴールデンセット評価ロジック。

use std::{collections::HashMap, collections::HashSet, fs, path::Path};

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
    /// Genre-wise average ROUGE scores (key: genre name in lowercase)
    pub rouge_by_genre: HashMap<String, RougeScores>,
    /// Count of samples per genre for ROUGE calculation
    pub rouge_count_by_genre: HashMap<String, usize>,
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
            rouge_by_genre: HashMap::new(),
            rouge_count_by_genre: HashMap::new(),
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

#[derive(Debug, Default, Clone, Copy)]
struct ScalarAccumulator {
    sum: f32,
    count: usize,
}

impl ScalarAccumulator {
    fn push(&mut self, value: f32) {
        self.sum += value;
        self.count += 1;
    }

    fn average(self) -> f32 {
        if self.count == 0 {
            0.0
        } else {
            self.sum / self.count as f32
        }
    }
}

#[derive(Debug, Default)]
struct RougeAccumulator {
    sum: RougeScores,
    count: usize,
    by_genre_sum: HashMap<String, RougeScores>,
    by_genre_count: HashMap<String, usize>,
}

impl RougeAccumulator {
    fn push(&mut self, rouge: RougeScores, genre: Option<&str>) {
        self.sum.rouge1_precision += rouge.rouge1_precision;
        self.sum.rouge1_recall += rouge.rouge1_recall;
        self.sum.rouge1_f += rouge.rouge1_f;
        self.sum.rouge_l_precision += rouge.rouge_l_precision;
        self.sum.rouge_l_recall += rouge.rouge_l_recall;
        self.sum.rouge_l_f += rouge.rouge_l_f;
        self.count += 1;

        if let Some(genre) = genre {
            let genre_key = genre.to_lowercase();
            let genre_sum = self.by_genre_sum.entry(genre_key.clone()).or_default();
            genre_sum.rouge1_precision += rouge.rouge1_precision;
            genre_sum.rouge1_recall += rouge.rouge1_recall;
            genre_sum.rouge1_f += rouge.rouge1_f;
            genre_sum.rouge_l_precision += rouge.rouge_l_precision;
            genre_sum.rouge_l_recall += rouge.rouge_l_recall;
            genre_sum.rouge_l_f += rouge.rouge_l_f;
            *self.by_genre_count.entry(genre_key).or_insert(0) += 1;
        }
    }

    fn average_from_sum(sum: &RougeScores, count: usize) -> RougeScores {
        if count == 0 {
            RougeScores::default()
        } else {
            RougeScores {
                rouge1_precision: sum.rouge1_precision / count as f32,
                rouge1_recall: sum.rouge1_recall / count as f32,
                rouge1_f: sum.rouge1_f / count as f32,
                rouge_l_precision: sum.rouge_l_precision / count as f32,
                rouge_l_recall: sum.rouge_l_recall / count as f32,
                rouge_l_f: sum.rouge_l_f / count as f32,
            }
        }
    }

    fn finalize(
        self,
    ) -> (
        RougeScores,
        HashMap<String, RougeScores>,
        HashMap<String, usize>,
    ) {
        let overall_avg = Self::average_from_sum(&self.sum, self.count);

        let mut by_genre_avg: HashMap<String, RougeScores> = HashMap::new();
        for (genre, count) in &self.by_genre_count {
            if let Some(genre_sum) = self.by_genre_sum.get(genre) {
                by_genre_avg.insert(genre.clone(), Self::average_from_sum(genre_sum, *count));
            }
        }

        (overall_avg, by_genre_avg, self.by_genre_count)
    }
}

fn extract_noise_ratio(diagnostics: &Value) -> Option<f32> {
    diagnostics
        .get("noise_ratio")
        .and_then(serde_json::Value::as_f64)
        .map(|v| v as f32)
}

fn maybe_push_rouge(acc: &mut RougeAccumulator, run: &GoldenRun) {
    let (Some(summary_text), Some(reference_summary)) = (&run.summary_text, &run.reference_summary)
    else {
        return;
    };
    if summary_text.trim().is_empty() || reference_summary.trim().is_empty() {
        return;
    }

    let rouge = compute_rouge(summary_text, reference_summary);
    acc.push(rouge, run.genre.as_deref());
}

fn maybe_push_classification(
    metrics_calculator: &mut MetricsCalculator,
    keywords: &GenreKeywords,
    run: &GoldenRun,
) {
    let (Some(expected_genre), Some(summary_text)) = (&run.genre, &run.summary_text) else {
        return;
    };

    let expected_set = HashSet::from([expected_genre.clone()]);
    let predicted_list: Vec<String> = keywords
        .top_genres(summary_text, 3)
        .into_iter()
        .map(|(genre, _)| genre)
        .collect();
    let predicted_set = predicted_list.iter().cloned().collect::<HashSet<String>>();
    metrics_calculator.push(expected_set, predicted_set, Some(&predicted_list));
}

/// ゴールデンセットを評価し、集計結果を返す。
pub fn evaluate_dataset(dataset: &GoldenDataset) -> EvaluationSummary {
    let mut summary = EvaluationSummary::default();
    let mut quality = ScalarAccumulator::default();
    let mut noise = ScalarAccumulator::default();
    let mut rouge = RougeAccumulator::default();

    let keywords = GenreKeywords::default_keywords();
    let mut metrics_calculator = MetricsCalculator::new(2);

    for run in dataset.good.iter().chain(dataset.bad.iter()) {
        if let Some(score) = run.quality_score {
            quality.push(score);
        }

        if let Some(noise_ratio) = extract_noise_ratio(&run.diagnostics) {
            noise.push(noise_ratio);
        }

        maybe_push_rouge(&mut rouge, run);
        maybe_push_classification(&mut metrics_calculator, &keywords, run);
    }

    summary.good_samples = dataset.good.len();
    summary.bad_samples = dataset.bad.len();
    summary.total_samples = summary.good_samples + summary.bad_samples;

    summary.avg_quality_score = quality.average();
    summary.avg_noise_ratio = noise.average();

    let (rouge_overall, rouge_by_genre, rouge_count_by_genre) = rouge.finalize();
    summary.rouge = rouge_overall;
    summary.rouge_by_genre = rouge_by_genre;
    summary.rouge_count_by_genre = rouge_count_by_genre;

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
