// TASK4: Complete memory-safe application initialization system
use super::initialization::InitializationError;
use super::logging_system::setup_logging_safe;
use super::config::{Config, LogLevel};

#[derive(Debug)]
pub struct InitializationResult {
    pub logging_initialized: bool,
    pub config_validated: bool,
    pub initialization_time_ms: u64,
}

pub struct ApplicationInitializer;

impl ApplicationInitializer {
    pub fn new() -> Self {
        Self
    }
    
    /// 段階的初期化（失敗時の詳細情報提供）
    pub fn initialize(&self, config: &Config) -> Result<InitializationResult, InitializationError> {
        let start_time = std::time::Instant::now();
        
        // Phase 1: ログシステムの初期化
        self.initialize_logging(config.log_level)
            .map_err(|e| {
                eprintln!("Failed to initialize logging: {}", e);
                e
            })?;
        
        // Phase 2: 設定検証
        self.validate_configuration(config)
            .map_err(|e| {
                tracing::error!("Configuration validation failed: {}", e);
                e
            })?;
        
        let elapsed = start_time.elapsed();
        tracing::info!("Application initialization completed in {:?}", elapsed);
        
        Ok(InitializationResult {
            logging_initialized: true,
            config_validated: true,
            initialization_time_ms: elapsed.as_millis() as u64,
        })
    }
    
    fn initialize_logging(&self, log_level: LogLevel) -> Result<(), InitializationError> {
        // Use the memory-safe logging setup
        setup_logging_safe(log_level)?;
        
        tracing::info!("Logging system initialized successfully with level: {:?}", log_level);
        Ok(())
    }
    
    pub fn validate_configuration(&self, config: &Config) -> Result<(), InitializationError> {
        // Validate endpoint
        if config.endpoint.is_empty() {
            return Err(InitializationError::ConfigValidationFailed {
                reason: "Endpoint cannot be empty".to_string(),
            });
        }
        
        if !config.endpoint.starts_with("http://") && !config.endpoint.starts_with("https://") {
            return Err(InitializationError::ConfigValidationFailed {
                reason: format!("Invalid endpoint URL: {}", config.endpoint),
            });
        }
        
        // Validate batch size
        if config.batch_size == 0 {
            return Err(InitializationError::ConfigValidationFailed {
                reason: "Batch size must be greater than 0".to_string(),
            });
        }
        
        if config.batch_size > 100_000 {
            return Err(InitializationError::ConfigValidationFailed {
                reason: format!("Batch size too large: {} (max: 100,000)", config.batch_size),
            });
        }
        
        // Validate buffer capacity
        if config.buffer_capacity == 0 {
            return Err(InitializationError::ConfigValidationFailed {
                reason: "Buffer capacity must be greater than 0".to_string(),
            });
        }
        
        // Validate flush interval
        if config.flush_interval_ms == 0 {
            return Err(InitializationError::ConfigValidationFailed {
                reason: "Flush interval must be greater than 0".to_string(),
            });
        }
        
        // Validate connection timeout
        if config.connection_timeout_secs == 0 {
            return Err(InitializationError::ConfigValidationFailed {
                reason: "Connection timeout must be greater than 0".to_string(),
            });
        }
        
        // Validate metrics port if metrics are enabled
        if config.enable_metrics && config.metrics_port == 0 {
            return Err(InitializationError::ConfigValidationFailed {
                reason: format!("Invalid metrics port: {} (must be > 0)", config.metrics_port),
            });
        }
        
        // Validate disk fallback settings
        if config.enable_disk_fallback {
            if config.max_disk_usage_mb == 0 {
                return Err(InitializationError::ConfigValidationFailed {
                    reason: "Max disk usage must be greater than 0 when disk fallback is enabled".to_string(),
                });
            }
            
            if !config.disk_fallback_path.exists() {
                // Try to create the directory
                if let Err(e) = std::fs::create_dir_all(&config.disk_fallback_path) {
                    return Err(InitializationError::ConfigValidationFailed {
                        reason: format!("Cannot create disk fallback directory: {}", e),
                    });
                }
            }
        }
        
        tracing::info!("Configuration validation passed");
        Ok(())
    }
    
    /// 設定に基づく初期化戦略の決定
    pub fn determine_initialization_strategy(&self, config: &Config) -> InitializationStrategy {
        if config.target_service.is_none() {
            InitializationStrategy::AutoDetection
        } else if config.enable_disk_fallback && config.enable_metrics {
            InitializationStrategy::FullFeatures
        } else if config.enable_metrics {
            InitializationStrategy::WithMetrics
        } else {
            InitializationStrategy::Basic
        }
    }
}

impl Default for ApplicationInitializer {
    fn default() -> Self {
        Self::new()
    }
}

#[derive(Debug, Clone, PartialEq)]
pub enum InitializationStrategy {
    Basic,
    WithMetrics,
    FullFeatures,
    AutoDetection,
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;
    use std::time::Duration;

    fn create_valid_config() -> Config {
        Config {
            target_service: Some("test".to_string()),
            endpoint: "http://localhost:9600/v1/aggregate".to_string(),
            batch_size: 1000,
            flush_interval_ms: 500,
            buffer_capacity: 100000,
            log_level: LogLevel::Info,
            enable_metrics: false,
            metrics_port: 9090,
            enable_disk_fallback: false,
            disk_fallback_path: PathBuf::from("/tmp/test"),
            max_disk_usage_mb: 1000,
            connection_timeout_secs: 30,
            max_connections: 10,
            config_file: None,
            enable_compression: false,
            // Derived fields
            flush_interval: Duration::from_millis(500),
            connection_timeout: Duration::from_secs(30),
            retry_config: Default::default(),
            disk_fallback_config: Default::default(),
            metrics_config: Default::default(),
        }
    }

