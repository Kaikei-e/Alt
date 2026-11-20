use reqwest::Url;
use std::time::{Duration, Instant};
use tokio::time::sleep;
use tracing::{debug, warn};
use uuid::Uuid;

use super::types::{
    ClusterDocument, ClusterInfo, ClusterJobParams, ClusterJobRequest, ClusterJobStatus,
    ClusterRepresentative, ClusteringResponse, DEFAULT_HDBSCAN_MIN_CLUSTER_SIZE,
    DEFAULT_MAX_SENTENCES_TOTAL, DEFAULT_MMR_LAMBDA, DEFAULT_UMAP_N_COMPONENTS,
    INITIAL_POLL_INTERVAL_MS, MAX_POLL_ATTEMPTS, MAX_POLL_INTERVAL_MS, MIN_FALLBACK_DOCUMENTS,
    MIN_PARAGRAPH_LEN, POLL_REQUEST_RETRIES, POLL_REQUEST_RETRY_DELAY_MS,
    POLL_REQUEST_TIMEOUT_SECS,
};
use super::utils::{summarize_validation_errors, truncate_error_message};
use crate::clients::subworker::SubworkerClient;
use crate::error::{RecapError, Result};
use crate::pipeline::evidence::EvidenceCorpus;
use crate::schema::{subworker::CLUSTERING_RESPONSE_SCHEMA, validate_json};
use crate::util::retry::{RetryConfig, is_retryable_error};
use serde_json::Value;
use xxhash_rust::xxh3::xxh3_64;

impl SubworkerClient {
    /// フォールバック用の単一クラスタレスポンスを生成する。
    pub(crate) fn create_fallback_response(
        job_id: Uuid,
        corpus: &EvidenceCorpus,
    ) -> ClusteringResponse {
        let mut representatives = Vec::new();
        for article in &corpus.articles {
            if let Some(first_sentence) = article.sentences.first() {
                representatives.push(ClusterRepresentative {
                    article_id: article.article_id.clone(),
                    paragraph_idx: Some(0),
                    text: first_sentence.clone(),
                    lang: Some(article.language.clone()),
                    score: Some(1.0),
                    reasons: vec!["fallback".to_string()],
                });
            }
        }

        let cluster = ClusterInfo {
            cluster_id: 0,
            size: corpus.articles.len(),
            label: Some(corpus.genre.clone()),
            top_terms: Vec::new(),
            stats: serde_json::json!({}),
            representatives,
        };

        ClusteringResponse {
            run_id: 0,
            job_id,
            genre: corpus.genre.clone(),
            status: ClusterJobStatus::Succeeded,
            cluster_count: 1,
            clusters: vec![cluster],
            genre_highlights: None,
            diagnostics: serde_json::json!({
                "fallback": true,
                "reason": "insufficient_documents_for_clustering"
            }),
        }
    }

    /// 証拠コーパスを送信してクラスタリング結果を取得する。
    ///
    /// timeoutを指定する場合は`cluster_corpus_with_timeout`を使用してください。
    #[allow(dead_code)] // 他の場所で使用される可能性がある
    pub(crate) async fn cluster_corpus(
        &self,
        job_id: Uuid,
        corpus: &EvidenceCorpus,
    ) -> Result<ClusteringResponse> {
        // デフォルトのtimeoutを使用
        let default_timeout = Duration::from_mins(5);
        self.cluster_corpus_with_timeout(job_id, corpus, default_timeout, None)
            .await
    }

    /// 証拠コーパスを送信してクラスタリング結果を取得する（timeout指定付き）。
    ///
    /// # Arguments
    /// * `job_id` - ジョブID
    /// * `corpus` - 証拠コーパス
    /// * `genre_timeout` - 全体のタイムアウト
    /// * `stuck_threshold` - 進捗なしでstuckと判定するまでの時間（None の場合は600秒）
    pub(crate) async fn cluster_corpus_with_timeout(
        &self,
        job_id: Uuid,
        corpus: &EvidenceCorpus,
        genre_timeout: Duration,
        stuck_threshold: Option<Duration>,
    ) -> Result<ClusteringResponse> {
        let runs_url = build_runs_url(&self.base_url)?;

        debug!(
            job_id = %job_id,
            genre = %corpus.genre,
            article_count = corpus.articles.len(),
            "sending evidence corpus to subworker"
        );

        let request_payload = build_cluster_job_request(corpus);
        // 決定的なIdempotency-Keyを生成（corpusのハッシュを使用）
        let corpus_digest = compute_corpus_digest(&request_payload);
        let idempotency_key = format!("{}::{}::{}", job_id, corpus.genre, corpus_digest);
        let document_count = request_payload.documents.len();

        if let Some(fallback) =
            self.check_min_documents_or_fallback(job_id, corpus, document_count)?
        {
            return Ok(fallback);
        }

        let response = self
            .post_clustering_request(
                &runs_url,
                job_id,
                &corpus.genre,
                &request_payload,
                &idempotency_key,
            )
            .await?;

        let mut run =
            parse_and_validate_clustering_response(job_id, &corpus.genre, response).await?;

        if run.status.is_running() {
            run = self
                .poll_run_with_timeout(run.run_id, genre_timeout, stuck_threshold)
                .await?;
        }

        if !run.is_success() {
            return Err(RecapError::Clustering(format!(
                "clustering run {} finished with status {}",
                run.run_id, run.status
            )));
        }

        Ok(run)
    }

