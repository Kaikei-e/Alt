use rask_log_forwarder::parser::{NginxParser, GoStructuredParser, PostgresParser, ServiceParser, LogLevel};

#[test]
fn test_nginx_access_log_parsing() {
    let parser = NginxParser::new();
    
    let nginx_log = r#"192.168.1.100 - - [01/Jan/2024:12:00:00 +0000] "GET /api/health HTTP/1.1" 200 1024 "-" "curl/7.68.0""#;
    
    let entry = parser.parse_log(nginx_log).unwrap();
    
    assert_eq!(entry.service_type, "nginx");
    assert_eq!(entry.log_type, "access");
    assert_eq!(entry.ip_address, Some("192.168.1.100".to_string()));
    assert_eq!(entry.method, Some("GET".to_string()));
    assert_eq!(entry.path, Some("/api/health".to_string()));
    assert_eq!(entry.status_code, Some(200));
    assert_eq!(entry.response_size, Some(1024));
}

#[test]
fn test_nginx_error_log_parsing() {
    let parser = NginxParser::new();
    
    let nginx_error = r#"2024/01/01 12:00:00 [error] 123#0: *456 connect() failed (111: Connection refused) while connecting to upstream"#;
    
    let entry = parser.parse_log(nginx_error).unwrap();
    
    assert_eq!(entry.log_type, "error");
    assert_eq!(entry.level, Some(LogLevel::Error));
    assert!(entry.message.contains("Connection refused"));
}

#[test]
fn test_go_structured_log_parsing() {
    let parser = GoStructuredParser::new();
    
    let go_log = r#"{"level":"info","ts":"2024-01-01T12:00:00.123Z","caller":"main.go:42","msg":"Request processed","method":"GET","path":"/api/users","status":200,"duration":"15ms"}"#;
    
    let entry = parser.parse_log(go_log).unwrap();
    
    assert_eq!(entry.service_type, "go");
    assert_eq!(entry.level, Some(LogLevel::Info));
    assert_eq!(entry.message, "Request processed");
    assert_eq!(entry.method, Some("GET".to_string()));
    assert_eq!(entry.path, Some("/api/users".to_string()));
    assert_eq!(entry.status_code, Some(200));
}

#[test]
fn test_postgres_log_parsing() {
    let parser = PostgresParser::new();
    
    let pg_log = r#"2024-01-01 12:00:00.123 UTC [123] LOG:  statement: SELECT * FROM users WHERE id = $1"#;
    
    let entry = parser.parse_log(pg_log).unwrap();
    
    assert_eq!(entry.service_type, "postgres");
    assert_eq!(entry.level, Some(LogLevel::Info));
    assert!(entry.message.contains("SELECT * FROM users"));
}

#[test]
fn test_unknown_format_fallback() {
    let parser = GoStructuredParser::new();
    
    let unknown_log = "This is just plain text without structure";
    
    let entry = parser.parse_log(unknown_log).unwrap();
    
    assert_eq!(entry.service_type, "go");
    assert_eq!(entry.message, unknown_log);
    assert_eq!(entry.level, Some(LogLevel::Info)); // Default level
}