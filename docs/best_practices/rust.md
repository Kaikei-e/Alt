# Rust Best Practices — Alt (Edition 2024)

## 1. Edition 2024 Essentials

- Set `edition = "2024"` in `Cargo.toml`, then run `cargo fix --edition` and review the migration lints
- Understand RPIT lifetime capture changes: return-position `impl Trait` now captures all in-scope lifetimes by default — use `use<'a>` syntax to restrict if needed
- Mark all `extern` blocks as `unsafe extern` — Edition 2024 requires explicit `unsafe` on extern declarations
- Note that `std::env::set_var` and `std::env::remove_var` are `unsafe` in Edition 2024 — avoid them outside tests
- `Future` and `IntoFuture` are in the prelude — no manual `use std::future::Future` needed
- `gen` is a reserved keyword — do not use it as an identifier

```toml
# ✅ Cargo.toml — Edition 2024
[package]
name = "alt-service"
version = "0.1.0"
edition = "2024"
```

### RPIT Lifetime Capture

```rust
// Edition 2024: impl Trait captures all in-scope lifetimes by default
fn process(data: &[u8]) -> impl Iterator<Item = u8> + '_ {
    data.iter().copied()
}

// Restrict capture with use<> if needed
fn keys<'a>(map: &'a HashMap<String, String>) -> impl Iterator<Item = &'a str> + use<'a> {
    map.keys().map(|k| k.as_str())
}
```

### Match Ergonomics Changes

```rust
// Edition 2024: stricter match ergonomics — explicit & in patterns when needed
match &some_option {
    Some(val) => {}, // val is &T, not T
    None => {},
}
```

## 2. Project Structure & Modules

- Use `lib.rs` to declare public modules — keep `main.rs` as a thin entry point
- Use `pub(crate)` for internal-only items — avoid `pub` unless it's part of the public API
- Group related modules in directories with `mod.rs` or named module files
- Re-export key types at module boundaries with `pub use`

> **Alt:** Keep `lib.rs` focused on module boundaries and `main.rs` focused on startup, shutdown, and dependency wiring in Rust services such as `rask-log-forwarder`, `rask-log-aggregator`, and `recap-worker`.

```rust
// ✅ lib.rs — module declarations
pub mod adapter;
pub mod config;
pub mod connect;
pub mod db;
pub mod enrichment;
pub mod error;
pub mod http;
pub mod osv;
pub mod outbox;
pub mod rpc;
pub mod sbom;
pub mod scan;
```

```rust
// ✅ main.rs — thin entry point
#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info")),
        )
        .init();

    let config = ServiceConfig::from_env()?;
    let pool = PgPoolOptions::new()
        .max_connections(10)
        .connect(&config.database_url)
        .await?;

    // ... wire services, start server, await shutdown
    Ok(())
}
```

## 3. Error Handling

- Use `thiserror` for defining error enums — derive `Error` and `Debug`
- Use `#[from]` for automatic conversion from library errors (`sqlx::Error`, `std::io::Error`, etc.)
- Define a crate-level `type Result<T> = std::result::Result<T, MyError>` alias
- Use string variants for errors that need context: `#[error("action failed: {0}")]`
- Add context with `map_err` when `#[from]` is too broad

```rust
// ✅ Crate error type with thiserror
use thiserror::Error;

#[derive(Debug, Error)]
pub enum ServiceError {
    #[error("target access failed: {0}")]
    TargetAccess(String),

    #[error("SBOM parse failed: {0}")]
    SbomParse(String),

    #[error("OSV API error: {0}")]
    OsvApi(String),

    #[error("database error: {0}")]
    Database(#[from] sqlx::Error),

    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),

    #[error("JSON error: {0}")]
    Json(#[from] serde_json::Error),

    #[error("HTTP error: {0}")]
    Http(#[from] reqwest::Error),

    #[error("Redis error: {0}")]
    Redis(#[from] redis::RedisError),

    #[error("configuration error: {0}")]
    Config(String),
}

pub type Result<T> = std::result::Result<T, ServiceError>;
```

