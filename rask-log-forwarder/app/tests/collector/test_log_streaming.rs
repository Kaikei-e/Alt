//! Log-stream initialization tests for `DockerCollector`.
//!
//! Both tests previously "passed" unconditionally: every branch (Docker
//! unavailable, container failed to start, log tailing failed, log tailing
//! succeeded) only printed a message, so a `start_tailing_logs[_with_options]`
//! that silently did nothing would never fail the test. They now require
//! actual log bytes to arrive on the broadcast channel whenever a real
//! Docker daemon is available, and only fall back to a skip when it isn't
//! (matching the convention in `test_reconnect.rs`).
//!
//! Each test uses a unique PID-scoped label filter via `docker_fixture` and
//! `#[serial(docker)]` so containers from other collector tests (or a live
//! host stack) can never contaminate its broadcast stream.
use std::time::Duration;

use bytes::Bytes;
use rask_log_forwarder::collector::{DockerCollector, LogStreamOptions};
use serial_test::serial;
use tokio::sync::broadcast;

use super::docker_fixture::{TestContainerGuard, unique_container_name, unique_label_filter};

/// Start a container that continuously emits distinguishable log lines, so a
/// receiver can positively confirm it saw *this test's* traffic rather than
/// leftover noise from another labeled container.
fn start_chatty_container(name: String, label: &str) -> Option<TestContainerGuard> {
    TestContainerGuard::start(
        name,
        Some(label),
        "busybox",
        &[
            "sh",
            "-c",
            "i=0; while true; do echo \"streaming-test-line $i\"; i=$((i+1)); sleep 0.1; done",
        ],
    )
}

/// Poll the broadcast receiver until non-empty bytes arrive or `timeout` elapses.
async fn wait_for_log_bytes(
    rx: &mut broadcast::Receiver<Bytes>,
    timeout: Duration,
) -> Option<Bytes> {
    let deadline = tokio::time::Instant::now() + timeout;
    while tokio::time::Instant::now() < deadline {
        match rx.try_recv() {
            Ok(bytes) if !bytes.is_empty() => return Some(bytes),
            Ok(_) | Err(broadcast::error::TryRecvError::Lagged(_)) => {}
            Err(broadcast::error::TryRecvError::Closed) => return None,
            Err(broadcast::error::TryRecvError::Empty) => {
                tokio::time::sleep(Duration::from_millis(50)).await;
            }
        }
    }
    None
}

#[tokio::test]
#[serial(docker)]
async fn test_nginx_log_stream_initialization() {
    let collector = match DockerCollector::from_endpoint("unix:///var/run/docker.sock") {
        Ok(c) => c,
        Err(e) => {
            println!("Docker not available, skipping: {e}");
            return;
        }
    };

    let container_name = unique_container_name("log-stream-init");
    let label = unique_label_filter("log-stream-init");
    let Some(guard) = start_chatty_container(container_name, &label) else {
        println!("Cannot start test container, skipping (Docker may not be available)");
        return;
    };

    let (tx, mut rx) = broadcast::channel::<Bytes>(1000);

    collector
        .start_tailing_logs(tx, &label)
        .await
        .expect("start_tailing_logs must succeed once a labeled container exists");

    let received = wait_for_log_bytes(&mut rx, Duration::from_secs(10)).await;
    drop(guard);

    let bytes = received.expect("must receive at least one log chunk from the tailed container");
    let text = String::from_utf8_lossy(&bytes);
    assert!(
        text.contains("streaming-test-line"),
        "received bytes should contain this test's log content, got: {text:?}"
    );
}

#[tokio::test]
#[serial(docker)]
async fn test_log_stream_with_options() {
    let collector = match DockerCollector::from_endpoint("unix:///var/run/docker.sock") {
        Ok(c) => c,
        Err(e) => {
            println!("Docker not available, skipping: {e}");
            return;
        }
    };

    let container_name = unique_container_name("log-stream-options");
    let label = unique_label_filter("log-stream-options");
    let Some(guard) = start_chatty_container(container_name, &label) else {
        println!("Cannot start test container, skipping (Docker may not be available)");
        return;
    };

    let (tx, mut rx) = broadcast::channel::<Bytes>(1000);

    let options = LogStreamOptions {
        follow: true,
        stdout: true,
        stderr: true,
        timestamps: true,
        tail: "100".to_string(),
        since: 0,
    };

    collector
        .start_tailing_logs_with_options(tx, &label, options)
        .await
        .expect("start_tailing_logs_with_options must succeed once a labeled container exists");

    let received = wait_for_log_bytes(&mut rx, Duration::from_secs(10)).await;
    drop(guard);

    let bytes = received.expect("must receive at least one log chunk from the tailed container");
    let text = String::from_utf8_lossy(&bytes);
    assert!(
        text.contains("streaming-test-line"),
        "received bytes should contain this test's log content, got: {text:?}"
    );
}
