/// Prometheusメトリクス定義。
use prometheus::{
    register_counter_with_registry, register_gauge_with_registry, register_histogram_with_registry,
    Counter, Gauge, Histogram, Registry,
};
use std::sync::Arc;

/// メトリクスコレクター。
#[derive(Debug, Clone)]
pub(crate) struct Metrics {
    // カウンター
    pub(crate) articles_fetched: Counter,
    pub(crate) articles_processed: Counter,
    pub(crate) articles_dropped: Counter,
    pub(crate) clusters_created: Counter,
    pub(crate) summaries_generated: Counter,
    pub(crate) jobs_completed: Counter,
    pub(crate) jobs_failed: Counter,
    pub(crate) retries_total: Counter,

    // ヒストグラム
    pub(crate) fetch_duration: Histogram,
    pub(crate) preprocess_duration: Histogram,
    pub(crate) dedup_duration: Histogram,
    pub(crate) clustering_duration: Histogram,
    pub(crate) summary_duration: Histogram,
    pub(crate) job_duration: Histogram,

    // ゲージ
    pub(crate) active_jobs: Gauge,
    pub(crate) queue_size: Gauge,
}

impl Metrics {
    /// 新しいメトリクスコレクターを作成する。
    pub(crate) fn new(registry: Arc<Registry>) -> Result<Self, prometheus::Error> {
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
            clustering_duration: register_histogram_with_registry!(
                "recap_clustering_duration_seconds",
                "Duration of clustering operations",
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
