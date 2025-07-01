use rask_log_forwarder::collector::ContainerInfo;
use rask_log_forwarder::parser::{LogLevel, UniversalParser};
use std::collections::HashMap;

#[tokio::test]
async fn test_universal_parser_with_nginx() {
    let mut labels = HashMap::new();
    labels.insert("rask.group".to_string(), "alt-frontend".to_string());

    let container_info = ContainerInfo {
        id: "nginx123".to_string(),
        service_name: "nginx".to_string(),
        labels,
        group: Some("alt-frontend".to_string()),
    };

    let parser = UniversalParser::new();

    let docker_log = r#"{"log":"192.168.1.1 - - [01/Jan/2024:00:00:00 +0000] \"GET /health HTTP/1.1\" 200 2\n","stream":"stdout","time":"2024-01-01T00:00:00Z"}"#;

    let entry = parser
        .parse_docker_log(docker_log.as_bytes(), &container_info)
        .await
        .unwrap();

    assert_eq!(entry.service_type, "nginx");
    assert_eq!(entry.log_type, "access");
    assert_eq!(entry.status_code, Some(200));
    assert_eq!(entry.container_id, "nginx123");
    assert_eq!(entry.service_group, Some("alt-frontend".to_string()));
}

#[tokio::test]
async fn test_universal_parser_with_go_backend() {
    let mut labels = HashMap::new();
    labels.insert("rask.group".to_string(), "alt-backend".to_string());

    let container_info = ContainerInfo {
        id: "backend456".to_string(),
        service_name: "alt-backend".to_string(),
        labels,
        group: Some("alt-backend".to_string()),
    };

    let parser = UniversalParser::new();

    let docker_log = r#"{"log":"{\"level\":\"info\",\"msg\":\"Processing request\",\"method\":\"GET\"}\n","stream":"stdout","time":"2024-01-01T00:00:00Z"}"#;

    let entry = parser
        .parse_docker_log(docker_log.as_bytes(), &container_info)
        .await
        .unwrap();

    assert_eq!(entry.service_type, "go");
    assert_eq!(entry.log_type, "structured");
    assert_eq!(entry.level, Some(LogLevel::Info));
    assert_eq!(entry.container_id, "backend456");
}

#[tokio::test]
async fn test_universal_parser_with_unknown_service() {
    let container_info = ContainerInfo {
        id: "unknown789".to_string(),
        service_name: "unknown-service".to_string(),
        labels: HashMap::new(),
        group: None,
    };

    let parser = UniversalParser::new();

    let docker_log =
        r#"{"log":"Some random log message\n","stream":"stdout","time":"2024-01-01T00:00:00Z"}"#;

    let entry = parser
        .parse_docker_log(docker_log.as_bytes(), &container_info)
        .await
        .unwrap();

    assert_eq!(entry.service_type, "unknown-service");
    assert_eq!(entry.log_type, "plain");
    assert!(entry.message.contains("Some random log message"));
}
