use anyhow::{Context, Result, anyhow};
use serde_json::Value;
use std::time::{Duration, Instant};
use uuid::Uuid;

use super::types::{
    ADMIN_JOB_INITIAL_BACKOFF_MS, ADMIN_JOB_MAX_BACKOFF_MS, ADMIN_JOB_TIMEOUT_SECS,
    AdminJobKickResponse, AdminJobStatusResponse, POLL_REQUEST_TIMEOUT_SECS, SubworkerAuthError,
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
            if SubworkerAuthError::is_auth_status(status) {
                return Err(
                    SubworkerAuthError::new("admin/build-graph", status, &error_text).into(),
                );
            }
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
            if SubworkerAuthError::is_auth_status(status) {
                return Err(SubworkerAuthError::new("admin/learning", status, &error_text).into());
            }
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
            if SubworkerAuthError::is_auth_status(status) {
                return Err(SubworkerAuthError::new("admin/jobs", status, &error_text).into());
            }
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

            let body = self.fetch_admin_job_status(&url, endpoint, job_id).await?;
            poll_count += 1;
            let progress = AdminPollProgress {
                poll_count,
                elapsed: deadline.saturating_duration_since(Instant::now()),
                backoff,
            };

            match handle_admin_job_status(endpoint, job_id, body, progress)? {
                AdminPollDecision::Completed => return Ok(()),
                AdminPollDecision::StillRunning => {
                    tokio::time::sleep(backoff).await;
                    backoff =
                        std::cmp::min(backoff * 2, Duration::from_millis(ADMIN_JOB_MAX_BACKOFF_MS));
                }
            }
        }
    }

    async fn fetch_admin_job_status(
        &self,
        url: &reqwest::Url,
        endpoint: &str,
        job_id: Uuid,
    ) -> Result<AdminJobStatusResponse> {
        let response = self
            .client
            .get(url.clone())
            .timeout(Duration::from_secs(POLL_REQUEST_TIMEOUT_SECS))
            .send()
            .await
            .with_context(|| {
                format!(
                    "admin job polling request failed for job_id {} at {}",
                    job_id, endpoint
                )
            })?;

        let status = response.status();
        if !status.is_success() {
            let body = response.text().await.unwrap_or_default();
            if SubworkerAuthError::is_auth_status(status) {
                return Err(SubworkerAuthError::new("admin/jobs poll", status, &body).into());
            }
            return Err(anyhow!(
                "admin job polling endpoint returned error status {} for job_id {}: {}",
                status,
                job_id,
                truncate_error_message(&body)
            ));
        }

        response.json().await.with_context(|| {
            format!(
                "failed to deserialize admin job status for job_id {}",
                job_id
            )
        })
    }
}

enum AdminPollDecision {
    Completed,
    StillRunning,
}

struct AdminPollProgress {
    poll_count: u32,
    elapsed: Duration,
    backoff: Duration,
}

fn handle_admin_job_status(
    endpoint: &str,
    job_id: Uuid,
    body: AdminJobStatusResponse,
    progress: AdminPollProgress,
) -> Result<AdminPollDecision> {
    match body.status.as_str() {
        "succeeded" | "partial" => {
            tracing::info!(
                job_id = %job_id,
                endpoint,
                status = %body.status,
                poll_count = progress.poll_count,
                "admin job completed successfully"
            );
            Ok(AdminPollDecision::Completed)
        }
        "failed" => Err(anyhow!(
            "admin job {} failed: {}",
            job_id,
            body.error.unwrap_or_else(|| "unknown error".to_string())
        )),
        _ => {
            log_admin_poll_running(endpoint, job_id, &body.status, &progress);
            Ok(AdminPollDecision::StillRunning)
        }
    }
}

