use anyhow::{Context, Result, anyhow};
use reqwest::{Response, Url};
use std::time::Duration;
use tokio::time::sleep;
use tracing::{debug, error, info, warn};
use uuid::Uuid;

use super::types::{
    CLASSIFY_POST_BACKOFF_MS, CLASSIFY_POST_RETRIES, ClassificationJobResponse,
    ClassificationRequest, ClassificationResult, INITIAL_POLL_INTERVAL_MS, MAX_POLL_ATTEMPTS,
    MAX_POLL_INTERVAL_MS, POLL_REQUEST_RETRIES, POLL_REQUEST_RETRY_DELAY_MS,
    SUBWORKER_TIMEOUT_SECS,
};
use super::utils::truncate_error_message;
use crate::clients::subworker::SubworkerClient;
use crate::queue::ClassificationJobQueue;

impl SubworkerClient {
    /// Classify texts using the queue system (recommended for large batches)
    pub(crate) async fn classify_texts_queued(
        &self,
        queue: &ClassificationJobQueue,
        job_id: Uuid,
        texts: Vec<String>,
    ) -> Result<Vec<ClassificationResult>> {
        // Enqueue all chunks
        let _job_ids = queue.enqueue_classification(job_id, texts).await?;

        // Wait for completion (with timeout from client)
        let timeout = Duration::from_secs(SUBWORKER_TIMEOUT_SECS);
        let queue_results = queue.wait_for_completion(job_id, timeout).await?;

        // Convert from queue::ClassificationResult to subworker::ClassificationResult
        Ok(queue_results
            .into_iter()
            .map(|r| ClassificationResult {
                top_genre: r.top_genre,
                confidence: r.confidence,
                scores: r.scores,
            })
            .collect())
    }

    /// Classify a single chunk of texts
    pub(crate) async fn classify_chunk(
        &self,
        job_id: Uuid,
        texts: Vec<String>,
        chunk_idx: usize,
    ) -> Result<Vec<ClassificationResult>> {
        let request = ClassificationRequest {
            texts: texts.clone(),
        };
        let url = self.build_classify_url()?;

        info!(
            job_id = %job_id,
            chunk_idx,
            text_count = texts.len(),
            "processing classification chunk"
        );

        // Generate unique idempotency key for this chunk
        let idempotency_key = format!("{}-chunk-{}", job_id, chunk_idx);

        let response = self
            .send_classify_request(job_id, &request, &url, Some(&idempotency_key))
            .await
            .with_context(|| {
                format!(
                    "classify-runs POST request failed for job {} chunk {}",
                    job_id, chunk_idx
                )
            })?;
        let body = self
            .parse_classify_response(job_id, response)
            .await
            .with_context(|| {
                format!(
                    "failed to parse classify-runs response for job {} chunk {}",
                    job_id, chunk_idx
                )
            })?;

        let results = self
            .process_classify_body(job_id, body)
            .await
            .with_context(|| {
                format!(
                    "failed to process classification chunk {} for job {}",
                    chunk_idx, job_id
                )
            })?;

        info!(
            job_id = %job_id,
            chunk_idx,
            result_count = results.len(),
            "classification chunk completed"
        );

        Ok(results)
    }

    /// Coarse classification (single text)
    pub(crate) async fn classify_coarse(
        &self,
        text: &str,
    ) -> Result<std::collections::HashMap<String, f32>> {
        use super::types::{CoarseClassifyRequest, CoarseClassifyResponse};

        let url = self
            .base_url
            .join("v1/classify/coarse")
            .context("failed to build classify_coarse URL")?;

        let request = CoarseClassifyRequest { text };

        let response = self
            .client
            .post(url)
            .json(&request)
            .send()
            .await
            .context("classify_coarse request failed")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(anyhow!(
                "classify_coarse endpoint error {}: {}",
                status,
                body
            ));
        }

