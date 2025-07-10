use super::serialization::{BatchSerializer, SerializationError, SerializationFormat};
use super::{ClientError, HttpClient};
use crate::buffer::Batch;
use reqwest::header::{
    CONTENT_ENCODING, CONTENT_TYPE, HeaderMap, HeaderName, HeaderValue, USER_AGENT,
};
use std::time::{Duration, Instant};
use thiserror::Error;
use tracing::{debug, error, info, warn};

#[derive(Error, Debug)]
pub enum TransmissionError {
    #[error("Serialization failed: {0}")]
    SerializationFailed(#[from] SerializationError),
    #[error("Client error: {0}")]
    ClientError(#[from] ClientError),
    #[error("Request error: {0}")]
    RequestError(#[from] reqwest::Error),
    #[error("Transmission timeout")]
    Timeout,
    #[error("Invalid response: {0}")]
    InvalidResponse(String),
    #[error("Invalid header value: {0}")]
    InvalidHeaderValue(String),
}

#[derive(Debug, Clone)]
pub struct TransmissionResult {
    pub success: bool,
    pub status_code: u16,
    pub latency: Duration,
    pub batch_id: String,
    pub bytes_sent: usize,
    pub compressed: bool,
    pub retry_count: u32,
}

#[derive(Clone)]
pub struct BatchTransmitter {
    pub client: HttpClient,
    serializer: BatchSerializer,
}

impl BatchTransmitter {
    pub fn new(client: HttpClient) -> Self {
        Self {
            client,
            serializer: BatchSerializer::new(),
        }
    }

    pub async fn send_batch(&self, batch: Batch) -> Result<TransmissionResult, TransmissionError> {
        self.send_batch_with_retry(batch, 0).await
    }

    pub async fn send_batch_with_retry(
        &self,
        batch: Batch,
        retry_count: u32,
    ) -> Result<TransmissionResult, TransmissionError> {
        let start = Instant::now();
        let batch_id = batch.id().to_string();
        let batch_size = batch.size();

        debug!(
            "Sending batch {} with {} entries (attempt {})",
            batch_id,
            batch_size,
            retry_count + 1
        );

        // Prepare payload
        let use_compression = self.client.config.enable_compression && batch.size() > 100;
        let payload = self.prepare_payload(&batch, use_compression)?;
        let bytes_sent = payload.len();

        // Build headers
        let headers = self.build_headers(&batch, use_compression)?;

        // Send request
        let mut request_builder = self
            .client
            .client
            .post(self.client.aggregate_url.clone())
            .headers(headers)
            .timeout(self.client.config.timeout);

        request_builder = request_builder.body(payload);

        let response = request_builder.send().await?;
        let latency = start.elapsed();

        let status_code = response.status().as_u16();
        let success = response.status().is_success();

        // Record metrics
        self.client.stats.record_request(success, latency);

        if success {
            info!(
                "Successfully sent batch {} ({} entries, {} bytes) in {:?}",
                batch_id, batch_size, bytes_sent, latency
            );
        } else {
            warn!(
                "Failed to send batch {} (attempt {}): HTTP {}",
                batch_id,
                retry_count + 1,
                status_code
            );
        }

        Ok(TransmissionResult {
            success,
            status_code,
            latency,
            batch_id,
            bytes_sent,
            compressed: use_compression,
            retry_count,
        })
    }

    pub fn prepare_payload(
        &self,
        batch: &Batch,
        compress: bool,
    ) -> Result<Vec<u8>, SerializationError> {
        if compress {
            self.serializer
                .serialize_compressed(batch, SerializationFormat::NDJSON)
        } else {
            let ndjson = self.serializer.serialize_ndjson(batch)?;
            Ok(ndjson.into_bytes())
        }
    }

    pub fn build_headers(&self, batch: &Batch, compressed: bool) -> Result<HeaderMap, TransmissionError> {
        let mut headers = HeaderMap::new();

        // Content type
        headers.insert(
            CONTENT_TYPE,
            HeaderValue::from_static("application/x-ndjson"),
        );

        // Compression
        if compressed {
            headers.insert(CONTENT_ENCODING, HeaderValue::from_static("gzip"));
        }

        // Batch metadata
        headers.insert(
            HeaderName::from_static("x-batch-id"),
            HeaderValue::from_str(batch.id())
                .map_err(|e| TransmissionError::InvalidHeaderValue(format!("Invalid batch ID: {}", e)))?,
        );

        headers.insert(
            HeaderName::from_static("x-batch-size"),
            HeaderValue::from_str(&batch.size().to_string())
                .map_err(|e| TransmissionError::InvalidHeaderValue(format!("Invalid batch size: {}", e)))?,
        );

        headers.insert(
            HeaderName::from_static("x-batch-type"),
            HeaderValue::from_str(&format!("{:?}", batch.batch_type()))
                .map_err(|e| TransmissionError::InvalidHeaderValue(format!("Invalid batch type: {}", e)))?,
        );

        // Forwarder info
        headers.insert(
            HeaderName::from_static("x-forwarder-version"),
            HeaderValue::from_str(env!("CARGO_PKG_VERSION"))
                .map_err(|e| TransmissionError::InvalidHeaderValue(format!("Invalid version: {}", e)))?,
        );

        headers.insert(
            USER_AGENT,
            HeaderValue::from_str(&self.client.config.user_agent)
                .map_err(|e| TransmissionError::InvalidHeaderValue(format!("Invalid user agent: {}", e)))?,
        );

        Ok(headers)
    }

    pub async fn send_batch_streaming(
        &self,
        _batch: Batch,
    ) -> Result<TransmissionResult, TransmissionError> {
        // For very large batches, implement streaming transmission
        // This would serialize and send data in chunks
        todo!("Implement streaming transmission for very large batches")
    }
}
