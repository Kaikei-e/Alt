use parking_lot::Mutex;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::Duration;

#[derive(Debug, Clone)]
pub struct PerformanceMetrics {
    pub total_batches_sent: u64,
    pub total_entries_sent: u64,
    pub total_bytes_sent: u64,
    pub successful_transmissions: u64,
    pub failed_transmissions: u64,
    pub average_latency: Duration,
    pub p95_latency: Duration,
    pub p99_latency: Duration,
    pub compression_ratio: f64,
}

#[derive(Clone)]
pub struct MetricsCollector {
    total_batches: Arc<AtomicU64>,
    total_entries: Arc<AtomicU64>,
    total_bytes: Arc<AtomicU64>,
    successful_transmissions: Arc<AtomicU64>,
    failed_transmissions: Arc<AtomicU64>,
    total_latency: Arc<AtomicU64>,
    latency_samples: Arc<Mutex<Vec<Duration>>>,
    compressed_bytes: Arc<AtomicU64>,
    uncompressed_bytes: Arc<AtomicU64>,
}

impl MetricsCollector {
    pub fn new() -> Self {
        Self {
            total_batches: Arc::new(AtomicU64::new(0)),
            total_entries: Arc::new(AtomicU64::new(0)),
            total_bytes: Arc::new(AtomicU64::new(0)),
            successful_transmissions: Arc::new(AtomicU64::new(0)),
            failed_transmissions: Arc::new(AtomicU64::new(0)),
            total_latency: Arc::new(AtomicU64::new(0)),
            latency_samples: Arc::new(Mutex::new(Vec::new())),
            compressed_bytes: Arc::new(AtomicU64::new(0)),
            uncompressed_bytes: Arc::new(AtomicU64::new(0)),
        }
    }

    pub fn record_transmission(
        &self,
        success: bool,
        entries_count: usize,
        bytes_sent: usize,
        latency: Duration,
        compression_info: Option<(usize, usize)>, // (compressed, uncompressed)
    ) {
        self.total_batches.fetch_add(1, Ordering::Relaxed);
        self.total_entries
            .fetch_add(entries_count as u64, Ordering::Relaxed);
        self.total_bytes
            .fetch_add(bytes_sent as u64, Ordering::Relaxed);
        self.total_latency
            .fetch_add(latency.as_millis() as u64, Ordering::Relaxed);

        if success {
            self.successful_transmissions
                .fetch_add(1, Ordering::Relaxed);
        } else {
            self.failed_transmissions.fetch_add(1, Ordering::Relaxed);
        }

        // Record latency sample (keep last 1000 samples)
        {
            let mut samples = self.latency_samples.lock();
            samples.push(latency);
            if samples.len() > 1000 {
                samples.remove(0);
            }
        }

        // Record compression metrics
        if let Some((compressed, uncompressed)) = compression_info {
            self.compressed_bytes
                .fetch_add(compressed as u64, Ordering::Relaxed);
            self.uncompressed_bytes
                .fetch_add(uncompressed as u64, Ordering::Relaxed);
        }
    }

    pub fn snapshot(&self) -> PerformanceMetrics {
        let total_batches = self.total_batches.load(Ordering::Relaxed);
        let total_latency = self.total_latency.load(Ordering::Relaxed);

        let average_latency = if total_batches > 0 {
            Duration::from_millis(total_latency / total_batches)
        } else {
            Duration::ZERO
        };

        // Calculate percentiles from samples
        let (p95_latency, p99_latency) = {
            let mut samples = self.latency_samples.lock();
            if samples.is_empty() {
                (Duration::ZERO, Duration::ZERO)
            } else {
                samples.sort();
                let p95_idx = (samples.len() as f64 * 0.95) as usize;
                let p99_idx = (samples.len() as f64 * 0.99) as usize;

                (
                    samples.get(p95_idx).copied().unwrap_or(Duration::ZERO),
                    samples.get(p99_idx).copied().unwrap_or(Duration::ZERO),
                )
            }
        };

        let compressed = self.compressed_bytes.load(Ordering::Relaxed);
        let uncompressed = self.uncompressed_bytes.load(Ordering::Relaxed);
        let compression_ratio = if uncompressed > 0 {
            compressed as f64 / uncompressed as f64
        } else {
            1.0
        };

        PerformanceMetrics {
            total_batches_sent: total_batches,
            total_entries_sent: self.total_entries.load(Ordering::Relaxed),
            total_bytes_sent: self.total_bytes.load(Ordering::Relaxed),
            successful_transmissions: self.successful_transmissions.load(Ordering::Relaxed),
            failed_transmissions: self.failed_transmissions.load(Ordering::Relaxed),
            average_latency,
            p95_latency,
            p99_latency,
            compression_ratio,
        }
    }

    pub fn reset(&self) {
        self.total_batches.store(0, Ordering::Relaxed);
        self.total_entries.store(0, Ordering::Relaxed);
        self.total_bytes.store(0, Ordering::Relaxed);
        self.successful_transmissions.store(0, Ordering::Relaxed);
        self.failed_transmissions.store(0, Ordering::Relaxed);
        self.total_latency.store(0, Ordering::Relaxed);
        self.compressed_bytes.store(0, Ordering::Relaxed);
        self.uncompressed_bytes.store(0, Ordering::Relaxed);

        self.latency_samples.lock().clear();
    }
}

impl Default for MetricsCollector {
    fn default() -> Self {
        Self::new()
    }
}