    #[test]
    fn test_application_initializer_creation() {
        let initializer = ApplicationInitializer::new();
        let config = create_valid_config();
        
        let strategy = initializer.determine_initialization_strategy(&config);
        assert_eq!(strategy, InitializationStrategy::Basic);
    }

    #[test]
    fn test_initialization_strategy_determination() {
        let initializer = ApplicationInitializer::new();
        
        // Basic strategy
        let config = create_valid_config();
        assert_eq!(
            initializer.determine_initialization_strategy(&config),
            InitializationStrategy::Basic
        );
        
        // Auto-detection strategy
        let mut config = create_valid_config();
        config.target_service = None;
        assert_eq!(
            initializer.determine_initialization_strategy(&config),
            InitializationStrategy::AutoDetection
        );
        
        // With metrics strategy
        let mut config = create_valid_config();
        config.enable_metrics = true;
        assert_eq!(
            initializer.determine_initialization_strategy(&config),
            InitializationStrategy::WithMetrics
        );
        
        // Full features strategy
        let mut config = create_valid_config();
        config.enable_metrics = true;
        config.enable_disk_fallback = true;
        assert_eq!(
            initializer.determine_initialization_strategy(&config),
            InitializationStrategy::FullFeatures
        );
    }

    #[test]
    fn test_config_validation_valid_config() {
        let initializer = ApplicationInitializer::new();
        let config = create_valid_config();
        
        let result = initializer.validate_configuration(&config);
        assert!(result.is_ok(), "Valid config should pass validation");
    }

    #[test]
    fn test_config_validation_invalid_endpoint() {
        let initializer = ApplicationInitializer::new();
        
        // Empty endpoint
        let mut config = create_valid_config();
        config.endpoint = "".to_string();
        assert!(initializer.validate_configuration(&config).is_err());
        
        // Invalid URL scheme
        let mut config = create_valid_config();
        config.endpoint = "ftp://invalid.com".to_string();
        assert!(initializer.validate_configuration(&config).is_err());
    }

    #[test]
    fn test_config_validation_invalid_batch_size() {
        let initializer = ApplicationInitializer::new();
        
        // Zero batch size
        let mut config = create_valid_config();
        config.batch_size = 0;
        assert!(initializer.validate_configuration(&config).is_err());
        
        // Too large batch size
        let mut config = create_valid_config();
        config.batch_size = 200_000;
        assert!(initializer.validate_configuration(&config).is_err());
    }

    #[test]
    fn test_config_validation_invalid_buffer_capacity() {
        let initializer = ApplicationInitializer::new();
        
        let mut config = create_valid_config();
        config.buffer_capacity = 0;
        assert!(initializer.validate_configuration(&config).is_err());
    }

    #[test]
    fn test_config_validation_invalid_timing_params() {
        let initializer = ApplicationInitializer::new();
        
        // Zero flush interval
        let mut config = create_valid_config();
        config.flush_interval_ms = 0;
        assert!(initializer.validate_configuration(&config).is_err());
        
        // Zero connection timeout
        let mut config = create_valid_config();
        config.connection_timeout_secs = 0;
        assert!(initializer.validate_configuration(&config).is_err());
    }

    #[test]
    fn test_config_validation_invalid_metrics_port() {
        let initializer = ApplicationInitializer::new();
        
        let mut config = create_valid_config();
        config.enable_metrics = true;
        config.metrics_port = 0; // Invalid port (must be > 0)
        assert!(initializer.validate_configuration(&config).is_err());
    }

    #[test]
    fn test_config_validation_disk_fallback() {
        let initializer = ApplicationInitializer::new();
        
        // Invalid disk usage
        let mut config = create_valid_config();
        config.enable_disk_fallback = true;
        config.max_disk_usage_mb = 0;
        assert!(initializer.validate_configuration(&config).is_err());
    }

    #[test]
    fn test_initialization_with_valid_config() {
        let initializer = ApplicationInitializer::new();
        let config = create_valid_config();
        
        // Initialize with valid config
        let result = initializer.initialize(&config);
        
        // The result depends on whether tracing is already initialized
        match result {
            Ok(init_result) => {
                assert!(init_result.logging_initialized);
                assert!(init_result.config_validated);
                assert!(init_result.initialization_time_ms > 0);
            }
            Err(InitializationError::LoggingInitFailed { .. }) => {
                // Expected when tracing is already initialized in other tests
            }
            Err(e) => {
                panic!("Unexpected error: {:?}", e);
            }
        }
    }

    #[test]
    fn test_initialization_with_invalid_config() {
        let initializer = ApplicationInitializer::new();
        
        let mut config = create_valid_config();
        config.endpoint = "".to_string(); // Invalid
        
        let result = initializer.initialize(&config);
        assert!(result.is_err(), "Should fail with invalid config");
        
        if let Err(InitializationError::ConfigValidationFailed { reason }) = result {
            assert!(reason.contains("Endpoint cannot be empty"));
        } else {
            panic!("Expected ConfigValidationFailed error");
        }
    }
}