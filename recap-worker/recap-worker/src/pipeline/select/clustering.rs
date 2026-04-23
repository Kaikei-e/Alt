//! Subgenre clustering for the select stage.

use std::collections::HashMap;
use std::sync::Arc;

use crate::pipeline::embedding::Embedder;
use crate::pipeline::genre::GenreAssignment;
use crate::util::kmeans::MiniBatchKMeans;

use super::{
    embedding_batches::{EMBEDDING_REQUEST_BATCH_SIZE, encode_batched},
    types::SubgenreConfig,
};

const MINI_BATCH_KMEANS_BATCH_SIZE: usize = 64;
const MINI_BATCH_KMEANS_MAX_ITERATIONS: usize = 20;

/// Minimum number of articles a sub-cluster must hold before it earns a
/// `{genre}_{id:03}` subgenre label. Smaller clusters keep the parent genre
/// label so the downstream clustering stage is never asked to run HDBSCAN on
/// a single-document corpus (`MIN_FALLBACK_DOCUMENTS`). Aligns with the
/// clients-side fallback threshold in
/// `crate::clients::subworker::types::MIN_FALLBACK_DOCUMENTS`.
const MIN_SUB_CLUSTER_SIZE: usize = 2;

/// Subcluster "other" genre assignments into groups.
pub(crate) async fn subcluster_others(
    embedding_service: Option<&Arc<dyn Embedder>>,
    assignments: Vec<GenreAssignment>,
) -> anyhow::Result<Vec<GenreAssignment>> {
    // Separate "other" assignments
    let (mut others, mut rest): (Vec<GenreAssignment>, Vec<GenreAssignment>) = assignments
        .into_iter()
        .partition(|a| matches!(a.primary_genre(), None | Some("other")));

    if others.is_empty() {
        return Ok(rest);
    }

    // Use local K-Means if embedding service is available
    if let Some(service) = embedding_service {
        let texts: Vec<String> = others
            .iter()
            .map(|a| {
                let title = a.article.title.as_deref().unwrap_or("");
                let body = a
                    .article
                    .sentences
                    .iter()
                    .take(3)
                    .cloned()
                    .collect::<Vec<_>>()
                    .join(" ");
                format!("{title}\n{body}")
            })
            .collect();

        if let Ok(embeddings) =
            encode_batched(service.as_ref(), &texts, EMBEDDING_REQUEST_BATCH_SIZE).await
        {
            // Determine K dynamically: at least 3 items per cluster
            let k = (others.len() / 3).clamp(1, 5);

            if k > 1 {
                let kmeans = MiniBatchKMeans::new(
                    &embeddings,
                    k,
                    MINI_BATCH_KMEANS_MAX_ITERATIONS,
                    MINI_BATCH_KMEANS_BATCH_SIZE,
                );

                for (i, &cluster_id) in kmeans.assignments.iter().enumerate() {
                    if let Some(assignment) = others.get_mut(i) {
                        let new_genre = format!("other.{}", cluster_id);
                        // Insert at index 0 to make it primary
                        assignment.genres.insert(0, new_genre.clone());
                        assignment.genre_scores.insert(new_genre.clone(), 100);
                        assignment.genre_confidence.insert(new_genre, 1.0);
                    }
                }
            }
        } else {
            tracing::warn!("failed to embed 'other' articles for subclustering");
        }
    }
    // If no embedding service, we just keep them as "other" (no op)

    rest.append(&mut others);
    Ok(rest)
}

