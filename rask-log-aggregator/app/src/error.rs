use thiserror::Error;

#[derive(Error, Debug)]
pub enum AggregatorError {
    #[error("Failed to load configuration: {0}")]
    Config(String),

    #[error("Failed to bind to address {address}: {source}")]
    Bind {
        address: String,
        #[source]
        source: std::io::Error,
    },

    #[error("Server error: {0}")]
    Server(#[from] std::io::Error),
}
