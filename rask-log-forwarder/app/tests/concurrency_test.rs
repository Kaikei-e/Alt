// Concurrency test for TASK3 validation
use rask_log_forwarder::collector::ContainerInfo;
use rask_log_forwarder::parser::universal::UniversalParser;
use std::collections::HashMap;
use std::sync::Arc;
use tokio::task::JoinSet;

#[tokio::test]
async fn test_concurrent_parsing_thread_safety() {
    let parser = Arc::new(UniversalParser::new());
    let mut container_info = ContainerInfo {
        id: "test-container".to_string(),
        service_name: "nginx".to_string(),
        labels: HashMap::new(),
        group: Some("test-group".to_string()),
    };
    container_info
        .labels
        .insert("rask.group".to_string(), "test-group".to_string());
    let container_info = Arc::new(container_info);

    // Sample logs for concurrent parsing
    let test_logs = vec![
        r#"{"log":"192.168.1.1 - - [01/Jan/2024:12:00:00 +0000] \"GET /api/health HTTP/1.1\" 200 1024 \"-\" \"curl/7.68.0\"\n","stream":"stdout","time":"2024-01-01T12:00:00Z"}"#,
        r#"{"log":"2024/01/01 12:00:00 [error] 123#0: *456 connect() failed while connecting to upstream\n","stream":"stderr","time":"2024-01-01T12:00:00Z"}"#,
        r#"{"log":"2025-07-03T18:53:46.741706684Z {\"level\":\"info\",\"msg\":\"processing request\",\"service\":\"alt-backend\"}\n","stream":"stdout","time":"2025-07-03T18:53:46.741Z"}"#,
        r#"{"log":"2024-01-01 12:00:00.123 UTC [123] LOG: statement: SELECT * FROM users WHERE id = $1\n","stream":"stdout","time":"2024-01-01T12:00:00Z"}"#,
    ];

    let mut join_set = JoinSet::new();
    let num_threads = 10;
    let iterations_per_thread = 100;

    // Spawn multiple concurrent tasks
    for thread_id in 0..num_threads {
        let parser_clone = parser.clone();
        let container_info_clone = container_info.clone();
        let test_logs_clone = test_logs.clone();

        join_set.spawn(async move {
            let mut results = Vec::new();

            for iteration in 0..iterations_per_thread {
                for (log_idx, log_data) in test_logs_clone.iter().enumerate() {
                    let result = parser_clone
                        .parse_docker_log(log_data.as_bytes(), &container_info_clone)
                        .await;

                    // Verify parsing works correctly under concurrency
                    assert!(
                        result.is_ok(),
                        "Thread {} parse failed at iteration {} for log {}: {:?}",
                        thread_id,
                        iteration,
                        log_idx,
                        result.err()
                    );

                    let entry = result.unwrap();
                    assert!(
                        !entry.message.is_empty(),
                        "Thread {} empty message at iteration {} for log {}",
                        thread_id,
                        iteration,
                        log_idx
                    );
                    assert!(
                        !entry.service_name.is_empty(),
                        "Thread {} empty service name at iteration {} for log {}",
                        thread_id,
                        iteration,
                        log_idx
                    );

                    results.push(entry);
                }
            }

            (thread_id, results)
        });
    }

    // Wait for all tasks to complete
    let mut total_parsed = 0;
    while let Some(result) = join_set.join_next().await {
        match result {
            Ok((thread_id, results)) => {
                total_parsed += results.len();
                println!(
                    "✓ Thread {} completed: parsed {} log entries",
                    thread_id,
                    results.len()
                );

                // Verify all entries are properly parsed
                for entry in results {
                    assert!(
                        !entry.message.is_empty(),
                        "Thread {} produced empty message",
                        thread_id
                    );
                    assert!(
                        !entry.container_id.is_empty(),
                        "Thread {} produced empty container ID",
                        thread_id
                    );
                }
            }
            Err(e) => {
                panic!("Thread failed: {:?}", e);
            }
        }
    }

    println!(
        "✓ Concurrency test completed successfully: {} threads parsed {} total log entries",
        num_threads, total_parsed
    );
    assert_eq!(
        total_parsed,
        num_threads * iterations_per_thread * test_logs.len()
    );
}

