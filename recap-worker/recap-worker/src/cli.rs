//! Thin CLI helpers extracted from `main` (healthcheck / warmup / panic hook).

use std::env;
use std::time::Duration;

use tracing::error;

/// Returns `Some(exit_code)` when `args` requests the healthcheck subcommand.
pub(crate) fn try_healthcheck(args: &[String]) -> Option<i32> {
    if args.get(1).map(String::as_str) == Some("healthcheck") {
        Some(run_healthcheck())
    } else {
        None
    }
}

/// Returns `Some(exit_code)` when `args` requests the warmup subcommand.
pub(crate) async fn try_warmup(args: &[String]) -> Option<i32> {
    if args.get(1).map(String::as_str) != Some("warmup") {
        return None;
    }
    // warmup subcommand: populate the rust-bert AllMiniLmL12V2 model cache
    // so the runtime container can boot in a network-isolated stack.
    Some(match recap_worker::warmup_embedding_cache().await {
        Ok(()) => {
            eprintln!("warmup: rust-bert AllMiniLmL12V2 cache populated");
            0
        }
        Err(e) => {
            eprintln!("warmup failed: {e:?}");
            1
        }
    })
}

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

/// Install a panic hook that routes panics through `tracing`.
///
/// Must be called after tracing has been initialized (e.g. after
/// `ComponentRegistry::build`), otherwise early panics are silently lost.
pub(crate) fn install_panic_hook() {
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
}