```rust
// ✅ Add context with map_err
let cve_ids: Vec<String> = sqlx::query_as(
    "SELECT DISTINCT advisory_id FROM vulnerability_instances WHERE advisory_id LIKE 'CVE-%'"
)
.fetch_all(pool)
.await
.map_err(ServiceError::Database)?;

// ❌ Bare ? without context when #[from] is not appropriate
let data = some_fallible_call()?; // caller has no idea what failed
```

> **Alt:** Prefer one crate-level domain error enum per service, then translate it into transport-specific errors only at HTTP or RPC boundaries.

## 4. Async & Concurrency

- Prefer native `async fn` in private/internal traits on Rust 1.75+
- For public traits, be explicit about `Send` requirements; `trait_variant` is the recommended pattern when you want both local and `Send` variants
- Keep `async-trait` only when you need older compiler support or dynamic dispatch
- Remember that traits using `async fn` / `-> impl Trait` are not object-safe
- Suppress `async_fn_in_trait` lint only when the trait is intentionally crate-local and the `Send` story is already clear
- Use `tokio::spawn` only for detached work that can safely outlive the current stack frame
- Use `tokio::JoinSet` when you need to spawn and await multiple tasks
- Use `CancellationToken` (from `tokio-util`) for cooperative shutdown
- Use `tokio::select!` for multiplexing — be aware of cancellation safety

```rust
// ✅ Native async fn in a crate-local trait
#[allow(async_fn_in_trait)]
pub(crate) trait Adapter {
    async fn prepare(&self, target: &Target, work_dir: &Path) -> Result<()>;
    async fn materialize_sbom(&self, target: &Target, work_dir: &Path) -> Result<NormalizedSbom>;
    async fn fingerprint(&self, target: &Target, work_dir: &Path) -> Result<String>;
}

// ✅ Public trait with explicit Send story
#[trait_variant::make(HttpService: Send)]
pub trait LocalHttpService {
    async fn fetch(&self, url: Url) -> HtmlBody;
}

// ⚠️ Keep async-trait only for specific compatibility needs
// #[async_trait]
// pub trait DynCompatibleAdapter { ... }
```

### CancellationToken + select!

```rust
// ✅ Graceful shutdown with CancellationToken
pub async fn run(&self, cancel: CancellationToken) {
    info!("outbox publisher started");

    loop {
        tokio::select! {
            _ = cancel.cancelled() => {
                info!("outbox publisher shutting down");
                return;
            }
            _ = tokio::time::sleep(Duration::from_secs(1)) => {
                if let Err(e) = self.publish_pending(&mut conn).await {
                    warn!(error = %e, "outbox publish cycle failed");
                }
            }
        }
    }
}
```

### Fire-and-Forget with tokio::spawn

```rust
// ✅ Detached background task with cancellation
let cancel = CancellationToken::new();
let outbox = OutboxPublisher::new(pool.clone(), config.redis_url.clone());
let outbox_cancel = cancel.clone();
tokio::spawn(async move {
    outbox.run(outbox_cancel).await;
});

// If task completion matters, keep the JoinHandle or use JoinSet.
// On shutdown:
cancel.cancel();
```

### Graceful Shutdown (main.rs)

```rust
// ✅ axum graceful shutdown
let cancel_for_shutdown = cancel.clone();
axum::serve(listener, app)
    .with_graceful_shutdown(async move {
        let _ = tokio::signal::ctrl_c().await;
        info!("received SIGINT, shutting down");
        cancel_for_shutdown.cancel();
    })
    .await?;
```

> **Alt:** Use `CancellationToken` to coordinate shutdown between the HTTP server and background tasks when a Rust service owns both.

## 5. Enum Dispatch

- Prefer enum dispatch over `Box<dyn Trait>` when the set of variants is fixed and small
- Use a `delegate_adapter!` macro to reduce boilerplate for method forwarding
- Use `dyn Trait` only when variants are determined at runtime or the set is open/extensible
- Implement `From<Variant>` for the enum when constructing from concrete types

