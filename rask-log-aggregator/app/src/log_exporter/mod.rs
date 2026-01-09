pub mod clickhouse_exporter;
pub mod json_file_exporter;

use crate::domain::EnrichedLogEntry;
use std::future::Future;
use std::pin::Pin;

/// Log exporter trait for various backends (ClickHouse, JSON file, etc.)
///
/// This trait is dyn-compatible by using boxed futures instead of `impl Future`.
pub trait LogExporter: Send + Sync {
    fn export_batch(
        &self,
        logs: Vec<EnrichedLogEntry>,
    ) -> Pin<Box<dyn Future<Output = Result<(), anyhow::Error>> + Send + '_>>;
}
