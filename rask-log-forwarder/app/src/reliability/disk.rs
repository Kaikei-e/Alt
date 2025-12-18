use crate::buffer::Batch;
use flate2::Compression;
use flate2::read::GzDecoder;
use flate2::write::GzEncoder;
use serde::{Deserialize, Serialize};
use std::io::{Read, Write};
use std::path::{Path, PathBuf};
use std::time::{Duration, SystemTime, UNIX_EPOCH};
use thiserror::Error;
use tokio::fs;
use tokio::io::{AsyncReadExt, AsyncWriteExt};

#[derive(Error, Debug)]
pub enum DiskError {
    #[error("IO error: {0}")]
    IoError(#[from] std::io::Error),
    #[error("Serialization error: {0}")]
    SerializationError(#[from] bincode::error::EncodeError),
    #[error("Deserialization error: {0}")]
    DeserializationError(#[from] bincode::error::DecodeError),
    #[error("Batch not found: {0}")]
    BatchNotFound(String),
    #[error("Disk space exceeded")]
    DiskSpaceExceeded,
    #[error("Invalid storage path: {0}")]
    InvalidStoragePath(String),
    #[error("System time error: {0}")]
    SystemTimeError(String),
}

#[derive(Debug, Clone)]
pub struct DiskConfig {
    pub storage_path: PathBuf,
    pub max_disk_usage: u64, // bytes
    pub retention_period: Duration,
    pub compression: bool,
}

impl Default for DiskConfig {
    fn default() -> Self {
        Self {
            storage_path: PathBuf::from("/tmp/rask-log-forwarder/fallback"),
            max_disk_usage: 1024 * 1024 * 1024, // 1GB
            retention_period: Duration::from_secs(24 * 3600), // 24 hours
            compression: true,
        }
    }
}

#[derive(Serialize, Deserialize)]
struct StoredBatch {
    id: String,
    entries: Vec<crate::parser::EnrichedLogEntry>,
    batch_type: crate::buffer::BatchType,
    estimated_size: usize,
    stored_at: u64, // Unix timestamp
    compressed: bool,
}

pub struct DiskFallback {
    config: DiskConfig,
    current_usage: u64,
}

impl DiskFallback {
    pub async fn new(config: DiskConfig) -> Result<Self, DiskError> {
        // Create storage directory if it doesn't exist
        fs::create_dir_all(&config.storage_path).await?;

        // Calculate current disk usage
        let current_usage = Self::calculate_disk_usage(&config.storage_path).await?;

        Ok(Self {
            config,
            current_usage,
        })
    }

    pub async fn store_batch(&mut self, batch: Batch) -> Result<(), DiskError> {
        let batch_id = batch.id().to_string();
        let batch_type = batch.batch_type();
        let estimated_size = batch.estimated_memory_size();
        let entries = batch.into_entries();

        let stored_at = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map_err(|e| DiskError::SystemTimeError(format!("Invalid system time: {e}")))?
            .as_secs();

        let stored_batch = StoredBatch {
            id: batch_id.clone(),
            entries,
            batch_type,
            estimated_size,
            stored_at,
            compressed: self.config.compression,
        };

        // Serialize batch
        let serialized = bincode::serde::encode_to_vec(&stored_batch, bincode::config::standard())?;

        // Compress if enabled
        let data = if self.config.compression {
            let mut encoder = GzEncoder::new(Vec::new(), Compression::fast());
            encoder.write_all(&serialized)?;
            encoder.finish()?
        } else {
            serialized
        };

        // Check disk space
        if self.current_usage + data.len() as u64 > self.config.max_disk_usage {
            return Err(DiskError::DiskSpaceExceeded);
        }

        // Write to file
        let file_path = self.get_batch_file_path(&batch_id);
        let mut file = fs::File::create(&file_path).await?;
        file.write_all(&data).await?;
        file.sync_all().await?;

        self.current_usage += data.len() as u64;

        tracing::debug!("Stored batch {} to disk ({} bytes)", batch_id, data.len());
        Ok(())
    }

    pub async fn retrieve_batch(&self, batch_id: &str) -> Result<Batch, DiskError> {
        let file_path = self.get_batch_file_path(batch_id);

        if !file_path.exists() {
            return Err(DiskError::BatchNotFound(batch_id.to_string()));
        }

        // Read file
        let mut file = fs::File::open(&file_path).await?;
        let mut data = Vec::new();
        file.read_to_end(&mut data).await?;

        // Decompress if needed (try both compressed and uncompressed)
        let deserialized_data = if let Ok(decompressed) = self.decompress_data(&data) {
            decompressed
        } else {
            data // Assume uncompressed
        };

        // Deserialize
        let (stored_batch, _): (StoredBatch, usize) =
            bincode::serde::decode_from_slice(&deserialized_data, bincode::config::standard())?;

        tracing::debug!("Retrieved batch {batch_id} from disk");

        // Reconstruct the Batch with original ID
        let batch = Batch::with_id(
            stored_batch.id,
            stored_batch.entries,
            stored_batch.batch_type,
            stored_batch.estimated_size,
        );
        Ok(batch)
    }

    pub async fn has_batch(&self, batch_id: &str) -> bool {
        self.get_batch_file_path(batch_id).exists()
    }

    pub async fn delete_batch(&mut self, batch_id: &str) -> Result<(), DiskError> {
        let file_path = self.get_batch_file_path(batch_id);

        if !file_path.exists() {
            return Err(DiskError::BatchNotFound(batch_id.to_string()));
        }

        // Get file size before deletion
        let metadata = fs::metadata(&file_path).await?;
        let file_size = metadata.len();

        // Delete file
        fs::remove_file(&file_path).await?;

        self.current_usage = self.current_usage.saturating_sub(file_size);

        tracing::debug!("Deleted batch {batch_id} from disk");
        Ok(())
    }

    pub async fn list_stored_batches(&self) -> Result<Vec<String>, DiskError> {
        let mut batch_ids = Vec::new();
        let mut entries = fs::read_dir(&self.config.storage_path).await?;

        while let Some(entry) = entries.next_entry().await? {
            if let Some(file_name) = entry.file_name().to_str()
                && file_name.ends_with(".batch")
            {
                let batch_id = file_name.trim_end_matches(".batch");
                batch_ids.push(batch_id.to_string());
            }
        }

        Ok(batch_ids)
    }

    pub async fn cleanup_old_batches(&mut self) -> Result<u32, DiskError> {
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map_err(|e| DiskError::SystemTimeError(format!("Invalid system time: {e}")))?
            .as_secs();

        let batch_ids = self.list_stored_batches().await?;
        let mut deleted_count = 0;

        for batch_id in batch_ids {
            if let Ok(stored_batch) = self.load_stored_batch(&batch_id).await {
                let age = now.saturating_sub(stored_batch.stored_at);

                if age > self.config.retention_period.as_secs()
                    && let Ok(()) = self.delete_batch(&batch_id).await
                {
                    deleted_count += 1;
                }
            }
        }

        if deleted_count > 0 {
            tracing::info!("Cleaned up {deleted_count} old batches from disk");
        }

        Ok(deleted_count)
    }

    pub fn current_disk_usage(&self) -> u64 {
        self.current_usage
    }

    pub fn disk_usage_percentage(&self) -> f64 {
        (self.current_usage as f64 / self.config.max_disk_usage as f64) * 100.0
    }

    fn get_batch_file_path(&self, batch_id: &str) -> PathBuf {
        self.config.storage_path.join(format!("{batch_id}.batch"))
    }

    async fn load_stored_batch(&self, batch_id: &str) -> Result<StoredBatch, DiskError> {
        let file_path = self.get_batch_file_path(batch_id);
        let mut file = fs::File::open(&file_path).await?;
        let mut data = Vec::new();
        file.read_to_end(&mut data).await?;

        let deserialized_data = if let Ok(decompressed) = self.decompress_data(&data) {
            decompressed
        } else {
            data
        };

        let (stored_batch, _): (StoredBatch, usize) =
            bincode::serde::decode_from_slice(&deserialized_data, bincode::config::standard())
                .map_err(DiskError::DeserializationError)?;
        Ok(stored_batch)
    }

    fn decompress_data(&self, data: &[u8]) -> Result<Vec<u8>, std::io::Error> {
        let mut decoder = GzDecoder::new(data);
        let mut decompressed = Vec::new();
        decoder.read_to_end(&mut decompressed)?;
        Ok(decompressed)
    }

    async fn calculate_disk_usage(path: &Path) -> Result<u64, DiskError> {
        let mut total_size = 0u64;
        let mut entries = fs::read_dir(path).await?;

        while let Some(entry) = entries.next_entry().await? {
            if entry.file_type().await?.is_file() {
                total_size += entry.metadata().await?.len();
            }
        }

        Ok(total_size)
    }
}
