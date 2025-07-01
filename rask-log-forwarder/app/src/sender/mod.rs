pub mod client;
pub mod http;
pub mod metrics;
pub mod serialization;
pub mod transmission;

// Export old interface for compatibility
pub use http::{BatchSender, ConnectionStats, SenderConfig, SenderError};

// Export new TASK4 interface
pub use client::{ClientConfig, ClientError, ConnectionStats as NewConnectionStats, HttpClient};
pub use metrics::{MetricsCollector, PerformanceMetrics};
pub use serialization::{BatchSerializer, SerializationError, SerializationFormat};
pub use transmission::{BatchTransmitter, TransmissionError, TransmissionResult};

// High-level sender that combines all components
#[derive(Clone)]
pub struct LogSender {
    transmitter: BatchTransmitter,
    metrics: MetricsCollector,
}

impl LogSender {
    pub async fn new(config: ClientConfig) -> Result<Self, ClientError> {
        let client = HttpClient::new(config).await?;
        let transmitter = BatchTransmitter::new(client);
        let metrics = MetricsCollector::new();

        Ok(Self {
            transmitter,
            metrics,
        })
    }

    pub async fn send_batch(
        &self,
        batch: crate::buffer::Batch,
    ) -> Result<TransmissionResult, TransmissionError> {
        let entries_count = batch.size();
        let start = std::time::Instant::now();

        let result = self.transmitter.send_batch(batch).await;

        match &result {
            Ok(transmission_result) => {
                self.metrics.record_transmission(
                    transmission_result.success,
                    entries_count,
                    transmission_result.bytes_sent,
                    transmission_result.latency,
                    if transmission_result.compressed {
                        Some((
                            transmission_result.bytes_sent,
                            transmission_result.bytes_sent * 2,
                        )) // Estimate
                    } else {
                        None
                    },
                );
            }
            Err(_) => {
                self.metrics
                    .record_transmission(false, entries_count, 0, start.elapsed(), None);
            }
        }

        result
    }

    pub fn metrics(&self) -> PerformanceMetrics {
        self.metrics.snapshot()
    }

    pub async fn health_check(&self) -> Result<(), ClientError> {
        self.transmitter.client.health_check().await
    }
}
