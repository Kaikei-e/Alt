use std::net::TcpListener;
use std::time::Duration;
use tokio::time::sleep;

/// Test that healthcheck succeeds when server is running
#[tokio::test]
async fn test_healthcheck_succeeds_when_server_running() {
    // Find an available port
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let port = listener.local_addr().unwrap().port();
    drop(listener);

    // Start a minimal mock server that responds to /v1/health
    let mock_server = tokio::spawn(async move {
        let app =
            axum::Router::new().route("/v1/health", axum::routing::get(|| async { "Healthy" }));
        let listener = tokio::net::TcpListener::bind(format!("127.0.0.1:{}", port))
            .await
            .unwrap();
        axum::serve(listener, app).await.unwrap();
    });

    // Wait for server to start
    sleep(Duration::from_millis(100)).await;

    // Run healthcheck
    let result = rask::healthcheck_with_port(port).await;
    assert!(
        result.is_ok(),
        "Healthcheck should succeed when server is running"
    );

    mock_server.abort();
}

/// Test that healthcheck fails when server is not running
#[tokio::test]
async fn test_healthcheck_fails_when_server_not_running() {
    // Use a port that's definitely not in use
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let port = listener.local_addr().unwrap().port();
    drop(listener);

    // Run healthcheck without starting server
    let result = rask::healthcheck_with_port(port).await;
    assert!(
        result.is_err(),
        "Healthcheck should fail when server is not running"
    );
}

/// Test that healthcheck fails when server returns non-2xx status
#[tokio::test]
async fn test_healthcheck_fails_on_non_success_status() {
    // Find an available port
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let port = listener.local_addr().unwrap().port();
    drop(listener);

    // Start a mock server that returns 503
    let mock_server = tokio::spawn(async move {
        let app = axum::Router::new().route(
            "/v1/health",
            axum::routing::get(|| async {
                (axum::http::StatusCode::SERVICE_UNAVAILABLE, "Unhealthy")
            }),
        );
        let listener = tokio::net::TcpListener::bind(format!("127.0.0.1:{}", port))
            .await
            .unwrap();
        axum::serve(listener, app).await.unwrap();
    });

    // Wait for server to start
    sleep(Duration::from_millis(100)).await;

    // Run healthcheck
    let result = rask::healthcheck_with_port(port).await;
    assert!(result.is_err(), "Healthcheck should fail on non-2xx status");

    mock_server.abort();
}
