# rask/CLAUDE.md

<!-- Model Configuration -->
<!-- ALWAYS use claude-4-sonnet for this project -->
<!-- Use 'think' for basic analysis, 'ultrathink' for complex architectural decisions -->

## About Rask

**Rask** (Norwegian for "fast") is a high-performance log aggregation and processing service for the Alt RSS reader microservice ecosystem. Built with **Rust 1.87+**, **Axum framework**, and **Clean Architecture** principles, it collects, processes, and analyzes logs from all Alt microservices in real-time.

**Core Responsibilities:**
- Real-time log ingestion from multiple Alt microservices
- High-throughput log parsing and enrichment
- Structured log storage and indexing
- Real-time alerting and anomaly detection
- Log querying and analytics API

**Critical Guidelines:**
- **Performance First:** Optimize for high-throughput, low-latency processing
- **TDD Approach:** Always write failing tests BEFORE implementation
  - RED, GREEN, REFACTOR
  - RED: Write failing test
  - GREEN: Write code to pass the test
  - REFACTOR: Refactor the code to be more readable and maintainable
  - REPEAT
- **Quality Over Speed:** Prevent regressions and maintain code quality
- **Rate Limiting:** External API calls must have minimum 5-second intervals
- **Clean Architecture:** Strict layer dependencies and separation of concerns
- **Zero Data Loss:** Ensure reliable log delivery with proper buffering
- **Clean Architecture:** Strict layer dependencies and separation of concerns

## Architecture Overview

### Five-Layer Clean Architecture
```
REST/gRPC Handler → Usecase → Port → Gateway (ACL) → Driver
```

**Layer Dependencies (Dependency Rule):**
- **REST/gRPC:** API handlers, log ingestion endpoints → depends on Usecase
- **Usecase:** Log processing logic, analytics → depends on Port
- **Port:** Interface definitions (traits) → depends on Gateway
- **Gateway:** Anti-corruption layer, format translation → depends on Driver
- **Driver:** External integrations (storage, queues, etc.) → no dependencies

### Directory Structure
```
/rask/
├── Cargo.toml         # Project manifest
├── src/
│   ├── main.rs        # Application entry point
│   ├── api/           # API layer (REST + gRPC)
│   │   ├── mod.rs
│   │   ├── rest/      # HTTP endpoints
│   │   │   ├── handler.rs
│   │   │   └── schema.rs
│   │   └── grpc/      # gRPC services
│   │       ├── proto/
│   │       └── service.rs
│   ├── usecase/       # Business logic
│   │   ├── mod.rs
│   │   ├── ingest_log/
│   │   ├── process_log/
│   │   ├── query_log/
│   │   └── alert/
│   ├── port/          # Trait definitions
│   │   ├── mod.rs
│   │   ├── log_repository.rs
│   │   ├── log_processor.rs
│   │   ├── alert_notifier.rs
│   │   └── metrics_collector.rs
│   ├── gateway/       # Anti-corruption layer
│   │   ├── mod.rs
│   │   ├── log_parser/
│   │   ├── storage_gateway/
│   │   └── notification_gateway/
│   ├── driver/        # External integrations
│   │   ├── mod.rs
│   │   ├── kafka/     # Message queue
│   │   ├── clickhouse/# Time-series storage
│   │   ├── redis/     # Cache & buffer
│   │   └── s3/        # Long-term storage
│   ├── domain/        # Core entities
│   │   ├── mod.rs
│   │   ├── log_entry.rs
│   │   ├── log_level.rs
│   │   ├── service_id.rs
│   │   └── alert_rule.rs
│   ├── di/            # Dependency injection
│   │   └── container.rs
│   └── utils/         # Cross-cutting concerns
│       ├── mod.rs
│       ├── buffer/    # Ring buffer for logs
│       ├── parser/    # High-perf parsers
│       └── metrics/   # Performance metrics
├── proto/             # Protocol buffer definitions
├── benches/           # Performance benchmarks
├── tests/             # Integration tests
└── CLAUDE.md          # This file
```

## Performance-Critical Design

