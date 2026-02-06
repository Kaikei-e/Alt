// DiskCleaner: periodically ensures the total size of log files stays under a given limit.
// It deletes the oldest .json files in the directory until the total is below the threshold.

use chrono::{DateTime, Local};
use std::path::PathBuf;
use std::time::Duration;
use tokio::fs;
use tokio::io;
use tokio::task::JoinHandle;
use tokio::time::sleep;
use tokio_util::sync::CancellationToken;
use tracing::{error, info, warn};

#[allow(dead_code)]
pub struct DiskCleaner {
    directory: PathBuf,
    max_total_bytes: u64,
    interval: Duration,
}

#[allow(dead_code)]
impl DiskCleaner {
    /// Create a new cleaner. `max_total_bytes` defines the quota, `interval` the check period.
    #[must_use]
    pub fn new(directory: impl Into<PathBuf>, max_total_bytes: u64, interval: Duration) -> Self {
        Self {
            directory: directory.into(),
            max_total_bytes,
            interval,
        }
    }

    /// Spawn an async task that runs cleanup with graceful shutdown support.
    ///
    /// Returns a `JoinHandle` that resolves when the cleaner is shutdown.
    /// Use `cancel_token.cancel()` to trigger graceful shutdown.
    #[must_use]
    pub fn spawn(self, cancel_token: CancellationToken) -> JoinHandle<()> {
        tokio::spawn(async move { self.run(cancel_token).await })
    }

    async fn run(self, cancel_token: CancellationToken) {
        info!(
            "DiskCleaner started for {:?}, interval {:?}",
            self.directory, self.interval
        );

        loop {
            tokio::select! {
                () = cancel_token.cancelled() => {
                    info!("DiskCleaner received shutdown signal, stopping");
                    break;
                }
                () = sleep(self.interval) => {
                    if let Err(e) = self.perform_cleanup().await {
                        error!("disk cleanup error: {e}");
                    }
                }
            }
        }

        info!("DiskCleaner shutdown complete");
    }

