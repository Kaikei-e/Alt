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
    background_tasks: Vec<tokio::task::JoinHandle<()>>,
}

impl ShutdownHandle {
    #[must_use]
    pub fn new(
        shutdown_tx: mpsc::UnboundedSender<()>,
        signal_handler: SignalHandler,
        processing_loop: tokio::task::JoinHandle<()>,
    ) -> Self {
        Self {
            shutdown_tx,
            signal_handler,
            processing_loop,
            background_tasks: Vec::new(),
        }
    }

    /// Attaches background task handles that should be joined or aborted when
    /// shutdown completes.
    #[must_use]
    pub fn with_background_tasks(mut self, tasks: Vec<tokio::task::JoinHandle<()>>) -> Self {
        self.background_tasks = tasks;
        self
    }

    /// Signals the processing loop to shut down and waits up to
    /// `SHUTDOWN_TIMEOUT` for it to drain and flush.
    ///
    /// # Errors
    ///
    /// Returns [`ServiceError::ShutdownTimeout`] if the processing loop panics
    /// or fails to complete within `SHUTDOWN_TIMEOUT`.
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
        let deadline = tokio::time::Instant::now() + SHUTDOWN_TIMEOUT;
        match tokio::time::timeout_at(deadline, self.processing_loop).await {
            Ok(Ok(())) => {
                for task in self.background_tasks {
                    match tokio::time::timeout_at(deadline, task).await {
                        Ok(Ok(())) => {}
                        Ok(Err(e)) => {
                            warn!("Background task panicked during shutdown: {e}");
                        }
                        Err(_) => {
                            warn!("Background task did not stop before shutdown deadline");
                        }
                    }
                }
                info!("Graceful shutdown completed");
                Ok(())
            }
            Ok(Err(e)) => {
                for task in &self.background_tasks {
                    task.abort();
                }
                error!("Processing loop task panicked during shutdown: {e}");
                Err(ServiceError::ShutdownTimeout)
            }
            Err(_) => {
                for task in &self.background_tasks {
                    task.abort();
                }
                error!("Shutdown timeout exceeded");
                Err(ServiceError::ShutdownTimeout)
            }
        }
    }

    /// Waits for either a termination signal (triggering graceful shutdown) or
    /// an unexpected exit of the processing loop.
    ///
    /// # Errors
    ///
    /// Returns [`ServiceError::ShutdownTimeout`] if graceful shutdown times
    /// out or panics, or [`ServiceError::CollectorError`] if the processing
    /// loop exits without a shutdown signal.
    pub async fn wait_for_shutdown(mut self) -> Result<(), ServiceError> {
        tokio::select! {
            () = self.signal_handler.wait() => {
                let result = self.shutdown().await;
                if let Err(ref e) = result {
                    error!("Shutdown error: {}", e);
                }
                result
            }
            join_result = &mut self.processing_loop => {
                match join_result {
                    Ok(()) => {
                        error!("Processing loop exited unexpectedly without shutdown signal");
                    }
                    Err(e) => {
                        error!("Processing loop task panicked: {e}");
                    }
                }
                Err(ServiceError::CollectorError(
                    crate::collector::CollectorError::CollectionStopped,
                ))
            }
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
    #[must_use]
    pub fn new(shutdown_tx: mpsc::UnboundedSender<()>) -> Self {
        let handler = Self {
            shutdown_tx,
            shutdown_token: CancellationToken::new(),
        };

        handler.setup_handlers();
        handler
    }

    fn setup_handlers(&self) {
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

    #[must_use]
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
    use std::sync::Arc;
    use std::sync::atomic::{AtomicBool, Ordering};
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
    /// constant, so raising `STOP_GRACE_PERIOD` here without touching
    /// `compose/logging.yaml` fails the build here instead of in production.
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

    #[tokio::test]
    async fn wait_for_shutdown_returns_when_processing_loop_exits_without_signal() {
        let (tx, _rx) = mpsc::unbounded_channel();
        let processing_loop = tokio::spawn(async {});

        let result = timeout(
            TokioDuration::from_millis(200),
            ShutdownHandle::new(tx, test_handler(), processing_loop).wait_for_shutdown(),
        )
        .await
        .expect(
            "wait_for_shutdown must return promptly when the processing loop finishes on its own \
             instead of hanging until SIGTERM",
        );

        assert!(
            matches!(
                result,
                Err(ServiceError::CollectorError(
                    crate::collector::CollectorError::CollectionStopped
                ))
            ),
            "unexpected processing-loop exit must return an error so the binary exits non-zero, got {result:?}"
        );
    }

    #[tokio::test]
    async fn wait_for_shutdown_succeeds_when_signal_arrives() {
        let (tx, processing_loop) = fake_processing_loop(Duration::from_millis(10));
        let handler = test_handler();
        handler.shutdown_token.cancel();

        let result = timeout(
            TokioDuration::from_millis(500),
            ShutdownHandle::new(tx, handler, processing_loop).wait_for_shutdown(),
        )
        .await
        .expect("wait_for_shutdown must complete once a signal is observed");

        assert!(
            result.is_ok(),
            "signal-triggered graceful shutdown must return Ok(()), got {result:?}"
        );
    }

    #[tokio::test(start_paused = true)]
    async fn shutdown_awaits_background_task_finishing_after_processing_loop() {
        // Background tasks may still be winding down after the processing loop
        // exits; shutdown must wait for them before returning so their final
        // work is not dropped.
        let (tx, processing_loop) = fake_processing_loop(Duration::from_millis(50));
        let completed = Arc::new(AtomicBool::new(false));
        let completed_task = Arc::clone(&completed);
        let task_duration = Duration::from_secs(2);
        let bg = tokio::spawn(async move {
            tokio::time::sleep(task_duration).await;
            completed_task.store(true, Ordering::Release);
        });

        let started = tokio::time::Instant::now();
        let result = ShutdownHandle::new(tx, test_handler(), processing_loop)
            .with_background_tasks(vec![bg])
            .shutdown()
            .await;

        assert!(
            result.is_ok(),
            "shutdown must return Ok(()) once background task finishes in time, got {result:?}"
        );
        assert!(
            completed.load(Ordering::Acquire),
            "shutdown returned before the background task finished its work"
        );
        assert!(
            started.elapsed() >= task_duration,
            "shutdown returned after {:?}, earlier than the {task_duration:?} background task",
            started.elapsed()
        );
    }

    #[tokio::test(start_paused = true)]
    async fn shutdown_shares_deadline_when_background_task_never_finishes() {
        // The processing loop and background tasks share one deadline derived
        // from stop_grace_period; restarting the clock for background work
        // would push shutdown past Docker's SIGKILL.
        let (tx, processing_loop) = fake_processing_loop(Duration::from_secs(3));
        let bg = tokio::spawn(async {
            tokio::time::sleep(Duration::from_secs(300)).await;
        });

        let started = tokio::time::Instant::now();
        let result = ShutdownHandle::new(tx, test_handler(), processing_loop)
            .with_background_tasks(vec![bg])
            .shutdown()
            .await;

        assert!(
            result.is_ok(),
            "a hung background task must log a warning and still return Ok(()), got {result:?}"
        );
        assert!(
            started.elapsed() >= SHUTDOWN_TIMEOUT,
            "shutdown gave up after {:?}, short of the {SHUTDOWN_TIMEOUT:?} shared deadline",
            started.elapsed()
        );
        assert!(
            started.elapsed() < STOP_GRACE_PERIOD,
            "shutdown gave up after {:?}, at or past the {STOP_GRACE_PERIOD:?} stop_grace_period - \
             deadline was not shared with the processing loop",
            started.elapsed()
        );
    }

    #[tokio::test(start_paused = true)]
    async fn shutdown_bounds_multiple_never_finishing_background_tasks_to_shared_deadline() {
        // Regression guard against a per-task timeout: two hung background
        // tasks must still stop waiting at the single shared deadline rather
        // than multiplying the wait time.
        let (tx, processing_loop) = fake_processing_loop(Duration::from_secs(3));
        let bg1 = tokio::spawn(async {
            tokio::time::sleep(Duration::from_secs(300)).await;
        });
        let bg2 = tokio::spawn(async {
            tokio::time::sleep(Duration::from_secs(300)).await;
        });

        let started = tokio::time::Instant::now();
        let result = ShutdownHandle::new(tx, test_handler(), processing_loop)
            .with_background_tasks(vec![bg1, bg2])
            .shutdown()
            .await;

        assert!(
            result.is_ok(),
            "multiple hung background tasks must still return Ok(()), got {result:?}"
        );
        assert!(
            started.elapsed() >= SHUTDOWN_TIMEOUT,
            "shutdown gave up after {:?}, short of the {SHUTDOWN_TIMEOUT:?} shared deadline",
            started.elapsed()
        );
        assert!(
            started.elapsed() < STOP_GRACE_PERIOD,
            "shutdown gave up after {:?}, at or past the {STOP_GRACE_PERIOD:?} stop_grace_period - \
             background tasks appear to each get their own deadline",
            started.elapsed()
        );
    }

    #[tokio::test(start_paused = true)]
    async fn shutdown_returns_ok_when_background_task_panics() {
        // A panicked background task should be logged as a warning without
        // turning an otherwise clean processing-loop flush into an error exit.
        let (tx, processing_loop) = fake_processing_loop(Duration::from_millis(50));
        let bg = tokio::spawn(async {
            tokio::time::sleep(Duration::from_millis(100)).await;
            panic!("simulated background task panic");
        });

        let result = ShutdownHandle::new(tx, test_handler(), processing_loop)
            .with_background_tasks(vec![bg])
            .shutdown()
            .await;

        assert!(
            result.is_ok(),
            "panicking background task must not fail shutdown, got {result:?}"
        );
    }

    #[tokio::test(start_paused = true)]
    async fn shutdown_aborts_background_tasks_when_processing_loop_times_out() {
        // When the processing loop itself hangs past the deadline, remaining
        // background tasks must be aborted so they do not linger past timeout.
        struct DropFlag(Arc<AtomicBool>);
        impl Drop for DropFlag {
            fn drop(&mut self) {
                self.0.store(true, Ordering::Release);
            }
        }

        let (tx, processing_loop) = fake_processing_loop(Duration::from_secs(300));
        let aborted = Arc::new(AtomicBool::new(false));
        let guard = DropFlag(Arc::clone(&aborted));
        let bg = tokio::spawn(async move {
            std::future::pending::<()>().await;
            drop(guard);
        });

        let result = ShutdownHandle::new(tx, test_handler(), processing_loop)
            .with_background_tasks(vec![bg])
            .shutdown()
            .await;

        assert!(
            matches!(result, Err(ServiceError::ShutdownTimeout)),
            "timed-out processing loop must return Err(ServiceError::ShutdownTimeout), got {result:?}"
        );

        for _ in 0..4 {
            tokio::task::yield_now().await;
        }
        assert!(
            aborted.load(Ordering::Acquire),
            "background task was not aborted when processing loop timed out"
        );
    }
}
