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

/// Spawn the background task that cancels `token` once a shutdown signal
/// arrives. Every long-running loop (HTTP listeners, batch/morning daemons)
/// observes the same token, so one signal coordinates every consumer.
fn spawn_shutdown_signal_task(token: CancellationToken) {
    tokio::spawn(async move {
        wait_for_shutdown_signal().await;
        token.cancel();
    });
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
    let registry = ComponentRegistry::build(config.clone())
        .await
        .context("failed to build component registry")?;
    cli::install_panic_hook();

    let scheduler = registry.scheduler().clone();
    let telemetry = registry.telemetry().clone();
    let default_genres = registry.config().recap_genres().to_vec();
    // Knowledge-loop owner for the persist-stage recap.topic_snapshotted.v1
    // emit. Resolved once from env; threaded into every JobContext both
    // daemons build. `None` keeps emission off (intentionally disabled).
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
    let router = build_router(registry);

    // When MTLS_ENFORCE=true, bind the axum router to a rustls-backed
    // listener on :9443 (MTLS_PORT overrides) that requires a client cert
    // signed by the alt-CA. The existing plaintext listener stays up so
    // dev/test stacks without step-ca keep working.
    let mtls_handle = axum_server::Handle::new();
    let mtls_listener_task = match recap_worker::tls::load_server_tls_config() {
        Ok(Some(server_config)) => {
            let mtls_port = std::env::var("MTLS_PORT").unwrap_or_else(|_| "9443".to_string());
            let mtls_addr: std::net::SocketAddr = format!("0.0.0.0:{mtls_port}")
                .parse()
                .with_context(|| format!("parse mTLS bind addr for port {mtls_port}"))?;
            let mtls_router = router.clone();
            let handle = mtls_handle.clone();
            info!(%mtls_addr, "mTLS listener enabled");
            Some(tokio::spawn(async move {
                let tls_cfg = axum_server::tls_rustls::RustlsConfig::from_config(server_config);
                if let Err(e) = axum_server::bind_rustls(mtls_addr, tls_cfg)
                    .handle(handle)
                    .serve(mtls_router.into_make_service())
                    .await
                {
                    error!(error = %e, "mTLS server exited with error");
                }
            }))
        }
        Ok(None) => {
            info!("MTLS_ENFORCE!=true — mTLS listener disabled");
            None
        }
        Err(e) => {
            error!(error = %e, "failed to load mTLS config (fail-closed); refusing to start");
            return Err(e);
        }
    };

    let listener = TcpListener::bind(bind_addr)
        .await
        .with_context(|| format!("failed to bind listener on {bind_addr}"))?;

    info!(%bind_addr, "listening");

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

    scheduler.shutdown().await;
    telemetry.shutdown();

    Ok(())
}
