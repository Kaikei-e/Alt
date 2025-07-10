// TASK5 Phase 1: Comprehensive buffer error types for lock-free operations
use thiserror::Error;

#[derive(Error, Debug, Clone, PartialEq)]
pub enum BufferError {
    #[error("Buffer is closed")]
    BufferClosed,
    
    #[error("Buffer is full")]
    BufferFull,
    
    #[error("Invalid buffer capacity: {capacity}")]
    InvalidCapacity { capacity: usize },
    
    #[error("Buffer operation failed: {details}")]
    OperationFailed { details: String },
    
    #[error("Send timeout")]
    SendTimeout,
    
    #[error("Receive timeout")]
    ReceiveTimeout,
    
    #[error("Concurrency error: {0}")]
    ConcurrencyError(String),
    
    #[error("Buffer poisoned: {reason}")]
    BufferPoisoned { reason: String },
    
    #[error("Buffer initialization failed: {reason}")]
    InitializationFailed { reason: String },
    
    #[error("Buffer disconnected: {reason}")]
    Disconnected { reason: String },
    
    #[error("Channel configuration error: {reason}")]
    ChannelConfigError { reason: String },
    
    #[error("Memory allocation failed: {bytes_requested}")]
    MemoryAllocationFailed { bytes_requested: usize },
    
    #[error("Buffer overflow: attempted to write {attempted} bytes, capacity {capacity}")]
    BufferOverflow { attempted: usize, capacity: usize },
    
    #[error("Buffer underflow: attempted to read {attempted} bytes, available {available}")]
    BufferUnderflow { attempted: usize, available: usize },
}

#[derive(Error, Debug)]
pub enum MetricsError {
    #[error("Metric '{name}' already registered as {metric_type}")]
    AlreadyRegistered { name: String, metric_type: String },
    
    #[error("Invalid metric name '{name}': {reason}")]
    InvalidName { name: String, reason: String },
    
    #[error("Metric registration failed: {details}")]
    RegistrationFailed { details: String },
    
    #[error("Metric initialization failed: {reason}")]
    InitializationFailed { reason: String },
    
    #[error("Metric collection failed: {reason}")]
    CollectionFailed { reason: String },
    
    #[error("Metric update failed: {metric_name} - {reason}")]
    UpdateFailed { metric_name: String, reason: String },
    
    #[error("Metric export failed: {format} - {reason}")]
    ExportFailed { format: String, reason: String },
    
    #[error("Metric configuration error: {reason}")]
    ConfigurationError { reason: String },
    
    #[error("Metric validation failed: {metric_name} - {reason}")]
    ValidationFailed { metric_name: String, reason: String },
    
    #[error("Metric overflow: {metric_name} - value {value} exceeds max {max}")]
    Overflow { metric_name: String, value: u64, max: u64 },
    
    #[error("Metric underflow: {metric_name} - value {value} below min {min}")]
    Underflow { metric_name: String, value: u64, min: u64 },
    
    #[cfg(feature = "metrics")]
    #[error("Prometheus error: {0}")]
    PrometheusError(#[from] prometheus::Error),
    
    #[error("HTTP server error: {0}")]
    HttpError(String),
}

#[derive(Error, Debug, Clone, PartialEq)]
pub enum ParseError {
    #[error("Empty input")]
    EmptyInput,
    
    #[error("Input too long: '{input}' (max length: {max_len})")]
    TooLong { input: String, max_len: usize },
    
    #[error("Invalid character '{character}' at position {position} in '{input}'")]
    InvalidCharacter { input: String, position: usize, character: char },
    
    #[error("Overflow in '{input}' (max value: {max_value})")]
    Overflow { input: String, max_value: u64 },
    
    #[error("Underflow in '{input}' (min value: {min_value})")]
    Underflow { input: String, min_value: u64 },
    
    #[error("Invalid format: {reason}")]
    InvalidFormat { reason: String },
    
    #[error("Missing field: {field}")]
    MissingField { field: String },
    
