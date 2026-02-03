use crate::clients::alt_backend::{AltBackendClient, AltBackendConfig};
use crate::config::Config;
use crate::pipeline::dedup::{DedupStage, HashDedupStage};
use crate::pipeline::fetch::{AltBackendFetchStage, FetchStage};
use crate::pipeline::preprocess::{PreprocessStage, TextPreprocessStage};
use crate::scheduler::JobContext;
use crate::store::dao::RecapDao;
use crate::util::retry::RetryConfig;
use anyhow::Result;
use std::sync::Arc;
use uuid::Uuid;

pub struct MorningPipeline {
    #[allow(dead_code)]
    config: Arc<Config>,
    fetch: Arc<dyn FetchStage>,
    preprocess: Arc<dyn PreprocessStage>,
    dedup: Arc<dyn DedupStage>,
    recap_dao: Arc<dyn RecapDao>,
}

impl MorningPipeline {
    pub(crate) fn new(config: Arc<Config>, recap_dao: Arc<dyn RecapDao>) -> Self {
        let alt_backend_config = AltBackendConfig {
            base_url: config.alt_backend_base_url().to_string(),
            connect_timeout: config.alt_backend_connect_timeout(),
            total_timeout: config.alt_backend_total_timeout(),
            service_token: config.alt_backend_service_token().map(ToString::to_string),
        };
        let alt_backend_client = Arc::new(
            AltBackendClient::new(alt_backend_config).expect("failed to create alt-backend client"),
        );

        let retry_config = RetryConfig {
            max_attempts: config.http_max_retries(),
            base_delay_ms: config.http_backoff_base_ms(),
            max_delay_ms: config.http_backoff_cap_ms(),
        };

        let cpu_count = num_cpus::get();
        let max_concurrent = (cpu_count * 3) / 2;

        // Morning Update uses a 1-day window (window_days is now taken from JobContext)
        let fetch = Arc::new(AltBackendFetchStage::new(
            alt_backend_client,
            None, // No tag generator needed for just grouping
            Arc::clone(&recap_dao),
            retry_config,
        ));

        let subworker_client = Arc::new(
            crate::clients::SubworkerClient::new(
                config.subworker_base_url(),
                config.min_documents_per_genre(),
            )
            .expect("failed to create subworker client"),
        );
        let preprocess = Arc::new(TextPreprocessStage::new(
            max_concurrent.max(2),
            Arc::clone(&recap_dao),
            Arc::clone(&subworker_client),
        ));

        let dedup = Arc::new(HashDedupStage::with_defaults());

        Self {
            config,
            fetch,
            preprocess,
            dedup,
            recap_dao,
        }
    }

    pub(crate) async fn execute_update(&self, job: &JobContext) -> Result<()> {
        tracing::info!(job_id = %job.job_id, "starting morning update pipeline");

        let fetched = self.fetch.fetch(job).await?;
        let preprocessed = self.preprocess.preprocess(job, fetched).await?;
        let deduplicated = self.dedup.deduplicate(job, preprocessed).await?;

        let mut groups = Vec::new();
        for article in deduplicated.articles {
            let group_id = Uuid::new_v4();

            if let Ok(article_id) = Uuid::parse_str(&article.id) {
                // Primary article
                groups.push((group_id, article_id, true));

                // Duplicates
                for dup_id_str in article.duplicates {
                    if let Ok(dup_id) = Uuid::parse_str(&dup_id_str) {
                        groups.push((group_id, dup_id, false));
                    }
                }
            } else {
                tracing::warn!(article_id = %article.id, "skipping non-uuid article id");
            }
        }

        if !groups.is_empty() {
            self.recap_dao.save_morning_article_groups(&groups).await?;
            tracing::info!(
                job_id = %job.job_id,
                groups_count = groups.len(),
                "persisted morning article groups"
            );
        }

        Ok(())
    }
}
