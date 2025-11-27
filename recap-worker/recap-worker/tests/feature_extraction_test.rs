use recap_worker::classification::features::{EMBEDDING_DIM, FeatureExtractor};

#[test]
fn test_feature_extraction_known_token() {
    // Setup a simple extractor with minimal vocabulary
    let vocab = vec!["人工知能".to_string()];
    let idf = vec![1.0];
    let extractor = FeatureExtractor::from_metadata(&vocab, &idf, 1.2, 0.75, 10.0);

    // "人工知能" is in the hardcoded lookup
    let tokens = vec!["人工知能".to_string()];
    let feature_vector = extractor.extract(&tokens);

    // Check embedding is non-zero
    let embedding_norm: f32 = feature_vector.embedding.iter().map(|x| x * x).sum();
    assert!(
        embedding_norm > 0.0,
        "Known token should have non-zero embedding"
    );

    // Check specific value from lookup: [1.0, 0.0, 0.0, 0.0, 0.0, 0.0]
    assert_eq!(feature_vector.embedding[0], 1.0);
}

#[test]
fn test_feature_extraction_unknown_token_hashing() {
    // Setup a simple extractor
    let vocab = vec!["unknown_word".to_string()];
    let idf = vec![1.0];
    let extractor = FeatureExtractor::from_metadata(&vocab, &idf, 1.2, 0.75, 10.0);

    // "unknown_word" is NOT in the hardcoded lookup
    let tokens = vec!["unknown_word".to_string()];
    let feature_vector = extractor.extract(&tokens);

    // Check embedding is non-zero (this will fail before the fix)
    let embedding_norm: f32 = feature_vector.embedding.iter().map(|x| x * x).sum();
    assert!(
        embedding_norm > 0.0,
        "Unknown token should have non-zero embedding via hashing fallback"
    );
}

#[test]
fn test_feature_extraction_determinism() {
    let vocab = vec!["random_word".to_string()];
    let idf = vec![1.0];
    let extractor = FeatureExtractor::from_metadata(&vocab, &idf, 1.2, 0.75, 10.0);

    let tokens1 = vec!["random_word".to_string()];
    let feature_vector1 = extractor.extract(&tokens1);

    let tokens2 = vec!["random_word".to_string()];
    let feature_vector2 = extractor.extract(&tokens2);

    assert_eq!(
        feature_vector1.embedding, feature_vector2.embedding,
        "Hashing should be deterministic"
    );
}
