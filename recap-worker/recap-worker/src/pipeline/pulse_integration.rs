//! Pulse generation integration for the pipeline.

use std::collections::HashMap;

use chrono::{DateTime, Utc};

use crate::scheduler::JobContext;

use super::dispatch::DispatchResult;
use super::pulse::{self, PulseRollout, PulseStage};
use super::PipelineOrchestrator;

impl PipelineOrchestrator {
    /// Generate Evening Pulse if enabled via rollout.
    ///
    /// Extracts cluster data from dispatch results and generates pulse topics.
    /// Results are saved to the pulse_generations table.
    pub(crate) async fn generate_pulse_if_enabled(
        &self,
        job: &JobContext,
        dispatched: &DispatchResult,
    ) {
        // Check if Pulse is enabled for this job
        if !self.pulse_rollout().allows(job.job_id) {
            tracing::debug!(
                job_id = %job.job_id,
                rollout_pct = self.pulse_rollout().percentage(),
                "pulse generation skipped: not in rollout"
            );
            return;
        }

        tracing::info!(
            job_id = %job.job_id,
            version = %self.pulse_rollout().version(),
            "starting pulse generation"
        );

        // Collect all article_ids from dispatch results
        let article_ids: Vec<String> = dispatched
            .genre_results
            .values()
            .filter_map(|gr| gr.clustering_response.as_ref())
            .flat_map(|cr| &cr.clusters)
            .flat_map(|c| &c.representatives)
            .map(|rep| rep.article_id.clone())
            .collect();

        // Fetch metadata (published_at, source_url) from DB
        let metadata = self
            .recap_dao()
            .get_article_metadata(job.job_id, &article_ids)
            .await
            .unwrap_or_else(|e| {
                tracing::warn!(
                    job_id = %job.job_id,
                    error = ?e,
                    "failed to fetch article metadata for pulse, using empty metadata"
                );
                HashMap::new()
            });

        tracing::debug!(
            job_id = %job.job_id,
            article_count = article_ids.len(),
            metadata_count = metadata.len(),
            "fetched article metadata for pulse generation"
        );

        // Convert dispatch results to PulseInput with metadata
        let pulse_input = build_pulse_input(job.job_id, dispatched, &metadata);

        // Generate pulse
        match self.pulse_stage().generate(pulse_input).await {
            Ok(result) => {
                let topic_count = result.topic_count();
                let target_date = chrono::Utc::now().date_naive();

                // Save pulse generation result
                match self
                    .recap_dao()
                    .save_pulse_generation(&result, target_date)
                    .await
                {
                    Ok(generation_id) => {
                        tracing::info!(
                            job_id = %job.job_id,
                            generation_id = generation_id,
                            topic_count = topic_count,
                            version = %result.version,
                            "pulse generation saved successfully"
                        );
                    }
                    Err(e) => {
                        tracing::warn!(
                            job_id = %job.job_id,
                            error = ?e,
                            "failed to save pulse generation"
                        );
                    }
                }
            }
            Err(e) => {
                tracing::warn!(
                    job_id = %job.job_id,
                    error = ?e,
                    "pulse generation failed"
                );
            }
        }
    }

    /// Access the pulse rollout configuration.
    pub(super) fn pulse_rollout(&self) -> &PulseRollout {
        &self.pulse_rollout
    }

    /// Access the pulse stage.
    pub(super) fn pulse_stage(&self) -> &dyn PulseStage {
        self.pulse_stage.as_ref()
    }
}

/// Metadata for an article (published_at, source_url).
type ArticleMetadata = HashMap<String, (Option<DateTime<Utc>>, Option<String>)>;

/// Build PulseInput from dispatch results with article metadata.
pub(super) fn build_pulse_input(
    job_id: uuid::Uuid,
    dispatched: &DispatchResult,
    metadata: &ArticleMetadata,
) -> pulse::PulseInput {
    let mut clusters = Vec::new();

    for (genre, genre_result) in &dispatched.genre_results {
        if let Some(clustering_response) = &genre_result.clustering_response {
            for cluster in &clustering_response.clusters {
                // Convert cluster representatives to articles with metadata from DB
                let articles: Vec<pulse::ArticleInput> = cluster
                    .representatives
                    .iter()
                    .map(|rep| {
                        let (published_at, source_url) = metadata
                            .get(&rep.article_id)
                            .cloned()
                            .unwrap_or((None, None));

                        pulse::ArticleInput {
                            id: rep.article_id.clone(),
                            title: rep.text.clone(),
                            source_url: source_url.unwrap_or_default(),
                            canonical_url: None,
                            og_url: None,
                            entities: Vec::new(), // Not stored in DB
                            published_at: published_at.map(|dt| dt.to_rfc3339()),
                        }
                    })
                    .collect();

                if !articles.is_empty() {
                    clusters.push(pulse::ClusterInput {
                        cluster_id: i64::from(cluster.cluster_id),
                        label: cluster.label.clone().or_else(|| {
                            Some(format!("{} Cluster {}", genre, cluster.cluster_id))
                        }),
                        articles,
                        embeddings: Vec::new(), // Not available in clustering response
                        impact_score: None,
                        burst_score: None,
                        novelty_score: None,
                        recency_score: None,
                    });
                }
            }
        }
    }

    pulse::PulseInput { job_id, clusters }
}