```rust
// ✅ Enum dispatch with delegate macro
pub enum TargetAdapter {
    Git(git::GitTargetAdapter),
    Container(container::ContainerTargetAdapter),
    Sbom(sbom::SbomTargetAdapter),
}

macro_rules! delegate_adapter {
    ($self:expr, $method:ident, $($arg:expr),*) => {
        match $self {
            Self::Git(a) => a.$method($($arg),*).await,
            Self::Container(a) => a.$method($($arg),*).await,
            Self::Sbom(a) => a.$method($($arg),*).await,
        }
    };
}

impl TargetAdapter {
    pub fn from_target(target: &Target) -> Result<Self> {
        match target.target_type.as_str() {
            "git" | "repository" => Ok(Self::Git(git::GitTargetAdapter)),
            "container" => Ok(Self::Container(container::ContainerTargetAdapter)),
            "sbom" => Ok(Self::Sbom(sbom::SbomTargetAdapter)),
            other => Err(ServiceError::TargetAccess(format!(
                "unsupported target type: {other}"
            ))),
        }
    }

    pub async fn prepare(&self, target: &Target, work_dir: &Path) -> Result<()> {
        delegate_adapter!(self, prepare, target, work_dir)
    }
}
```

```rust
// ❌ Avoid Box<dyn Trait> when variants are fixed
let adapter: Box<dyn Adapter> = match target_type {
    "git" => Box::new(GitAdapter),
    "container" => Box::new(ContainerAdapter),
    _ => return Err(..),
};
```

> **Alt:** Use enum dispatch when the implementation set is small and known at compile time. Switch to trait objects only when extensibility is a real requirement.

## 6. Database (sqlx)

- Use `PgPool` — never raw connections in server code
- Set `max_connections` proportional to CPU cores: `cores * 2 + 1` is a good starting point
- Define query functions with generic executor bounds to support both `Pool` and `Transaction`
- Derive `sqlx::FromRow` on model structs for automatic row mapping
- Use parameterized queries (`$1`, `$2`) — never string-interpolate SQL
- Use `UPSERT` (`ON CONFLICT ... DO UPDATE`) for idempotent writes
- Wrap multi-step mutations in transactions

### Generic Executor Pattern

```rust
// ✅ Generic executor — works with both &PgPool and &mut PgConnection (Transaction)
pub async fn get_target<'e, E>(executor: E, target_id: Uuid) -> sqlx::Result<Target>
where
    E: sqlx::Executor<'e, Database = sqlx::Postgres>,
{
    sqlx::query_as::<_, Target>("SELECT * FROM targets WHERE id = $1")
        .bind(target_id)
        .fetch_one(executor)
        .await
}
```

```rust
// ✅ Call with pool
let target = queries::get_target(&pool, target_id).await?;

// ✅ Call with transaction
let mut tx = pool.begin().await?;
let target = queries::get_target(&mut *tx, target_id).await?;
tx.commit().await?;
```

### FromRow Model

```rust
// ✅ Derive sqlx::FromRow for automatic mapping
#[derive(Debug, sqlx::FromRow)]
pub struct Target {
    pub id: Uuid,
    pub project_id: Uuid,
    pub name: String,
    pub target_type: String,
    pub source_ref: Option<String>,
    pub branch: Option<String>,
    pub exposure_class: Option<String>,
    pub created_at: DateTime<Utc>,
}
```

### UPSERT Pattern

```rust
// ✅ Idempotent upsert with ON CONFLICT
pub async fn upsert_vulnerability_instance<'e, E>(
    executor: E,
    target_id: Uuid,
    package_name: &str,
    advisory_id: &str,
) -> sqlx::Result<Uuid>
where
    E: sqlx::Executor<'e, Database = sqlx::Postgres>,
{
    let row: (Uuid,) = sqlx::query_as(
        r#"
        INSERT INTO vulnerability_instances
            (target_id, package_name, advisory_id)
        VALUES ($1, $2, $3)
        ON CONFLICT (target_id, package_name, advisory_id)
        DO UPDATE SET advisory_source = EXCLUDED.advisory_source
        RETURNING id
        "#,
    )
    .bind(target_id)
    .bind(package_name)
    .bind(advisory_id)
    .fetch_one(executor)
    .await?;
    Ok(row.0)
}
```

### Transaction Boundaries

