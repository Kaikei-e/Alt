/// Test that healthcheck succeeds when server is running
#[tokio::test]
async fn test_healthcheck_succeeds_when_server_running() {
    // Bind the listener synchronously (before spawning the accept loop) so
    // the socket is already listening — and therefore able to accept the
    // healthcheck's connection into its backlog — the moment `bind` returns.
    // The previous version bound with `std::net::TcpListener`, dropped it,
    // and hoped a fixed `sleep(100ms)` was long enough for `axum::serve` to
    // rebind the freed port and start its accept loop before the healthcheck
    // ran: a real TOCTOU race against the OS reusing the port, made "usually
    // fine" only by an arbitrary wall-clock guess.
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let port = listener.local_addr().unwrap().port();

    let mock_server = tokio::spawn(async move {
        let app =
            axum::Router::new().route("/v1/health", axum::routing::get(|| async { "Healthy" }));
        axum::serve(listener, app).await.unwrap();
    });

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
    let listener = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
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
    // Same pre-bound-listener approach as the success test: the listener is
    // already accepting connections before the healthcheck call, so no
    // arbitrary "wait for server to start" sleep is needed.
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let port = listener.local_addr().unwrap().port();

    let mock_server = tokio::spawn(async move {
        let app = axum::Router::new().route(
            "/v1/health",
            axum::routing::get(|| async {
                (axum::http::StatusCode::SERVICE_UNAVAILABLE, "Unhealthy")
            }),
        );
        axum::serve(listener, app).await.unwrap();
    });

    // Run healthcheck
    let result = rask::healthcheck_with_port(port).await;
    assert!(result.is_err(), "Healthcheck should fail on non-2xx status");

    mock_server.abort();
}
