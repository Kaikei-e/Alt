// Regex error handling for memory-safe and compile-time validated patterns
use thiserror::Error;

#[derive(Error, Debug, Clone)]
pub enum RegexError {
    #[error("Regex compilation failed for pattern '{pattern}' (name: {name}): {source}")]
    CompilationFailed {
        pattern: String,
        name: String,
        #[source]
        source: regex::Error,
    },

    #[error("Regex index out of bounds: {index} >= {max}")]
    IndexOutOfBounds { index: usize, max: usize },

    #[error("Regex pattern not found: {name}")]
    PatternNotFound { name: String },

    #[error("Regex execution failed: {details}")]
    ExecutionFailed { details: String },
}

impl RegexError {
    /// Runtime fallback strategy for regex failures
    pub fn fallback_strategy(&self) -> FallbackStrategy {
        match self {
            RegexError::CompilationFailed { .. } => FallbackStrategy::UseSimpleParser,
            RegexError::IndexOutOfBounds { .. } => FallbackStrategy::UseDefaultPattern,
            RegexError::PatternNotFound { .. } => FallbackStrategy::UseDefaultPattern,
            RegexError::ExecutionFailed { .. } => FallbackStrategy::SkipEntry,
        }
    }
}

#[derive(Debug, Clone, PartialEq)]
pub enum FallbackStrategy {
    UseSimpleParser,
    UseDefaultPattern,
    SkipEntry,
}

#[cfg(test)]
mod tests {
    use super::*;
    use regex::Regex;

    #[test]
    fn test_compilation_failed_fallback_strategy() {
        let invalid_pattern = r#"[invalid regex pattern"#;
        let result = Regex::new(invalid_pattern);
        assert!(result.is_err());

        let error = RegexError::CompilationFailed {
            pattern: invalid_pattern.to_string(),
            name: "test_pattern".to_string(),
            source: result.unwrap_err(),
        };

        assert_eq!(error.fallback_strategy(), FallbackStrategy::UseSimpleParser);
    }

    #[test]
    fn test_index_out_of_bounds_fallback_strategy() {
        let error = RegexError::IndexOutOfBounds { index: 5, max: 3 };

        assert_eq!(
            error.fallback_strategy(),
            FallbackStrategy::UseDefaultPattern
        );
    }

    #[test]
    fn test_pattern_not_found_fallback_strategy() {
        let error = RegexError::PatternNotFound {
            name: "nonexistent_pattern".to_string(),
        };

        assert_eq!(
            error.fallback_strategy(),
            FallbackStrategy::UseDefaultPattern
        );
    }

    #[test]
    fn test_execution_failed_fallback_strategy() {
        let error = RegexError::ExecutionFailed {
            details: "regex timeout".to_string(),
        };

        assert_eq!(error.fallback_strategy(), FallbackStrategy::SkipEntry);
    }
}
