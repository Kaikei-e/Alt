use std::collections::HashMap;
use std::path::{Path, PathBuf};
use tempfile::TempDir;

/// Isolated in-memory environment map for configuration tests (replaces mutating
/// process-wide `std::env`, which is `unsafe` in Rust 2024).
pub struct EnvGuard {
    vars: HashMap<String, String>,
}

impl EnvGuard {
    /// Creates an empty `EnvGuard`.
    pub fn new() -> Self {
        Self {
            vars: HashMap::new(),
        }
    }

    /// Sets an environment variable in this isolated map.
    pub fn set_var<K: Into<String>, V: AsRef<Path>>(&mut self, key: K, val: V) {
        self.vars
            .insert(key.into(), val.as_ref().to_string_lossy().into_owned());
    }

    /// Removes an environment variable from this isolated map.
    pub fn remove_var(&mut self, key: &str) {
        self.vars.remove(key);
    }

    /// Looks up a variable in this isolated map.
    pub fn get(&self, key: &str) -> Option<&str> {
        self.vars.get(key).map(String::as_str)
    }

    /// Returns a lookup closure suitable for `*_with` configuration entry points.
    pub fn lookup(&self) -> impl Fn(&str) -> Option<String> + '_ {
        move |k| self.vars.get(k).cloned()
    }
}

/// Creates a temporary bearer token file inside a new, isolated `TempDir`.
///
/// Returns the path to the token file and the owning `TempDir`.
/// Callers MUST bind the returned `TempDir` to a variable (e.g. `_dir`)
/// so that the directory and file remain valid until the `App` or `Config`
/// constructor has finished reading it. On drop, `tempfile` removes the
/// isolated folder without deleting any user files.
pub fn create_token_in_temp_dir(token_content: &str) -> Result<(PathBuf, TempDir), std::io::Error> {
    let dir = TempDir::new()?;
    let path = dir.path().join("ingest_token");
    std::fs::write(&path, token_content)?;
    Ok((path, dir))
}
