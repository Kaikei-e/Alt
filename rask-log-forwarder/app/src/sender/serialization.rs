use crate::buffer::Batch;
// use crate::parser::EnrichedLogEntry;
use serde::{Deserialize, Serialize};
use serde_json;
use std::io::Write;
use thiserror::Error;

// Maximum safe buffer size to prevent memory allocation issues
const MAX_SAFE_BUFFER_SIZE: usize = 100 * 1024 * 1024; // 100MB
const MAX_SAFE_BATCH_SIZE: usize = 1_000_000; // 1M entries
const ESTIMATED_ENTRY_SIZE: usize = 500; // bytes per entry
const METADATA_OVERHEAD: usize = 1024; // bytes

#[derive(Error, Debug)]
pub enum SerializationError {
    #[error("JSON serialization failed: {0}")]
    JsonError(#[from] serde_json::Error),
    #[error("IO error during serialization: {0}")]
    IoError(#[from] std::io::Error),
    #[error("Batch is empty")]
    EmptyBatch,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SerializationFormat {
    NDJSON,
    JsonArray,
    BatchWithMetadata,
    #[cfg(feature = "otlp")]
    OTLP,
}

#[derive(Serialize, Deserialize)]
struct BatchMetadata {
    batch_id: String,
    batch_size: usize,
    batch_type: String,
    timestamp: String,
    forwarder_version: String,
}

#[derive(Clone)]
pub struct BatchSerializer {
    forwarder_version: String,
}

impl BatchSerializer {
    pub fn new() -> Self {
        Self {
            forwarder_version: env!("CARGO_PKG_VERSION").to_string(),
        }
    }

    pub fn serialize_ndjson(&self, batch: &Batch) -> Result<String, SerializationError> {
        if batch.is_empty() {
            return Err(SerializationError::EmptyBatch);
        }

        let estimated_size = self.estimate_serialized_size(batch);
        let safe_capacity = if estimated_size > MAX_SAFE_BUFFER_SIZE {
            MAX_SAFE_BUFFER_SIZE
        } else {
            estimated_size
        };

        let mut buffer = Vec::with_capacity(safe_capacity);

        for entry in batch.entries() {
            serde_json::to_writer(&mut buffer, entry)?;
            buffer.write_all(b"\n")?;
        }

        Ok(String::from_utf8_lossy(&buffer).into_owned())
    }

    pub fn serialize_json_array(&self, batch: &Batch) -> Result<String, SerializationError> {
        if batch.is_empty() {
            return Err(SerializationError::EmptyBatch);
        }

        serde_json::to_string(batch.entries()).map_err(SerializationError::JsonError)
    }

    pub fn serialize_batch_with_metadata(
        &self,
        batch: &Batch,
    ) -> Result<String, SerializationError> {
        if batch.is_empty() {
            return Err(SerializationError::EmptyBatch);
        }

        let estimated_size = self.estimate_serialized_size(batch);
        let safe_capacity = estimated_size.saturating_add(METADATA_OVERHEAD);
        let safe_capacity = if safe_capacity > MAX_SAFE_BUFFER_SIZE {
            MAX_SAFE_BUFFER_SIZE
        } else {
            safe_capacity
        };

        let mut buffer = Vec::with_capacity(safe_capacity);

        // Write batch metadata as first line
        let metadata = BatchMetadata {
            batch_id: batch.id().to_string(),
            batch_size: batch.size(),
            batch_type: format!("{:?}", batch.batch_type()),
            timestamp: chrono::Utc::now().to_rfc3339(),
            forwarder_version: self.forwarder_version.clone(),
        };

        serde_json::to_writer(&mut buffer, &metadata)?;
        buffer.write_all(b"\n")?;

        // Write log entries
        for entry in batch.entries() {
            serde_json::to_writer(&mut buffer, entry)?;
            buffer.write_all(b"\n")?;
        }

        Ok(String::from_utf8_lossy(&buffer).into_owned())
    }

    pub fn serialize_compressed(
        &self,
        batch: &Batch,
        format: SerializationFormat,
    ) -> Result<Vec<u8>, SerializationError> {
        // OTLP format is already binary (protobuf), so we handle it separately
        #[cfg(feature = "otlp")]
        if matches!(format, SerializationFormat::OTLP) {
            return self.serialize_otlp_compressed(batch);
        }

        let data = match format {
            SerializationFormat::NDJSON => self.serialize_ndjson(batch)?,
            SerializationFormat::JsonArray => self.serialize_json_array(batch)?,
            SerializationFormat::BatchWithMetadata => self.serialize_batch_with_metadata(batch)?,
            #[cfg(feature = "otlp")]
            SerializationFormat::OTLP => unreachable!(), // Handled above
        };

        // Compress using gzip
        use flate2::{Compression, write::GzEncoder};

        let mut encoder = GzEncoder::new(Vec::new(), Compression::fast());
        encoder.write_all(data.as_bytes())?;
        let compressed = encoder.finish()?;

        Ok(compressed)
    }

    /// Serialize batch to OTLP protobuf format (optionally compressed).
    #[cfg(feature = "otlp")]
    fn serialize_otlp_compressed(&self, batch: &Batch) -> Result<Vec<u8>, SerializationError> {
        use crate::sender::otlp::OtlpSerializer;
        use flate2::{Compression, write::GzEncoder};

        let otlp_serializer = OtlpSerializer::new();
        let protobuf_bytes = otlp_serializer.serialize_batch(batch)?;

        // Compress using gzip
        let mut encoder = GzEncoder::new(Vec::new(), Compression::fast());
        encoder.write_all(&protobuf_bytes)?;
        let compressed = encoder.finish()?;

        Ok(compressed)
    }

    /// Serialize batch to OTLP protobuf format (uncompressed).
    #[cfg(feature = "otlp")]
    pub fn serialize_otlp(&self, batch: &Batch) -> Result<Vec<u8>, SerializationError> {
        use crate::sender::otlp::OtlpSerializer;

        let otlp_serializer = OtlpSerializer::new();
        otlp_serializer.serialize_batch(batch)
    }

    pub fn estimate_serialized_size(&self, batch: &Batch) -> usize {
        // Validate batch size to prevent overflow
        let batch_size = batch.size();
        if batch_size > MAX_SAFE_BATCH_SIZE {
            return MAX_SAFE_BUFFER_SIZE;
        }

        // Use checked arithmetic to prevent overflow
        let entry_size = batch_size.saturating_mul(ESTIMATED_ENTRY_SIZE);
        entry_size.saturating_add(METADATA_OVERHEAD)
    }
}

impl Default for BatchSerializer {
    fn default() -> Self {
        Self::new()
    }
}
