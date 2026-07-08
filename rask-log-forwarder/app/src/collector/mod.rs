pub mod discovery;
pub mod docker;

use std::time::Duration;
use thiserror::Error;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;
// Ensure zero-copy processing by using Bytes throughout
use bytes::Bytes;

/// Initial delay before the first reconnect attempt after a Docker log
/// stream error, EOF (container restart), or discovery failure.
const INITIAL_RECONNECT_DELAY: Duration = Duration::from_millis(500);
/// Upper bound on the reconnect backoff so a persistently-missing container
/// doesn't grow the retry interval without limit.
const MAX_RECONNECT_DELAY: Duration = Duration::from_secs(30);

/// Doubles `current`, capped at `MAX_RECONNECT_DELAY`.
fn next_reconnect_delay(current: Duration) -> Duration {
    (current * 2).min(MAX_RECONNECT_DELAY)
}

/// Outcome of a single streaming attempt, distinguishing a deliberate
/// shutdown from the stream simply ending (e.g. the container stopped),
/// so the reconnect loop knows whether to give up or try again.
#[derive(Debug, PartialEq, Eq)]
enum StreamExit {
    Cancelled,
    StreamEnded,
}

pub type LogBytes = Bytes;
pub use discovery::{ContainerInfo, DiscoveryError, ServiceDiscovery, ServiceDiscoveryTrait};
pub use docker::{
    CollectorError as DockerError, ContainerInfo as DockerContainerInfo, DockerCollector,
    LogStreamOptions,
};

