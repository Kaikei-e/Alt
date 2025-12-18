use std::str::FromStr;
use thiserror::Error;

#[derive(Error, Debug)]
pub enum InitializationError {
    #[error("Invalid log level '{input}'. Valid levels: {valid_levels:?}")]
    InvalidLogLevel {
        input: String,
        valid_levels: Vec<String>,
    },

    #[error("Invalid directive format '{input}'. Expected: '{expected}'")]
    InvalidDirectiveFormat { input: String, expected: String },

    #[error("Empty target in directive '{input}'")]
    EmptyTarget { input: String },

    #[error("Logging system initialization failed: {details}")]
    LoggingInitFailed {
        details: String,
        #[source]
        source: Box<dyn std::error::Error + Send + Sync>,
    },

    #[error("Configuration validation failed: {reason}")]
    ConfigValidationFailed { reason: String },

    #[error("Resource initialization failed: {resource}")]
    ResourceInitFailed {
        resource: String,
        #[source]
        source: Box<dyn std::error::Error + Send + Sync>,
    },
}

impl InitializationError {
    /// 回復可能性の判定
    pub fn is_recoverable(&self) -> bool {
        match self {
            InitializationError::InvalidLogLevel { .. } => true,
            InitializationError::InvalidDirectiveFormat { .. } => true,
            InitializationError::EmptyTarget { .. } => true,
            InitializationError::LoggingInitFailed { .. } => false,
            InitializationError::ConfigValidationFailed { .. } => true,
            InitializationError::ResourceInitFailed { .. } => false,
        }
    }

    /// フォールバック戦略
    pub fn fallback_strategy(&self) -> FallbackStrategy {
        match self {
            InitializationError::InvalidLogLevel { .. } => FallbackStrategy::UseDefaultLevel,
            InitializationError::InvalidDirectiveFormat { .. } => FallbackStrategy::SkipDirective,
            InitializationError::EmptyTarget { .. } => FallbackStrategy::SkipDirective,
            InitializationError::LoggingInitFailed { .. } => FallbackStrategy::UseStdoutLogging,
            InitializationError::ConfigValidationFailed { .. } => FallbackStrategy::UseDefaults,
            InitializationError::ResourceInitFailed { .. } => FallbackStrategy::AbortStartup,
        }
    }
}

#[derive(Debug, Clone)]
pub enum FallbackStrategy {
    UseDefaultLevel,
    SkipDirective,
    UseStdoutLogging,
    UseDefaults,
    AbortStartup,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum LogLevel {
    Error,
    Warn,
    Info,
    Debug,
    Trace,
}

impl FromStr for LogLevel {
    type Err = InitializationError;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s.to_lowercase().as_str() {
            "error" => Ok(LogLevel::Error),
            "warn" | "warning" => Ok(LogLevel::Warn),
            "info" => Ok(LogLevel::Info),
            "debug" => Ok(LogLevel::Debug),
            "trace" => Ok(LogLevel::Trace),
            _ => Err(InitializationError::InvalidLogLevel {
                input: s.to_string(),
                valid_levels: vec![
                    "error".to_string(),
                    "warn".to_string(),
                    "info".to_string(),
                    "debug".to_string(),
                    "trace".to_string(),
                ],
            }),
        }
    }
}

impl LogLevel {
    pub fn as_str(&self) -> &'static str {
        match self {
            LogLevel::Error => "error",
            LogLevel::Warn => "warn",
            LogLevel::Info => "info",
            LogLevel::Debug => "debug",
            LogLevel::Trace => "trace",
        }
    }
}

impl From<LogLevel> for tracing::Level {
    fn from(val: LogLevel) -> Self {
        match val {
            LogLevel::Error => tracing::Level::ERROR,
            LogLevel::Warn => tracing::Level::WARN,
            LogLevel::Info => tracing::Level::INFO,
            LogLevel::Debug => tracing::Level::DEBUG,
            LogLevel::Trace => tracing::Level::TRACE,
        }
    }
}

#[derive(Debug, Clone)]
pub struct LogDirective {
    pub target: String,
    pub level: LogLevel,
}

impl LogDirective {
    pub fn new(target: impl Into<String>, level: LogLevel) -> Self {
        Self {
            target: target.into(),
            level,
        }
    }

    /// メモリセーフなパース（unwrap一切なし）
    pub fn parse(directive: &str) -> Result<Self, InitializationError> {
        let parts: Vec<&str> = directive.split('=').collect();

        if parts.len() != 2 {
            return Err(InitializationError::InvalidDirectiveFormat {
                input: directive.to_string(),
                expected: "target=level".to_string(),
            });
        }

        let target = parts[0].trim();
        let level = parts[1].trim();

        if target.is_empty() {
            return Err(InitializationError::EmptyTarget {
                input: directive.to_string(),
            });
        }

        let parsed_level = LogLevel::from_str(level)?;

        Ok(LogDirective::new(target, parsed_level))
    }

