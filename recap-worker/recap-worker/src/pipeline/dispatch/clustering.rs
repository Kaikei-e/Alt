//! Clustering operations for dispatch stage.

use std::collections::HashMap;
use std::sync::Arc;

use anyhow::{Context, Result};
use tokio::{sync::Semaphore, time::timeout};
use tracing::{debug, error, info, warn};
use uuid::Uuid;

use crate::clients::subworker::{ClusteringResponse, SubworkerClient};
use crate::config::Config;
use crate::pipeline::evidence::{EvidenceBundle, EvidenceCorpus};
use crate::scheduler::JobContext;
use crate::store::dao::RecapDao;

/// Clustering operations helper.
pub(crate) struct ClusteringOps<'a> {
    pub(crate) subworker_client: &'a Arc<SubworkerClient>,
    pub(crate) dao: &'a Arc<dyn RecapDao>,
    pub(crate) config: &'a Arc<Config>,
    pub(crate) concurrency_semaphore: &'a Arc<Semaphore>,
}

impl ClusteringOps<'_> {
    /// 単一ジャンルのクラスタリングのみを実行する。
    pub(crate) async fn cluster_genre(
        &self,
        job_id: Uuid,
        genre: &str,
        evidence: &EvidenceCorpus,
    ) -> Result<ClusteringResponse> {
        debug!(
            job_id = %job_id,
            genre = %genre,
            article_count = evidence.articles.len(),
            alt.processing.stage = "dispatch",
            alt.processing.phase = "clustering",
            alt.processing.genre = %genre,
            "clustering genre"
        );

        let genre_timeout = self.config.clustering_genre_timeout();
        let stuck_threshold = Some(self.config.clustering_stuck_threshold());
        let mut clustering_response = timeout(
            genre_timeout,
            self.subworker_client.cluster_corpus_with_timeout(
                job_id,
                evidence,
                genre_timeout,
                stuck_threshold,
            ),
        )
        .await
        .context("clustering timeout")?
        .context("clustering failed")?;

        // Fallback: If clustering succeeded but returned NO clusters (e.g. all noise),
        // we force a fallback response using the evidence corpus.
        if clustering_response.clusters.is_empty() && !evidence.articles.is_empty() {
            warn!(
                job_id = %job_id,
                genre = %genre,
                article_count = evidence.articles.len(),
                "clustering returned no clusters (noise), forcing fallback response"
            );
            clustering_response = SubworkerClient::create_fallback_response(job_id, evidence);
        }

        // Handle fallback response (run_id == 0)
        // If the subworker client returns a fallback response (due to insufficient documents),
        // it will have run_id = 0, which doesn't exist in the database.
        // We need to insert a record for it so that we can persist clusters (foreign key constraint).
        if clustering_response.run_id == 0 {
            info!(
                job_id = %job_id,
                genre = %genre,
                "handling fallback clustering response (run_id=0), creating db record"
            );

            let run = crate::store::models::NewSubworkerRun::new(
                job_id,
                genre,
                serde_json::json!({
                    "fallback": true,
                    "reason": "insufficient_documents",
                    "article_count": evidence.articles.len()
                }),
            )
            .with_status(crate::store::models::SubworkerRunStatus::Succeeded);

            let new_run_id = self
                .dao
                .insert_subworker_run(&run)
                .await
                .context("failed to insert fallback subworker run")?;

            // Update the response with the real DB ID
            clustering_response.run_id = new_run_id;

            // Also mark it as success with the cluster count
            self.dao
                .mark_subworker_run_success(
                    new_run_id,
                    clustering_response.clusters.len() as i32,
                    &serde_json::json!({"fallback": true}),
                )
                .await
                .context("failed to mark fallback run as success")?;
        }

        info!(
            job_id = %job_id,
            genre = %genre,
            cluster_count = clustering_response.clusters.len(),
            alt.processing.stage = "dispatch",
            alt.processing.phase = "clustering",
            alt.processing.genre = %genre,
            alt.processing.status = "completed",
            "clustering completed successfully"
        );

        // システムメトリクス（クラスタリング）を保存
        if let Err(e) = self
            .dao
            .save_system_metrics(job_id, "clustering", &clustering_response.diagnostics)
            .await
        {
            warn!(
                job_id = %job_id,
                genre = %genre,
                error = ?e,
                "failed to save clustering metrics"
            );
        }

        Ok(clustering_response)
    }

    /// Phase 1: 全ジャンルを並列でクラスタリング
    #[allow(clippy::too_many_lines)]
    pub(crate) async fn cluster_all_genres(
        &self,
        job: &JobContext,
        genres: &[String],
        evidence: Arc<EvidenceBundle>,
    ) -> HashMap<String, Result<ClusteringResponse>> {
        let total_genres = genres.len();
        info!(
            job_id = %job.job_id,
            genre_count = total_genres,
            alt.processing.stage = "dispatch",
            alt.processing.phase = "clustering",
            alt.processing.progress.total = total_genres,
            "starting parallel clustering for all genres"
        );

        let mut tasks = Vec::new();

        for genre in genres {
            // Check existence without cloning
            if evidence.get_corpus(genre).is_some() {
                // Capture Arc instead of cloning corpus data
                let evidence_clone = evidence.clone();
                let subworker_client = Arc::clone(self.subworker_client);
                let dao = Arc::clone(self.dao);
                let config = Arc::clone(self.config);
                let job_id = job.job_id;
                let genre_clone = genre.clone();
                let semaphore = Arc::clone(self.concurrency_semaphore);

                let genre_timeout = self.config.clustering_genre_timeout();
                let task = tokio::spawn(async move {
                    // Acquire permission to run (throttling)
                    let _permit = semaphore
                        .clone()
                        .acquire_owned()
                        .await
                        .expect("dispatch semaphore should not be closed");

                    // Lazy access: Get corpus reference only AFTER acquiring semaphore
                    // This ensures we don't load memory until allowed to run
                    let corpus = evidence_clone
                        .get_corpus(&genre_clone)
                        .expect("corpus must exist as checked before spawn");

                    // Create ops for this task
                    let ops = ClusteringOps {
                        subworker_client: &subworker_client,
                        dao: &dao,
                        config: &config,
                        concurrency_semaphore: &semaphore,
                    };

                    // timeoutで包んで、stuckしても他のジャンルに影響しないようにする
                    let result = timeout(
                        genre_timeout,
                        ops.cluster_genre(job_id, &genre_clone, corpus),
                    )
                    .await
                    .unwrap_or_else(|_| {
                        Err(anyhow::anyhow!(
                            "clustering genre {} timed out after {}s",
                            genre_clone,
                            genre_timeout.as_secs()
                        ))
                    });
                    (genre_clone, result)
                });

                tasks.push(task);
            } else {
                warn!(
                    job_id = %job.job_id,
                    genre = %genre,
                    "evidence corpus missing for genre"
                );
            }
        }

        // すべてのクラスタリングタスクを待機
        let results = futures::future::join_all(tasks).await;
        let mut clustering_results: HashMap<String, Result<ClusteringResponse>> = HashMap::new();

        for result in results {
            match result {
                Ok((genre, clustering_result)) => {
                    clustering_results.insert(genre, clustering_result);
                }
                Err(join_error) => match join_error.try_into_panic() {
                    Ok(panic_payload) => {
                        let panic_message = panic_payload
                            .downcast_ref::<&str>()
                            .map(|s| (*s).to_string())
                            .or_else(|| {
                                panic_payload
                                    .downcast_ref::<String>()
                                    .map(std::string::ToString::to_string)
                            })
                            .unwrap_or_else(|| "unknown panic payload".to_string());
                        error!(
                            job_id = %job.job_id,
                            panic_message,
                            "clustering task panicked"
                        );
                    }
                    Err(join_error) => {
                        warn!(
                            job_id = %job.job_id,
                            error = ?join_error,
                            "clustering task failed"
                        );
                    }
                },
            }
        }

        info!(
            job_id = %job.job_id,
            completed_count = clustering_results.len(),
            alt.processing.stage = "dispatch",
            alt.processing.phase = "clustering",
            alt.processing.progress.current = clustering_results.len(),
            alt.processing.progress.total = total_genres,
            alt.processing.status = "completed",
            "completed parallel clustering phase"
        );

        clustering_results
    }
}
