use crate::buffer::Batch;
// use crate::parser::EnrichedLogEntry;
use serde::{Deserialize, Serialize};
use serde_json;
use std::io::Write;
use thiserror::Error;

#[derive(Error, Debug)]
pub enum SerializationError {
    #[error("JSON serialization failed: {0}")]
    JsonError(#[from] serde_json::Error),
    #[error("IO error during serialization: {0}")]
    IoError(#[from] std::io::Error),
    #[error("Batch is empty")]
    EmptyBatch,
}

#[derive(Debug, Clone, Copy)]
pub enum SerializationFormat {
    NDJSON,
    JsonArray,
    BatchWithMetadata,
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

        let mut buffer = Vec::with_capacity(self.estimate_serialized_size(batch));

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

        let mut buffer = Vec::with_capacity(self.estimate_serialized_size(batch) + 1024);

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
        let data = match format {
            SerializationFormat::NDJSON => self.serialize_ndjson(batch)?,
            SerializationFormat::JsonArray => self.serialize_json_array(batch)?,
            SerializationFormat::BatchWithMetadata => self.serialize_batch_with_metadata(batch)?,
        };

        // Compress using gzip
        use flate2::{Compression, write::GzEncoder};

        let mut encoder = GzEncoder::new(Vec::new(), Compression::fast());
        encoder.write_all(data.as_bytes())?;
        let compressed = encoder.finish()?;

        Ok(compressed)
    }

    pub fn estimate_serialized_size(&self, batch: &Batch) -> usize {
        // Rough estimation: each entry ~500 bytes in JSON + metadata overhead
        batch.size() * 500 + 1024
    }
}

impl Default for BatchSerializer {
    fn default() -> Self {
        Self::new()
    }
}
