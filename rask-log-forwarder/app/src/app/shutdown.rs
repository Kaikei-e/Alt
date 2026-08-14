use super::service::ServiceError;
use std::time::Duration;
use tokio::signal;
#[cfg(unix)]
use tokio::signal::unix::{SignalKind, signal as unix_signal};
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;
use tracing::{error, info, warn};

/// Docker's `stop_grace_period` for the forwarder containers
/// (`compose/logging.yaml`): SIGKILL lands this long after SIGTERM.
const STOP_GRACE_PERIOD: Duration = Duration::from_secs(12);

/// Deadline for the processing loop to drain the channel and flush its last
/// batch. That final flush goes through the retry ladder before it reaches
/// disk fallback, so it needs as much of the grace period as we can give it -
/// a shorter deadline drops the batch without sending it or persisting it.
/// The remaining headroom is for the runtime to unwind and the tracing
/// subscriber to flush before Docker SIGKILLs us.
const SHUTDOWN_TIMEOUT: Duration = Duration::from_secs(STOP_GRACE_PERIOD.as_secs() - 2);

#[derive(Debug)]
pub struct ShutdownHandle {
    shutdown_tx: mpsc::UnboundedSender<()>,
    signal_handler: SignalHandler,
    processing_loop: tokio::task::JoinHandle<()>,
}

impl ShutdownHandle {
    pub fn new(
        shutdown_tx: mpsc::UnboundedSender<()>,
        signal_handler: SignalHandler,
        processing_loop: tokio::task::JoinHandle<()>,
    ) -> Self {
        Self {
            shutdown_tx,
            signal_handler,
            processing_loop,
        }
    }

    pub async fn shutdown(self) -> Result<(), ServiceError> {
        info!("Initiating graceful shutdown...");

        // Send shutdown signal
        if self.shutdown_tx.send(()).is_err() {
            warn!("Shutdown channel already closed");
        }

        // Wait for the processing loop task to actually finish rather than
        // polling a flag it sets - this resolves the instant the task exits
        // instead of up to 100ms late, and doesn't spin a wakeup every tick
        // while shutdown is in progress.
        match tokio::time::timeout(SHUTDOWN_TIMEOUT, self.processing_loop).await {
            Ok(Ok(())) => {
                info!("Graceful shutdown completed");
                Ok(())
            }
            Ok(Err(e)) => {
                error!("Processing loop task panicked during shutdown: {e}");
                Err(ServiceError::ShutdownTimeout)
            }
            Err(_) => {
                error!("Shutdown timeout exceeded");
                Err(ServiceError::ShutdownTimeout)
            }
        }
    }

    pub async fn wait_for_shutdown(self) {
        self.signal_handler.wait().await;
        if let Err(e) = self.shutdown().await {
            error!("Shutdown error: {}", e);
        }
    }
}

#[derive(Debug)]
pub struct SignalHandler {
    shutdown_tx: mpsc::UnboundedSender<()>,
    // Cancelled by `setup_handlers` once a shutdown signal (SIGTERM/SIGINT) is
    // observed. `wait()` resolves as soon as this token is cancelled instead of
    // polling a flag that nothing ever flips.
    shutdown_token: CancellationToken,
}

impl SignalHandler {
    pub async fn new(shutdown_tx: mpsc::UnboundedSender<()>) -> Self {
        let handler = Self {
            shutdown_tx,
            shutdown_token: CancellationToken::new(),
        };

        handler.setup_handlers().await;
        handler
    }

    async fn setup_handlers(&self) {
        let shutdown_tx = self.shutdown_tx.clone();
        let shutdown_token = self.shutdown_token.clone();

        tokio::spawn(async move {
            #[cfg(unix)]
            {
                match unix_signal(SignalKind::terminate()) {
                    Ok(mut sigterm) => {
                        tokio::select! {
                            result = signal::ctrl_c() => {
                                match result {
                                    Ok(()) => {
                                        info!("Received SIGINT (Ctrl+C), initiating graceful shutdown");
                                    }
                                    Err(err) => {
                                        error!("Failed to listen for SIGINT: {}", err);
                                        shutdown_token.cancel();
                                        return;
                                    }
                                }
                            }
                            _ = sigterm.recv() => {
                                info!("Received SIGTERM, initiating graceful shutdown");
                            }
                        }
                    }
                    Err(err) => {
                        // Fall back to SIGINT-only handling instead of
                        // panicking the signal-handling task in production.
                        error!(
                            "Failed to create SIGTERM handler: {err}; falling back to SIGINT (Ctrl+C) only"
                        );
                        match signal::ctrl_c().await {
                            Ok(()) => {
                                info!("Received SIGINT (Ctrl+C), initiating graceful shutdown");
                            }
                            Err(err) => {
                                error!("Failed to listen for SIGINT: {}", err);
                                shutdown_token.cancel();
                                return;
                            }
                        }
                    }
                }

                if shutdown_tx.send(()).is_err() {
                    error!("Failed to send shutdown signal");
                }
                shutdown_token.cancel();
            }

            #[cfg(not(unix))]
            {
                match signal::ctrl_c().await {
                    Ok(()) => {
                        info!("Received SIGINT (Ctrl+C), initiating graceful shutdown");
                        if shutdown_tx.send(()).is_err() {
                            error!("Failed to send shutdown signal");
                        }
                    }
                    Err(err) => {
                        error!("Failed to listen for SIGINT: {}", err);
                    }
                }
                shutdown_token.cancel();
            }
        });
    }

    pub fn is_active(&self) -> bool {
        !self.shutdown_token.is_cancelled()
    }

