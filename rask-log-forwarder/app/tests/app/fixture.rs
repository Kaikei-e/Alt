use std::path::PathBuf;
use tempfile::TempDir;

/// Complete list of environment variables read by `rask-log-forwarder`.
pub const TRACKED_ENV_VARS: &[&str] = &[
    "RASK_CONFIG",
    "RASK_INGEST_TOKEN_FILE",
    "TARGET_SERVICE",
    "RASK_ENDPOINT",
    "BATCH_SIZE",
    "LOG_LEVEL",
    "ENABLE_DISK_FALLBACK",
    "ENABLE_METRICS",
    "ENABLE_COMPRESSION",
    "FLUSH_INTERVAL_MS",
    "BUFFER_CAPACITY",
    "CONNECTION_TIMEOUT_SECS",
    "MAX_CONNECTIONS",
    "MAX_DISK_USAGE_MB",
    "METRICS_PORT",
    "DISK_FALLBACK_PATH",
    "CONFIG_FILE",
    "PROTOCOL",
    "OTLP_ENDPOINT",
    "HOSTNAME",
    "NETWORK_MODE",
    "RASK_GROUP",
    "COMPOSE_SERVICE",
];

fn build_isolated_env<'a>(
    overrides: &[(&'a str, Option<&'a str>)],
) -> Vec<(&'a str, Option<&'a str>)> {
    let mut vars: Vec<(&'a str, Option<&'a str>)> =
        TRACKED_ENV_VARS.iter().map(|&k| (k, None)).collect();
    for &(k, v) in overrides {
        if let Some(slot) = vars.iter_mut().find(|(existing, _)| *existing == k) {
            slot.1 = v;
        } else {
            vars.push((k, v));
        }
    }
    vars
}

/// Runs `f` with every tracked environment variable cleared unless explicitly
/// specified in `overrides`, restoring the original process environment on exit.
pub fn with_isolated_env<F, R>(overrides: &[(&str, Option<&str>)], f: F) -> R
where
    F: FnOnce() -> R,
{
    temp_env::with_vars(build_isolated_env(overrides), f)
}

/// Async variant of [`with_isolated_env`] for `#[tokio::test]` bodies.
pub async fn async_with_isolated_env<Fut>(
    overrides: &[(&str, Option<&str>)],
    fut: Fut,
) -> Fut::Output
where
    Fut: std::future::Future,
{
    temp_env::async_with_vars(build_isolated_env(overrides), fut).await
}

/// Creates a temporary bearer token file inside a new, isolated `TempDir`.
///
/// Returns the path to the token file and the owning `TempDir`.
/// Callers MUST bind the returned `TempDir` to a variable (e.g. `_dir`)
/// so that the directory and file remain valid until the `App` or `Config`
/// constructor has finished reading it. On drop, `tempfile` removes the
/// isolated folder without deleting any user files.
///
/// # Errors
///
/// Returns an [`std::io::Error`] if creating the temporary directory or
/// writing the token file fails.
pub fn create_token_in_temp_dir(token_content: &str) -> Result<(PathBuf, TempDir), std::io::Error> {
    let dir = TempDir::new()?;
    let path = dir.path().join("ingest_token");
    std::fs::write(&path, token_content)?;
    Ok((path, dir))
}
