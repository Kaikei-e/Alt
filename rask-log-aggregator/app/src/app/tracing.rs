use tracing::Level;
use tracing_subscriber::{EnvFilter, fmt, prelude::*};

/// Initialize the tracing subscriber.
/// Uses JSON format when `RUST_LOG_FORMAT=json` (default for production).
pub fn init_tracing() {
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
}
