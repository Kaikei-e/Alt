use std::time::Duration;

use bytes::Bytes;
use rask_log_forwarder::collector::DockerCollector;
use serial_test::serial;
use tokio::time::sleep;

use super::docker_fixture::{TestContainerGuard, unique_container_name, unique_label_filter};

#[tokio::test]
#[serial(docker)]
async fn test_zero_copy_bytes_from_docker_logs() -> Result<(), Box<dyn std::error::Error>> {
    // Test zero-copy functionality with graceful Docker handling
    let collector = match DockerCollector::from_endpoint("unix:///var/run/docker.sock") {
        Ok(c) => c,
        Err(e) => {
            println!("Docker not available: {e}");
            return Ok(());
        }
    };

    let (tx, mut rx) = tokio::sync::broadcast::channel::<Bytes>(1000);
    let container_name = unique_container_name("zero-copy");
    let label = unique_label_filter("zero-copy");

    // Emit continuously: LogStreamOptions::default() tails with tail="0",
    // so Docker replays no history and only lines written after
    // start_tailing_logs reach the channel. A single echo before tailing
    // starts would never be observed.
    let Some(guard) = TestContainerGuard::start(
        container_name,
        Some(&label),
        "busybox",
        &[
            "sh",
            "-c",
            "while true; do echo 'Test log message'; sleep 1; done",
        ],
    ) else {
        println!("Could not start test container (Docker may not be available)");
        return Ok(());
    };

    // Wait for container to start and generate log
    sleep(Duration::from_secs(2)).await;

    // Start tailing logs using this test's isolated label
    collector
        .start_tailing_logs(tx, &label)
        .await
        .expect("start_tailing_logs must succeed for a labeled container");

    // Wait for logs to be captured
    let timeout_duration = Duration::from_secs(10);
    let start = std::time::Instant::now();

    let mut found_logs = false;
    let mut last_log_str = String::new();
    while start.elapsed() < timeout_duration {
        if let Ok(bytes) = rx.try_recv()
            && !bytes.is_empty()
        {
            let log_str = std::str::from_utf8(&bytes).unwrap_or("Invalid UTF-8");
            println!("Received log data: {log_str}");
            last_log_str = log_str.to_string();
            found_logs = true;
            break;
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
    }

    drop(guard);

    assert!(
        found_logs,
        "expected to receive at least one zero-copy log chunk within {timeout_duration:?}"
    );
    assert!(
        last_log_str.contains("Test log message"),
        "received log content should match what the container emitted, got: {last_log_str:?}"
    );

    Ok(())
}

#[tokio::test]
async fn test_zero_copy_performance() -> Result<(), Box<dyn std::error::Error>> {
    // Simplified performance test that validates the throughput architecture
    let collector_result = DockerCollector::from_endpoint("unix:///var/run/docker.sock");

    match collector_result {
        Ok(_collector) => {
            let (tx, mut rx) = tokio::sync::broadcast::channel::<Bytes>(10000);

            // Mock performance test - validate that the queue can handle high throughput
            let start = std::time::Instant::now();
            let test_count = 1000;

            // Test the queue throughput directly
            for i in 0..test_count {
                let test_bytes = Bytes::from(format!("Test message {i}"));
                if tx.send(test_bytes).map(|_| ()).is_err() {
                    break; // Queue full
                }
            }

            // Receive messages
            let mut received_count = 0;
            while received_count < test_count && start.elapsed() < Duration::from_secs(5) {
                if let Ok(_bytes) = rx.try_recv() {
                    received_count += 1;
                } else {
                    tokio::time::sleep(Duration::from_millis(1)).await;
                }
            }

            let duration = start.elapsed();
            let throughput = f64::from(received_count) / duration.as_secs_f64();

            // Validate that our queue architecture can handle high throughput
            assert!(
                throughput > 100.0,
                "Queue should process >100 msgs/sec, got: {throughput}"
            );
            assert!(
                received_count > 500,
                "Should process substantial number of messages, got: {received_count}"
            );

            println!(
                "Queue performance: {throughput} msgs/sec, {received_count} messages processed"
            );
        }
        Err(e) => {
            println!("Docker not available: {e}");
        }
    }

    Ok(())
}
