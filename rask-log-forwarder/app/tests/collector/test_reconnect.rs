//! Regression test for the collector's reconnect-on-disconnect behaviour.
//!
//! Before the fix, `LogCollector::start_collection` gave up permanently as
//! soon as the Docker log stream ended (e.g. because the target container
//! restarted): `start_docker_api_streaming` returned `Ok(())` on stream EOF,
//! and nothing above it ever reconnected. A single container restart was
//! enough to kill log collection for that service for good.
use rask_log_forwarder::collector::{CollectorConfig, LogCollector};
use std::process::{Command, Stdio};
use std::time::Duration;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

const CONTAINER_NAME: &str = "test-rlf-reconnect";

fn docker_ok(args: &[&str]) -> bool {
    Command::new("docker")
        .args(args)
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status()
        .map(|s| s.success())
        .unwrap_or(false)
}

fn cleanup() {
    let _ = Command::new("docker")
        .args(["rm", "-f", CONTAINER_NAME])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .output();
}

#[tokio::test]
async fn collector_reconnects_after_container_restart() {
    cleanup();

    let started = docker_ok(&[
        "run",
        "-d",
        "--name",
        CONTAINER_NAME,
        "busybox",
        "sh",
        "-c",
        "i=0; while true; do echo \"line $i\"; i=$((i+1)); sleep 0.2; done",
    ]);

    if !started {
        println!("Docker not available, skipping collector reconnect integration test");
        return;
    }

    // Give the container a moment to start emitting logs.
    tokio::time::sleep(Duration::from_millis(500)).await;

    let config = CollectorConfig {
        auto_discover: false,
        target_service: Some(CONTAINER_NAME.to_string()),
        follow_rotations: true,
        buffer_size: 8192,
    };

    let collector = match LogCollector::new(config).await {
        Ok(c) => c,
        Err(e) => {
            println!("LogCollector::new failed ({e}), skipping (Docker may be unavailable)");
            cleanup();
            return;
        }
    };

    let (tx, mut rx) = mpsc::channel(1024);
    let cancel_token = CancellationToken::new();
    let collection_cancel = cancel_token.clone();

    let handle = tokio::spawn(async move {
        let mut collector = collector;
        collector.start_collection(tx, collection_cancel).await
    });

    // Confirm we receive log entries from the first connection.
    let first = tokio::time::timeout(Duration::from_secs(10), rx.recv()).await;
    assert!(
        matches!(first, Ok(Some(_))),
        "expected to receive log entries before the restart"
    );

    // Drain whatever is already buffered so the post-restart check below
    // can't be satisfied by stale pre-restart backlog.
    while rx.try_recv().is_ok() {}

    // Restarting the container ends the current Docker log stream (EOF from
    // the collector's point of view). Before the fix, the collector would
    // stop here permanently: `start_docker_api_streaming` returned `Ok(())`
    // and its `tx` was dropped, closing the channel for good.
    docker_ok(&["restart", "-t", "1", CONTAINER_NAME]);

    // Require entries to keep arriving across several distinct windows well
    // after the restart. A lucky leftover-backlog burst (already ruled out
    // above) could satisfy one window; sustained arrival across multiple
    // windows can only happen if the collector actually reconnected.
    let mut windows_with_data = 0;
    for _ in 0..4 {
        match tokio::time::timeout(Duration::from_secs(5), rx.recv()).await {
            Ok(Some(_)) => {
                windows_with_data += 1;
                // Drain the rest of this window's backlog without blocking.
                while rx.try_recv().is_ok() {}
            }
            Ok(None) => break, // channel closed: collector gave up permanently
            Err(_) => {}       // no entry in this window; keep trying
        }
    }

    cancel_token.cancel();
    let _ = tokio::time::timeout(Duration::from_secs(5), handle).await;
    cleanup();

    assert!(
        windows_with_data >= 2,
        "collector must keep receiving log entries across multiple post-restart windows \
         (reconnect loop), not go permanently silent after a single container restart \
         (got data in {windows_with_data}/4 windows)"
    );
}
