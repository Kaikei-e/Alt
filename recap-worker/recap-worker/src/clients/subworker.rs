use std::{cmp, collections::HashMap, fmt, time::Duration};

use anyhow::{Context, Result, anyhow};
use reqwest::{Client, Url};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use tokio::time::sleep;
use tracing::{debug, warn};
use uuid::Uuid;

use crate::pipeline::evidence::{ArticleFeatureSignal, CorpusMetadata, EvidenceCorpus};
use crate::schema::{subworker::CLUSTERING_RESPONSE_SCHEMA, validate_json};

#[derive(Debug, Clone)]
pub(crate) struct SubworkerClient {
    client: Client,
    base_url: Url,
    min_documents_per_genre: usize,
}

const DEFAULT_MAX_SENTENCES_TOTAL: usize = 2_000;
const DEFAULT_UMAP_N_COMPONENTS: usize = 25;
const DEFAULT_HDBSCAN_MIN_CLUSTER_SIZE: usize = 5;
const DEFAULT_MMR_LAMBDA: f32 = 0.35;
const MIN_PARAGRAPH_LEN: usize = 30;
const MAX_POLL_ATTEMPTS: usize = 30;
const INITIAL_POLL_INTERVAL_MS: u64 = 500;
const MAX_POLL_INTERVAL_MS: u64 = 5_000;
const SUBWORKER_TIMEOUT_SECS: u64 = 120;
const MAX_ERROR_MESSAGE_LENGTH: usize = 500;

/// エラーメッセージを要約して切り詰める。
fn truncate_error_message(msg: &str) -> String {
    let char_count = msg.chars().count();
    if char_count <= MAX_ERROR_MESSAGE_LENGTH {
        return msg.to_string();
    }
    let truncated: String = msg.chars().take(MAX_ERROR_MESSAGE_LENGTH).collect();
    format!("{truncated}... (truncated, {char_count} chars)")
}

/// バリデーションエラーのリストを要約する。
fn summarize_validation_errors(errors: &[String]) -> Vec<String> {
    errors.iter().map(|e| truncate_error_message(e)).collect()
}

/// クラスタリングレスポンス (POST/GET `/v1/runs`).
#[allow(dead_code)]
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct ClusteringResponse {
    pub(crate) run_id: i64,
    pub(crate) job_id: Uuid,
    pub(crate) genre: String,
    pub(crate) status: ClusterJobStatus,
    #[serde(default)]
    pub(crate) cluster_count: usize,
    #[serde(default)]
    pub(crate) clusters: Vec<ClusterInfo>,
    #[serde(default)]
    pub(crate) diagnostics: Value,
}

impl ClusteringResponse {
    fn is_success(&self) -> bool {
        self.status.is_success()
    }
}

#[derive(Debug, Clone, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub(crate) enum ClusterJobStatus {
    Running,
    Succeeded,
    Partial,
    Failed,
}

impl ClusterJobStatus {
    fn is_running(&self) -> bool {
        matches!(self, ClusterJobStatus::Running)
    }

    fn is_success(&self) -> bool {
        matches!(
            self,
            ClusterJobStatus::Succeeded | ClusterJobStatus::Partial
        )
    }

    fn is_terminal(&self) -> bool {
        !self.is_running()
    }
}

impl fmt::Display for ClusterJobStatus {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ClusterJobStatus::Running => write!(f, "running"),
            ClusterJobStatus::Succeeded => write!(f, "succeeded"),
            ClusterJobStatus::Partial => write!(f, "partial"),
            ClusterJobStatus::Failed => write!(f, "failed"),
        }
    }
}

#[allow(dead_code)]
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct ClusterInfo {
    pub(crate) cluster_id: i32,
    pub(crate) size: usize,
    #[serde(default)]
    pub(crate) label: Option<String>,
    #[serde(default)]
    pub(crate) top_terms: Vec<String>,
    #[serde(default)]
    pub(crate) stats: Value,
    #[serde(default)]
    pub(crate) representatives: Vec<ClusterRepresentative>,
}