    fn check_min_documents_or_fallback(
        &self,
        job_id: Uuid,
        corpus: &EvidenceCorpus,
        document_count: usize,
    ) -> Result<Option<ClusteringResponse>> {
        if document_count >= self.min_documents_per_genre {
            return Ok(None);
        }
        if document_count >= MIN_FALLBACK_DOCUMENTS {
            warn!(
                job_id = %job_id,
                genre = %corpus.genre,
                document_count,
                min_required = self.min_documents_per_genre,
                "using fallback single-cluster response due to insufficient documents"
            );
            return Ok(Some(Self::create_fallback_response(job_id, corpus)));
        }
        warn!(
            job_id = %job_id,
            genre = %corpus.genre,
            document_count,
            min_fallback = MIN_FALLBACK_DOCUMENTS,
            "skipping clustering because document count is below minimum fallback threshold"
        );
        Err(RecapError::InsufficientDocuments {
            min: MIN_FALLBACK_DOCUMENTS,
            found: document_count,
        })
    }

    async fn post_clustering_request(
        &self,
        runs_url: &Url,
        job_id: Uuid,
        genre: &str,
        request_payload: &ClusterJobRequest<'_>,
        idempotency_key: &str,
    ) -> Result<reqwest::Response> {
        let response = self
            .client
            .post(runs_url.clone())
            .json(request_payload)
            .header("X-Alt-Job-Id", job_id.to_string())
            .header("X-Alt-Genre", genre)
            .header("Idempotency-Key", idempotency_key)
            .send()
            .await
            .map_err(|e| RecapError::Clustering(format!("clustering request failed: {e}")))?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            let truncated_body = truncate_error_message(&body);
            return Err(RecapError::Clustering(format!(
                "clustering endpoint returned error status {status}: {truncated_body}"
            )));
        }

