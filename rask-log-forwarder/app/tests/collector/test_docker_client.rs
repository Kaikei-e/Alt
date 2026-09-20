use rask_log_forwarder::collector::DockerCollector;
use serial_test::serial;

#[tokio::test]
#[serial]
async fn test_docker_client_connection() {
    unsafe {
        std::env::set_var("DOCKER_HOST", "unix:///var/run/docker.sock");
    }

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

    unsafe {
        std::env::remove_var("DOCKER_HOST");
    }
}

#[tokio::test]
#[serial]
async fn test_docker_client_connection_failure() {
    unsafe {
        std::env::set_var("DOCKER_HOST", "unix:///nonexistent/docker.sock");
    }

    let collector_result = DockerCollector::new().await;
    match collector_result {
        Ok(collector) => {
            assert!(
                !collector.can_connect().await,
                "Should report unreachable when Docker daemon socket is nonexistent"
            );
        }
        Err(_) => {
            // Initialization error is also an acceptable failure mode
        }
    }

    unsafe {
        std::env::remove_var("DOCKER_HOST");
    }
}
