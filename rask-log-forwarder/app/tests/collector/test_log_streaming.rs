use rask_log_forwarder::collector::{DockerCollector, LogStreamOptions};
use bytes::Bytes;
use std::process::{Command, Stdio};
use std::time::Duration;
use tokio::time::sleep;

async fn start_test_nginx_container() -> String {
    let output = Command::new("docker")
        .args(&[
            "run", "-d", 
            "--label", "com.alt.log-forward=true",
            "--name", "test-nginx-streaming",
            "nginx:alpine"
        ])
        .stdout(Stdio::piped())
        .output()
        .expect("Failed to start test container");
    
    let container_id = String::from_utf8(output.stdout)
        .expect("Invalid UTF-8 in container ID")
        .trim()
        .to_string();
    
    // Wait for container to be ready
    sleep(Duration::from_secs(2)).await;
    
    container_id
}

async fn cleanup_test_container(container_id: String) {
    Command::new("docker")
        .args(&["rm", "-f", &container_id])
        .output()
        .expect("Failed to cleanup test container");
}

#[tokio::test]
async fn test_nginx_log_stream_initialization() {
    let collector = DockerCollector::new().await.unwrap();
    let (tx, _rx) = multiqueue::broadcast_queue::<Bytes>(1000);
    
    // Start test nginx container
    let test_container = start_test_nginx_container().await;
    
    let result = collector.start_tailing_logs(tx, "com.alt.log-forward=true").await;
    assert!(result.is_ok(), "Should successfully start tailing logs");
    
    cleanup_test_container(test_container).await;
}

#[tokio::test]
async fn test_log_stream_with_options() {
    let collector = DockerCollector::new().await.unwrap();
    let (tx, _rx) = multiqueue::broadcast_queue::<Bytes>(1000);
    
    let options = LogStreamOptions {
        follow: true,
        stdout: true,
        stderr: true,
        timestamps: true,
        tail: "100".to_string(),
    };
    
    let test_container = start_test_nginx_container().await;
    
    let result = collector.start_tailing_logs_with_options(tx, "com.alt.log-forward=true", options).await;
    assert!(result.is_ok(), "Should start tailing with custom options");
    
    cleanup_test_container(test_container).await;
}