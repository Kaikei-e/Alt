#![warn(rust_2018_idioms)]

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
