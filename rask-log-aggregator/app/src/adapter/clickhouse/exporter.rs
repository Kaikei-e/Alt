use crate::adapter::clickhouse::otel_row::{OTelLogRow, OTelTraceRow};
use crate::adapter::clickhouse::row::LogRow;
use crate::domain::{EnrichedLogEntry, OTelLog, OTelTrace};
use crate::error::AggregatorError;
use clickhouse::Client;
use std::time::Duration;
use tracing::error;

// ClickHouse inserter configuration constants
const INSERTER_SEND_TIMEOUT: Duration = Duration::from_secs(10);
const INSERTER_END_TIMEOUT: Duration = Duration::from_secs(10);
const INSERTER_MAX_BYTES: u64 = 50_000_000;
const INSERTER_MAX_ROWS: u64 = 1000;

pub struct ClickHouseExporter {
    client: Client,
}

impl ClickHouseExporter {
    #[must_use]
    pub fn new(client: Client) -> Self {
        Self { client }
    }

    /// Create a configured inserter for the given table
    fn create_inserter<T: clickhouse::Row>(
        &self,
        table: &str,
    ) -> Result<clickhouse::inserter::Inserter<T>, clickhouse::error::Error> {
        Ok(self
            .client
            .inserter::<T>(table)?
            .with_timeouts(Some(INSERTER_SEND_TIMEOUT), Some(INSERTER_END_TIMEOUT))
            .with_max_bytes(INSERTER_MAX_BYTES)
            .with_max_rows(INSERTER_MAX_ROWS))
    }

    /// Export OpenTelemetry logs to ClickHouse
    pub async fn export_otel_logs(&self, logs: Vec<OTelLog>) -> Result<(), AggregatorError> {
        if logs.is_empty() {
            return Ok(());
        }

        let rows: Vec<OTelLogRow> = logs.into_iter().map(OTelLogRow::from).collect();

        let mut inserter = self.create_inserter::<OTelLogRow>("otel_logs")?;

        for row in &rows {
            if let Err(e) = inserter.write(row) {
                error!("Failed to write OTel log row to ClickHouse: {e}");
            }
        }

        inserter.end().await?;
        Ok(())
    }

    /// Export OpenTelemetry traces to ClickHouse
    pub async fn export_otel_traces(&self, traces: Vec<OTelTrace>) -> Result<(), AggregatorError> {
        if traces.is_empty() {
            return Ok(());
        }

        let rows: Vec<OTelTraceRow> = traces.into_iter().map(OTelTraceRow::from).collect();

        let mut inserter = self.create_inserter::<OTelTraceRow>("otel_traces")?;

        for row in &rows {
            if let Err(e) = inserter.write(row) {
                error!("Failed to write OTel trace row to ClickHouse: {e}");
            }
        }

        inserter.end().await?;
        Ok(())
    }
}

impl crate::port::LogExporter for ClickHouseExporter {
    fn export_batch(
        &self,
        logs: Vec<EnrichedLogEntry>,
    ) -> std::pin::Pin<Box<dyn std::future::Future<Output = Result<(), AggregatorError>> + Send + '_>>
    {
        Box::pin(async move {
            let rows: Vec<LogRow> = logs.into_iter().map(LogRow::from).collect();

            let mut inserter = self.create_inserter::<LogRow>("logs")?;

            for row in &rows {
                if let Err(e) = inserter.write(row) {
                    error!("Failed to write row to ClickHouse: {e}");
                }
            }
            inserter.end().await?;

            Ok(())
        })
    }
}

impl crate::port::OTelExporter for ClickHouseExporter {
    fn export_otel_logs(
        &self,
        logs: Vec<OTelLog>,
    ) -> std::pin::Pin<Box<dyn std::future::Future<Output = Result<(), AggregatorError>> + Send + '_>>
    {
        Box::pin(self.export_otel_logs(logs))
    }

    fn export_otel_traces(
        &self,
        traces: Vec<OTelTrace>,
    ) -> std::pin::Pin<Box<dyn std::future::Future<Output = Result<(), AggregatorError>> + Send + '_>>
    {
        Box::pin(self.export_otel_traces(traces))
    }
}
