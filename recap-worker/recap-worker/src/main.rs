use anyhow::Context;
use tokio::net::TcpListener;
use tracing::{info, warn};

use recap_worker::{
    app::{ComponentRegistry, build_router},
    config::Config,
    scheduler::daemon::spawn_jst_batch_daemon,
};

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // Tracing initialization is handled by Telemetry::new()
    let config = Config::from_env().context("failed to load configuration")?;
    let bind_addr = config.http_bind();
    let registry =
        ComponentRegistry::build(config.clone()).context("failed to build component registry")?;
    let scheduler = registry.scheduler().clone();
    let default_genres = registry.config().recap_genres().to_vec();

    if default_genres.is_empty() {
        warn!("skipping automatic batch daemon because no default genres are configured");
    } else {
        let _batch_daemon = spawn_jst_batch_daemon(scheduler, default_genres);
    }
    let router = build_router(registry);

    let listener = TcpListener::bind(bind_addr)
        .await
        .with_context(|| format!("failed to bind listener on {bind_addr}"))?;

    info!(%bind_addr, "listening");

    if let Err(error) = axum::serve(listener, router).await {
        warn!(error = %error, "server exited with error");
    }

    // シャットダウン時にトレースをフラッシュ（将来実装）
    // observability::shutdown_tracing();

    Ok(())
}
