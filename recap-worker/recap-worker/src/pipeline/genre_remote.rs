use std::collections::HashMap;
use std::sync::Arc;

use anyhow::Result;
use async_trait::async_trait;
use tracing::{debug, info};

use crate::clients::SubworkerClient;
use crate::pipeline::dedup::{DeduplicatedArticle, DeduplicatedCorpus};
use crate::pipeline::genre::{
    FeatureProfile, GenreAssignment, GenreBundle, GenreCandidate, GenreStage,
};
use crate::queue::ClassificationJobQueue;
use crate::scheduler::JobContext;

pub(crate) struct RemoteGenreStage {
    client: Arc<SubworkerClient>,
    queue: Arc<ClassificationJobQueue>,
}

impl RemoteGenreStage {
    pub(crate) fn new(client: Arc<SubworkerClient>, queue: Arc<ClassificationJobQueue>) -> Self {
        Self { client, queue }
    }

    /// Prepare texts for classification from articles
    fn prepare_texts_for_classification(articles: &[DeduplicatedArticle]) -> Vec<String> {
        articles
            .iter()
            .map(|a| {
                let title = a.title.as_deref().unwrap_or("");
                let body = a
                    .sentences
                    .iter()
                    .take(5)
                    .cloned()
                    .collect::<Vec<_>>()
                    .join(" ");
                format!("{title} {body}")
            })
            .collect()
    }

    /// Convert classification scores to genre_scores format (0-100 scale)
    fn convert_scores_to_genre_scores(scores: &HashMap<String, f32>) -> HashMap<String, usize> {
        scores
            .iter()
            .map(|(k, v)| {
                let scaled = (v * 100.0).max(0.0).round();
                // Safe conversion: handle NaN/negative, then convert via i64 to avoid sign loss
                let scaled_usize = if scaled.is_nan() || scaled < 0.0 {
                    0
                } else {
                    // Convert to i64 first (safe for non-negative values), then to usize
                    let as_i64 = scaled as i64;
                    if as_i64 < 0 {
                        0
                    } else {
                        usize::try_from(as_i64).unwrap_or(0)
                    }
                };
                (k.clone(), scaled_usize)
            })
            .collect()
    }

    /// Create genre candidates from classification result
    fn create_genre_candidates(scores: &HashMap<String, f32>) -> Vec<GenreCandidate> {
        let mut candidates: Vec<GenreCandidate> = scores
            .iter()
            .map(|(name, score)| GenreCandidate {
                name: name.clone(),
                score: *score,
                keyword_support: 0, // Not available from remote
                classifier_confidence: *score,
            })
            .collect();

        // Sort candidates by score descending
        candidates.sort_by(|a, b| {
            b.score
                .partial_cmp(&a.score)
                .unwrap_or(std::cmp::Ordering::Equal)
        });

        candidates
    }

    /// Create a genre assignment from an article and classification result
    #[allow(clippy::unused_self)] // Reserved for future use
    fn create_genre_assignment(
        &self,
        article: DeduplicatedArticle,
        result: crate::clients::subworker::ClassificationResult,
    ) -> GenreAssignment {
        // Always use the top_genre returned by the subworker, as it has already
        // applied its own genre-specific thresholds
        let top_genre = result.top_genre;

        debug!(
            article_id = %article.id,
            top_genre = %top_genre,
            confidence = %result.confidence,
            "classified article"
        );

        let genres = vec![top_genre.clone()];
        let candidates = Self::create_genre_candidates(&result.scores);
        let genre_scores = Self::convert_scores_to_genre_scores(&result.scores);

        GenreAssignment {
            genres,
            candidates,
            genre_scores,
            genre_confidence: result.scores,
            // Remote stage assumes refinement happens later or remotely, so empty profile initially
            feature_profile: FeatureProfile::default(),
            article,
            embedding: None,
        }
    }
}

