//! Log-stream initialization tests for `DockerCollector`.
//!
//! Both tests previously "passed" unconditionally: every branch (Docker
//! unavailable, container failed to start, log tailing failed, log tailing
//! succeeded) only printed a message, so a `start_tailing_logs[_with_options]`
//! that silently did nothing would never fail the test. They now require
//! actual log bytes to arrive on the broadcast channel whenever a real
//! Docker daemon is available, and only fall back to a skip when it isn't
//! (matching the convention in `test_reconnect.rs`).
use bytes::Bytes;
use rask_log_forwarder::collector::{DockerCollector, LogStreamOptions};
use std::process::{Command, Stdio};
use std::time::Duration;
use tokio::sync::broadcast;

fn docker_ok(args: &[&str]) -> bool {
    Command::new("docker")
        .args(args)
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status()
        .map(|s| s.success())
        .unwrap_or(false)
}

fn cleanup(container_name: &str) {
    let _ = Command::new("docker")
        .args(["rm", "-f", container_name])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .output();
}

/// Start a container that continuously emits distinguishable log lines, so a
/// receiver can positively confirm it saw *this test's* traffic rather than
/// leftover noise from another labeled container.
fn start_chatty_container(name: &str) -> bool {
    docker_ok(&[
        "run",
        "-d",
        "--label",
        "com.alt.log-forward=true",
        "--name",
        name,
        "busybox",
        "sh",
        "-c",
        "i=0; while true; do echo \"streaming-test-line $i\"; i=$((i+1)); sleep 0.1; done",
    ])
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
            Ok(_) => continue,
            Err(broadcast::error::TryRecvError::Lagged(_)) => continue,
            Err(broadcast::error::TryRecvError::Closed) => return None,
            Err(broadcast::error::TryRecvError::Empty) => {
                tokio::time::sleep(Duration::from_millis(50)).await;
            }
        }
    }
    None
}

#[tokio::test]
async fn test_nginx_log_stream_initialization() {
    const CONTAINER_NAME: &str = "test-rlf-log-stream-init";
    cleanup(CONTAINER_NAME);

    let collector = match DockerCollector::new().await {
        Ok(c) => c,
        Err(e) => {
            println!("Docker not available, skipping: {e}");
            return;
        }
    };

    if !start_chatty_container(CONTAINER_NAME) {
        println!("Cannot start test container, skipping (Docker may not be available)");
        return;
    }

    let (tx, mut rx) = broadcast::channel::<Bytes>(1000);

    collector
        .start_tailing_logs(tx, "com.alt.log-forward=true")
        .await
        .expect("start_tailing_logs must succeed once a labeled container exists");

    let received = wait_for_log_bytes(&mut rx, Duration::from_secs(10)).await;
    cleanup(CONTAINER_NAME);

    let bytes = received.expect("must receive at least one log chunk from the tailed container");
    let text = String::from_utf8_lossy(&bytes);
    assert!(
        text.contains("streaming-test-line"),
        "received bytes should contain this test's log content, got: {text:?}"
    );
}

#[tokio::test]
async fn test_log_stream_with_options() {
    const CONTAINER_NAME: &str = "test-rlf-log-stream-options";
    cleanup(CONTAINER_NAME);

    let collector = match DockerCollector::new().await {
        Ok(c) => c,
        Err(e) => {
            println!("Docker not available, skipping: {e}");
            return;
        }
    };

    if !start_chatty_container(CONTAINER_NAME) {
        println!("Cannot start test container, skipping (Docker may not be available)");
        return;
    }

    let (tx, mut rx) = broadcast::channel::<Bytes>(1000);

    let options = LogStreamOptions {
        follow: true,
        stdout: true,
        stderr: true,
        timestamps: true,
        tail: "100".to_string(),
    };

    collector
        .start_tailing_logs_with_options(tx, "com.alt.log-forward=true", options)
        .await
        .expect("start_tailing_logs_with_options must succeed once a labeled container exists");

    let received = wait_for_log_bytes(&mut rx, Duration::from_secs(10)).await;
    cleanup(CONTAINER_NAME);

    let bytes = received.expect("must receive at least one log chunk from the tailed container");
    let text = String::from_utf8_lossy(&bytes);
    assert!(
        text.contains("streaming-test-line"),
        "received bytes should contain this test's log content, got: {text:?}"
    );
}
