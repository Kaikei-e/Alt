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
pub async fn health_check() -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    // Basic health check - verify we can create a minimal config
    let _config = Config::default();
    Ok(())
}