fn log_admin_poll_running(
    endpoint: &str,
    job_id: Uuid,
    status: &str,
    progress: &AdminPollProgress,
) {
    if progress.poll_count.is_multiple_of(10) {
        tracing::info!(
            job_id = %job_id,
            endpoint,
            status = %status,
            poll_count = progress.poll_count,
            elapsed_seconds = progress.elapsed.as_secs(),
            backoff_ms = progress.backoff.as_millis(),
            "admin job still running"
        );
    } else {
        tracing::debug!(
            job_id = %job_id,
            status = %status,
            poll_count = progress.poll_count,
            backoff_ms = progress.backoff.as_millis(),
            "admin job still running"
        );
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::clients::subworker::is_subworker_auth_error;
    use wiremock::matchers::{header, method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    #[tokio::test]
    async fn poll_admin_job_sends_get_with_auth_and_succeeds_immediately() {
        let server = MockServer::start().await;
        let job_id = Uuid::new_v4();
        let expected_path = format!("/admin/graph-jobs/{job_id}");

        Mock::given(method("GET"))
            .and(path(expected_path))
            .and(header("authorization", "Bearer test-admin-token"))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
                "job_id": job_id,
                "kind": "graph",
                "status": "succeeded",
            })))
            .expect(1)
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri(), 10, Some("test-admin-token".to_string()))
            .expect("client should build");

        client
            .poll_admin_job("admin/graph-jobs", job_id)
            .await
            .expect("succeeded status should complete Ok");
        server.verify().await;
    }

    #[tokio::test]
    async fn poll_admin_job_accepts_partial_status() {
        let server = MockServer::start().await;
        let job_id = Uuid::new_v4();

        Mock::given(method("GET"))
            .and(path(format!("/admin/learning-jobs/{job_id}")))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
                "job_id": job_id,
                "kind": "learning",
                "status": "partial",
            })))
            .expect(1)
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri(), 10, None).expect("client should build");
        client
            .poll_admin_job("admin/learning-jobs", job_id)
            .await
            .expect("partial status should succeed");
        server.verify().await;
    }

    #[tokio::test]
    async fn poll_admin_job_returns_error_on_failed_status_with_and_without_message() {
        let server = MockServer::start().await;
        let job_with_msg = Uuid::new_v4();
        let job_without_msg = Uuid::new_v4();

        Mock::given(method("GET"))
            .and(path(format!("/admin/graph-jobs/{job_with_msg}")))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
                "job_id": job_with_msg,
                "kind": "graph",
                "status": "failed",
                "error": "graph build boom",
            })))
            .expect(1)
            .mount(&server)
            .await;

        Mock::given(method("GET"))
            .and(path(format!("/admin/graph-jobs/{job_without_msg}")))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
                "job_id": job_without_msg,
                "kind": "graph",
                "status": "failed",
            })))
            .expect(1)
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri(), 10, None).expect("client should build");

        let err_with = client
            .poll_admin_job("admin/graph-jobs", job_with_msg)
            .await
            .expect_err("failed status must error");
        assert!(err_with.to_string().contains("failed: graph build boom"));

        let err_without = client
            .poll_admin_job("admin/graph-jobs", job_without_msg)
            .await
            .expect_err("failed status without message must error");
        assert!(err_without.to_string().contains("failed: unknown error"));
        server.verify().await;
    }

    #[tokio::test]
    async fn poll_admin_job_returns_auth_error_on_401_and_403() {
        let server = MockServer::start().await;
        let job_401 = Uuid::new_v4();
        let job_403 = Uuid::new_v4();

        Mock::given(method("GET"))
            .and(path(format!("/admin/graph-jobs/{job_401}")))
            .respond_with(ResponseTemplate::new(401).set_body_string("unauthorized"))
            .expect(1)
            .mount(&server)
            .await;
        Mock::given(method("GET"))
            .and(path(format!("/admin/graph-jobs/{job_403}")))
            .respond_with(ResponseTemplate::new(403).set_body_string("forbidden"))
            .expect(1)
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri(), 10, None).expect("client should build");

        let err_401 = client
            .poll_admin_job("admin/graph-jobs", job_401)
            .await
            .expect_err("401 must fail");
        assert!(is_subworker_auth_error(&err_401));
        assert!(format!("{err_401:#}").contains("401 Unauthorized"));

        let err_403 = client
            .poll_admin_job("admin/graph-jobs", job_403)
            .await
            .expect_err("403 must fail");
        assert!(is_subworker_auth_error(&err_403));
        assert!(format!("{err_403:#}").contains("403 Forbidden"));
        server.verify().await;
    }

    #[tokio::test]
    async fn poll_admin_job_returns_error_on_404_and_500_with_truncation() {
        let server = MockServer::start().await;
        let job_404 = Uuid::new_v4();
        let job_500 = Uuid::new_v4();
        let long_body = "x".repeat(600);

        Mock::given(method("GET"))
            .and(path(format!("/admin/graph-jobs/{job_404}")))
            .respond_with(ResponseTemplate::new(404).set_body_string("missing job"))
            .expect(1)
            .mount(&server)
            .await;
        Mock::given(method("GET"))
            .and(path(format!("/admin/graph-jobs/{job_500}")))
            .respond_with(ResponseTemplate::new(500).set_body_string(long_body))
            .expect(1)
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri(), 10, None).expect("client should build");

        let err_404 = client
            .poll_admin_job("admin/graph-jobs", job_404)
            .await
            .expect_err("404 must fail");
        assert!(!is_subworker_auth_error(&err_404));
        assert!(err_404.to_string().contains("404 Not Found"));

        let err_500 = client
            .poll_admin_job("admin/graph-jobs", job_500)
            .await
            .expect_err("500 must fail");
        assert!(!is_subworker_auth_error(&err_500));
        let msg_500 = err_500.to_string();
        assert!(msg_500.contains("500 Internal Server Error"));
        assert!(msg_500.contains("truncated, 600 chars"));
        server.verify().await;
    }

    #[tokio::test]
    async fn poll_admin_job_rejects_invalid_json_200_body() {
        let server = MockServer::start().await;
        let job_id = Uuid::new_v4();

        Mock::given(method("GET"))
            .and(path(format!("/admin/graph-jobs/{job_id}")))
            .respond_with(ResponseTemplate::new(200).set_body_string("not-json"))
            .expect(1)
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri(), 10, None).expect("client should build");
        let err = client
            .poll_admin_job("admin/graph-jobs", job_id)
            .await
            .expect_err("invalid json must fail");
        assert!(
            err.to_string()
                .contains("failed to deserialize admin job status")
        );
        server.verify().await;
    }

    #[tokio::test]
    async fn poll_admin_job_propagates_transport_error_on_unreachable_endpoint() {
        let listener = std::net::TcpListener::bind("127.0.0.1:0").expect("bind ephemeral port");
        let addr = listener.local_addr().expect("local addr");
        drop(listener);

        let client =
            SubworkerClient::new(format!("http://{addr}/"), 10, None).expect("client should build");
        let job_id = Uuid::new_v4();
        let err = client
            .poll_admin_job("admin/graph-jobs", job_id)
            .await
            .expect_err("unreachable endpoint must fail");
        assert!(err.to_string().contains("admin job polling request failed"));
    }
}
