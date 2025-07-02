use crate::domain::{EnrichedLogEntry, LogLevel};
use anyhow::Result;
use async_trait::async_trait;
use chrono::{DateTime, Utc};
use clickhouse::Client;
use clickhouse::serde::chrono::datetime64::millis;
use serde::{Deserialize, Serialize};
use std::time::Duration;
use tracing::error;

#[derive(clickhouse::Row, Serialize, Deserialize, Clone, Debug)]
pub struct LogRow {
    service_type: String, // LowCardinality(String)
    log_type: String,     // LowCardinality(String)
    message: String,      // String
    level: i8,            // Enum8 -> underlying UInt8
    #[serde(with = "millis")]
    timestamp: DateTime<Utc>, // DateTime64(3,'UTC')
    stream: String,       // LowCardinality(String)
    container_id: String, // String
    service_name: String, // LowCardinality(String)
    service_group: String, // LowCardinality(String)
    fields: Vec<(String, String)>, // Map(String,String)
}

impl From<EnrichedLogEntry> for LogRow {
    fn from(log: EnrichedLogEntry) -> Self {
        let mut fields = Vec::new();
        for (k, v) in log.fields {
            fields.push((k, v));
        }
        Self {
            service_type: log.service_type,
            log_type: log.log_type,
            message: log.message,
            level: match log.level {
                Some(LogLevel::Debug) => 0,
                Some(LogLevel::Info) | None => 1,
                Some(LogLevel::Warn) => 2,
                Some(LogLevel::Error) => 3,
                Some(LogLevel::Fatal) => 4,
            },
            timestamp: log
                .timestamp
                .parse::<DateTime<Utc>>()
                .unwrap_or_else(|_| Utc::now()),
            stream: log.stream,
            container_id: log.container_id,
            service_name: log.service_name,
            service_group: log.service_group.unwrap_or_else(|| "unknown".into()),
            fields,
        }
    }
}

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
    async fn export_batch(&self, logs: Vec<EnrichedLogEntry>) -> Result<()> {
        // 変換時に所有権を奪い clone を削減
        let rows: Vec<LogRow> = logs.into_iter().map(LogRow::from).collect();

        // バッチを 1000 行単位で送信
        let mut inserter = self
            .client
            .inserter::<LogRow>("logs")?
            .with_timeouts(Some(Duration::from_secs(10)), Some(Duration::from_secs(10)))
            .with_max_bytes(50_000_000)
            .with_max_rows(1000);

        for row in &rows {
            match inserter.write(row) {
                Ok(_) => (),
                Err(e) => {
                    error!("Failed to write row to ClickHouse: {e}");
                }
            }
        }
        inserter.end().await?; // commit 相当

        Ok(())
    }
}
