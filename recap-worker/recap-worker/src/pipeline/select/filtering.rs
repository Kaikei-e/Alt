//! Outlier filtering for the select stage.

use std::collections::HashMap;

use crate::pipeline::embedding::{Embedder, cosine_similarity};
use crate::pipeline::genre::GenreAssignment;

use super::embedding_batches::{EMBEDDING_REQUEST_BATCH_SIZE, encode_batched};

/// Filter outliers from assignments based on embedding similarity.
pub(crate) async fn filter_outliers(
    service: &dyn Embedder,
    assignments: Vec<GenreAssignment>,
    min_docs_thresholds: &HashMap<String, usize>,
    _cosine_thresholds: &HashMap<String, f32>, // unused now
    min_documents_per_genre: usize,
) -> Vec<GenreAssignment> {
    let by_genre = group_by_primary_genre(assignments);
    let mut filtered_assignments = Vec::new();

    for (genre, mut genre_assignments) in by_genre {
        if genre == "other" || genre_assignments.len() < 3 {
            filtered_assignments.append(&mut genre_assignments);
            continue;
        }

        let mut kept = filter_genre_outliers(
            service,
            &genre,
            genre_assignments,
            min_docs_thresholds,
            min_documents_per_genre,
        )
        .await;
        filtered_assignments.append(&mut kept);
    }
    filtered_assignments
}

fn group_by_primary_genre(
    assignments: Vec<GenreAssignment>,
) -> HashMap<String, Vec<GenreAssignment>> {
    let mut by_genre: HashMap<String, Vec<GenreAssignment>> = HashMap::new();
    for a in assignments {
        let genre = a
            .primary_genre()
            .map_or("other".to_string(), ToString::to_string);
        by_genre.entry(genre).or_default().push(a);
    }
    by_genre
}

fn build_embedding_texts(assignments: &[GenreAssignment]) -> Vec<String> {
    assignments
        .iter()
        .map(|a| {
            let title = a.article.title.as_deref().unwrap_or("");
            let snippet = a
                .article
                .sentences
                .iter()
                .take(3)
                .cloned()
                .collect::<Vec<_>>()
                .join(" ");
            format!("{title} {snippet}")
        })
        .collect()
}

async fn filter_genre_outliers(
    service: &dyn Embedder,
    genre: &str,
    genre_assignments: Vec<GenreAssignment>,
    min_docs_thresholds: &HashMap<String, usize>,
    min_documents_per_genre: usize,
) -> Vec<GenreAssignment> {
    let texts = build_embedding_texts(&genre_assignments);
    if let Ok(embeddings) = encode_batched(service, &texts, EMBEDDING_REQUEST_BATCH_SIZE).await {
        filter_genre_by_embeddings(
            genre,
            genre_assignments,
            &embeddings,
            min_docs_thresholds,
            min_documents_per_genre,
        )
    } else {
        tracing::warn!(
            genre = %genre,
            "failed to embed articles for outlier filtering, skipping filter"
        );
        genre_assignments
    }
}

fn filter_genre_by_embeddings(
    genre: &str,
    genre_assignments: Vec<GenreAssignment>,
    embeddings: &[Vec<f32>],
    min_docs_thresholds: &HashMap<String, usize>,
    min_documents_per_genre: usize,
) -> Vec<GenreAssignment> {
    let pre_filter_count = genre_assignments.len();
    let centroid = compute_centroid(embeddings);

    let mut distances: Vec<f32> = Vec::with_capacity(embeddings.len());
    let mut all_with_similarity: Vec<(f32, GenreAssignment)> = Vec::new();
    for (i, assignment) in genre_assignments.into_iter().enumerate() {
        let similarity = cosine_similarity(&embeddings[i], &centroid);
        // Similarity is 1.0 for identical, -1.0 for opposite.
        // Distance can be 1 - similarity.
        // We want to filter out things that allow FAR from centroid (low similarity).
        distances.push(1.0 - similarity);
        all_with_similarity.push((similarity, assignment));
    }

    let p80_distance = compute_p80_distance(&distances);
    let effective_min = compute_effective_min(
        genre,
        pre_filter_count,
        min_docs_thresholds,
        min_documents_per_genre,
    );

    let (valid_assignments, filtered_out_count) =
        select_valid_assignments(all_with_similarity, p80_distance, effective_min);

    let post_filter_count = valid_assignments.len();
    tracing::debug!(
        genre = %genre,
        pre_filter = pre_filter_count,
        post_filter = post_filter_count,
        filtered_out = filtered_out_count,
        p80_distance = p80_distance,
        "outlier filtering completed"
    );

    valid_assignments
}

