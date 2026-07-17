use crate::error::AggregatorError;
use axum::Router;
use tokio::signal;
use tokio_util::sync::CancellationToken;
use tracing::{error, info};

/// Start both the main and OTLP servers with graceful shutdown.
///
/// The `shutdown_token` is shared with `BatchWriter` so that the flush loop
/// drains remaining rows before the process exits.
///
/// Both server tasks are watched via `select!`: if either exits with an error,
/// the other is cancelled so the process does not limp on a half-dead listener.
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

    // Signal handler cancels the shared token for both servers.
    let signal_token = shutdown_token.clone();
    tokio::spawn(async move {
        shutdown_signal().await;
        signal_token.cancel();
    });

    let main_shutdown = shutdown_token.child_token();
    let mut main_handle = tokio::spawn(async move {
        axum::serve(main_listener, main_app)
            .with_graceful_shutdown(main_shutdown.cancelled_owned())
            .await
    });

    let otlp_shutdown = shutdown_token.child_token();
    let mut otlp_handle = tokio::spawn(async move {
        axum::serve(otlp_listener, otlp_app)
            .with_graceful_shutdown(otlp_shutdown.cancelled_owned())
            .await
    });

    // Watch both tasks; if either fails, cancel the other so we don't keep serving
    // on a single listener while the peer is dead.
    let serve_result = tokio::select! {
        result = &mut main_handle => {
            shutdown_token.cancel();
            flatten_server_result("main", result)
        }
        result = &mut otlp_handle => {
            shutdown_token.cancel();
            flatten_server_result("otlp", result)
        }
    };

    // Ensure the peer task finishes after cancel (JoinHandle kept alive via &mut select).
    if !main_handle.is_finished()
        && let Err(e) = main_handle.await
    {
        error!("Main server task join failed during shutdown: {e}");
    }
    if !otlp_handle.is_finished()
        && let Err(e) = otlp_handle.await
    {
        error!("OTLP server task join failed during shutdown: {e}");
    }

    serve_result?;
    info!("Server shutdown complete");
    Ok(())
}

fn flatten_server_result(
    name: &str,
    result: Result<Result<(), std::io::Error>, tokio::task::JoinError>,
) -> Result<(), AggregatorError> {
    match result {
        Ok(Ok(())) => Ok(()),
        Ok(Err(e)) => {
            error!("{name} server exited with error: {e}");
            Err(e.into())
        }
        Err(e) => {
            error!("{name} server task panicked: {e}");
            Err(AggregatorError::Export(format!(
                "{name} server task panicked: {e}"
            )))
        }
    }
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
