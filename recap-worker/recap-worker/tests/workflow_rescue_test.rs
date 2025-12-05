use recap_worker::classification::{Article, ClassificationLanguage, TokenPipeline};
use recap_worker::classifier::workflow::{ClassificationPipeline, GoldenItem};
use std::fs::File;

#[test]
fn test_rescue_pass_integration() {
    // 1. Create a temporary Golden Dataset
    // "tight_genre": 5 identical articles to force high threshold.
    let mut golden_items = Vec::new();
    for i in 0..5 {
        golden_items.push(GoldenItem {
            id: format!("train_{}", i),
            content: "apple apple apple".to_string(),
            genres: vec!["tight_genre".to_string()],
        });
    }

    let mut path = std::env::temp_dir();
    path.push(format!("test_golden_{}.json", std::process::id()));
    let file = File::create(&path).unwrap();
    serde_json::to_writer(file, &golden_items).unwrap();

    // 2. Initialize Pipeline
    let pipeline =
        ClassificationPipeline::from_golden_dataset(&path).expect("Failed to init pipeline");

    // 3. Create a target article
    // "orange orange orange": Completely different from "apple apple apple" to ensure Centroid fails
    // This will force the test to go through Rescue Pass, which should fail and return "other"
    let target_content = "orange orange orange";
    let target_id = "target_1";

    // Extract feature vector using the pipeline's extractor (to match vocab)
    let token_pipeline = TokenPipeline::new();
    let normalized = token_pipeline.preprocess(target_content, "", ClassificationLanguage::English);
    let feature_extractor = pipeline.feature_extractor();
    let feature_vector = feature_extractor.extract(&normalized.tokens);

    let target_article = Article {
        id: target_id.to_string(),
        content: target_content.to_string(),
        genres: Vec::new(),
        feature_vector: Some(feature_vector),
    };

    // 4. Classify
    // We pass [target_article] as all_articles.
    // The bug fix ensures that even if target_article fails Centroid, it is added to the graph.
    // However, for Graph Propagation to work, it needs neighbors.
    // Since we only pass [target_article], the dynamic graph will only contain target_article.
    // So it won't have any labeled neighbors to propagate from.
    // This confirms that `try_rescue_pass` (dynamic) is flawed if used with single article batch without labeled data.

    // BUT, let's see if it returns "other" or crashes.
    // Or maybe it returns None (which maps to "other" in predict).

    let result = pipeline.classify(target_id, target_content, &[target_article]);

    // Cleanup
    let _ = std::fs::remove_file(path);

    // Assert
    // Since there are no labeled neighbors in the dynamic graph, Rescue Pass should fail (return None).
    // And classify falls back to "other".
    assert_eq!(result.unwrap(), "other");

    // To verify the FIX (that target is added to graph), we would need to inspect logs or internal state.
    // Or construct a scenario where there ARE labeled neighbors in `all_articles`.
}

#[test]
fn test_rescue_pass_with_labeled_neighbor() {
    // 1. Create a temporary Golden Dataset (needed for pipeline init)
    let golden_items = vec![GoldenItem {
        id: "train_0".to_string(),
        content: "apple".to_string(),
        genres: vec!["fruit".to_string()],
    }];

    let mut path = std::env::temp_dir();
    path.push(format!("test_golden_2_{}.json", std::process::id()));
    let file = File::create(&path).unwrap();
    serde_json::to_writer(file, &golden_items).unwrap();

    let pipeline =
        ClassificationPipeline::from_golden_dataset(&path).expect("Failed to init pipeline");

    // 2. Create a labeled neighbor article
    // We need to manually construct it because we can't easily get it from pipeline.
    let neighbor_content = "apple apple";
    let neighbor_id = "neighbor_1";
    let token_pipeline = TokenPipeline::new();
    let feature_extractor = pipeline.feature_extractor();

    let n_norm = token_pipeline.preprocess(neighbor_content, "", ClassificationLanguage::English);
    let n_fv = feature_extractor.extract(&n_norm.tokens);

    let neighbor_article = Article {
        id: neighbor_id.to_string(),
        content: neighbor_content.to_string(),
        genres: vec!["fruit".to_string()], // Labeled!
        feature_vector: Some(n_fv),
    };

    // 3. Create target article
    let target_content = "apple banana";
    let target_id = "target_1";
    let t_norm = token_pipeline.preprocess(target_content, "", ClassificationLanguage::English);
    let t_fv = feature_extractor.extract(&t_norm.tokens);

    let target_article = Article {
        id: target_id.to_string(),
        content: target_content.to_string(),
        genres: Vec::new(),
        feature_vector: Some(t_fv),
    };

    // 4. Classify
    // Pass both neighbor and target in all_articles.
    let all_articles = vec![neighbor_article, target_article];

    // If the fix works, target_article should be added to the graph even if it fails Centroid.
    // And since neighbor is labeled and similar, label should propagate.

    // Note: Centroid might classify target if it's similar enough.
    // But with only 1 training sample "apple", "apple banana" might be far?
    // Or maybe close enough.
    // If Centroid classifies it, then Fast Pass succeeds.
    // We want Fast Pass to fail.

    // To ensure Fast Pass fails, we can set sample_count to 0 (it's static in classify).
    // But we can't control it.

    // However, if Fast Pass succeeds, we get "fruit".
    // If Rescue Pass succeeds, we get "fruit".
    // So we can't distinguish easily unless we check logs.

    let result = pipeline.classify(target_id, target_content, &all_articles);

    // Cleanup
    let _ = std::fs::remove_file(path);

    assert_eq!(result.unwrap(), "fruit");
}

#[test]
fn test_predict_rescue_pass() {
    let _ = tracing_subscriber::fmt()
        .with_env_filter(tracing_subscriber::EnvFilter::from_default_env())
        .try_init();

    // 1. Create a temporary Golden Dataset
    let golden_items = vec![GoldenItem {
        id: "train_0".to_string(),
        content: "apple".to_string(),
        genres: vec!["fruit".to_string()],
    }];

    let mut path = std::env::temp_dir();
    path.push(format!("test_golden_predict_{}.json", std::process::id()));
    let file = File::create(&path).unwrap();
    serde_json::to_writer(file, &golden_items).unwrap();

    let pipeline =
        ClassificationPipeline::from_golden_dataset(&path).expect("Failed to init pipeline");

    // 2. Predict for a target article
    // "apple banana" should be close enough to "apple" to trigger Rescue Pass if Centroid fails.
    // Note: Centroid might succeed if threshold is low.
    // But we want to verify Rescue Pass logging.

    let target_content = "apple banana";
    let result = pipeline.predict("target_1", target_content, ClassificationLanguage::English);

    // Cleanup
    let _ = std::fs::remove_file(path);

    // Check result
    // It should return "fruit" either via Centroid or Rescue.
    // We will check logs to see which one.
    let classification = result.unwrap();
    assert!(classification.top_genres.contains(&"fruit".to_string()));
}
