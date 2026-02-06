use thiserror::Error;

/// Top-level error type for the forwarder pipeline.
#[derive(Error, Debug)]
pub enum ForwarderError {
    #[error("Configuration error: {0}")]
    Config(String),

    #[error("Collection error: {0}")]
    Collection(String),

    #[error("Parse error: {0}")]
    Parse(String),

    #[error("Buffer error: {0}")]
    Buffer(String),

    #[error("Transmission error: {0}")]
    Transmission(String),

    #[error("Shutdown error: {0}")]
    Shutdown(String),
}
