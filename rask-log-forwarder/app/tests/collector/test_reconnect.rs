//! Regression test for the collector's reconnect-on-disconnect behaviour.
//!
//! Before the fix, `LogCollector::start_collection` gave up permanently as
//! soon as the Docker log stream ended (e.g. because the target container
//! restarted): `start_docker_api_streaming` returned `Ok(())` on stream EOF,
//! and nothing above it ever reconnected. A single container restart was
//! enough to kill log collection for that service for good.
use std::sync::Arc;
use std::time::Duration;

use rask_log_forwarder::collector::{CollectorConfig, LogCollector};
use serial_test::serial;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use super::docker_fixture::{TestContainerGuard, docker_ok, unique_container_name};

#[tokio::test]
#[serial(docker)]
async fn collector_reconnects_after_container_restart() {
    let container_name = unique_container_name("reconnect");
    let Some(guard) = TestContainerGuard::start(
        container_name,
        None,
        "busybox",
        &[
            "sh",
            "-c",
            "i=0; while true; do echo \"line $i\"; i=$((i+1)); sleep 0.2; done",
        ],
    ) else {
        println!("Docker not available, skipping collector reconnect integration test");
        return;
    };

    // Give the container a moment to start emitting logs.
    tokio::time::sleep(Duration::from_millis(500)).await;

    let config = CollectorConfig {
        auto_discover: false,
        target_service: Some(guard.name().to_string()),
        follow_rotations: true,
        buffer_size: 8192,
    };

    let collector = match LogCollector::new_with_env(
        config,
        Arc::new(|k| (k == "DOCKER_HOST").then(|| "unix:///var/run/docker.sock".to_string())),
    ) {
        Ok(c) => c,
        Err(e) => {
            println!(
                "LogCollector::new_with_env failed ({e}), skipping (Docker may be unavailable)"
            );
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
    assert!(
        docker_ok(&["restart", "-t", "1", guard.name()]),
        "docker restart failed for test container {}",
        guard.name()
    );

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
    if let Err(err) = tokio::time::timeout(Duration::from_secs(5), handle).await {
        eprintln!("Collector task did not stop within timeout: {err}");
    }
    drop(guard);

    assert!(
        windows_with_data >= 2,
        "collector must keep receiving log entries across multiple post-restart windows \
         (reconnect loop), not go permanently silent after a single container restart \
         (got data in {windows_with_data}/4 windows)"
    );
}
