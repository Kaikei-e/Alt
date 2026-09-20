use std::collections::HashMap;
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::Duration;

use bollard::Docker;
use bollard::models::ContainerSummary;
use bollard::query_parameters::LogsOptions;
use bytes::Bytes;
use chrono::{DateTime, Utc};
use futures::StreamExt;
use thiserror::Error;
use tokio::sync::broadcast::Sender as BroadcastSender;

#[derive(Error, Debug, PartialEq, Eq, Clone)]
pub enum DockerEndpointError {
    #[error(
        "DOCKER_HOST environment variable must be set (e.g. tcp://docker-socket-proxy-ro:2375 \
         or unix:///var/run/docker.sock)"
    )]
    MissingDockerHost,
    #[error("DOCKER_HOST is empty")]
    EmptyDockerHost,
    #[error(
        "unsupported Docker endpoint scheme in '{0}'; only tcp://, unix://, http://, and \
         https:// are supported"
    )]
    UnsupportedScheme(String),
    #[error("invalid Docker endpoint URL '{0}': {1}")]
    InvalidUrl(String, String),
}

#[derive(Error, Debug)]
pub enum DockerConnectError {
    #[error("Docker endpoint configuration error: {0}")]
    Endpoint(#[from] DockerEndpointError),
    #[error("Docker connection error: {0}")]
    Bollard(#[from] bollard::errors::Error),
}

/// Validates a raw Docker endpoint URL string.
/// Supported schemes: `tcp://`, `unix://`, `http://`, and `https://`.
pub fn validate_docker_endpoint(raw: &str) -> Result<&str, DockerEndpointError> {
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        return Err(DockerEndpointError::EmptyDockerHost);
    }
    if !(trimmed.starts_with("unix://")
        || trimmed.starts_with("tcp://")
        || trimmed.starts_with("http://")
        || trimmed.starts_with("https://"))
    {
        return Err(DockerEndpointError::UnsupportedScheme(trimmed.to_string()));
    }
    let after_scheme = trimmed
        .split_once("://")
        .map(|(_, rest)| rest)
        .unwrap_or_default();
    if after_scheme.trim().is_empty() || after_scheme.starts_with(':') {
        return Err(DockerEndpointError::InvalidUrl(
            trimmed.to_string(),
            "missing host or path".to_string(),
        ));
    }
    Ok(trimmed)
}

/// Reads and validates `DOCKER_HOST` from the environment.
/// Fails fast if unset, empty, or using an unsupported scheme.
pub fn get_validated_docker_endpoint() -> Result<String, DockerEndpointError> {
    match std::env::var("DOCKER_HOST") {
        Ok(val) => validate_docker_endpoint(&val).map(str::to_string),
        Err(_) => Err(DockerEndpointError::MissingDockerHost),
    }
}

/// Connects to the Docker daemon using configuration from `DOCKER_HOST`.
///
/// Logs `docker_endpoint=<value>` at startup. Fails startup loudly when `DOCKER_HOST`
/// is unset or has an unsupported scheme, avoiding silent fallback to platform defaults.
pub fn connect_docker() -> Result<Docker, DockerConnectError> {
    let endpoint = get_validated_docker_endpoint().map_err(|e| {
        tracing::error!(
            error = %e,
            "Failed to initialize Docker client: invalid or missing DOCKER_HOST"
        );
        e
    })?;

    static ENDPOINT_LOGGED: AtomicBool = AtomicBool::new(false);
    if !ENDPOINT_LOGGED.swap(true, Ordering::Relaxed) {
        tracing::info!(docker_endpoint = %endpoint, "Connecting to Docker daemon");
    } else {
        tracing::debug!(docker_endpoint = %endpoint, "Reconnecting to Docker daemon");
    }

    // connect_with_host dispatches tcp:///http:// (or TLS if DOCKER_TLS_VERIFY is set) and unix://
    let docker = Docker::connect_with_host(&endpoint).map_err(|e| {
        tracing::error!(
            error = %e,
            docker_endpoint = %endpoint,
            "Failed to connect to Docker daemon with endpoint"
        );
        e
    })?;

    Ok(docker)
}

#[derive(Error, Debug)]
pub enum CollectorError {
    #[error("Docker endpoint configuration error: {0}")]
    EndpointError(#[from] DockerEndpointError),
    #[error("Docker connection failed: {0}")]
    ConnectionFailed(#[from] bollard::errors::Error),
    #[error("Container discovery failed: {0}")]
    DiscoveryFailed(String),
}

impl From<DockerConnectError> for CollectorError {
    fn from(err: DockerConnectError) -> Self {
        match err {
            DockerConnectError::Endpoint(e) => Self::EndpointError(e),
            DockerConnectError::Bollard(e) => Self::ConnectionFailed(e),
        }
    }
}

#[derive(Debug, Clone)]
pub struct DockerContainerInfo {
    pub id: String,
    pub name: String,
    pub image: String,
    pub labels: HashMap<String, String>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct LogStreamOptions {
    pub follow: bool,
    pub stdout: bool,
    pub stderr: bool,
    pub timestamps: bool,
    pub tail: String,
    pub since: i32,
}

impl Default for LogStreamOptions {
    fn default() -> Self {
        Self {
            follow: true,
            stdout: true,
            stderr: true,
            timestamps: false,
            tail: "0".to_string(), // All logs
            since: 0,
        }
    }
}

/// Computes resume log stream options for Docker container logs.
///
/// - On initial connection (`last_timestamp == None`), tails 0 lines (`tail: "0"`, `since: 0`)
///   with `timestamps: true` so that historical logs are not replayed on startup while enabling
///   per-line timestamp extraction.
/// - On reconnect (`last_timestamp == Some(ts)`), fetches all logs since floor second of `ts`
///   (`tail: "all"`, `since: ts.timestamp() as i32`) with `timestamps: true`. This prevents log
///   loss during proxy disconnects or backoff windows, while boundary-second duplicate lines
///   at or before `ts` are deduplicated and dropped by [`DedupeFilter`] before forwarding.
pub fn compute_resume_log_options(last_timestamp: Option<DateTime<Utc>>) -> LogStreamOptions {
    match last_timestamp {
        Some(ts) => LogStreamOptions {
            follow: true,
            stdout: true,
            stderr: true,
            timestamps: true,
            tail: "all".to_string(),
            since: ts.timestamp() as i32,
        },
        None => LogStreamOptions {
            follow: true,
            stdout: true,
            stderr: true,
            timestamps: true,
            tail: "0".to_string(),
            since: 0,
        },
    }
}

fn parse_line_timestamp_prefix(line: &[u8]) -> (Option<DateTime<Utc>>, usize) {
    if let Some(space_pos) = line.iter().position(|&b| b == b' ')
        && let Ok(prefix_str) = std::str::from_utf8(&line[..space_pos])
        && let Ok(parsed) = DateTime::parse_from_rfc3339(prefix_str)
    {
        (Some(parsed.with_timezone(&Utc)), space_pos + 1)
    } else {
        (None, 0)
    }
}

/// Extracts the RFC3339 timestamp prefix from a raw Docker log line if present,
/// returning the parsed UTC timestamp and the remaining log payload with the prefix
/// and following space stripped.
///
/// For multi-line frames (e.g. from TTY-attached containers), lines are parsed individually,
/// the last parsed timestamp is returned as the frame timestamp, and all prefixes are stripped
/// while preserving original line terminators.
///
/// If the prefix is missing or malformed, returns `(None, raw)` unchanged.
pub fn parse_log_timestamp_prefix(raw: Bytes) -> (Option<DateTime<Utc>>, Bytes) {
    let is_single_line = match raw.iter().position(|&b| b == b'\n') {
        Some(pos) => pos == raw.len().saturating_sub(1),
        None => true,
    };

    if is_single_line {
        let (ts, offset) = parse_line_timestamp_prefix(&raw);
        let payload = if offset > 0 { raw.slice(offset..) } else { raw };
        return (ts, payload);
    }

    let mut last_ts = None;
    let mut output = Vec::with_capacity(raw.len());
    let mut remainder: &[u8] = &raw;

    while !remainder.is_empty() {
        let (line_with_term, next_remainder) = match remainder.iter().position(|&b| b == b'\n') {
            Some(pos) => (&remainder[..=pos], &remainder[pos + 1..]),
            None => (remainder, &[][..]),
        };
        remainder = next_remainder;

        let has_newline = line_with_term.ends_with(b"\n");
        let line_content = if has_newline {
            &line_with_term[..line_with_term.len() - 1]
        } else {
            line_with_term
        };

        let (ts, offset) = parse_line_timestamp_prefix(line_content);
        if let Some(parsed) = ts {
            last_ts = Some(parsed);
        }
        output.extend_from_slice(&line_content[offset..]);
        if has_newline {
            output.push(b'\n');
        }
    }

    (last_ts, Bytes::from(output))
}

/// Filter state for deduplicating boundary-second logs upon stream reconnection.
#[derive(Debug, Clone)]
pub struct DedupeFilter {
    threshold: Option<DateTime<Utc>>,
    active: bool,
}

impl DedupeFilter {
    pub fn new(last_timestamp: Option<DateTime<Utc>>) -> Self {
        Self {
            threshold: last_timestamp,
            active: last_timestamp.is_some(),
        }
    }

    /// Determines whether an incoming log line should be dropped.
    ///
    /// If deduplication is active and `timestamp <= threshold`, returns `true` (drop).
    /// Once a timestamp `> threshold` is encountered, deactivates deduplication and returns `false` (keep).
    /// If no timestamp could be parsed (`None`), returns `false` (keep).
    pub fn should_drop(&mut self, timestamp: Option<DateTime<Utc>>) -> bool {
        if !self.active {
            return false;
        }
        match (timestamp, self.threshold) {
            (Some(ts), Some(thresh)) => {
                if ts <= thresh {
                    true
                } else {
                    self.active = false;
                    false
                }
            }
            _ => false,
        }
    }

    pub fn is_active(&self) -> bool {
        self.active
    }
}

pub struct DockerCollector {
    docker: Docker,
}

impl DockerCollector {
    /// Expose the Docker client for reuse by other components.
    pub fn docker(&self) -> &Docker {
        &self.docker
    }
}

impl DockerCollector {
    pub async fn new() -> Result<Self, CollectorError> {
        let docker = connect_docker()?;
        Ok(Self { docker })
    }

    pub async fn can_connect(&self) -> bool {
        self.docker.ping().await.is_ok()
    }

    pub async fn find_labeled_containers(
        &self,
        label_filter: &str,
    ) -> Result<Vec<DockerContainerInfo>, CollectorError> {
        let filters = HashMap::from([("label".to_string(), vec![label_filter.to_string()])]);

        let options = Some(bollard::query_parameters::ListContainersOptions {
            all: false, // Only running containers
            filters: Some(filters),
            ..Default::default()
        });

        let containers = self
            .docker
            .list_containers(options)
            .await
            .map_err(|e| CollectorError::DiscoveryFailed(e.to_string()))?;

        let container_infos = containers
            .into_iter()
            .filter_map(|container| self.container_to_info(container))
            .collect();

        Ok(container_infos)
    }

    fn container_to_info(&self, container: ContainerSummary) -> Option<DockerContainerInfo> {
        Some(DockerContainerInfo {
            id: container.id?,
            name: container
                .names?
                .first()?
                .trim_start_matches('/')
                .to_string(),
            image: container.image?,
            labels: container.labels.unwrap_or_default(),
        })
    }

    pub async fn start_tailing_logs(
        &self,
        tx: BroadcastSender<Bytes>,
        label_filter: &str,
    ) -> Result<(), CollectorError> {
        self.start_tailing_logs_with_options(tx, label_filter, LogStreamOptions::default())
            .await
    }

    pub async fn start_tailing_logs_with_options(
        &self,
        tx: BroadcastSender<Bytes>,
        label_filter: &str,
        options: LogStreamOptions,
    ) -> Result<(), CollectorError> {
        let containers = self.find_labeled_containers(label_filter).await?;

        if containers.is_empty() {
            tracing::warn!("No containers found with label: {}", label_filter);
            return Ok(());
        }

        let log_options = LogsOptions {
            follow: options.follow,
            stdout: options.stdout,
            stderr: options.stderr,
            since: options.since,
            timestamps: options.timestamps,
            tail: options.tail,
            ..Default::default()
        };

        for container in containers {
            let docker = self.docker.clone();
            let tx = tx.clone();
            let container_id = container.id.clone();
            let container_name = container.name.clone();
            let log_options = log_options.clone();

            tokio::spawn(async move {
                tracing::info!(
                    "Starting to tail logs for container: {} ({})",
                    container_name,
                    container_id
                );

                let mut stream = docker.logs(&container_id, Some(log_options));

                while let Some(chunk_result) = stream.next().await {
                    match chunk_result {
                        Ok(chunk) => {
                            // Zero-copy: chunk.into_bytes() returns Bytes directly
                            let bytes = chunk.into_bytes();

                            if tx.send(bytes).is_err() {
                                // A broadcast send only errs when there are no
                                // active receivers - it never blocks or fails
                                // on a "full" queue (lagging receivers instead
                                // silently miss old values). This is not
                                // backpressure; retry after a short wait in
                                // case a receiver (re)subscribes.
                                tracing::warn!(
                                    "No active receivers for container {} logs, retrying",
                                    container_name
                                );
                                tokio::time::sleep(Duration::from_micros(100)).await;
                            }
                        }
                        Err(e) => {
                            tracing::error!(
                                "Error reading logs from container {}: {}",
                                container_name,
                                e
                            );
                            break;
                        }
                    }
                }

                tracing::info!("Stopped tailing logs for container: {}", container_name);
            });
        }

        Ok(())
    }
}
