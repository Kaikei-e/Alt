# rask-log-forwarder/CLAUDE.md

<!-- Model Configuration -->
<!-- ALWAYS use claude-4-sonnet for this project -->
<!-- Use 'think' for basic analysis, 'ultrathink' for complex architectural decisions -->

## About rask-log-forwarder

**rask-log-forwarder** is an ultra-high-performance sidecar container that runs alongside each Alt microservice, tailing its stdout/stderr logs, performing zero-copy parsing, and forwarding logs in batches to the Rask aggregation server. Built with **Rust 1.87+** (2024 edition) using SIMD JSON parsing, lock-free data structures, and efficient streaming protocols.

**Deployment Architecture:**
- **One forwarder per service**: Each Alt service has its own dedicated log forwarder instance
- **Network namespace sharing**: Uses `network_mode: "service:xxx"` to share the target service's network
- **Direct log access**: Mounts Docker's json-file logs for zero-copy reading

**Target Services:**
- **nginx-logs** → collects from nginx
- **alt-frontend-logs** → collects from alt-frontend
- **alt-backend-logs** → collects from alt-backend
- **news-creator-logs** → collects from news-creator (Ollama)
- **pre-processor-logs** → collects from pre-processor
- **search-indexer-logs** → collects from search-indexer
- **tag-generator-logs** → collects from tag-generator
- **meilisearch-logs** → collects from meilisearch
- **db-logs** → collects from PostgreSQL

**Core Capabilities:**
- Zero-copy log collection from Docker json-file driver
- Generic log parsing supporting multiple formats (JSON, plain text, nginx format)
- Service-aware enrichment using Docker labels (`rask.group`)
- SIMD-accelerated JSON parsing (>4 GB/s throughput)
- Lock-free buffering with tokio::sync::broadcast (8-9M msgs/sec)
- Batch transmission via HTTP/1.1 keep-alive or gRPC streaming
- Guaranteed delivery with exponential backoff and disk fallback

**Performance Targets (per forwarder instance):**
- **Throughput:** >100K logs/second with <20% CPU usage
- **Memory:** <16MB per forwarder instance
- **Latency:** <5ms p99 forwarding latency
- **Batch Size:** 10,000 logs per request
- **Zero Data Loss:** Disk persistence for failed transmissions

**Critical Guidelines:**
- **Performance First:** Optimize for high-throughput, low-latency processing
- **TDD Approach:** RED → GREEN → REFACTOR cycle
- **Rust 2024 Edition:** Use latest edition features and idioms
- **Zero Data Loss:** Ensure reliable log delivery with proper buffering

### Rust Coding Workflow for Claude Code (2025-06-26): **CRITICAL**

1. **Workspace discovery**
   - Always launch Claude Code from the directory that contains a *visible* `Cargo.toml`, **or** set
     ```jsonc
     "rust-analyzer.linkedProjects": ["rask-log-forwarder/app/Cargo.toml"]
     ```
     in your editor settings. This removes 90 % of “failed to fetch/discover workspace” errors. :contentReference[oaicite:1]{index=1}

2. **Toolchain baseline**
   - Pin a local compiler and warm the cache once:
     ```bash
     rustup override set stable   # or 1.79.0
     cargo check
     ```
     Claude re-uses this cache to speed up subsequent `cargo` invocations. :contentReference[oaicite:2]{index=2}

3. **Context hygiene**
   - Add `target/`, `node_modules/`, `dist/`, media files, and secrets to `.cursorindexingignore` / `.cursorignore`.
     This frees the context window for type definitions rather than junk. :contentReference[oaicite:3]{index=3}

4. **Test-driven micro-diff cadence**
   - **/write-tests \<PATH\>** – generate failing unit + edge-case tests first. :contentReference[oaicite:4]{index=4}
   - **/fix-types** – run `cargo check --message-format=short`; patch lifetimes / borrows only, with diffs ≤ 150 tokens. :contentReference[oaicite:5]{index=5}
   - **/impl-feature** – implement until all tests pass; stop on green. :contentReference[oaicite:6]{index=6}

