pub mod clickhouse_exporter;

use async_trait::async_trait;
use crate::domain::EnrichedLogEntry;

#[async_trait]
pub trait LogExporter: Send + Sync {
    async fn export_batch(&self, logs: Vec<EnrichedLogEntry>) -> Result<(), anyhow::Error>;
}