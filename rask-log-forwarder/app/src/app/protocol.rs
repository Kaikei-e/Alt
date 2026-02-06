#[cfg(feature = "otlp")]
use super::config::Protocol;

/// Sender configuration for protocol-aware log transmission.
#[derive(Clone)]
pub struct SenderConfig {
    #[cfg(feature = "otlp")]
    pub protocol: Protocol,
    #[cfg(feature = "otlp")]
    pub otlp_endpoint: String,
}
