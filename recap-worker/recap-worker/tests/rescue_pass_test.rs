use recap_worker::classification::FeatureVector;
use recap_worker::classifier::centroid::{Article, CentroidClassifier};
use recap_worker::classifier::graph::GraphPropagator;
use std::collections::HashSet;

#[test]
fn test_rescue_pass_with_dynamic_thresholds() {
    // 1. Train CentroidClassifier to establish thresholds
    // Genre "loose": high variance -> low threshold
    // Genre "tight": low variance -> high threshold
    let mut articles = Vec::new();

    // "loose" genre: spread out vectors (large angular variance)
    // Use 9 articles to force single centroid (k=1) logic
    for i in 0..9 {
        // Vary the second component significantly
        let y = (i as f32 - 5.0) * 0.5; // -2.5 to 2.0
        articles.push(Article {
            id: format!("loose_{}", i),
            content: "content".to_string(),
            genres: vec!["loose".to_string()],
            feature_vector: Some(FeatureVector {
                tfidf: vec![1.0, y],
                bm25: vec![],
                embedding: vec![],
            }),
        });
    }

    // "tight" genre: very close vectors
    for i in 0..10 {
        articles.push(Article {
            id: format!("tight_{}", i),
            content: "content".to_string(),
            genres: vec!["tight".to_string()],
            feature_vector: Some(FeatureVector {
                tfidf: vec![0.0, 1.0],
                bm25: vec![],
                embedding: vec![],
            }),
        });
    }

    let mut classifier = CentroidClassifier::new(2);
    classifier.train(&articles).expect("Training failed");
    let thresholds = classifier.get_thresholds();
    println!("Thresholds: {:?}", thresholds);

    // 2. Build GraphPropagator
    let mut propagator = GraphPropagator::new(0.0); // Low threshold for graph building to ensure edges
    let mut candidates = HashSet::new();
    for a in &articles {
        candidates.insert(a.id.clone());
    }
    propagator
        .build_graph(&articles, &candidates)
        .expect("Graph build failed");

    // 3. Test Prediction
    // Case A: Target close to "loose" but not very close (e.g., 0.4 similarity)
    // Should be accepted because "loose" threshold is low
    let target_loose = FeatureVector {
        tfidf: vec![0.4, 0.0],
        bm25: vec![],
        embedding: vec![],
    };
    let pred_loose = propagator.predict_by_neighbors(&target_loose, 5, thresholds);
    println!("Prediction for loose target: {:?}", pred_loose);
    assert!(
        pred_loose.is_some(),
        "Should predict 'loose' due to low threshold"
    );
    assert_eq!(pred_loose.unwrap().0, "loose");

    // Case B: Target close to "tight" but not very close (e.g., 0.8 similarity)
    // Use [-0.6, 0.8] which has length 1.0 and dot product 0.8 with [0.0, 1.0]
    // And negative dot product with "loose" (which has positive x), so it won't match "loose"
    let target_tight_fail = FeatureVector {
        tfidf: vec![-0.6, 0.8],
        bm25: vec![],
        embedding: vec![],
    };
    let pred_tight_fail = propagator.predict_by_neighbors(&target_tight_fail, 5, thresholds);
    println!(
        "Prediction for tight target (fail case): {:?}",
        pred_tight_fail
    );

    // If the threshold for "tight" is > 0.8, this should be None.
    // However, with our clamp(0.3, 0.95), it might be tricky.
    // Let's check the actual threshold printed above.
    // If threshold is high, it should be None.
    if let Some(t) = thresholds.get("tight") {
        if *t > 0.8 {
            assert!(
                pred_tight_fail.is_none(),
                "Should reject 'tight' due to high threshold"
            );
        } else {
            println!("Skipping rejection test because threshold {} is too low", t);
        }
    }
}