5. **Borrow-checker repair loop**
   - On any compiler error burst, re-run **/fix-types**.
   - If the same diagnostic repeats twice, paste it back and prompt:
     > “Think step by step—what minimal change fixes this lifetime without cloning?”
     This deep-reasoning pass resolves most stubborn issues. :contentReference[oaicite:7]{index=7}

6. **Safety & style rules**
   - Prefer safe Rust; introduce `unsafe` blocks *only* when explicitly requested.
   - Follow edition 2024, enable `clippy::pedantic`, and use `thiserror`/`anyhow` for error handling. :contentReference[oaicite:8]{index=8}

7. **Common recovery steps**
   | Symptom | Action |
   |---------|--------|
   | “Codebase not indexed yet…” | Clear Codebase Index → Compute Index |
   | “failed to discover workspace” | Verify `Cargo.toml` visibility or `linkedProjects` path, then restart Claude Code | :contentReference[oaicite:9]{index=9} |

8. **Slash-command reference**

   | Command | Purpose |
   |---------|---------|
   | **/write-tests {PATH}** | Generate failing tests (no code changes). |
   | **/fix-types** | Auto-patch lifetimes/borrows until `cargo check` is clean. |
   | **/impl-feature** | Implement code, looping with micro-diffs until tests pass. |
   | **/cost** | Show current token / USD spend. |

> Follow this sequence—workspace ↔ hygiene ↔ TDD micro-diff—and Claude Code should compile Rust on the first or second pass instead of drowning in type errors. Happy hacking! :contentReference[oaicite:10]{index=10}

## Architecture Overview

### Data Flow Pipeline
```
Docker json-file → Tail (Bollard) → Zero-copy Bytes → SIMD Parse → Lock-free Queue → Batch & Send
```

### Component Responsibilities

| Layer | Responsibility |
|-------|---------------|
| **Collection** | Tail Docker container logs via Unix socket, auto-discover services |
| **Parsing** | Zero-copy intake with `Bytes`, SIMD decode, format detection |
| **Buffering** | Lock-free MPMC queue with `multiqueue`, no waits at 8-9M msg/s |
| **Transmission** | Stream batches via `hyper`/`reqwest`, keep-alive for minimal latency |
| **Retry** | Failed sends → exponential backoff → disk fallback with `sled` |
| **Config** | CLI args + env vars unified with `clap` env feature |

### Directory Structure
```
/rask-log-forwarder/
├── Cargo.toml          # Dependencies with edition = "2024"
├── src/
│   ├── main.rs         # Entry point, tokio runtime setup
│   ├── collector/      # Docker log collection
│   │   ├── mod.rs
│   │   ├── docker.rs   # Bollard client, container discovery
│   │   └── monitor.rs  # Container lifecycle events
│   ├── parser/         # Zero-copy JSON parsing
│   │   ├── mod.rs
│   │   ├── universal.rs # Multi-format parser
│   │   ├── simd.rs     # SIMD-JSON implementation
│   │   └── formats/    # Service-specific parsers
│   │       ├── go.rs
│   │       ├── nginx.rs
│   │       └── postgres.rs
│   ├── buffer/         # Lock-free queueing
│   │   ├── mod.rs
│   │   └── queue.rs    # Multiqueue wrapper
│   ├── sender/         # Batch transmission
│   │   ├── mod.rs
│   │   ├── http.rs     # HTTP/1.1 client
│   │   └── retry.rs    # Backoff and persistence
│   └── config/         # Configuration management
│       └── mod.rs
├── benches/            # Performance benchmarks
└── tests/              # Integration tests
```

## High-Performance Implementation

