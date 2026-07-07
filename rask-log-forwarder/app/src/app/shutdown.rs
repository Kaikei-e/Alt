use super::service::ServiceError;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::signal;
#[cfg(unix)]
use tokio::signal::unix::{signal as unix_signal, SignalKind};
use tokio::sync::{RwLock, mpsc};
use tokio_util::sync::CancellationToken;
use tracing::{error, info, warn};

#[derive(Debug)]
pub struct ShutdownHandle {
    shutdown_tx: mpsc::UnboundedSender<()>,
    signal_handler: SignalHandler,
    running: Arc<RwLock<bool>>,
}

impl ShutdownHandle {
    pub fn new(
        shutdown_tx: mpsc::UnboundedSender<()>,
        signal_handler: SignalHandler,
        running: Arc<RwLock<bool>>,
    ) -> Self {
        Self {
            shutdown_tx,
            signal_handler,
            running,
        }
    }

    pub async fn shutdown(self) -> Result<(), ServiceError> {
        info!("Initiating graceful shutdown...");

        // Send shutdown signal
        if self.shutdown_tx.send(()).is_err() {
            warn!("Shutdown channel already closed");
        }

        // Wait for service to stop
        // Note: 4 seconds is chosen to fit within Docker's stop_grace_period (12s)
        // while leaving buffer time for cleanup operations
        let timeout_duration = Duration::from_secs(4);
        let start = Instant::now();

        while *self.running.read().await && start.elapsed() < timeout_duration {
            tokio::time::sleep(Duration::from_millis(100)).await;
        }

        if *self.running.read().await {
            error!("Shutdown timeout exceeded");
            return Err(ServiceError::ShutdownTimeout);
        }

        info!("Graceful shutdown completed");
        Ok(())
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
                let mut sigterm =
                    unix_signal(SignalKind::terminate()).expect("Failed to create SIGTERM handler");

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
}
