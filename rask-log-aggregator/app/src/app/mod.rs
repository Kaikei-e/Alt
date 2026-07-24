mod router;
pub mod server;
mod state;
pub mod tracing;

use crate::config;
use crate::error::AggregatorError;
use std::time::Duration;
use tokio::task::JoinHandle;
use tokio_util::sync::CancellationToken;

/// Upper bound on how long to wait for the `BatchWriter` flush loop to
/// drain and write its final batch on shutdown.
///
/// Kept comfortably under Docker Compose's default `stop_grace_period`
/// (10s) so the process exits itself — logging what happened — instead of
/// being SIGKILLed mid-flush with no trace of the loss.
const FLUSH_SHUTDOWN_TIMEOUT: Duration = Duration::from_secs(8);

/// Application entry point. Initializes tracing, configuration, and starts servers.
pub async fn run() -> Result<(), AggregatorError> {
    // Handle healthcheck subcommand (for Docker healthcheck in distroless image).
    // Tracing is not fully configured yet; install a minimal stderr subscriber so
    // failures still go through the tracing facade instead of bare `eprintln!`.
    if std::env::args().nth(1).as_deref() == Some("healthcheck") {
        let _ = tracing_subscriber::fmt()
            .with_writer(std::io::stderr)
            .with_max_level(::tracing::Level::ERROR)
            .try_init();
        match crate::healthcheck().await {
            Ok(()) => std::process::exit(0),
            Err(e) => {
                ::tracing::error!(error = %e, "Healthcheck failed");
                std::process::exit(1)
            }
        }
    }

    tracing::init_tracing();

    let settings = config::get_configuration()?;
    ::tracing::info!("Loaded settings");

    // Shared shutdown token: used by BatchWriter and both servers
    let shutdown_token = CancellationToken::new();

    let app_state = state::AppState::from_settings(&settings, shutdown_token.clone());

    let main_app = router::main_router(app_state.log_exporter);
    let otlp_app = router::otlp_router(app_state.otel_exporter);

    let serve_result = server::serve(
        main_app,
        otlp_app,
        settings.http_port,
        settings.otlp_http_port,
        shutdown_token.clone(),
    )
    .await;

    // `serve` already cancels `shutdown_token` on SIGTERM/SIGINT before it
    // returns, but if it returned early via an I/O error that path never
    // ran — cancel unconditionally here as a safety net. Then hold and await
    // the flush loop's JoinHandle so the runtime can't drop it mid-write:
    // detached background tasks get silently aborted on shutdown, losing
    // the final batch.
    shutdown_token.cancel();
    await_flush_shutdown(app_state.flush_handle, FLUSH_SHUTDOWN_TIMEOUT).await;

    serve_result
}

/// Await the `BatchWriter` flush loop's `JoinHandle`, bounded by `timeout`.
///
/// The flush loop's shutdown branch can legitimately take a while (ClickHouse
/// retries with their own send/end timeouts), but it must never be allowed to
/// run past the container's stop grace period: an unbounded await here means
/// Docker SIGKILLs the process mid-flush, losing the drained-but-unwritten
/// batch without so much as a log line. Timing out here instead lets the
/// process log the loss and exit on its own terms.
async fn await_flush_shutdown(handle: JoinHandle<()>, timeout: Duration) {
    match tokio::time::timeout(timeout, handle).await {
        Ok(Ok(())) => {}
        Ok(Err(e)) => {
            ::tracing::error!("BatchWriter flush task panicked: {e}");
        }
        Err(_) => {
            ::tracing::error!(
                timeout_secs = timeout.as_secs(),
                "BatchWriter flush task did not complete before shutdown timeout; \
                 buffered rows may not have been written to ClickHouse"
            );
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn await_flush_shutdown_returns_promptly_when_flush_completes_first() {
        let handle = tokio::spawn(async {});

        let elapsed = {
            let start = tokio::time::Instant::now();
            await_flush_shutdown(handle, Duration::from_secs(5)).await;
            start.elapsed()
        };

        assert!(
            elapsed < Duration::from_secs(1),
            "should return as soon as the flush task completes, not wait out the timeout"
        );
    }

    #[tokio::test(start_paused = true)]
    async fn await_flush_shutdown_times_out_instead_of_hanging_forever() {
        // Simulates a flush loop stuck retrying against a down ClickHouse:
        // the handle never completes on its own.
        let handle = tokio::spawn(async {
            tokio::time::sleep(Duration::from_secs(3600)).await;
        });

        let timeout = Duration::from_secs(8);
        let start = tokio::time::Instant::now();
        await_flush_shutdown(handle, timeout).await;

        assert!(
            start.elapsed() >= timeout,
            "must not return before the configured timeout elapses"
        );
        assert!(
            start.elapsed() < Duration::from_secs(3600),
            "must not wait for the stuck flush task to finish"
        );
    }
}