### Zero-Copy Log Collection (Sidecar Pattern)
```rust
#![deny(warnings, rust_2024_idioms)]

use bollard::{Docker, container::LogsOptions};
use bytes::Bytes;
use futures::StreamExt;
use std::env;

pub struct DockerCollector {
    docker: Docker,
    target_service: String,  // Service name from environment
}

impl DockerCollector {
    pub fn new() -> Result<Self, CollectorError> {
        // Each forwarder instance targets a specific service
        let target_service = env::var("TARGET_SERVICE")
            .or_else(|_| {
                // Extract service name from hostname (e.g., "nginx-logs" -> "nginx")
                hostname::get()
                    .ok()
                    .and_then(|h| h.to_str().map(|s| s.to_string()))
                    .map(|h| h.trim_end_matches("-logs").to_string())
            })
            .ok_or(CollectorError::NoTargetService)?;

        Ok(Self {
            docker: Docker::connect_with_unix_defaults()?,
            target_service,
        })
    }

    pub async fn tail_target_service(
        &self,
        tx: multiqueue::BroadcastSender<(Bytes, ContainerInfo)>,
    ) -> Result<(), CollectorError> {
        let options = LogsOptions::<String> {
            stdout: true,
            stderr: true,
            follow: true,
            timestamps: true,
            tail: Some("all"),  // Get all logs from container start
            ..Default::default()
        };

        // Find the target container by name
        let containers = self.docker
            .list_containers(Some(ListContainersOptions {
                all: true,
                filters: HashMap::from([
                    ("name", vec![&self.target_service])
                ]),
                ..Default::default()
            }))
            .await?;

        let container = containers
            .into_iter()
            .find(|c| {
                c.names.as_ref()
                    .map(|names| names.iter().any(|n| n.contains(&self.target_service)))
                    .unwrap_or(false)
            })
            .ok_or(CollectorError::ContainerNotFound(self.target_service.clone()))?;

        let id = container.id.unwrap();
        let labels = container.labels.unwrap_or_default();

        let container_info = ContainerInfo {
            id: id.clone(),
            name: self.target_service.clone(),
            labels: labels.clone(),
            service_group: labels.get("rask.group").cloned(),
        };

        // Since we use network_mode: "service:xxx", we're in the same network namespace
        // This allows direct localhost access to the service if needed
        tracing::info!(
            "Starting log collection for service: {} (container: {})",
            self.target_service,
            id
        );

        let mut stream = self.docker.logs(&id, Some(options));

        while let Some(Ok(chunk)) = stream.next().await {
            let bytes = chunk.into_bytes();
            if tx.try_send((bytes, container_info.clone())).is_err() {
                // Apply backpressure
                tokio::time::sleep(Duration::from_micros(100)).await;
            }
        }

        tracing::warn!("Log stream ended for service: {}", self.target_service);
        Ok(())
    }
}
```

### Universal Parser with Service Detection
```rust
use simd_json::OwnedValue;
use once_cell::sync::Lazy;
use regex::Regex;

static NGINX_ACCESS_PATTERN: Lazy<Regex> = Lazy::new(|| {
    Regex::new(r#"^(\S+) \S+ \S+ \[([^\]]+)\] "([^"]+)" (\d+) (\d+)"#).unwrap()
});

pub struct UniversalParser {
    parsers: HashMap<String, Box<dyn LogParser>>,
}

impl UniversalParser {
    pub fn new() -> Self {
        let mut parsers = HashMap::new();

        // Register service-specific parsers
        parsers.insert("alt-frontend".to_string(), Box::new(NextJsParser));
        parsers.insert("alt-backend".to_string(), Box::new(GoStructuredParser));
        parsers.insert("nginx".to_string(), Box::new(NginxParser));
        parsers.insert("db".to_string(), Box::new(PostgresParser));

        Self { parsers }
    }

    pub fn parse_log(
        &self,
        mut bytes: Bytes,
        container_info: &ContainerInfo,
    ) -> Result<LogEntry, ParseError> {
        // Try Docker JSON wrapper first
        if let Ok(docker_entry) = self.parse_docker_json(&mut bytes.clone()) {
            let inner_log = docker_entry.message;

            // Apply service-specific parsing
            if let Some(parser) = self.parsers.get(&container_info.name) {
                return parser.parse(inner_log, container_info);
            }

            // Auto-detect format
            return self.detect_and_parse(inner_log, container_info);
        }

        // Raw log parsing
        self.detect_and_parse(bytes, container_info)
    }

    fn parse_docker_json(&self, bytes: &mut Bytes) -> Result<DockerLogEntry, ParseError> {
        let json = unsafe { simd_json::from_slice::<OwnedValue>(bytes)? };

        let obj = json.as_object().ok_or(ParseError::InvalidFormat)?;
        let log = obj.get("log").and_then(|v| v.as_str()).ok_or(ParseError::MissingField("log"))?;
        let timestamp = obj.get("time").and_then(|v| v.as_str()).ok_or(ParseError::MissingField("time"))?;

        Ok(DockerLogEntry {
            message: Bytes::copy_from_slice(log.trim_end().as_bytes()),
            stream: obj.get("stream").and_then(|v| v.as_str()).unwrap_or("stdout").into(),
            timestamp: timestamp.parse()?,
        })
    }
}

// Service-specific parser trait (Rust 2024: async fn in traits)
trait LogParser: Send + Sync {
    async fn parse(&self, bytes: Bytes, info: &ContainerInfo) -> Result<LogEntry, ParseError>;
}
```