        Ok(response)
    }

    /// 経過時間ベースのdeadline付きでpollingを実行する。
    async fn poll_run_with_timeout(
        &self,
        run_id: i64,
        deadline: Duration,
        stuck_threshold: Option<Duration>,
    ) -> Result<ClusteringResponse> {
        let start_time = Instant::now();
        let run_url = build_run_url(&self.base_url, run_id)?;

        // ジッター付きバックオフ設定
        let backoff_config = RetryConfig {
            max_attempts: MAX_POLL_ATTEMPTS,
            base_delay_ms: INITIAL_POLL_INTERVAL_MS,
            max_delay_ms: MAX_POLL_INTERVAL_MS,
        };

        let mut attempt = 0;
        let mut progress_tracker = ProgressTracker::new(start_time);

        loop {
            // deadline/stuck検知
            self.check_polling_constraints(
                run_id,
                start_time,
                deadline,
                &progress_tracker,
                stuck_threshold,
            )?;

            // 1回のpoll実行（リクエスト送信、検証、デシリアライズ）
            let run = self
                .poll_run_once(run_id, &run_url, attempt, start_time, deadline)
                .await?;

            // 進捗追跡の更新
            progress_tracker.update_if_progressed(&run);

            // 終了判定
            if run.status.is_terminal() {
                if !run.is_success() {
                    warn!(
                        run_id,
                        status = %run.status,
                        elapsed_secs = start_time.elapsed().as_secs(),
                        "clustering run completed with non-success status"
                    );
                }
                return Ok(run);
            }

            debug!(
                run_id,
                attempt,
                status = %run.status,
                elapsed_secs = start_time.elapsed().as_secs(),
                deadline_secs = deadline.as_secs(),
                "clustering run still in progress"
            );

            // ジッター付きバックオフで待機
            let backoff = backoff_config.delay_for_attempt(attempt);
            sleep(backoff).await;

            attempt += 1;
            if attempt >= MAX_POLL_ATTEMPTS {
                return Err(RecapError::Clustering(format!(
                    "clustering run {run_id} did not complete within {MAX_POLL_ATTEMPTS} attempts"
                )));
            }
        }
    }

    /// deadlineとstuck検知をチェックする。
    #[allow(clippy::unused_self)] // 将来の拡張性のため（ログ等でselfを使う可能性）
    fn check_polling_constraints(
        &self,
        run_id: i64,
        start_time: Instant,
        deadline: Duration,
        progress_tracker: &ProgressTracker,
        stuck_threshold: Option<Duration>,
    ) -> Result<()> {
        // 経過時間ベースのdeadlineチェック
        let elapsed = start_time.elapsed();
        if elapsed >= deadline {
            return Err(RecapError::Clustering(format!(
                "clustering run {} did not complete within timeout ({}s elapsed, deadline: {}s)",
                run_id,
                elapsed.as_secs(),
                deadline.as_secs()
            )));
        }

        // Stuck検知: 進捗が無い場合（statusやcluster_countが変化しない）
        // デフォルトは600秒（10分）
        let stuck_limit = stuck_threshold.unwrap_or_else(|| Duration::from_mins(10));
        let time_since_progress = progress_tracker.time_since_progress();
        if time_since_progress > stuck_limit {
            warn!(
                run_id,
                elapsed_secs = elapsed.as_secs(),
                time_since_progress_secs = time_since_progress.as_secs(),
                stuck_threshold_secs = stuck_limit.as_secs(),
                "clustering run appears stuck (no progress for {}s, threshold: {}s)",
                time_since_progress.as_secs(),
                stuck_limit.as_secs()
            );
            return Err(RecapError::Clustering(format!(
                "clustering run {} appears stuck (no progress for {}s, threshold: {}s)",
                run_id,
                time_since_progress.as_secs(),
                stuck_limit.as_secs()
            )));
        }

        Ok(())
    }

    /// 1回のpollリクエストを実行し、検証・デシリアライズまで行う。
    async fn poll_run_once(
        &self,
        run_id: i64,
        run_url: &Url,
        attempt: usize,
        start_time: Instant,
        deadline: Duration,
    ) -> Result<ClusteringResponse> {
        // pollリクエストをリトライ付きで実行
        let response = match self
            .send_clustering_poll_request_with_retry(run_id, run_url, attempt)
            .await
        {
            Ok(resp) => resp,
            Err(e) => {
                // リトライ可能なエラーでも、deadlineを超えそうなら即失敗
                if start_time.elapsed() + Duration::from_secs(10) >= deadline {
                    return Err(e.context(format!(
                        "clustering run {run_id} polling failed near deadline"
                    )));
                }
                return Err(e);
            }
        };

        // HTTPステータスチェック
        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            let truncated_body = truncate_error_message(&body);
            return Err(RecapError::Clustering(format!(
                "run polling endpoint returned error status {status}: {truncated_body}"
            )));
        }

        // JSONデシリアライズ
        let response_json: Value = response.json().await.map_err(|e| {
            RecapError::Clustering(format!(
                "failed to deserialize polling response as JSON: {e}"
            ))
        })?;

        // JSON Schema検証
        let validation = validate_json(&CLUSTERING_RESPONSE_SCHEMA, &response_json);
        if !validation.valid {
            let summarized_errors = summarize_validation_errors(&validation.errors);
            warn!(
                run_id,
                error_count = validation.errors.len(),
                first_error = %summarized_errors.first().map_or("unknown", String::as_str),
                "polling response failed JSON Schema validation"
            );
            return Err(RecapError::Clustering(format!(
                "run polling response validation failed: {} errors (first: {})",
                validation.errors.len(),
                summarized_errors.first().map_or("unknown", String::as_str)
            )));
        }

        // 検証済みJSONからClusteringResponseへデシリアライズ
        serde_json::from_value(response_json).map_err(|e| {
            RecapError::Clustering(format!(
                "failed to deserialize validated polling response: {e}"
            ))
        })
    }

    /// pollリクエストをリトライ付きで送信する（classification側と同様）。
    async fn send_clustering_poll_request_with_retry(
        &self,
        run_id: i64,
        url: &Url,
        poll_attempt: usize,
    ) -> Result<reqwest::Response> {
        let mut last_error = None;

        for retry in 0..POLL_REQUEST_RETRIES {
            match self
                .client
                .get(url.clone())
                .timeout(Duration::from_secs(POLL_REQUEST_TIMEOUT_SECS))
                .send()
                .await
            {
                Ok(response) => return Ok(response),
                Err(e) => {
                    if is_retryable_error(&e) {
                        warn!(
                            run_id,
                            poll_attempt = poll_attempt + 1,
                            retry = retry + 1,
                            max_retries = POLL_REQUEST_RETRIES,
                            error = %e,
                            "polling request failed, retrying"
                        );
                        sleep(Duration::from_millis(POLL_REQUEST_RETRY_DELAY_MS)).await;
                        last_error = Some(e);
                        continue;
                    }
                    return Err(RecapError::Clustering(format!(
                        "clustering run polling request failed for run_id {} (attempt {}): {}",
                        run_id,
                        poll_attempt + 1,
                        e
                    )));
                }
            }
        }

        match last_error {
            Some(e) => Err(RecapError::Clustering(format!(
                "clustering run polling request failed after {} retries for run_id {} (attempt {}): {}",
                POLL_REQUEST_RETRIES,
                run_id,
                poll_attempt + 1,
                e
            ))),
            None => Err(RecapError::Clustering(format!(
                "clustering run polling request failed after {} retries for run_id {} (attempt {})",
                POLL_REQUEST_RETRIES,
                run_id,
                poll_attempt + 1
            ))),
        }
    }
}

