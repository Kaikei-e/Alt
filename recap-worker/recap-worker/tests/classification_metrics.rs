// ゴールデンセットを用いたジャンル分類の回帰テスト。
use std::collections::HashSet;
use std::fs;
use std::path::PathBuf;

use recap_worker::classification::{ClassificationLanguage, GenreClassifier};
use recap_worker::evaluation::metrics::MetricsCalculator;
use serde::Deserialize;

#[derive(Debug, Deserialize)]
struct GoldenSample {
    id: String,
    lang: String,
    title: String,
    body: String,
    #[serde(default)]
    expected_genres: Vec<String>,
}

fn load_samples() -> Vec<GoldenSample> {
    let mut path = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    path.push("tests/data/golden_classification.json");
    let raw = fs::read_to_string(path).expect("failed to read golden dataset");
    serde_json::from_str(&raw).expect("failed to parse golden dataset")
}

#[test]
fn golden_dataset_meets_precision_threshold() {
    let samples = load_samples();
    assert!(
        !samples.is_empty(),
        "golden dataset must contain at least one sample"
    );

    let classifier = GenreClassifier::new_test();
    let mut calculator = MetricsCalculator::new();

    for sample in samples {
        let lang = ClassificationLanguage::from_code(&sample.lang);
        let expected: HashSet<String> = sample
            .expected_genres
            .into_iter()
            .map(|g| g.to_lowercase())
            .collect();
        let prediction = classifier
            .predict(&sample.title, &sample.body, lang)
            .expect("prediction should succeed");
        println!(
            "sample={} expected={:?} predicted={:?} scores={:?}",
            sample.id, expected, prediction.top_genres, prediction.scores
        );
        calculator.push(expected, prediction.top_genres.into_iter().collect());
    }

    let metrics = calculator.finalize();
    assert!(
        metrics.macro_f1 >= 0.6,
        "macro F1 is too low for baseline rules: {metrics:?}"
    );
    assert!(
        metrics.accuracy >= 0.8,
        "accuracy threshold not met: {metrics:?}"
    );
}
