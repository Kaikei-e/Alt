// TASK4: Memory-safe logging system implementation
use super::config::LogLevel as ConfigLogLevel;
use super::initialization::{InitializationError, LogDirective, LogLevel, FallbackStrategy};
use std::sync::Arc;
use parking_lot::RwLock;
use tracing_subscriber::{EnvFilter, fmt, prelude::*};

pub struct LoggingSystem {
    directives: Arc<RwLock<Vec<LogDirective>>>,
    fallback_level: LogLevel,
}

impl LoggingSystem {
    pub fn new() -> Self {
        Self {
            directives: Arc::new(RwLock::new(Vec::new())),
            fallback_level: LogLevel::Info,
        }
    }
    
    /// 完全メモリセーフなディレクティブ追加
    pub fn add_directive(&self, directive_str: &str) -> Result<(), InitializationError> {
        match LogDirective::parse(directive_str) {
            Ok(directive) => {
                let mut directives = self.directives.write();
                directives.push(directive);
                Ok(())
            }
            Err(e) => {
                // エラーログ出力（fallback戦略適用）
                match e.fallback_strategy() {
                    FallbackStrategy::UseDefaultLevel => {
                        eprintln!("Warning: {}, using default level", e);
                        self.add_default_directive(directive_str)
                    }
                    FallbackStrategy::SkipDirective => {
                        eprintln!("Warning: {}, skipping directive", e);
                        Ok(())
                    }
                    _ => Err(e),
                }
            }
        }
    }
    
    /// デフォルトレベルでのディレクティブ追加
    fn add_default_directive(&self, directive_str: &str) -> Result<(), InitializationError> {
        let target = directive_str.split('=').next().unwrap_or("unknown");
        let directive = LogDirective::new(target, self.fallback_level);
        
        let mut directives = self.directives.write();
        directives.push(directive);
        Ok(())
    }
    
    /// 型安全なバッチ追加
    pub fn add_default_directives(&self) -> Result<(), InitializationError> {
        let default_directives = &[
            ("hyper", LogLevel::Warn),
            ("reqwest", LogLevel::Warn),
            ("h2", LogLevel::Warn),
            ("tower", LogLevel::Warn),
            ("tonic", LogLevel::Warn),
        ];
        
        for (target, level) in default_directives {
            let directive = LogDirective::new(*target, *level);
            let mut directives = self.directives.write();
            directives.push(directive);
        }
        
        Ok(())
    }
    
    /// メモリセーフなtracing初期化
    pub fn initialize_tracing(&self, default_level: LogLevel) -> Result<(), InitializationError> {
        let filter_string = self.build_filter_string(default_level);
        
        let env_filter = EnvFilter::try_new(&filter_string)
            .map_err(|e| InitializationError::LoggingInitFailed {
                details: format!("Failed to create EnvFilter with '{}'", filter_string),
                source: Box::new(e),
            })?;
        
        let subscriber = tracing_subscriber::registry()
            .with(env_filter)
            .with(
                fmt::layer()
                    .with_target(true)
                    .with_thread_ids(true)
                    .with_file(true)
                    .with_line_number(true)
                    .with_level(true)
                    .with_ansi(true)
                    .compact()
            );
        
        tracing::subscriber::set_global_default(subscriber)
            .map_err(|e| InitializationError::LoggingInitFailed {
                details: "Failed to set global tracing subscriber".to_string(),
                source: Box::new(e),
            })?;
        
        Ok(())
    }
    
    /// フィルタ文字列の構築
    pub fn build_filter_string(&self, default_level: LogLevel) -> String {
        let directives = self.directives.read();
        
        if directives.is_empty() {
            return default_level.as_str().to_string();
        }
        
        let mut filter_parts = Vec::with_capacity(directives.len() + 1);
        
        // Default level first
        filter_parts.push(default_level.as_str().to_string());
        
        for directive in directives.iter() {
            filter_parts.push(directive.to_filter_string());
        }
        
        filter_parts.join(",")
    }

    /// 現在のディレクティブ数を取得（テスト用）
    pub fn directive_count(&self) -> usize {
        self.directives.read().len()
    }