async fn parse_and_validate_clustering_response(
    job_id: Uuid,
    genre: &str,
    response: reqwest::Response,
) -> Result<ClusteringResponse> {
    let response_json: Value = response.json().await.map_err(|e| {
        RecapError::Clustering(format!(
            "failed to deserialize clustering response as JSON: {e}"
        ))
    })?;

    validate_clustering_response_json(job_id, genre, &response_json)?;

    serde_json::from_value(response_json).map_err(|e| {
        RecapError::Clustering(format!(
            "failed to deserialize validated clustering response: {e}"
        ))
    })
}

fn validate_clustering_response_json(
    job_id: Uuid,
    genre: &str,
    response_json: &Value,
) -> Result<()> {
    let validation = validate_json(&CLUSTERING_RESPONSE_SCHEMA, response_json);
    if !validation.valid {
        let summarized_errors = summarize_validation_errors(&validation.errors);
        warn!(
            job_id = %job_id,
            genre = %genre,
            error_count = validation.errors.len(),
            first_error = %summarized_errors.first().map_or("unknown", String::as_str),
            "clustering response failed JSON Schema validation"
        );
        return Err(RecapError::Clustering(format!(
            "clustering response validation failed: {} errors (first: {})",
            validation.errors.len(),
            summarized_errors.first().map_or("unknown", String::as_str)
        )));
    }

    debug!(
        job_id = %job_id,
        genre = %genre,
        "clustering response passed JSON Schema validation"
    );

    Ok(())
}

fn build_runs_url(base: &Url) -> Result<Url> {
    let mut url = base.clone();
    url.path_segments_mut()
        .map_err(|()| RecapError::Clustering("subworker base URL must be absolute".to_string()))?
        .extend(["v1", "runs"]);
    Ok(url)
}

fn build_run_url(base: &Url, run_id: i64) -> Result<Url> {
    let mut url = base.clone();
    url.path_segments_mut()
        .map_err(|()| RecapError::Clustering("subworker base URL must be absolute".to_string()))?
        .extend(["v1", "runs", &run_id.to_string()]);
    Ok(url)
}

fn build_cluster_job_request(corpus: &EvidenceCorpus) -> ClusterJobRequest<'_> {
    let max_sentences_total = corpus
        .total_sentences
        .clamp(MIN_PARAGRAPH_LEN, DEFAULT_MAX_SENTENCES_TOTAL);

    let documents = corpus
        .articles
        .iter()
        .map(|article| ClusterDocument {
            article_id: &article.article_id,
            title: article.title.as_ref(),
            lang_hint: Some(&article.language),
            published_at: None,
            source_url: None,
            paragraphs: vec![build_paragraph(&article.sentences)],
            genre_scores: article.genre_scores.as_ref(),
            confidence: article.confidence,
            signals: article.signals.as_ref(),
        })
        .collect();

    ClusterJobRequest {
        params: ClusterJobParams {
            max_sentences_total,
            max_sentences_per_cluster: 20,
            umap_n_components: DEFAULT_UMAP_N_COMPONENTS,
            hdbscan_min_cluster_size: DEFAULT_HDBSCAN_MIN_CLUSTER_SIZE,
            mmr_lambda: DEFAULT_MMR_LAMBDA,
        },
        documents,
        metadata: Some(&corpus.metadata),
    }
}

