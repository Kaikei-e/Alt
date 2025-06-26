
#[derive(Debug, Clone)]
pub struct BufferMetrics {
    pub capacity: usize,
    pub len: usize,
    pub pushed: u64,
    pub popped: u64,
    pub dropped: u64,
    pub memory_usage_bytes: usize,
}

#[derive(Debug, Clone)]
pub struct DetailedMetrics {
    pub capacity: usize,
    pub len: usize,
    pub pushed: u64,
    pub popped: u64,
    pub dropped: u64,
    pub memory_usage_bytes: usize,
    pub throughput_per_second: f64,
    pub average_latency_ns: u64,
    pub peak_queue_size: usize,
    pub fill_ratio: f64,
}