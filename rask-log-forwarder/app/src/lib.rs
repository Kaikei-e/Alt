#![deny(warnings, rust_2024_compatibility)]
// Specific pedantic lints enforced (not blanket allow):
#![deny(
    clippy::explicit_iter_loop,
    clippy::manual_let_else,
    clippy::semicolon_if_nothing_returned,
    clippy::inconsistent_struct_constructor
)]
// Noisy pedantic lints suppressed with justification:

pub mod app;
pub mod buffer;
pub mod collector;
pub mod domain;
pub mod parser;
pub mod reliability;
pub mod sender;

// Re-export main types for easy access
pub use app::{App, Config};

// Version information
pub const VERSION: &str = env!("CARGO_PKG_VERSION");

/// Runs the Docker health check by validating the configuration loaded from the
/// process environment.
///
/// # Errors
///
/// Returns an error if loading the configuration from the environment or
/// validating the loaded configuration fails.
pub fn health_check() -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    health_check_with(|k| std::env::var(k).ok())
}

/// Runs the Docker health check using the supplied environment lookup closure.
///
/// # Errors
///
/// Returns an error if loading the configuration from `env` or validating the
/// loaded configuration fails.
pub fn health_check_with<F>(env: F) -> Result<(), Box<dyn std::error::Error + Send + Sync>>
where
    F: Fn(&str) -> Option<String>,
{
    // Validate that the configuration from environment is well-formed.
    let config = Config::from_env_with(env)?;
    config.validate()?;
    Ok(())
}

#[cfg(test)]
mod health_check_tests {
    use std::io::Write;
    use tempfile::NamedTempFile;

    #[test]
    fn health_check_validates_default_config() {
        let mut token_file = NamedTempFile::new().unwrap();
        writeln!(token_file, "valid-token-123").unwrap();
        let path_str = token_file.path().to_string_lossy().into_owned();
        let res =
            super::health_check_with(|k| (k == "RASK_INGEST_TOKEN_FILE").then(|| path_str.clone()));
        res.expect("config with valid token must pass health check");
    }
}
