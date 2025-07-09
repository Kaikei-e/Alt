use crate::buffer::DetailedMetrics;
use crate::parser::EnrichedLogEntry;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, AtomicUsize, Ordering};
use std::time::{Duration, Instant};
use thiserror::Error;
use tokio::sync::broadcast::{self, Receiver as BroadcastReceiver, Sender as BroadcastSender};
use tokio::time::{sleep, timeout};

#[derive(Error, Debug)]
pub enum BufferError {
    #[error("Buffer is full")]
    BufferFull,
    #[error("Buffer is closed")]
    BufferClosed,
    #[error("Send timeout")]
    SendTimeout,
    #[error("Receive timeout")]
    ReceiveTimeout,
}

#[derive(Debug, Clone)]
pub struct BufferConfig {
    pub capacity: usize,
    pub batch_size: usize,
    pub batch_timeout: Duration,
    pub enable_backpressure: bool,
    pub backpressure_threshold: f64, // 0.0 to 1.0
    pub backpressure_delay: Duration,
}

impl Default for BufferConfig {
    fn default() -> Self {
        Self {
            capacity: 100_000,
            batch_size: 10_000,
            batch_timeout: Duration::from_millis(500),
            enable_backpressure: true,
            backpressure_threshold: 0.8, // Apply backpressure at 80% capacity
            backpressure_delay: Duration::from_micros(100),
        }
    }
}

#[derive(Debug, Clone)]
pub struct BufferMetrics {
    pub messages_sent: u64,
    pub messages_received: u64,
    pub messages_dropped: u64,
    pub queue_depth: usize,
    pub batches_formed: u64,
    pub backpressure_events: u64,
}

pub struct BufferMetricsCollector {
    messages_sent: AtomicU64,
    messages_received: AtomicU64,
    messages_dropped: AtomicU64,
    queue_depth: AtomicUsize,
    batches_formed: AtomicU64,
    backpressure_events: AtomicU64,
    start_time: Instant,
}

impl BufferMetricsCollector {
    fn new() -> Self {
        Self {
            messages_sent: AtomicU64::new(0),
            messages_received: AtomicU64::new(0),
            messages_dropped: AtomicU64::new(0),
            queue_depth: AtomicUsize::new(0),
            batches_formed: AtomicU64::new(0),
            backpressure_events: AtomicU64::new(0),
            start_time: Instant::now(),
        }
    }

    pub fn snapshot(&self) -> BufferMetrics {
        BufferMetrics {
            messages_sent: self.messages_sent.load(Ordering::Relaxed),
            messages_received: self.messages_received.load(Ordering::Relaxed),
            messages_dropped: self.messages_dropped.load(Ordering::Relaxed),
            queue_depth: self.queue_depth.load(Ordering::Relaxed),
            batches_formed: self.batches_formed.load(Ordering::Relaxed),
            backpressure_events: self.backpressure_events.load(Ordering::Relaxed),
        }
    }

    #[allow(dead_code)]
    fn increment_batches(&self) {
        self.batches_formed.fetch_add(1, Ordering::Relaxed);
    }

    pub fn reset(&self) {
        self.messages_sent.store(0, Ordering::Relaxed);
        self.messages_received.store(0, Ordering::Relaxed);
        self.messages_dropped.store(0, Ordering::Relaxed);
        self.batches_formed.store(0, Ordering::Relaxed);
        self.backpressure_events.store(0, Ordering::Relaxed);
        // Note: queue_depth is not reset as it represents current state
    }
}

#[derive(Clone)]
pub struct LogBufferSender {
    sender: BroadcastSender<EnrichedLogEntry>,
    config: BufferConfig,
    metrics: Arc<BufferMetricsCollector>,
}

impl LogBufferSender {
    pub async fn send(&self, entry: EnrichedLogEntry) -> Result<(), BufferError> {
        // Check if buffer is full
        let current_depth = self.metrics.queue_depth.load(Ordering::Relaxed);
        if current_depth >= self.config.capacity {
            self.metrics.messages_sent.fetch_add(1, Ordering::Relaxed);
            self.metrics
                .messages_dropped
                .fetch_add(1, Ordering::Relaxed);
            return Err(BufferError::BufferFull);
        }

        // Check backpressure
        if self.config.enable_backpressure {
            let threshold =
                (self.config.capacity as f64 * self.config.backpressure_threshold) as usize;

            if current_depth > threshold {
                self.metrics
                    .backpressure_events
                    .fetch_add(1, Ordering::Relaxed);
                sleep(self.config.backpressure_delay).await;
            }
        }

        // Try to send
        match self.sender.send(entry) {
            Ok(_receiver_count) => {
                self.metrics.messages_sent.fetch_add(1, Ordering::Relaxed);
                self.metrics.queue_depth.fetch_add(1, Ordering::Relaxed);
                Ok(())
            }
            Err(_) => {
                self.metrics.messages_sent.fetch_add(1, Ordering::Relaxed);
                self.metrics
                    .messages_dropped
                    .fetch_add(1, Ordering::Relaxed);
                Err(BufferError::BufferFull)
            }
        }
    }