#[allow(dead_code)]
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct ClusterRepresentative {
    #[serde(default)]
    pub(crate) article_id: String,
    #[serde(default)]
    pub(crate) paragraph_idx: Option<i32>,
    #[serde(rename = "sentence_text")]
    pub(crate) text: String,
    #[serde(default)]
    pub(crate) lang: Option<String>,
    #[serde(default)]
    pub(crate) score: Option<f32>,
}

#[derive(Debug, Clone, Serialize)]
struct ClusterJobRequest<'a> {
    params: ClusterJobParams,
    documents: Vec<ClusterDocument<'a>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    metadata: Option<&'a CorpusMetadata>,
}

#[derive(Debug, Clone, Serialize)]
struct ClusterJobParams {
    max_sentences_total: usize,
    umap_n_components: usize,
    hdbscan_min_cluster_size: usize,
    mmr_lambda: f32,
}

#[derive(Debug, Clone, Serialize)]
struct ClusterDocument<'a> {
    article_id: &'a str,
    #[serde(skip_serializing_if = "Option::is_none")]
    title: Option<&'a String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    lang_hint: Option<&'a String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    published_at: Option<&'a String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    source_url: Option<&'a String>,
    paragraphs: Vec<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    genre_scores: Option<&'a HashMap<String, usize>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    confidence: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    signals: Option<&'a ArticleFeatureSignal>,
}

impl SubworkerClient {
    pub(crate) fn new(endpoint: impl Into<String>, min_documents_per_genre: usize) -> Result<Self> {
        let client = Client::builder()
            .timeout(Duration::from_secs(SUBWORKER_TIMEOUT_SECS))
            .build()
            .context("failed to build subworker client")?;

        let base_url = Url::parse(&endpoint.into()).context("invalid subworker base URL")?;

        Ok(Self {
            client,
            base_url,
            min_documents_per_genre,
        })
    }

    pub(crate) async fn ping(&self) -> Result<()> {
        let url = self
            .base_url
            .join("health")
            .context("failed to build subworker health URL")?;

        self.client
            .get(url)
            .send()
            .await
            .context("subworker health request failed")?
            .error_for_status()
            .context("subworker health endpoint returned error status")?;

        Ok(())
    }

    /// /admin/build-graphを呼び出してtag_label_graphを再構築
    pub(crate) async fn trigger_graph_rebuild(&self) -> Result<()> {
        let url = self
            .base_url
            .join("admin/build-graph")
            .context("failed to build subworker build-graph URL")?;

        tracing::info!("triggering tag_label_graph rebuild");
        let response = self
            .client
            .post(url)
            .send()
            .await
            .context("subworker build-graph request failed")?;

        let status = response.status();
        if !status.is_success() {
            let error_text = response
                .text()
                .await
                .unwrap_or_else(|_| "unknown error".to_string());
            return Err(anyhow!(
                "subworker build-graph endpoint returned error status {}: {}",
                status,
                error_text
            ));
        }

        let result: Value = response
            .json()
            .await
            .context("failed to parse build-graph response")?;

        tracing::info!(
            result = ?result,
            "tag_label_graph rebuild completed"
        );

        Ok(())
    }

    /// /admin/learningを呼び出してジャンル学習を実行
    pub(crate) async fn trigger_learning(&self) -> Result<()> {
        let url = self
            .base_url
            .join("admin/learning")
            .context("failed to build subworker learning URL")?;

        tracing::info!("triggering genre learning");
        let response = self
            .client
            .post(url)
            .send()
            .await
            .context("subworker learning request failed")?;

        let status = response.status();
        // 202 Accepted は正常なレスポンス（非同期処理開始）
        if status != reqwest::StatusCode::ACCEPTED && !status.is_success() {
            let error_text = response
                .text()
                .await
                .unwrap_or_else(|_| "unknown error".to_string());
            return Err(anyhow!(
                "subworker learning endpoint returned error status {}: {}",
                status,
                error_text
            ));
        }

        tracing::info!(
            status = %status,
            "genre learning triggered successfully"
        );

        Ok(())
    }