    /// tracing_subscriber::EnvFilter用の文字列変換
    pub fn to_filter_string(&self) -> String {
        format!("{}={}", self.target, self.level.as_str())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_log_level_from_str() {
        assert_eq!(LogLevel::from_str("error").unwrap(), LogLevel::Error);
        assert_eq!(LogLevel::from_str("warn").unwrap(), LogLevel::Warn);
        assert_eq!(LogLevel::from_str("warning").unwrap(), LogLevel::Warn);
        assert_eq!(LogLevel::from_str("info").unwrap(), LogLevel::Info);
        assert_eq!(LogLevel::from_str("debug").unwrap(), LogLevel::Debug);
        assert_eq!(LogLevel::from_str("trace").unwrap(), LogLevel::Trace);

        // Case insensitive
        assert_eq!(LogLevel::from_str("ERROR").unwrap(), LogLevel::Error);
        assert_eq!(LogLevel::from_str("WARN").unwrap(), LogLevel::Warn);

        // Invalid level
        assert!(LogLevel::from_str("invalid").is_err());
    }

    #[test]
    fn test_log_level_as_str() {
        assert_eq!(LogLevel::Error.as_str(), "error");
        assert_eq!(LogLevel::Warn.as_str(), "warn");
        assert_eq!(LogLevel::Info.as_str(), "info");
        assert_eq!(LogLevel::Debug.as_str(), "debug");
        assert_eq!(LogLevel::Trace.as_str(), "trace");
    }

    #[test]
    fn test_log_directive_parsing_valid_cases() {
        let valid_cases = [
            ("hyper=warn", LogLevel::Warn),
            ("reqwest=error", LogLevel::Error),
            ("h2=debug", LogLevel::Debug),
            ("tower=info", LogLevel::Info),
            ("tonic=trace", LogLevel::Trace),
        ];

        for (input, expected_level) in valid_cases {
            let result = LogDirective::parse(input);
            assert!(result.is_ok(), "Should parse successfully: {input}");

            let directive = result.unwrap();
            let expected_target = input.split('=').next().unwrap();
            assert_eq!(directive.target, expected_target);
            assert_eq!(directive.level, expected_level);
        }
    }

    #[test]
    fn test_log_directive_parsing_invalid_cases() {
        let invalid_cases = [
            ("", "empty string"),
            ("hyper", "missing level"),
            ("=warn", "empty target"),
            ("hyper=", "empty level"),
            ("hyper=invalid", "invalid level"),
            ("hyper=warn=extra", "too many parts"),
            ("  =warn", "empty target with spaces"),
            ("hyper=  ", "empty level with spaces"),
        ];

        for (input, description) in invalid_cases {
            let result = LogDirective::parse(input);
            assert!(result.is_err(), "Should fail for {description}: {input}");
        }
    }

    #[test]
    fn test_log_directive_to_filter_string() {
        let directive = LogDirective::new("hyper", LogLevel::Warn);
        assert_eq!(directive.to_filter_string(), "hyper=warn");

        let directive = LogDirective::new("reqwest", LogLevel::Error);
        assert_eq!(directive.to_filter_string(), "reqwest=error");
    }

    #[test]
    fn test_initialization_error_is_recoverable() {
        let recoverable_errors = [
            InitializationError::InvalidLogLevel {
                input: "test".to_string(),
                valid_levels: vec!["info".to_string()],
            },
            InitializationError::InvalidDirectiveFormat {
                input: "test".to_string(),
                expected: "target=level".to_string(),
            },
            InitializationError::EmptyTarget {
                input: "test".to_string(),
            },
            InitializationError::ConfigValidationFailed {
                reason: "test".to_string(),
            },
        ];

        for error in recoverable_errors {
            assert!(error.is_recoverable(), "Should be recoverable: {error:?}");
        }

        let non_recoverable_errors = [
            InitializationError::LoggingInitFailed {
                details: "test".to_string(),
                source: Box::new(std::io::Error::other("test")),
            },
            InitializationError::ResourceInitFailed {
                resource: "test".to_string(),
                source: Box::new(std::io::Error::other("test")),
            },
        ];

        for error in non_recoverable_errors {
            assert!(
                !error.is_recoverable(),
                "Should not be recoverable: {error:?}"
            );
        }
    }

    #[test]
    fn test_fallback_strategy() {
        let error = InitializationError::InvalidLogLevel {
            input: "test".to_string(),
            valid_levels: vec!["info".to_string()],
        };
        assert!(matches!(
            error.fallback_strategy(),
            FallbackStrategy::UseDefaultLevel
        ));

        let error = InitializationError::InvalidDirectiveFormat {
            input: "test".to_string(),
            expected: "target=level".to_string(),
        };
        assert!(matches!(
            error.fallback_strategy(),
            FallbackStrategy::SkipDirective
        ));

        let error = InitializationError::LoggingInitFailed {
            details: "test".to_string(),
            source: Box::new(std::io::Error::other("test")),
        };
        assert!(matches!(
            error.fallback_strategy(),
            FallbackStrategy::UseStdoutLogging
        ));
    }
}