### Zero-Copy Log Processing
```rust
use bytes::{Bytes, BytesMut};
use nom::IResult;

// Zero-copy log entry using Bytes
#[derive(Clone)]
pub struct LogEntry {
    timestamp: i64,
    level: LogLevel,
    service: Bytes,     // Zero-copy reference
    message: Bytes,     // Zero-copy reference
    fields: HashMap<Bytes, Bytes>,
}

// High-performance parser using nom
pub fn parse_log_line(input: &[u8]) -> IResult<&[u8], LogEntry> {
    // Parse without allocating new strings
    // ...
}
```

### Lock-Free Data Structures
```rust
use crossbeam::channel::{bounded, Receiver, Sender};
use parking_lot::RwLock;

pub struct LogBuffer {
    // SPSC channel for high-throughput
    sender: Sender<Bytes>,
    receiver: Receiver<Bytes>,
    // Lock-free metrics
    processed: AtomicU64,
    dropped: AtomicU64,
}

impl LogBuffer {
    pub fn new(capacity: usize) -> Self {
        let (sender, receiver) = bounded(capacity);
        Self {
            sender,
            receiver,
            processed: AtomicU64::new(0),
            dropped: AtomicU64::new(0),
        }
    }
}
```

### Vectorized Processing
```rust
use rayon::prelude::*;

pub struct BatchProcessor {
    batch_size: usize,
}

impl BatchProcessor {
    pub async fn process_batch(&self, logs: Vec<Bytes>) -> Result<(), ProcessError> {
        // Parallel processing with rayon
        let results: Vec<_> = logs
            .par_chunks(self.batch_size)
            .map(|chunk| self.process_chunk(chunk))
            .collect();

        // Check for errors
        results.into_iter().collect::<Result<Vec<_>, _>>()?;
        Ok(())
    }
}
```

## High-Performance Patterns

### Efficient Log Ingestion
```rust
use axum::extract::ws::{WebSocket, WebSocketUpgrade};
use tokio::sync::mpsc;

pub struct IngestHandler {
    buffer: Arc<LogBuffer>,
    processor: Arc<BatchProcessor>,
}

impl IngestHandler {
    // HTTP endpoint for batch ingestion
    pub async fn ingest_batch(
        State(handler): State<Arc<IngestHandler>>,
        body: Bytes,
    ) -> Result<impl IntoResponse, AppError> {
        // Parse logs without copying
        let logs = parse_log_batch(&body)?;

        // Send to buffer
        for log in logs {
            if let Err(_) = handler.buffer.try_send(log) {
                handler.buffer.dropped.fetch_add(1, Ordering::Relaxed);
            }
        }

        Ok(StatusCode::ACCEPTED)
    }

    // WebSocket for real-time streaming
    pub async fn stream_logs(
        ws: WebSocketUpgrade,
        State(handler): State<Arc<IngestHandler>>,
    ) -> impl IntoResponse {
        ws.on_upgrade(|socket| handle_socket(socket, handler))
    }
}
```

### Memory-Efficient Storage
```rust
// Use memory-mapped files for temporary storage
use memmap2::MmapMut;

pub struct LogStore {
    mmap: MmapMut,
    write_pos: AtomicUsize,
    capacity: usize,
}

impl LogStore {
    pub fn append(&self, data: &[u8]) -> Result<(), StoreError> {
        let len = data.len();
        let pos = self.write_pos.fetch_add(len, Ordering::SeqCst);

        if pos + len > self.capacity {
            return Err(StoreError::CapacityExceeded);
        }

        // Zero-copy write
        unsafe {
            std::ptr::copy_nonoverlapping(
                data.as_ptr(),
                self.mmap.as_mut_ptr().add(pos),
                len
            );
        }

        Ok(())
    }
}
```

## Log Processing Pipeline

