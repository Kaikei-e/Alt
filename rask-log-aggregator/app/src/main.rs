mod config;
mod log_exporter;
mod domain;

use crate::log_exporter::clickhouse_exporter::ClickHouseExporter;
use crate::log_exporter::LogExporter;
use crate::domain::EnrichedLogEntry;
use axum::{
    Router,
    extract::State,
    routing::{get, post},
};
use std::sync::Arc;
use clickhouse::Client;
use tracing::{info, error, Level};
use tracing_subscriber::{fmt, prelude::*, EnvFilter};

#[tokio::main]
async fn main() {
    tracing_subscriber::registry()
        .with(fmt::layer())
        .with(EnvFilter::from_default_env().add_directive(Level::INFO.into()))
        .init();

    let settings = config::get_configuration().expect("Failed to read configuration.");
    info!("Loaded settings: {:?}", settings);

    let client = Client::default()
        .with_url(format!("http://{}:{}", settings.clickhouse_host, settings.clickhouse_port))
        .with_user(&settings.clickhouse_user)
        .with_password(&settings.clickhouse_password)
        .with_database(&settings.clickhouse_database);

    let exporter: Arc<dyn LogExporter> = Arc::new(ClickHouseExporter::new(client));

    let v1_health_router: Router = Router::new().route("/v1/health", get(|| async {
        info!("Health check requested");
        "Healthy"
    }));
    let v1_aggregate_router: Router = Router::new()
        .route("/v1/aggregate", post(aggregate_handler))
        .with_state(exporter);

    let app = Router::new()
        .merge(v1_health_router)
        .merge(v1_aggregate_router);

    let listener = tokio::net::TcpListener::bind("0.0.0.0:9600").await.unwrap();
    let ip_addr = listener.local_addr().unwrap();
    info!("Listening on {}", ip_addr);
    let _ = axum::serve(listener, app).await.unwrap();
}

// Add handler function for /v1/aggregate
async fn aggregate_handler(State(exporter): State<Arc<dyn LogExporter>>, body: String) -> &'static str {
    let logs: Vec<EnrichedLogEntry> = body
        .lines()
        .filter_map(|line| serde_json::from_str(line).ok())
        .collect();

    if let Err(e) = exporter.export_batch(logs).await {
        error!("Failed to export logs to ClickHouse: {}", e);
        // ここでリトライやフォールバック処理を検討
    }

    "OK"
}