    #[error("Type conversion error: {from_type} to {to_type} - {reason}")]
    ConversionError { from_type: String, to_type: String, reason: String },
    
    #[error("Parse timeout: {timeout_ms}ms")]
    ParseTimeout { timeout_ms: u64 },
    
    #[error("Parser initialization failed: {reason}")]
    InitializationFailed { reason: String },
}

// Error recovery and fallback mechanisms
#[derive(Debug, Clone, PartialEq)]
pub enum ErrorRecovery {
    /// Retry the operation with the same parameters
    Retry,
    /// Retry with modified parameters
    RetryWithFallback,
    /// Use a fallback value
    UseFallback,
    /// Skip the operation
    Skip,
    /// Fail permanently
    Fail,
}

impl BufferError {
    pub fn is_recoverable(&self) -> bool {
        match self {
            BufferError::BufferFull
            | BufferError::SendTimeout
            | BufferError::ReceiveTimeout
            | BufferError::ConcurrencyError(_) => true,
            BufferError::BufferClosed
            | BufferError::InvalidCapacity { .. }
            | BufferError::BufferPoisoned { .. }
            | BufferError::InitializationFailed { .. }
            | BufferError::Disconnected { .. }
            | BufferError::ChannelConfigError { .. }
            | BufferError::MemoryAllocationFailed { .. }
            | BufferError::BufferOverflow { .. }
            | BufferError::BufferUnderflow { .. }
            | BufferError::OperationFailed { .. } => false,
        }
    }
    
    pub fn recovery_strategy(&self) -> ErrorRecovery {
        match self {
            BufferError::BufferFull => ErrorRecovery::RetryWithFallback,
            BufferError::SendTimeout | BufferError::ReceiveTimeout => ErrorRecovery::Retry,
            BufferError::ConcurrencyError(_) => ErrorRecovery::Retry,
            BufferError::BufferClosed | BufferError::Disconnected { .. } => ErrorRecovery::Fail,
            BufferError::InvalidCapacity { .. } => ErrorRecovery::Fail,
            BufferError::BufferPoisoned { .. } => ErrorRecovery::Fail,
            BufferError::InitializationFailed { .. } => ErrorRecovery::Fail,
            BufferError::ChannelConfigError { .. } => ErrorRecovery::Fail,
            BufferError::MemoryAllocationFailed { .. } => ErrorRecovery::RetryWithFallback,
            BufferError::BufferOverflow { .. } => ErrorRecovery::UseFallback,
            BufferError::BufferUnderflow { .. } => ErrorRecovery::UseFallback,
            BufferError::OperationFailed { .. } => ErrorRecovery::Skip,
        }
    }
}

impl MetricsError {
    pub fn is_recoverable(&self) -> bool {
        match self {
            MetricsError::UpdateFailed { .. }
            | MetricsError::CollectionFailed { .. }
            | MetricsError::ExportFailed { .. } => true,
            MetricsError::AlreadyRegistered { .. }
            | MetricsError::InvalidName { .. }
            | MetricsError::RegistrationFailed { .. }
            | MetricsError::InitializationFailed { .. }
            | MetricsError::ConfigurationError { .. }
            | MetricsError::ValidationFailed { .. }
            | MetricsError::Overflow { .. }
            | MetricsError::Underflow { .. } => false,
            #[cfg(feature = "metrics")]
            MetricsError::PrometheusError(_) => false,
            MetricsError::HttpError(_) => true,
        }
    }
    
