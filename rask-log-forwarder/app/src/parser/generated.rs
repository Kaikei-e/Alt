// Include build-time validated regex patterns
include!(concat!(env!("OUT_DIR"), "/validated_regexes.rs"));

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_generated_patterns_exist() {
        // Verify that patterns were generated
        assert!(VALIDATED_PATTERNS.len() > 0);
        
        // Test that we can access patterns
        let pattern_names = VALIDATED_PATTERNS.pattern_names();
        assert!(pattern_names.contains(&"docker_native_timestamp"));
        assert!(pattern_names.contains(&"nginx_access_fallback"));
        assert!(pattern_names.contains(&"postgres_log"));
    }

    #[test]
    fn test_pattern_index_constants() {
        // Test that index constants are generated and valid
        assert_eq!(get_pattern_name(pattern_index::DOCKER_NATIVE_TIMESTAMP), Some("docker_native_timestamp"));
        assert_eq!(get_pattern_name(pattern_index::ISO_TIMESTAMP_FALLBACK), Some("iso_timestamp_fallback"));
        assert_eq!(get_pattern_name(pattern_index::NGINX_ACCESS_FALLBACK), Some("nginx_access_fallback"));
        assert_eq!(get_pattern_name(pattern_index::POSTGRES_LOG), Some("postgres_log"));
    }

    #[test]
    fn test_pattern_compilation() {
        // Test that all generated patterns compile successfully
        for i in 0..VALIDATED_PATTERNS.len() {
            let result = VALIDATED_PATTERNS.get(i);
            assert!(result.is_ok(), "Pattern at index {} should compile successfully", i);
        }
    }

    #[test]
    fn test_pattern_by_name_access() {
        // Test accessing patterns by name
        assert!(VALIDATED_PATTERNS.get_by_name("docker_native_timestamp").is_ok());
        assert!(VALIDATED_PATTERNS.get_by_name("nginx_access_fallback").is_ok());
        assert!(VALIDATED_PATTERNS.get_by_name("postgres_log").is_ok());
        
        // Test non-existent pattern
        assert!(VALIDATED_PATTERNS.get_by_name("nonexistent_pattern").is_err());
    }
}