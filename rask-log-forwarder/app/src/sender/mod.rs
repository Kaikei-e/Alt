pub mod client;
pub mod metrics;
#[cfg(feature = "otlp")]
pub mod otlp;
pub mod serialization;
pub mod stats;
pub mod transmission;

pub use client::{ClientConfig, ClientError, ConnectionStats, HttpClient};
pub use metrics::{MetricsCollector, PerformanceMetrics};
#[cfg(feature = "otlp")]
pub use otlp::OtlpSerializer;
pub use serialization::{BatchSerializer, SerializationError, SerializationFormat};
pub use stats::{AtomicConnectionStats, ConnectionStatsSnapshot};
#[cfg(feature = "otlp")]
pub use transmission::OtlpBatchTransmitter;
pub use transmission::{BatchTransmitter, TransmissionError, TransmissionResult};

// High-level sender that combines all components
#[derive(Clone)]
pub struct LogSender {
    transmitter: BatchTransmitter,
    metrics: MetricsCollector,
}

impl LogSender {
    /// Creates a new [`LogSender`] from `config`.
    ///
    /// # Errors
    ///
    /// Returns [`ClientError`] if constructing the underlying [`HttpClient`] fails (invalid
    /// endpoint URL or HTTP client initialization failure).
    pub fn new(config: ClientConfig) -> Result<Self, ClientError> {
        let client = HttpClient::new(config)?;
        let transmitter = BatchTransmitter::new(client);
        let metrics = MetricsCollector::new();

        Ok(Self {
            transmitter,
            metrics,
        })
    }

    /// Sends a batch of log entries and records transmission metrics.
    ///
    /// # Errors
    ///
    /// Propagates any [`TransmissionError`] from [`BatchTransmitter::send_batch`].
    pub async fn send_batch(
        &self,
        batch: crate::buffer::Batch,
    ) -> Result<TransmissionResult, TransmissionError> {
        let entries_count = batch.size();
        let start = std::time::Instant::now();

        let result = self.transmitter.send_batch(batch).await;

        match &result {
            Ok(transmission_result) => {
                // `TransmissionResult` doesn't carry the pre-compression
                // size, so there's no real compressed/uncompressed pair to
                // report here - passing a fabricated `bytes_sent * 2`
                // "estimate" would just make `compression_ratio` a made-up
                // number. Leave it unrecorded rather than lie about it.
                self.metrics.record_transmission(
                    transmission_result.success,
                    entries_count,
                    transmission_result.bytes_sent,
                    transmission_result.latency,
                    None,
                );
            }
            Err(_) => {
                self.metrics
                    .record_transmission(false, entries_count, 0, start.elapsed(), None);
            }
        }

        result
    }

    #[must_use]
    pub fn metrics(&self) -> PerformanceMetrics {
        self.metrics.snapshot()
    }

    /// Runs a health check against the configured aggregator endpoint.
    ///
    /// # Errors
    ///
    /// Propagates any [`ClientError`] from [`HttpClient::health_check`].
    pub async fn health_check(&self) -> Result<(), ClientError> {
        self.transmitter.client.health_check().await
    }

    /// Expose the underlying HTTP client for protocol-specific transmitters (e.g., OTLP).
    #[must_use]
    pub fn transmitter_client(&self) -> &HttpClient {
        &self.transmitter.client
    }
}