### Lock-Free Buffering
```rust
use multiqueue::{BroadcastReceiver, BroadcastSender};
use std::sync::atomic::{AtomicU64, Ordering};

pub struct LogBuffer {
    sender: BroadcastSender<(Bytes, ContainerInfo)>,
    receiver: BroadcastReceiver<(Bytes, ContainerInfo)>,
    metrics: BufferMetrics,
}

#[derive(Default)]
struct BufferMetrics {
    pushed: AtomicU64,
    popped: AtomicU64,
    dropped: AtomicU64,
}

impl LogBuffer {
    pub fn new(capacity: usize) -> Self {
        let (sender, receiver) = multiqueue::broadcast_queue(capacity);
        Self {
            sender,
            receiver,
            metrics: BufferMetrics::default(),
        }
    }

    #[inline(always)]
    pub fn push(&self, item: (Bytes, ContainerInfo)) -> Result<(), BufferError> {
        match self.sender.try_send(item) {
            Ok(()) => {
                self.metrics.pushed.fetch_add(1, Ordering::Relaxed);
                Ok(())
            }
            Err(_) => {
                self.metrics.dropped.fetch_add(1, Ordering::Relaxed);
                Err(BufferError::Full)
            }
        }
    }
}
```

### Batch Transmission with Keep-Alive
```rust
use hyper::{Body, Client, Request};
use tokio::time::{interval, Duration};

pub struct BatchSender {
    client: Client<hyper::client::HttpConnector>,
    endpoint: Uri,
    batch_size: usize,
    flush_interval: Duration,
}

impl BatchSender {
    pub async fn run(
        &self,
        mut receiver: BroadcastReceiver<(Bytes, ContainerInfo)>,
        retry_tx: mpsc::Sender<Vec<Bytes>>,
    ) {
        let mut interval = interval(self.flush_interval);
        let mut batch = Vec::with_capacity(self.batch_size);

        loop {
            tokio::select! {
                Some((log, info)) = receiver.recv() => {
                    // Enrich log with service info
                    let enriched = self.enrich_log(log, &info);
                    batch.push(enriched);

                    if batch.len() >= self.batch_size {
                        self.send_batch(&mut batch, &retry_tx).await;
                    }
                }
                _ = interval.tick() => {
                    if !batch.is_empty() {
                        self.send_batch(&mut batch, &retry_tx).await;
                    }
                }
            }
        }
    }

    async fn send_batch(
        &self,
        batch: &mut Vec<Bytes>,
        retry_tx: &mpsc::Sender<Vec<Bytes>>,
    ) {
        let body = Body::wrap_stream(
            tokio_stream::iter(batch.drain(..).map(Ok::<_, std::io::Error>))
        );

        let req = Request::post(&self.endpoint)
            .header("content-type", "application/x-ndjson")
            .header("x-batch-size", batch.len().to_string())
            .body(body)
            .expect("request builder");

        match self.client.request(req).await {
            Ok(resp) if resp.status().is_success() => {
                tracing::debug!("Sent {} logs", batch.len());
            }
            _ => {
                let failed_batch = batch.clone();
                retry_tx.send(failed_batch).await.ok();
            }
        }
    }
}
```

## Configuration

