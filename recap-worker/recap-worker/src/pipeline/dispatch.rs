use std::{collections::HashMap, fs, sync::Arc};

use anyhow::{Context, Result};
use async_trait::async_trait;
use tokio::{sync::Semaphore, time::timeout};
use tracing::{debug, error, info, warn};
use uuid::Uuid;

use crate::{
    clients::{
        NewsCreatorClient,
        news_creator::models::{
            BatchSummaryError, BatchSummaryResponse, SummaryOptions, SummaryRequest,
            SummaryResponse,
        },
        subworker::{ClusteringResponse, SubworkerClient},
    },
    config::Config,
    scheduler::JobContext,
    store::dao::RecapDao,
};

use super::evidence::EvidenceBundle;
use serde::{Deserialize, Serialize};

/// ディスパッチ結果。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct DispatchResult {
    pub(crate) job_id: Uuid,
    pub(crate) genre_results: HashMap<String, GenreResult>,
    pub(crate) success_count: usize,
    pub(crate) failure_count: usize,
    /// 設定された全ジャンルリスト（証拠がないジャンルも含む）
    pub(crate) all_genres: Vec<String>,
}

/// ジャンル別の処理結果。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct GenreResult {
    #[allow(dead_code)] // kept for debugging and future use
    pub(crate) genre: String,
    #[allow(dead_code)] // kept for debugging and future use
    pub(crate) clustering_response: Option<ClusteringResponse>,
    pub(crate) summary_response_id: Option<String>,
    pub(crate) summary_response: Option<crate::clients::news_creator::SummaryResponse>,
    pub(crate) error: Option<String>,
}

/// ステージ状態保存用の軽量版ディスパッチ結果。
/// clustering_responseとsummary_responseを除外してサイズを削減。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct DispatchResultLightweight {
    pub(crate) job_id: Uuid,
    pub(crate) genre_results: HashMap<String, GenreResultLightweight>,
    pub(crate) success_count: usize,
    pub(crate) failure_count: usize,
    /// 設定された全ジャンルリスト（証拠がないジャンルも含む）
    pub(crate) all_genres: Vec<String>,
}

/// ステージ状態保存用の軽量版ジャンル結果。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct GenreResultLightweight {
    pub(crate) genre: String,
    /// クラスタリングのrun_id（データベースから再取得可能）
    pub(crate) clustering_run_id: Option<i64>,
    pub(crate) summary_response_id: Option<String>,
    pub(crate) error: Option<String>,
}

impl DispatchResult {
    /// 軽量版に変換（大きなデータを除外）
    pub(crate) fn to_lightweight(&self) -> DispatchResultLightweight {
        let genre_results: HashMap<String, GenreResultLightweight> = self
            .genre_results
            .iter()
            .map(|(genre, result)| {
                let clustering_run_id = result.clustering_response.as_ref().map(|cr| cr.run_id);
                (
                    genre.clone(),
                    GenreResultLightweight {
                        genre: result.genre.clone(),
                        clustering_run_id,
                        summary_response_id: result.summary_response_id.clone(),
                        error: result.error.clone(),
                    },
                )
            })
            .collect();

        DispatchResultLightweight {
            job_id: self.job_id,
            genre_results,
            success_count: self.success_count,
            failure_count: self.failure_count,
            all_genres: self.all_genres.clone(),
        }
    }
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
    dao: Arc<dyn RecapDao>,
    concurrency_semaphore: Arc<Semaphore>,
    config: Arc<Config>,
}