    /// グラフ最新化シーケンス（build-graph -> learning）を実行
    pub(crate) async fn refresh_graph_and_learning(&self) -> Result<()> {
        // Step 1: Build graph
        self.trigger_graph_rebuild()
            .await
            .context("graph rebuild failed")?;

        // Step 2: Trigger learning (which also rebuilds graph, but we do it explicitly first)
        self.trigger_learning()
            .await
            .context("learning trigger failed")?;

        tracing::info!("graph refresh and learning sequence completed");
        Ok(())
    }

    /// 証拠コーパスを送信してクラスタリング結果を取得する。
    ///
    /// # Arguments
    /// * `job_id` - ジョブID
    /// * `corpus` - 証拠コーパス
    ///
    /// # Returns
    /// クラスタリング結果（JSON Schema検証済み）
    pub(crate) async fn cluster_corpus(
        &self,
        job_id: Uuid,
        corpus: &EvidenceCorpus,
    ) -> Result<ClusteringResponse> {
        let runs_url = build_runs_url(&self.base_url)?;

        debug!(
            job_id = %job_id,
            genre = %corpus.genre,
            article_count = corpus.articles.len(),
            "sending evidence corpus to subworker"
        );

        let request_payload = build_cluster_job_request(corpus);
        let idempotency_key = format!("{}::{}", job_id, corpus.genre);
        let document_count = request_payload.documents.len();

        if document_count < self.min_documents_per_genre {
            warn!(
                job_id = %job_id,
                genre = %corpus.genre,
                document_count,
                min_required = self.min_documents_per_genre,
                "skipping clustering because document count is below minimum"
            );
            return Err(anyhow!(
                "insufficient documents for clustering: expected >= {}, found {}",
                self.min_documents_per_genre,
                document_count
            ));
        }

        let response = self
            .client
            .post(runs_url.clone())
            .json(&request_payload)
            .header("X-Alt-Job-Id", job_id.to_string())
            .header("X-Alt-Genre", &corpus.genre)
            .header("Idempotency-Key", idempotency_key)
            .send()
            .await
            .context("clustering request failed")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            let truncated_body = truncate_error_message(&body);
            return Err(anyhow!(
                "clustering endpoint returned error status {}: {}",
                status,
                truncated_body
            ));
        }

        // レスポンスをJSONとして取得
        let response_json: Value = response
            .json()
            .await
            .context("failed to deserialize clustering response as JSON")?;

        // JSON Schemaで検証
        let validation = validate_json(&CLUSTERING_RESPONSE_SCHEMA, &response_json);
        if !validation.valid {
            let summarized_errors = summarize_validation_errors(&validation.errors);
            warn!(
                job_id = %job_id,
                genre = %corpus.genre,
                error_count = validation.errors.len(),
                first_error = %summarized_errors.first().map_or("unknown", String::as_str),
                "clustering response failed JSON Schema validation"
            );
            return Err(anyhow!(
                "clustering response validation failed: {} errors (first: {})",
                validation.errors.len(),
                summarized_errors.first().map_or("unknown", String::as_str)
            ));
        }

        debug!(
            job_id = %job_id,
            genre = %corpus.genre,
            "clustering response passed JSON Schema validation"
        );

        // 検証済みのJSONを構造体にデシリアライズ
        let mut run: ClusteringResponse = serde_json::from_value(response_json)
            .context("failed to deserialize validated clustering response")?;

        if run.status.is_running() {
            run = self.poll_run(run.run_id).await?;
        }

        if !run.is_success() {
            return Err(anyhow!(
                "clustering run {} finished with status {}",
                run.run_id,
                run.status
            ));
        }

