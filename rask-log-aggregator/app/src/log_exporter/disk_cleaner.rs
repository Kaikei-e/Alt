// DiskCleaner: periodically ensures the total size of log files stays under a given limit.
// It deletes the oldest .json files in the directory until the total is below the threshold.

use chrono::{DateTime, Local};
use std::path::PathBuf;
use std::time::Duration;
use tokio::fs;
use tokio::io;
use tokio::time::sleep;
use tracing::{error, info, warn};

pub struct DiskCleaner {
    directory: PathBuf,
    max_total_bytes: u64,
    interval: Duration,
}

impl DiskCleaner {
    /// Create a new cleaner. `max_total_bytes` defines the quota, `interval` the check period.
    pub fn new(directory: impl Into<PathBuf>, max_total_bytes: u64, interval: Duration) -> Self {
        Self {
            directory: directory.into(),
            max_total_bytes,
            interval,
        }
    }

    /// Spawn an async task that runs cleanup forever.
    pub fn spawn(self) {
        tokio::spawn(async move { self.run().await });
    }

    async fn run(self) {
        loop {
            if let Err(e) = self.perform_cleanup().await {
                error!("disk cleanup error: {}", e);
            }
            sleep(self.interval).await;
        }
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
            "Total log size {} bytes exceeds limit {}, starting cleanup",
            total, self.max_total_bytes
        );

        // Delete oldest files until under limit, but keep at least one newest file
        for (idx, (path, size, _)) in files.iter().enumerate() {
            // Keep last file (idx == files.len()-1)
            if idx == files.len() - 1 {
                break;
            }
            match fs::remove_file(path).await {
                Ok(_) => {
                    warn!("Removed {:?} ({} bytes)", path, size);
                    total = total.saturating_sub(*size);
                    if total <= self.max_total_bytes {
                        break;
                    }
                }
                Err(e) => error!("Failed to remove {:?}: {}", path, e),
            }
        }

        Ok(())
    }
}