impl MlLlmDispatchStage {
    pub(crate) fn new(
        subworker_client: Arc<SubworkerClient>,
        news_creator_client: Arc<NewsCreatorClient>,
        dao: Arc<dyn RecapDao>,
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

        let genre_timeout = self.config.clustering_genre_timeout();
        let stuck_threshold = Some(self.config.clustering_stuck_threshold());
        let mut clustering_response = timeout(
            genre_timeout,
            self.subworker_client
                .cluster_corpus_with_timeout(job_id, evidence, genre_timeout, stuck_threshold),
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
    /// 後方互換性のため保持（バッチ API ではインライン構築）。
    #[allow(dead_code)]
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
    /// 後方互換性のため保持（バッチ API への移行後は未使用）。
    #[allow(dead_code)]
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

    /// メタデータを取得して SummaryRequest を構築する（HTTP 呼び出しなし）。
    /// バッチ処理用にリクエスト構築のみを行う。
    async fn build_summary_request_for_batch(
        &self,
        job_id: Uuid,
        genre: &str,
        clustering_response: &ClusteringResponse,
    ) -> Result<SummaryRequest> {
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

        // SummaryRequest 構築（CPU集約的なためブロッキングタスクで実行）
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

        // 出力フォーマットオプションを設定
        summary_request.options = Some(SummaryOptions {
            max_bullets: Some(15), // Plan 9: Max 15 bullets for output
            temperature: Some(0.7),
        });

        Ok(summary_request)
    }

    /// 要約メトリクスを保存するヘルパー。
    async fn save_summary_metrics(&self, job_id: Uuid, genre: &str, response: &SummaryResponse) {
        match serde_json::to_value(&response.metadata) {
            Ok(metadata_value) => {
                if let Err(e) = self
                    .dao
                    .save_system_metrics(job_id, "summarization", &metadata_value)
                    .await
                {
                    warn!(
                        job_id = %job_id,
                        genre = %genre,
                        error = ?e,
                        "failed to save summarization metrics"
                    );
                }
            }
            Err(e) => {
                warn!(
                    job_id = %job_id,
                    genre = %genre,
                    error = ?e,
                    "failed to serialize summary metadata for metrics"
                );
            }
        }
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

        // Phase 2: サマリー生成はバッチ API で実行（1回の HTTP 呼び出し）
        let mut genre_results = self
            .generate_summaries_with_batch(job, clustering_results, evidence_arc)
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

                let genre_timeout = self.config.clustering_genre_timeout();
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

                    // timeoutで包んで、stuckしても他のジャンルに影響しないようにする
                    let result = timeout(
                        genre_timeout,
                        self_clone.cluster_genre(job_id, &genre_clone, corpus),
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
            "completed parallel clustering phase"
        );

        clustering_results
    }

    /// Phase 2: サマリー生成を完全直列（キュー方式）で実行
    /// 後方互換性のため保持（バッチ API への移行後は未使用）。
    #[allow(dead_code)]
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

    /// Phase 2: サマリー生成をバッチ API で実行（1回の HTTP 呼び出しで全ジャンル処理）
    async fn generate_summaries_with_batch(
        &self,
        job: &JobContext,
        clustering_results: HashMap<String, Result<ClusteringResponse>>,
        _evidence: Arc<EvidenceBundle>,
    ) -> HashMap<String, GenreResult> {
        info!(
            job_id = %job.job_id,
            genre_count = clustering_results.len(),
            "starting batch summary generation"
        );

        // 1. クラスタリング成功/失敗を分離
        let mut successful: Vec<(String, ClusteringResponse)> = Vec::new();
        let mut genre_results: HashMap<String, GenreResult> = HashMap::new();

        for (genre, result) in clustering_results {
            match result {
                Ok(clustering_response) => {
                    successful.push((genre, clustering_response));
                }
                Err(e) => {
                    warn!(
                        job_id = %job.job_id,
                        genre = %genre,
                        error = ?e,
                        "skipping summary generation due to clustering failure"
                    );
                    genre_results.insert(genre.clone(), Self::clustering_error_result(&genre, e));
                }
            }
        }

        if successful.is_empty() {
            info!(
                job_id = %job.job_id,
                "no successful clustering results, skipping batch summary generation"
            );
            return genre_results;
        }

        // 2. 全リクエストを並列構築
        info!(
            job_id = %job.job_id,
            genre_count = successful.len(),
            "building summary requests for batch"
        );

        let request_futures: Vec<_> = successful
            .iter()
            .map(|(genre, clustering_response)| {
                self.build_summary_request_for_batch(job.job_id, genre, clustering_response)
            })
            .collect();

        let request_results = futures::future::join_all(request_futures).await;

        // 3. 有効なリクエストを収集し、失敗したものはエラーとして記録
        let mut valid_requests: Vec<SummaryRequest> = Vec::new();
        let mut genre_clustering_map: HashMap<String, ClusteringResponse> = HashMap::new();

        for ((genre, clustering_response), req_result) in
            successful.into_iter().zip(request_results)
        {
            match req_result {
                Ok(request) => {
                    genre_clustering_map.insert(genre, clustering_response);
                    valid_requests.push(request);
                }
                Err(e) => {
                    warn!(
                        job_id = %job.job_id,
                        genre = %genre,
                        error = ?e,
                        "failed to build summary request"
                    );
                    genre_results.insert(
                        genre.clone(),
                        GenreResult {
                            genre,
                            clustering_response: Some(clustering_response),
                            summary_response_id: None,
                            summary_response: None,
                            error: Some(format!("Failed to build request: {}", e)),
                        },
                    );
                }
            }
        }

        if valid_requests.is_empty() {
            info!(
                job_id = %job.job_id,
                "no valid summary requests, skipping batch API call"
            );
            return genre_results;
        }

        // 4. バッチ API 呼び出し（50件ごとにチャンク分割）
        const BATCH_SUMMARY_CHUNK_SIZE: usize = 50;

        info!(
            job_id = %job.job_id,
            request_count = valid_requests.len(),
            chunk_count = (valid_requests.len() + BATCH_SUMMARY_CHUNK_SIZE - 1) / BATCH_SUMMARY_CHUNK_SIZE,
            "calling batch summary API in chunks"
        );

        let mut all_responses: Vec<SummaryResponse> = Vec::new();
        let mut all_errors: Vec<BatchSummaryError> = Vec::new();

        for (chunk_idx, chunk) in valid_requests.chunks(BATCH_SUMMARY_CHUNK_SIZE).enumerate() {
            info!(
                job_id = %job.job_id,
                chunk_idx = chunk_idx,
                chunk_size = chunk.len(),
                "processing batch summary chunk"
            );

            let chunk_vec: Vec<SummaryRequest> = chunk.to_vec();
            match self
                .news_creator_client
                .generate_batch_summary(chunk_vec)
                .await
            {
                Ok(response) => {
                    all_responses.extend(response.responses);
                    all_errors.extend(response.errors);
                }
                Err(e) => {
                    // チャンク全体失敗時はエラーメッセージをログに記録
                    // 個別ジャンルは process_batch_response で "Missing from batch response" として処理される
                    error!(
                        job_id = %job.job_id,
                        chunk_idx = chunk_idx,
                        error = ?e,
                        "batch summary chunk failed"
                    );
                }
            }
        }

        let batch_response = BatchSummaryResponse {
            responses: all_responses,
            errors: all_errors,
        };

        // 5. レスポンスをマッピング
        self.process_batch_response(job, Ok(batch_response), genre_clustering_map, &mut genre_results)
            .await;

        info!(
            job_id = %job.job_id,
            completed_count = genre_results.len(),
            "completed batch summary generation phase"
        );

        genre_results
    }

    /// バッチサマリーレスポンスを処理し、ジャンル結果を更新する。
    async fn process_batch_response(
        &self,
        job: &JobContext,
        batch_result: Result<BatchSummaryResponse>,
        mut genre_clustering_map: HashMap<String, ClusteringResponse>,
        genre_results: &mut HashMap<String, GenreResult>,
    ) {
        match batch_result {
            Ok(response) => {
                info!(
                    job_id = %job.job_id,
                    success_count = response.responses.len(),
                    error_count = response.errors.len(),
                    "batch summary API completed"
                );

                // 成功したレスポンスを処理
                for summary_response in response.responses {
                    let genre = summary_response.genre.clone();
                    if let Some(clustering_response) = genre_clustering_map.remove(&genre) {
                        self.save_summary_metrics(job.job_id, &genre, &summary_response)
                            .await;

                        let summary_id =
                            format!("{}-{}", summary_response.job_id, summary_response.genre);
                        genre_results.insert(
                            genre.clone(),
                            GenreResult {
                                genre,
                                clustering_response: Some(clustering_response),
                                summary_response_id: Some(summary_id),
                                summary_response: Some(summary_response),
                                error: None,
                            },
                        );
                    }
                }

                // エラーを処理
                for error in response.errors {
                    let genre = error.genre.clone();
                    if let Some(clustering_response) = genre_clustering_map.remove(&genre) {
                        warn!(
                            job_id = %job.job_id,
                            genre = %genre,
                            error = %error.error,
                            "batch summary failed for genre"
                        );
                        genre_results.insert(
                            genre.clone(),
                            GenreResult {
                                genre,
                                clustering_response: Some(clustering_response),
                                summary_response_id: None,
                                summary_response: None,
                                error: Some(error.error),
                            },
                        );
                    }
                }

                // 残ったジャンル（レスポンスもエラーもない）を処理
                for (genre, clustering_response) in genre_clustering_map {
                    warn!(
                        job_id = %job.job_id,
                        genre = %genre,
                        "genre missing from batch response"
                    );
                    genre_results.insert(
                        genre.clone(),
                        GenreResult {
                            genre,
                            clustering_response: Some(clustering_response),
                            summary_response_id: None,
                            summary_response: None,
                            error: Some("Missing from batch response".to_string()),
                        },
                    );
                }
            }
            Err(e) => {
                // バッチ全体が失敗した場合
                error!(
                    job_id = %job.job_id,
                    error = ?e,
                    "batch summary API failed completely"
                );
                for (genre, clustering_response) in genre_clustering_map {
                    genre_results.insert(
                        genre.clone(),
                        GenreResult {
                            genre,
                            clustering_response: Some(clustering_response),
                            summary_response_id: None,
                            summary_response: None,
                            error: Some(format!("Batch API failed: {}", e)),
                        },
                    );
                }
            }
        }
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
