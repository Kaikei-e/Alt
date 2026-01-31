//! Pulse generation integration for the pipeline.

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

        // Convert dispatch results to PulseInput
        let pulse_input = build_pulse_input(job.job_id, dispatched);

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

/// Build PulseInput from dispatch results.
pub(super) fn build_pulse_input(
    job_id: uuid::Uuid,
    dispatched: &DispatchResult,
) -> pulse::PulseInput {
    let mut clusters = Vec::new();

    for (genre, genre_result) in &dispatched.genre_results {
        if let Some(clustering_response) = &genre_result.clustering_response {
            for cluster in &clustering_response.clusters {
                // Convert cluster representatives to articles
                let articles: Vec<pulse::ArticleInput> = cluster
                    .representatives
                    .iter()
                    .map(|rep| pulse::ArticleInput {
                        id: rep.article_id.clone(),
                        title: rep.text.clone(),
                        source_url: String::new(), // Not available in clustering response
                        canonical_url: None,
                        og_url: None,
                        entities: Vec::new(), // Not available in clustering response
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
