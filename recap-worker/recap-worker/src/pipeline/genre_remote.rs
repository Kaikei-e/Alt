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
    threshold: f32,
}

impl RemoteGenreStage {
    pub(crate) fn new(
        client: Arc<SubworkerClient>,
        queue: Arc<ClassificationJobQueue>,
        threshold: f32,
    ) -> Self {
        Self {
            client,
            queue,
            threshold,
        }
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
    fn create_genre_assignment(
        &self,
        article: DeduplicatedArticle,
        result: crate::clients::subworker::ClassificationResult,
    ) -> GenreAssignment {
        let top_genre = if result.confidence >= self.threshold {
            result.top_genre
        } else {
            "other".to_string()
        };

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