### CLI with Environment Variables
```rust
use clap::Parser;

#[derive(Parser, Debug)]
#[command(author, version, about, long_about = None)]
pub struct Config {
    /// Target service name (auto-detected from hostname if not set)
    #[arg(long, env = "TARGET_SERVICE")]
    pub target_service: Option<String>,

    /// Rask aggregator endpoint
    #[arg(long, env = "RASK_ENDPOINT", default_value = "http://rask-log-aggregator:9600/v1/aggregate")]
    pub endpoint: String,

    /// Batch size
    #[arg(long, env = "BATCH_SIZE", default_value = "10000")]
    pub batch_size: usize,

    /// Flush interval (ms)
    #[arg(long, env = "FLUSH_INTERVAL_MS", default_value = "500")]
    pub flush_interval_ms: u64,

    /// Buffer capacity
    #[arg(long, env = "BUFFER_CAPACITY", default_value = "100000")]
    pub buffer_capacity: usize,

    /// Enable disk fallback
    #[arg(long, env = "ENABLE_DISK_FALLBACK")]
    pub enable_disk_fallback: bool,
}

impl Config {
    pub fn get_target_service(&self) -> Result<String, ConfigError> {
        self.target_service.clone().or_else(|| {
            // Auto-detect from hostname (e.g., "nginx-logs" → "nginx")
            hostname::get()
                .ok()
                .and_then(|h| h.to_str().map(|s| s.to_string()))
                .map(|h| h.trim_end_matches("-logs").to_string())
        }).ok_or(ConfigError::NoTargetService)
    }
}
```

## Testing Strategy

### TDD Workflow
```rust
#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_parse_go_structured_log() {
        // RED: Write failing test first
        let parser = GoStructuredParser::new();
        let log = br#"{"level":"info","msg":"Request processed","latency":123}"#;

        let result = parser.parse(Bytes::from_static(log)).await;

        assert!(result.is_ok());
        let entry = result.unwrap();
        assert_eq!(entry.level, LogLevel::Info);
        assert_eq!(entry.message, "Request processed");
        assert_eq!(entry.fields.get("latency"), Some(&"123".to_string()));
    }

    #[tokio::test]
    async fn test_nginx_access_log_parsing() {
        let parser = NginxParser::new();
        let log = b"192.168.1.1 - - [01/Jan/2024:00:00:00 +0000] \"GET /api/health HTTP/1.1\" 200 2";

        let result = parser.parse(Bytes::from_static(log)).await;

        assert!(result.is_ok());
        let entry = result.unwrap();
        assert_eq!(entry.fields.get("status_code"), Some(&"200".to_string()));
    }
}
```

### Performance Benchmarks
```rust
use criterion::{black_box, criterion_group, criterion_main, Criterion};

fn benchmark_simd_parsing(c: &mut Criterion) {
    let sample = br#"{"log":"2024-01-01 INFO Message\n","stream":"stdout","time":"2024-01-01T00:00:00Z"}"#;

    c.bench_function("simd_json_parse", |b| {
        b.iter(|| {
            let mut bytes = Bytes::from_static(sample);
            simd_json::from_slice::<OwnedValue>(black_box(&mut bytes))
        });
    });
}

criterion_group!(benches, benchmark_simd_parsing);
criterion_main!(benches);
```

## Dependencies

```toml
[package]
name = "rask-log-forwarder"
version = "0.1.0"
edition = "2024"
rust-version = "1.87"

[dependencies]
# Async runtime
tokio = { version = "1.40", features = ["full"] }

# Docker API
bollard = { version = "0.17", features = ["time", "ssl"] }

# Performance
bytes = "1.7"
simd-json = { version = "0.13", features = ["serde_impl"] }
multiqueue = "0.3"

# HTTP
axum = "0.8"

# CLI
clap = { version = "4.5", features = ["derive", "env"] }

# Serialization
serde = { version = "1.0", features = ["derive"] }
bincode = "1.3"

# Patterns
once_cell = "1.19"
regex = "1.10"

# Optional features
sled = { version = "0.34", optional = true }
prometheus = { version = "0.13", optional = true }

# Logging
tracing = "0.1"
tracing-subscriber = { version = "0.3", features = ["env-filter"] }

# Error handling
thiserror = "1.0"
anyhow = "1.0"

[dev-dependencies]
criterion = { version = "0.5", features = ["html_reports"] }
tokio-test = "0.4"

[features]
default = ["disk-fallback", "metrics"]
disk-fallback = ["sled"]
metrics = ["prometheus"]

[profile.release]
lto = "fat"
codegen-units = 1
opt-level = 3
```