    /// Resolves as soon as a shutdown signal has been observed (or the handler
    /// is otherwise cancelled), instead of hanging forever.
    pub async fn wait(&self) {
        self.shutdown_token.cancelled().await;
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tokio::time::{Duration as TokioDuration, timeout};

    fn test_handler() -> SignalHandler {
        let (tx, _rx) = mpsc::unbounded_channel();
        SignalHandler {
            shutdown_tx: tx,
            shutdown_token: CancellationToken::new(),
        }
    }

    #[tokio::test]
    async fn wait_returns_immediately_when_already_cancelled() {
        let handler = test_handler();
        handler.shutdown_token.cancel();

        timeout(TokioDuration::from_millis(200), handler.wait())
            .await
            .expect("wait() must return once the shutdown token is cancelled");
        assert!(!handler.is_active());
    }

    #[tokio::test]
    async fn wait_resolves_once_signal_is_observed() {
        let handler = test_handler();
        assert!(handler.is_active());

        let token = handler.shutdown_token.clone();
        tokio::spawn(async move {
            tokio::time::sleep(TokioDuration::from_millis(50)).await;
            // Simulates what setup_handlers() does on SIGTERM/SIGINT receipt.
            token.cancel();
        });

        timeout(TokioDuration::from_secs(2), handler.wait())
            .await
            .expect("wait() never returned after the signal was observed - it would hang until SIGKILL in production");
        assert!(!handler.is_active());
    }

    #[tokio::test]
    async fn wait_does_not_return_before_cancellation() {
        let handler = test_handler();

        let wait_result = timeout(TokioDuration::from_millis(150), handler.wait()).await;
        assert!(
            wait_result.is_err(),
            "wait() returned before any signal was observed"
        );
    }

    /// Stands in for `pipeline::run_processing_loop`: wakes on the shutdown
    /// signal, then spends `flush_duration` draining and flushing its last
    /// batch (the real one walks the retry ladder before disk fallback).
    fn fake_processing_loop(
        flush_duration: Duration,
    ) -> (mpsc::UnboundedSender<()>, tokio::task::JoinHandle<()>) {
        let (tx, mut rx) = mpsc::unbounded_channel();
        let handle = tokio::spawn(async move {
            rx.recv().await;
            tokio::time::sleep(flush_duration).await;
        });
        (tx, handle)
    }

    #[tokio::test(start_paused = true)]
    async fn shutdown_waits_for_a_final_flush_that_needs_most_of_the_grace_period() {
        // The last batch only reaches the aggregator (or disk fallback) after
        // the retry ladder has run, which routinely takes several seconds. A
        // deadline that expires first drops those entries entirely.
        let (tx, processing_loop) = fake_processing_loop(Duration::from_secs(9));

        ShutdownHandle::new(tx, test_handler(), processing_loop)
            .shutdown()
            .await
            .expect(
                "shutdown must wait out the final flush - giving up early drops the last batch \
                 without sending it or persisting it to disk",
            );
    }

    #[tokio::test(start_paused = true)]
    async fn shutdown_gives_up_inside_the_container_stop_grace_period() {
        // The deadline still has to land before Docker's SIGKILL, otherwise
        // the process is killed mid-flush instead of reporting the timeout.
        let (tx, processing_loop) = fake_processing_loop(Duration::from_secs(300));

        let started = tokio::time::Instant::now();
        let result = ShutdownHandle::new(tx, test_handler(), processing_loop)
            .shutdown()
            .await;

        assert!(
            matches!(result, Err(ServiceError::ShutdownTimeout)),
            "a processing loop that never finishes must surface as a shutdown timeout"
        );
        // Lower bound first: this is what makes the test a regression guard for
        // the 4s deadline. Without it the assertion below is satisfied by
        // construction, since SHUTDOWN_TIMEOUT is derived from STOP_GRACE_PERIOD.
        assert!(
            started.elapsed() >= SHUTDOWN_TIMEOUT,
            "shutdown gave up after {:?}, short of the {SHUTDOWN_TIMEOUT:?} deadline - the final \
             flush is being cut off early",
            started.elapsed()
        );
        assert!(
            started.elapsed() < STOP_GRACE_PERIOD,
            "shutdown gave up after {:?}, at or past the {STOP_GRACE_PERIOD:?} stop_grace_period - \
             Docker would SIGKILL us before we could report it",
            started.elapsed()
        );
    }

    /// The deadline and compose's `stop_grace_period` are one contract living in
    /// two files. Parse the container spec rather than trusting the local
    /// constant, so raising STOP_GRACE_PERIOD here without touching
    /// compose/logging.yaml fails the build here instead of in production.
    #[test]
    fn stop_grace_period_matches_the_compose_service_spec() {
        const LOGGING_COMPOSE: &str = include_str!(concat!(
            env!("CARGO_MANIFEST_DIR"),
            "/../../compose/logging.yaml"
        ));

        let declared: Vec<u64> = LOGGING_COMPOSE
            .lines()
            .filter_map(|line| line.trim().strip_prefix("stop_grace_period:"))
            .map(|value| {
                value
                    .trim()
                    .trim_end_matches('s')
                    .parse::<u64>()
                    .expect("stop_grace_period must be a whole number of seconds")
            })
            .collect();

        assert_eq!(
            declared.len(),
            1,
            "expected exactly one stop_grace_period in compose/logging.yaml, found {declared:?} - \
             this test would otherwise pin the wrong service's value"
        );
        assert_eq!(
            STOP_GRACE_PERIOD.as_secs(),
            declared[0],
            "STOP_GRACE_PERIOD disagrees with compose/logging.yaml; the deadline derived from it \
             would let Docker SIGKILL the forwarder mid-flush"
        );
    }
}