    /// ディレクティブをクリア（テスト用）
    pub fn clear_directives(&self) {
        self.directives.write().clear();
    }
}

impl Default for LoggingSystem {
    fn default() -> Self {
        Self::new()
    }
}

// Conversion from config LogLevel to initialization LogLevel
impl From<ConfigLogLevel> for LogLevel {
    fn from(config_level: ConfigLogLevel) -> Self {
        match config_level {
            ConfigLogLevel::Error => LogLevel::Error,
            ConfigLogLevel::Warn => LogLevel::Warn,
            ConfigLogLevel::Info => LogLevel::Info,
            ConfigLogLevel::Debug => LogLevel::Debug,
            ConfigLogLevel::Trace => LogLevel::Trace,
        }
    }
}

// Safe logging setup using the memory-safe logging system
pub fn setup_logging_safe(config_level: ConfigLogLevel) -> Result<(), InitializationError> {
    use std::sync::{Mutex, Once};
    
    static INIT: Once = Once::new();
    static INIT_SUCCESS: Mutex<bool> = Mutex::new(false);
    
    INIT.call_once(|| {
        let logging_system = LoggingSystem::new();
        
        let result: Result<(), InitializationError> = (|| {
            // Add default directives (this replaces the expect() calls)
            logging_system.add_default_directives()?;
            
            // Convert config level to initialization level
            let init_level: LogLevel = config_level.into();
            
            // Initialize tracing with the safe system
            logging_system.initialize_tracing(init_level)?;
            
            Ok(())
        })();
        
        if result.is_ok() {
            *INIT_SUCCESS.lock().unwrap() = true;
        }
    });
    
    // Return the initialization result
    if *INIT_SUCCESS.lock().unwrap() {
        Ok(())
    } else {
        Err(InitializationError::LoggingInitFailed {
            details: "Logging system initialization failed".to_string(),
            source: Box::new(std::io::Error::other("Logging initialization error")),
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Arc;
    use std::thread;

    #[test]
    fn test_logging_system_creation() {
        let logging_system = LoggingSystem::new();
        assert_eq!(logging_system.directive_count(), 0);
    }

    #[test]
    fn test_add_valid_directive() {
        let logging_system = LoggingSystem::new();
        
        assert!(logging_system.add_directive("hyper=warn").is_ok());
        assert_eq!(logging_system.directive_count(), 1);
        
        assert!(logging_system.add_directive("reqwest=error").is_ok());
        assert_eq!(logging_system.directive_count(), 2);
    }

    #[test]
    fn test_add_invalid_directive_with_fallback() {
        let logging_system = LoggingSystem::new();
        
        // Invalid format - should skip
        assert!(logging_system.add_directive("invalid").is_ok());
        assert_eq!(logging_system.directive_count(), 0); // Skipped
        
        // Invalid level - should use default
        assert!(logging_system.add_directive("target=invalid_level").is_ok());
        assert_eq!(logging_system.directive_count(), 1); // Added with default level
    }

    #[test]
    fn test_add_default_directives() {
        let logging_system = LoggingSystem::new();
        
        assert!(logging_system.add_default_directives().is_ok());
        assert_eq!(logging_system.directive_count(), 5); // hyper, reqwest, h2, tower, tonic
    }

    #[test]
    fn test_build_filter_string() {
        let logging_system = LoggingSystem::new();
        
        // Empty directives
        let filter = logging_system.build_filter_string(LogLevel::Info);
        assert_eq!(filter, "info");
        
        // With directives
        logging_system.add_directive("hyper=warn").unwrap();
        logging_system.add_directive("reqwest=error").unwrap();
        
        let filter = logging_system.build_filter_string(LogLevel::Debug);
        assert!(filter.contains("debug"));
        assert!(filter.contains("hyper=warn"));
        assert!(filter.contains("reqwest=error"));
    }

    #[test]
    fn test_concurrent_directive_modification() {
        let logging_system = Arc::new(LoggingSystem::new());
        
        // 並行書き込み
        let handles: Vec<_> = (0..100)
            .map(|i| {
                let logging_system = logging_system.clone();
                thread::spawn(move || {
                    let directive = format!("target{}=info", i);
                    logging_system.add_directive(&directive)
                })
            })
            .collect();
        
        // 並行読み取り
        let read_handles: Vec<_> = (0..50)
            .map(|_| {
                let logging_system = logging_system.clone();
                thread::spawn(move || {
                    logging_system.build_filter_string(LogLevel::Info)
                })
            })
            .collect();
        
        // 全てのスレッドが正常終了
        for handle in handles {
            assert!(handle.join().is_ok());
        }
        
        for handle in read_handles {
            assert!(handle.join().is_ok());
        }
        
        // All 100 directives should be added
        assert_eq!(logging_system.directive_count(), 100);
    }

    #[test]
    fn test_memory_safety_with_heavy_load() {
        let logging_system = Arc::new(LoggingSystem::new());
        
        // Heavy concurrent load
        let handles: Vec<_> = (0..1000)
            .map(|i| {
                let logging_system = logging_system.clone();
                thread::spawn(move || {
                    // Mix of valid and invalid directives
                    let directives = vec![
                        format!("valid{}=info", i),
                        format!("invalid{}", i), // Invalid format
                        format!("bad{}=invalid_level", i), // Invalid level
                        format!("=empty_target{}", i), // Empty target
                    ];
                    
                    for directive in directives {
                        let _ = logging_system.add_directive(&directive);
                    }
                    
                    // Also read frequently
                    let _ = logging_system.build_filter_string(LogLevel::Info);
                    let _ = logging_system.directive_count();
                })
            })
            .collect();
        
        // All threads should complete without panic
        for handle in handles {
            assert!(handle.join().is_ok());
        }
        
        // Should have some valid directives (exact count depends on timing)
        let count = logging_system.directive_count();
        assert!(count > 0, "Should have some directives added");
        assert!(count <= 2000, "Should not exceed maximum possible (valid + fallback)");
    }

    #[test]
    fn test_fallback_strategies() {
        let logging_system = LoggingSystem::new();
        
        let test_cases = vec![
            ("hyper=warn", true, 1), // Valid
            ("invalid_format", true, 1), // Skip directive
            ("target=invalid_level", true, 2), // Use default level
            ("=empty", true, 2), // Skip directive
            ("", true, 2), // Skip directive
        ];
        
        for (directive, should_succeed, expected_count) in test_cases {
            let result = logging_system.add_directive(directive);
            assert_eq!(result.is_ok(), should_succeed, "Directive: {}", directive);
            assert_eq!(logging_system.directive_count(), expected_count, "Directive: {}", directive);
        }
    }

    #[test] 
    fn test_clear_directives() {
        let logging_system = LoggingSystem::new();
        
        logging_system.add_directive("hyper=warn").unwrap();
        logging_system.add_directive("reqwest=error").unwrap();
        assert_eq!(logging_system.directive_count(), 2);
        
        logging_system.clear_directives();
        assert_eq!(logging_system.directive_count(), 0);
    }

    #[test]
    fn test_config_log_level_conversion() {
        assert_eq!(LogLevel::from(ConfigLogLevel::Error), LogLevel::Error);
        assert_eq!(LogLevel::from(ConfigLogLevel::Warn), LogLevel::Warn);
        assert_eq!(LogLevel::from(ConfigLogLevel::Info), LogLevel::Info);
        assert_eq!(LogLevel::from(ConfigLogLevel::Debug), LogLevel::Debug);
        assert_eq!(LogLevel::from(ConfigLogLevel::Trace), LogLevel::Trace);
    }

    #[test]
    fn test_setup_logging_safe() {
        // Test that setup_logging_safe works without panics
        // Note: We can't easily test tracing initialization in unit tests
        // but we can test that the function doesn't panic or return errors
        // for invalid configuration scenarios
        
        // This should work fine
        let result = setup_logging_safe(ConfigLogLevel::Info);
        // The function might fail due to tracing already being initialized 
        // in other tests, but it shouldn't panic
        match result {
            Ok(()) => {
                // Success case
            }
            Err(InitializationError::LoggingInitFailed { .. }) => {
                // Expected when tracing is already initialized
            }
            Err(e) => {
                panic!("Unexpected error type: {:?}", e);
            }
        }
    }
}