use super::BoxFuture;
use crate::domain::EnrichedLogEntry;
use crate::error::AggregatorError;

/// Log exporter trait for various backends (ClickHouse, JSON file, etc.)
///
/// This trait is dyn-compatible by using boxed futures instead of `impl Future`.
pub trait LogExporter: Send + Sync {
    fn export_batch(
        &self,
        logs: Vec<EnrichedLogEntry>,
    ) -> BoxFuture<'_, Result<(), AggregatorError>>;
}
