use bytes::Bytes;
use chrono::{DateTime, Duration, Utc};
use rask_log_forwarder::collector::{
    DedupeFilter, DockerCollector, DockerEndpointError, ServiceDiscovery,
    compute_resume_log_options, get_validated_docker_endpoint, parse_log_timestamp_prefix,
    validate_docker_endpoint,
};
use serial_test::serial;

#[test]
fn test_validate_docker_endpoint_valid_tcp() {
    let endpoint = "tcp://docker-socket-proxy-ro:2375";
    assert_eq!(validate_docker_endpoint(endpoint), Ok(endpoint));
}

#[test]
fn test_validate_docker_endpoint_valid_tcp_with_ip() {
    let endpoint = "tcp://127.0.0.1:2375";
    assert_eq!(validate_docker_endpoint(endpoint), Ok(endpoint));
}

#[test]
fn test_validate_docker_endpoint_valid_unix() {
    let endpoint = "unix:///var/run/docker.sock";
    assert_eq!(validate_docker_endpoint(endpoint), Ok(endpoint));
}

#[test]
fn test_validate_docker_endpoint_valid_http() {
    let endpoint = "http://127.0.0.1:2375";
    assert_eq!(validate_docker_endpoint(endpoint), Ok(endpoint));
}

#[test]
fn test_validate_docker_endpoint_valid_https() {
    let endpoint = "https://127.0.0.1:2376";
    assert_eq!(validate_docker_endpoint(endpoint), Ok(endpoint));
}

#[test]
fn test_validate_docker_endpoint_trims_whitespace() {
    let endpoint = "   tcp://docker-socket-proxy-ro:2375   ";
    assert_eq!(
        validate_docker_endpoint(endpoint),
        Ok("tcp://docker-socket-proxy-ro:2375")
    );
}

#[test]
fn test_validate_docker_endpoint_empty_fails() {
    assert_eq!(
        validate_docker_endpoint(""),
        Err(DockerEndpointError::EmptyDockerHost)
    );
    assert_eq!(
        validate_docker_endpoint("   "),
        Err(DockerEndpointError::EmptyDockerHost)
    );
}

#[test]
fn test_validate_docker_endpoint_unsupported_scheme_ftp() {
    let res = validate_docker_endpoint("ftp://docker:2375");
    assert_eq!(
        res,
        Err(DockerEndpointError::UnsupportedScheme(
            "ftp://docker:2375".to_string()
        ))
    );
}

#[test]
fn test_validate_docker_endpoint_unsupported_scheme_ssh() {
    let res = validate_docker_endpoint("ssh://root@localhost");
    assert_eq!(
        res,
        Err(DockerEndpointError::UnsupportedScheme(
            "ssh://root@localhost".to_string()
        ))
    );
}

#[test]
fn test_validate_docker_endpoint_missing_host_after_scheme() {
    let res = validate_docker_endpoint("tcp://");
    assert!(matches!(res, Err(DockerEndpointError::InvalidUrl(..))));

    let res = validate_docker_endpoint("unix://");
    assert!(matches!(res, Err(DockerEndpointError::InvalidUrl(..))));

    let res = validate_docker_endpoint("http://");
    assert!(matches!(res, Err(DockerEndpointError::InvalidUrl(..))));

    let res = validate_docker_endpoint("https://");
    assert!(matches!(res, Err(DockerEndpointError::InvalidUrl(..))));
}

#[test]
fn test_validate_docker_endpoint_missing_host_port_only() {
    let res = validate_docker_endpoint("tcp://:2375");
    assert!(matches!(res, Err(DockerEndpointError::InvalidUrl(..))));
}

#[test]
fn test_parse_log_timestamp_prefix_with_nanos() {
    let raw = Bytes::from("2026-09-20T08:00:00.123456789Z test log line with nanos\n");
    let (ts, stripped) = parse_log_timestamp_prefix(raw);

    assert!(ts.is_some());
    let parsed_ts = ts.unwrap();
    assert_eq!(parsed_ts.timestamp(), 1789891200);
    assert_eq!(parsed_ts.timestamp_subsec_nanos(), 123456789);
    assert_eq!(stripped, Bytes::from("test log line with nanos\n"));
}

