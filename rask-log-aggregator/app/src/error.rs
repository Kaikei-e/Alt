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

    #[error("ClickHouse error: {0}")]
    ClickHouse(#[from] clickhouse::error::Error),

    #[error("Failed to decode protobuf message: {0}")]
    ProtoDecode(#[from] prost::DecodeError),

    #[error("Export error: {0}")]
    #[allow(dead_code)]
    Export(String),
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_config_error_display() {
        let err = AggregatorError::Config("missing env var".into());
        assert!(err.to_string().contains("configuration"));
        assert!(err.to_string().contains("missing env var"));
    }

    #[test]
    fn test_clickhouse_error_display() {
        let err = AggregatorError::ClickHouse(clickhouse::error::Error::Custom(
            "connection failed".to_string(),
        ));
        assert!(err.to_string().contains("ClickHouse"));
        assert!(err.to_string().contains("connection failed"));
    }

    #[test]
    fn test_proto_decode_error_display() {
        #[allow(deprecated)]
        let decode_err = prost::DecodeError::new("invalid wire type");
        let err = AggregatorError::ProtoDecode(decode_err);
        assert!(err.to_string().contains("protobuf"));
        assert!(err.to_string().contains("invalid wire type"));
    }

    #[test]
    fn test_export_error_display() {
        let err = AggregatorError::Export("batch failed".into());
        assert!(err.to_string().contains("Export"));
        assert!(err.to_string().contains("batch failed"));
    }

    #[test]
    fn test_error_implements_std_error() {
        fn assert_error<E: std::error::Error>() {}
        assert_error::<AggregatorError>();
    }

    #[test]
    fn test_proto_decode_error_from_string() {
        // Test that ProtoDecode variant works correctly
        #[allow(deprecated)]
        let decode_err = prost::DecodeError::new("test decode error");
        let err = AggregatorError::ProtoDecode(decode_err);
        assert!(matches!(err, AggregatorError::ProtoDecode(_)));
        assert!(err.to_string().contains("decode"));
    }
}
