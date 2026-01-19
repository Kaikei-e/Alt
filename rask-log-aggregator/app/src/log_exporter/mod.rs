pub mod clickhouse_exporter;
pub mod disk_cleaner;
pub mod json_file_exporter;
pub mod otel_exporter;

use crate::domain::EnrichedLogEntry;
use crate::error::AggregatorError;
use std::future::Future;
use std::pin::Pin;

pub use otel_exporter::OTelExporter;

/// Log exporter trait for various backends (ClickHouse, JSON file, etc.)
///
/// This trait is dyn-compatible by using boxed futures instead of `impl Future`.
pub trait LogExporter: Send + Sync {
    fn export_batch(
        &self,
        logs: Vec<EnrichedLogEntry>,
    ) -> Pin<Box<dyn Future<Output = Result<(), AggregatorError>> + Send + '_>>;
}
