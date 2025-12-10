use anyhow::{Context, Result, anyhow};
use reqwest::Url;
use std::{cmp, time::Duration};
use tokio::time::sleep;
use tracing::{debug, warn};
use uuid::Uuid;

use super::types::{
    ClusterDocument, ClusterInfo, ClusterJobParams, ClusterJobRequest, ClusterJobStatus,
    ClusterRepresentative, ClusteringResponse, DEFAULT_HDBSCAN_MIN_CLUSTER_SIZE,
    DEFAULT_MAX_SENTENCES_TOTAL, DEFAULT_MMR_LAMBDA, DEFAULT_UMAP_N_COMPONENTS,
    INITIAL_POLL_INTERVAL_MS, MAX_POLL_ATTEMPTS, MAX_POLL_INTERVAL_MS, MIN_FALLBACK_DOCUMENTS,
    MIN_PARAGRAPH_LEN,
};
use super::utils::{summarize_validation_errors, truncate_error_message};
use crate::clients::subworker::SubworkerClient;
use crate::pipeline::evidence::EvidenceCorpus;
use crate::schema::{subworker::CLUSTERING_RESPONSE_SCHEMA, validate_json};
use serde_json::Value;

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
        let idempotency_key = format!("{}::{}::{}", job_id, corpus.genre, Uuid::new_v4());
        let document_count = request_payload.documents.len();

        if document_count < self.min_documents_per_genre {
            if document_count >= MIN_FALLBACK_DOCUMENTS {
                warn!(
                    job_id = %job_id,
                    genre = %corpus.genre,
                    document_count,
                    min_required = self.min_documents_per_genre,
                    "using fallback single-cluster response due to insufficient documents"
                );
                return Ok(Self::create_fallback_response(job_id, corpus));
            }
            warn!(
                job_id = %job_id,
                genre = %corpus.genre,
                document_count,
                min_fallback = MIN_FALLBACK_DOCUMENTS,
                "skipping clustering because document count is below minimum fallback threshold"
            );
            return Err(anyhow!(
                "insufficient documents for clustering: expected >= {}, found {}",
                MIN_FALLBACK_DOCUMENTS,
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

        let response_json: Value = response
            .json()
            .await
            .context("failed to deserialize clustering response as JSON")?;

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

    #[allow(dead_code)]
    pub(crate) async fn cluster_other(
        &self,
        sentences: Vec<String>,
    ) -> anyhow::Result<(Vec<i32>, Option<Vec<i32>>, Option<Vec<Vec<f32>>>)> {
        use super::types::{SubClusterOtherRequest, SubClusterOtherResponse};

        let url = self
            .base_url
            .join("v1/cluster/other")
            .context("failed to build cluster_other URL")?;

        let body = SubClusterOtherRequest { sentences };
        let response = self
            .client
            .post(url)
            .json(&body)
            .send()
            .await
            .context("cluster_other request failed")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(anyhow!("cluster_other endpoint error {}: {}", status, body));
        }

        let body: SubClusterOtherResponse = response
            .json()
            .await
            .context("failed to parse cluster_other response")?;
        Ok((body.cluster_ids, body.labels, body.centers))
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
}