#[test]
fn test_parse_log_timestamp_prefix_without_nanos() {
    let raw = Bytes::from("2026-09-20T08:00:00Z test log line without nanos\n");
    let (ts, stripped) = parse_log_timestamp_prefix(raw);

    assert!(ts.is_some());
    let parsed_ts = ts.unwrap();
    assert_eq!(parsed_ts.timestamp(), 1789891200);
    assert_eq!(parsed_ts.timestamp_subsec_nanos(), 0);
    assert_eq!(stripped, Bytes::from("test log line without nanos\n"));
}

#[test]
fn test_parse_log_timestamp_prefix_two_line_frame() {
    let raw = Bytes::from(
        "2026-09-20T08:00:00.100000000Z first line\n2026-09-20T08:00:01.200000000Z second line\n",
    );
    let (ts, stripped) = parse_log_timestamp_prefix(raw);

    assert!(ts.is_some());
    let parsed_ts = ts.unwrap();
    assert_eq!(parsed_ts.timestamp(), 1789891201);
    assert_eq!(parsed_ts.timestamp_subsec_nanos(), 200000000);
    assert_eq!(stripped, Bytes::from("first line\nsecond line\n"));
}

#[test]
fn test_parse_log_timestamp_prefix_malformed_prefix_forwarded_unchanged() {
    let raw = Bytes::from("not-a-timestamp test log line\n");
    let (ts, stripped) = parse_log_timestamp_prefix(raw.clone());

    assert!(ts.is_none());
    assert_eq!(stripped, raw);
}

#[test]
fn test_parse_log_timestamp_prefix_no_space_forwarded_unchanged() {
    let raw = Bytes::from("no_space_line");
    let (ts, stripped) = parse_log_timestamp_prefix(raw.clone());

    assert!(ts.is_none());
    assert_eq!(stripped, raw);
}

#[test]
fn test_compute_resume_log_options() {
    // Initial connection: last_timestamp is None -> tail "0", since 0, timestamps true
    let initial = compute_resume_log_options(None);
    assert_eq!(initial.tail, "0");
    assert_eq!(initial.since, 0);
    assert!(initial.follow);
    assert!(initial.stdout);
    assert!(initial.stderr);
    assert!(initial.timestamps);

    // Reconnection: last_timestamp is Some(ts) -> tail "all", since floor(ts) as i32, timestamps true
    let ts = DateTime::parse_from_rfc3339("2026-09-20T08:00:00.987654321Z")
        .unwrap()
        .with_timezone(&Utc);
    let resumed = compute_resume_log_options(Some(ts));
    assert_eq!(resumed.tail, "all");
    assert_eq!(resumed.since, ts.timestamp() as i32);
    assert!(resumed.follow);
    assert!(resumed.stdout);
    assert!(resumed.stderr);
    assert!(resumed.timestamps);
}

#[test]
fn test_dedupe_filter_boundary_dedupe() {
    let ts = DateTime::parse_from_rfc3339("2026-09-20T08:00:00.500000000Z")
        .unwrap()
        .with_timezone(&Utc);

    let mut filter = DedupeFilter::new(Some(ts));
    assert!(filter.is_active());

    // 1. Line at ts (exact match of boundary timestamp) -> must be dropped
    assert!(filter.should_drop(Some(ts)), "line at ts must be dropped");

    // 2. Line at ts - 1s (older line within boundary second) -> must be dropped
    let older = ts - Duration::seconds(1);
    assert!(
        filter.should_drop(Some(older)),
        "line at ts - 1s must be dropped"
    );

    // 3. Line at ts + 1ns (first newer line) -> must be forwarded (should_drop is false)
    let newer = ts + Duration::nanoseconds(1);
    assert!(
        !filter.should_drop(Some(newer)),
        "line at ts + 1ns must be forwarded"
    );

    // Filter should now be inactive after encountering the first newer line
    assert!(!filter.is_active());

    // Subsequent lines should all be forwarded
    assert!(
        !filter.should_drop(Some(ts)),
        "subsequent lines must be forwarded once filter deactivated"
    );
}

#[test]
fn test_dedupe_filter_inactive_on_none() {
    let mut filter = DedupeFilter::new(None);
    assert!(!filter.is_active());

    let ts = DateTime::parse_from_rfc3339("2026-09-20T08:00:00.500000000Z")
        .unwrap()
        .with_timezone(&Utc);

    // When no prior timestamp exists, no lines are dropped
    assert!(!filter.should_drop(Some(ts)));
    assert!(!filter.should_drop(None));
}

