use std::{collections::HashMap, fs, sync::Arc};

use anyhow::{Context, Result};
use async_trait::async_trait;
use tokio::sync::Semaphore;
use tracing::{debug, error, info, warn};
use uuid::Uuid;

use crate::{
    clients::{
        NewsCreatorClient,
        subworker::{ClusteringResponse, SubworkerClient},
    },
    config::Config,
    scheduler::JobContext,
    store::dao::RecapDao,
};

use super::evidence::EvidenceBundle;

/// ディスパッチ結果。
#[derive(Debug, Clone)]
pub(crate) struct DispatchResult {
    pub(crate) job_id: Uuid,
    pub(crate) genre_results: HashMap<String, GenreResult>,
    pub(crate) success_count: usize,
    pub(crate) failure_count: usize,
    /// 設定された全ジャンルリスト（証拠がないジャンルも含む）
    pub(crate) all_genres: Vec<String>,
}

/// ジャンル別の処理結果。
#[derive(Debug, Clone)]
pub(crate) struct GenreResult {
    #[allow(dead_code)] // kept for debugging and future use
    pub(crate) genre: String,
    #[allow(dead_code)] // kept for debugging and future use
    pub(crate) clustering_response: Option<ClusteringResponse>,
    pub(crate) summary_response_id: Option<String>,
    pub(crate) summary_response: Option<crate::clients::news_creator::SummaryResponse>,
    pub(crate) error: Option<String>,
}

#[async_trait]
pub(crate) trait DispatchStage: Send + Sync {
    async fn dispatch(&self, job: &JobContext, evidence: EvidenceBundle) -> Result<DispatchResult>;
}

/// SubworkerとNews-Creatorを連携させるディスパッチステージ。
///
/// 各ジャンルごとに：
/// 1. SubworkerでML処理（クラスタリング）
/// 2. News-CreatorでLLM処理（日本語要約生成）
#[derive(Clone)]
pub(crate) struct MlLlmDispatchStage {
    subworker_client: Arc<SubworkerClient>,
    news_creator_client: Arc<NewsCreatorClient>,
    dao: Arc<RecapDao>,
    concurrency_semaphore: Arc<Semaphore>,
    config: Arc<Config>,
}

impl MlLlmDispatchStage {
    pub(crate) fn new(
        subworker_client: Arc<SubworkerClient>,
        news_creator_client: Arc<NewsCreatorClient>,
        dao: Arc<RecapDao>,
        max_concurrency: usize,
        config: Arc<Config>,
    ) -> Self {
        Self {
            subworker_client,
            news_creator_client,
            dao,
            concurrency_semaphore: Arc::new(Semaphore::new(max_concurrency.max(1))),
            config,
        }
    }

