use std::collections::HashMap;
use std::sync::Arc;

use anyhow::Result;
use async_trait::async_trait;
use tracing::{debug, info};

use crate::clients::SubworkerClient;
use crate::pipeline::dedup::DeduplicatedCorpus;
use crate::pipeline::genre::{
    FeatureProfile, GenreAssignment, GenreBundle, GenreCandidate, GenreStage,
};
use crate::scheduler::JobContext;

pub(crate) struct RemoteGenreStage {
    client: Arc<SubworkerClient>,
    threshold: f32,
}

impl RemoteGenreStage {
    pub(crate) fn new(client: Arc<SubworkerClient>, threshold: f32) -> Self {
        Self { client, threshold }
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
        // We use title + first 5 sentences as the input text
        let texts: Vec<String> = corpus
            .articles
            .iter()
            .map(|a| {
                let title = a.title.as_deref().unwrap_or("");
                let body = a.sentences.iter().take(5).cloned().collect::<Vec<_>>().join(" ");
                format!("{title} {body}")
            })
            .collect();

        // Call remote service (async job pattern)
        let results = self.client.classify_texts(job.job_id, texts).await?;

        let mut assignments = Vec::with_capacity(total_articles);
        let mut genre_distribution: HashMap<String, usize> = HashMap::new();

        for (article, result) in corpus.articles.into_iter().zip(results) {
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

            // Update distribution
            *genre_distribution.entry(top_genre.clone()).or_insert(0) += 1;

            // Create candidates
            let mut candidates: Vec<GenreCandidate> = result
                .scores
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

            // Create assignment
            // For now, we just take the top genre.
            let genres = vec![top_genre.clone()];

            // Convert scores to usize for genre_scores (legacy format, scaled 0-100)
            let genre_scores: HashMap<String, usize> = result
                .scores
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
                .collect();

            assignments.push(GenreAssignment {
                genres,
                candidates,
                genre_scores,
                genre_confidence: result.scores,
                feature_profile: FeatureProfile::default(), // We don't have feature profile from remote yet
                article,
            });
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
