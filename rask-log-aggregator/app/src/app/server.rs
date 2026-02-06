use crate::error::AggregatorError;
use axum::Router;
use tokio::signal;
use tokio_util::sync::CancellationToken;
use tracing::{error, info};

/// Start both the main and OTLP servers with graceful shutdown.
///
/// The `shutdown_token` is shared with `BatchWriter` so that the flush loop
/// drains remaining rows before the process exits.
pub async fn serve(
    main_app: Router,
    otlp_app: Router,
    http_port: u16,
    otlp_http_port: u16,
    shutdown_token: CancellationToken,
) -> Result<(), AggregatorError> {
    // Bind main server
    let main_bind_addr = format!("0.0.0.0:{http_port}");
    let main_listener = tokio::net::TcpListener::bind(&main_bind_addr)
        .await
        .map_err(|e| AggregatorError::Bind {
            address: main_bind_addr.clone(),
            source: e,
        })?;
    info!("Main server listening on {}", main_listener.local_addr()?);
    info!("  - GET  /v1/health     (health check)");
    info!("  - POST /v1/aggregate  (legacy NDJSON logs)");

    // Bind OTLP server
    let otlp_bind_addr = format!("0.0.0.0:{otlp_http_port}");
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

    // Spawn OTLP server task
    let otlp_shutdown = shutdown_token.child_token();
    let otlp_handle = tokio::spawn(async move {
        axum::serve(otlp_listener, otlp_app)
            .with_graceful_shutdown(otlp_shutdown.cancelled_owned())
            .await
    });

    // Run main server
    let main_shutdown = shutdown_token.clone();
    axum::serve(main_listener, main_app)
        .with_graceful_shutdown(async move {
            shutdown_signal().await;
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

/// Wait for SIGTERM or SIGINT (Ctrl+C) for graceful shutdown.
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
