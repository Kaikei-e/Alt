/// Prometheusメトリクス定義。
use prometheus::{
    Counter, Gauge, Histogram, Registry, register_counter_with_registry,
    register_gauge_with_registry, register_histogram_with_registry,
};
use std::sync::Arc;

/// メトリクスコレクター。
#[derive(Debug, Clone)]
pub struct Metrics {
    // カウンター
    pub articles_fetched: Counter,
    pub articles_processed: Counter,
    pub articles_dropped: Counter,
    pub clusters_created: Counter,
    pub summaries_generated: Counter,
    pub jobs_completed: Counter,
    pub jobs_failed: Counter,
    pub retries_total: Counter,
    pub api_evidence_duplicates: Counter,
    pub genre_refine_graph_hits: Counter,
    pub genre_refine_fallback_total: Counter,
    pub genre_refine_rollout_enabled: Counter,
    pub genre_refine_rollout_skipped: Counter,
    pub clustering_timeouts: Counter,
    pub clustering_stuck_detected: Counter,
    pub clustering_poll_attempts: Counter,
    pub clustering_partial_success: Counter,

    // Evening Pulse カウンター
    pub pulse_generations_total: Counter,
    pub pulse_generations_failed: Counter,
    pub pulse_quality_tier_ok: Counter,
    pub pulse_quality_tier_caution: Counter,
    pub pulse_quality_tier_ng: Counter,
    pub pulse_syndication_removed: Counter,
    pub pulse_rollout_enabled: Counter,
    pub pulse_rollout_skipped: Counter,
    pub pulse_fallback_triggered: Counter,

    // ヒストグラム
    pub fetch_duration: Histogram,
    pub preprocess_duration: Histogram,
    pub dedup_duration: Histogram,
    pub genre_candidate_latency: Histogram,
    pub genre_refine_llm_latency: Histogram,
    pub clustering_duration: Histogram,
    pub clustering_poll_duration: Histogram,
    pub clustering_run_age: Histogram,
    pub summary_duration: Histogram,
    pub job_duration: Histogram,
    pub api_latest_fetch_duration: Histogram,
    pub api_cluster_query_duration: Histogram,

    // Evening Pulse ヒストグラム
    pub pulse_generation_duration: Histogram,
    pub pulse_quality_evaluation_duration: Histogram,
    pub pulse_selection_duration: Histogram,

    // ゲージ
    pub active_jobs: Gauge,
    pub queue_size: Gauge,
}