```rust
// ✅ Multi-step mutation in a transaction
let mut tx = pool.begin().await.map_err(ServiceError::Database)?;

for vuln in &vulnerabilities {
    let instance_id = queries::upsert_vulnerability_instance(&mut *tx, ...).await?;
    queries::insert_vulnerability_observation(&mut *tx, ...).await?;
    queries::upsert_current_finding_status(&mut *tx, ...).await?;
}

// Commit atomically
tx.commit().await.map_err(ScannerError::Database)?;
```

> **Alt:** If a Rust service stores immutable events, keep those tables append-only and isolate mutable current-state projections behind explicit upsert paths.

## 7. Serialization (serde)

- Derive `Serialize` and `Deserialize` together — use `serde(rename_all = "camelCase")` or `snake_case` as needed
- Use `#[serde(skip_serializing_if = "Option::is_none")]` to omit null fields in JSON
- Use `#[serde(default)]` for optional fields in deserialization
- Use internally tagged enums for API types: `#[serde(tag = "type")]`

```rust
// ✅ Clean serde patterns
#[derive(Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ScanEvent {
    pub scan_id: Uuid,
    pub target_id: Uuid,
    pub findings_count: u32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error_message: Option<String>,
}

// ✅ Enum tagging for API payloads
#[derive(Debug, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum StreamEvent {
    ScanCompleted { scan_id: Uuid, findings_count: u32 },
    ScanFailed { scan_id: Uuid, error: String },
}
```

```rust
// ❌ Don't serialize None fields when they add noise
#[derive(Serialize)]
pub struct Response {
    pub data: Vec<Item>,
    pub error: Option<String>, // Always sent as "error": null — noisy
}
```

## 8. HTTP & RPC

- Use `axum` for HTTP routing — extract state with `State<T>`
- Use `reqwest` for outbound HTTP — configure timeout and user-agent on a shared client
- Use Connect-RPC (prost + pbjson) for inter-service communication — not REST
- Map domain errors to `ConnectError` with appropriate `ConnectCode`

### axum Router

```rust
// ✅ axum routing with State
let app = Router::new()
    .route("/healthz", get(healthz))
    .route(
        "/scanner.v1.ScannerService/RunScan",
        post(|State(s): State<AppState>, Json(req)| async move {
            connect_response(s.scanner.run_scan(req).await)
        }),
    )
    .with_state(state);
```

### reqwest Client Factory

```rust
// ✅ Shared HTTP client with defaults
const DEFAULT_TIMEOUT: Duration = Duration::from_secs(30);
const USER_AGENT: &str = "alt-service/0.1";

pub fn default_client() -> reqwest::Client {
    reqwest::Client::builder()
        .user_agent(USER_AGENT)
        .timeout(DEFAULT_TIMEOUT)
        .build()
        .expect("failed to build HTTP client")
}

// ✅ Reuse client across calls
pub struct EpssClient {
    http: reqwest::Client,
}

impl EpssClient {
    pub fn new() -> Self {
        Self { http: crate::http::default_client() }
    }
}
```

### ConnectError Pattern

```rust
// ✅ Connect-RPC error mapping
#[derive(Debug, Serialize)]
pub struct ConnectError {
    pub code: ConnectCode,
    pub message: String,
}

#[derive(Debug, Clone, Copy, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum ConnectCode {
    InvalidArgument,
    NotFound,
    Internal,
    Unavailable,
}

impl ConnectCode {
    pub fn http_status(self) -> StatusCode {
        match self {
            Self::InvalidArgument => StatusCode::BAD_REQUEST,
            Self::NotFound => StatusCode::NOT_FOUND,
            Self::Internal => StatusCode::INTERNAL_SERVER_ERROR,
            Self::Unavailable => StatusCode::SERVICE_UNAVAILABLE,
        }
    }
}

// ✅ Map domain errors at RPC boundary
let job = queries::get_scan_job(&self.pool, job_id)
    .await
    .map_err(|e| match e {
        sqlx::Error::RowNotFound => ConnectError {
            code: ConnectCode::NotFound,
            message: "scan job not found".to_string(),
        },
        other => ConnectError {
            code: ConnectCode::Internal,
            message: format!("database error: {other}"),
        },
    })?;
```