fn compute_centroid(embeddings: &[Vec<f32>]) -> Vec<f32> {
    let count = embeddings.len() as f32;
    let dim = embeddings[0].len();
    let mut centroid = vec![0.0; dim];

    for vec in embeddings {
        for (i, val) in vec.iter().enumerate() {
            centroid[i] += val;
        }
    }
    for val in &mut centroid {
        *val /= count;
    }
    centroid
}

fn non_negative_f64_to_usize(computed_f64: f64) -> usize {
    // computed_f64は既に非負（.max(0.0)で保証）かつusize::MAX以下であることを確認済み
    if computed_f64 <= usize::MAX as f64 && computed_f64 >= 0.0 {
        // f64をfloor()で整数に丸めてからi64に変換し、usizeに変換（符号損失を回避）
        let value_i64 = computed_f64.floor() as i64;
        usize::try_from(value_i64).unwrap_or(0)
    } else {
        usize::MAX
    }
}

fn compute_p80_distance(distances: &[f32]) -> f32 {
    let mut sorted_distances = distances.to_vec();
    sorted_distances.sort_by(|a, b| a.partial_cmp(b).unwrap_or(std::cmp::Ordering::Equal));

    let computed_f64 = ((sorted_distances.len() as f64) * 0.8).max(0.0);
    let p80_idx = non_negative_f64_to_usize(computed_f64);
    sorted_distances.get(p80_idx).copied().unwrap_or(2.0) // 2.0 is max distance
}

fn compute_effective_min(
    genre: &str,
    pre_filter_count: usize,
    min_docs_thresholds: &HashMap<String, usize>,
    min_documents_per_genre: usize,
) -> usize {
    // Use consistent Dynamic Min logic: max(3, ceil(pre_filter_count * 0.1))
    let computed_f64 = ((pre_filter_count as f64) * 0.1).ceil().max(0.0);
    let dynamic_min = non_negative_f64_to_usize(computed_f64);
    match min_docs_thresholds.get(genre) {
        Some(&v) => v.max(dynamic_min).max(3),
        None => min_documents_per_genre.max(dynamic_min).max(3),
    }
}

fn select_valid_assignments(
    mut all_with_similarity: Vec<(f32, GenreAssignment)>,
    p80_distance: f32,
    effective_min: usize,
) -> (Vec<GenreAssignment>, usize) {
    // Sort all by score (similarity) descending first to prioritize "central" items
    all_with_similarity.sort_by(|a, b| b.0.partial_cmp(&a.0).unwrap_or(std::cmp::Ordering::Equal));

    let mut valid_assignments: Vec<GenreAssignment> = Vec::new();
    let mut filtered_out: Vec<(f32, GenreAssignment)> = Vec::new();
    let mut filtered_out_count = 0;

    for (similarity, assignment) in all_with_similarity {
        let distance = 1.0 - similarity;
        if distance <= p80_distance {
            valid_assignments.push(assignment);
        } else {
            filtered_out_count += 1;
            filtered_out.push((similarity, assignment));
        }
    }

    // Ensure diversity/min count:
    if valid_assignments.len() < effective_min {
        let needed = effective_min - valid_assignments.len();
        for (_, assignment) in filtered_out.into_iter().take(needed) {
            valid_assignments.push(assignment);
            filtered_out_count -= 1;
        }
    }

    (valid_assignments, filtered_out_count)
}
