use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::time::Duration;

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum MemoryPressure {
    None,
    Warning,
    Critical,
}

#[derive(Debug, Clone)]
pub struct MemoryConfig {
    pub max_memory: usize,
    pub warning_threshold: f64,
    pub critical_threshold: f64,
}

impl Default for MemoryConfig {
    fn default() -> Self {
        Self {
            max_memory: 100 * 1024 * 1024, // 100MB
            warning_threshold: 0.8,
            critical_threshold: 0.95,
        }
    }
}

#[derive(Debug)]
pub struct BackpressureDecision {
    pub delay: Duration,
    pub should_drop: bool,
}

pub struct MemoryManager {
    config: MemoryConfig,
    current_usage: Arc<AtomicUsize>,
}

impl MemoryManager {
    pub fn new(config: MemoryConfig) -> Self {
        Self {
            config,
            current_usage: Arc::new(AtomicUsize::new(0)),
        }
    }

    pub async fn allocate(&self, size: usize) {
        self.current_usage.fetch_add(size, Ordering::Relaxed);
    }

    pub async fn deallocate(&self, size: usize) {
        self.current_usage.fetch_sub(size, Ordering::Relaxed);
    }

    pub fn current_pressure(&self) -> MemoryPressure {
        let current = self.current_usage.load(Ordering::Relaxed);
        let usage_ratio = current as f64 / self.config.max_memory as f64;

        if usage_ratio >= self.config.critical_threshold {
            MemoryPressure::Critical
        } else if usage_ratio >= self.config.warning_threshold {
            MemoryPressure::Warning
        } else {
            MemoryPressure::None
        }
    }

    pub fn calculate_backpressure(&self) -> BackpressureDecision {
        match self.current_pressure() {
            MemoryPressure::None => BackpressureDecision {
                delay: Duration::ZERO,
                should_drop: false,
            },
            MemoryPressure::Warning => BackpressureDecision {
                delay: Duration::from_millis(1),
                should_drop: false,
            },
            MemoryPressure::Critical => BackpressureDecision {
                delay: Duration::from_millis(10),
                should_drop: true,
            },
        }
    }

    pub fn memory_usage(&self) -> usize {
        self.current_usage.load(Ordering::Relaxed)
    }

    pub fn memory_usage_ratio(&self) -> f64 {
        self.memory_usage() as f64 / self.config.max_memory as f64
    }
}
