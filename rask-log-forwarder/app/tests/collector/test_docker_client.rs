use rask_log_forwarder::collector::DockerCollector;

#[tokio::test]
async fn test_docker_client_connection() {
    let collector = DockerCollector::new().await;
    assert!(collector.is_ok(), "Should connect to Docker daemon");

    let collector = collector.unwrap();
    assert!(
        collector.can_connect().await,
        "Should be able to ping Docker"
    );
}

#[tokio::test]
async fn test_docker_client_connection_failure() {
    // Mock scenario where Docker is not available
    let collector = DockerCollector::new_with_socket("unix:///nonexistent/docker.sock").await;
    assert!(collector.is_err(), "Should fail when Docker is unavailable");
}
