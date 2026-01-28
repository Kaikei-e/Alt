mod config;
mod domain;
mod error;
mod log_exporter;
mod otlp;

use crate::domain::EnrichedLogEntry;
use crate::error::AggregatorError;
use crate::log_exporter::clickhouse_exporter::ClickHouseExporter;
use crate::log_exporter::{LogExporter, OTelExporter};
use crate::otlp::otlp_routes;
use crate::otlp::receiver::OTLPState;
use axum::{
    Router,
    extract::State,
    http::StatusCode,
    response::IntoResponse,
    routing::{get, post},
};
use clickhouse::Client;
use std::sync::Arc;
use tokio::signal;
use tokio_util::sync::CancellationToken;
use tracing::{Level, error, info};
use tracing_subscriber::{EnvFilter, fmt, prelude::*};

#[tokio::main]
async fn main() -> Result<(), AggregatorError> {
    // Handle healthcheck subcommand (for Docker healthcheck in distroless image)
    if std::env::args().nth(1).as_deref() == Some("healthcheck") {
        match rask::healthcheck().await {
            Ok(()) => std::process::exit(0),
            Err(e) => {
                eprintln!("Healthcheck failed: {}", e);
                std::process::exit(1)
            }
        }
    }
    // Use JSON format if RUST_LOG_FORMAT=json, otherwise use human-readable format
    let use_json = std::env::var("RUST_LOG_FORMAT")
        .map(|v| v == "json")
        .unwrap_or(true); // Default to JSON for production

    if use_json {
        tracing_subscriber::registry()
            .with(
                fmt::layer()
                    .json()
                    .flatten_event(true)
                    .with_current_span(true),
            )
            .with(EnvFilter::from_default_env().add_directive(Level::INFO.into()))
            .init();
    } else {
        tracing_subscriber::registry()
            .with(fmt::layer())
            .with(EnvFilter::from_default_env().add_directive(Level::INFO.into()))
            .init();
    }

    let settings =
        config::get_configuration().map_err(|e| AggregatorError::Config(e.to_string()))?;
    info!("Loaded settings");

    let client = Client::default()
        .with_url(format!(
            "http://{}:{}",
            settings.clickhouse_host, settings.clickhouse_port
        ))
        .with_user(&settings.clickhouse_user)
        .with_password(&settings.clickhouse_password)
        .with_database(&settings.clickhouse_database);

    // Create ClickHouse exporter (shared between legacy and OTLP endpoints)
    let ch_exporter = Arc::new(ClickHouseExporter::new(client));
    let exporter: Arc<dyn LogExporter> = ch_exporter.clone();
    let otel_exporter: Arc<dyn OTelExporter> = ch_exporter;

    // OTLP state
    let otlp_state = OTLPState {
        exporter: otel_exporter,
    };

    // Health check endpoint
    let v1_health_router: Router = Router::new().route(
        "/v1/health",
        get(|| async {
            info!("Health check requested");
            "Healthy"
        }),
    );

    // Legacy aggregate endpoint (for rask-log-forwarder)
    let v1_aggregate_router: Router = Router::new()
        .route("/v1/aggregate", post(aggregate_handler))
        .with_state(exporter);

    // OTLP endpoints router
    let otlp_router = otlp_routes(otlp_state);

    // Main app (health + aggregate) - port 9600
    let main_app = Router::new()
        .merge(v1_health_router)
        .merge(v1_aggregate_router);

    // Bind main server
    let main_bind_addr = format!("0.0.0.0:{}", settings.http_port);
    let main_listener = tokio::net::TcpListener::bind(&main_bind_addr)
        .await
        .map_err(|e| AggregatorError::Bind {
            address: main_bind_addr.clone(),
            source: e,
        })?;
    info!("Main server listening on {}", main_listener.local_addr()?);
    info!("  - GET  /v1/health     (health check)");
    info!("  - POST /v1/aggregate  (legacy NDJSON logs)");

    // Bind OTLP server - port 4318
    let otlp_bind_addr = format!("0.0.0.0:{}", settings.otlp_http_port);
    let otlp_listener = tokio::net::TcpListener::bind(&otlp_bind_addr)
        .await
        .map_err(|e| AggregatorError::Bind {
            address: otlp_bind_addr.clone(),
            source: e,
        })?;
    info!(
        "OTLP HTTP server listening on {}",
        otlp_listener.local_addr()?
    );
    info!("  - POST /v1/logs       (OTLP logs)");
    info!("  - POST /v1/traces     (OTLP traces)");

    // CancellationToken for graceful shutdown coordination
    let shutdown_token = CancellationToken::new();

    // Spawn OTLP server task
    let otlp_shutdown = shutdown_token.child_token();
    let otlp_handle = tokio::spawn(async move {
        axum::serve(otlp_listener, otlp_router)
            .with_graceful_shutdown(otlp_shutdown.cancelled_owned())
            .await
    });

    // Run main server
    let main_shutdown = shutdown_token.clone();
    axum::serve(main_listener, main_app)
        .with_graceful_shutdown(async move {
            shutdown_signal().await;
            // Signal OTLP server to shutdown
            main_shutdown.cancel();
        })
        .await?;

    // Wait for OTLP server to finish
    if let Err(e) = otlp_handle.await {
        error!("OTLP server task failed: {}", e);
    }

    info!("Server shutdown complete");
    Ok(())
}

/// Wait for SIGTERM or SIGINT (Ctrl+C) for graceful shutdown
async fn shutdown_signal() {
    let ctrl_c = async {
        if let Err(e) = signal::ctrl_c().await {
            tracing::warn!("Failed to install Ctrl+C handler: {}", e);
            std::future::pending::<()>().await;
        }
    };

    #[cfg(unix)]
    let terminate = async {
        match signal::unix::signal(signal::unix::SignalKind::terminate()) {
            Ok(mut signal) => {
                signal.recv().await;
            }
            Err(e) => {
                tracing::warn!("Failed to install SIGTERM handler: {}", e);
                std::future::pending::<()>().await;
            }
        }
    };

    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        () = ctrl_c => info!("Received SIGINT, initiating graceful shutdown"),
        () = terminate => info!("Received SIGTERM, initiating graceful shutdown"),
    }
}

// Add handler function for /v1/aggregate
async fn aggregate_handler(
    State(exporter): State<Arc<dyn LogExporter>>,
    body: String,
) -> impl IntoResponse {
    info!(
        "Received aggregate request with body length: {}",
        body.len()
    );

    // Handle empty body
    if body.is_empty() {
        return (StatusCode::OK, "No logs to process");
    }

    let logs: Vec<EnrichedLogEntry> = body
        .lines()
        .filter_map(|line| match serde_json::from_str(line) {
            Ok(entry) => Some(entry),
            Err(e) => {
                error!("Failed to parse log entry: {e} - Line: {line}");
                None
            }
        })
        .collect();

    info!("Parsed {} log entries from request", logs.len());

    // Handle case where no valid logs were parsed
    if logs.is_empty() {
        return (StatusCode::OK, "No valid logs to export");
    }

    let log_count = logs.len();

    match exporter.export_batch(logs).await {
        Ok(()) => {
            info!("Successfully exported {log_count} log entries to ClickHouse");
            (StatusCode::OK, "OK")
        }
        Err(e) => {
            error!("Failed to export logs to ClickHouse: {e}");
            (StatusCode::INTERNAL_SERVER_ERROR, "Export failed")
        }
    }
}
