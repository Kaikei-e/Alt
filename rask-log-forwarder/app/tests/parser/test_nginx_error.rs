use bytes::Bytes;
use rask_log_forwarder::parser::SimdParser;

#[test]
fn test_parse_nginx_error_log() {
    let docker_log = r#"{"log":"2023/12/25 10:00:00 [error] 29#29: *1 connect() failed (111: Connection refused) while connecting to upstream\n","stream":"stderr","time":"2023-12-25T10:00:00.000000000Z"}"#;
    let parser = SimdParser::new();
    let result = parser.parse_nginx_log(Bytes::from(docker_log)).unwrap();
    
    assert_eq!(result.service_type, "nginx");
    assert_eq!(result.log_type, "error");
    assert_eq!(result.level.unwrap(), "error");
    assert!(result.message.contains("Connection refused"));
}

#[test]
fn test_parse_nginx_warning_log() {
    let docker_log = r#"{"log":"2023/12/25 10:05:00 [warn] 29#29: *2 upstream server temporarily disabled while reading response header from upstream\n","stream":"stderr","time":"2023-12-25T10:05:00.000000000Z"}"#;
    let parser = SimdParser::new();
    let result = parser.parse_nginx_log(Bytes::from(docker_log)).unwrap();
    
    assert_eq!(result.service_type, "nginx");
    assert_eq!(result.log_type, "error");
    assert_eq!(result.level.unwrap(), "warn");
}

#[test]
fn test_parse_nginx_info_log() {
    let docker_log = r#"{"log":"2023/12/25 10:10:00 [notice] 29#29: signal process started\n","stream":"stderr","time":"2023-12-25T10:10:00.000000000Z"}"#;
    let parser = SimdParser::new();
    let result = parser.parse_nginx_log(Bytes::from(docker_log)).unwrap();
    
    assert_eq!(result.level.unwrap(), "notice");
}