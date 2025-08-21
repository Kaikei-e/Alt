# CLAUDE.md - Sidecar Proxy (Alt Backend)

## About This Service

The sidecar-proxy is a lightweight Go HTTP proxy deployed alongside `alt-backend`. It centralizes outbound HTTP policy (timeouts, TLS, allowlist) and enables consistent egress control independent of in-process client code.

- **Language**: Go (1.24+)
- **Deployment**: Independent Deployment and ClusterIP service.
- **Consumed by**: `alt-backend` when `SIDECAR_PROXY_BASE_URL` is configured.

## Core Responsibilities

- **Policy Enforcement**: Enforce egress policies such as HTTPS-only and host allowlisting.
- **Connection Management**: Apply shared timeouts, retries, and exponential backoff where safe.
- **Header Manipulation**: Attach standard headers like trace IDs and user agents.
- **Observability**: Emit structured logs (`slog`) and metrics for all outbound calls.

## Design Principles

- **Lightweight & Performant**: Single, efficient Go binary with minimal overhead.
- **Configurability**: Configuration managed via environment variables and flags.
- **Resilience**: Bounded connection pools to apply backpressure under heavy load.
- **Transparency**: Acts as a transparent HTTP proxy for internal services.

## Test-Driven Development (TDD) for a Proxy

### Testing Strategy
Testing a proxy requires a multi-layered approach to verify its core logic, routing, and error handling capabilities without relying on external network calls in unit tests.

### 1. In-Memory Testing with `net/http/httptest`
The `httptest` package is the foundation for testing Go HTTP components.
- **`httptest.NewServer`**: Used to create a mock destination server. This allows you to simulate various backend responses (e.g., 200 OK, 404 Not Found, 500 Internal Server Error) and assert that the proxy handles them correctly.
- **`httptest.NewRequest`**: Used to create incoming requests to your proxy's handler.
- **`httptest.ResponseRecorder`**: Acts as a mock `http.ResponseWriter` to capture the proxy's response to the client for inspection.

### 2. Three-Part Testing Setup
A robust test setup for a proxy involves three components:
1.  **Mock Client**: Simulates a user or service sending a request to your proxy.
2.  **Proxy Instance**: The actual `httputil.ReverseProxy` or custom proxy handler being tested.
3.  **Mock Destination Server**: An `httptest.Server` that acts as the upstream service the proxy forwards requests to.

This setup provides full control over the request lifecycle, enabling you to test behavior from end to end in an isolated environment.

### 3. Key Testing Scenarios
- **Header Manipulation**: Verify that hop-by-hop headers (e.g., `Connection`, `Keep-Alive`) are correctly stripped and that proxy-specific headers (e.g., `X-Forwarded-For`) are added or updated.
- **HTTPS & `CONNECT` Tunneling**: For forward proxies, test the `CONNECT` method to ensure HTTPS traffic is tunneled correctly.
- **Error Handling**:
    - **Upstream Failure**: Test what happens when the destination server is unavailable (e.g., by closing the `httptest.Server`).
    - **Timeouts**: Ensure the proxy correctly handles timeouts when the destination server is slow to respond.
- **Body Handling**: Verify that request and response bodies are streamed efficiently without excessive buffering, especially for large payloads.

### Example Test
```go
func TestProxyHandler(t *testing.T) {
    // 1. Create a mock destination server
    backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Assert that the backend received the correct headers
        assert.Equal(t, "true", r.Header.Get("X-Forwarded-For"))
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("Hello from backend"))
    }))
    defer backendServer.Close()

    // 2. Create the proxy handler
    backendURL, _ := url.Parse(backendServer.URL)
    proxy := httputil.NewSingleHostReverseProxy(backendURL)

    // 3. Create a request to the proxy
    req := httptest.NewRequest("GET", "/", nil)
    rr := httptest.NewRecorder()

    // 4. Serve the request through the proxy
    proxy.ServeHTTP(rr, req)

    // 5. Assert the response
    assert.Equal(t, http.StatusOK, rr.Code)
    assert.Equal(t, "Hello from backend", rr.Body.String())
}
```

## Operations

- **Health Checks**: Provides `/healthz` and `/readyz` endpoints for Kubernetes probes.
- **Configuration**: Configuration can be reloaded via SIGHUP where applicable.
- **Observability**: Integrates with the cluster's telemetry and logging infrastructure.

## References
- [Testing HTTP handlers in Go](https://golang.cafe/blog/how-to-test-http-handlers-in-go.html)
- [Go `httptest` package](https://pkg.go.dev/net/http/httptest/)

