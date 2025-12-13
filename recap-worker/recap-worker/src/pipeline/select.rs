use async_trait::async_trait;
use uuid::Uuid;

use crate::scheduler::JobContext;

use super::embedding::cosine_similarity;
use super::genre::{GenreAssignment, GenreBundle};
use crate::clients::SubworkerClient;
use crate::pipeline::embedding::Embedder;
use crate::store::dao::RecapDao;
use crate::util::kmeans::KMeans;
use chrono::Utc;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::sync::Arc;

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub(crate) struct SelectedSummary {
    pub(crate) job_id: Uuid,
    pub(crate) assignments: Vec<GenreAssignment>,
}

#[async_trait]
pub(crate) trait SelectStage: Send + Sync {
    async fn select(
        &self,
        job: &JobContext,
        bundle: GenreBundle,
    ) -> anyhow::Result<SelectedSummary>;
}

#[derive(Clone)]
pub(crate) struct SubgenreConfig {
    max_docs_per_genre: usize,
    target_docs_per_subgenre: usize,
    max_k: usize,
}

impl SubgenreConfig {
    pub(crate) fn new(
        max_docs_per_genre: usize,
        target_docs_per_subgenre: usize,
        max_k: usize,
    ) -> Self {
        Self {
            max_docs_per_genre,
            target_docs_per_subgenre,
            max_k,
        }
    }
}

#[derive(Clone)]
pub(crate) struct SummarySelectStage {
    max_articles_per_genre: usize,
    min_documents_per_genre: usize,
    #[allow(dead_code)]
    similarity_threshold: f32, // now implicitly used only via config or unused if pure percentile
    subgenre_config: SubgenreConfig,
    embedding_service: Option<Arc<dyn Embedder>>,
    dao: Option<Arc<dyn RecapDao>>,
    #[allow(dead_code)]
    subworker: Option<Arc<SubworkerClient>>,
}

impl SummarySelectStage {
    pub(crate) fn new(
        embedding_service: Option<Arc<dyn Embedder>>,
        min_documents_per_genre: usize,
        similarity_threshold: f32,
        dao: Option<Arc<dyn RecapDao>>,
        subworker: Option<Arc<SubworkerClient>>,
        subgenre_config: SubgenreConfig,
    ) -> Self {
        Self {
            max_articles_per_genre: 20,
            min_documents_per_genre,
            similarity_threshold,
            subgenre_config,
            embedding_service,
            dao,
            subworker,
        }
    }

    async fn get_dynamic_thresholds(&self) -> (HashMap<String, usize>, HashMap<String, f32>) {
        if let Some(dao) = &self.dao {
            match dao.get_latest_worker_config("genre_distribution").await {
                Ok(Some(payload)) => {
                    // payload is {"genre": {"min_docs_threshold": N, "cosine_threshold": F, ...}}
                    if let Some(obj) = payload.as_object() {
                        let mut min_docs_map = HashMap::new();
                        let mut cosine_map = HashMap::new();
                        for (genre, stats) in obj {
                            if let Some(threshold) = stats
                                .get("min_docs_threshold")
                                .and_then(serde_json::Value::as_u64)
                            {
                                min_docs_map.insert(genre.clone(), threshold as usize);
                            }
                            if let Some(threshold) = stats
                                .get("cosine_threshold")
                                .and_then(serde_json::Value::as_f64)
                            {
                                cosine_map.insert(genre.clone(), threshold as f32);
                            }
                        }
                        return (min_docs_map, cosine_map);
                    }
                }
                Ok(None) => {
                    tracing::debug!("no dynamic genre distribution config found");
                }
                Err(e) => {
                    tracing::warn!("failed to fetch dynamic genre distribution config: {}", e);
                }
            }
        }
        (HashMap::new(), HashMap::new())
    }