#[async_trait]
impl GenreStage for RemoteGenreStage {
    async fn assign(&self, job: &JobContext, corpus: DeduplicatedCorpus) -> Result<GenreBundle> {
        let total_articles = corpus.articles.len();
        info!(
            job_id = %job.job_id,
            count = total_articles,
            "starting remote genre assignment"
        );

        if total_articles == 0 {
            return Ok(GenreBundle {
                job_id: job.job_id,
                assignments: vec![],
                genre_distribution: HashMap::new(),
            });
        }

        // Prepare texts for classification
        let texts = Self::prepare_texts_for_classification(&corpus.articles);

        // Call remote service via queue (async job pattern with retry)
        let results = self
            .client
            .classify_texts_queued(&self.queue, job.job_id, texts)
            .await?;

        // Process results and create assignments
        let mut assignments = Vec::with_capacity(total_articles);
        let mut genre_distribution: HashMap<String, usize> = HashMap::new();

        for (article, result) in corpus.articles.into_iter().zip(results) {
            let assignment = self.create_genre_assignment(article, result);
            let top_genre = assignment.genres[0].clone();
            *genre_distribution.entry(top_genre).or_insert(0) += 1;
            assignments.push(assignment);
        }

        info!(
            job_id = %job.job_id,
            total_assignments = assignments.len(),
            genre_distribution = ?genre_distribution,
            "completed remote genre assignment"
        );

        Ok(GenreBundle {
            job_id: job.job_id,
            assignments,
            genre_distribution,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::pipeline::dedup::DeduplicatedArticle;

    #[test]
    fn test_prepare_texts_for_classification() {
        let articles = vec![
            DeduplicatedArticle {
                id: "art-1".to_string(),
                title: Some("Test Title".to_string()),
                sentences: vec![
                    "First sentence.".to_string(),
                    "Second sentence.".to_string(),
                    "Third sentence.".to_string(),
                    "Fourth sentence.".to_string(),
                    "Fifth sentence.".to_string(),
                    "Sixth sentence.".to_string(),
                ],
                sentence_hashes: vec![],
                language: "en".to_string(),
                published_at: None,
                source_url: None,
                tags: vec![],
                duplicates: vec![],
            },
            DeduplicatedArticle {
                id: "art-2".to_string(),
                title: None,
                sentences: vec!["Only one sentence.".to_string()],
                sentence_hashes: vec![],
                language: "en".to_string(),
                published_at: None,
                source_url: None,
                tags: vec![],
                duplicates: vec![],
            },
        ];

        let texts = RemoteGenreStage::prepare_texts_for_classification(&articles);

        assert_eq!(texts.len(), 2);
        assert!(texts[0].contains("Test Title"));
        assert!(texts[0].contains("First sentence"));
        assert!(texts[0].contains("Fifth sentence"));
        assert!(!texts[0].contains("Sixth sentence")); // Only first 5 sentences
        assert!(texts[1].contains("Only one sentence"));
        assert!(!texts[1].contains("None")); // No title prefix when title is None
    }

    #[test]
    fn test_convert_scores_to_genre_scores() {
        let mut scores = HashMap::new();
        scores.insert("tech".to_string(), 0.85);
        scores.insert("science".to_string(), 0.42);
        scores.insert("other".to_string(), 0.01);

        let genre_scores = RemoteGenreStage::convert_scores_to_genre_scores(&scores);

        assert_eq!(genre_scores.get("tech"), Some(&85));
        assert_eq!(genre_scores.get("science"), Some(&42));
        assert_eq!(genre_scores.get("other"), Some(&1));
    }

    #[test]
    fn test_convert_scores_to_genre_scores_handles_negative() {
        let mut scores = HashMap::new();
        scores.insert("negative".to_string(), -0.5);

        let genre_scores = RemoteGenreStage::convert_scores_to_genre_scores(&scores);

        assert_eq!(genre_scores.get("negative"), Some(&0));
    }

    #[test]
    fn test_create_genre_candidates() {
        let mut scores = HashMap::new();
        scores.insert("tech".to_string(), 0.9);
        scores.insert("science".to_string(), 0.7);
        scores.insert("other".to_string(), 0.1);

        let candidates = RemoteGenreStage::create_genre_candidates(&scores);

        assert_eq!(candidates.len(), 3);
        // Should be sorted by score descending
        assert_eq!(candidates[0].name, "tech");
        assert!((candidates[0].score - 0.9).abs() < f32::EPSILON);
        assert_eq!(candidates[1].name, "science");
        assert!((candidates[1].score - 0.7).abs() < f32::EPSILON);
        assert_eq!(candidates[2].name, "other");
        assert!((candidates[2].score - 0.1).abs() < f32::EPSILON);
        // All should have keyword_support = 0 (not available from remote)
        assert_eq!(candidates[0].keyword_support, 0);
        assert_eq!(candidates[1].keyword_support, 0);
        assert_eq!(candidates[2].keyword_support, 0);
    }

    #[tokio::test]
    async fn test_create_genre_assignment_above_threshold() {
        // Use a dummy pool for testing (won't actually connect)
        let pool = sqlx::PgPool::connect_lazy("postgres://test:test@localhost/test").unwrap();
        let stage = RemoteGenreStage::new(
            Arc::new(SubworkerClient::new("http://localhost:8002", 10).unwrap()),
            Arc::new(crate::queue::ClassificationJobQueue::new(
                crate::queue::QueueStore::new(pool),
                SubworkerClient::new("http://localhost:8002", 10).unwrap(),
                1,
                200,
                3,
                5000,
            )),
        );

        let article = DeduplicatedArticle {
            id: "art-1".to_string(),
            title: Some("Test Article".to_string()),
            sentences: vec!["Content".to_string()],
            sentence_hashes: vec![],
            language: "en".to_string(),
            published_at: None,
            source_url: None,
            tags: vec![],
            duplicates: vec![],
        };

        let mut scores = HashMap::new();
        scores.insert("tech".to_string(), 0.9);
        scores.insert("science".to_string(), 0.3);

        let result = crate::clients::subworker::ClassificationResult {
            top_genre: "tech".to_string(),
            confidence: 0.8,
            scores,
        };

        let assignment = stage.create_genre_assignment(article, result);

        assert_eq!(assignment.genres, vec!["tech"]);
        assert_eq!(assignment.candidates.len(), 2);
        assert_eq!(assignment.candidates[0].name, "tech");
    }

    #[tokio::test]
    async fn test_create_genre_assignment_below_threshold() {
        // Use a dummy pool for testing (won't actually connect)
        let pool = sqlx::PgPool::connect_lazy("postgres://test:test@localhost/test").unwrap();
        let stage = RemoteGenreStage::new(
            Arc::new(SubworkerClient::new("http://localhost:8002", 10).unwrap()),
            Arc::new(crate::queue::ClassificationJobQueue::new(
                crate::queue::QueueStore::new(pool),
                SubworkerClient::new("http://localhost:8002", 10).unwrap(),
                1,
                200,
                3,
                5000,
            )),
        );

        let article = DeduplicatedArticle {
            id: "art-1".to_string(),
            title: Some("Test Article".to_string()),
            sentences: vec!["Content".to_string()],
            sentence_hashes: vec![],
            language: "en".to_string(),
            published_at: None,
            source_url: None,
            tags: vec![],
            duplicates: vec![],
        };

        let mut scores = HashMap::new();
        scores.insert("tech".to_string(), 0.3);
        scores.insert("science".to_string(), 0.2);

        let result = crate::clients::subworker::ClassificationResult {
            top_genre: "tech".to_string(),
            confidence: 0.3, // Low confidence, but subworker already applied its thresholds
            scores,
        };

        let assignment = stage.create_genre_assignment(article, result);

        // After removing the threshold gate, we always use the subworker's top_genre
        assert_eq!(assignment.genres, vec!["tech"]);
    }
}
