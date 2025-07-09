#![deny(warnings)]

use super::backpressure::{BackpressureLevel, BackpressureStrategy};
use super::metrics::{BufferMetrics, DetailedMetrics};
use crate::parser::NginxLogEntry;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, AtomicUsize, Ordering};
use std::time::Instant;
use thiserror::Error;
use tokio::sync::broadcast::{Receiver as BroadcastReceiver, Sender as BroadcastSender};
use tokio::time::sleep;

#[derive(Error, Debug)]
pub enum BufferError {
    #[error("Invalid buffer capacity")]
    InvalidCapacity,
    #[error("Buffer is full")]
    Full,
    #[error("Buffer is empty")]
    Empty,
    #[error("Send failed: {0}")]
    SendFailed(String),
    #[error("Receive failed: {0}")]
    ReceiveFailed(String),
}

pub struct LogBuffer {
    sender: BroadcastSender<Arc<NginxLogEntry>>,
    receiver: BroadcastReceiver<Arc<NginxLogEntry>>,
    capacity: usize,
    // Atomic metrics for lock-free tracking
    pushed: AtomicU64,
    popped: AtomicU64,
    dropped: AtomicU64,
    current_len: AtomicUsize,
    peak_size: AtomicUsize,
    start_time: Instant,
}

impl LogBuffer {
    pub fn new(capacity: usize) -> Result<Self, BufferError> {
        if capacity == 0 {
            return Err(BufferError::InvalidCapacity);
        }

        // Prevent excessive memory allocation
        if capacity > 100_000_000 {
            return Err(BufferError::InvalidCapacity);
        }

        let (sender, receiver) = tokio::sync::broadcast::channel(capacity);

        Ok(Self {
            sender,
            receiver,
            capacity,
            pushed: AtomicU64::new(0),
            popped: AtomicU64::new(0),
            dropped: AtomicU64::new(0),
            current_len: AtomicUsize::new(0),
            peak_size: AtomicUsize::new(0),
            start_time: Instant::now(),
        })
    }

    pub fn capacity(&self) -> usize {
        self.capacity
    }

    pub fn len(&self) -> usize {
        self.current_len.load(Ordering::Relaxed)
    }

    pub fn is_empty(&self) -> bool {
        self.len() == 0
    }

    pub fn is_full(&self) -> bool {
        self.len() >= self.capacity
    }

    pub fn metrics(&self) -> BufferMetrics {
        let len = self.len();
        let memory_usage_bytes = len
            .saturating_mul(std::mem::size_of::<Arc<NginxLogEntry>>());
        
        BufferMetrics {
            capacity: self.capacity,
            len,
            pushed: self.pushed.load(Ordering::Relaxed),
            popped: self.popped.load(Ordering::Relaxed),
            dropped: self.dropped.load(Ordering::Relaxed),
            memory_usage_bytes,
        }
    }

    #[inline(always)]
    pub fn push(&self, log_entry: Arc<NginxLogEntry>) -> Result<(), BufferError> {
        loop {
            let current_len = self.current_len.load(Ordering::Acquire);
            if current_len >= self.capacity {
                self.dropped.fetch_add(1, Ordering::Relaxed);
                return Err(BufferError::Full);
            }

            // Try to atomically increment if under capacity
            match self.current_len.compare_exchange_weak(
                current_len,
                current_len + 1,
                Ordering::AcqRel,
                Ordering::Acquire,
            ) {
                Ok(_) => {
                    // Successfully reserved space, now try to send
                    match self.sender.send(log_entry) {
                        Ok(_) => {
                            self.pushed.fetch_add(1, Ordering::Relaxed);
                            // Update peak size if necessary
                            self.update_peak_size(current_len + 1);
                            return Ok(());
                        }
                        Err(_) => {
                            // Failed to send, release the reserved space
                            self.current_len.fetch_sub(1, Ordering::Relaxed);
                            self.dropped.fetch_add(1, Ordering::Relaxed);
                            return Err(BufferError::Full);
                        }
                    }
                }
                Err(_) => {
                    // CAS failed, retry
                    continue;
                }
            }
        }
    }

    #[inline(always)]
    pub fn pop(&mut self) -> Result<Arc<NginxLogEntry>, BufferError> {
        match self.receiver.try_recv() {
            Ok(log_entry) => {
                self.popped.fetch_add(1, Ordering::Relaxed);
                self.current_len.fetch_sub(1, Ordering::Release);
                Ok(log_entry)
            }
            Err(_) => Err(BufferError::Empty),
        }
    }

    // Non-blocking push with immediate result
    #[inline(always)]
    pub fn try_push(&self, log_entry: Arc<NginxLogEntry>) -> bool {
        self.push(log_entry).is_ok()
    }

    // Non-blocking pop with immediate result
    #[inline(always)]
    pub fn try_pop(&mut self) -> Option<Arc<NginxLogEntry>> {
        self.pop().ok()
    }

    // Batch operations for better performance
    pub fn push_batch(&self, entries: Vec<Arc<NginxLogEntry>>) -> Result<usize, BufferError> {
        let mut pushed_count = 0;

        for entry in entries {
            if self.push(entry).is_ok() {
                pushed_count += 1;
            } else {
                break; // Stop on first failure to maintain order
            }
        }

        Ok(pushed_count)
    }

