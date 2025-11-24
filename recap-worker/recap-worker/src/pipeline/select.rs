use async_trait::async_trait;
use uuid::Uuid;

use crate::scheduler::JobContext;

use super::embedding::{EmbeddingService, cosine_similarity};
use super::genre::{GenreAssignment, GenreBundle};

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
    embedding_service: Option<EmbeddingService>,
}

impl SummarySelectStage {
    pub(crate) fn new(embedding_service: Option<EmbeddingService>) -> Self {
        Self {
            max_articles_per_genre: 20,
            embedding_service,
        }
    }

    fn trim_assignments(&self, bundle: GenreBundle) -> Vec<GenreAssignment> {
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
            let count = per_genre_count
                .entry(primary_genre.clone())
                .or_insert(0usize);
            if *count >= self.max_articles_per_genre {
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

    async fn filter_outliers(
        &self,
        service: &EmbeddingService,
        assignments: Vec<GenreAssignment>,
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

                // Filter and Sort
                let mut valid_assignments: Vec<(f32, GenreAssignment)> = Vec::new();
                for (i, assignment) in genre_assignments.into_iter().enumerate() {
                    let similarity = cosine_similarity(&embeddings[i], &centroid);
                    // Threshold 0.6: Keep articles reasonably close to the center.
                    if similarity >= 0.6 {
                        valid_assignments.push((similarity, assignment));
                    } else {
                        tracing::debug!(
                            article_id = %assignment.article.id,
                            genre = %genre,
                            similarity = %similarity,
                            "filtered out outlier article"
                        );
                    }
                }

                // Sort by similarity descending (Representative Selection)
                valid_assignments
                    .sort_by(|a, b| b.0.partial_cmp(&a.0).unwrap_or(std::cmp::Ordering::Equal));

                for (_, assignment) in valid_assignments {
                    filtered_assignments.push(assignment);
                }
            } else {
                // If embedding fails, keep all
                filtered_assignments.append(&mut genre_assignments);
            }
        }

        filtered_assignments
    }
}

impl Default for SummarySelectStage {
    fn default() -> Self {
        Self::new(None)
    }
}

#[async_trait]
impl SelectStage for SummarySelectStage {
    async fn select(
        &self,
        job: &JobContext,
        bundle: GenreBundle,
    ) -> anyhow::Result<SelectedSummary> {
        let mut assignments = self.trim_assignments(bundle);

        if let Some(service) = &self.embedding_service {
            assignments = self.filter_outliers(service, assignments).await;
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
            embedding_service: None,
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

        assert_eq!(selected.assignments.len(), 2);
        assert!(
            selected
                .assignments
                .iter()
                .any(|a| a.genres.contains(&"ai".to_string()))
        );
    }
}
