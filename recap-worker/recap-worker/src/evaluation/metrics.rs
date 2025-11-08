use std::collections::{HashMap, HashSet};

#[derive(Debug, Default, Clone, Copy)]
struct LabelStats {
    true_positive: f32,
    false_positive: f32,
    false_negative: f32,
}

/// 分類メトリクス。
#[derive(Debug, Clone, Copy)]
pub struct ClassificationMetrics {
    pub macro_precision: f32,
    pub macro_recall: f32,
    pub macro_f1: f32,
    pub accuracy: f32,
}

impl Default for ClassificationMetrics {
    fn default() -> Self {
        Self {
            macro_precision: 0.0,
            macro_recall: 0.0,
            macro_f1: 0.0,
            accuracy: 0.0,
        }
    }
}

/// ゴールデンセットに対するメトリクス集計器。
#[derive(Debug, Default)]
pub struct MetricsCalculator {
    per_label: HashMap<String, LabelStats>,
    total_samples: usize,
    correct_samples: usize,
}

impl MetricsCalculator {
    #[must_use]
    pub fn new() -> Self {
        Self::default()
    }

    /// 期待ラベルと予測ラベルを登録する。
    pub fn push(&mut self, expected: HashSet<String>, predicted: HashSet<String>) {
        if expected.is_empty() && predicted.is_empty() {
            return;
        }
        self.total_samples += 1;
        if predicted.iter().any(|label| expected.contains(label)) {
            self.correct_samples += 1;
        }

        let labels: HashSet<String> = expected
            .union(&predicted)
            .cloned()
            .collect::<HashSet<String>>();

        for label in labels {
            let stats = self.per_label.entry(label.clone()).or_default();
            let expected_contains = expected.contains(label.as_str());
            let predicted_contains = predicted.contains(label.as_str());
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
            accuracy: if self.total_samples > 0 {
                self.correct_samples as f32 / self.total_samples as f32
            } else {
                0.0
            },
        }
    }
}