    pub async fn send_with_timeout(
        &self,
        entry: EnrichedLogEntry,
        timeout_duration: Duration,
    ) -> Result<(), BufferError> {
        timeout(timeout_duration, self.send(entry))
            .await
            .map_err(|_| BufferError::SendTimeout)?
    }
}

pub struct LogBufferReceiver {
    receiver: BroadcastReceiver<EnrichedLogEntry>,
    metrics: Arc<BufferMetricsCollector>,
}

impl LogBufferReceiver {
    pub async fn recv(&mut self) -> Result<EnrichedLogEntry, BufferError> {
        loop {
            match self.receiver.try_recv() {
                Ok(entry) => {
                    self.metrics
                        .messages_received
                        .fetch_add(1, Ordering::Relaxed);
                    // Ensure we don't underflow
                    let current = self.metrics.queue_depth.load(Ordering::Relaxed);
                    if current > 0 {
                        self.metrics.queue_depth.fetch_sub(1, Ordering::Relaxed);
                    }
                    return Ok(entry);
                }
                Err(_) => {
                    // Wait a short time before trying again
                    tokio::task::yield_now().await;
                    sleep(Duration::from_micros(1)).await;
                }
            }
        }
    }

    pub async fn recv_with_timeout(
        &mut self,
        timeout_duration: Duration,
    ) -> Result<Option<EnrichedLogEntry>, BufferError> {
        match timeout(timeout_duration, self.recv()).await {
            Ok(Ok(entry)) => Ok(Some(entry)),
            Ok(Err(e)) => Err(e),
            Err(_) => Ok(None), // Timeout
        }
    }
}

pub struct LogBuffer {
    config: BufferConfig,
    metrics: Arc<BufferMetricsCollector>,
    sender: Option<BroadcastSender<EnrichedLogEntry>>,
    receiver: Option<BroadcastReceiver<EnrichedLogEntry>>,
}

impl LogBuffer {
    pub fn new(capacity: usize) -> Result<Self, BufferError> {
        if capacity == 0 {
            return Err(BufferError::BufferClosed);
        }
        let config = BufferConfig {
            capacity,
            ..Default::default()
        };
        let metrics = Arc::new(BufferMetricsCollector::new());
        let (sender, receiver) = broadcast::channel(capacity);

        Ok(Self {
            config,
            metrics,
            sender: Some(sender),
            receiver: Some(receiver),
        })
    }

    pub async fn new_with_config(config: BufferConfig) -> Result<Self, BufferError> {
        if config.capacity == 0 {
            return Err(BufferError::BufferClosed);
        }
        let metrics = Arc::new(BufferMetricsCollector::new());
        let (sender, receiver) = broadcast::channel(config.capacity);

        Ok(Self {
            config,
            metrics,
            sender: Some(sender),
            receiver: Some(receiver),
        })
    }

    pub fn split(&self) -> (LogBufferSender, LogBufferReceiver) {
        // Clone the existing sender/receiver instead of creating new ones
        let buffer_sender = LogBufferSender {
            sender: self.sender.as_ref().expect("Buffer closed").clone(),
            config: self.config.clone(),
            metrics: self.metrics.clone(),
        };

        let buffer_receiver = LogBufferReceiver {
            receiver: self.sender.as_ref().expect("Buffer closed").subscribe(),
            metrics: self.metrics.clone(),
        };

        (buffer_sender, buffer_receiver)
    }

    pub fn push(&self, entry: impl Into<EnrichedLogEntry>) -> Result<(), BufferError> {
        if let Some(sender) = &self.sender {
            // Check if buffer is full
            let current_depth = self.metrics.queue_depth.load(Ordering::Relaxed);
            if current_depth >= self.config.capacity {
                self.metrics.messages_sent.fetch_add(1, Ordering::Relaxed);
                self.metrics
                    .messages_dropped
                    .fetch_add(1, Ordering::Relaxed);
                return Err(BufferError::BufferFull);
            }

            let enriched_entry = entry.into();
            match sender.send(enriched_entry) {
                Ok(_receiver_count) => {
                    self.metrics.messages_sent.fetch_add(1, Ordering::Relaxed);
                    self.metrics.queue_depth.fetch_add(1, Ordering::Relaxed);
                    Ok(())
                }
                Err(_) => {
                    self.metrics.messages_sent.fetch_add(1, Ordering::Relaxed);
                    self.metrics
                        .messages_dropped
                        .fetch_add(1, Ordering::Relaxed);
                    Err(BufferError::BufferFull)
                }
            }
        } else {
            Err(BufferError::BufferClosed)
        }
    }

    pub fn pop(&mut self) -> Result<EnrichedLogEntry, BufferError> {
        if let Some(receiver) = &mut self.receiver {
            match receiver.try_recv() {
                Ok(entry) => {
                    self.metrics
                        .messages_received
                        .fetch_add(1, Ordering::Relaxed);
                    // Ensure we don't underflow
                    let current = self.metrics.queue_depth.load(Ordering::Relaxed);
                    if current > 0 {
                        self.metrics.queue_depth.fetch_sub(1, Ordering::Relaxed);
                    }
                    Ok(entry)
                }
                Err(_) => Err(BufferError::BufferClosed),
            }
        } else {
            Err(BufferError::BufferClosed)
        }
    }