/// 文のリストからパラグラフを構築する。
fn build_paragraph(sentences: &[String]) -> String {
    fn ensure_min_length(mut line: String) -> String {
        let mut filler = line.trim().to_string();
        if filler.is_empty() {
            filler = "No content available.".to_string();
        }

        if line.trim().is_empty() {
            line.clone_from(&filler);
        }

        while line.chars().count() < MIN_PARAGRAPH_LEN {
            if !line.is_empty() {
                line.push(' ');
            }
            line.push_str(&filler);
        }

        line
    }

    if sentences.is_empty() {
        let line = ensure_min_length(String::new());
        return format!("{line}\n{line}");
    }

    let mut lines = Vec::new();
    let mut current = String::new();

    for sentence in sentences {
        let trimmed = sentence.trim();
        if trimmed.is_empty() {
            continue;
        }

        if !current.is_empty() {
            current.push(' ');
        }
        current.push_str(trimmed);

        if current.chars().count() >= MIN_PARAGRAPH_LEN {
            let line = ensure_min_length(std::mem::take(&mut current));
            lines.push(line);
        }
    }

    if !current.trim().is_empty() {
        let line = ensure_min_length(std::mem::take(&mut current));
        lines.push(line);
    }

    if lines.is_empty() {
        let line = ensure_min_length(String::new());
        return format!("{line}\n{line}");
    }

    lines.join("\n")
}

/// 進捗追跡用のヘルパー構造体。
#[allow(clippy::struct_field_names)] // `last_` プレフィックスは「前回の値」を表す意味で適切
struct ProgressTracker {
    last_progress_at: Instant,
    last_status: Option<ClusterJobStatus>,
    last_cluster_count: usize,
}

impl ProgressTracker {
    fn new(start_time: Instant) -> Self {
        Self {
            last_progress_at: start_time,
            last_status: None,
            last_cluster_count: 0,
        }
    }

    /// 進捗があった場合（statusが変わった、またはcluster_countが増えた）に更新する。
    fn update_if_progressed(&mut self, run: &ClusteringResponse) {
        let status_changed = self.last_status.as_ref().is_none_or(|s| *s != run.status);
        let cluster_count_increased = run.cluster_count > self.last_cluster_count;
        if status_changed || cluster_count_increased {
            self.last_progress_at = Instant::now();
            self.last_status = Some(run.status.clone());
            self.last_cluster_count = run.cluster_count;
        }
    }

    /// 最後の進捗から経過した時間を返す。
    fn time_since_progress(&self) -> Duration {
        Instant::now().saturating_duration_since(self.last_progress_at)
    }
}