    async fn perform_cleanup(&self) -> io::Result<()> {
        // Gather json files in directory
        let mut dir = fs::read_dir(&self.directory).await?;
        let mut files: Vec<(PathBuf, u64, DateTime<Local>)> = Vec::new();

        while let Some(entry) = dir.next_entry().await? {
            let path = entry.path();
            if path.extension().and_then(|s| s.to_str()) != Some("json") {
                continue;
            }
            let metadata = entry.metadata().await?;
            if !metadata.is_file() {
                continue;
            }
            let len = metadata.len();
            let modified: DateTime<Local> = metadata.modified()?.into();
            files.push((path, len, modified));
        }

        // Sort by modified ascending (oldest first)
        files.sort_by_key(|(_, _, dt)| *dt);

        let mut total: u64 = files.iter().map(|(_, sz, _)| *sz).sum();
        if total <= self.max_total_bytes {
            return Ok(());
        }

        info!(
            "Total log size {total} bytes exceeds limit {}, starting cleanup",
            self.max_total_bytes
        );

        // Delete oldest files until under limit, but keep at least one newest file
        for (idx, (path, size, _)) in files.iter().enumerate() {
            // Keep last file (idx == files.len()-1)
            if idx == files.len() - 1 {
                break;
            }
            match fs::remove_file(path).await {
                Ok(()) => {
                    warn!("Removed {path:?} ({size} bytes)");
                    total = total.saturating_sub(*size);
                    if total <= self.max_total_bytes {
                        break;
                    }
                }
                Err(e) => error!("Failed to remove {path:?}: {e}"),
            }
        }

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::Duration;
    use tempfile::TempDir;
    use tokio::fs::File;
    use tokio::io::AsyncWriteExt;

    #[tokio::test]
    async fn test_disk_cleaner_shutdown_signal() {
        let temp_dir = TempDir::new().unwrap();
        let cancel_token = CancellationToken::new();

        let cleaner = DiskCleaner::new(
            temp_dir.path(),
            1024 * 1024, // 1MB limit
            Duration::from_millis(100),
        );

        let handle = cleaner.spawn(cancel_token.clone());

        // Give it a moment to start
        tokio::time::sleep(Duration::from_millis(50)).await;

        // Send shutdown signal
        cancel_token.cancel();

        // Should complete within reasonable time
        let result = tokio::time::timeout(Duration::from_secs(1), handle).await;
        assert!(result.is_ok(), "DiskCleaner should shutdown gracefully");
    }

    #[tokio::test]
    async fn test_disk_cleaner_performs_cleanup() {
        let temp_dir = TempDir::new().unwrap();

        // Create test files that exceed the limit
        let file1 = temp_dir.path().join("test1.json");
        let file2 = temp_dir.path().join("test2.json");

        // Create files with different timestamps (file1 is older)
        {
            let mut f = File::create(&file1).await.unwrap();
            f.write_all(&vec![b'a'; 600]).await.unwrap();
            f.sync_all().await.unwrap();
        }

        // Small delay to ensure different timestamps
        tokio::time::sleep(Duration::from_millis(10)).await;

        {
            let mut f = File::create(&file2).await.unwrap();
            f.write_all(&vec![b'b'; 600]).await.unwrap();
            f.sync_all().await.unwrap();
        }

        let cancel_token = CancellationToken::new();

        let cleaner = DiskCleaner::new(
            temp_dir.path(),
            500, // 500 bytes limit - should trigger cleanup
            Duration::from_millis(50),
        );

        let handle = cleaner.spawn(cancel_token.clone());

        // Wait for cleanup to happen
        tokio::time::sleep(Duration::from_millis(150)).await;

        // Shutdown
        cancel_token.cancel();
        let _ = tokio::time::timeout(Duration::from_secs(1), handle).await;

        // Verify that the oldest file was removed (test1.json)
        // and the newest file was kept (test2.json)
        let remaining_entries: Vec<_> = std::fs::read_dir(temp_dir.path())
            .unwrap()
            .filter_map(std::result::Result::ok)
            .collect();

        assert_eq!(
            remaining_entries.len(),
            1,
            "Should have removed the oldest file"
        );
        let remaining_file = remaining_entries[0].file_name();
        assert_eq!(
            remaining_file.to_str().unwrap(),
            "test2.json",
            "Should keep the newest file"
        );
    }

    #[tokio::test]
    async fn test_disk_cleaner_no_cleanup_needed() {
        let temp_dir = TempDir::new().unwrap();

        // Create small files under the limit
        let file1 = temp_dir.path().join("test1.json");
        {
            let mut f = File::create(&file1).await.unwrap();
            f.write_all(b"small").await.unwrap();
        }

        let cancel_token = CancellationToken::new();

        let cleaner = DiskCleaner::new(
            temp_dir.path(),
            1024 * 1024, // 1MB limit
            Duration::from_millis(50),
        );

        let handle = cleaner.spawn(cancel_token.clone());

        // Wait for a cleanup cycle
        tokio::time::sleep(Duration::from_millis(100)).await;

        // Shutdown
        cancel_token.cancel();
        let _ = tokio::time::timeout(Duration::from_secs(1), handle).await;

        // File should still exist
        assert!(
            file1.exists(),
            "File should not be removed when under limit"
        );
    }

    #[tokio::test]
    async fn test_disk_cleaner_keeps_at_least_one_file() {
        let temp_dir = TempDir::new().unwrap();

        // Create only one large file
        let file1 = temp_dir.path().join("only.json");
        {
            let mut f = File::create(&file1).await.unwrap();
            f.write_all(&vec![b'x'; 2000]).await.unwrap();
        }

        let cancel_token = CancellationToken::new();

        let cleaner = DiskCleaner::new(
            temp_dir.path(),
            100, // Very small limit
            Duration::from_millis(50),
        );

        let handle = cleaner.spawn(cancel_token.clone());

        // Wait for cleanup cycle
        tokio::time::sleep(Duration::from_millis(100)).await;

        // Shutdown
        cancel_token.cancel();
        let _ = tokio::time::timeout(Duration::from_secs(1), handle).await;

        // File should still exist (keep at least one)
        assert!(file1.exists(), "Should keep at least one file");
    }
}
