pub mod clickhouse_exporter;

use crate::domain::EnrichedLogEntry;
use async_trait::async_trait;

#[async_trait]
pub trait LogExporter: Send + Sync {
    async fn export_batch(&self, logs: Vec<EnrichedLogEntry>) -> Result<(), anyhow::Error>;
}
