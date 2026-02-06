//! Domain layer for rask-log-forwarder.
//!
//! Contains the canonical types shared across all modules:
//! - `EnrichedLogEntry`: The pipeline's core data type
//! - `LogLevel`: Domain log severity (Debug/Info/Warn/Error/Fatal)
//! - `ForwarderError`: Top-level error type

pub mod error;
pub mod log_entry;
pub mod log_level;

pub use error::ForwarderError;
pub use log_entry::EnrichedLogEntry;
pub use log_level::LogLevel;