#[test]
fn test_dedupe_filter_malformed_timestamp_forwarded() {
    let ts = DateTime::parse_from_rfc3339("2026-09-20T08:00:00.500000000Z")
        .unwrap()
        .with_timezone(&Utc);

    let mut filter = DedupeFilter::new(Some(ts));
    assert!(filter.is_active());

    // Malformed line (None timestamp) is not dropped
    assert!(!filter.should_drop(None));
}

#[test]
#[serial]
fn test_get_validated_docker_endpoint_from_env() {
    // Valid env var
    unsafe {
        std::env::set_var("DOCKER_HOST", "tcp://docker-socket-proxy-ro:2375");
    }
    assert_eq!(
        get_validated_docker_endpoint(),
        Ok("tcp://docker-socket-proxy-ro:2375".to_string())
    );

    // Empty env var
    unsafe {
        std::env::set_var("DOCKER_HOST", "");
    }
    assert_eq!(
        get_validated_docker_endpoint(),
        Err(DockerEndpointError::EmptyDockerHost)
    );

    // Unset env var
    unsafe {
        std::env::remove_var("DOCKER_HOST");
    }
    assert_eq!(
        get_validated_docker_endpoint(),
        Err(DockerEndpointError::MissingDockerHost)
    );
}

#[tokio::test]
#[serial]
async fn test_collector_and_discovery_fail_when_docker_host_unset() {
    unsafe {
        std::env::remove_var("DOCKER_HOST");
    }

    // DockerCollector::new() must fail startup loudly when DOCKER_HOST is unset
    let collector_result = DockerCollector::new().await;
    assert!(
        collector_result.is_err(),
        "DockerCollector::new() must fail when DOCKER_HOST is unset"
    );

    // ServiceDiscovery::new() must also fail startup loudly when DOCKER_HOST is unset
    let discovery_result = ServiceDiscovery::new().await;
    assert!(
        discovery_result.is_err(),
        "ServiceDiscovery::new() must fail when DOCKER_HOST is unset"
    );
}

#[tokio::test]
#[serial]
async fn test_collector_and_discovery_fail_on_unsupported_scheme() {
    unsafe {
        std::env::set_var("DOCKER_HOST", "ftp://docker-socket-proxy-ro:2375");
    }

    let collector_result = DockerCollector::new().await;
    assert!(
        collector_result.is_err(),
        "DockerCollector::new() must fail on unsupported scheme"
    );

    let discovery_result = ServiceDiscovery::new().await;
    assert!(
        discovery_result.is_err(),
        "ServiceDiscovery::new() must fail on unsupported scheme"
    );

    // Clean up
    unsafe {
        std::env::remove_var("DOCKER_HOST");
    }
}

#[tokio::test]
#[serial]
async fn test_collector_and_discovery_succeed_construction_with_valid_tcp() {
    unsafe {
        std::env::set_var("DOCKER_HOST", "tcp://docker-socket-proxy-ro:2375");
    }

    // Initializing the client object succeeds without pinging the daemon
    let collector_result = DockerCollector::new().await;
    assert!(
        collector_result.is_ok(),
        "DockerCollector::new() must succeed client creation with valid tcp endpoint: {:?}",
        collector_result.err()
    );

    let discovery_result = ServiceDiscovery::new().await;
    assert!(
        discovery_result.is_ok(),
        "ServiceDiscovery::new() must succeed client creation with valid tcp endpoint: {:?}",
        discovery_result.err()
    );

    // Clean up
    unsafe {
        std::env::remove_var("DOCKER_HOST");
    }
}

#[tokio::test]
#[serial]
async fn test_collector_and_discovery_succeed_construction_with_valid_unix() {
    unsafe {
        std::env::set_var("DOCKER_HOST", "unix:///var/run/docker.sock");
    }

    let collector_result = DockerCollector::new().await;
    assert!(
        collector_result.is_ok(),
        "DockerCollector::new() must succeed client creation with valid unix endpoint: {:?}",
        collector_result.err()
    );

    let discovery_result = ServiceDiscovery::new().await;
    assert!(
        discovery_result.is_ok(),
        "ServiceDiscovery::new() must succeed client creation with valid unix endpoint: {:?}",
        discovery_result.err()
    );

    // Clean up
    unsafe {
        std::env::remove_var("DOCKER_HOST");
    }
}
