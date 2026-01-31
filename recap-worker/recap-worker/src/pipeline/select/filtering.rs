//! Outlier filtering for the select stage.

use std::collections::HashMap;

use crate::pipeline::embedding::{cosine_similarity, Embedder};
use crate::pipeline::genre::GenreAssignment;

/// Filter outliers from assignments based on embedding similarity.
#[allow(clippy::too_many_lines)]
pub(crate) async fn filter_outliers(
    service: &dyn Embedder,
    assignments: Vec<GenreAssignment>,
    min_docs_thresholds: &HashMap<String, usize>,
    _cosine_thresholds: &HashMap<String, f32>, // unused now
    min_documents_per_genre: usize,
) -> Vec<GenreAssignment> {
    // Group by genre
    let mut by_genre: std::collections::HashMap<String, Vec<GenreAssignment>> =
        std::collections::HashMap::new();
    for a in assignments {
        let genre = a
            .primary_genre()
            .map_or("other".to_string(), ToString::to_string);
        by_genre.entry(genre).or_default().push(a);
    }

    let mut filtered_assignments = Vec::new();

    for (genre, mut genre_assignments) in by_genre {
        let pre_filter_count = genre_assignments.len();
        if genre == "other" || genre_assignments.len() < 3 {
            filtered_assignments.append(&mut genre_assignments);
            continue;
        }

        // Prepare texts for embedding
        let texts: Vec<String> = genre_assignments
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
            .collect();

        if let Ok(embeddings) = service.encode(&texts).await {
            // Calculate centroid
            let count = embeddings.len() as f32;
            let dim = embeddings[0].len();
            let mut centroid = vec![0.0; dim];

            for vec in &embeddings {
                for (i, val) in vec.iter().enumerate() {
                    centroid[i] += val;
                }
            }
            for val in &mut centroid {
                *val /= count;
            }

            // Calculate distances and similarities
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

            // Calculate 80th percentile distance
            // Sort distances first
            let mut sorted_distances = distances.clone();
            sorted_distances.sort_by(|a, b| a.partial_cmp(b).unwrap_or(std::cmp::Ordering::Equal));

            let computed_f64 = ((sorted_distances.len() as f64) * 0.8).max(0.0);
            // computed_f64は既に非負（.max(0.0)で保証）かつusize::MAX以下であることを確認済み
            let p80_idx = if computed_f64 <= usize::MAX as f64 && computed_f64 >= 0.0 {
                // f64をfloor()で整数に丸めてからi64に変換し、usizeに変換（符号損失を回避）
                let value_i64 = computed_f64.floor() as i64;
                usize::try_from(value_i64).unwrap_or(0)
            } else {
                usize::MAX
            };
            let p80_distance = sorted_distances.get(p80_idx).copied().unwrap_or(2.0); // 2.0 is max distance

            // Filter
            let mut valid_assignments: Vec<GenreAssignment> = Vec::new();
            let mut filtered_out: Vec<(f32, GenreAssignment)> = Vec::new();
            let mut filtered_out_count = 0;

            // Determine min docs for this genre (dynamic fallback)
            // Use consistent Dynamic Min logic: max(3, ceil(total * 0.1))
            // Note: we calculated dynamic min in trim_assignments, but here we recalculate local based on input size.
            // Or we should trust the passed `min_docs_thresholds` if they were updated.
            // However, `trim_assignments` uses bundle stats. Here we only see survivor slice.
            // Let's rely on `min_docs_thresholds` which comes from `trim_assignments`?
            // No, `trim_assignments` logic was separate.
            // Let's re-apply the dynamic logic: max(3, ceil(pre_filter_count * 0.1))
            let computed_f64 = ((pre_filter_count as f64) * 0.1).ceil().max(0.0);
            // computed_f64は既に非負（.max(0.0)で保証）かつusize::MAX以下であることを確認済み
            let dynamic_min = if computed_f64 <= usize::MAX as f64 && computed_f64 >= 0.0 {
                // f64をfloor()で整数に丸めてからi64に変換し、usizeに変換（符号損失を回避）
                let value_i64 = computed_f64.floor() as i64;
                usize::try_from(value_i64).unwrap_or(0)
            } else {
                usize::MAX
            };
            let effective_min = match min_docs_thresholds.get(&genre) {
                Some(&v) => v.max(dynamic_min).max(3),
                None => min_documents_per_genre.max(dynamic_min).max(3),
            };

            // Sort all by score (similarity) descending first to prioritize "central" items
            all_with_similarity
                .sort_by(|a, b| b.0.partial_cmp(&a.0).unwrap_or(std::cmp::Ordering::Equal));

            for (similarity, assignment) in all_with_similarity {
                let distance = 1.0 - similarity;

                // Keep if distance is within percentile OR if we haven't met minimum
                if distance <= p80_distance {
                    valid_assignments.push(assignment);
                } else {
                    // Candidate for filtering
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

            let post_filter_count = valid_assignments.len();
            tracing::debug!(
                genre = %genre,
                pre_filter = pre_filter_count,
                post_filter = post_filter_count,
                filtered_out = filtered_out_count,
                p80_distance = p80_distance,
                "outlier filtering completed"
            );

            filtered_assignments.append(&mut valid_assignments);
        } else {
            filtered_assignments.append(&mut genre_assignments);
        }
    }
    filtered_assignments
}
