use anyhow::{Context, Result, anyhow};
use serde_json::Value;
use std::time::{Duration, Instant};
use uuid::Uuid;

use super::types::{
    ADMIN_JOB_INITIAL_BACKOFF_MS, ADMIN_JOB_MAX_BACKOFF_MS, ADMIN_JOB_TIMEOUT_SECS,
    AdminJobKickResponse, AdminJobStatusResponse,
};
use super::utils::truncate_error_message;
use crate::clients::subworker::SubworkerClient;

impl SubworkerClient {
    /// /admin/build-graphを呼び出してtag_label_graphを再構築
    #[allow(dead_code)]
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
    #[allow(dead_code)]
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
        self.kick_and_poll_admin_job("admin/graph-jobs")
            .await
            .context("graph job failed")?;
        self.kick_and_poll_admin_job("admin/learning-jobs")
            .await
            .context("learning job failed")?;
        tracing::info!("graph refresh and learning sequence completed via admin jobs");
        Ok(())
    }

    async fn kick_and_poll_admin_job(&self, endpoint: &str) -> Result<()> {
        let job_id = self
            .start_admin_job(endpoint)
            .await
            .with_context(|| format!("failed to start admin job at {}", endpoint))?;
        self.poll_admin_job(endpoint, job_id).await?;
        Ok(())
    }

    async fn start_admin_job(&self, endpoint: &str) -> Result<Uuid> {
        let url = self
            .base_url
            .join(endpoint)
            .with_context(|| format!("failed to build admin job URL for {}", endpoint))?;

        tracing::info!(endpoint = %url, "starting admin job");
        let response = self
            .client
            .post(url.clone())
            .send()
            .await
            .with_context(|| format!("admin job POST request failed for {}", endpoint))?;

        let status = response.status();
        if status != reqwest::StatusCode::ACCEPTED && !status.is_success() {
            let error_text = response
                .text()
                .await
                .unwrap_or_else(|_| "unknown error".to_string());
            return Err(anyhow!(
                "admin job endpoint {} returned error status {}: {}",
                endpoint,
                status,
                truncate_error_message(&error_text)
            ));
        }

        let body: AdminJobKickResponse = response
            .json()
            .await
            .with_context(|| format!("failed to parse admin job kick response for {}", endpoint))?;
        tracing::info!(
            job_id = %body.job_id,
            endpoint,
            "admin job started, beginning polling"
        );
        Ok(body.job_id)
    }

    async fn poll_admin_job(&self, endpoint: &str, job_id: Uuid) -> Result<()> {
        let mut backoff = Duration::from_millis(ADMIN_JOB_INITIAL_BACKOFF_MS);
        let deadline = Instant::now() + Duration::from_secs(ADMIN_JOB_TIMEOUT_SECS);
        let url = self
            .base_url
            .join(&format!("{}/{}", endpoint, job_id))
            .with_context(|| format!("failed to build admin job status URL for {}", job_id))?;

        tracing::info!(
            job_id = %job_id,
            endpoint = %url,
            "starting admin job polling"
        );

        let mut poll_count = 0u32;
        loop {
            if Instant::now() > deadline {
                return Err(anyhow!(
                    "admin job {} at {} timed out after {:?}",
                    job_id,
                    endpoint,
                    Duration::from_secs(ADMIN_JOB_TIMEOUT_SECS)
                ));
            }

            let response = self.client.get(url.clone()).send().await.with_context(|| {
                format!(
                    "admin job polling request failed for job_id {} at {}",
                    job_id, endpoint
                )
            })?;

            let status = response.status();
            if !status.is_success() {
                let body = response.text().await.unwrap_or_default();
                return Err(anyhow!(
                    "admin job polling endpoint returned error status {} for job_id {}: {}",
                    status,
                    job_id,
                    truncate_error_message(&body)
                ));
            }

            let body: AdminJobStatusResponse = response.json().await.with_context(|| {
                format!(
                    "failed to deserialize admin job status for job_id {}",
                    job_id
                )
            })?;

            poll_count += 1;
            let elapsed = deadline.saturating_duration_since(Instant::now());

            match body.status.as_str() {
                "succeeded" | "partial" => {
                    tracing::info!(
                        job_id = %job_id,
                        endpoint,
                        status = %body.status,
                        poll_count,
                        "admin job completed successfully"
                    );
                    return Ok(());
                }
                "failed" => {
                    return Err(anyhow!(
                        "admin job {} failed: {}",
                        job_id,
                        body.error.unwrap_or_else(|| "unknown error".to_string())
                    ));
                }
                _ => {
                    if poll_count.is_multiple_of(10) {
                        tracing::info!(
                            job_id = %job_id,
                            endpoint,
                            status = %body.status,
                            poll_count,
                            elapsed_seconds = elapsed.as_secs(),
                            backoff_ms = backoff.as_millis(),
                            "admin job still running"
                        );
                    } else {
                        tracing::debug!(
                            job_id = %job_id,
                            status = %body.status,
                            poll_count,
                            backoff_ms = backoff.as_millis(),
                            "admin job still running"
                        );
                    }
                    tokio::time::sleep(backoff).await;
                    backoff =
                        std::cmp::min(backoff * 2, Duration::from_millis(ADMIN_JOB_MAX_BACKOFF_MS));
                }
            }
        }
    }
}
