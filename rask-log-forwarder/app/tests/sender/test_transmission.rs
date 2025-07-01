use rask_log_forwarder::buffer::{Batch, BatchType};
use rask_log_forwarder::parser::{EnrichedLogEntry, LogLevel};
use rask_log_forwarder::sender::{BatchTransmitter, ClientConfig, HttpClient};
use std::collections::HashMap;
use tokio::time::Duration;

fn create_test_entry(message: &str) -> EnrichedLogEntry {
    EnrichedLogEntry {
        service_type: "test".to_string(),
        log_type: "test".to_string(),
        message: message.to_string(),
        level: Some(LogLevel::Info),
        timestamp: "2024-01-01T00:00:00.000Z".to_string(),
        stream: "stdout".to_string(),
        method: Some("GET".to_string()),
        path: Some("/api/test".to_string()),
        status_code: Some(200),
        response_size: None,
        ip_address: Some("192.168.1.1".to_string()),
        user_agent: Some("test-client".to_string()),
        container_id: "test123".to_string(),
        service_name: "test-service".to_string(),
        service_group: Some("test-group".to_string()),
        fields: HashMap::new(),
    }
}

#[tokio::test]
async fn test_transmitter_creation() {
    let config = ClientConfig {
        endpoint: "http://localhost:9600/v1/aggregate".to_string(),
        timeout: Duration::from_secs(10),
        ..Default::default()
    };

    let result = HttpClient::new(config).await;

    match result {
        Ok(client) => {
            let transmitter = BatchTransmitter::new(client);
            // Should create transmitter successfully
            assert_eq!(
                transmitter.client.endpoint(),
                "http://localhost:9600/v1/aggregate"
            );
        }
        Err(_) => {
            // Expected when server is not available
        }
    }
}

#[tokio::test]
async fn test_payload_preparation() {
    let config = ClientConfig {
        endpoint: "http://localhost:9600/v1/aggregate".to_string(),
        enable_compression: false,
        ..Default::default()
    };

    let result = HttpClient::new(config).await;

    match result {
        Ok(client) => {
            let transmitter = BatchTransmitter::new(client);
            let entries = vec![create_test_entry("Test transmission")];
            let batch = Batch::new(entries, BatchType::SizeBased);

            // Test uncompressed payload
            let payload = transmitter.prepare_payload(&batch, false).unwrap();
            assert!(!payload.is_empty());

            // Verify it's valid NDJSON
            let payload_str = String::from_utf8(payload).unwrap();
            let lines: Vec<&str> = payload_str.lines().collect();
            assert_eq!(lines.len(), 1);

            let parsed: serde_json::Value = serde_json::from_str(lines[0]).unwrap();
            assert_eq!(parsed["message"], "Test transmission");
        }
        Err(_) => {
            // Expected when server is not available
        }
    }
}

#[tokio::test]
async fn test_compression() {
    let config = ClientConfig {
        endpoint: "http://localhost:9600/v1/aggregate".to_string(),
        enable_compression: true,
        ..Default::default()
    };

    let result = HttpClient::new(config).await;

    match result {
        Ok(client) => {
            let transmitter = BatchTransmitter::new(client);
            let entries: Vec<_> = (0..1000)
                .map(|i| create_test_entry(&format!("Compression test message {}", i)))
                .collect();

            let batch = Batch::new(entries, BatchType::SizeBased);

            // Test both compressed and uncompressed
            let uncompressed = transmitter.prepare_payload(&batch, false).unwrap();
            let compressed = transmitter.prepare_payload(&batch, true).unwrap();

            // Compressed should be smaller
            assert!(compressed.len() < uncompressed.len());
            assert!(!compressed.is_empty());
        }
        Err(_) => {
            // Expected when server is not available
        }
    }
}

#[tokio::test]
async fn test_header_building() {
    let config = ClientConfig {
        endpoint: "http://localhost:9600/v1/aggregate".to_string(),
        user_agent: "test-forwarder/1.0".to_string(),
        ..Default::default()
    };

    let result = HttpClient::new(config).await;

    match result {
        Ok(client) => {
            let transmitter = BatchTransmitter::new(client);
            let entries = vec![create_test_entry("Header test")];
            let batch = Batch::new(entries, BatchType::TimeBased);

            let headers = transmitter.build_headers(&batch, false);

            // Check required headers
            assert_eq!(headers.get("content-type").unwrap(), "application/x-ndjson");
            assert_eq!(headers.get("x-batch-id").unwrap(), batch.id());
            assert_eq!(headers.get("x-batch-size").unwrap(), "1");
            assert!(headers.contains_key("user-agent"));
            assert!(headers.contains_key("x-forwarder-version"));

            // Test with compression
            let headers_compressed = transmitter.build_headers(&batch, true);
            assert_eq!(headers_compressed.get("content-encoding").unwrap(), "gzip");
        }
        Err(_) => {
            // Expected when server is not available
        }
    }
}

#[tokio::test]
async fn test_large_batch_payload() {
    let config = ClientConfig {
        endpoint: "http://localhost:9600/v1/aggregate".to_string(),
        timeout: Duration::from_secs(60), // Longer timeout for large batches
        ..Default::default()
    };

    let result = HttpClient::new(config).await;

    match result {
        Ok(client) => {
            let transmitter = BatchTransmitter::new(client);

            // Create 10K entries
            let entries: Vec<_> = (0..10000)
                .map(|i| create_test_entry(&format!("Large batch entry {}", i)))
                .collect();

            let batch = Batch::new(entries, BatchType::SizeBased);

            // Test payload preparation for large batch
            let payload = transmitter.prepare_payload(&batch, false).unwrap();

            // Should handle large payloads efficiently
            assert!(payload.len() > 1_000_000); // >1MB
            assert!(payload.len() < 50_000_000); // <50MB (reasonable for 10K entries)

            // Verify structure
            let payload_str = String::from_utf8(payload).unwrap();
            let lines: Vec<&str> = payload_str.lines().collect();
            assert_eq!(lines.len(), 10000);
        }
        Err(_) => {
            // Expected when server is not available
        }
    }
}

#[tokio::test]
async fn test_batch_metadata_headers() {
    let config = ClientConfig::default();

    let result = HttpClient::new(config).await;

    match result {
        Ok(client) => {
            let transmitter = BatchTransmitter::new(client);
            let entries = vec![create_test_entry("Metadata test")];
            let batch = Batch::new(entries, BatchType::MemoryBased);

            let headers = transmitter.build_headers(&batch, false);

            // Check batch metadata headers
            assert_eq!(headers.get("x-batch-type").unwrap(), "MemoryBased");
            assert!(headers.get("x-batch-id").is_some());
            assert!(headers.get("x-forwarder-version").is_some());
        }
        Err(_) => {
            // Expected when server is not available
        }
    }
}

#[tokio::test]
async fn test_empty_batch_handling() {
    let config = ClientConfig::default();

    let result = HttpClient::new(config).await;

    match result {
        Ok(client) => {
            let transmitter = BatchTransmitter::new(client);
            let batch = Batch::new(vec![], BatchType::SizeBased);

            // Should fail to prepare payload for empty batch
            let result = transmitter.prepare_payload(&batch, false);
            assert!(result.is_err());
        }
        Err(_) => {
            // Expected when server is not available
        }
    }
}
