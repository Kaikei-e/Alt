pub mod backpressure;
pub mod batch;
pub mod concurrency;
pub mod error;
pub mod lockfree;
pub mod memory;
pub mod metrics;
pub mod queue;

// New TASK3 exports
pub use batch::{Batch, BatchConfig, BatchFormer, BatchType};
pub use concurrency::{ConcurrencyError, RecoveryStrategy, RobustMutex, RobustRwLock};
pub use error::{BufferError, MetricsError, ParseError, ErrorRecovery, safe_buffer_operation, safe_metrics_operation, safe_parse_operation};
pub use lockfree::{
    BufferConfig, BufferMetrics, BufferMetricsCollector, LogBuffer, LogBufferReceiver,
    LogBufferSender,
};
pub use memory::{BackpressureDecision, MemoryConfig, MemoryManager, MemoryPressure};

// Integration struct that combines all buffer components
pub struct BufferManager {
    buffer: LogBuffer,
    batch_former: BatchFormer,
    memory_manager: MemoryManager,
}

impl BufferManager {
    pub async fn new(
        buffer_config: BufferConfig,
        batch_config: BatchConfig,
        memory_config: MemoryConfig,
    ) -> Result<Self, BufferError> {
        Ok(Self {
            buffer: LogBuffer::new_with_config(buffer_config).await?,
            batch_former: BatchFormer::new(batch_config),
            memory_manager: MemoryManager::new(memory_config),
        })
    }

    pub fn split(&self) -> Result<(LogBufferSender, LogBufferReceiver), BufferError> {
        self.buffer.split()
    }
    
    /// Legacy split method for backward compatibility
    pub fn split_legacy(&self) -> (LogBufferSender, LogBufferReceiver) {
        self.buffer.split_legacy()
    }

    pub fn batch_former(&self) -> &BatchFormer {
        &self.batch_former
    }

    pub fn memory_manager(&self) -> &MemoryManager {
        &self.memory_manager
    }
}

// Legacy exports (keep for compatibility)
pub use backpressure::{BackpressureLevel, BackpressureStrategy};
pub use metrics::{BufferMetrics as LegacyBufferMetrics, DetailedMetrics};
pub use queue::LogBuffer as LegacyLogBuffer;
