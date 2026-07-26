use rask_log_forwarder::collector::DockerCollector;

#[tokio::test]
async fn test_docker_client_connection() {
    // `DockerCollector::new()` only opens the local socket; it does not ping
    // the daemon. So a successful `Ok(collector)` here says nothing about
    // whether Docker is actually reachable -- that's what `can_connect()` is
    // for, and it must be checked, not merely printed. Only the "Docker
    // wasn't even available to open a client for" case is allowed to skip.
    let collector_result = DockerCollector::new().await;

    match collector_result {
        Ok(collector) => {
            let can_connect = collector.can_connect().await;
            assert!(
                can_connect,
                "DockerCollector::new() succeeded but can_connect() reported unreachable; \
                 the client and the daemon-reachability check must agree"
            );
        }
        Err(e) => {
            // If Docker is not available, that's also a valid test case
            println!("Docker not available (expected in some environments): {e}");
        }
    }
}

#[tokio::test]
async fn test_docker_client_connection_failure() {
    // Mock scenario where Docker is not available
    let collector = DockerCollector::new_with_socket("unix:///nonexistent/docker.sock").await;
    assert!(collector.is_err(), "Should fail when Docker is unavailable");
}