/// Split large genres into subgenres when the article count exceeds the threshold.
#[allow(clippy::too_many_lines)]
pub(crate) async fn subcluster_large_genres(
    embedding_service: Option<&Arc<dyn Embedder>>,
    subgenre_config: &SubgenreConfig,
    assignments: Vec<GenreAssignment>,
) -> anyhow::Result<Vec<GenreAssignment>> {
    // Group assignments by primary genre
    let mut by_genre: HashMap<String, Vec<GenreAssignment>> = HashMap::new();
    for assignment in assignments {
        let genre = assignment
            .primary_genre()
            .map_or("other".to_string(), ToString::to_string);
        by_genre.entry(genre).or_default().push(assignment);
    }

    let mut result = Vec::new();

    for (genre, mut genre_assignments) in by_genre {
        let n = genre_assignments.len();

        // Only split if the genre exceeds the threshold and is not "other" (which is handled separately)
        if n <= subgenre_config.max_docs_per_genre || genre == "other" {
            result.append(&mut genre_assignments);
            continue;
        }

        // Calculate k: ceil(n / target_docs_per_subgenre), capped at max_k
        let k = n
            .div_ceil(subgenre_config.target_docs_per_subgenre)
            .min(subgenre_config.max_k)
            .max(2); // At least 2 clusters

        // Use embedding-based clustering if available
        if let Some(service) = embedding_service {
            let texts: Vec<String> = genre_assignments
                .iter()
                .map(|a| {
                    let title = a.article.title.as_deref().unwrap_or("");
                    let body = a
                        .article
                        .sentences
                        .iter()
                        .take(3)
                        .cloned()
                        .collect::<Vec<_>>()
                        .join(" ");
                    format!("{title}\n{body}")
                })
                .collect();

            if let Ok(embeddings) =
                encode_batched(service.as_ref(), &texts, EMBEDDING_REQUEST_BATCH_SIZE).await
            {
                if embeddings.len() == genre_assignments.len() {
                    let kmeans = MiniBatchKMeans::new(
                        &embeddings,
                        k,
                        MINI_BATCH_KMEANS_MAX_ITERATIONS,
                        MINI_BATCH_KMEANS_BATCH_SIZE,
                    );

                    // Count articles per cluster so tiny MiniBatchKMeans
                    // clusters (e.g. 1 article) do not inherit a subgenre
                    // label — they would later trip the clustering client's
                    // `MIN_FALLBACK_DOCUMENTS` guard and skip summary
                    // generation entirely.
                    let mut cluster_counts: HashMap<usize, usize> = HashMap::new();
                    for &cluster_id in &kmeans.assignments {
                        *cluster_counts.entry(cluster_id).or_insert(0) += 1;
                    }

                    let mut labeled = 0usize;
                    let mut kept_parent = 0usize;
                    for (i, &cluster_id) in kmeans.assignments.iter().enumerate() {
                        let Some(assignment) = genre_assignments.get_mut(i) else {
                            continue;
                        };
                        let count = cluster_counts.get(&cluster_id).copied().unwrap_or(0);
                        if count < MIN_SUB_CLUSTER_SIZE {
                            // Article stays in the parent genre bucket; the
                            // downstream clustering stage handles it as part
                            // of the larger parent corpus instead of a
                            // single-article island.
                            kept_parent += 1;
                            continue;
                        }
                        // Format: base_001, base_002, etc. (1-indexed)
                        let subgenre = format!("{}_{:03}", genre, cluster_id + 1);
                        // Insert at index 0 to make it primary
                        assignment.genres.insert(0, subgenre.clone());
                        assignment.genre_scores.insert(subgenre.clone(), 100);
                        assignment.genre_confidence.insert(subgenre, 1.0);
                        labeled += 1;
                    }

                    tracing::info!(
                        genre = %genre,
                        original_count = n,
                        k,
                        labeled,
                        kept_parent,
                        tiny_cluster_count = cluster_counts
                            .values()
                            .filter(|&&c| c < MIN_SUB_CLUSTER_SIZE)
                            .count(),
                        "split large genre into subgenres"
                    );
                } else {
                    tracing::warn!(
                        genre = %genre,
                        expected = genre_assignments.len(),
                        got = embeddings.len(),
                        "embedding count mismatch, skipping subgenre split"
                    );
                }
            } else {
                tracing::warn!(
                    genre = %genre,
                    "failed to embed articles for subgenre split, keeping original genre"
                );
            }
        } else {
            tracing::warn!(
                genre = %genre,
                count = n,
                "embedding service unavailable, cannot split large genre into subgenres"
            );
        }

        result.append(&mut genre_assignments);
    }

    Ok(result)
}
