use bytes::Bytes;
use rask_log_forwarder::parser::{DockerJsonParser, ParseError};

#[test]
fn test_parse_docker_json_log_format() {
    let json_log = r#"{"log":"192.168.1.1 - - [25/Dec/2023:10:00:00 +0000] \"GET / HTTP/1.1\" 200 612\n","stream":"stdout","time":"2023-12-25T10:00:00.000000000Z"}"#;
    let parser = DockerJsonParser::new();
    let result = parser.parse(Bytes::from(json_log)).unwrap();

    assert_eq!(result.stream, "stdout");
    assert!(result.log.contains("GET /"));
    assert_eq!(result.time, "2023-12-25T10:00:00.000000000Z");
}

#[test]
fn test_parse_docker_json_stderr() {
    let json_log = r#"{"log":"2023/12/25 10:00:00 [error] 29#29: *1 connect() failed\n","stream":"stderr","time":"2023-12-25T10:00:00.000000000Z"}"#;
    let parser = DockerJsonParser::new();
    let result = parser.parse(Bytes::from(json_log)).unwrap();

    assert_eq!(result.stream, "stderr");
    assert!(result.log.contains("connect() failed"));
}

#[test]
fn test_parse_invalid_docker_json() {
    let invalid_json = r#"{"log":"test","stream":"stdout""#; // Missing closing brace
    let parser = DockerJsonParser::new();
    let result = parser.parse(Bytes::from(invalid_json));

    assert!(result.is_err());
    assert!(matches!(result.unwrap_err(), ParseError::JsonError(_)));
}

#[test]
fn test_parse_missing_required_fields() {
    let json_without_log = r#"{"stream":"stdout","time":"2023-12-25T10:00:00.000000000Z"}"#;
    let parser = DockerJsonParser::new();
    let result = parser.parse(Bytes::from(json_without_log));

    assert!(result.is_err());
    assert!(matches!(result.unwrap_err(), ParseError::MissingField(_)));
}
