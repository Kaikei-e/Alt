#[cfg(not(target_env = "msvc"))]
use tikv_jemallocator::Jemalloc;

#[cfg(not(target_env = "msvc"))]
#[global_allocator]
static GLOBAL: Jemalloc = Jemalloc;

use anyhow::Context;
use std::env;
use std::time::Duration;
use tokio::net::TcpListener;
use tokio_util::sync::CancellationToken;
use tracing::{error, info, warn};

use recap_worker::{
    app::{ComponentRegistry, build_router},
    config::Config,
    scheduler::daemon::spawn_jst_batch_daemon,
};

mod cli;

/// Wait for SIGINT (Ctrl-C) or SIGTERM, whichever arrives first.
///
/// SIGTERM is the signal `docker stop` / Kubernetes send; without handling
/// it explicitly the process previously relied on the runtime killing it
/// outright once its shutdown grace period elapsed, with no chance to drain
/// in-flight work or flush telemetry.
async fn wait_for_shutdown_signal() {
    let ctrl_c = async {
        let _ = tokio::signal::ctrl_c().await;
    };

    #[cfg(unix)]
    let terminate = async {
        match tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate()) {
            Ok(mut sig) => {
                sig.recv().await;
            }
            Err(e) => {
                error!(error = %e, "failed to install SIGTERM handler");
            }
        }
    };

    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        () = ctrl_c => { info!("received SIGINT/Ctrl-C, starting graceful shutdown"); }
        () = terminate => { info!("received SIGTERM, starting graceful shutdown"); }
    }
}

/// How long the process may spend getting to a bound listener. Generous enough
/// for a cold model load on a busy host, short enough that a stalled dependency
/// becomes a restart rather than an open-ended outage.
const DEFAULT_STARTUP_DEADLINE_SECS: u64 = 180;

/// Resolve the startup deadline, rejecting a value that was set but unusable
/// rather than quietly falling back to the default.
fn startup_deadline() -> anyhow::Result<Duration> {
    const NAME: &str = "RECAP_WORKER_STARTUP_DEADLINE_SECS";
    match env::var(NAME) {
        Ok(raw) => {
            let secs: u64 = raw.trim().parse().with_context(|| {
                format!("{NAME} must be a whole number of seconds, got {raw:?}")
            })?;
            Ok(Duration::from_secs(secs))
        }
        Err(_) => Ok(Duration::from_secs(DEFAULT_STARTUP_DEADLINE_SECS)),
    }
}

/// Bound the stretch of startup that runs before the listener is bound.
///
/// Until then the service can answer nothing, and a dependency that blocks
/// rather than fails cannot be caught by any fail-closed policy — those act on
/// errors, and a stall is not an error. Returns the guard the caller disarms
/// once the port is bound, plus the deadline for logging.
///
/// Nothing is logged here on purpose: the tracing subscriber is installed
/// inside `ComponentRegistry::build`, which has not run yet.
fn arm_startup_deadline() -> anyhow::Result<(recap_worker::startup::StartupGuard, Duration)> {
    let deadline = startup_deadline()?;
    let guard = recap_worker::startup::watch_startup(deadline, move || {
        error!(
            deadline_secs = deadline.as_secs(),
            "startup_deadline_exceeded: dependency initialization never finished; \
             exiting non-zero so the restart policy retries instead of leaving the \
             port unbound"
        );
        // Not a graceful return. Dropping the runtime waits indefinitely for a
        // blocking task to finish, and a stuck blocking task is precisely the
        // state this fires in. `exit` runs no destructors, so the line above has
        // to have been written already — the fmt layer writes synchronously.
        std::process::exit(1);
    });
    Ok((guard, deadline))
}

/// Spawn the background task that cancels `token` once a shutdown signal
/// arrives. Every long-running loop (HTTP listeners, batch/morning daemons)
/// observes the same token, so one signal coordinates every consumer.
fn spawn_shutdown_signal_task(token: CancellationToken) {
    tokio::spawn(async move {
        wait_for_shutdown_signal().await;
        token.cancel();
    });
}

