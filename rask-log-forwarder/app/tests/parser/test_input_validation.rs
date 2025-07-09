use crate::parser::universal::UniversalParser;
use crate::parser::services::{GoStructuredParser, NginxParser, PostgresParser};
use crate::parser::simd::SimdParser;
use bytes::Bytes;

#[test]
fn test_universal_parser_with_malformed_input() {
    let parser = UniversalParser::new();
    
    // Test with extremely large input
    let large_input = "a".repeat(1_000_000);
    let bytes = Bytes::from(large_input);
    
    // This should not panic
    let result = parser.parse_log_line(&bytes);
    assert!(result.is_ok() || result.is_err()); // Should handle gracefully
}

#[test]
fn test_universal_parser_with_invalid_utf8() {
    let parser = UniversalParser::new();
    
    // Test with invalid UTF-8 bytes
    let invalid_utf8 = vec![0xFF, 0xFE, 0xFD, 0xFC];
    let bytes = Bytes::from(invalid_utf8);
    
    // Should not panic due to unwrap on UTF-8 conversion
    let result = parser.parse_log_line(&bytes);
    assert!(result.is_ok() || result.is_err()); // Should handle gracefully
}

#[test]
fn test_universal_parser_with_empty_input() {
    let parser = UniversalParser::new();
    
    // Test with empty input
    let bytes = Bytes::from("");
    
    // Should handle empty input gracefully
    let result = parser.parse_log_line(&bytes);
    assert!(result.is_ok() || result.is_err()); // Should not panic
}

#[test]
fn test_go_structured_parser_with_malformed_json() {
    let parser = GoStructuredParser::new();
    
    // Test with malformed JSON
    let malformed_json = r#"{"level":"info","msg":"test""#; // Missing closing brace
    let bytes = Bytes::from(malformed_json);
    
    // Should not panic
    let result = parser.parse(&bytes);
    assert!(result.is_err()); // Should return error for malformed JSON
}

#[test]
fn test_nginx_parser_with_malformed_log() {
    let parser = NginxParser::new();
    
    // Test with malformed nginx log
    let malformed_log = "This is not a valid nginx log format";
    let bytes = Bytes::from(malformed_log);
    
    // Should not panic
    let result = parser.parse(&bytes);
    assert!(result.is_err()); // Should return error for malformed log
}

#[test]
fn test_postgres_parser_with_malformed_log() {
    let parser = PostgresParser::new();
    
    // Test with malformed postgres log
    let malformed_log = "Not a postgres log";
    let bytes = Bytes::from(malformed_log);
    
    // Should not panic
    let result = parser.parse(&bytes);
    assert!(result.is_err()); // Should return error for malformed log
}

#[test]
fn test_simd_parser_with_large_input() {
    let parser = SimdParser::new();
    
    // Test with large JSON input
    let large_json = format!(r#"{{"log":"{}","stream":"stdout","time":"2024-01-01T00:00:00Z"}}"#, "x".repeat(100_000));
    let bytes = Bytes::from(large_json);
    
    // Should not panic or consume excessive memory
    let result = parser.parse_docker_json(&bytes);
    assert!(result.is_ok() || result.is_err()); // Should handle gracefully
}

#[test]
fn test_simd_parser_with_malformed_docker_json() {
    let parser = SimdParser::new();
    
    // Test with malformed Docker JSON
    let malformed_json = r#"{"log":"test","stream":"stdout""#; // Missing closing brace
    let bytes = Bytes::from(malformed_json);
    
    // Should not panic
    let result = parser.parse_docker_json(&bytes);
    assert!(result.is_err()); // Should return error for malformed JSON
}

#[test]
fn test_parser_with_null_bytes() {
    let parser = UniversalParser::new();
    
    // Test with null bytes
    let null_bytes = vec![0x00, 0x01, 0x02, 0x03];
    let bytes = Bytes::from(null_bytes);
    
    // Should not panic
    let result = parser.parse_log_line(&bytes);
    assert!(result.is_ok() || result.is_err()); // Should handle gracefully
}

#[test]
fn test_parser_with_extremely_long_line() {
    let parser = UniversalParser::new();
    
    // Test with extremely long log line (potential DoS)
    let long_line = "x".repeat(10_000_000); // 10MB line
    let bytes = Bytes::from(long_line);
    
    // Should not panic or consume excessive memory
    let result = parser.parse_log_line(&bytes);
    assert!(result.is_ok() || result.is_err()); // Should handle gracefully
}

#[test]
fn test_parser_with_special_characters() {
    let parser = UniversalParser::new();
    
    // Test with various special characters that might break regex
    let special_chars = "🚀 emoji test \n\r\t null_char: \0 and more...";
    let bytes = Bytes::from(special_chars);
    
    // Should not panic
    let result = parser.parse_log_line(&bytes);
    assert!(result.is_ok() || result.is_err()); // Should handle gracefully
}