## Deployment

### Docker Compose Sidecar Pattern
```yaml
version: '3.8'

# Shared environment for all forwarders
x-rask-env: &rask-env
  environment:
    - RASK_ENDPOINT=http://rask-log-aggregator:9600/v1/aggregate
    - BATCH_SIZE=10000
    - BUFFER_CAPACITY=100000
    - FLUSH_INTERVAL_MS=500

services:
  # Example Alt service
  nginx:
    image: nginx:latest
    labels:
      - "rask.group=alt-frontend"
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
    networks:
      - alt-network

  # Dedicated forwarder for nginx
  nginx-logs:
    build:
      context: ./rask-log-forwarder
      dockerfile: Dockerfile.rask-log-forwarder
    network_mode: "service:nginx"  # Share nginx's network namespace
    <<: *rask-env
    environment:
      - TARGET_SERVICE=nginx
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /var/lib/docker/containers:/var/lib/docker/containers:ro
    restart: unless-stopped

  # Forwarder for alt-backend
  alt-backend-logs:
    build:
      context: ./rask-log-forwarder
      dockerfile: Dockerfile.rask-log-forwarder
    network_mode: "service:alt-backend"
    <<: *rask-env
    environment:
      - TARGET_SERVICE=alt-backend
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /var/lib/docker/containers:/var/lib/docker/containers:ro
    restart: unless-stopped

  # ... repeat for each service
```

### Key Architecture Points
1. **One forwarder per service**: Each service gets its own dedicated log forwarder
2. **Network namespace sharing**: `network_mode: "service:xxx"` allows the forwarder to share the service's network
3. **Direct log file access**: Mounts `/var/lib/docker/containers` for direct json-file reading
4. **Service identification**: Each forwarder knows its target via `TARGET_SERVICE` env var

## Security & Operations

1. **Docker Socket:** Read-only mount (`/var/run/docker.sock:ro`)
2. **Container Logs:** Read-only access to `/var/lib/docker/containers`
3. **Network Isolation:** Each forwarder runs in its service's network namespace
4. **Resource Limits:** Each forwarder limited to minimal CPU/memory
5. **Monitoring:** Service-specific metrics via labels

## Architecture Benefits

### Sidecar Pattern Advantages
1. **Isolation**: Each forwarder failure only affects its target service
2. **Scaling**: Forwarders scale automatically with services
3. **Resource Efficiency**: Each forwarder uses minimal resources (~16MB)
4. **Network Locality**: Direct access via shared network namespace
5. **Simple Configuration**: Each forwarder only needs to know its target

### Direct Log File Access
By mounting `/var/lib/docker/containers`, forwarders can:
- Read logs directly from json-file driver
- Avoid Docker API overhead
- Achieve true zero-copy performance
- Handle high-throughput services efficiently

## Future Enhancements

1. **gRPC Streaming:** Bidirectional streams with per-message ACKs
2. **eBPF Enrichment:** Syscall timing via Aya
3. **WASM Transforms:** User-defined parsers with Wasmtime
4. **Compression:** LZ4 for batch payloads
5. **Multi-tenancy:** Route by service group labels

## References

- [Rust 2024 Edition Guide](https://doc.rust-lang.org/edition-guide/rust-2024/)
- [SIMD-JSON Performance](https://github.com/simd-lite/simd-json)
- [Multiqueue Lock-free Queue](https://github.com/schets/multiqueue)
- [Zero-Copy in Rust](https://manishearth.github.io/blog/2022/08/03/zero-copy-1-not-a-yoking-matter/)
- [High Performance Logging](https://www.usenix.org/conference/osdi14/technical-sessions/presentation/lee)