> **Alt:** When a Rust service exposes Connect-RPC, keep protobuf/JSON boundaries explicit and map transport codes from domain errors in one place.

## 9. Logging & Tracing

- Use `tracing` (not `log`) with `tracing-subscriber` — configure `EnvFilter` for runtime log level control
- Use structured fields in log macros: `info!(key = %value, "message")`
- Use `%` for Display formatting and `?` for Debug formatting in tracing fields
- Use `#[instrument]` to auto-generate spans for functions (with field capture)
- Set the default filter to `info` — override with `RUST_LOG` env var

```rust
// ✅ Tracing setup with EnvFilter
tracing_subscriber::fmt()
    .with_env_filter(
        tracing_subscriber::EnvFilter::try_from_default_env()
            .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info")),
    )
    .init();
```

```rust
// ✅ Structured logging with tracing
info!(scan_id = %scan_id, job_id = %job_id, "scan completed successfully");
error!(scan_id = %scan_id, error = %e, "scan failed");
warn!(vuln_id = %id, %e, "failed to fetch OSV vuln details, skipping");
info!(packages = sbom.packages.len(), edges = sbom.edges.len(), "SBOM materialized");
```

```rust
// ❌ String interpolation in log messages
info!("scan {} completed for job {}", scan_id, job_id);
```

### Log Level Guide

| Level | Use for |
|-------|---------|
| `error!` | Failures requiring attention — scan failed, connection lost |
| `warn!` | Recoverable issues — retry, missing optional data, skip |
| `info!` | Normal operations — started, completed, connected, config loaded |
| `debug!` | Detailed flow — individual queries, HTTP requests, intermediate state |
| `trace!` | Very verbose — raw payloads, iteration details |

> **Alt:** Default to `info` logging and override with a service-specific `RUST_LOG` target such as `RUST_LOG=rask_log_forwarder=debug` when debugging.

## 10. Testing

- Colocate tests in the same file with `#[cfg(test)]` — not in a separate `tests/` directory
- Use `#[tokio::test]` for async tests
- Use `wiremock` for HTTP API mocking — start a `MockServer`, mount expectations, pass server URL to client
- Separate testable logic from the base URL so tests can inject the mock server URL
- Use `assert!` with descriptive messages, and `assert_eq!` / `assert_ne!` for value comparison

### Colocated Test Module

```rust
// ✅ Tests at the bottom of the same file
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn connect_error_serializes_to_spec_format() {
        let err = ConnectError {
            code: ConnectCode::NotFound,
            message: "scan job not found".to_string(),
        };
        let json = serde_json::to_string(&err).unwrap();
        assert_eq!(json, r#"{"code":"not_found","message":"scan job not found"}"#);
    }
}
```

### wiremock HTTP Mocking

```rust
// ✅ wiremock for external API tests
#[cfg(test)]
mod tests {
    use super::*;
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    #[tokio::test]
    async fn parse_epss_response() {
        let server = MockServer::start().await;

        Mock::given(method("GET"))
            .and(path("/data/v1/epss"))
            .respond_with(ResponseTemplate::new(200).set_body_json(sample_response()))
            .mount(&server)
            .await;

        let client = EpssClient::new();
        let url = format!("{}/data/v1/epss", server.uri());
        let entries = client.fetch_batch_from(&url, &cves).await.unwrap();

        assert_eq!(entries.len(), 2);
        assert!((entries[0].epss - 0.05432).abs() < 0.0001);
    }

    #[tokio::test]
    async fn handles_api_error() {
        let server = MockServer::start().await;

        Mock::given(method("GET"))
            .and(path("/data/v1/epss"))
            .respond_with(ResponseTemplate::new(500).set_body_string("internal error"))
            .mount(&server)
            .await;

        let client = EpssClient::new();
        let url = format!("{}/data/v1/epss", server.uri());
        let result = client.fetch_batch_from(&url, &["CVE-2024-0001".into()]).await;
        assert!(result.is_err());
    }
}
```

### Testable Design Pattern