/// Bind the mTLS listener when `MTLS_ENFORCE` asks for it.
///
/// Fail-closed: a TLS config that is requested but unloadable refuses startup
/// rather than serving the same router over plaintext only. The plaintext
/// listener stays up either way, so dev stacks without step-ca keep working.
fn spawn_mtls_listener(
    router: axum::Router,
    handle: axum_server::Handle<std::net::SocketAddr>,
) -> anyhow::Result<Option<tokio::task::JoinHandle<()>>> {
    let Some(server_config) = recap_worker::tls::load_server_tls_config().inspect_err(|e| {
        error!(error = %e, "failed to load mTLS config (fail-closed); refusing to start");
    })?
    else {
        info!("MTLS_ENFORCE!=true — mTLS listener disabled");
        return Ok(None);
    };

    let mtls_port = env::var("MTLS_PORT").unwrap_or_else(|_| "9443".to_string());
    let mtls_addr: std::net::SocketAddr = format!("0.0.0.0:{mtls_port}")
        .parse()
        .with_context(|| format!("parse mTLS bind addr for port {mtls_port}"))?;
    info!(%mtls_addr, "mTLS listener enabled");

    Ok(Some(tokio::spawn(async move {
        let tls_cfg = axum_server::tls_rustls::RustlsConfig::from_config(server_config);
        if let Err(e) = axum_server::bind_rustls(mtls_addr, tls_cfg)
            .handle(handle)
            .serve(router.into_make_service())
            .await
        {
            error!(error = %e, "mTLS server exited with error");
        }
    })))
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // Install rustls default crypto provider (required by rustls 0.23 when
    // multiple providers may be linked transitively, e.g. via reqwest +
    // axum-server). Ignore error if already installed by another path.
    let _ = rustls::crypto::aws_lc_rs::default_provider().install_default();

    let args: Vec<String> = env::args().collect();
    if let Some(code) = cli::try_healthcheck(&args) {
        std::process::exit(code);
    }
    if let Some(code) = cli::try_warmup(&args).await {
        std::process::exit(code);
    }

    // Tracing initialization is handled by Telemetry::new() inside
    // ComponentRegistry::build — install the panic hook only after that so
    // panics are actually visible in the tracing pipeline.
    let config = Config::from_env().context("failed to load configuration")?;
    let bind_addr = config.http_bind();

    let (startup, deadline) = arm_startup_deadline()?;

    let registry = ComponentRegistry::build(config.clone())
        .await
        .context("failed to build component registry")?;
    cli::install_panic_hook();
    info!(
        deadline_secs = deadline.as_secs(),
        "startup deadline armed; disarms once the listener is bound"
    );

    let scheduler = registry.scheduler().clone();
    let telemetry = registry.telemetry().clone();
    let default_genres = registry.config().recap_genres().to_vec();
    // Knowledge-loop owner for the persist-stage recap.topic_snapshotted.v1
    // emit. Resolved once from env; threaded into every JobContext both
    // daemons build. `None` only ever means RECAP_KNOWLEDGE_EMIT=false —
    // config validation refuses a half-wired emit at startup.
    let knowledge_owner = registry.config().knowledge_owner();

    // Coordinates graceful shutdown across every long-running consumer:
    // both HTTP listeners, the batch/morning daemons, and (via
    // `scheduler.shutdown()` below) the classification job queue's worker
    // tasks. One SIGTERM/SIGINT cancels all of them.
    let shutdown_token = CancellationToken::new();
    spawn_shutdown_signal_task(shutdown_token.clone());

    if default_genres.is_empty() {
        warn!("skipping automatic batch daemon because no default genres are configured");
    } else {
        let recap_window = registry.config().recap_3days_window_days();
        let _batch_daemon = spawn_jst_batch_daemon(
            scheduler.clone(),
            default_genres,
            recap_window,
            knowledge_owner,
            shutdown_token.clone(),
        );
    }
    // Morning Letter daemon: gated by MORNING_DAEMON_ENABLED env flag.
    // Default is "false" to preserve current behaviour; set to "true" to
    // re-enable the editorial projector tick.
    let morning_daemon_enabled = std::env::var("MORNING_DAEMON_ENABLED")
        .is_ok_and(|v| v.eq_ignore_ascii_case("true") || v == "1");
    if morning_daemon_enabled {
        info!("MORNING_DAEMON_ENABLED=true — starting morning editorial projector daemon");
        let _morning_daemon = recap_worker::scheduler::daemon::spawn_morning_update_daemon(
            scheduler.clone(),
            knowledge_owner,
            shutdown_token.clone(),
        );
    } else {
        info!("morning daemon disabled (set MORNING_DAEMON_ENABLED=true to enable)");
    }

    // Carries completed-recap notifications from recap-db's notification_outbox
    // to alt-data-hub. Without it the outbox accumulates rows nobody forwards,
    // so the handle is awaited at shutdown rather than dropped: an aborted
    // forward would leave a claimed row waiting out its whole lease.
    let notification_relay_task = registry.spawn_notification_relay(shutdown_token.clone());

    let pki = registry.pki_handle();
    let router = build_router(registry);

    // When MTLS_ENFORCE=true, bind the axum router to a rustls-backed
    // listener on :9443 (MTLS_PORT overrides) that requires a client cert
    // signed by the alt-CA. The existing plaintext listener stays up so
    // dev/test stacks without step-ca keep working.
    let mtls_handle = axum_server::Handle::new();
    let mtls_listener_task = spawn_mtls_listener(router.clone(), mtls_handle.clone())?;

    let listener = TcpListener::bind(bind_addr)
        .await
        .with_context(|| format!("failed to bind listener on {bind_addr}"))?;

    info!(%bind_addr, "listening");

    // The port is bound, so the service can serve: the deadline has done its job.
    startup.startup_complete();

    let plain_shutdown = shutdown_token.clone();
    if let Err(error) = axum::serve(listener, router)
        .with_graceful_shutdown(async move { plain_shutdown.cancelled().await })
        .await
    {
        warn!(error = %error, "server exited with error");
    }

    // The plain listener above only returns once `shutdown_token` fires (or
    // on a listener error). Either way, propagate cancellation to every
    // other consumer: the mTLS listener, the batch/morning daemons (already
    // observing the same token), and the classification queue workers.
    shutdown_token.cancel();
    mtls_handle.graceful_shutdown(Some(Duration::from_secs(10)));
    if let Some(task) = mtls_listener_task {
        let _ = task.await;
    }
    if let Some(task) = notification_relay_task {
        let _ = task.await;
    }

    scheduler.shutdown().await;
    if let Some(handle) = pki {
        handle.stop().await;
    }
    telemetry.shutdown();

    Ok(())
}
