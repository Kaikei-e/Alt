use anyhow::Context;
use tokio::net::TcpListener;
use tracing::{info, warn};

use recap_worker::{
    app::{ComponentRegistry, build_router},
    config::Config,
    observability,
};

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    if let Err(error) = observability::tracing::init() {
        eprintln!("failed to initialise tracing: {error:#}");
    }

    let config = Config::from_env().context("failed to load configuration")?;
    let bind_addr = config.http_bind();
    let registry =
        ComponentRegistry::build(config).context("failed to build component registry")?;
    let router = build_router(registry);

    let listener = TcpListener::bind(bind_addr)
        .await
        .with_context(|| format!("failed to bind listener on {bind_addr}"))?;

    info!(%bind_addr, "listening");

    if let Err(error) = axum::serve(listener, router).await {
        warn!(error = %error, "server exited with error");
    }

    Ok(())
}