```rust
// ✅ Separate base URL from logic for testability
impl EpssClient {
    pub async fn fetch_batch(&self, cve_ids: &[String]) -> Result<Vec<EpssEntry>> {
        self.fetch_batch_from(EPSS_API_URL, cve_ids).await
    }

    // Private method accepts base URL — tests inject mock server URL
    async fn fetch_batch_from(&self, base_url: &str, cve_ids: &[String]) -> Result<Vec<EpssEntry>> {
        // ... actual logic
    }
}
```

> **Alt:** Run tests from the concrete Rust service directory, for example `cd rask-log-forwarder && cargo test` or `cd recap-worker && cargo test`.

## 11. Configuration

- Load configuration from environment variables in a `from_env()` constructor
- Use `map_err` for required variables — fail fast with a clear message
- Use `.ok()` for optional variables — return `Option<String>`
- Use `.unwrap_or_else` for variables with defaults
- Return a typed struct, not raw strings

```rust
// ✅ Typed config with from_env()
#[derive(Debug, Clone)]
pub struct ServiceConfig {
    pub database_url: String,
    pub redis_url: String,
    pub listen_addr: String,
    pub nvd_api_key: Option<String>,
}

impl ServiceConfig {
    pub fn from_env() -> Result<Self> {
        Ok(Self {
            // Required — fail with clear error
            database_url: std::env::var("DATABASE_URL")
                .map_err(|_| ServiceError::Config("DATABASE_URL must be set".into()))?,
            // Optional with default
            redis_url: std::env::var("REDIS_URL")
                .unwrap_or_else(|_| "redis://127.0.0.1:6379".to_string()),
            listen_addr: std::env::var("LISTEN_ADDR")
                .unwrap_or_else(|_| "0.0.0.0:9000".to_string()),
            // Truly optional
            nvd_api_key: std::env::var("NVD_API_KEY").ok(),
        })
    }
}
```

```rust
// ❌ Don't pass raw strings around
fn start_server(db_url: &str, redis_url: &str, listen_addr: &str) { ... }
```

> **Alt:** Pick one primary configuration approach per service and document it clearly. Avoid mixing multiple partially overlapping config sources without a strong reason.

## 12. Security

- Always use parameterized queries (`$1`, `$2`) — never interpolate user input into SQL
- Run `cargo audit` to check for known vulnerabilities in dependencies
- Use `cargo deny` to enforce license and advisory policies
- Minimize `unsafe` blocks — when used, add a `// SAFETY:` comment explaining the invariant
- Enable `overflow-checks` in release profile for defense against integer overflow

```rust
// ✅ Parameterized query
sqlx::query_as::<_, Target>("SELECT * FROM targets WHERE id = $1")
    .bind(target_id)
    .fetch_one(executor)
    .await

// ❌ SQL injection risk — never do this
let sql = format!("SELECT * FROM targets WHERE id = '{target_id}'");
sqlx::query(&sql).fetch_one(executor).await
```

```toml
# ✅ Cargo.toml — release profile hardening
[profile.release]
overflow-checks = true
```

```rust
// ✅ Document unsafe invariants
// SAFETY: The buffer was allocated with the correct alignment and length
unsafe { std::slice::from_raw_parts(ptr, len) }
```

## 13. Clippy & Linting

- Configure lint levels in `Cargo.toml` under `[lints.clippy]` — not via command-line flags
- Enable `clippy::all` + `clippy::pedantic` with selective `allow` for noisy rules
- Run `cargo clippy -- -D warnings` in CI to fail on any lint
- Run `cargo fmt --check` in CI to enforce formatting

```toml
# ✅ Cargo.toml — lint configuration
[lints.clippy]
all = "warn"
pedantic = "warn"
# Selectively allow noisy pedantic lints
module_name_repetitions = "allow"
must_use_candidate = "allow"
missing_errors_doc = "allow"
missing_panics_doc = "allow"
```

```bash
# CI commands
cargo clippy -- -D warnings
cargo fmt --check
```

> **Alt:** Verification should be service-local, for example `cd rask-log-aggregator && cargo clippy -- -D warnings`.

## 14. Performance