    /// 単一ジャンルのクラスタリングのみを実行する。
    async fn cluster_genre(
        &self,
        job_id: Uuid,
        genre: &str,
        evidence: &super::evidence::EvidenceCorpus,
    ) -> Result<ClusteringResponse> {
        debug!(
            job_id = %job_id,
            genre = %genre,
            article_count = evidence.articles.len(),
            "clustering genre"
        );

        let mut clustering_response = self
            .subworker_client
            .cluster_corpus(job_id, evidence)
            .await
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

    /// クラスタリングエラー時の結果を構築する。
    fn clustering_error_result(genre: &str, e: anyhow::Error) -> GenreResult {
        warn!(
            genre = %genre,
            error = ?e,
            "clustering failed"
        );
        GenreResult {
            genre: genre.to_string(),
            clustering_response: None,
            summary_response_id: None,
            summary_response: None,
            error: Some(format!("Clustering failed: {}", e)),
        }
    }

    /// 要約生成結果からGenreResultを構築する。
    fn build_genre_result(
        genre: &str,
        clustering_response: ClusteringResponse,
        summary_result: Result<crate::clients::news_creator::SummaryResponse>,
    ) -> GenreResult {
        match summary_result {
            Ok(summary_response) => {
                info!(
                    genre = %genre,
                    bullet_count = summary_response.summary.bullets.len(),
                    "summary generation completed successfully"
                );
                let summary_id = format!("{}-{}", summary_response.job_id, summary_response.genre);
                GenreResult {
                    genre: genre.to_string(),
                    clustering_response: Some(clustering_response),
                    summary_response_id: Some(summary_id),
                    summary_response: Some(summary_response),
                    error: None,
                }
            }
            Err(e) => {
                warn!(
                    genre = %genre,
                    error = ?e,
                    "summary generation failed"
                );
                GenreResult {
                    genre: genre.to_string(),
                    clustering_response: Some(clustering_response),
                    summary_response_id: None,
                    summary_response: None,
                    error: Some(format!("Summary generation failed: {}", e)),
                }
            }
        }
    }

    /// メタデータを取得して要約リクエストを構築し、要約を生成する。
    async fn generate_summary_with_metadata(
        &self,
        job_id: Uuid,
        genre: &str,
        clustering_response: &ClusteringResponse,
    ) -> Result<crate::clients::news_creator::SummaryResponse> {
        // 記事IDのリストを収集
        let article_ids: Vec<String> = clustering_response
            .clusters
            .iter()
            .flat_map(|cluster| {
                cluster
                    .representatives
                    .iter()
                    .map(|rep| rep.article_id.clone())
            })
            .collect::<std::collections::HashSet<_>>()
            .into_iter()
            .collect();

        // メタデータを取得
        let article_metadata = match self.dao.get_article_metadata(job_id, &article_ids).await {
            Ok(metadata) => metadata,
            Err(e) => {
                warn!(
                    job_id = %job_id,
                    genre = %genre,
                    error = ?e,
                    "failed to fetch article metadata, proceeding without metadata"
                );
                std::collections::HashMap::new()
            }
        };

        let news_creator_client = self.news_creator_client.clone();
        let clustering_response_clone = clustering_response.clone();

        // Step 1: Build Summary Request (Budget Allocation & Sentence Selection)
        // This runs on a blocking thread because token counting is CPU-bound
        let mut summary_request = tokio::task::spawn_blocking(move || {
            news_creator_client.build_summary_request(
                job_id,
                &clustering_response_clone,
                8, // Plan 9: "5-8 sentences" for actual summary input
                &article_metadata,
            )
        })
        .await
        .context("failed to join build_summary_request task")?;

        // Step 2: Direct Summarization (Single Shot)
        // Skip Map phase (intermediate summarization) and directly generate final summary
        // from the selected sentences.
        info!(
            job_id = %job_id,
            genre = %genre,
            cluster_count = summary_request.clusters.len(),
            "starting single-shot summarization (skipping Map phase)"
        );

        // Enforce strict output format options
        summary_request.options = Some(crate::clients::news_creator::models::SummaryOptions {
            max_bullets: Some(15), // Plan 9: Max 15 bullets for output
            temperature: Some(0.7),
        });

        self.news_creator_client
            .generate_summary(&summary_request)
            .await
    }
}

#[async_trait]
impl DispatchStage for MlLlmDispatchStage {
    async fn dispatch(&self, job: &JobContext, evidence: EvidenceBundle) -> Result<DispatchResult> {
        let genres = evidence.genres();
        let genre_count = genres.len();

        info!(
            job_id = %job.job_id,
            genre_count = genre_count,
            "starting ML/LLM dispatch for all genres"
        );

        // 設定された全ジャンルを取得
        let all_genres = self.config.recap_genres().to_vec();

        if genre_count == 0 {
            return Ok(DispatchResult {
                job_id: job.job_id,
                genre_results: HashMap::new(),
                success_count: 0,
                failure_count: 0,
                all_genres,
            });
        }

        // Wrap evidence in Arc once to share across tasks without deep cloning
        let evidence_arc = Arc::new(evidence);

        // Phase 1: 全ジャンルを並列でクラスタリング
        let clustering_results = self
            .cluster_all_genres(job, &genres, evidence_arc.clone())
            .await;

        // Phase 2: サマリー生成は完全直列（キュー方式）
        let mut genre_results = self
            .generate_summaries_sequentially(job, clustering_results, evidence_arc)
            .await;

        // 証拠がないジャンル（設定されているが処理されていない）を追加
        let processed_genres: std::collections::HashSet<String> =
            genre_results.keys().cloned().collect();
        for genre in &all_genres {
            if !processed_genres.contains(genre) {
                // 証拠がないジャンルを結果に追加（エラーとして記録）
                genre_results.insert(
                    genre.clone(),
                    GenreResult {
                        genre: genre.clone(),
                        clustering_response: None,
                        summary_response_id: None,
                        summary_response: None,
                        error: Some("no evidence (no articles assigned)".to_string()),
                    },
                );
            }
        }

        let success_count = genre_results
            .values()
            .filter(|result| result.error.is_none())
            .count();
        let failure_count = genre_results.len() - success_count;

        let dispatch_result = DispatchResult {
            job_id: job.job_id,
            genre_results,
            success_count,
            failure_count,
            all_genres,
        };

        if let Some((rss_kb, peak_kb)) = read_process_memory_kb() {
            info!(
                job_id = %dispatch_result.job_id,
                success_count = dispatch_result.success_count,
                failure_count = dispatch_result.failure_count,
                genre_count = dispatch_result.genre_results.len(),
                memory_rss_kb = rss_kb,
                memory_peak_kb = peak_kb,
                "completed ML/LLM dispatch"
            );
        } else {
            info!(
                job_id = %dispatch_result.job_id,
                success_count = dispatch_result.success_count,
                failure_count = dispatch_result.failure_count,
                genre_count = dispatch_result.genre_results.len(),
                "completed ML/LLM dispatch"
            );
        }

        Ok(dispatch_result)
    }
}

impl MlLlmDispatchStage {
    /// Phase 1: 全ジャンルを並列でクラスタリング
    async fn cluster_all_genres(
        &self,
        job: &JobContext,
        genres: &[String],
        evidence: Arc<EvidenceBundle>,
    ) -> HashMap<String, Result<ClusteringResponse>> {
        info!(
            job_id = %job.job_id,
            genre_count = genres.len(),
            "starting parallel clustering for all genres"
        );

        let mut tasks = Vec::new();

        for genre in genres {
            // Check existence without cloning
            if evidence.get_corpus(genre).is_some() {
                // Capture Arc instead of cloning corpus data
                let evidence_clone = evidence.clone();
                let self_clone = self.clone();
                let job_id = job.job_id;
                let genre_clone = genre.clone();
                let semaphore = Arc::clone(&self.concurrency_semaphore);

                let task = tokio::spawn(async move {
                    // Acquire permission to run (throttling)
                    let _permit = semaphore
                        .acquire_owned()
                        .await
                        .expect("dispatch semaphore should not be closed");

                    // Lazy access: Get corpus reference only AFTER acquiring semaphore
                    // This ensures we don't load memory until allowed to run
                    let corpus = evidence_clone
                        .get_corpus(&genre_clone)
                        .expect("corpus must exist as checked before spawn");

                    let result = self_clone.cluster_genre(job_id, &genre_clone, corpus).await;
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
            "completed parallel clustering phase"
        );

        clustering_results
    }

    /// Phase 2: サマリー生成を完全直列（キュー方式）で実行
    async fn generate_summaries_sequentially(
        &self,
        job: &JobContext,
        clustering_results: HashMap<String, Result<ClusteringResponse>>,
        _evidence: Arc<EvidenceBundle>,
    ) -> HashMap<String, GenreResult> {
        info!(
            job_id = %job.job_id,
            genre_count = clustering_results.len(),
            "starting sequential summary generation (queue mode)"
        );

        let mut genre_results: HashMap<String, GenreResult> = HashMap::new();

        // クラスタリング成功したジャンルを順番に処理（完全直列）
        for (genre, clustering_result) in clustering_results {
            match clustering_result {
                Ok(clustering_response) => {
                    info!(
                        job_id = %job.job_id,
                        genre = %genre,
                        "processing summary generation (queue position)"
                    );

                    let summary_result = self
                        .generate_summary_with_metadata(job.job_id, &genre, &clustering_response)
                        .await;

                    // システムメトリクス（要約）を保存
                    if let Ok(ref response) = summary_result {
                        match serde_json::to_value(&response.metadata) {
                            Ok(metadata_value) => {
                                if let Err(e) = self
                                    .dao
                                    .save_system_metrics(
                                        job.job_id,
                                        "summarization",
                                        &metadata_value,
                                    )
                                    .await
                                {
                                    warn!(
                                        job_id = %job.job_id,
                                        genre = %genre,
                                        error = ?e,
                                        "failed to save summarization metrics"
                                    );
                                }
                            }
                            Err(e) => {
                                warn!(
                                    job_id = %job.job_id,
                                    genre = %genre,
                                    error = ?e,
                                    "failed to serialize summary metadata for metrics"
                                );
                            }
                        }
                    }

                    let genre_result =
                        Self::build_genre_result(&genre, clustering_response, summary_result);
                    genre_results.insert(genre, genre_result);
                }
                Err(e) => {
                    warn!(
                        job_id = %job.job_id,
                        genre = %genre,
                        error = ?e,
                        "skipping summary generation due to clustering failure"
                    );
                    let genre_clone = genre.clone();
                    genre_results.insert(genre, Self::clustering_error_result(&genre_clone, e));
                }
            }
        }

        info!(
            job_id = %job.job_id,
            completed_count = genre_results.len(),
            "completed sequential summary generation phase"
        );

        genre_results
    }
}

fn read_process_memory_kb() -> Option<(u64, u64)> {
    let status = fs::read_to_string("/proc/self/status").ok()?;
    let mut rss_kb: Option<u64> = None;
    let mut peak_kb: Option<u64> = None;

    for line in status.lines() {
        if let Some(value) = line.strip_prefix("VmRSS:") {
            rss_kb = value
                .split_whitespace()
                .next()
                .and_then(|raw| raw.parse::<u64>().ok());
        } else if let Some(value) = line.strip_prefix("VmHWM:") {
            peak_kb = value
                .split_whitespace()
                .next()
                .and_then(|raw| raw.parse::<u64>().ok());
        }
    }

    match (rss_kb, peak_kb) {
        (Some(rss), Some(peak)) => Some((rss, peak)),
        (Some(rss), None) => Some((rss, rss)),
        (None, Some(peak)) => Some((peak, peak)),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn genre_result_tracks_success_and_failure() {
        let success = GenreResult {
            genre: "ai".to_string(),
            clustering_response: None,
            summary_response_id: None,
            summary_response: None,
            error: None,
        };

        assert!(success.error.is_none());

        let failure = GenreResult {
            genre: "tech".to_string(),
            clustering_response: None,
            summary_response_id: None,
            summary_response: None,
            error: Some("Failed".to_string()),
        };

        assert!(failure.error.is_some());
    }
}
