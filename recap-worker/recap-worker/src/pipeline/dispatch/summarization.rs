//! Summarization operations for dispatch stage.

use std::collections::HashMap;
use std::sync::Arc;

use anyhow::{Context, Result};
use tracing::{debug, error, info, warn};
use uuid::Uuid;

use crate::clients::news_creator::models::{
    BatchSummaryError, BatchSummaryResponse, SummaryOptions, SummaryRequest, SummaryResponse,
};
use crate::clients::subworker::ClusteringResponse;
use crate::clients::NewsCreatorClient;
use crate::config::Config;
use crate::scheduler::JobContext;
use crate::store::dao::RecapDao;

use super::types::GenreResult;

/// Summarization operations helper.
pub(crate) struct SummarizationOps<'a> {
    pub(crate) news_creator_client: &'a Arc<NewsCreatorClient>,
    pub(crate) dao: &'a Arc<dyn RecapDao>,
    pub(crate) config: &'a Arc<Config>,
}

impl SummarizationOps<'_> {
    /// クラスタリングエラー時の結果を構築する。
    pub(crate) fn clustering_error_result(genre: &str, e: anyhow::Error) -> GenreResult {
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
    pub(crate) fn build_genre_result(
        genre: &str,
        clustering_response: ClusteringResponse,
        summary_result: Result<SummaryResponse>,
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
    pub(crate) async fn generate_summary_with_metadata(
        &self,
        job_id: Uuid,
        genre: &str,
        clustering_response: &ClusteringResponse,
    ) -> Result<SummaryResponse> {
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
        summary_request.options = Some(SummaryOptions {
            max_bullets: Some(15), // Plan 9: Max 15 bullets for output
            temperature: Some(0.7),
        });

        self.news_creator_client
            .generate_summary(&summary_request)
            .await
    }

    /// メタデータを取得して SummaryRequest を構築する（HTTP 呼び出しなし）。
    /// バッチ処理用にリクエスト構築のみを行う。
    pub(crate) async fn build_summary_request_for_batch(
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
    pub(crate) async fn save_summary_metrics(
        &self,
        job_id: Uuid,
        genre: &str,
        response: &SummaryResponse,
    ) {
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

    /// Phase 2: サマリー生成を完全直列（キュー方式）で実行
    /// 後方互換性のため保持（バッチ API への移行後は未使用）。
    #[allow(dead_code)]
    pub(crate) async fn generate_summaries_sequentially(
        &self,
        job: &JobContext,
        clustering_results: HashMap<String, Result<ClusteringResponse>>,
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
                    genre_results
                        .insert(genre, Self::clustering_error_result(&genre_clone, e));
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
    #[allow(clippy::too_many_lines)]
    pub(crate) async fn generate_summaries_with_batch(
        &self,
        job: &JobContext,
        clustering_results: HashMap<String, Result<ClusteringResponse>>,
    ) -> HashMap<String, GenreResult> {
        let total_genres = clustering_results.len();
        info!(
            job_id = %job.job_id,
            genre_count = total_genres,
            alt.processing.stage = "dispatch",
            alt.processing.phase = "summarization",
            alt.processing.progress.total = total_genres,
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
                Ok(mut request) => {
                    // Filter out clusters with empty representative_sentences
                    // This prevents 422 errors from the batch summary API
                    let original_cluster_count = request.clusters.len();
                    request
                        .clusters
                        .retain(|cluster| !cluster.representative_sentences.is_empty());

                    let filtered_count = original_cluster_count - request.clusters.len();
                    if filtered_count > 0 {
                        debug!(
                            job_id = %job.job_id,
                            genre = %genre,
                            filtered_count = filtered_count,
                            remaining_count = request.clusters.len(),
                            "filtered out clusters with empty representative_sentences"
                        );
                    }

                    // If all clusters were filtered out, treat as error
                    if request.clusters.is_empty() {
                        warn!(
                            job_id = %job.job_id,
                            genre = %genre,
                            original_cluster_count = original_cluster_count,
                            "all clusters had empty representative_sentences"
                        );
                        genre_results.insert(
                            genre.clone(),
                            GenreResult {
                                genre,
                                clustering_response: Some(clustering_response),
                                summary_response_id: None,
                                summary_response: None,
                                error: Some(
                                    "All clusters had empty representative_sentences".to_string(),
                                ),
                            },
                        );
                        continue;
                    }

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

        // 4. バッチ API 呼び出し（設定可能なチャンクサイズで分割）
        let batch_summary_chunk_size = self.config.batch_summary_chunk_size();

        info!(
            job_id = %job.job_id,
            request_count = valid_requests.len(),
            chunk_count = valid_requests.len().div_ceil(batch_summary_chunk_size),
            "calling batch summary API in chunks"
        );

        let mut all_responses: Vec<SummaryResponse> = Vec::new();
        let mut all_errors: Vec<BatchSummaryError> = Vec::new();

        for (chunk_idx, chunk) in valid_requests.chunks(batch_summary_chunk_size).enumerate() {
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
            alt.processing.stage = "dispatch",
            alt.processing.phase = "summarization",
            alt.processing.progress.current = genre_results.len(),
            alt.processing.progress.total = total_genres,
            alt.processing.status = "completed",
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
