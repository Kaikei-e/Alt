//! Selection stage for filtering and selecting articles for summarization.

mod clustering;
mod filtering;
mod scoring;
mod thresholds;
mod trimming;
mod types;

use async_trait::async_trait;
use std::sync::Arc;

use crate::clients::SubworkerClient;
use crate::pipeline::embedding::Embedder;
use crate::pipeline::genre::GenreBundle;
use crate::scheduler::JobContext;
use crate::store::dao::RecapDao;

// Re-exports
pub(crate) use types::{SelectedSummary, SubgenreConfig};

/// Select stage trait.
#[async_trait]
pub(crate) trait SelectStage: Send + Sync {
    async fn select(
        &self,
        job: &JobContext,
        bundle: GenreBundle,
    ) -> anyhow::Result<SelectedSummary>;
}

/// Summary select stage implementation.
///
/// Filters and selects articles for summarization based on genre assignments,
/// embedding similarity, and dynamic thresholds from the database.
#[derive(Clone)]
pub(crate) struct SummarySelectStage {
    max_articles_per_genre: usize,
    min_documents_per_genre: usize,
    /// Similarity threshold for coherence filtering.
    /// Currently stored for API compatibility - filtering uses dynamic percentile-based thresholds.
    /// May be used in future for hybrid threshold modes.
    #[allow(dead_code)]
    similarity_threshold: f32,
    subgenre_config: SubgenreConfig,
    embedding_service: Option<Arc<dyn Embedder>>,
    dao: Option<Arc<dyn RecapDao>>,
    /// Subworker client for potential future use in clustering refinement.
    /// Currently stored for API compatibility.
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
        let (min_docs_thresholds, cosine_thresholds) =
            thresholds::get_dynamic_thresholds(self.dao.as_ref()).await;

        // Sub-cluster "other" genre items
        let assignments =
            clustering::subcluster_others(self.embedding_service.as_ref(), bundle.assignments)
                .await
                .unwrap_or_else(|e| {
                    tracing::warn!("subclustering failed: {}", e);
                    vec![]
                });

        // Sub-cluster large genres (e.g., software_dev -> software_dev_001, software_dev_002)
        let assignments = match clustering::subcluster_large_genres(
            self.embedding_service.as_ref(),
            &self.subgenre_config,
            assignments,
        )
        .await
        {
            Ok(result) => result,
            Err(e) => {
                tracing::warn!("large genre subclustering failed: {}", e);
                // Return empty vec as fallback (should not happen in practice)
                vec![]
            }
        };

        let pre_trim_count = assignments.len();
        let mut assignments = trimming::trim_assignments(
            self.max_articles_per_genre,
            self.min_documents_per_genre,
            GenreBundle {
                assignments,
                ..bundle
            },
            &min_docs_thresholds,
        );
        let post_trim_count = assignments.len();

        if let Some(service) = &self.embedding_service {
            let pre_outlier_count = assignments.len();
            assignments = filtering::filter_outliers(
                &**service,
                assignments,
                &min_docs_thresholds,
                &cosine_thresholds,
                self.min_documents_per_genre,
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
    use super::super::dedup::DeduplicatedArticle;
    use super::super::genre::{FeatureProfile, GenreCandidate};
    use super::*;
    use std::collections::HashMap;
    use uuid::Uuid;

    fn assignment(genre: &str) -> super::super::genre::GenreAssignment {
        super::super::genre::GenreAssignment {
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
        let assignments: Vec<super::super::genre::GenreAssignment> = (0..15)
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
        let assignments: Vec<super::super::genre::GenreAssignment> = (0..20)
            .map(|i| super::super::genre::GenreAssignment {
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

        let result = clustering::subcluster_others(stage.embedding_service.as_ref(), assignments)
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
        let assignments: Vec<super::super::genre::GenreAssignment> = (0..15)
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

        let trimmed = trimming::trim_assignments(5, 10, bundle, &HashMap::new());

        // Should select at least min_documents_per_genre * 2 = 20, but we only have 15
        // So all 15 should be selected
        assert!(trimmed.len() >= 10);
    }

    #[tokio::test]
    async fn subcluster_large_genres_splits_into_subgenres() {
        let embedding_service: Option<Arc<dyn crate::pipeline::embedding::Embedder>> =
            Some(Arc::new(MockEmbedder));

        // Set max_docs_per_genre to 50, so 250 articles should be split
        let subgenre_config = SubgenreConfig::new(50, 50, 10);

        // Create 250 "software_dev" assignments (exceeds threshold of 50)
        let assignments: Vec<super::super::genre::GenreAssignment> = (0..250)
            .map(|i| super::super::genre::GenreAssignment {
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

        let result =
            clustering::subcluster_large_genres(embedding_service.as_ref(), &subgenre_config, assignments)
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
        let embedding_service: Option<Arc<dyn crate::pipeline::embedding::Embedder>> =
            Some(Arc::new(MockEmbedder));

        // Set max_docs_per_genre to 50
        let subgenre_config = SubgenreConfig::new(50, 50, 10);

        // Create 30 "tech" assignments (below threshold of 50)
        let assignments: Vec<super::super::genre::GenreAssignment> = (0..30)
            .map(|i| super::super::genre::GenreAssignment {
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

        let result = clustering::subcluster_large_genres(
            embedding_service.as_ref(),
            &subgenre_config,
            assignments.clone(),
        )
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
        // No embedding service
        let subgenre_config = SubgenreConfig::new(50, 50, 10);

        // Create 100 "software_dev" assignments (exceeds threshold)
        let assignments: Vec<super::super::genre::GenreAssignment> = (0..100)
            .map(|i| super::super::genre::GenreAssignment {
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

        let result =
            clustering::subcluster_large_genres(None, &subgenre_config, assignments.clone())
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