impl Metrics {
    /// 新しいメトリクスコレクターを作成する。
    #[allow(clippy::too_many_lines)]
    pub fn new(registry: Arc<Registry>) -> Result<Self, prometheus::Error> {
        Ok(Self {
            articles_fetched: register_counter_with_registry!(
                "recap_articles_fetched_total",
                "Total number of articles fetched",
                registry
            )?,
            articles_processed: register_counter_with_registry!(
                "recap_articles_processed_total",
                "Total number of articles processed",
                registry
            )?,
            articles_dropped: register_counter_with_registry!(
                "recap_articles_dropped_total",
                "Total number of articles dropped",
                registry
            )?,
            clusters_created: register_counter_with_registry!(
                "recap_clusters_created_total",
                "Total number of clusters created",
                registry
            )?,
            summaries_generated: register_counter_with_registry!(
                "recap_summaries_generated_total",
                "Total number of summaries generated",
                registry
            )?,
            jobs_completed: register_counter_with_registry!(
                "recap_jobs_completed_total",
                "Total number of jobs completed",
                registry
            )?,
            jobs_failed: register_counter_with_registry!(
                "recap_jobs_failed_total",
                "Total number of jobs failed",
                registry
            )?,
            retries_total: register_counter_with_registry!(
                "recap_retries_total",
                "Total number of retries",
                registry
            )?,
            api_evidence_duplicates: register_counter_with_registry!(
                "recap_api_evidence_duplicates_total",
                "Number of duplicate evidence links filtered at the API layer",
                registry
            )?,
            genre_refine_graph_hits: register_counter_with_registry!(
                "recap_genre_graph_hits_total",
                "Number of times graph-based boosts influenced genre refinement",
                registry
            )?,
            genre_refine_fallback_total: register_counter_with_registry!(
                "recap_genre_refine_fallback_total",
                "Number of genre refinement fallbacks to coarse results",
                registry
            )?,
            genre_refine_rollout_enabled: register_counter_with_registry!(
                "recap_genre_refine_rollout_enabled_total",
                "Jobs where the genre refine gate was opened",
                registry
            )?,
            genre_refine_rollout_skipped: register_counter_with_registry!(
                "recap_genre_refine_rollout_skipped_total",
                "Jobs where the genre refine gate was closed by rollout policy",
                registry
            )?,
            clustering_timeouts: register_counter_with_registry!(
                "recap_clustering_timeouts_total",
                "Number of clustering operations that timed out",
                registry
            )?,
            clustering_stuck_detected: register_counter_with_registry!(
                "recap_clustering_stuck_detected_total",
                "Number of clustering runs detected as stuck",
                registry
            )?,
            clustering_poll_attempts: register_counter_with_registry!(
                "recap_clustering_poll_attempts_total",
                "Total number of polling attempts for clustering runs",
                registry
            )?,
            clustering_partial_success: register_counter_with_registry!(
                "recap_clustering_partial_success_total",
                "Number of jobs completed with partial success (some genres failed)",
                registry
            )?,
            // Evening Pulse counters
            pulse_generations_total: register_counter_with_registry!(
                "recap_pulse_generations_total",
                "Total number of pulse generation runs",
                registry
            )?,
            pulse_generations_failed: register_counter_with_registry!(
                "recap_pulse_generations_failed_total",
                "Number of pulse generation failures",
                registry
            )?,
            pulse_quality_tier_ok: register_counter_with_registry!(
                "recap_pulse_quality_tier_ok_total",
                "Number of clusters with OK quality tier",
                registry
            )?,
            pulse_quality_tier_caution: register_counter_with_registry!(
                "recap_pulse_quality_tier_caution_total",
                "Number of clusters with Caution quality tier",
                registry
            )?,
            pulse_quality_tier_ng: register_counter_with_registry!(
                "recap_pulse_quality_tier_ng_total",
                "Number of clusters with NG quality tier",
                registry
            )?,
            pulse_syndication_removed: register_counter_with_registry!(
                "recap_pulse_syndication_removed_total",
                "Number of articles removed due to syndication detection",
                registry
            )?,
            pulse_rollout_enabled: register_counter_with_registry!(
                "recap_pulse_rollout_enabled_total",
                "Jobs where pulse v4 was enabled by rollout",
                registry
            )?,
            pulse_rollout_skipped: register_counter_with_registry!(
                "recap_pulse_rollout_skipped_total",
                "Jobs where pulse v4 was skipped by rollout policy",
                registry
            )?,
            pulse_fallback_triggered: register_counter_with_registry!(
                "recap_pulse_fallback_triggered_total",
                "Number of times pulse selection fell back to lower tiers",
                registry
            )?,
            fetch_duration: register_histogram_with_registry!(
                "recap_fetch_duration_seconds",
                "Duration of fetch operations",
                registry
            )?,
            preprocess_duration: register_histogram_with_registry!(
                "recap_preprocess_duration_seconds",
                "Duration of preprocessing operations",
                registry
            )?,
            dedup_duration: register_histogram_with_registry!(
                "recap_dedup_duration_seconds",
                "Duration of deduplication operations",
                registry
            )?,
            genre_candidate_latency: register_histogram_with_registry!(
                "recap_genre_candidate_latency_seconds",
                "Latency of coarse genre candidate production per article",
                registry
            )?,
            genre_refine_llm_latency: register_histogram_with_registry!(
                "recap_genre_refine_llm_latency_seconds",
                "Latency of LLM tie-breaker calls during genre refinement",
                registry
            )?,
            clustering_duration: register_histogram_with_registry!(
                "recap_clustering_duration_seconds",
                "Duration of clustering operations",
                registry
            )?,
            clustering_poll_duration: register_histogram_with_registry!(
                "recap_clustering_poll_duration_seconds",
                "Duration spent polling for clustering run completion",
                registry
            )?,
            clustering_run_age: register_histogram_with_registry!(
                "recap_clustering_run_age_seconds",
                "Age of clustering runs in running state",
                registry
            )?,
            summary_duration: register_histogram_with_registry!(
                "recap_summary_duration_seconds",
                "Duration of summary generation",
                registry
            )?,
            job_duration: register_histogram_with_registry!(
                "recap_job_duration_seconds",
                "Duration of entire job processing",
                registry
            )?,
            api_latest_fetch_duration: register_histogram_with_registry!(
                "recap_api_latest_fetch_duration_seconds",
                "Duration of GET /v1/recaps/7days handler",
                registry
            )?,
            api_cluster_query_duration: register_histogram_with_registry!(
                "recap_api_cluster_query_duration_seconds",
                "Duration spent loading cluster evidence inside GET /v1/recaps/7days",
                registry
            )?,
            // Evening Pulse histograms
            pulse_generation_duration: register_histogram_with_registry!(
                "recap_pulse_generation_duration_seconds",
                "Duration of pulse generation",
                registry
            )?,
            pulse_quality_evaluation_duration: register_histogram_with_registry!(
                "recap_pulse_quality_evaluation_duration_seconds",
                "Duration of cluster quality evaluation",
                registry
            )?,
            pulse_selection_duration: register_histogram_with_registry!(
                "recap_pulse_selection_duration_seconds",
                "Duration of topic selection",
                registry
            )?,
            active_jobs: register_gauge_with_registry!(
                "recap_active_jobs",
                "Number of currently active jobs",
                registry
            )?,
            queue_size: register_gauge_with_registry!(
                "recap_queue_size",
                "Number of jobs in queue",
                registry
            )?,
        })
    }
}