#[tokio::test]
async fn test_concurrent_regex_pattern_access() {
    use rask_log_forwarder::parser::generated::{VALIDATED_PATTERNS, pattern_index};

    let mut join_set = JoinSet::new();
    let num_threads = 20;
    let iterations_per_thread = 50;

    // Test concurrent access to the static regex patterns
    for thread_id in 0..num_threads {
        join_set.spawn(async move {
            let mut pattern_access_count = 0;

            for _iteration in 0..iterations_per_thread {
                // Access different patterns concurrently
                let patterns_to_test = [
                    pattern_index::DOCKER_NATIVE_TIMESTAMP,
                    pattern_index::NGINX_ACCESS_FULL,
                    pattern_index::NGINX_ERROR_FULL,
                    pattern_index::POSTGRES_LOG,
                    pattern_index::SIMD_NGINX_ACCESS,
                    pattern_index::SIMD_NGINX_COMBINED,
                ];

                for pattern_idx in patterns_to_test {
                    let pattern_result = VALIDATED_PATTERNS.get(pattern_idx);
                    assert!(
                        pattern_result.is_ok(),
                        "Thread {} failed to access pattern {}",
                        thread_id,
                        pattern_idx
                    );
                    pattern_access_count += 1;
                }
            }

            (thread_id, pattern_access_count)
        });
    }

    // Wait for all tasks to complete
    let mut total_accesses = 0;
    while let Some(result) = join_set.join_next().await {
        match result {
            Ok((thread_id, access_count)) => {
                total_accesses += access_count;
                println!(
                    "✓ Thread {} completed: {} pattern accesses",
                    thread_id, access_count
                );
            }
            Err(e) => {
                panic!("Thread failed: {:?}", e);
            }
        }
    }

    println!(
        "✓ Concurrent regex pattern access test completed: {} threads made {} total pattern accesses",
        num_threads, total_accesses
    );
    assert_eq!(total_accesses, num_threads * iterations_per_thread * 6); // 6 patterns per iteration
}

#[tokio::test]
async fn test_concurrent_batch_parsing() {
    let parser = Arc::new(UniversalParser::new());
    let container_info = Arc::new(ContainerInfo {
        id: "test-container".to_string(),
        service_name: "nginx".to_string(),
        labels: HashMap::new(),
        group: Some("test-group".to_string()),
    });

    let test_logs = vec![
        r#"{"log":"192.168.1.1 - - [01/Jan/2024:12:00:00 +0000] \"GET /api/health HTTP/1.1\" 200 1024 \"-\" \"curl/7.68.0\"\n","stream":"stdout","time":"2024-01-01T12:00:00Z"}"#,
        r#"{"log":"2024/01/01 12:00:00 [error] 123#0: *456 connect() failed while connecting to upstream\n","stream":"stderr","time":"2024-01-01T12:00:00Z"}"#,
        r#"{"log":"2025-07-03T18:53:46.741706684Z {\"level\":\"info\",\"msg\":\"processing request\",\"service\":\"alt-backend\"}\n","stream":"stdout","time":"2025-07-03T18:53:46.741Z"}"#,
    ];

    let mut join_set = JoinSet::new();
    let num_threads = 8;
    let batches_per_thread = 25;

    // Spawn multiple concurrent batch parsing tasks
    for thread_id in 0..num_threads {
        let parser_clone = parser.clone();
        let container_info_clone = container_info.clone();
        let test_logs_clone = test_logs.clone();

        join_set.spawn(async move {
            let mut total_entries = 0;

            for batch_idx in 0..batches_per_thread {
                let batch: Vec<&[u8]> = test_logs_clone.iter().map(|log| log.as_bytes()).collect();
                let batch_results = parser_clone.parse_batch(batch, &container_info_clone).await;

                // Verify batch parsing works correctly under concurrency
                assert_eq!(
                    batch_results.len(),
                    test_logs_clone.len(),
                    "Thread {} batch {} wrong length",
                    thread_id,
                    batch_idx
                );
                assert!(
                    batch_results.iter().all(|r| r.is_ok()),
                    "Thread {} batch {} had parsing errors",
                    thread_id,
                    batch_idx
                );

                total_entries += batch_results.len();
            }

            (thread_id, total_entries)
        });
    }

    // Wait for all tasks to complete
    let mut total_parsed = 0;
    while let Some(result) = join_set.join_next().await {
        match result {
            Ok((thread_id, entry_count)) => {
                total_parsed += entry_count;
                println!(
                    "✓ Thread {} completed: parsed {} log entries in batches",
                    thread_id, entry_count
                );
            }
            Err(e) => {
                panic!("Thread failed: {:?}", e);
            }
        }
    }

    println!(
        "✓ Concurrent batch parsing test completed: {} threads parsed {} total log entries",
        num_threads, total_parsed
    );
    assert_eq!(
        total_parsed,
        num_threads * batches_per_thread * test_logs.len()
    );
}
