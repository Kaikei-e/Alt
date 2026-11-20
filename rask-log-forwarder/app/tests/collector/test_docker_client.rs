use rask_log_forwarder::collector::DockerCollector;

#[tokio::test]
async fn test_docker_client_connection() {
    // `DockerCollector::from_endpoint()` only opens the local socket; it does not ping
    // the daemon. So a successful `Ok(collector)` here says nothing about
    // whether Docker is actually reachable -- that's what `can_connect()` is
    // for, and it must be checked, not merely printed. Only the "Docker
    // wasn't even available to open a client for" case is allowed to skip.
    let collector_result = DockerCollector::from_endpoint("unix:///var/run/docker.sock");

    match collector_result {
        Ok(collector) => {
            let can_connect = collector.can_connect().await;
            assert!(
                can_connect,
                "DockerCollector::from_endpoint() succeeded but can_connect() reported unreachable; \
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
    let collector_result = DockerCollector::from_endpoint("unix:///nonexistent/docker.sock");
    if let Ok(collector) = collector_result {
        assert!(
            !collector.can_connect().await,
            "Should report unreachable when Docker daemon socket is nonexistent"
        );
    } else {
        // Initialization error is also an acceptable failure mode
    }
}
