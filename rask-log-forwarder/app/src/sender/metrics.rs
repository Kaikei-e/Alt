use parking_lot::Mutex;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::Duration;

/// Safely calculate percentile from sorted samples with bounds checking
fn calculate_percentile(sorted_samples: &[Duration], percentile: f64) -> Duration {
    if sorted_samples.is_empty() {
        return Duration::ZERO;
    }

    if sorted_samples.len() == 1 {
        return sorted_samples[0];
    }

    // Clamp percentile to valid range [0.0, 1.0]
    let percentile = percentile.clamp(0.0, 1.0);

    // Use safer calculation that avoids floating point precision issues
    let len = sorted_samples.len();
    let index_f64 = percentile * (len.saturating_sub(1)) as f64;

    // Convert to usize with proper bounds checking
    let index = if index_f64.is_finite() && index_f64 >= 0.0 {
        let index_usize = index_f64.floor() as usize;
        index_usize.min(len.saturating_sub(1))
    } else {
        0 // Fallback to first element if floating point calculation fails
    };

    // Safe array access with explicit bounds checking
    match sorted_samples.get(index) {
        Some(&duration) => duration,
        None => {
            // This should never happen due to our bounds checking above,
            // but provide a safe fallback anyway
            match sorted_samples.last() {
                Some(&last_duration) => last_duration,
                None => Duration::ZERO,
            }
        }
    }
}

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

        // Calculate average and percentiles from the same sample set for consistency
        let (average_latency, p95_latency, p99_latency) = {
            let mut samples = self.latency_samples.lock();
            if samples.is_empty() {
                (Duration::ZERO, Duration::ZERO, Duration::ZERO)
            } else {
                samples.sort();

                // Calculate average from samples with overflow protection
                let average = {
                    let mut total_millis: u128 = 0;
                    for sample in samples.iter() {
                        // Use saturating_add to prevent overflow
                        total_millis = total_millis.saturating_add(sample.as_millis());
                    }

                    let samples_count = samples.len();
                    if samples_count > 0 {
                        // Safe division and conversion back to u64 with bounds checking
                        let avg_millis = total_millis / samples_count as u128;
                        let avg_millis_u64 = if avg_millis > u64::MAX as u128 {
                            u64::MAX
                        } else {
                            avg_millis as u64
                        };
                        Duration::from_millis(avg_millis_u64)
                    } else {
                        Duration::ZERO
                    }
                };

                // Calculate percentiles with safe bounds checking
                let p95 = calculate_percentile(&samples, 0.95);
                let p99 = calculate_percentile(&samples, 0.99);

                (average, p95, p99)
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
