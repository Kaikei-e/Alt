#![warn(rust_2024_compatibility)]
// Pedantic lints allowed at crate level with justification:
// - must_use_candidate: Most internal functions don't require must_use; critical APIs annotated selectively
// - missing_errors_doc: Internal APIs don't need exhaustive error documentation
// - cast_precision_loss: Acceptable for metrics/display where exact precision isn't critical
// - unused_async: Some async functions don't await internally but maintain async API for consistency
// - unused_self: Some methods keep &self for API consistency or future extensibility
// - match_same_arms: Sometimes intentional for code clarity and future differentiation
// - doc_markdown: Internal APIs don't need strict markdown formatting
// - module_name_repetitions: Type names may repeat module names for clarity
#![allow(
    clippy::must_use_candidate,
    clippy::missing_errors_doc,
    clippy::cast_precision_loss,
    clippy::unused_async,
    clippy::unused_self,
    clippy::match_same_arms,
    clippy::doc_markdown,
    clippy::module_name_repetitions
)]

pub mod app;
pub mod buffer;
pub mod collector;
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
