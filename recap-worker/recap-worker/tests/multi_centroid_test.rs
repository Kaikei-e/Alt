use ndarray::Array1;
use recap_worker::classification::FeatureVector;
use recap_worker::classifier::centroid::{Article, CentroidClassifier};
use std::collections::HashMap;

#[test]
fn test_multi_centroid_clustering() {
    // 1. Create dummy data with 2 distinct clusters for the same genre
    // Cluster A: around [1.0, 0.0]
    // Cluster B: around [0.0, 1.0]
    let mut articles = Vec::new();

    // 10 articles for Cluster A
    for i in 0..10 {
        articles.push(Article {
            id: format!("A{}", i),
            content: "content".to_string(),
            genres: vec!["test_genre".to_string()],
            feature_vector: Some(FeatureVector {
                tfidf: vec![1.0, 0.1], // Close to [1, 0]
                bm25: vec![],
                embedding: vec![],
            }),
        });
    }

    // 10 articles for Cluster B
    for i in 0..10 {
        articles.push(Article {
            id: format!("B{}", i),
            content: "content".to_string(),
            genres: vec!["test_genre".to_string()],
            feature_vector: Some(FeatureVector {
                tfidf: vec![0.1, 1.0], // Close to [0, 1]
                bm25: vec![],
                embedding: vec![],
            }),
        });
    }

    // 2. Train classifier
    let mut classifier = CentroidClassifier::new(2);
    classifier.train(&articles).expect("Training failed");

    // 3. Verify that multiple centroids were created
    // We expect at least 2 centroids because we have 20 articles and distinct clusters
    let centroids = classifier
        .get_centroid("test_genre")
        .expect("Genre not found");
    println!("Centroids found: {}", centroids.len());
    assert!(centroids.len() >= 2, "Should have at least 2 centroids");

    // 4. Verify prediction
    // Test point close to Cluster A
    let target_a = FeatureVector {
        tfidf: vec![0.9, 0.2],
        bm25: vec![],
        embedding: vec![],
    };
    let pred_a = classifier.predict(&target_a);
    if let Some((genre, score)) = &pred_a {
        println!("Predicted A: {} (score: {})", genre, score);
    } else {
        println!("Predicted A: None");
        // Debug: check top similarity
        if let Some((genre, score, threshold)) = classifier.get_top_similarity(&target_a) {
            println!(
                "Top similarity for A: {} (score: {}, threshold: {})",
                genre, score, threshold
            );
        }
    }
    assert!(pred_a.is_some(), "Should predict Cluster A");
    assert_eq!(pred_a.unwrap().0, "test_genre");

    // Test point close to Cluster B
    let target_b = FeatureVector {
        tfidf: vec![0.2, 0.9],
        bm25: vec![],
        embedding: vec![],
    };
    let pred_b = classifier.predict(&target_b);
    assert!(pred_b.is_some(), "Should predict Cluster B");
    assert_eq!(pred_b.unwrap().0, "test_genre");
}