    pub fn metrics(&self) -> Arc<BufferMetricsCollector> {
        self.metrics.clone()
    }

    pub fn config(&self) -> &BufferConfig {
        &self.config
    }

    pub fn capacity(&self) -> usize {
        self.config.capacity
    }

    pub fn len(&self) -> usize {
        self.metrics.queue_depth.load(Ordering::Relaxed)
    }

    pub fn is_empty(&self) -> bool {
        self.len() == 0
    }

    pub fn detailed_metrics(&self) -> DetailedMetrics {
        let snapshot = self.metrics.snapshot();
        let elapsed = self.metrics.start_time.elapsed();
        let elapsed_secs = elapsed.as_secs_f64();

        // Use safe arithmetic to prevent overflow
        let memory_usage_bytes = snapshot.queue_depth
            .saturating_mul(std::mem::size_of::<EnrichedLogEntry>());

        DetailedMetrics {
            capacity: self.config.capacity,
            len: snapshot.queue_depth,
            pushed: snapshot.messages_sent,
            popped: snapshot.messages_received,
            dropped: snapshot.messages_dropped,
            memory_usage_bytes,
            throughput_per_second: if elapsed_secs > 0.0 {
                snapshot.messages_sent as f64 / elapsed_secs
            } else {
                0.0
            },
            average_latency_ns: 0, // Not tracked in this implementation
            peak_queue_size: snapshot.queue_depth, // Simplified: current size as peak
            fill_ratio: snapshot.queue_depth as f64 / self.config.capacity as f64,
        }
    }

    pub fn reset_metrics(&self) {
        self.metrics.reset();
    }

    // Additional methods expected by tests
    pub fn try_pop(&mut self) -> Option<EnrichedLogEntry> {
        self.pop().ok()
    }

    pub fn is_full(&self) -> bool {
        self.len() >= self.capacity()
    }

    pub fn fill_ratio(&self) -> f64 {
        self.len() as f64 / self.capacity() as f64
    }

    pub fn needs_backpressure(&self) -> bool {
        self.fill_ratio() >= self.config.backpressure_threshold
    }

    pub fn backpressure_level(&self) -> crate::buffer::backpressure::BackpressureLevel {
        let ratio = self.fill_ratio();
        if ratio >= 0.95 {
            crate::buffer::backpressure::BackpressureLevel::High
        } else if ratio >= 0.8 {
            crate::buffer::backpressure::BackpressureLevel::Medium
        } else if ratio >= 0.5 {
            crate::buffer::backpressure::BackpressureLevel::Low
        } else {
            crate::buffer::backpressure::BackpressureLevel::None
        }
    }

    pub async fn push_with_strategy(
        &self,
        entry: impl Into<EnrichedLogEntry> + Clone,
        strategy: crate::buffer::backpressure::BackpressureStrategy,
    ) -> Result<(), BufferError> {
        use crate::buffer::backpressure::BackpressureStrategy;

        match strategy {
            BackpressureStrategy::Drop => {
                // Try once, if it fails, drop immediately
                self.push(entry)
            }
            BackpressureStrategy::Yield => {
                // Try once, if it fails, yield and try again once
                match self.push(entry.clone()) {
                    Ok(()) => Ok(()),
                    Err(BufferError::BufferFull) => {
                        tokio::task::yield_now().await;
                        self.push(entry)
                    }
                    Err(e) => Err(e),
                }
            }
            BackpressureStrategy::Sleep(duration) => {
                // Try once, if it fails, sleep and try again
                match self.push(entry.clone()) {
                    Ok(()) => Ok(()),
                    Err(BufferError::BufferFull) => {
                        tokio::time::sleep(duration).await;
                        self.push(entry)
                    }
                    Err(e) => Err(e),
                }
            }
            BackpressureStrategy::Block => {
                // Keep trying until success or permanent error
                loop {
                    match self.push(entry.clone()) {
                        Ok(()) => return Ok(()),
                        Err(BufferError::BufferFull) => {
                            tokio::task::yield_now().await;
                            continue;
                        }
                        Err(e) => return Err(e),
                    }
                }
            }
        }
    }
}

// SAFE: These types are automatically Send+Sync because:
// - BroadcastSender<EnrichedLogEntry> is Send+Sync (tokio guarantees this)
// - BroadcastReceiver<EnrichedLogEntry> is Send+Sync (tokio guarantees this)
// - Arc<BufferMetricsCollector> is Send+Sync (Arc provides this for thread-safe contents)
// - BufferConfig contains only primitive types which are Send+Sync
// - Option<T> is Send+Sync when T is Send+Sync
// 
// No unsafe implementations needed - Rust's type system automatically derives
// Send+Sync for these types based on their components.