### Stream Processing Architecture
```rust
use futures::stream::{Stream, StreamExt};

pub struct LogPipeline {
    parser: Arc<LogParser>,
    enricher: Arc<LogEnricher>,
    indexer: Arc<LogIndexer>,
}

impl LogPipeline {
    pub fn create_pipeline(&self) -> impl Stream<Item = Result<ProcessedLog, PipelineError>> {
        // Create processing pipeline
        self.create_source()
            .map(|raw| self.parser.parse(raw))
            .try_filter_map(|parsed| async move {
                // Filter out noise
                if should_process(&parsed) {
                    Ok(Some(parsed))
                } else {
                    Ok(None)
                }
            })
            .map(|result| {
                result.and_then(|log| self.enricher.enrich(log))
            })
            .buffer_unordered(100) // Parallel enrichment
            .map(|result| {
                result.and_then(|log| self.indexer.index(log))
            })
    }
}
```

### Real-time Analytics
```rust
use dashmap::DashMap;

pub struct MetricsCollector {
    // Lock-free concurrent hashmap
    service_metrics: DashMap<ServiceId, ServiceMetrics>,
    error_rates: DashMap<(ServiceId, ErrorType), AtomicU64>,
}

impl MetricsCollector {
    pub fn record_log(&self, entry: &LogEntry) {
        // Update metrics without locks
        let mut metrics = self.service_metrics
            .entry(entry.service_id.clone())
            .or_insert_with(ServiceMetrics::new);

        metrics.total_logs.fetch_add(1, Ordering::Relaxed);

        if entry.level >= LogLevel::Error {
            metrics.error_count.fetch_add(1, Ordering::Relaxed);

            // Track error types
            if let Some(error_type) = extract_error_type(entry) {
                self.error_rates
                    .entry((entry.service_id.clone(), error_type))
                    .or_insert_with(|| AtomicU64::new(0))
                    .fetch_add(1, Ordering::Relaxed);
            }
        }
    }
}
```

## Testing Strategy

### Performance Testing
```rust
#[cfg(test)]
mod benches {
    use criterion::{black_box, criterion_group, criterion_main, Criterion};

    fn benchmark_log_parsing(c: &mut Criterion) {
        let sample_log = b"2024-01-01T00:00:00Z INFO alt-backend Request processed id=123";

        c.bench_function("parse_single_log", |b| {
            b.iter(|| {
                parse_log_line(black_box(sample_log))
            });
        });
    }

    fn benchmark_batch_processing(c: &mut Criterion) {
        let logs: Vec<_> = (0..10000)
            .map(|i| format!("2024-01-01T00:00:00Z INFO service-{} Message", i))
            .collect();

        c.bench_function("process_10k_logs", |b| {
            b.iter(|| {
                tokio::runtime::Runtime::new()
                    .unwrap()
                    .block_on(process_batch(black_box(&logs)))
            });
        });
    }
}
```

### Load Testing
```rust
#[tokio::test]
async fn test_high_throughput_ingestion() {
    let handler = create_test_handler();
    let client = TestClient::new(handler);

    // Simulate 1M logs/second
    let handles: Vec<_> = (0..100)
        .map(|thread_id| {
            let client = client.clone();
            tokio::spawn(async move {
                for batch in 0..100 {
                    let logs = generate_log_batch(1000);
                    client.ingest_batch(logs).await.unwrap();
                }
            })
        })
        .collect();

    let start = Instant::now();
    futures::future::join_all(handles).await;
    let duration = start.elapsed();

    let total_logs = 100 * 100 * 1000;
    let throughput = total_logs as f64 / duration.as_secs_f64();

    assert!(throughput > 900_000.0, "Throughput too low: {}", throughput);
}
```

## Monitoring & Observability

### Self-Monitoring
```rust
pub struct HealthMonitor {
    ingestion_rate: Arc<RwLock<RateCounter>>,
    processing_lag: Arc<AtomicU64>,
    error_rate: Arc<RwLock<RateCounter>>,
}

impl HealthMonitor {
    pub async fn health_check(&self) -> HealthStatus {
        let ingestion = self.ingestion_rate.read().rate_per_second();
        let lag = self.processing_lag.load(Ordering::Relaxed);
        let errors = self.error_rate.read().rate_per_second();

        HealthStatus {
            status: if lag > 10_000 { "degraded" } else { "healthy" },
            ingestion_rate: ingestion,
            processing_lag_ms: lag,
            error_rate: errors,
            timestamp: chrono::Utc::now(),
        }
    }
}
```

