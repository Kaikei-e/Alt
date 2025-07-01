use async_trait::async_trait;
use clickhouse::Client;
use crate::domain::EnrichedLogEntry;
use anyhow::Result;

pub struct ClickHouseExporter {
    client: Client,
}

impl ClickHouseExporter {
    pub fn new(client: Client) -> Self {
        Self { client }
    }
}

#[async_trait]
impl super::LogExporter for ClickHouseExporter {
    async fn export_batch(&self, logs: Vec<EnrichedLogEntry>) -> Result<(), anyhow::Error> {
        // TODO: Implement actual ClickHouse batch insertion
        Ok(())
    }
}