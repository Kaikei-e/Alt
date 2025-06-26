use bytes::Bytes;
use rask_log_forwarder::parser::SimdParser;

#[test]
fn test_parse_nginx_access_log_common_format() {
    let docker_log = r#"{"log":"192.168.1.1 - - [25/Dec/2023:10:00:00 +0000] \"GET /api/health HTTP/1.1\" 200 612 \"-\" \"curl/7.68.0\"\n","stream":"stdout","time":"2023-12-25T10:00:00.000000000Z"}"#;
    let parser = SimdParser::new();
    let result = parser.parse_nginx_log(Bytes::from(docker_log)).unwrap();
    
    assert_eq!(result.service_type, "nginx");
    assert_eq!(result.log_type, "access");
    assert_eq!(result.ip_address.unwrap(), "192.168.1.1");
    assert_eq!(result.method.unwrap(), "GET");
    assert_eq!(result.path.unwrap(), "/api/health");
    assert_eq!(result.status_code.unwrap(), 200);
    assert_eq!(result.response_size.unwrap(), 612);
    assert_eq!(result.user_agent.unwrap(), "curl/7.68.0");
}

#[test]
fn test_parse_nginx_access_log_combined_format() {
    let docker_log = r#"{"log":"10.0.0.1 - user [25/Dec/2023:10:15:30 +0000] \"POST /api/users HTTP/1.1\" 201 1024 \"https://example.com/\" \"Mozilla/5.0 (Windows NT 10.0; Win64; x64)\"\n","stream":"stdout","time":"2023-12-25T10:15:30.000000000Z"}"#;
    let parser = SimdParser::new();
    let result = parser.parse_nginx_log(Bytes::from(docker_log)).unwrap();
    
    assert_eq!(result.service_type, "nginx");
    assert_eq!(result.log_type, "access");
    assert_eq!(result.ip_address.unwrap(), "10.0.0.1");
    assert_eq!(result.method.unwrap(), "POST");
    assert_eq!(result.path.unwrap(), "/api/users");
    assert_eq!(result.status_code.unwrap(), 201);
    assert_eq!(result.response_size.unwrap(), 1024);
}

#[test]
fn test_parse_nginx_access_log_with_query_params() {
    let docker_log = r#"{"log":"127.0.0.1 - - [25/Dec/2023:11:00:00 +0000] \"GET /search?q=test&limit=10 HTTP/1.1\" 200 2048\n","stream":"stdout","time":"2023-12-25T11:00:00.000000000Z"}"#;
    let parser = SimdParser::new();
    let result = parser.parse_nginx_log(Bytes::from(docker_log)).unwrap();
    
    assert_eq!(result.path.unwrap(), "/search?q=test&limit=10");
    assert_eq!(result.status_code.unwrap(), 200);
}

#[test]
fn test_parse_invalid_nginx_access_log() {
    let docker_log = r#"{"log":"This is not a valid nginx access log\n","stream":"stdout","time":"2023-12-25T10:00:00.000000000Z"}"#;
    let parser = SimdParser::new();
    let result = parser.parse_nginx_log(Bytes::from(docker_log));
    
    assert!(result.is_err());
    // Should fall back to generic log entry
}