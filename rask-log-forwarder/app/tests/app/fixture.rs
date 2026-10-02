use std::env;
use std::ffi::OsString;
use std::path::PathBuf;
use tempfile::TempDir;

/// Complete list of environment variables read by rask-log-forwarder.
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
];

/// RAII guard that snapshots the process environment on construction,
/// clears tracked RASK variables for deterministic test isolation,
/// and faithfully restores the original environment when dropped.
/// This guarantees that tests neither leak environment mutations to
/// other tests nor permanently erase the caller's pre-existing environment.
pub struct EnvGuard {
    saved: Vec<(&'static str, Option<OsString>)>,
}

impl EnvGuard {
    /// Creates a new `EnvGuard`, capturing current values and clearing all tracked variables.
    pub fn new() -> Self {
        let mut saved = Vec::with_capacity(TRACKED_ENV_VARS.len());
        for &var in TRACKED_ENV_VARS {
            saved.push((var, env::var_os(var)));
            unsafe {
                env::remove_var(var);
            }
        }
        Self { saved }
    }

    /// Sets an environment variable within this guarded scope.
    pub fn set_var<K: AsRef<std::ffi::OsStr>, V: AsRef<std::ffi::OsStr>>(&self, key: K, val: V) {
        unsafe {
            env::set_var(key, val);
        }
    }

    /// Removes an environment variable within this guarded scope.
    pub fn remove_var<K: AsRef<std::ffi::OsStr>>(&self, key: K) {
        unsafe {
            env::remove_var(key);
        }
    }
}

impl Drop for EnvGuard {
    fn drop(&mut self) {
        for (var, original) in &self.saved {
            unsafe {
                match original {
                    Some(val) => env::set_var(var, val),
                    None => env::remove_var(var),
                }
            }
        }
    }
}

/// Helper function to clean all environment variables immediately.
/// Included for backward compatibility with legacy tests that call `clean_all_env_vars()`.
#[allow(dead_code)]
pub fn clean_all_env_vars() {
    unsafe {
        for &var in TRACKED_ENV_VARS {
            env::remove_var(var);
        }
    }
}

/// Creates a temporary bearer token file inside a new, isolated `TempDir`.
///
/// Returns the path to the token file and the owning `TempDir`.
/// Callers MUST bind the returned `TempDir` to a variable (e.g. `_dir`)
/// so that the directory and file remain valid until the `App` or `Config`
/// constructor has finished reading it. On drop, `tempfile` removes the
/// isolated folder without deleting any user files.
pub fn create_token_in_temp_dir(token_content: &str) -> (PathBuf, TempDir) {
    let dir = TempDir::new().expect("Failed to create temporary directory for test token");
    let path = dir.path().join("ingest_token");
    std::fs::write(&path, token_content).expect("Failed to write temporary token file");
    (path, dir)
}
