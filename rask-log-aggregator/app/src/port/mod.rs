pub mod log_exporter;
pub mod otel_exporter;

pub use log_exporter::LogExporter;
pub use otel_exporter::OTelExporter;

use std::future::Future;
use std::pin::Pin;

/// Boxed, pinned future for dyn-compatible async trait methods.
///
/// Rust 2024's native `async fn` in traits doesn't support `dyn` dispatch,
/// but `LogExporter`/`OTelExporter` are used as trait objects (mocked in
/// tests, swapped at the composition root) - so methods return this instead.
pub type BoxFuture<'a, T> = Pin<Box<dyn Future<Output = T> + Send + 'a>>;
