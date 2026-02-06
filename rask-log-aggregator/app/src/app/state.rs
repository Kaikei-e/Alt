use crate::adapter::clickhouse::BatchWriter;
use crate::config::Settings;
use crate::port::{LogExporter, OTelExporter};
use clickhouse::Client;
use std::sync::Arc;
use tokio_util::sync::CancellationToken;

/// Shared application state holding the exporters.
pub struct AppState {
    pub log_exporter: Arc<dyn LogExporter>,
    pub otel_exporter: Arc<dyn OTelExporter>,
}

impl AppState {
    /// Create `AppState` from configuration settings.
    ///
    /// Spawns a background `BatchWriter` task that aggregates rows from
    /// channels and flushes them to ClickHouse periodically.
    #[must_use]
    pub fn from_settings(settings: &Settings, shutdown_token: CancellationToken) -> Self {
        let client = Client::default()
            .with_url(format!(
                "http://{}:{}",
                settings.clickhouse_host, settings.clickhouse_port
            ))
            .with_user(&settings.clickhouse_user)
            .with_password(&settings.clickhouse_password)
            .with_database(&settings.clickhouse_database);

        let batch_writer = Arc::new(BatchWriter::spawn(client, shutdown_token));
        let log_exporter: Arc<dyn LogExporter> = batch_writer.clone();
        let otel_exporter: Arc<dyn OTelExporter> = batch_writer;

        Self {
            log_exporter,
            otel_exporter,
        }
    }
}