        let body: CoarseClassifyResponse = response
            .json()
            .await
            .context("failed to parse classify_coarse response")?;
        Ok(body.scores)
    }

    fn build_classify_url(&self) -> Result<Url> {
        self.base_url
            .join("v1/classify-runs")
            .context("failed to build classify-runs URL")
    }

    async fn send_classify_request(
        &self,
        job_id: Uuid,
        request: &ClassificationRequest,
        url: &Url,
        idempotency_key: Option<&str>,
    ) -> Result<Response> {
        info!(
            job_id = %job_id,
            text_count = request.texts.len(),
            url = %url,
            idempotency_key = idempotency_key,
            "sending classification job request"
        );

        let mut response = None;
        for attempt in 0..CLASSIFY_POST_RETRIES {
            let mut req = self
                .client
                .post(url.clone())
                .timeout(Duration::from_secs(30))
                .header("X-Alt-Job-Id", job_id.to_string());

            if let Some(key) = idempotency_key {
                req = req.header("Idempotency-Key", key);
            }

            let req = req.json(request);
            match req.send().await {
                Ok(res) => {
                    response = Some(res);
                    break;
                }
                Err(err) => {
                    let attempt_num = attempt + 1;
                    let backoff_ms = CLASSIFY_POST_BACKOFF_MS * (attempt_num as u64);
                    warn!(
                        job_id = %job_id,
                        attempt = attempt_num,
                        max_attempts = CLASSIFY_POST_RETRIES,
                        backoff_ms,
                        error = %err,
                        "classify-runs POST request failed, waiting before retry"
                    );
                    if attempt_num >= CLASSIFY_POST_RETRIES {
                        return Err(anyhow!(
                            "classify-runs POST request failed for job {} after {} attempts: {}",
                            job_id,
                            CLASSIFY_POST_RETRIES,
                            err
                        ));
                    }
                    sleep(Duration::from_millis(backoff_ms)).await;
                }
            }
        }

        response.with_context(|| format!("classify-runs POST request failed for job {}", job_id))
    }

    async fn parse_classify_response(
        &self,
        job_id: Uuid,
        response: Response,
    ) -> Result<ClassificationJobResponse> {
        let status = response.status();
        info!(
            job_id = %job_id,
            http_status = %status,
            "received classification job response"
        );

        if !status.is_success() {
            let body = response.text().await.unwrap_or_default();
            let truncated_body = truncate_error_message(&body);
            error!(
                job_id = %job_id,
                http_status = %status,
                error_body = %truncated_body,
                "classify-runs endpoint returned error status"
            );
            return Err(anyhow!(
                "classify-runs endpoint returned error status {}: {}",
                status,
                truncated_body
            ));
        }

        response
            .json()
            .await
            .context("failed to parse classify-runs response")
    }

    async fn process_classify_body(
        &self,
        job_id: Uuid,
        body: ClassificationJobResponse,
    ) -> Result<Vec<ClassificationResult>> {
        info!(
            job_id = %job_id,
            run_id = body.run_id,
            status = %body.status,
            "classification job created"
        );

        if body.status == "running" {
            info!(
                job_id = %job_id,
                run_id = body.run_id,
                "starting classification run polling"
            );
            let results = self.poll_classification_run(body.run_id).await?;
            info!(
                job_id = %job_id,
                run_id = body.run_id,
                result_count = results.len(),
                "classification run polling completed"
            );
            Ok(results)
        } else if body.status == "succeeded" {
            info!(
                job_id = %job_id,
                run_id = body.run_id,
                result_count = body.results.as_ref().map_or(0, Vec::len),
                "classification job already completed"
            );
            Ok(body.results.unwrap_or_default())
        } else {
            error!(
                job_id = %job_id,
                run_id = body.run_id,
                status = %body.status,
                error_message = %body.error_message.as_deref().unwrap_or(""),
                "classification run finished with non-success status"
            );
            Err(anyhow!(
                "classification run {} finished with status {}: {}",
                body.run_id,
                body.status,
                body.error_message.unwrap_or_default()
            ))
        }
    }

    async fn poll_classification_run(&self, run_id: i64) -> Result<Vec<ClassificationResult>> {
        let run_url = self
            .base_url
            .join(&format!("v1/classify-runs/{}", run_id))
            .with_context(|| format!("failed to build classify-run URL for run_id {}", run_id))?;

        info!(run_id, url = %run_url, "starting classification run polling");

        for attempt in 0..MAX_POLL_ATTEMPTS {
            Self::log_polling_attempt(run_id, attempt, &run_url);

            let response = self
                .send_poll_request_with_retry(run_id, &run_url, attempt)
                .await?;

            let status = response.status();
            if !status.is_success() {
                let body = response.text().await.unwrap_or_default();
                let truncated_body = truncate_error_message(&body);
                return Err(anyhow!(
                    "classification run polling endpoint returned error status {} for run_id {}: {}",
                    status,
                    run_id,
                    truncated_body
                ));
            }

            let body: ClassificationJobResponse = response.json().await.with_context(|| {
                format!(
                    "failed to deserialize polling response for run_id {}",
                    run_id
                )
            })?;

            if let Some(result) = Self::handle_classification_status(run_id, attempt, &body)? {
                return Ok(result);
            }

            Self::log_progress(run_id, attempt, &body.status);
            Self::sleep_with_backoff(attempt).await;
        }

        Err(anyhow!(
            "classification run {} did not complete within timeout ({} attempts)",
            run_id,
            MAX_POLL_ATTEMPTS
        ))
    }

    fn log_polling_attempt(run_id: i64, attempt: usize, run_url: &Url) {
        #[allow(clippy::manual_is_multiple_of)]
        if attempt == 0 || (attempt + 1) % 5 == 0 {
            info!(
                run_id,
                attempt = attempt + 1,
                max_attempts = MAX_POLL_ATTEMPTS,
                "polling classification run status"
            );
        } else {
            debug!(
                run_id,
                attempt = attempt + 1,
                url = %run_url,
                "sending classification run polling request"
            );
        }
    }

    fn handle_classification_status(
        run_id: i64,
        attempt: usize,
        body: &ClassificationJobResponse,
    ) -> Result<Option<Vec<ClassificationResult>>> {
        if body.status == "succeeded" {
            info!(
                run_id,
                attempt = attempt + 1,
                result_count = body.results.as_ref().map_or(0, Vec::len),
                "classification run completed successfully"
            );
            Ok(Some(body.results.clone().unwrap_or_default()))
        } else if body.status == "failed" {
            error!(
                run_id,
                attempt = attempt + 1,
                error_message = %body.error_message.as_deref().unwrap_or(""),
                "classification run failed"
            );
            Err(anyhow!(
                "classification run {} failed: {}",
                run_id,
                body.error_message.as_deref().unwrap_or("")
            ))
        } else {
            Ok(None)
        }
    }

    fn log_progress(run_id: i64, attempt: usize, status: &str) {
        #[allow(clippy::manual_is_multiple_of)]
        if (attempt + 1) % 5 == 0 {
            info!(
                run_id,
                attempt = attempt + 1,
                status = %status,
                "classification run still in progress"
            );
        } else {
            debug!(
                run_id,
                attempt,
                status = %status,
                "classification run still in progress"
            );
        }
    }

    async fn send_poll_request_with_retry(
        &self,
        run_id: i64,
        url: &Url,
        poll_attempt: usize,
    ) -> Result<Response> {
        let mut last_error = None;

        for retry in 0..POLL_REQUEST_RETRIES {
            match self.client.get(url.clone()).send().await {
                Ok(response) => return Ok(response),
                Err(e) => {
                    if e.is_timeout() || e.is_connect() {
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
                    return Err(anyhow::Error::from(e)).with_context(|| {
                        format!(
                            "classification run polling request failed for run_id {} (attempt {})",
                            run_id,
                            poll_attempt + 1
                        )
                    });
                }
            }
        }

        match last_error {
            Some(e) => Err(anyhow!(
                "classification run polling request failed after {} retries for run_id {} (attempt {}): {}",
                POLL_REQUEST_RETRIES,
                run_id,
                poll_attempt + 1,
                e
            )),
            None => Err(anyhow!(
                "classification run polling request failed after {} retries for run_id {} (attempt {})",
                POLL_REQUEST_RETRIES,
                run_id,
                poll_attempt + 1
            )),
        }
    }

    async fn sleep_with_backoff(attempt: usize) {
        let mut interval_ms = INITIAL_POLL_INTERVAL_MS;
        for _ in 0..attempt {
            interval_ms = std::cmp::min(interval_ms * 2, MAX_POLL_INTERVAL_MS);
        }
        sleep(Duration::from_millis(interval_ms)).await;
    }
}