    pub fn recovery_strategy(&self) -> ErrorRecovery {
        match self {
            MetricsError::UpdateFailed { .. } => ErrorRecovery::Skip,
            MetricsError::CollectionFailed { .. } => ErrorRecovery::UseFallback,
            MetricsError::ExportFailed { .. } => ErrorRecovery::UseFallback,
            MetricsError::AlreadyRegistered { .. } => ErrorRecovery::UseFallback,
            MetricsError::InvalidName { .. } => ErrorRecovery::Skip,
            MetricsError::RegistrationFailed { .. } => ErrorRecovery::Skip,
            MetricsError::InitializationFailed { .. } => ErrorRecovery::Skip,
            MetricsError::ConfigurationError { .. } => ErrorRecovery::Fail,
            MetricsError::ValidationFailed { .. } => ErrorRecovery::Skip,
            MetricsError::Overflow { .. } => ErrorRecovery::UseFallback,
            MetricsError::Underflow { .. } => ErrorRecovery::UseFallback,
            #[cfg(feature = "metrics")]
            MetricsError::PrometheusError(_) => ErrorRecovery::Skip,
            MetricsError::HttpError(_) => ErrorRecovery::Retry,
        }
    }
}

impl ParseError {
    pub fn is_recoverable(&self) -> bool {
        match self {
            ParseError::EmptyInput => true,
            ParseError::TooLong { .. } => true,
            ParseError::InvalidCharacter { .. } => true,
            ParseError::Overflow { .. } => true,
            ParseError::Underflow { .. } => true,
            ParseError::InvalidFormat { .. } => true,
            ParseError::MissingField { .. } => true,
            ParseError::ConversionError { .. } => true,
            ParseError::ParseTimeout { .. } => true,
            ParseError::InitializationFailed { .. } => false,
        }
    }
    
