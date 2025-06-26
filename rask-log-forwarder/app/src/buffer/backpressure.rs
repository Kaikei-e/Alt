use tokio::time::Duration;

#[derive(Debug, Clone, Copy)]
pub enum BackpressureStrategy {
    Sleep(Duration),
    Yield,
    Drop,
    Block,
}

impl Default for BackpressureStrategy {
    fn default() -> Self {
        BackpressureStrategy::Sleep(Duration::from_micros(100))
    }
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum BackpressureLevel {
    None,
    Low,
    Medium,
    High,
}