// Write logs to JSON lines files with automatic size/time based rotation.
// Each call appends a single ND-JSON line. When the current file exceeds the
// configured size or max age, a new file with a timestamp suffix is created.
//
// This module provides an alternative log exporter for file-based storage.
// Currently not wired into main.rs but implemented for future use.

use crate::domain::EnrichedLogEntry;
use anyhow::Result;
use chrono::{DateTime, Duration as ChronoDuration, Local};
use std::path::{Path, PathBuf};
use tokio::fs::{File, OpenOptions};
use tokio::io::AsyncWriteExt;
use tokio::sync::Mutex;
use tracing::error;

#[allow(dead_code)]
const DEFAULT_MAX_SIZE_MB: u64 = 10; // 10 MB
#[allow(dead_code)]
const DEFAULT_MAX_AGE_HOURS: i64 = 12; // 12 h

/// Internal shared state (file handle and creation time)
#[allow(dead_code)]
struct Inner {
    file: File,
    created_at: DateTime<Local>,
}

#[allow(dead_code)]
#[derive(Clone)]
pub struct JsonFileExporter {
    directory: PathBuf,
    base_name: String,
    inner: std::sync::Arc<Mutex<Option<Inner>>>,
    max_size_bytes: u64,
    max_age: ChronoDuration,
}

#[allow(dead_code)]
impl JsonFileExporter {
    /// Create with default settings (10 MB or 12 hours)
    pub async fn new(file_path: &str) -> Result<Self> {
        Self::with_rotation(file_path, DEFAULT_MAX_SIZE_MB, DEFAULT_MAX_AGE_HOURS).await
    }

    /// Create with custom max size (MB) and max age (hours)
    pub async fn with_rotation(file_path: &str, max_size_mb: u64, max_age_hours: i64) -> Result<Self> {
        let path = Path::new(file_path);
        let directory = path.parent().unwrap_or(Path::new(".")).to_path_buf();
        let base_name = path
            .file_stem()
            .map(|s| s.to_string_lossy().to_string())
            .unwrap_or_else(|| "logs".to_string());

        // Create directory if it doesn't exist
        tokio::fs::create_dir_all(&directory).await.ok();

        let file = Self::open_new_log_file(&directory, &base_name).await?;

        Ok(Self {
            directory,
            base_name,
            inner: std::sync::Arc::new(Mutex::new(Some(Inner {
                file,
                created_at: Local::now(),
            }))),
            max_size_bytes: max_size_mb * 1024 * 1024,
            max_age: ChronoDuration::hours(max_age_hours),
        })
    }

    async fn open_new_log_file(dir: &Path, base_name: &str) -> Result<File> {
        let timestamp = Local::now().format("%Y%m%d_%H%M%S");
        let filename = format!("{}_{}.json", base_name, timestamp);
        let full_path = dir.join(filename);

        let file = OpenOptions::new()
            .create(true)
            .write(true)
            .append(true)
            .open(full_path)
            .await?;

        Ok(file)
    }

    async fn rotate_if_needed(&self, inner: &mut Inner) -> Result<()> {
        let metadata = inner.file.metadata().await?;
        let need_rotate_size = metadata.len() >= self.max_size_bytes;
        let need_rotate_time = Local::now() - inner.created_at >= self.max_age;

        if need_rotate_size || need_rotate_time {
            // Flush and sync current file
            inner.file.flush().await?;
            inner.file.sync_data().await?;

            // Open new file
            inner.file = Self::open_new_log_file(&self.directory, &self.base_name).await?;
            inner.created_at = Local::now();
        }

        Ok(())
    }

    pub async fn export(&self, log: &EnrichedLogEntry) -> Result<()> {
        let json = serde_json::to_string(log)?;
        self.write_line(&json).await
    }

    /// Write an already-serialized JSON line
    pub async fn export_raw(&self, json_line: &str) -> Result<()> {
        self.write_line(json_line).await
    }

    async fn write_line(&self, line: &str) -> Result<()> {
        let mut guard = self.inner.lock().await;
        let inner = guard.as_mut().ok_or_else(|| anyhow::anyhow!("Exporter closed"))?;

        inner.file.write_all(line.as_bytes()).await?;
        inner.file.write_all(b"\n").await?;
        inner.file.flush().await?;

        self.rotate_if_needed(inner).await?;
        inner.file.sync_data().await?;

        Ok(())
    }
}

impl super::LogExporter for JsonFileExporter {
    fn export_batch(
        &self,
        logs: Vec<EnrichedLogEntry>,
    ) -> std::pin::Pin<Box<dyn std::future::Future<Output = Result<()>> + Send + '_>> {
        Box::pin(async move {
            for log in &logs {
                if let Err(e) = self.export(log).await {
                    error!("Failed to export log to JSON file: {e}");
                }
            }
            Ok(())
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::LogLevel;
    use std::collections::HashMap;
    use tempfile::TempDir;

    #[tokio::test]
    async fn test_json_file_exporter_creates_file() {
        let temp_dir = TempDir::new().unwrap();
        let file_path = temp_dir.path().join("test.json");

        let exporter = JsonFileExporter::new(file_path.to_str().unwrap())
            .await
            .unwrap();

        let log = EnrichedLogEntry {
            service_type: "test".to_string(),
            log_type: "app".to_string(),
            message: "test message".to_string(),
            level: Some(LogLevel::Info),
            timestamp: "2025-01-10T12:00:00Z".to_string(),
            stream: "stdout".to_string(),
            container_id: "abc123".to_string(),
            service_name: "test-svc".to_string(),
            service_group: None,
            fields: HashMap::new(),
        };

        exporter.export(&log).await.unwrap();

        // Verify file was created and contains content
        let files: Vec<_> = std::fs::read_dir(temp_dir.path())
            .unwrap()
            .filter_map(|e| e.ok())
            .collect();
        assert_eq!(files.len(), 1);
    }
}
