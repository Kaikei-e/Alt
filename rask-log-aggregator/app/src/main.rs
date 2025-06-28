mod log_exporter;

use crate::log_exporter::disk_cleaner::DiskCleaner;
use crate::log_exporter::json_file_exporter::JsonFileExporter;
use axum::{
    Router,
    extract::State,
    routing::{get, post},
};
use std::time::Duration;

#[tokio::main]
async fn main() {
    let exporter = JsonFileExporter::new("/logs/logs.json");

    // Spawn background disk cleaner (1 GB quota, checks every 10 min)
    let cleaner = DiskCleaner::new("/logs", 1 * 1024 * 1024 * 1024, Duration::from_secs(600));
    cleaner.spawn();

    let v1_health_router: Router = Router::new().route("/v1/health", get(|| async { "Healthy" }));
    let v1_aggregate_router: Router = Router::new()
        .route("/v1/aggregate", post(aggregate_handler))
        .with_state(exporter);

    let app = Router::new()
        .merge(v1_health_router)
        .merge(v1_aggregate_router);

    let listener = tokio::net::TcpListener::bind("0.0.0.0:9600").await.unwrap();
    let ip_addr = listener.local_addr().unwrap();
    println!("Listening on {}", ip_addr);
    let _ = axum::serve(listener, app).await.unwrap();
}

// Add handler function for /v1/aggregate
async fn aggregate_handler(State(exporter): State<JsonFileExporter>, body: String) -> &'static str {
    let mut line_count = 0;
    for line in body.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() {
            continue;
        }
        exporter.export_raw(trimmed);
        line_count += 1;
    }

    println!("[aggregator] received {} log lines", line_count);

    "OK"
}
