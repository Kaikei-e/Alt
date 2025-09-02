# rask-log-forwarder/CLAUDE.md

## About rask-log-forwarder

**rask-log-forwarder** is an ultra-high-performance sidecar container that runs alongside each Alt microservice, tailing its logs, performing zero-copy parsing, and forwarding them in batches. It is built with **Rust 1.87+ (2024 Edition)**, SIMD JSON parsing, and lock-free data structures.

**Core Capabilities:**
-   Zero-copy log collection from the Docker `json-file` driver.
-   SIMD-accelerated JSON parsing (>4 GB/s).
-   Lock-free buffering with `tokio::sync::broadcast`.
-   Guaranteed delivery with exponential backoff and disk fallback (`sled`).

## TDD and Testing Strategy

Development is strictly **Test-Driven**. Our testing strategy is layered to ensure correctness from individual components to the full data pipeline.

### 1. Unit Testing: Parsers

Each log format parser is tested in isolation to ensure it correctly handles valid and malformed inputs.

```rust
#[cfg(test)]
mod tests {
    use super::*;
    use bytes::Bytes;

    #[test]
    fn test_parse_nginx_log_success() {
        let parser = NginxParser::new();
        let log = b"192.168.1.1 - - [01/Jan/2024:00:00:00 +0000] \"GET /api/health HTTP/1.1\" 200 2";
        let result = parser.parse(Bytes::from_static(log));
        assert!(result.is_ok());
        let entry = result.unwrap();
        assert_eq!(entry.fields.get("status_code"), Some(&"200".to_string()));
    }
}
```

### 2. Component Testing: The Collector

We test the `DockerCollector` by mocking the `bollard` Docker client. We use the `mockall` crate to create a mock implementation of the `Docker` trait.

```rust
#[cfg(test)]
mod tests {
    use super::*;
    use mockall::*;
    use futures::stream;

    // Create a mock Docker client
    #[automock]
    trait DockerApi {
        fn logs(&self, container_name: &str) -> Box<dyn Stream<Item = Result<Bytes, Error>> + Unpin>;
    }

    #[tokio::test]
    async fn test_collector_processes_log_stream() {
        // 1. Arrange: Create the mock and define its behavior
        let mut mock_docker = MockDockerApi::new();
        mock_docker.expect_logs()
            .with(eq("test-container"))
            .times(1)
            .returning(|_| Box::new(stream::iter(vec![Ok(Bytes::from("log line 1"))])));

        // 2. Act: Run the collector with the mock
        let collector = DockerCollector::new(mock_docker);
        let (tx, mut rx) = tokio::sync::mpsc::channel(10);
        collector.tail_container("test-container", tx).await.unwrap();

        // 3. Assert: Check that the log was received
        let received = rx.recv().await.unwrap();
        assert_eq!(received, Bytes::from("log line 1"));
    }
}
```

### 3. Integration Testing: The Full Pipeline

We write integration tests for the entire forwarder pipeline. These tests use a mock Docker API to generate logs and a mock HTTP server (`wiremock`) to act as the log aggregator endpoint.

```rust
#[cfg(test)]
mod tests {
    use super::*;
    use wiremock::{
        MockServer,
        Mock,
        ResponseTemplate
    };
    use wiremock::matchers::{method, path};

    #[tokio::test]
    async fn test_end_to_end_forwarding() {
        // 1. Arrange: Start a mock aggregator server
        let mock_server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/v1/aggregate"))
            .respond_with(ResponseTemplate::new(200))
            .mount(&mock_server)
            .await;

        // 2. Arrange: Mock the Docker log stream
        let mock_docker = MockDockerApi::new();
        // ... set expectations to produce a stream of logs ...

        // 3. Act: Run the forwarder
        let config = Config { endpoint: mock_server.uri(), ... };
        let forwarder = LogForwarder::new(config, mock_docker);
        forwarder.run().await;

        // 4. Assert: Verify that the mock server received the logs
        let requests = mock_server.received_requests().await.unwrap();
        assert_eq!(requests.len(), 1);
        assert_eq!(requests[0].body_string().await.unwrap(), "expected_log_batch");
    }
}
```

## Performance Benchmarks

We use `criterion` to benchmark critical paths, such as SIMD JSON parsing and buffer throughput, to prevent performance regressions.

## References

-   [Rust 2024 Edition Guide](https://doc.rust-lang.org/edition-guide/rust-2024/)
-   [Testing with `mockall`](https://crates.io/crates/mockall)
-   [HTTP mocking with `wiremock-rs`](https://crates.io/crates/wiremock)
-   [The Rust Performance Book](https://nnethercote.github.io/perf-book/)
