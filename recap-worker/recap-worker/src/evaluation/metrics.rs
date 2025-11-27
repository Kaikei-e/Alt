use std::collections::{HashMap, HashSet};

#[derive(Debug, Default, Clone, Copy)]
struct LabelStats {
    true_positive: f32,
    false_positive: f32,
    false_negative: f32,
    support: usize, // 正解データに含まれるそのラベルの個数
}

/// 分類メトリクス。
#[derive(Debug, Clone, Copy)]
pub struct ClassificationMetrics {
    pub macro_precision: f32,
    pub macro_recall: f32,
    pub macro_f1: f32,
    pub weighted_f1: f32,
    pub accuracy: f32,
    pub top_k_accuracy: Option<f32>, // Top-k Accuracy (k=2など)
}

impl Default for ClassificationMetrics {
    fn default() -> Self {
        Self {
            macro_precision: 0.0,
            macro_recall: 0.0,
            macro_f1: 0.0,
            weighted_f1: 0.0,
            accuracy: 0.0,
            top_k_accuracy: None,
        }
    }
}

/// ゴールデンセットに対するメトリクス集計器。
#[derive(Debug, Default)]
pub struct MetricsCalculator {
    per_label: HashMap<String, LabelStats>,
    total_samples: usize,
    correct_samples: usize,
    top_k_correct_samples: usize,
    k: usize,
}

impl MetricsCalculator {
    #[must_use]
    pub fn new(k: usize) -> Self {
        Self {
            per_label: HashMap::new(),
            total_samples: 0,
            correct_samples: 0,
            top_k_correct_samples: 0,
            k,
        }
    }

    /// 期待ラベルと予測ラベルを登録する。
    ///
    /// # Arguments
    /// * `expected` - 正解ラベルのセット
    /// * `predicted` - 予測ラベルのセット（Top-1）
    /// * `top_k_predictions` - 予測スコア上位k個のラベルリスト（オプション）
    pub fn push(
        &mut self,
        expected: HashSet<String>,
        predicted: HashSet<String>,
        top_k_predictions: Option<&[String]>,
    ) {
        if expected.is_empty() && predicted.is_empty() {
            return;
        }
        self.total_samples += 1;

        // Exact Match / Subset Match check for Accuracy (Top-1)
        if predicted.iter().any(|label| expected.contains(label)) {
            self.correct_samples += 1;
        }

        // Top-k Accuracy check
        if let Some(top_k) = top_k_predictions {
            // Top-kの中に正解が含まれていればOK
            if top_k
                .iter()
                .take(self.k)
                .any(|label| expected.contains(label))
            {
                self.top_k_correct_samples += 1;
            }
        }

        let labels: HashSet<String> = expected
            .union(&predicted)
            .cloned()
            .collect::<HashSet<String>>();

        for label in labels {
            let stats = self.per_label.entry(label.clone()).or_default();
            let expected_contains = expected.contains(label.as_str());
            let predicted_contains = predicted.contains(label.as_str());

            if expected_contains {
                stats.support += 1;
            }

            match (expected_contains, predicted_contains) {
                (true, true) => {
                    stats.true_positive += 1.0;
                }
                (false, true) => {
                    stats.false_positive += 1.0;
                }
                (true, false) => {
                    stats.false_negative += 1.0;
                }
                (false, false) => {}
            }
        }
    }

    #[must_use]
    pub fn finalize(&self) -> ClassificationMetrics {
        if self.per_label.is_empty() {
            return ClassificationMetrics::default();
        }

        let mut precision_sum = 0.0;
        let mut recall_sum = 0.0;
        let mut f1_sum = 0.0;
        let mut weighted_f1_sum = 0.0;
        let mut total_support = 0;
        let mut counted_labels = 0.0;

        for stats in self.per_label.values() {
            let precision = if stats.true_positive + stats.false_positive > 0.0 {
                stats.true_positive / (stats.true_positive + stats.false_positive)
            } else {
                0.0
            };
            let recall = if stats.true_positive + stats.false_negative > 0.0 {
                stats.true_positive / (stats.true_positive + stats.false_negative)
            } else {
                0.0
            };
            let f1 = if precision + recall > 0.0 {
                2.0 * precision * recall / (precision + recall)
            } else {
                0.0
            };

            precision_sum += precision;
            recall_sum += recall;
            f1_sum += f1;

            weighted_f1_sum += f1 * (stats.support as f32);
            total_support += stats.support;

            counted_labels += 1.0;
        }

        ClassificationMetrics {
            macro_precision: if counted_labels > 0.0 {
                precision_sum / counted_labels
            } else {
                0.0
            },
            macro_recall: if counted_labels > 0.0 {
                recall_sum / counted_labels
            } else {
                0.0
            },
            macro_f1: if counted_labels > 0.0 {
                f1_sum / counted_labels
            } else {
                0.0
            },
            weighted_f1: if total_support > 0 {
                weighted_f1_sum / (total_support as f32)
            } else {
                0.0
            },
            accuracy: if self.total_samples > 0 {
                self.correct_samples as f32 / self.total_samples as f32
            } else {
                0.0
            },
            top_k_accuracy: if self.total_samples > 0 {
                Some(self.top_k_correct_samples as f32 / self.total_samples as f32)
            } else {
                None
            },
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_metrics_calculation() {
        let mut calculator = MetricsCalculator::new(2);

        // Case 1: Exact match (Top-1 correct)
        // Expected: A, Predicted: A
        calculator.push(
            HashSet::from(["A".to_string()]),
            HashSet::from(["A".to_string()]),
            Some(&["A".to_string(), "B".to_string()]),
        );

        // Case 2: Top-k match (Top-1 incorrect, but in Top-2)
        // Expected: B, Predicted: C, Top-k: [C, B]
        calculator.push(
            HashSet::from(["B".to_string()]),
            HashSet::from(["C".to_string()]),
            Some(&["C".to_string(), "B".to_string()]),
        );

        // Case 3: Complete miss
        // Expected: C, Predicted: D, Top-k: [D, E]
        calculator.push(
            HashSet::from(["C".to_string()]),
            HashSet::from(["D".to_string()]),
            Some(&["D".to_string(), "E".to_string()]),
        );

        let metrics = calculator.finalize();

        // Accuracy (Top-1): 1/3 = 0.333...
        assert!((metrics.accuracy - 1.0 / 3.0).abs() < 1e-4);

        // Top-k Accuracy (k=2): 2/3 = 0.666... (Case 1 and Case 2 are correct in Top-2)
        assert!((metrics.top_k_accuracy.unwrap() - 2.0 / 3.0).abs() < 1e-4);

        // Macro F1
        // Label A: TP=1, FP=0, FN=0 -> F1=1.0
        // Label B: TP=0, FP=0, FN=1 -> F1=0.0
        // Label C: TP=0, FP=1, FN=1 -> F1=0.0
        // Label D: TP=0, FP=1, FN=0 -> F1=0.0
        // Macro F1 = (1.0 + 0.0 + 0.0 + 0.0) / 4 = 0.25
        assert!((metrics.macro_f1 - 0.25).abs() < 1e-4);

        // Weighted F1
        // Support: A=1, B=1, C=1, D=0
        // Weighted F1 = (1.0*1 + 0.0*1 + 0.0*1 + 0.0*0) / 3 = 0.333...
        assert!((metrics.weighted_f1 - 1.0 / 3.0).abs() < 1e-4);
    }
}
