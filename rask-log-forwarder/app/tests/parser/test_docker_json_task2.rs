use bytes::Bytes;
use rask_log_forwarder::parser::{DockerJsonParser, ParseError};

#[test]
fn test_docker_json_parsing() {
    let parser = DockerJsonParser::new();
    
    let json_log = r#"{"log":"Hello nginx\n","stream":"stdout","time":"2024-01-01T12:00:00.123456789Z"}"#;
    let bytes = Bytes::from(json_log);
    
    let entry = parser.parse(bytes).unwrap();
    
    assert_eq!(entry.log, "Hello nginx\n");
    assert_eq!(entry.stream, "stdout");
    assert_eq!(entry.time, "2024-01-01T12:00:00.123456789Z");
}

#[test]
fn test_docker_json_stderr_parsing() {
    let parser = DockerJsonParser::new();
    
    let json_log = r#"{"log":"ERROR: Something went wrong\n","stream":"stderr","time":"2024-01-01T12:00:01.000000000Z"}"#;
    let bytes = Bytes::from(json_log);
    
    let entry = parser.parse(bytes).unwrap();
    
    assert_eq!(entry.stream, "stderr");
    assert!(entry.log.contains("ERROR"));
}

#[test]
fn test_docker_json_multiline_log() {
    let parser = DockerJsonParser::new();
    
    let json_log = r#"{"log":"Line 1\nLine 2\nLine 3\n","stream":"stdout","time":"2024-01-01T12:00:00.000000000Z"}"#;
    let bytes = Bytes::from(json_log);
    
    let entry = parser.parse(bytes).unwrap();
    
    assert!(entry.log.contains("Line 1"));
    assert!(entry.log.contains("Line 2"));
    assert!(entry.log.contains("Line 3"));
}

#[test]
fn test_docker_json_invalid_format() {
    let parser = DockerJsonParser::new();
    
    let invalid_json = r#"{"log":"Missing fields"}"#;
    let bytes = Bytes::from(invalid_json);
    
    let result = parser.parse(bytes);
    assert!(result.is_err());
    assert!(matches!(result.unwrap_err(), ParseError::MissingField(_)));
}

#[test]
fn test_docker_json_malformed() {
    let parser = DockerJsonParser::new();
    
    let malformed = r#"{"log":"test""#; // Missing closing brace
    let bytes = Bytes::from(malformed);
    
    let result = parser.parse(bytes);
    assert!(result.is_err());
    assert!(matches!(result.unwrap_err(), ParseError::JsonError(_)));
}