#[derive(Error, Debug)]
pub enum CollectorError {
    #[error("Discovery error: {0}")]
    DiscoveryError(#[from] discovery::DiscoveryError),
    #[error("Docker error: {0}")]
    DockerError(#[from] docker::CollectorError),
    #[error("Collection stopped")]
    CollectionStopped,
}

/// Raw collected log line. `log`/`stream`/`time` are deliberately absent:
/// they get re-derived from `raw_bytes` by the downstream Docker JSON parser,
/// so allocating placeholder values for them here would be pure waste on the
/// per-line hot path.
#[derive(Debug, Clone)]
pub struct LogEntry {
    pub id: String,
    pub container_id: String,
    pub raw_bytes: Bytes,
}

#[derive(Debug)]
pub struct CollectorConfig {
    pub auto_discover: bool,
    pub target_service: Option<String>,
    pub follow_rotations: bool,
    pub buffer_size: usize,
}

impl Default for CollectorConfig {
    fn default() -> Self {
        Self {
            auto_discover: true,
            target_service: None,
            follow_rotations: true,
            buffer_size: 8192,
        }
    }
}

#[derive(Debug)]
pub struct LogCollector {
    #[allow(dead_code)]
    config: CollectorConfig,
    discovery: discovery::ServiceDiscovery,
    container_info: Option<discovery::ContainerInfo>,
    target_service: String,
}

impl LogCollector {
    pub async fn new(config: CollectorConfig) -> Result<Self, CollectorError> {
        let discovery = discovery::ServiceDiscovery::new().await?;

        let target_service = if let Some(ref service) = config.target_service {
            service.clone()
        } else if config.auto_discover {
            discovery.get_target_service()?
        } else {
            return Err(CollectorError::DiscoveryError(
                discovery::DiscoveryError::NoTargetService,
            ));
        };

        Ok(Self {
            config,
            discovery,
            container_info: None,
            target_service,
        })
    }

    pub fn get_target_service(&self) -> &str {
        &self.target_service
    }

    pub async fn start_collection(
        &mut self,
        tx: mpsc::Sender<LogEntry>,
        cancel_token: CancellationToken,
    ) -> Result<(), CollectorError> {
        let mut reconnect_delay = INITIAL_RECONNECT_DELAY;

        loop {
            if cancel_token.is_cancelled() {
                return Ok(());
            }

            // (Re)discover the container on every attempt: a restarted
            // container commonly gets a new container ID, so re-resolving by
            // service name (rather than reusing a stale ID) is required for
            // reconnection to actually find it.
            let container_info = match self
                .discovery
                .find_container_by_service(&self.target_service)
                .await
            {
                Ok(info) => info,
                Err(e) => {
                    tracing::warn!(
                        "Container discovery failed for service '{}': {e}; retrying in {:?}",
                        self.target_service,
                        reconnect_delay
                    );
                    if Self::sleep_or_cancelled(reconnect_delay, &cancel_token).await {
                        return Ok(());
                    }
                    reconnect_delay = next_reconnect_delay(reconnect_delay);
                    continue;
                }
            };
            self.container_info = Some(container_info.clone());

            let docker_collector = match DockerCollector::new().await {
                Ok(c) => c,
                Err(e) => {
                    tracing::warn!(
                        "Failed to create Docker client for service '{}': {e}; retrying in {:?}",
                        self.target_service,
                        reconnect_delay
                    );
                    if Self::sleep_or_cancelled(reconnect_delay, &cancel_token).await {
                        return Ok(());
                    }
                    reconnect_delay = next_reconnect_delay(reconnect_delay);
                    continue;
                }
            };

            tracing::info!(
                "Starting log collection for service '{}' (container: {})",
                self.target_service,
                container_info.id
            );

            match self
                .start_docker_api_streaming(
                    docker_collector,
                    &container_info.id,
                    tx.clone(),
                    cancel_token.clone(),
                )
                .await
            {
                Ok(StreamExit::Cancelled) => return Ok(()),
                Ok(StreamExit::StreamEnded) => {
                    tracing::warn!(
                        "Docker log stream for service '{}' (container: {}) ended - \
                         reconnecting in {:?} (container likely restarted)",
                        self.target_service,
                        container_info.id,
                        reconnect_delay
                    );
                }
                Err(CollectorError::CollectionStopped) => {
                    // The receiver was dropped (pipeline shut down); no point retrying.
                    return Err(CollectorError::CollectionStopped);
                }
                Err(e) => {
                    tracing::warn!(
                        "Docker log stream error for service '{}' (container: {}): {e}; \
                         reconnecting in {:?}",
                        self.target_service,
                        container_info.id,
                        reconnect_delay
                    );
                }
            }

            if Self::sleep_or_cancelled(reconnect_delay, &cancel_token).await {
                return Ok(());
            }
            reconnect_delay = next_reconnect_delay(reconnect_delay);
        }
    }

    /// Sleeps for `delay`, returning early (with `true`) if cancelled first.
    async fn sleep_or_cancelled(delay: Duration, cancel_token: &CancellationToken) -> bool {
        tokio::select! {
            _ = cancel_token.cancelled() => true,
            _ = tokio::time::sleep(delay) => false,
        }
    }

    async fn start_docker_api_streaming(
        &self,
        docker_collector: DockerCollector,
        container_id: &str,
        tx: mpsc::Sender<LogEntry>,
        cancel_token: CancellationToken,
    ) -> Result<StreamExit, CollectorError> {
        use bollard::query_parameters::LogsOptions;
        use futures::StreamExt;

        // Reuse the DockerCollector's existing Docker client
        let docker = docker_collector.docker();

        // IMPORTANT: Set timestamps to false to avoid Docker adding timestamps to log messages
        let options = LogsOptions {
            follow: true,
            stdout: true,
            stderr: true,
            timestamps: false, // Changed from true to false
            // "0": stream new lines only. "all" would re-send the container's
            // entire log history on every forwarder restart/reconnect.
            tail: "0".to_string(),
            ..Default::default()
        };

        let mut stream = docker.logs(container_id, Some(options));

        loop {
            tokio::select! {
                // Check for cancellation signal first
                _ = cancel_token.cancelled() => {
                    tracing::info!("Collector received cancellation signal, stopping log collection");
                    return Ok(StreamExit::Cancelled);
                }
                // Process log stream
                log_output = stream.next() => {
                    match log_output {
                        Some(Ok(log_chunk)) => {
                            // Convert Docker log output to LogEntry
                            let log_bytes = log_chunk.into_bytes();

                            // Create LogEntry with raw bytes - let the parser handle the actual parsing
                            let entry = LogEntry {
                                id: container_id.to_string(),
                                container_id: container_id.to_string(),
                                raw_bytes: log_bytes,
                            };

                            // Bounded channel: block (apply backpressure) rather than
                            // drop when the batching/send side can't keep up, while
                            // staying responsive to cancellation in the meantime.
                            tokio::select! {
                                _ = cancel_token.cancelled() => {
                                    tracing::info!("Collector received cancellation signal while backpressured, stopping log collection");
                                    return Ok(StreamExit::Cancelled);
                                }
                                send_result = tx.send(entry) => {
                                    if send_result.is_err() {
                                        return Err(CollectorError::CollectionStopped);
                                    }
                                }
                            }
                        }
                        Some(Err(e)) => {
                            tracing::error!("Docker log stream error: {e}");
                            return Err(CollectorError::DiscoveryError(
                                discovery::DiscoveryError::DockerError(e),
                            ));
                        }
                        None => {
                            // Stream ended (e.g. the container stopped/restarted).
                            return Ok(StreamExit::StreamEnded);
                        }
                    }
                }
            }
        }
    }

    pub fn get_container_info(&self) -> Option<&discovery::ContainerInfo> {
        self.container_info.as_ref()
    }
}

#[cfg(test)]
mod reconnect_tests {
    use super::*;

    #[test]
    fn backoff_doubles_up_to_the_cap() {
        let mut delay = INITIAL_RECONNECT_DELAY;
        assert_eq!(delay, Duration::from_millis(500));

        delay = next_reconnect_delay(delay);
        assert_eq!(delay, Duration::from_millis(1000));

        delay = next_reconnect_delay(delay);
        assert_eq!(delay, Duration::from_millis(2000));

        // Keep doubling well past the cap and confirm it never exceeds it.
        for _ in 0..10 {
            delay = next_reconnect_delay(delay);
            assert!(
                delay <= MAX_RECONNECT_DELAY,
                "reconnect backoff must never exceed the cap, got {delay:?}"
            );
        }
        assert_eq!(delay, MAX_RECONNECT_DELAY);
    }

    #[tokio::test]
    async fn sleep_or_cancelled_returns_true_immediately_when_already_cancelled() {
        let token = CancellationToken::new();
        token.cancel();

        let cancelled = tokio::time::timeout(
            Duration::from_millis(200),
            LogCollector::sleep_or_cancelled(Duration::from_secs(30), &token),
        )
        .await
        .expect("sleep_or_cancelled must return promptly once cancelled, not wait out the full delay");

        assert!(cancelled, "sleep_or_cancelled must report cancellation, not a timed-out sleep");
    }

    #[tokio::test]
    async fn sleep_or_cancelled_returns_false_after_the_delay_elapses() {
        let token = CancellationToken::new();

        let cancelled = LogCollector::sleep_or_cancelled(Duration::from_millis(10), &token).await;

        assert!(!cancelled, "an uncancelled sleep must report false once the delay elapses");
    }
}