    pub fn pop_batch(&mut self, max_size: usize) -> Vec<Arc<NginxLogEntry>> {
        // Validate max_size to prevent excessive memory allocation
        const MAX_BATCH_SIZE: usize = 1_000_000; // 1M entries max
        let safe_max_size = if max_size > MAX_BATCH_SIZE {
            MAX_BATCH_SIZE
        } else {
            max_size
        };

        let mut batch = Vec::with_capacity(safe_max_size);

        for _ in 0..safe_max_size {
            if let Ok(entry) = self.pop() {
                batch.push(entry);
            } else {
                break;
            }
        }

        batch
    }

    // Helper method to update peak size atomically
    fn update_peak_size(&self, current_size: usize) {
        let mut peak = self.peak_size.load(Ordering::Relaxed);
        while current_size > peak {
            match self.peak_size.compare_exchange_weak(
                peak,
                current_size,
                Ordering::Relaxed,
                Ordering::Relaxed,
            ) {
                Ok(_) => break,
                Err(x) => peak = x,
            }
        }
    }

    pub fn detailed_metrics(&self) -> DetailedMetrics {
        let basic_metrics = self.metrics();
        let now = Instant::now();

        // Calculate throughput
        let total_time = now.duration_since(self.start_time).as_secs_f64();
        let throughput = if total_time > 0.0 {
            basic_metrics.pushed as f64 / total_time
        } else {
            0.0
        };

        DetailedMetrics {
            capacity: basic_metrics.capacity,
            len: basic_metrics.len,
            pushed: basic_metrics.pushed,
            popped: basic_metrics.popped,
            dropped: basic_metrics.dropped,
            memory_usage_bytes: basic_metrics.memory_usage_bytes,
            throughput_per_second: throughput,
            average_latency_ns: 0, // TODO: Implement latency tracking
            peak_queue_size: self.peak_size.load(Ordering::Relaxed),
            fill_ratio: basic_metrics.len as f64 / basic_metrics.capacity as f64,
        }
    }

    // Reset metrics (useful for testing and monitoring)
    pub fn reset_metrics(&self) {
        self.pushed.store(0, Ordering::Relaxed);
        self.popped.store(0, Ordering::Relaxed);
        self.dropped.store(0, Ordering::Relaxed);
        self.peak_size.store(self.len(), Ordering::Relaxed);
    }

    // Backpressure methods
    pub async fn push_with_backpressure(
        &self,
        log_entry: Arc<NginxLogEntry>,
    ) -> Result<(), BufferError> {
        self.push_with_strategy(log_entry, BackpressureStrategy::default())
            .await
    }

    pub async fn push_with_strategy(
        &self,
        log_entry: Arc<NginxLogEntry>,
        strategy: BackpressureStrategy,
    ) -> Result<(), BufferError> {
        let mut attempts = 0;
        const MAX_ATTEMPTS: usize = 100; // Prevent infinite loops

        loop {
            match self.push(log_entry.clone()) {
                Ok(()) => return Ok(()),
                Err(BufferError::Full) => {
                    attempts += 1;
                    if attempts >= MAX_ATTEMPTS {
                        return Err(BufferError::Full);
                    }

                    match strategy {
                        BackpressureStrategy::Sleep(duration) => {
                            sleep(duration).await;
                        }
                        BackpressureStrategy::Yield => {
                            tokio::task::yield_now().await;
                        }
                        BackpressureStrategy::Drop => {
                            return Err(BufferError::Full);
                        }
                        BackpressureStrategy::Block => {
                            // Block until space is available with timeout
                            let mut retries = 0;
                            while self.is_full() && retries < 100 {
                                tokio::task::yield_now().await;
                                retries += 1;
                            }
                            if retries >= 100 {
                                return Err(BufferError::Full);
                            }
                        }
                    }
                }
                Err(e) => return Err(e),
            }
        }
    }

    pub fn fill_ratio(&self) -> f64 {
        self.len() as f64 / self.capacity as f64
    }

    pub fn needs_backpressure(&self) -> bool {
        self.fill_ratio() > 0.8 // Apply backpressure when >80% full
    }

    pub fn backpressure_level(&self) -> BackpressureLevel {
        let ratio = self.fill_ratio();

        if ratio < 0.5 {
            BackpressureLevel::None
        } else if ratio < 0.8 {
            BackpressureLevel::Low
        } else if ratio < 0.95 {
            BackpressureLevel::Medium
        } else {
            BackpressureLevel::High
        }
    }
}

// SAFE: LogBuffer is automatically Send+Sync because:
// - BroadcastSender<Arc<NginxLogEntry>> is Send+Sync (tokio guarantees this)
// - BroadcastReceiver<Arc<NginxLogEntry>> is Send+Sync (tokio guarantees this)
// - Arc<NginxLogEntry> is Send+Sync (Arc provides this for thread-safe contents)
// - AtomicU64 and AtomicUsize are Send+Sync (std guarantees this)
// - usize and Instant are Send+Sync (std guarantees this)
// 
// No unsafe implementations needed - Rust's type system automatically derives
// Send+Sync for this type based on its components.

impl std::fmt::Debug for LogBuffer {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("LogBuffer")
            .field("capacity", &self.capacity)
            .field("len", &self.len())
            .field("pushed", &self.pushed.load(Ordering::Relaxed))
            .field("popped", &self.popped.load(Ordering::Relaxed))
            .field("dropped", &self.dropped.load(Ordering::Relaxed))
            .field("peak_size", &self.peak_size.load(Ordering::Relaxed))
            .finish()
    }
}