## Configuration

### Performance Tuning
```toml
[performance]
# Buffer sizes
ingest_buffer_size = 1_000_000
process_batch_size = 10_000
output_buffer_size = 100_000

# Threading
worker_threads = 16
blocking_threads = 4

# Memory limits
max_memory_gb = 8
mmap_size_gb = 2

# Network
tcp_nodelay = true
keep_alive_secs = 30

```

## Rust 2024 Edition — **CRITICAL**: Mandatory Rules for Claude Code

1. **Create projects on the 2024 edition by default**

   ```bash
   cargo new --edition=2024 <name>
   ```

   The Cargo Book lists 2024 as the current default when `--edition` is omitted. ([doc.rust-lang.org][1])

2. **Upgrade existing crates in three steps**

   ```bash
   cargo update          # latest deps
   cargo fix --edition   # automatic rewrite
   cargo clippy --deny warnings
   ```

   The official Edition Guide and community upgrade tutorials recommend this exact flow. ([doc.rust-lang.org][2], [codeandbitters.com][3])

3. **Keep the build warning-free**
   Add to the crate root:

   ```rust
   #![deny(warnings, rust_2024_idioms)]
   ```

   The new `rust_2024_idioms` lint group enforces edition-specific hygiene. ([github.com][4], [doc.rust-lang.org][5])

4. **Use `async fn` and return-position `impl Trait` directly in traits** — drop `async_trait` and manual wrappers. These features are now stable and ergonomic. ([blog.rust-lang.org][6], [smallcultfollowing.com][7], [blog.rust-lang.org][8])

5. **Eliminate `static mut`**
   The 2024 edition turns `static_mut_refs` into a hard error; replace global mutables with `OnceCell`, `Mutex`, or other interior-mutability types. ([doc.rust-lang.org][9], [users.rust-lang.org][10])

6. **Maintain cross-edition compatibility for published libraries**
   Compile your crate with both `--edition 2021` and `--edition 2024` in CI to avoid accidental breaking changes. Practical upgrade notes highlight this need. ([codeandbitters.com][3])

7. **Automate dependency freshness**
   Install `cargo-outdated`, run it in CI, and follow with `cargo update` to pull patched versions. ([github.com][11], [doc.rust-lang.org][12], [rustprojectprimer.com][13])

8. **Deny unsafe reference creation inside unsafe functions**
   Tighten unsafety by adding `#![deny(unsafe_op_in_unsafe_fn)]`, a pattern encouraged for 2024 code bases. ([github.com][4])

9. **Enforce edition hygiene continuously**
   In CI (GitHub Actions, GitLab CI, etc.) run:

   ```bash
   cargo +stable fix --edition --check
   cargo clippy --all-targets --all-features --deny warnings
   ```

   This catches drift the moment it happens. ([doc.rust-lang.org][2], [github.com][4])

10. **Document the edition in every public snippet**
    When you share code, include `edition = "2024"` in the `Cargo.toml` example to prevent accidental fallback to older semantics. ([doc.rust-lang.org][1])