- Pre-allocate collections when size is known: `Vec::with_capacity(n)`
- Prefer iterator chains over manual loops — they compile to equivalent code (zero-cost abstraction)
- Use `Criterion` for benchmarks — measure before optimizing
- Configure release profile for maximum optimization

```rust
// ✅ Pre-allocate when size is known
let mut all_results = Vec::with_capacity(packages.len());

// ✅ Iterator chain — zero-cost
let cve_ids: Vec<String> = rows.into_iter().map(|r| r.0).collect();
```

```toml
# ✅ Release profile for maximum performance
[profile.release]
lto = "fat"
codegen-units = 1
strip = true
overflow-checks = true
```

### Batch Processing

```rust
// ✅ Chunk large inputs to control memory and API rate
for chunk in cve_ids.chunks(CHUNK_SIZE) {
    let cve_param = chunk.join(",");
    let url = format!("{base_url}?cve={cve_param}");
    // ... process chunk
    all_entries.extend(api_response.data);
}
```

## 15. Docker

- Use `cargo-chef` for 3-stage Docker builds: planner → builder → runtime
- Cache dependency compilation separately from source compilation (planner + recipe.json)
- Use `debian:bookworm-slim` for runtime — avoid `alpine` with glibc Rust binaries
- Set `HEALTHCHECK` and `EXPOSE` in the Dockerfile
- Run as non-root user in production

```dockerfile
# ✅ 3-stage cargo-chef build
# Stage 1: Chef + planner
FROM rust:1.94.0-bookworm AS chef
RUN cargo install cargo-chef --locked
WORKDIR /app

FROM chef AS planner
COPY Cargo.toml Cargo.lock ./
COPY src ./src
RUN cargo chef prepare --recipe-path recipe.json

# Stage 2: Build (dependencies cached separately from source)
FROM chef AS builder
COPY --from=planner /app/recipe.json recipe.json
RUN cargo chef cook --release --recipe-path recipe.json
COPY . .
RUN cargo build --release

# Stage 3: Minimal runtime
FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/target/release/my-service /usr/local/bin/my-service
EXPOSE 9000
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:9000/healthz || exit 1
CMD ["my-service"]
```

> **Alt:** Runtime images should install only the tools the service genuinely needs in production. Keep the runtime surface area smaller than the build image whenever possible.

## 16. Modern Rust Patterns

### let-else

```rust
// ✅ let-else for early returns on pattern mismatch
let Some(purl) = &pkg.purl else {
    warn!("no purl for package {}", pkg.name);
    continue;
};
```

### matches! Macro

```rust
// ✅ matches! for concise pattern checks
if matches!(status, "completed" | "failed") {
    // ...
}

// ❌ Verbose match for a boolean check
let is_done = match status {
    "completed" | "failed" => true,
    _ => false,
};
```

### impl Trait

```rust
// ✅ impl Trait in argument position — accept any iterator
fn process_items(items: impl IntoIterator<Item = String>) {
    for item in items {
        // ...
    }
}

// ✅ impl Trait in return position — return closures or iterators
fn active_findings(findings: &[Finding]) -> impl Iterator<Item = &Finding> {
    findings.iter().filter(|f| f.is_active)
}
```

### Default Implementation

```rust
// ✅ impl Default for constructors with no arguments
impl Default for OsvClient {
    fn default() -> Self {
        Self::new()
    }
}

// Enables both:
let client = OsvClient::new();
let client = OsvClient::default();
```

---

## References

- [The Rust Edition Guide — 2024 Edition](https://doc.rust-lang.org/edition-guide/rust-2024/)
- [The Rust Programming Language (The Book)](https://doc.rust-lang.org/book/)
- [Rust API Guidelines](https://rust-lang.github.io/api-guidelines/)
- [Rust Design Patterns](https://rust-unofficial.github.io/patterns/)
- [Tokio Tutorial](https://tokio.rs/tokio/tutorial)
- [sqlx Documentation](https://docs.rs/sqlx/)
- [axum Documentation](https://docs.rs/axum/)
- [cargo-chef (LukeMathWalker)](https://github.com/LukeMathWalker/cargo-chef)
- [Clippy Lints](https://rust-lang.github.io/rust-clippy/master/)
