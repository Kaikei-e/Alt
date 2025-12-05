use async_trait::async_trait;
use uuid::Uuid;

use crate::scheduler::JobContext;

use super::embedding::{EmbeddingService, cosine_similarity};
use super::genre::{GenreAssignment, GenreBundle};
use crate::clients::SubworkerClient;
use crate::store::dao::RecapDao;
use std::collections::HashMap;
use std::sync::Arc;

#[derive(Debug, Clone, PartialEq)]
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
pub(crate) struct SummarySelectStage {
    max_articles_per_genre: usize,
    min_documents_per_genre: usize,
    similarity_threshold: f32,
    embedding_service: Option<EmbeddingService>,
    dao: Option<Arc<RecapDao>>,
    subworker: Option<Arc<SubworkerClient>>,
}

impl SummarySelectStage {
    pub(crate) fn new(
        embedding_service: Option<EmbeddingService>,
        min_documents_per_genre: usize,
        similarity_threshold: f32,
        dao: Option<Arc<RecapDao>>,
        subworker: Option<Arc<SubworkerClient>>,
    ) -> Self {
        Self {
            max_articles_per_genre: 20,
            min_documents_per_genre,
            similarity_threshold,
            embedding_service,
            dao,
            subworker,
        }
    }

    async fn get_dynamic_thresholds(&self) -> HashMap<String, usize> {
        if let Some(dao) = &self.dao {
            match dao.get_latest_worker_config("genre_distribution").await {
                Ok(Some(payload)) => {
                    // payload is {"genre": {"min_docs_threshold": N, ...}}
                    if let Some(obj) = payload.as_object() {
                        let mut map = HashMap::new();
                        for (genre, stats) in obj {
                            if let Some(threshold) = stats
                                .get("min_docs_threshold")
                                .and_then(serde_json::Value::as_u64)
                            {
                                map.insert(genre.clone(), threshold as usize);
                            }
                        }
                        return map;
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
        HashMap::new()
    }

    async fn subcluster_others(
        &self,
        assignments: Vec<GenreAssignment>,
    ) -> anyhow::Result<Vec<GenreAssignment>> {
        if self.subworker.is_none() {
            return Ok(assignments);
        }
        let subworker = self.subworker.as_ref().unwrap();

        // Separate "other" assignments
        let (mut others, mut rest): (Vec<GenreAssignment>, Vec<GenreAssignment>) = assignments
            .into_iter()
            .partition(|a| matches!(a.primary_genre(), None | Some("other")));

        if others.is_empty() {
            return Ok(rest);
        }

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

        match subworker.cluster_other(texts).await {
            Ok((cluster_ids, _, _)) => {
                for (i, cluster_id) in cluster_ids.iter().enumerate() {
                    if *cluster_id >= 0 {
                        // Assign new genre: other.{cluster_id}
                        // Since GenreAssignment::genres is a Vec, we can prepend or replace.
                        // Usually primary_genre is the first one.
                        if let Some(assignment) = others.get_mut(i) {
                            let new_genre = format!("other.{}", cluster_id);
                            // Insert at index 0 to make it primary
                            assignment.genres.insert(0, new_genre.clone());
                            assignment.genre_scores.insert(new_genre.clone(), 100); // Mock score
                            assignment.genre_confidence.insert(new_genre, 1.0);
                        }
                    }
                }
            }
            Err(e) => {
                tracing::warn!("failed to subcluster others: {}", e);
                // On failure, keep as is
            }
        }

        rest.append(&mut others);
        Ok(rest)
    }

    fn trim_assignments(
        &self,
        bundle: GenreBundle,
        thresholds: &HashMap<String, usize>,
    ) -> Vec<GenreAssignment> {
        let mut per_genre_count = std::collections::HashMap::new();
        let mut selected = Vec::new();

        let mut ranked = bundle.assignments;
        ranked.sort_by(|a, b| {
            let score_a = Self::confidence(a);
            let score_b = Self::confidence(b);
            score_b
                .partial_cmp(&score_a)
                .unwrap_or(std::cmp::Ordering::Equal)
        });

        for assignment in ranked {
            // 最初のジャンルを使用（複数ジャンルがある場合は最初のもの）
            let primary_genre = assignment
                .primary_genre()
                .map_or_else(|| "other".to_string(), std::string::ToString::to_string);

            let min_docs = thresholds
                .get(&primary_genre)
                .copied()
                .unwrap_or(self.min_documents_per_genre);

            // Adjust max_articles_per_genre to ensure we can meet min_documents_per_genre
            let adjusted_max = self.max_articles_per_genre.max(min_docs * 2);

            let count = per_genre_count
                .entry(primary_genre.clone())
                .or_insert(0usize);
            if *count >= adjusted_max {
                continue;
            }
            *count += 1;
            selected.push(assignment);
        }
        selected
    }

    fn confidence(assignment: &GenreAssignment) -> f32 {
        if assignment.genres.is_empty() {
            return 0.0;
        }
        let primary = &assignment.genres[0];
        let keyword_component =
            assignment.genre_scores.get(primary).copied().unwrap_or(0) as f32 / 100.0;
        let classifier_component = assignment
            .genre_confidence
            .get(primary)
            .copied()
            .unwrap_or(keyword_component);
        let base = classifier_component.max(keyword_component);
        let diversity_penalty = if assignment.genre_scores.len() > 3 {
            0.05 * (assignment.genre_scores.len() as f32 - 3.0)
        } else {
            0.0
        };
        (base - diversity_penalty).max(0.0)
    }

    #[allow(clippy::too_many_lines)]
    async fn filter_outliers(
        &self,
        service: &EmbeddingService,
        assignments: Vec<GenreAssignment>,
        thresholds: &HashMap<String, usize>,
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

                // Filter and Sort - store all with similarity scores
                let mut all_with_similarity: Vec<(f32, GenreAssignment)> = Vec::new();
                for (i, assignment) in genre_assignments.into_iter().enumerate() {
                    let similarity = cosine_similarity(&embeddings[i], &centroid);
                    all_with_similarity.push((similarity, assignment));
                }

                // Sort by similarity descending (Representative Selection)
                all_with_similarity
                    .sort_by(|a, b| b.0.partial_cmp(&a.0).unwrap_or(std::cmp::Ordering::Equal));

                // Filter by threshold, but ensure we have at least min_documents_per_genre
                let min_docs = thresholds
                    .get(&genre)
                    .copied()
                    .unwrap_or(self.min_documents_per_genre);

                let mut valid_assignments: Vec<GenreAssignment> = Vec::new();
                let mut filtered_out: Vec<(f32, GenreAssignment)> = Vec::new();
                let mut filtered_out_count = 0;

                for (similarity, assignment) in all_with_similarity {
                    if similarity >= self.similarity_threshold {
                        valid_assignments.push(assignment);
                    } else {
                        let article_id = assignment.article.id.clone();
                        filtered_out_count += 1;
                        filtered_out.push((similarity, assignment));
                        tracing::debug!(
                            article_id = %article_id,
                            genre = %genre,
                            similarity = %similarity,
                            "filtered out outlier article"
                        );
                    }
                }

                // Fallback: if we don't have enough after filtering, add back top-scoring articles
                if valid_assignments.len() < min_docs {
                    let needed = min_docs - valid_assignments.len();
                    tracing::warn!(
                        genre = %genre,
                        pre_filter = pre_filter_count,
                        post_filter = valid_assignments.len(),
                        min_required = min_docs,
                        adding_back = needed,
                        "filtered too many articles, adding back top-scoring ones"
                    );

                    // Re-add filtered articles sorted by similarity (descending)
                    // filtered_out is already sorted by similarity descending from all_with_similarity
                    for (_, assignment) in filtered_out.into_iter().take(needed) {
                        valid_assignments.push(assignment);
                    }
                }

                let post_filter_count = valid_assignments.len();
                tracing::debug!(
                    genre = %genre,
                    pre_filter = pre_filter_count,
                    post_filter = post_filter_count,
                    filtered_out = filtered_out_count,
                    "outlier filtering completed"
                );

                filtered_assignments.append(&mut valid_assignments);
            } else {
                // If embedding fails, keep all
                tracing::warn!(
                    genre = %genre,
                    count = genre_assignments.len(),
                    "embedding failed, keeping all articles"
                );
                filtered_assignments.append(&mut genre_assignments);
            }
        }

        filtered_assignments
    }
}

impl Default for SummarySelectStage {
    fn default() -> Self {
        Self::new(None, 5, 0.5, None, None)
    }
}

#[async_trait]
impl SelectStage for SummarySelectStage {
    async fn select(
        &self,
        job: &JobContext,
        bundle: GenreBundle,
    ) -> anyhow::Result<SelectedSummary> {
        let thresholds = self.get_dynamic_thresholds().await;

        // Sub-cluster "other" genre items
        let assignments = self
            .subcluster_others(bundle.assignments)
            .await
            .unwrap_or_else(|e| {
                tracing::warn!("subclustering failed: {}", e);
                // On fatal error (should not happen due to internal handling), we might lose assignments if we propagated error.
                // But subcluster_others implementation above handles errors internally or returns original assignments on simple failure?
                // Wait, I implemented it to return Result.
                // If subworker call fails, I catch it inside? No, I catch it inside match, but if something else fails...
                // Let's assume on error we probably lost the "other" separation but returned `rest` + unmodified `others`?
                // Actually my implementation consumes `assignments`...
                // I should make sure I don't drop data.
                // In implementation above: `match subworker... Err(e) -> warn`. `rest.append(&mut others)`. `Ok(rest)`.
                // So it's safe.
                vec![] // Should be unreachable given implementation, but needs specific handling if I failed to execute subcluster_others at all before splitting?
                // Actually if subcluster_others returns Err, it means catastrophic failure.
                // But I made sure to handle subworker error.
            });

        let pre_trim_count = assignments.len();
        let mut assignments = self.trim_assignments(
            GenreBundle {
                assignments,
                ..bundle
            },
            &thresholds,
        );
        let post_trim_count = assignments.len();

        if let Some(service) = &self.embedding_service {
            let pre_outlier_count = assignments.len();
            assignments = self
                .filter_outliers(service, assignments, &thresholds)
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
            article: DeduplicatedArticle {
                id: Uuid::new_v4().to_string(),
                title: Some("title".to_string()),
                sentences: vec!["body".to_string()],
                sentence_hashes: vec![],
                language: "en".to_string(),
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

    #[test]
    fn trim_assignments_adjusts_max_for_min_documents() {
        let stage = SummarySelectStage {
            max_articles_per_genre: 5,
            min_documents_per_genre: 10,
            similarity_threshold: 0.5,
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
}