[1]: https://doc.rust-lang.org/cargo/commands/cargo-new.html?utm_source=chatgpt.com "cargo new - The Cargo Book - Rust Documentation"
[2]: https://doc.rust-lang.org/edition-guide/editions/transitioning-an-existing-project-to-a-new-edition.html?utm_source=chatgpt.com "Transitioning an existing project to a new edition"
[3]: https://codeandbitters.com/rust-2024-upgrade/?utm_source=chatgpt.com "updating a large codebase to Rust 2024 edition - Code and Bitters"
[4]: https://github.com/rust-lang/rust/issues/131725?utm_source=chatgpt.com "Elided lifetime changes in `rust_2018_idioms` lint is very noisy and ..."
[5]: https://doc.rust-lang.org/beta/rustc/lints/groups.html?utm_source=chatgpt.com "Lint Groups - The rustc book - Rust Documentation"
[6]: https://blog.rust-lang.org/2023/12/21/async-fn-rpit-in-traits.html?utm_source=chatgpt.com "Announcing `async fn` and return-position `impl Trait` in traits"
[7]: https://smallcultfollowing.com/babysteps/blog/2024/01/03/async-rust-2024/?utm_source=chatgpt.com "What I'd like to see for Async Rust in 2024 · baby steps"
[8]: https://blog.rust-lang.org/2024/09/05/impl-trait-capture-rules.html?utm_source=chatgpt.com "Changes to `impl Trait` in Rust 2024"
[9]: https://doc.rust-lang.org/edition-guide/rust-2024/static-mut-references.html?utm_source=chatgpt.com "Disallow references to static mut - The Rust Edition Guide"
[10]: https://users.rust-lang.org/t/whats-the-correct-way-of-doing-static-mut-in-2024-rust/120403?utm_source=chatgpt.com "What's the \"correct\" way of doing static mut in 2024 Rust? - embedded"
[11]: https://github.com/kbknapp/cargo-outdated?utm_source=chatgpt.com "kbknapp/cargo-outdated - GitHub"
[12]: https://doc.rust-lang.org/cargo/commands/cargo-update.html?utm_source=chatgpt.com "cargo update - The Cargo Book - Rust Documentation"
[13]: https://rustprojectprimer.com/checks/outdated.html?utm_source=chatgpt.com "Outdated Dependencies - Rust Project Primer"


## Dependencies

```toml
[dependencies]
# Web framework
axum = { version = "0.8", features = ["ws"] }
tower = "0.5"
tower-http = { version = "0.6", features = ["trace", "compression"] }

# Async runtime
tokio = { version = "1.40", features = ["full"] }
futures = "0.3"

# Performance
rayon = "1.10"
crossbeam = "0.8"
parking_lot = "0.12"
dashmap = "6.0"

# Serialization
serde = { version = "1.0", features = ["derive"] }
bincode = "1.3"
rmp-serde = "1.3"

# Parsing
nom = "7.1"
regex = "1.10"

# Storage
clickhouse = "0.12"
redis = { version = "0.27", features = ["tokio-comp", "connection-manager"] }
aws-sdk-s3 = "1.0"

# Memory
bytes = "1.7"
memmap2 = "0.9"

# Metrics
prometheus = "0.13"
metrics = "0.23"

# Logging (meta!)
tracing = "0.1"
tracing-subscriber = { version = "0.3", features = ["env-filter", "json"] }

# Error handling
thiserror = "1.0"

[dev-dependencies]
criterion = { version = "0.5", features = ["html_reports"] }
proptest = "1.5"
tempfile = "3.12"

[profile.release]
lto = "fat"
codegen-units = 1
opt-level = 3
```

## Performance Goals

### Target Metrics
- **Ingestion Rate:** > 1M logs/second sustained
- **Processing Latency:** < 10ms p99
- **Memory Usage:** < 10GB for 1M logs/sec
- **Query Response:** < 100ms for time-range queries
- **Zero Data Loss:** 99.999% delivery guarantee

### Optimization Techniques
1. **Zero-Copy Parsing:** Use `bytes::Bytes` throughout
2. **Lock-Free Structures:** `crossbeam` channels, `dashmap`
3. **Vectorization:** SIMD operations where applicable
4. **Memory Pooling:** Reuse allocations
5. **Batch Processing:** Amortize syscall overhead
6. **Compression:** LZ4 for network, Zstd for storage

## References

- [High Performance Rust](https://www.oreilly.com/library/view/rust-high-performance/9781788399487/)
- [The Rust Performance Book](https://nnethercote.github.io/perf-book/)
- [Lock-free Programming in Rust](https://github.com/crossbeam-rs/crossbeam)
- [Building Observable Systems](https://www.honeycomb.io/blog/building-observable-systems)
- [ClickHouse Best Practices](https://clickhouse.com/docs/en/operations/tips)
- [Zero-Copy in Rust](https://manishearth.github.io/blog/2022/08/03/zero-copy-1-not-a-yoking-matter/)