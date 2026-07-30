//! Bounds how long the process may spend becoming able to serve.
//!
//! Startup builds every client, pool and model before the HTTP listener is
//! bound, so anything that blocks in there takes the whole API down — and a
//! block is not a failure, so no fail-closed policy can fire on it. A silent
//! dependency stall once held this process short of `TcpListener::bind` for
//! more than sixteen hours while the container reported `Up`.
//!
//! The deadline turns that into a non-zero exit, which the container restart
//! policy already knows how to retry with backoff.

use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::Duration;

/// Granularity at which the watchdog notices that startup finished. Small
/// enough that a completed startup does not linger, large enough to be free.
const POLL_INTERVAL: Duration = Duration::from_millis(250);

/// Marks startup as finished, disarming the watchdog.
///
/// Deliberately not `Drop`-based: the watchdog must keep running if this value
/// is dropped on an error path, and "startup got far enough to serve" is a
/// decision the caller states explicitly rather than one inferred from scope.
pub struct StartupGuard {
    finished: Arc<AtomicBool>,
}

impl StartupGuard {
    /// Record that the service reached the point where it can serve traffic.
    pub fn startup_complete(&self) {
        self.finished.store(true, Ordering::Release);
    }
}

/// Run `on_timeout` unless [`StartupGuard::startup_complete`] is called within
/// `deadline`.
///
/// The watchdog is a plain OS thread, not a tokio task: the failures worth
/// catching here are exactly the ones that block runtime workers, and a timer
/// that needs a free worker to fire cannot be trusted to observe them.
pub fn watch_startup<F>(deadline: Duration, on_timeout: F) -> StartupGuard
where
    F: FnOnce() + Send + 'static,
{
    let finished = Arc::new(AtomicBool::new(false));
    let observed = Arc::clone(&finished);

    // Counted ticks rather than a clock read: the thread only needs to know
    // whether the budget is spent, not what time it is.
    let ticks = (deadline.as_millis() / POLL_INTERVAL.as_millis()).max(1);

    let spawned = std::thread::Builder::new()
        .name("startup-watchdog".to_string())
        .spawn(move || {
            for _ in 0..ticks {
                std::thread::sleep(POLL_INTERVAL);
                if observed.load(Ordering::Acquire) {
                    return;
                }
            }
            if !observed.load(Ordering::Acquire) {
                on_timeout();
            }
        });

    if let Err(error) = spawned {
        // Without the watchdog the process can wedge unnoticed, which is the
        // whole failure this module exists to prevent. Say so loudly rather
        // than starting up in a state nobody can distinguish from healthy.
        tracing::error!(
            %error,
            "startup_watchdog_unavailable: a stalled dependency will no longer \
             be bounded by the startup deadline"
        );
    }

    StartupGuard { finished }
}

#[cfg(test)]
mod tests {
    use super::watch_startup;
    use std::sync::Arc;
    use std::sync::atomic::{AtomicBool, Ordering};
    use std::time::Duration;

    const DEADLINE: Duration = Duration::from_millis(50);
    const SETTLE: Duration = Duration::from_millis(600);

    #[test]
    fn fires_when_startup_never_completes() {
        let fired = Arc::new(AtomicBool::new(false));
        let flag = Arc::clone(&fired);

        let _guard = watch_startup(DEADLINE, move || flag.store(true, Ordering::Release));
        std::thread::sleep(SETTLE);

        assert!(
            fired.load(Ordering::Acquire),
            "a startup that never completes must trip the deadline"
        );
    }

    #[test]
    fn stays_quiet_when_startup_completes() {
        let fired = Arc::new(AtomicBool::new(false));
        let flag = Arc::clone(&fired);

        let guard = watch_startup(DEADLINE, move || flag.store(true, Ordering::Release));
        guard.startup_complete();
        std::thread::sleep(SETTLE);

        assert!(
            !fired.load(Ordering::Acquire),
            "a completed startup must not be killed"
        );
    }
}
