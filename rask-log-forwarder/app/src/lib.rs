#![deny(warnings, rust_2024_compatibility)]
// Specific pedantic lints enforced (not blanket allow):
#![deny(
    clippy::explicit_iter_loop,
    clippy::manual_let_else,
    clippy::semicolon_if_nothing_returned,
    clippy::inconsistent_struct_constructor
)]
// Noisy pedantic lints suppressed with justification:
#![allow(
    clippy::cast_lossless,            // Infallible casts are clear enough with `as`
    clippy::cast_possible_truncation, // Safe within realistic value bounds (durations, sizes)
    clippy::cast_possible_wrap,       // Safe in non-negative contexts
    clippy::cast_precision_loss,      // Acceptable for metrics/display
    clippy::cast_sign_loss,           // Safe where values are known non-negative
    clippy::missing_errors_doc,       // Internal API
    clippy::missing_panics_doc,       // Internal API
    clippy::module_name_repetitions,  // e.g. CollectorError in collector module
    clippy::must_use_candidate,       // Annotated selectively on critical APIs
    clippy::doc_markdown              // Internal API
)]

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

// Health check endpoint for Docker
pub fn health_check() -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    health_check_with(|k| std::env::var(k).ok())
}

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
