pub mod backpressure;
pub mod batch;
pub mod concurrency;
pub mod error;
pub mod lockfree;
pub mod memory;
pub mod metrics;

pub use batch::{Batch, BatchConfig, BatchFormer, BatchType};
pub use concurrency::{ConcurrencyError, RecoveryStrategy, RobustMutex, RobustRwLock};
pub use error::{
    BufferError, ErrorRecovery, MetricsError, ParseError, safe_buffer_operation,
    safe_metrics_operation, safe_parse_operation,
};
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

    pub fn batch_former(&self) -> &BatchFormer {
        &self.batch_former
    }

    pub fn memory_manager(&self) -> &MemoryManager {
        &self.memory_manager
    }
}

pub use backpressure::{BackpressureLevel, BackpressureStrategy};
pub use metrics::DetailedMetrics;
