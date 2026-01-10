use anyhow::Context;
use std::env;
use std::time::Duration;
use tokio::net::TcpListener;
use tracing::{error, info, warn};

use recap_worker::{
    app::{ComponentRegistry, build_router},
    config::Config,
    scheduler::daemon::spawn_jst_batch_daemon,
};

/// Perform a health check against the local HTTP server.
/// Returns exit code 0 on success, 1 on failure.
fn run_healthcheck() -> i32 {
    let port = env::var("PORT").unwrap_or_else(|_| "9005".to_string());
    let url = format!("http://127.0.0.1:{}/health/live", port);

    let client = reqwest::blocking::Client::builder()
        .timeout(Duration::from_secs(5))
        .build();

    let client = match client {
        Ok(c) => c,
        Err(e) => {
            eprintln!("healthcheck failed: failed to create client: {}", e);
            return 1;
        }
    };

    match client.get(&url).send() {
        Ok(resp) if resp.status().is_success() => 0,
        Ok(resp) => {
            eprintln!("healthcheck failed: status {}", resp.status());
            1
        }
        Err(e) => {
            eprintln!("healthcheck failed: {}", e);
            1
        }
    }
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // Handle healthcheck subcommand
    let args: Vec<String> = env::args().collect();
    if args.len() > 1 && args[1] == "healthcheck" {
        std::process::exit(run_healthcheck());
    }
    std::panic::set_hook(Box::new(|panic_info| {
        let thread = std::thread::current();
        let thread_name = thread.name().unwrap_or("unnamed");
        let message = panic_info
            .payload()
            .downcast_ref::<&str>()
            .copied()
            .or_else(|| {
                panic_info
                    .payload()
                    .downcast_ref::<String>()
                    .map(|s| s.as_str())
            })
            .unwrap_or("unknown panic payload");

        if let Some(location) = panic_info.location() {
            error!(
                thread = thread_name,
                file = location.file(),
                line = location.line(),
                column = location.column(),
                message,
                "panic occurred"
            );
        } else {
            error!(
                thread = thread_name,
                message, "panic occurred without location information"
            );
        }
    }));

    // Tracing initialization is handled by Telemetry::new()
    let config = Config::from_env().context("failed to load configuration")?;
    let bind_addr = config.http_bind();
    let registry = ComponentRegistry::build(config.clone())
        .await
        .context("failed to build component registry")?;
    let scheduler = registry.scheduler().clone();
    let default_genres = registry.config().recap_genres().to_vec();

    if default_genres.is_empty() {
        warn!("skipping automatic batch daemon because no default genres are configured");
    } else {
        let _batch_daemon = spawn_jst_batch_daemon(scheduler.clone(), default_genres);
    }
    // Morning Letter機能を一時停止
    // let _morning_daemon = recap_worker::scheduler::daemon::spawn_morning_update_daemon(scheduler);
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