        Ok(run)
    }

    async fn poll_run(&self, run_id: i64) -> Result<ClusteringResponse> {
        let run_url = build_run_url(&self.base_url, run_id)?;

        for attempt in 0..MAX_POLL_ATTEMPTS {
            let response = self
                .client
                .get(run_url.clone())
                .send()
                .await
                .context("clustering run polling request failed")?;

            if !response.status().is_success() {
                let status = response.status();
                let body = response.text().await.unwrap_or_default();
                let truncated_body = truncate_error_message(&body);
                return Err(anyhow!(
                    "run polling endpoint returned error status {}: {}",
                    status,
                    truncated_body
                ));
            }

            let response_json: Value = response
                .json()
                .await
                .context("failed to deserialize polling response as JSON")?;

            let validation = validate_json(&CLUSTERING_RESPONSE_SCHEMA, &response_json);
            if !validation.valid {
                let summarized_errors = summarize_validation_errors(&validation.errors);
                warn!(
                    run_id,
                    error_count = validation.errors.len(),
                    first_error = %summarized_errors.first().map_or("unknown", String::as_str),
                    "polling response failed JSON Schema validation"
                );
                return Err(anyhow!(
                    "run polling response validation failed: {} errors (first: {})",
                    validation.errors.len(),
                    summarized_errors.first().map_or("unknown", String::as_str)
                ));
            }

            let run: ClusteringResponse = serde_json::from_value(response_json)
                .context("failed to deserialize validated polling response")?;

            if run.status.is_terminal() {
                if !run.is_success() {
                    warn!(
                        run_id,
                        status = %run.status,
                        "clustering run completed with non-success status"
                    );
                }
                return Ok(run);
            }

            debug!(
                run_id,
                attempt,
                status = %run.status,
                "clustering run still in progress"
            );

            let backoff = cmp::min(
                INITIAL_POLL_INTERVAL_MS * (1_u64 << attempt.min(10)),
                MAX_POLL_INTERVAL_MS,
            );
            sleep(Duration::from_millis(backoff)).await;
        }

        Err(anyhow!(
            "clustering run {} did not complete within timeout",
            run_id
        ))
    }
}

fn build_runs_url(base: &Url) -> Result<Url> {
    let mut url = base.clone();
    url.path_segments_mut()
        .map_err(|()| anyhow!("subworker base URL must be absolute"))?
        .extend(["v1", "runs"]);
    Ok(url)
}

fn build_run_url(base: &Url, run_id: i64) -> Result<Url> {
    let mut url = base.clone();
    url.path_segments_mut()
        .map_err(|()| anyhow!("subworker base URL must be absolute"))?
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
            umap_n_components: DEFAULT_UMAP_N_COMPONENTS,
            hdbscan_min_cluster_size: DEFAULT_HDBSCAN_MIN_CLUSTER_SIZE,
            mmr_lambda: DEFAULT_MMR_LAMBDA,
        },
        documents,
        metadata: Some(&corpus.metadata),
    }
}

/// 文のリストからパラグラフを構築する。
/// 改行で分割した各パラグラフが30文字以上になることを保証する。
/// Unicode文字数を使用して正確にカウントする。
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

#[cfg(test)]
mod tests {
    use super::*;
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    #[tokio::test]
    async fn ping_succeeds_for_ok_response() {
        let server = MockServer::start().await;
        Mock::given(method("GET"))
            .and(path("/health"))
            .respond_with(ResponseTemplate::new(204))
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri(), 10).expect("client should build");

