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

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CardsTriggerPosture {
    Enabled(uuid::Uuid),
    Disabled,
}

/// Emit the startup log line for the cards manual trigger.
///
/// Logged next to the `cards_job_enabled/disabled` line so operators can
/// determine whether manual card generation can be triggered over HTTP.
pub fn log_cards_trigger_status(user_id: Option<uuid::Uuid>) -> CardsTriggerPosture {
    if let Some(user_id) = user_id {
        tracing::info!(%user_id, "cards_trigger_enabled");
        CardsTriggerPosture::Enabled(user_id)
    } else {
        tracing::info!("cards_trigger_disabled");
        CardsTriggerPosture::Disabled
    }
}

pub fn eval_listener_outside_peer_guard_addr(
    peer_identity: &crate::config::PeerIdentityConfig,
    eval_listener: &crate::config::EvalListenerConfig,
) -> Option<std::net::SocketAddr> {
    match (peer_identity, eval_listener) {
        (
            crate::config::PeerIdentityConfig::Enabled { .. },
            crate::config::EvalListenerConfig::Enabled { listen_addr, .. },
        ) => Some(*listen_addr),
        _ => None,
    }
}

pub fn log_eval_listener_outside_peer_guard(
    peer_identity: &crate::config::PeerIdentityConfig,
    eval_listener: &crate::config::EvalListenerConfig,
) -> Option<std::net::SocketAddr> {
    let addr = eval_listener_outside_peer_guard_addr(peer_identity, eval_listener)?;
    tracing::warn!(
        listen_addr = %addr,
        "eval_listener_outside_peer_guard: /v1/eval/* is served outside the mTLS peer guard and is authenticated by the admin bearer token only"
    );
    Some(addr)
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
    fn test_log_cards_trigger_status() {
        let user_id = uuid::Uuid::new_v4();
        assert_eq!(
            super::log_cards_trigger_status(Some(user_id)),
            super::CardsTriggerPosture::Enabled(user_id)
        );
        assert_eq!(
            super::log_cards_trigger_status(None),
            super::CardsTriggerPosture::Disabled
        );
    }

    #[test]
    fn eval_listener_outside_peer_guard_decision_covers_all_four_combinations_and_warns_once() {
        use crate::config::{EvalListenerConfig, PeerIdentityConfig};
        use std::net::SocketAddr;
        use std::sync::{Arc, Mutex};

        #[derive(Clone, Default)]
        struct BufWriter(Arc<Mutex<Vec<u8>>>);
        impl std::io::Write for BufWriter {
            fn write(&mut self, buf: &[u8]) -> std::io::Result<usize> {
                self.0.lock().unwrap().extend_from_slice(buf);
                Ok(buf.len())
            }
            fn flush(&mut self) -> std::io::Result<()> {
                Ok(())
            }
        }

        let listen_addr: SocketAddr = "127.0.0.1:9006".parse().unwrap();
        let peer_disabled = PeerIdentityConfig::Disabled;
        let peer_enabled = PeerIdentityConfig::Enabled {
            allowed_peers: vec!["alt-backend".to_string()],
        };
        let eval_disabled = EvalListenerConfig::Disabled;
        let eval_enabled = EvalListenerConfig::Enabled {
            listen_addr,
            admin_token: "test-admin-token-0123456789".to_string(),
        };

        assert_eq!(
            super::eval_listener_outside_peer_guard_addr(&peer_disabled, &eval_disabled),
            None
        );
        assert_eq!(
            super::eval_listener_outside_peer_guard_addr(&peer_disabled, &eval_enabled),
            None
        );
        assert_eq!(
            super::eval_listener_outside_peer_guard_addr(&peer_enabled, &eval_disabled),
            None
        );
        assert_eq!(
            super::eval_listener_outside_peer_guard_addr(&peer_enabled, &eval_enabled),
            Some(listen_addr)
        );

        let buf = BufWriter::default();
        let writer = buf.clone();
        let subscriber = tracing_subscriber::fmt()
            .with_max_level(tracing::Level::WARN)
            .without_time()
            .with_writer(move || writer.clone())
            .finish();

        tracing::subscriber::with_default(subscriber, || {
            assert_eq!(
                super::log_eval_listener_outside_peer_guard(&peer_disabled, &eval_disabled),
                None
            );
            assert_eq!(
                super::log_eval_listener_outside_peer_guard(&peer_disabled, &eval_enabled),
                None
            );
            assert_eq!(
                super::log_eval_listener_outside_peer_guard(&peer_enabled, &eval_disabled),
                None
            );
            assert_eq!(
                super::log_eval_listener_outside_peer_guard(&peer_enabled, &eval_enabled),
                Some(listen_addr)
            );
        });

        let captured = String::from_utf8(buf.0.lock().unwrap().clone()).expect("utf8");
        let lines: Vec<&str> = captured.lines().collect();
        assert_eq!(
            lines.len(),
            1,
            "expected exactly one WARN line across all four combinations, got: {captured}"
        );
        assert!(captured.contains("WARN"));
        assert!(captured.contains("eval_listener_outside_peer_guard"));
        assert!(captured.contains("127.0.0.1:9006"));
        assert!(captured.contains("/v1/eval/*"));
        assert!(captured.contains("bearer"));
    }

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
