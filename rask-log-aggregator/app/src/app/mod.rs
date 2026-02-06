mod router;
pub mod server;
mod state;
pub mod tracing;

use crate::config;
use crate::error::AggregatorError;
use tokio_util::sync::CancellationToken;

/// Application entry point. Initializes tracing, configuration, and starts servers.
pub async fn run() -> Result<(), AggregatorError> {
    // Handle healthcheck subcommand (for Docker healthcheck in distroless image)
    if std::env::args().nth(1).as_deref() == Some("healthcheck") {
        match crate::healthcheck().await {
            Ok(()) => std::process::exit(0),
            Err(e) => {
                eprintln!("Healthcheck failed: {e}");
                std::process::exit(1)
            }
        }
    }

    tracing::init_tracing();

    let settings =
        config::get_configuration().map_err(|e| AggregatorError::Config(e.to_string()))?;
    ::tracing::info!("Loaded settings");

    // Shared shutdown token: used by BatchWriter and both servers
    let shutdown_token = CancellationToken::new();

    let app_state = state::AppState::from_settings(&settings, shutdown_token.clone());

    let main_app = router::main_router(app_state.log_exporter);
    let otlp_app = router::otlp_router(app_state.otel_exporter);

    server::serve(
        main_app,
        otlp_app,
        settings.http_port,
        settings.otlp_http_port,
        shutdown_token,
    )
    .await
}
