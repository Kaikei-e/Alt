pub mod discovery;
pub mod docker;

use std::sync::Arc;
use std::time::Duration;

use bollard::container::LogOutput;
use bytes::Bytes;
use chrono::{DateTime, Utc};
use thiserror::Error;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

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
    CollectorError as DockerError, DedupeFilter, DockerCollector, DockerConnectError,
    DockerContainerInfo, DockerEndpointError, LogStreamOptions, compute_resume_log_options,
    connect_docker, get_validated_docker_endpoint, parse_log_timestamp_prefix,
    validate_docker_endpoint,
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

/// Which of the container's standard streams Docker demultiplexed a line
/// from. It has to be carried explicitly: `LogOutput::into_bytes()` drops the
/// variant, and the payload bollard hands us has no frame header left, so
/// nothing downstream can tell a panic backtrace from ordinary output.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum LogStream {
    Stdout,
    Stderr,
}

impl LogStream {
    /// Value written to the aggregator's `stream` column, which
    /// `WHERE stream='stderr'` matches against.
    #[must_use]
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Stdout => "stdout",
            Self::Stderr => "stderr",
        }
    }

    /// Classify a Docker log frame. Variants are listed exhaustively rather
    /// than collapsed into `_` so a new bollard variant breaks the build
    /// instead of quietly being recorded as stdout.
    pub(crate) fn from_log_output(output: &LogOutput) -> Self {
        match output {
            LogOutput::StdErr { .. } => Self::Stderr,
            LogOutput::StdOut { .. } => Self::Stdout,
            // TTY-attached containers: Docker merges both streams into a
            // single console stream and reports it as stdout.
            LogOutput::Console { .. } => Self::Stdout,
            // Only produced by the attach endpoint's input side, never by
            // `docker logs`.
            LogOutput::StdIn { .. } => Self::Stdout,
        }
    }
}

/// Raw collected log line. `log`/`time` are deliberately absent: they get
/// re-derived from `raw_bytes` by the downstream Docker JSON parser, so
/// allocating placeholder values for them here would be pure waste on the
/// per-line hot path. `stream` and `container` are the opposite case - they
/// are known here and nowhere else, so dropping them loses them for good.
#[derive(Debug, Clone)]
pub struct LogEntry {
    /// Container metadata discovery resolved for the connection this line
    /// arrived on, including the `rask.group` label. Shared per connection
    /// rather than cloned per line.
    pub container: Arc<ContainerInfo>,
    pub stream: LogStream,
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
    container_info: Option<Arc<discovery::ContainerInfo>>,
    target_service: String,
    last_timestamp: Option<DateTime<Utc>>,
    warned_malformed_prefix: bool,
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
            last_timestamp: None,
            warned_malformed_prefix: false,
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
                // Shared with every entry streamed off this connection, so
                // the `rask.group` resolved here reaches the aggregator
                // instead of being rebuilt (and lost) further downstream.
                Ok(info) => Arc::new(info),
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
            if self.container_info.as_ref().map(|c| c.id.as_str()) != Some(&container_info.id) {
                self.warned_malformed_prefix = false;
            }
            self.container_info = Some(Arc::clone(&container_info));

            let docker_collector = match DockerCollector::new().await {
                Ok(c) => c,
                Err(e @ docker::CollectorError::EndpointError(_)) => {
                    tracing::error!(
                        "Fatal Docker endpoint configuration error for service '{}': {e}",
                        self.target_service
                    );
                    return Err(CollectorError::DockerError(e));
                }
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
                    &container_info,
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
        &mut self,
        docker_collector: DockerCollector,
        container: &Arc<ContainerInfo>,
        tx: mpsc::Sender<LogEntry>,
        cancel_token: CancellationToken,
    ) -> Result<StreamExit, CollectorError> {
        use bollard::query_parameters::LogsOptions;
        use futures::StreamExt;

        // Reuse the DockerCollector's existing Docker client
        let docker = docker_collector.docker();

        let resume_options = compute_resume_log_options(self.last_timestamp);
        let options = LogsOptions {
            follow: resume_options.follow,
            stdout: resume_options.stdout,
            stderr: resume_options.stderr,
            since: resume_options.since,
            timestamps: resume_options.timestamps,
            tail: resume_options.tail,
            ..Default::default()
        };

        let mut dedupe_filter = DedupeFilter::new(self.last_timestamp);

        let mut stream = docker.logs(&container.id, Some(options));

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
                            // Read the stream off the frame before into_bytes()
                            // discards the variant - it is the only place
                            // stderr is still distinguishable.
                            let stream = LogStream::from_log_output(&log_chunk);
                            let raw_bytes = log_chunk.into_bytes();

                            let (parsed_ts, payload) = parse_log_timestamp_prefix(raw_bytes);

                            if parsed_ts.is_none() && !self.warned_malformed_prefix {
                                tracing::warn!(
                                    container_id = %container.id,
                                    container_name = %container.service_name,
                                    "Malformed timestamp prefix in Docker log stream; forwarding line unchanged"
                                );
                                self.warned_malformed_prefix = true;
                            }

                            if dedupe_filter.should_drop(parsed_ts) {
                                tracing::debug!(
                                    container_id = %container.id,
                                    timestamp = ?parsed_ts,
                                    "Dropping duplicate boundary log line on reconnect"
                                );
                                continue;
                            }

                            // Create LogEntry with raw bytes - let the parser handle the actual parsing
                            let entry = LogEntry {
                                container: Arc::clone(container),
                                stream,
                                raw_bytes: payload,
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

                            if let Some(ts) = parsed_ts {
                                self.last_timestamp = Some(ts);
                            }
                        }
                        Some(Err(e)) => {
                            tracing::error!("Docker log stream error: {e}");
                            return Err(CollectorError::DiscoveryError(
                                discovery::DiscoveryError::DockerError(e.into()),
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
        self.container_info.as_deref()
    }
}

#[cfg(test)]
mod log_stream_tests {
    use super::*;

    fn frame_of(output: &LogOutput) -> &'static str {
        LogStream::from_log_output(output).as_str()
    }

    #[test]
    fn docker_frame_variant_decides_the_stream() {
        let message = Bytes::from_static(b"boom\n");

        assert_eq!(
            frame_of(&LogOutput::StdErr {
                message: message.clone()
            }),
            "stderr"
        );
        assert_eq!(
            frame_of(&LogOutput::StdOut {
                message: message.clone()
            }),
            "stdout"
        );
        // TTY containers only ever produce Console frames; Docker itself
        // reports those as stdout.
        assert_eq!(
            frame_of(&LogOutput::Console {
                message: message.clone()
            }),
            "stdout"
        );
        assert_eq!(frame_of(&LogOutput::StdIn { message }), "stdout");
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
        .expect(
            "sleep_or_cancelled must return promptly once cancelled, not wait out the full delay",
        );

        assert!(
            cancelled,
            "sleep_or_cancelled must report cancellation, not a timed-out sleep"
        );
    }

    #[tokio::test]
    async fn sleep_or_cancelled_returns_false_after_the_delay_elapses() {
        let token = CancellationToken::new();

        let cancelled = LogCollector::sleep_or_cancelled(Duration::from_millis(10), &token).await;

        assert!(
            !cancelled,
            "an uncancelled sleep must report false once the delay elapses"
        );
    }
}