        client.ping().await.expect("ping should succeed");
    }

    #[tokio::test]
    async fn ping_fails_on_error_status() {
        let server = MockServer::start().await;
        Mock::given(method("GET"))
            .and(path("/health"))
            .respond_with(ResponseTemplate::new(500))
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri(), 10).expect("client should build");

        let error = client.ping().await.expect_err("ping should fail");
        assert!(error.to_string().contains("error status"));
    }

    #[test]
    fn build_paragraph_uses_newlines_and_extends_length() {
        let sentences = vec![
            "This sentence is plenty long for clustering purposes.".to_string(),
            "Another sufficiently detailed sentence to pass validation.".to_string(),
        ];

        let paragraph = build_paragraph(&sentences);
        assert!(paragraph.contains('\n'));
        // Unicode文字数で検証
        assert!(paragraph.chars().count() >= MIN_PARAGRAPH_LEN);
        // recap-subworker側はsplitlines()を使用するので、それに合わせて検証
        let parts: Vec<&str> = paragraph.lines().collect();
        assert!(parts.iter().all(|part| !part.trim().is_empty()));
        // 各パラグラフが30文字以上であることを確認
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
        // 分割後の各パラグラフが30文字以上であることを確認（splitlines()を使用）
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
        // エラーが発生した実際のケース: 18文字の日本語文
        let sentences = vec!["やっぱり洗練されたサーバーの構成は美しい".to_string()];

        let paragraph = build_paragraph(&sentences);
        // 分割後の各パラグラフが30文字以上であることを確認（splitlines()を使用）
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
        // 複数の短い文を結合するケース
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
        // 29文字の文（境界値テスト）
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
        // 30文字の文（境界値テスト）
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
        // 31文字の文（境界値テスト）
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
        // 日本語と英語が混在するケース
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
        // 複数の長い文で、分割後の各パラグラフが30文字以上であることを確認
        let sentences = vec![
            "This is a very long sentence that exceeds the minimum paragraph length requirement."
                .to_string(),
            "Another long sentence that also exceeds the minimum requirement for paragraph length."
                .to_string(),
            "A third sentence that is also sufficiently long to meet the requirements.".to_string(),
        ];

        let paragraph = build_paragraph(&sentences);
        // 改行で分割した各パラグラフを検証（splitlines()を使用）
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

    #[test]
    fn truncate_error_message_handles_short_messages() {
        let msg = "Short error message";
        let result = truncate_error_message(msg);
        assert_eq!(result, msg);
    }

    #[test]
    fn truncate_error_message_handles_long_ascii_messages() {
        let msg = "a".repeat(600);
        let result = truncate_error_message(&msg);
        assert!(result.starts_with('a'));
        assert!(result.contains("... (truncated"));
        assert!(result.contains("600 chars"));
        // 切り詰められた部分が正確に500文字であることを確認
        let truncated_part = result.split("...").next().unwrap();
        assert_eq!(truncated_part.chars().count(), MAX_ERROR_MESSAGE_LENGTH);
    }

    #[test]
    fn truncate_error_message_handles_japanese_at_boundary() {
        // ログで実際に発生したケース: 500文字目が日本語文字の途中
        // 'れ' (bytes 498..501) や 'ン' (bytes 499..502) が境界に来る
        let mut msg = "a".repeat(498);
        msg.push_str("れン");
        msg.push_str(&"b".repeat(100));

        // パニックが発生しないことを確認
        let result = truncate_error_message(&msg);
        assert!(result.contains("... (truncated"));
        // 切り詰められた部分が文字境界で切れていることを確認
        let truncated_part = result.split("...").next().unwrap();
        assert_eq!(truncated_part.chars().count(), MAX_ERROR_MESSAGE_LENGTH);
        // 文字列が有効なUTF-8であることを確認（パニックしない）
        assert!(truncated_part.is_char_boundary(truncated_part.len()));
    }

    #[test]
    fn truncate_error_message_handles_mixed_languages() {
        let mut msg = String::new();
        msg.push_str("This is an English error message. ");
        msg.push_str("これは日本語のエラーメッセージです。 ");
        msg.push_str("This is more English text. ");
        msg.push_str("さらに日本語のテキストが続きます。 ");
        // 500文字を超えるように繰り返す
        msg = msg.repeat(20);

        let result = truncate_error_message(&msg);
        assert!(result.contains("... (truncated"));
        let truncated_part = result.split("...").next().unwrap();
        assert_eq!(truncated_part.chars().count(), MAX_ERROR_MESSAGE_LENGTH);
        // 文字列が有効なUTF-8であることを確認
        assert!(truncated_part.is_char_boundary(truncated_part.len()));
    }

    #[test]
    fn truncate_error_message_preserves_exact_length_messages() {
        let msg = "a".repeat(MAX_ERROR_MESSAGE_LENGTH);
        let result = truncate_error_message(&msg);
        assert_eq!(result, msg);
        assert!(!result.contains("... (truncated"));
    }
}