    async fn subcluster_others(
        &self,
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
        if let Some(service) = &self.embedding_service {
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

            if let Ok(embeddings) = service.encode(&texts).await {
                // Determine K dynamically: at least 3 items per cluster
                let k = (others.len() / 3).clamp(1, 5);

                if k > 1 {
                    let kmeans = KMeans::new(&embeddings, k, 20);

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

    /// Split large genres (e.g., software_dev) into subgenres (e.g., software_dev_001, software_dev_002)
    /// when the article count exceeds the threshold.
    async fn subcluster_large_genres(
        &self,
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
            if n <= self.subgenre_config.max_docs_per_genre || genre == "other" {
                result.append(&mut genre_assignments);
                continue;
            }

            // Calculate k: ceil(n / target_docs_per_subgenre), capped at max_k
            let k = n
                .div_ceil(self.subgenre_config.target_docs_per_subgenre)
                .min(self.subgenre_config.max_k)
                .max(2); // At least 2 clusters

            // Use embedding-based clustering if available
            if let Some(service) = &self.embedding_service {
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

                if let Ok(embeddings) = service.encode(&texts).await {
                    if embeddings.len() == genre_assignments.len() {
                        let kmeans = KMeans::new(&embeddings, k, 20);

                        for (i, &cluster_id) in kmeans.assignments.iter().enumerate() {
                            if let Some(assignment) = genre_assignments.get_mut(i) {
                                // Format: base_001, base_002, etc. (1-indexed)
                                let subgenre = format!("{}_{:03}", genre, cluster_id + 1);
                                // Insert at index 0 to make it primary
                                assignment.genres.insert(0, subgenre.clone());
                                assignment.genre_scores.insert(subgenre.clone(), 100);
                                assignment.genre_confidence.insert(subgenre, 1.0);
                            }
                        }

                        tracing::info!(
                            genre = %genre,
                            original_count = n,
                            k,
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

    fn trim_assignments(
        &self,
        bundle: GenreBundle,
        thresholds: &HashMap<String, usize>,
    ) -> Vec<GenreAssignment> {
        let mut selected = Vec::new();

        // Group by genre
        let mut by_genre: HashMap<String, Vec<GenreAssignment>> = HashMap::new();
        for assignment in bundle.assignments {
            let g = assignment
                .primary_genre()
                .map_or("other".to_string(), ToString::to_string);
            by_genre.entry(g).or_default().push(assignment);
        }

        for (genre, mut candidates) in by_genre {
            // Sort by score descending
            candidates.sort_by(|a, b| {
                let score_a = Self::calculate_score(a);
                let score_b = Self::calculate_score(b);
                score_b
                    .partial_cmp(&score_a)
                    .unwrap_or(std::cmp::Ordering::Equal)
            });

            // Determine limits
            let total_in_genre = candidates.len() as f64;
            let computed_f64 = (total_in_genre * 0.1).ceil().max(0.0);
            // computed_f64は既に非負（.max(0.0)で保証）かつusize::MAX以下であることを確認済み
            let dynamic_min = if computed_f64 <= usize::MAX as f64 && computed_f64 >= 0.0 {
                // f64をfloor()で整数に丸めてからi64に変換し、usizeに変換（符号損失を回避）
                let value_i64 = computed_f64.floor() as i64;
                usize::try_from(value_i64).unwrap_or(0)
            } else {
                usize::MAX
            };

            let base_min = thresholds
                .get(&genre)
                .copied()
                .unwrap_or(self.min_documents_per_genre);
            let effective_min = base_min.max(dynamic_min);
            let adjusted_max = self.max_articles_per_genre.max(effective_min * 2);

            // Group by source for Round-Robin
            let mut by_source: HashMap<String, std::collections::VecDeque<GenreAssignment>> =
                HashMap::new();
            for c in candidates {
                // simple hostname extraction or just use source_url as is
                let source = c
                    .article
                    .source_url
                    .as_deref()
                    .unwrap_or("unknown")
                    .to_string();
                by_source.entry(source).or_default().push_back(c);
            }

            // Round-robin selection
            let mut genre_selected = Vec::new();
            let mut sources: Vec<String> = by_source.keys().cloned().collect();
            // Sort sources to be deterministic? Or random?
            // Deterministic is better. Sort by best article score in that source?
            // For now just sort by name for stability.
            sources.sort();

            while genre_selected.len() < adjusted_max && !by_source.is_empty() {
                let mut articles_picked_this_round = 0;
                let mut empty_sources = Vec::new();

                for source in &sources {
                    if let Some(deque) = by_source.get_mut(source) {
                        if let Some(article) = deque.pop_front() {
                            genre_selected.push(article);
                            articles_picked_this_round += 1;
                        }
                        if deque.is_empty() {
                            empty_sources.push(source.clone());
                        }
                    }
                    if genre_selected.len() >= adjusted_max {
                        break;
                    }
                }

                // Cleanup empty sources
                for s in empty_sources {
                    by_source.remove(&s);
                }

                // Remove keys from sources list effectively?
                // We just rebuild sources list or ignore missing ones?
                // Efficiency: sources list is small (dozens).
                sources.retain(|s| by_source.contains_key(s));

                if articles_picked_this_round == 0 {
                    break;
                }
            }
            selected.append(&mut genre_selected);
        }
        selected
    }

    fn calculate_score(assignment: &GenreAssignment) -> f32 {
        let Some(primary) = assignment.primary_genre() else {
            return 0.0;
        };

        // 1. Classification Confidence
        let keyword_component =
            assignment.genre_scores.get(primary).copied().unwrap_or(0) as f32 / 100.0;
        let classifier_component = assignment
            .genre_confidence
            .get(primary)
            .copied()
            .unwrap_or(keyword_component);
        let base_confidence = classifier_component.max(keyword_component);

        // 2. Freshness (Exponential decay)
        // age in hours. If unknown, assume 24h (neutral) or 0 (fresh). Let's assume 24h.
        let age_hours = if let Some(published_at) = assignment.article.published_at {
            (Utc::now() - published_at).num_hours().max(0) as f32
        } else {
            24.0
        };
        // Decay score: exp(-0.01 * hours) -> decreases slowly.
        // 24h -> 0.78, 48h -> 0.61, 1 week -> 0.18
        // Let's use a milder decay: exp(-0.005 * hours) => 24h -> 0.88, 1 week -> 0.43
        let freshness_score = (-0.005 * age_hours).exp();

        // 3. Tag Match Score
        // Normalize overlap count. 5+ tags = 1.0
        let tag_score = (assignment.feature_profile.tag_overlap_count as f32 / 5.0).min(1.0);

        // Weighted Sum
        // Weights: Confidence 0.5, Freshness 0.3, Tags 0.2

        (base_confidence * 0.5) + (freshness_score * 0.3) + (tag_score * 0.2)
    }

    #[allow(clippy::too_many_lines)]
    async fn filter_outliers(
        &self,
        service: &dyn Embedder,
        assignments: Vec<GenreAssignment>,
        min_docs_thresholds: &HashMap<String, usize>,
        _cosine_thresholds: &HashMap<String, f32>, // unused now
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
                sorted_distances
                    .sort_by(|a, b| a.partial_cmp(b).unwrap_or(std::cmp::Ordering::Equal));

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
                    None => self.min_documents_per_genre.max(dynamic_min).max(3),
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
}

impl Default for SummarySelectStage {
    fn default() -> Self {
        Self::new(None, 5, 0.5, None, None, SubgenreConfig::new(200, 50, 10))
    }
}

#[async_trait]
impl SelectStage for SummarySelectStage {
    async fn select(
        &self,
        job: &JobContext,
        bundle: GenreBundle,
    ) -> anyhow::Result<SelectedSummary> {
        let (min_docs_thresholds, cosine_thresholds) = self.get_dynamic_thresholds().await;

        // Sub-cluster "other" genre items
        let assignments = self
            .subcluster_others(bundle.assignments)
            .await
            .unwrap_or_else(|e| {
                tracing::warn!("subclustering failed: {}", e);
                vec![]
            });

        // Sub-cluster large genres (e.g., software_dev -> software_dev_001, software_dev_002)
        let assignments = match self.subcluster_large_genres(assignments).await {
            Ok(result) => result,
            Err(e) => {
                tracing::warn!("large genre subclustering failed: {}", e);
                // Return empty vec as fallback (should not happen in practice)
                vec![]
            }
        };

        let pre_trim_count = assignments.len();
        let mut assignments = self.trim_assignments(
            GenreBundle {
                assignments,
                ..bundle
            },
            &min_docs_thresholds,
        );
        let post_trim_count = assignments.len();

        if let Some(service) = &self.embedding_service {
            let pre_outlier_count = assignments.len();
            assignments = self
                .filter_outliers(
                    &**service,
                    assignments,
                    &min_docs_thresholds,
                    &cosine_thresholds,
                )
                .await;
            let post_outlier_count = assignments.len();
            tracing::debug!(
                job_id = %job.job_id,
                pre_trim = pre_trim_count,
                post_trim = post_trim_count,
                pre_outlier = pre_outlier_count,
                post_outlier = post_outlier_count,
                "selection stage counts"
            );
        }

        Ok(SelectedSummary {
            job_id: job.job_id,
            assignments,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::super::genre::FeatureProfile;
    use super::super::genre::GenreCandidate;
    use super::*;

    fn assignment(genre: &str) -> GenreAssignment {
        use super::super::dedup::DeduplicatedArticle;
        GenreAssignment {
            genres: vec![genre.to_string()],
            candidates: vec![GenreCandidate {
                name: genre.to_string(),
                score: 0.7,
                keyword_support: 5,
                classifier_confidence: 0.75,
            }],
            genre_scores: std::collections::HashMap::from([(genre.to_string(), 10)]),
            genre_confidence: std::collections::HashMap::from([(genre.to_string(), 0.75)]),
            feature_profile: FeatureProfile::default(),
            embedding: None,
            article: DeduplicatedArticle {
                id: Uuid::new_v4().to_string(),
                title: Some("title".to_string()),
                sentences: vec!["body".to_string()],
                sentence_hashes: vec![],
                language: "en".to_string(),
                published_at: None,
                source_url: None,
                tags: Vec::new(),
                duplicates: Vec::new(),
            },
        }
    }

    #[tokio::test]
    async fn trims_to_max_per_genre() {
        let stage = SummarySelectStage {
            max_articles_per_genre: 1,
            min_documents_per_genre: 10,
            similarity_threshold: 0.5,
            subgenre_config: SubgenreConfig::new(200, 50, 10),
            embedding_service: None,
            dao: None,
            subworker: None,
        };
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        let bundle = GenreBundle {
            job_id: job.job_id,
            assignments: vec![assignment("ai"), assignment("ai"), assignment("security")],
            genre_distribution: std::collections::HashMap::new(),
        };

        let selected = stage
            .select(&job, bundle)
            .await
            .expect("selection succeeds");

        // max_articles_per_genre is adjusted to min_documents_per_genre * 2 = 20
        // So all 3 assignments should be selected
        assert_eq!(selected.assignments.len(), 3);
        assert!(
            selected
                .assignments
                .iter()
                .any(|a| a.genres.contains(&"ai".to_string()))
        );
    }

    #[tokio::test]
    async fn ensures_min_documents_per_genre_after_trim() {
        let stage = SummarySelectStage {
            max_articles_per_genre: 5,
            min_documents_per_genre: 10,
            similarity_threshold: 0.5,
            subgenre_config: SubgenreConfig::new(200, 50, 10),
            embedding_service: None,
            dao: None,
            subworker: None,
        };
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        // Create 15 assignments for a single genre to test max_articles adjustment
        let assignments: Vec<GenreAssignment> = (0..15)
            .map(|i| {
                let mut a = assignment("tech");
                a.article.id = format!("tech-{}", i);
                a
            })
            .collect();
        let bundle = GenreBundle {
            job_id: job.job_id,
            assignments,
            genre_distribution: std::collections::HashMap::new(),
        };

        let selected = stage
            .select(&job, bundle)
            .await
            .expect("selection succeeds");

        // max_articles_per_genre should be adjusted to min_documents_per_genre * 2 = 20
        // So all 15 should be selected
        assert!(selected.assignments.len() >= 10);
    }

    #[derive(Debug, Clone)]
    struct MockEmbedder;

    #[async_trait]
    impl crate::pipeline::embedding::Embedder for MockEmbedder {
        async fn encode(&self, texts: &[String]) -> anyhow::Result<Vec<Vec<f32>>> {
            // Return dummy embeddings: vectors of 0.1 * index
            let embeddings = texts
                .iter()
                .enumerate()
                .map(|(i, _)| vec![(i as f32) * 0.1; 384])
                .collect();
            Ok(embeddings)
        }
    }

    #[tokio::test]
    async fn subcluster_others_splits_into_groups() {
        use super::super::dedup::DeduplicatedArticle;
        let embedding_service: Option<Arc<dyn crate::pipeline::embedding::Embedder>> =
            Some(Arc::new(MockEmbedder));

        let stage = SummarySelectStage::new(
            embedding_service,
            3,
            0.8,
            None,
            None,
            SubgenreConfig::new(200, 50, 10),
        );

        // Create 20 "other" assignments
        let assignments: Vec<GenreAssignment> = (0..20)
            .map(|i| GenreAssignment {
                genres: vec!["other".to_string()],
                candidates: vec![],
                genre_scores: std::collections::HashMap::from([("other".to_string(), 10)]),
                genre_confidence: std::collections::HashMap::from([("other".to_string(), 0.9)]),
                feature_profile: FeatureProfile::default(),
                article: DeduplicatedArticle {
                    id: format!("art-{}", i),
                    title: Some(format!("Title {}", i)),
                    sentences: vec![format!("Sentence {}", i)],
                    ..Default::default()
                },
                embedding: None,
            })
            .collect();

        let result = stage
            .subcluster_others(assignments)
            .await
            .expect("subcluster failed");

        // Should have split into multiple clusters (other.0, other.1, etc.)
        // 20 items -> k should be > 1. (20/3).clamp(1,5) = 6 clamp(1,5) = 5.
        // So we expect multiple "other.X" genres.

        let genres: std::collections::HashSet<String> =
            result.iter().flat_map(|a| a.genres.clone()).collect();

        assert!(
            genres.len() > 1,
            "Should have split 'other' into multiple sub-genres"
        );
        assert!(
            genres.iter().any(|g| g.starts_with("other.")),
            "Should contain other.X genres"
        );
    }

    #[test]
    fn trim_assignments_adjusts_max_for_min_documents() {
        let stage = SummarySelectStage {
            max_articles_per_genre: 5,
            min_documents_per_genre: 10,
            similarity_threshold: 0.5,
            subgenre_config: SubgenreConfig::new(200, 50, 10),
            embedding_service: None,
            dao: None,
            subworker: None,
        };
        let assignments: Vec<GenreAssignment> = (0..15)
            .map(|i| {
                let mut a = assignment("tech");
                a.article.id = format!("tech-{}", i);
                a
            })
            .collect();
        let bundle = GenreBundle {
            job_id: Uuid::new_v4(),
            assignments,
            genre_distribution: std::collections::HashMap::new(),
        };

        let trimmed = stage.trim_assignments(bundle, &HashMap::new());

        // Should select at least min_documents_per_genre * 2 = 20, but we only have 15
        // So all 15 should be selected
        assert!(trimmed.len() >= 10);
    }

    #[tokio::test]
    async fn subcluster_large_genres_splits_into_subgenres() {
        use super::super::dedup::DeduplicatedArticle;
        let embedding_service: Option<Arc<dyn crate::pipeline::embedding::Embedder>> =
            Some(Arc::new(MockEmbedder));

        // Set max_docs_per_genre to 50, so 250 articles should be split
        let stage = SummarySelectStage::new(
            embedding_service,
            3,
            0.8,
            None,
            None,
            SubgenreConfig::new(50, 50, 10),
        );

        // Create 250 "software_dev" assignments (exceeds threshold of 50)
        let assignments: Vec<GenreAssignment> = (0..250)
            .map(|i| GenreAssignment {
                genres: vec!["software_dev".to_string()],
                candidates: vec![],
                genre_scores: std::collections::HashMap::from([("software_dev".to_string(), 10)]),
                genre_confidence: std::collections::HashMap::from([(
                    "software_dev".to_string(),
                    0.9,
                )]),
                feature_profile: FeatureProfile::default(),
                article: DeduplicatedArticle {
                    id: format!("art-{}", i),
                    title: Some(format!("Title {}", i)),
                    sentences: vec![format!("Sentence {}", i)],
                    ..Default::default()
                },
                embedding: None,
            })
            .collect();

        let result = stage
            .subcluster_large_genres(assignments)
            .await
            .expect("subcluster_large_genres failed");

        // Should have split into multiple subgenres (software_dev_001, software_dev_002, etc.)
        // 250 items with target 50 -> k = ceil(250/50) = 5, capped at max_k=10, so k=5
        let genres: std::collections::HashSet<String> =
            result.iter().flat_map(|a| a.genres.clone()).collect();

        assert!(
            genres.iter().any(|g| g.starts_with("software_dev_")),
            "Should contain software_dev_### subgenres"
        );

        // Check that subgenres are primary (first in genres vector)
        let subgenre_assignments: Vec<_> = result
            .iter()
            .filter(|a| {
                a.genres
                    .first()
                    .is_some_and(|g| g.starts_with("software_dev_"))
            })
            .collect();
        assert!(
            !subgenre_assignments.is_empty(),
            "Should have at least some assignments with subgenre as primary"
        );

        // Verify subgenre format (e.g., software_dev_001, software_dev_002)
        for assignment in &result {
            if let Some(primary) = assignment.genres.first() {
                if let Some(suffix) = primary.strip_prefix("software_dev_") {
                    // Should match pattern software_dev_XXX where XXX is 3 digits
                    assert!(
                        primary.len() > "software_dev_".len(),
                        "Subgenre should have numeric suffix"
                    );
                    assert!(
                        suffix.parse::<u32>().is_ok(),
                        "Subgenre suffix should be numeric: {}",
                        primary
                    );
                }
            }
        }
    }

    #[tokio::test]
    async fn subcluster_large_genres_does_not_split_small_genres() {
        use super::super::dedup::DeduplicatedArticle;
        let embedding_service: Option<Arc<dyn crate::pipeline::embedding::Embedder>> =
            Some(Arc::new(MockEmbedder));

        // Set max_docs_per_genre to 50
        let stage = SummarySelectStage::new(
            embedding_service,
            3,
            0.8,
            None,
            None,
            SubgenreConfig::new(50, 50, 10),
        );

        // Create 30 "tech" assignments (below threshold of 50)
        let assignments: Vec<GenreAssignment> = (0..30)
            .map(|i| GenreAssignment {
                genres: vec!["tech".to_string()],
                candidates: vec![],
                genre_scores: std::collections::HashMap::from([("tech".to_string(), 10)]),
                genre_confidence: std::collections::HashMap::from([("tech".to_string(), 0.9)]),
                feature_profile: FeatureProfile::default(),
                article: DeduplicatedArticle {
                    id: format!("art-{}", i),
                    title: Some(format!("Title {}", i)),
                    sentences: vec![format!("Sentence {}", i)],
                    ..Default::default()
                },
                embedding: None,
            })
            .collect();

        let result = stage
            .subcluster_large_genres(assignments.clone())
            .await
            .expect("subcluster_large_genres failed");

        // Should not be split (below threshold)
        assert_eq!(result.len(), assignments.len());
        for assignment in &result {
            assert_eq!(
                assignment.genres.first(),
                Some(&"tech".to_string()),
                "Small genre should not be split"
            );
        }
    }

    #[tokio::test]
    async fn subcluster_large_genres_handles_no_embedding_service() {
        use super::super::dedup::DeduplicatedArticle;
        // No embedding service
        let stage =
            SummarySelectStage::new(None, 3, 0.8, None, None, SubgenreConfig::new(50, 50, 10));

        // Create 100 "software_dev" assignments (exceeds threshold)
        let assignments: Vec<GenreAssignment> = (0..100)
            .map(|i| GenreAssignment {
                genres: vec!["software_dev".to_string()],
                candidates: vec![],
                genre_scores: std::collections::HashMap::from([("software_dev".to_string(), 10)]),
                genre_confidence: std::collections::HashMap::from([(
                    "software_dev".to_string(),
                    0.9,
                )]),
                feature_profile: FeatureProfile::default(),
                article: DeduplicatedArticle {
                    id: format!("art-{}", i),
                    title: Some(format!("Title {}", i)),
                    sentences: vec![format!("Sentence {}", i)],
                    ..Default::default()
                },
                embedding: None,
            })
            .collect();

        let result = stage
            .subcluster_large_genres(assignments.clone())
            .await
            .expect("subcluster_large_genres failed");

        // Should return original assignments unchanged (no embedding service)
        assert_eq!(result.len(), assignments.len());
        for assignment in &result {
            assert_eq!(
                assignment.genres.first(),
                Some(&"software_dev".to_string()),
                "Without embedding service, genre should remain unchanged"
            );
        }
    }
}
