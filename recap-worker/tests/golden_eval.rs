use recap_worker::evaluation::{
    golden::{evaluate_dataset, load_default_dataset},
    rouge::RougeScores,
};

#[ignore]
#[test]
fn evaluate_golden_runs_snapshot() {
    let dataset = load_default_dataset().expect("golden dataset should be loadable");
    assert!(
        !dataset.good.is_empty() || !dataset.bad.is_empty(),
        "golden dataset must contain at least one sample"
    );

    let summary = evaluate_dataset(&dataset);
    assert!(
        summary.total_samples >= dataset.good.len() + dataset.bad.len(),
        "summary should include all samples"
    );

    // 平均品質スコアは0から1の範囲内
    assert!(
        (0.0..=1.0).contains(&summary.avg_quality_score),
        "quality score should be normalized"
    );

    // ROUGEスコアも0から1の範囲に収まることを確認
    assert!(
        is_valid_rouge(summary.rouge),
        "invalid rouge scores detected"
    );

    // ジャンル別ROUGEが計算されている場合、各値が0から1の範囲内であることを確認
    for (genre, rouge) in &summary.rouge_by_genre {
        assert!(
            is_valid_rouge(*rouge),
            "invalid rouge scores for genre: {}",
            genre
        );
    }

    // ジャンル別ROUGEのカウントとサマリーの整合性を確認
    for (genre, count) in &summary.rouge_count_by_genre {
        assert!(
            *count > 0,
            "genre {} should have at least one sample",
            genre
        );
        assert!(
            summary.rouge_by_genre.contains_key(genre),
            "genre {} should have ROUGE scores",
            genre
        );
    }
}

fn is_valid_rouge(scores: RougeScores) -> bool {
    let components = [
        scores.rouge1_precision,
        scores.rouge1_recall,
        scores.rouge1_f,
        scores.rouge_l_precision,
        scores.rouge_l_recall,
        scores.rouge_l_f,
    ];
    components.iter().all(|value| (0.0..=1.0).contains(value))
}