    pub fn recovery_strategy(&self) -> ErrorRecovery {
        match self {
            ParseError::EmptyInput => ErrorRecovery::UseFallback,
            ParseError::TooLong { .. } => ErrorRecovery::UseFallback,
            ParseError::InvalidCharacter { .. } => ErrorRecovery::UseFallback,
            ParseError::Overflow { .. } => ErrorRecovery::UseFallback,
            ParseError::Underflow { .. } => ErrorRecovery::UseFallback,
            ParseError::InvalidFormat { .. } => ErrorRecovery::UseFallback,
            ParseError::MissingField { .. } => ErrorRecovery::UseFallback,
            ParseError::ConversionError { .. } => ErrorRecovery::UseFallback,
            ParseError::ParseTimeout { .. } => ErrorRecovery::Retry,
            ParseError::InitializationFailed { .. } => ErrorRecovery::Fail,
        }
    }
}

// Helper functions for safe error handling
pub fn safe_buffer_operation<T, F>(mut operation: F) -> Result<T, BufferError>
where
    F: FnMut() -> Result<T, BufferError>,
{
    match operation() {
        Ok(result) => Ok(result),
        Err(error) => {
            match error.recovery_strategy() {
                ErrorRecovery::Retry => {
                    // Try once more
                    operation()
                }
                ErrorRecovery::RetryWithFallback => {
                    // Return the original error - caller should handle fallback
                    Err(error)
                }
                ErrorRecovery::UseFallback => {
                    // Return the original error - caller should use fallback
                    Err(error)
                }
                ErrorRecovery::Skip => {
                    // Return the original error - caller should skip
                    Err(error)
                }
                ErrorRecovery::Fail => {
                    // Propagate the error
                    Err(error)
                }
            }
        }
    }
}

pub fn safe_metrics_operation<T, F>(mut operation: F) -> Result<T, MetricsError>
where
    F: FnMut() -> Result<T, MetricsError>,
{
    match operation() {
        Ok(result) => Ok(result),
        Err(error) => {
            match error.recovery_strategy() {
                ErrorRecovery::Retry => {
                    // Try once more
                    operation()
                }
                ErrorRecovery::RetryWithFallback => {
                    // Return the original error - caller should handle fallback
                    Err(error)
                }
                ErrorRecovery::UseFallback => {
                    // Return the original error - caller should use fallback
                    Err(error)
                }
                ErrorRecovery::Skip => {
                    // Return the original error - caller should skip
                    Err(error)
                }
                ErrorRecovery::Fail => {
                    // Propagate the error
                    Err(error)
                }
            }
        }
    }
}

pub fn safe_parse_operation<T, F>(mut operation: F) -> Result<T, ParseError>
where
    F: FnMut() -> Result<T, ParseError>,
{
    match operation() {
        Ok(result) => Ok(result),
        Err(error) => {
            match error.recovery_strategy() {
                ErrorRecovery::Retry => {
                    // Try once more
                    operation()
                }
                ErrorRecovery::RetryWithFallback => {
                    // Return the original error - caller should handle fallback
                    Err(error)
                }
                ErrorRecovery::UseFallback => {
                    // Return the original error - caller should use fallback
                    Err(error)
                }
                ErrorRecovery::Skip => {
                    // Return the original error - caller should skip
                    Err(error)
                }
                ErrorRecovery::Fail => {
                    // Propagate the error
                    Err(error)
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_buffer_error_recovery_strategies() {
        assert_eq!(BufferError::BufferFull.recovery_strategy(), ErrorRecovery::RetryWithFallback);
        assert!(BufferError::BufferFull.is_recoverable());
        
        assert_eq!(BufferError::BufferClosed.recovery_strategy(), ErrorRecovery::Fail);
        assert!(!BufferError::BufferClosed.is_recoverable());
        
        assert_eq!(BufferError::SendTimeout.recovery_strategy(), ErrorRecovery::Retry);
        assert!(BufferError::SendTimeout.is_recoverable());
    }

    #[test]
    fn test_metrics_error_recovery_strategies() {
        assert_eq!(MetricsError::UpdateFailed { 
            metric_name: "test".to_string(), 
            reason: "test".to_string() 
        }.recovery_strategy(), ErrorRecovery::Skip);
        
        assert_eq!(MetricsError::AlreadyRegistered { 
            name: "test".to_string(), 
            metric_type: "counter".to_string() 
        }.recovery_strategy(), ErrorRecovery::UseFallback);
        
        assert!(!MetricsError::InvalidName { 
            name: "".to_string(), 
            reason: "empty".to_string() 
        }.is_recoverable());
    }

    #[test]
    fn test_parse_error_recovery_strategies() {
        assert_eq!(ParseError::EmptyInput.recovery_strategy(), ErrorRecovery::UseFallback);
        assert!(ParseError::EmptyInput.is_recoverable());
        
        assert_eq!(ParseError::Overflow { 
            input: "999999".to_string(), 
            max_value: 65535 
        }.recovery_strategy(), ErrorRecovery::UseFallback);
        
        assert!(!ParseError::InitializationFailed { 
            reason: "system failure".to_string() 
        }.is_recoverable());
    }

    #[test]
    fn test_safe_buffer_operation() {
        // Test successful operation
        let result: Result<i32, BufferError> = safe_buffer_operation(|| Ok(42));
        assert_eq!(result, Ok(42));
        
        // Test failing operation
        let result: Result<i32, BufferError> = safe_buffer_operation(|| Err(BufferError::BufferClosed));
        assert_eq!(result, Err(BufferError::BufferClosed));
    }

    #[test]
    fn test_safe_metrics_operation() {
        // Test successful operation
        let result: Result<&str, MetricsError> = safe_metrics_operation(|| Ok("success"));
        assert!(result.is_ok());
        if let Ok(value) = result {
            assert_eq!(value, "success");
        }
        
        // Test failing operation
        let result: Result<&str, MetricsError> = safe_metrics_operation(|| Err(MetricsError::InvalidName { 
            name: "".to_string(), 
            reason: "empty".to_string() 
        }));
        assert!(result.is_err());
    }

    #[test]
    fn test_safe_parse_operation() {
        // Test successful operation
        let result: Result<u16, ParseError> = safe_parse_operation(|| Ok(123u16));
        assert_eq!(result, Ok(123));
        
        // Test failing operation
        let result: Result<u16, ParseError> = safe_parse_operation(|| Err(ParseError::EmptyInput));
        assert_eq!(result, Err(ParseError::EmptyInput));
    }
}