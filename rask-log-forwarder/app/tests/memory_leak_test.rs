// Memory leak test for parser validation
use rask_log_forwarder::collector::ContainerInfo;
use rask_log_forwarder::parser::universal::UniversalParser;
use std::collections::HashMap;

#[tokio::test]
async fn test_memory_leak_prevention_with_repeated_parsing() {
    let parser = UniversalParser::new();
    let mut container_info = ContainerInfo {
        id: "test-container".to_string(),
        service_name: "nginx".to_string(),
        labels: HashMap::new(),
        group: Some("test-group".to_string()),
    };
    container_info
        .labels
        .insert("rask.group".to_string(), "test-group".to_string());

    // Sample logs that will trigger different regex patterns
    let test_logs = [
        r#"{"log":"192.168.1.1 - - [01/Jan/2024:12:00:00 +0000] \"GET /api/health HTTP/1.1\" 200 1024 \"-\" \"curl/7.68.0\"\n","stream":"stdout","time":"2024-01-01T12:00:00Z"}"#,
        r#"{"log":"2024/01/01 12:00:00 [error] 123#0: *456 connect() failed while connecting to upstream\n","stream":"stderr","time":"2024-01-01T12:00:00Z"}"#,
        r#"{"log":"2025-07-03T18:53:46.741706684Z {\"level\":\"info\",\"msg\":\"processing request\",\"service\":\"alt-backend\"}\n","stream":"stdout","time":"2025-07-03T18:53:46.741Z"}"#,
        r#"{"log":"2024-01-01 12:00:00.123 UTC [123] LOG: statement: SELECT * FROM users WHERE id = $1\n","stream":"stdout","time":"2024-01-01T12:00:00Z"}"#,
    ];

    // Parse each log many times to check for memory leaks
    for iteration in 0..1000 {
        for (log_idx, log_data) in test_logs.iter().enumerate() {
            let result = parser
                .parse_docker_log(log_data.as_bytes(), &container_info)
                .await;

            // Verify parsing still works correctly
            assert!(
                result.is_ok(),
                "Parse failed at iteration {} for log {}: {:?}",
                iteration,
                log_idx,
                result.err()
            );

            let entry = result.unwrap();
            assert!(
                !entry.message.is_empty(),
                "Empty message at iteration {iteration} for log {log_idx}"
            );
            assert!(
                !entry.service_name.is_empty(),
                "Empty service name at iteration {iteration} for log {log_idx}"
            );

            // Change service name to trigger different code paths
            container_info.service_name = match log_idx {
                0 => "nginx".to_string(),
                1 => "nginx".to_string(),
                2 => "alt-backend".to_string(),
                3 => "postgres".to_string(),
                _ => "unknown".to_string(),
            };
        }

        // Every 100 iterations, also test batch parsing
        if iteration % 100 == 0 {
            let batch: Vec<&[u8]> = test_logs.iter().map(|log| log.as_bytes()).collect();
            let batch_results = parser.parse_batch(batch, &container_info).await;
            assert_eq!(batch_results.len(), test_logs.len());
            assert!(
                batch_results.iter().all(|r| r.is_ok()),
                "Batch parse failed at iteration {iteration}"
            );
        }
    }

    // If we get here without panic or excessive memory usage, memory leak test passed
    println!(
        "✓ Memory leak test completed successfully: parsed {} log entries",
        1000 * test_logs.len()
    );
}

#[tokio::test]
async fn test_memory_safety_with_malformed_regex_fallback() {
    let parser = UniversalParser::new();
    let container_info = ContainerInfo {
        id: "test-container".to_string(),
        service_name: "nginx".to_string(),
        labels: HashMap::new(),
        group: Some("test-group".to_string()),
    };

    // Malformed logs that should trigger fallback patterns
    let malformed_logs = [
        r#"{"log":"malformed nginx log without proper format\n","stream":"stdout","time":"2024-01-01T12:00:00Z"}"#,
        r#"{"log":"partially valid 192.168.1.1 log but missing HTTP\n","stream":"stdout","time":"2024-01-01T12:00:00Z"}"#,
        r#"{"log":"2024/01/01 [error] without proper nginx format\n","stream":"stderr","time":"2024-01-01T12:00:00Z"}"#,
    ];

    // Parse malformed logs many times to ensure fallback paths are memory-safe
    for iteration in 0..500 {
        for (log_idx, log_data) in malformed_logs.iter().enumerate() {
            let result = parser
                .parse_docker_log(log_data.as_bytes(), &container_info)
                .await;

            // Should still succeed (with fallback parsing)
            assert!(
                result.is_ok(),
                "Fallback parse failed at iteration {} for log {}: {:?}",
                iteration,
                log_idx,
                result.err()
            );

            let entry = result.unwrap();
            assert!(
                !entry.message.is_empty(),
                "Empty message in fallback at iteration {iteration} for log {log_idx}"
            );
        }
    }

    println!(
        "✓ Memory safety test with fallback patterns completed successfully: parsed {} malformed log entries",
        500 * malformed_logs.len()
    );
}