/// コーパスのハッシュを計算してIdempotency-Keyに使用する。
fn compute_corpus_digest(request: &ClusterJobRequest<'_>) -> String {
    // リクエストの主要部分をシリアライズしてハッシュ化
    // ドキュメントIDとパラグラフの最初の数文字を使用（完全な内容は大きすぎる可能性がある）
    let mut digest_input = String::new();
    for doc in &request.documents {
        digest_input.push_str(doc.article_id);
        if let Some(first_para) = doc.paragraphs.first() {
            // 最初の100文字のみ使用（パフォーマンスと一意性のバランス）
            let truncated: String = first_para.chars().take(100).collect();
            digest_input.push_str(&truncated);
        }
    }
    // パラメータも含める
    if let Ok(params_json) = serde_json::to_string(&request.params) {
        digest_input.push_str(&params_json);
    }

    format!("{:016x}", xxh3_64(digest_input.as_bytes()))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn build_paragraph_uses_newlines_and_extends_length() {
        let sentences = vec![
            "This sentence is plenty long for clustering purposes.".to_string(),
            "Another sufficiently detailed sentence to pass validation.".to_string(),
        ];

        let paragraph = build_paragraph(&sentences);
        assert!(paragraph.contains('\n'));
        assert!(paragraph.chars().count() >= MIN_PARAGRAPH_LEN);
        let parts: Vec<&str> = paragraph.lines().collect();
        assert!(parts.iter().all(|part| !part.trim().is_empty()));
        for part in parts {
            assert!(
                part.chars().count() >= MIN_PARAGRAPH_LEN,
                "Each paragraph must be at least {} characters, got {}",
                MIN_PARAGRAPH_LEN,
                part.chars().count()
            );
        }
    }

    #[test]
    fn build_paragraph_falls_back_for_empty_articles() {
        let paragraph = build_paragraph(&[]);
        assert!(paragraph.contains('\n'));
        assert!(paragraph.chars().count() >= MIN_PARAGRAPH_LEN);
        let parts: Vec<&str> = paragraph.lines().collect();
        for part in parts {
            assert!(
                part.chars().count() >= MIN_PARAGRAPH_LEN,
                "Each paragraph must be at least {} characters",
                MIN_PARAGRAPH_LEN
            );
        }
    }

    #[test]
    fn build_paragraph_handles_japanese_short_sentences() {
        let sentences = vec!["やっぱり洗練されたサーバーの構成は美しい".to_string()];

        let paragraph = build_paragraph(&sentences);
        let parts: Vec<&str> = paragraph.lines().collect();
        for part in parts {
            let char_count = part.chars().count();
            assert!(
                char_count >= MIN_PARAGRAPH_LEN,
                "Each paragraph must be at least {} characters, got {}: '{}'",
                MIN_PARAGRAPH_LEN,
                char_count,
                part
            );
        }
    }

    #[test]
    fn build_paragraph_handles_multiple_short_sentences() {
        let sentences = vec![
            "短い文1".to_string(),
            "短い文2".to_string(),
            "短い文3".to_string(),
            "短い文4".to_string(),
        ];

        let paragraph = build_paragraph(&sentences);
        let parts: Vec<&str> = paragraph.lines().collect();
        for part in parts {
            assert!(
                part.chars().count() >= MIN_PARAGRAPH_LEN,
                "Each paragraph must be at least {} characters",
                MIN_PARAGRAPH_LEN
            );
        }
    }

    #[test]
    fn build_paragraph_boundary_test_29_chars() {
        let sentence_29 = "a".repeat(29);
        let sentences = vec![sentence_29.clone()];

        let paragraph = build_paragraph(&sentences);
        let parts: Vec<&str> = paragraph.lines().collect();
        for part in parts {
            assert!(
                part.chars().count() >= MIN_PARAGRAPH_LEN,
                "29-char sentence should be extended to at least {} characters",
                MIN_PARAGRAPH_LEN
            );
        }
    }

    #[test]
    fn build_paragraph_boundary_test_30_chars() {
        let sentence_30 = "a".repeat(30);
        let sentences = vec![sentence_30.clone()];

        let paragraph = build_paragraph(&sentences);
        let parts: Vec<&str> = paragraph.lines().collect();
        for part in parts {
            assert!(
                part.chars().count() >= MIN_PARAGRAPH_LEN,
                "30-char sentence should be at least {} characters",
                MIN_PARAGRAPH_LEN
            );
        }
    }

    #[test]
    fn build_paragraph_boundary_test_31_chars() {
        let sentence_31 = "a".repeat(31);
        let sentences = vec![sentence_31.clone()];

        let paragraph = build_paragraph(&sentences);
        let parts: Vec<&str> = paragraph.lines().collect();
        for part in parts {
            assert!(
                part.chars().count() >= MIN_PARAGRAPH_LEN,
                "31-char sentence should be at least {} characters",
                MIN_PARAGRAPH_LEN
            );
        }
    }

    #[test]
    fn build_paragraph_handles_mixed_languages() {
        let sentences = vec![
            "This is a short English sentence.".to_string(),
            "これは短い日本語の文です。".to_string(),
        ];

        let paragraph = build_paragraph(&sentences);
        let parts: Vec<&str> = paragraph.lines().collect();
        for part in parts {
            assert!(
                part.chars().count() >= MIN_PARAGRAPH_LEN,
                "Each paragraph must be at least {} characters",
                MIN_PARAGRAPH_LEN
            );
        }
    }

    #[test]
    fn build_paragraph_verifies_each_split_paragraph() {
        let sentences = vec![
            "This is a very long sentence that exceeds the minimum paragraph length requirement."
                .to_string(),
            "Another long sentence that also exceeds the minimum requirement for paragraph length."
                .to_string(),
            "A third sentence that is also sufficiently long to meet the requirements.".to_string(),
        ];

        let paragraph = build_paragraph(&sentences);
        let parts: Vec<&str> = paragraph.lines().collect();
        for (idx, part) in parts.iter().enumerate() {
            let char_count = part.chars().count();
            assert!(
                char_count >= MIN_PARAGRAPH_LEN,
                "Paragraph {} must be at least {} characters, got {}: '{}'",
                idx,
                MIN_PARAGRAPH_LEN,
                char_count,
                part
            );
        }
    }

    fn sample_corpus(genre: &str, article_count: usize) -> EvidenceCorpus {
        use crate::pipeline::evidence::{CorpusMetadata, EvidenceArticle};
        use std::collections::HashMap;

        let articles = (0..article_count)
            .map(|i| EvidenceArticle {
                article_id: format!("art-{i}"),
                title: Some(format!("Title {i}")),
                sentences: vec![
                    "This is a sufficiently long sentence for clustering tests.".to_string(),
                ],
                language: "en".to_string(),
                published_at: None,
                source_url: None,
                score: 1.0,
                genre_scores: None,
                confidence: None,
                signals: None,
            })
            .collect();

        let mut lang_dist = HashMap::new();
        lang_dist.insert("en".to_string(), article_count);

        EvidenceCorpus {
            genre: genre.to_string(),
            articles,
            total_sentences: article_count,
            metadata: CorpusMetadata {
                article_count,
                sentence_count: article_count,
                primary_language: "en".to_string(),
                language_distribution: lang_dist,
                character_count: article_count * 58,
                classifier: None,
            },
        }
    }

    fn valid_clustering_response_json(
        run_id: i64,
        job_id: Uuid,
        genre: &str,
        status: &str,
        with_cluster: bool,
    ) -> Value {
        let clusters = if with_cluster {
            vec![serde_json::json!({
                "cluster_id": 0,
                "size": 3,
                "label": genre,
                "top_terms": ["ai", "model"],
                "stats": {},
                "representatives": [{
                    "article_id": "art-0",
                    "paragraph_idx": 0,
                    "sentence_text": "This is a sufficiently long sentence for clustering tests.",
                    "lang": "en",
                    "score": 0.9
                }]
            })]
        } else {
            Vec::new()
        };

        serde_json::json!({
            "run_id": run_id,
            "job_id": job_id.to_string(),
            "genre": genre,
            "status": status,
            "cluster_count": clusters.len(),
            "clusters": clusters,
            "diagnostics": {}
        })
    }

    #[tokio::test]
    async fn cluster_corpus_with_timeout_returns_fallback_when_below_min_documents() {
        use wiremock::matchers::method;
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .respond_with(ResponseTemplate::new(500))
            .expect(0)
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri(), 3, None).expect("client should build");
        let job_id = Uuid::new_v4();
        let corpus = sample_corpus("ai", 1);

        let res = client
            .cluster_corpus_with_timeout(job_id, &corpus, Duration::from_secs(5), None)
            .await
            .expect("single article below min_documents_per_genre should return fallback");

        assert_eq!(res.run_id, 0);
        assert_eq!(res.cluster_count, 1);
        assert_eq!(res.clusters.len(), 1);
        server.verify().await;
    }

    #[tokio::test]
    async fn cluster_corpus_with_timeout_errors_when_zero_documents() {
        use wiremock::matchers::method;
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .respond_with(ResponseTemplate::new(500))
            .expect(0)
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri(), 3, None).expect("client should build");
        let corpus = sample_corpus("ai", 0);

        let err = client
            .cluster_corpus_with_timeout(Uuid::new_v4(), &corpus, Duration::from_secs(5), None)
            .await
            .expect_err("zero documents must fail");

        assert!(matches!(
            err,
            RecapError::InsufficientDocuments { min: 1, found: 0 }
        ));
        server.verify().await;
    }

    #[tokio::test]
    async fn cluster_corpus_with_timeout_sends_expected_request_and_accepts_succeeded_and_partial()
    {
        use wiremock::matchers::{header, header_exists, method, path};
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let server = MockServer::start().await;
        let job_id = Uuid::new_v4();
        let corpus = sample_corpus("ai", 3);

        Mock::given(method("POST"))
            .and(path("/v1/runs"))
            .and(header("x-alt-job-id", job_id.to_string().as_str()))
            .and(header("x-alt-genre", "ai"))
            .and(header_exists("idempotency-key"))
            .respond_with(
                ResponseTemplate::new(200).set_body_json(valid_clustering_response_json(
                    42,
                    job_id,
                    "ai",
                    "succeeded",
                    true,
                )),
            )
            .up_to_n_times(1)
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri(), 2, None).expect("client should build");
        let succeeded = client
            .cluster_corpus_with_timeout(job_id, &corpus, Duration::from_secs(5), None)
            .await
            .expect("succeeded clustering response should be accepted");
        assert_eq!(succeeded.run_id, 42);
        assert_eq!(succeeded.clusters.len(), 1);

        Mock::given(method("POST"))
            .and(path("/v1/runs"))
            .respond_with(
                ResponseTemplate::new(200).set_body_json(valid_clustering_response_json(
                    43, job_id, "ai", "partial", true,
                )),
            )
            .up_to_n_times(1)
            .mount(&server)
            .await;

        let partial = client
            .cluster_corpus_with_timeout(job_id, &corpus, Duration::from_secs(5), None)
            .await
            .expect("partial clustering response is treated as success");
        assert_eq!(partial.run_id, 43);
        assert_eq!(partial.status, ClusterJobStatus::Partial);
    }

    #[tokio::test]
    async fn cluster_corpus_with_timeout_polls_running_run_until_succeeded() {
        use wiremock::matchers::{method, path};
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let server = MockServer::start().await;
        let job_id = Uuid::new_v4();
        let corpus = sample_corpus("tech", 3);

        Mock::given(method("POST"))
            .and(path("/v1/runs"))
            .respond_with(
                ResponseTemplate::new(202).set_body_json(valid_clustering_response_json(
                    99, job_id, "tech", "running", false,
                )),
            )
            .expect(1)
            .mount(&server)
            .await;

        Mock::given(method("GET"))
            .and(path("/v1/runs/99"))
            .respond_with(
                ResponseTemplate::new(200).set_body_json(valid_clustering_response_json(
                    99,
                    job_id,
                    "tech",
                    "succeeded",
                    true,
                )),
            )
            .expect(1)
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri(), 2, None).expect("client should build");
        let res = client
            .cluster_corpus_with_timeout(job_id, &corpus, Duration::from_secs(60), None)
            .await
            .expect("running then succeeded poll should complete");

        assert_eq!(res.status, ClusterJobStatus::Succeeded);
        assert_eq!(res.clusters.len(), 1);
        server.verify().await;
    }

    #[tokio::test]
    async fn cluster_corpus_with_timeout_times_out_immediately_when_deadline_zero() {
        use wiremock::matchers::{method, path};
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let server = MockServer::start().await;
        let job_id = Uuid::new_v4();
        let corpus = sample_corpus("tech", 3);

        Mock::given(method("POST"))
            .and(path("/v1/runs"))
            .respond_with(
                ResponseTemplate::new(202).set_body_json(valid_clustering_response_json(
                    55, job_id, "tech", "running", false,
                )),
            )
            .expect(1)
            .mount(&server)
            .await;

        Mock::given(method("GET"))
            .and(path("/v1/runs/55"))
            .respond_with(ResponseTemplate::new(200))
            .expect(0)
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri(), 2, None).expect("client should build");
        let err = client
            .cluster_corpus_with_timeout(job_id, &corpus, Duration::ZERO, None)
            .await
            .expect_err("zero deadline must time out before polling");

        assert!(err.to_string().contains("did not complete within timeout"));
        server.verify().await;
    }

    #[tokio::test]
    async fn cluster_corpus_with_timeout_rejects_non_2xx_and_invalid_bodies() {
        use wiremock::matchers::{method, path};
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let server = MockServer::start().await;
        let client = SubworkerClient::new(server.uri(), 2, None).expect("client should build");
        let corpus = sample_corpus("ai", 3);

        for status in [401u16, 403, 400, 500] {
            Mock::given(method("POST"))
                .and(path("/v1/runs"))
                .respond_with(ResponseTemplate::new(status).set_body_string("error body"))
                .up_to_n_times(1)
                .mount(&server)
                .await;

            let err = client
                .cluster_corpus_with_timeout(Uuid::new_v4(), &corpus, Duration::from_secs(5), None)
                .await
                .expect_err("non-2xx must fail");
            assert!(err.to_string().contains(&format!(
                "clustering endpoint returned error status {status}"
            )));
        }

        Mock::given(method("POST"))
            .and(path("/v1/runs"))
            .respond_with(ResponseTemplate::new(200).set_body_string("not-json"))
            .up_to_n_times(1)
            .mount(&server)
            .await;
        let err_json = client
            .cluster_corpus_with_timeout(Uuid::new_v4(), &corpus, Duration::from_secs(5), None)
            .await
            .expect_err("invalid json must fail");
        assert!(
            err_json
                .to_string()
                .contains("failed to deserialize clustering response as JSON")
        );

        Mock::given(method("POST"))
            .and(path("/v1/runs"))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
                "run_id": 1
            })))
            .up_to_n_times(1)
            .mount(&server)
            .await;
        let err_schema = client
            .cluster_corpus_with_timeout(Uuid::new_v4(), &corpus, Duration::from_secs(5), None)
            .await
            .expect_err("schema invalid json must fail");
        assert!(
            err_schema
                .to_string()
                .contains("clustering response validation failed")
        );
    }

    #[tokio::test]
    async fn cluster_corpus_with_timeout_fails_on_terminal_failed_status_and_transport_error() {
        use wiremock::matchers::{method, path};
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let server = MockServer::start().await;
        let job_id = Uuid::new_v4();
        let corpus = sample_corpus("ai", 3);

        Mock::given(method("POST"))
            .and(path("/v1/runs"))
            .respond_with(
                ResponseTemplate::new(200).set_body_json(valid_clustering_response_json(
                    77, job_id, "ai", "failed", false,
                )),
            )
            .expect(1)
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri(), 2, None).expect("client should build");
        let err_failed = client
            .cluster_corpus_with_timeout(job_id, &corpus, Duration::from_secs(5), None)
            .await
            .expect_err("terminal failed status must error");
        assert!(
            err_failed
                .to_string()
                .contains("clustering run 77 finished with status failed")
        );
        server.verify().await;

        let listener = std::net::TcpListener::bind("127.0.0.1:0").expect("bind ephemeral port");
        let addr = listener.local_addr().expect("local addr");
        drop(listener);
        let dead_client =
            SubworkerClient::new(format!("http://{addr}/"), 2, None).expect("client should build");
        let err_transport = dead_client
            .cluster_corpus_with_timeout(job_id, &corpus, Duration::from_secs(5), None)
            .await
            .expect_err("unreachable endpoint must fail");
        assert!(
            err_transport
                .to_string()
                .contains("clustering request failed")
        );
    }
}
