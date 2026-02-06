use super::service::ServiceError;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::signal;
#[cfg(unix)]
use tokio::signal::unix::{signal as unix_signal, SignalKind};
use tokio::sync::{RwLock, mpsc};
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
    active: Arc<RwLock<bool>>,
}

impl SignalHandler {
    pub async fn new(shutdown_tx: mpsc::UnboundedSender<()>) -> Self {
        let handler = Self {
            shutdown_tx,
            active: Arc::new(RwLock::new(true)),
        };

        handler.setup_handlers().await;
        handler
    }

    async fn setup_handlers(&self) {
        let shutdown_tx = self.shutdown_tx.clone();
        let active = self.active.clone();

        tokio::spawn(async move {
            if *active.read().await {
                #[cfg(unix)]
                {
                    let mut sigterm = unix_signal(SignalKind::terminate())
                        .expect("Failed to create SIGTERM handler");

                    tokio::select! {
                        result = signal::ctrl_c() => {
                            match result {
                                Ok(()) => {
                                    info!("Received SIGINT (Ctrl+C), initiating graceful shutdown");
                                }
                                Err(err) => {
                                    error!("Failed to listen for SIGINT: {}", err);
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
                }
            }
        });
    }

    pub async fn is_active(&self) -> bool {
        *self.active.read().await
    }

    pub async fn wait(&self) {
        while *self.active.read().await {
            tokio::time::sleep(Duration::from_millis(100)).await;
        }
    }
}
