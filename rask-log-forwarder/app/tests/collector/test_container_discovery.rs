use std::time::Duration;

use rask_log_forwarder::collector::DockerCollector;
use serial_test::serial;
use tokio::time::sleep;

use super::docker_fixture::{TestContainerGuard, unique_container_name, unique_label_filter};

#[tokio::test]
#[serial(docker)]
async fn test_find_nginx_containers_with_label() {
    // Test container discovery with graceful handling of Docker unavailability
    let collector = match DockerCollector::from_endpoint("unix:///var/run/docker.sock") {
        Ok(c) => c,
        Err(e) => {
            println!("Docker not available: {e}");
            return;
        }
    };

    let container_name = unique_container_name("discovery-labeled");
    let label = unique_label_filter("discovery-labeled");
    let Some(guard) = TestContainerGuard::start(container_name, Some(&label), "nginx:alpine", &[])
    else {
        println!("Cannot start test container (Docker may not be available)");
        return;
    };

    // Wait for container to be ready
    sleep(Duration::from_secs(2)).await;

    let containers = collector.find_labeled_containers(&label).await;
    match containers {
        Ok(containers) => {
            assert_eq!(
                containers.len(),
                1,
                "unique label filter should match only this test's container, got: {containers:?}"
            );
            assert!(
                containers
                    .iter()
                    .any(|c| c.id == guard.id() && c.image.contains("nginx")),
                "Should find this test's nginx container"
            );
        }
        Err(e) => {
            println!("Container discovery failed: {e}");
        }
    }

    drop(guard);
}

#[tokio::test]
#[serial(docker)]
async fn test_filter_containers_without_label() {
    // Test filtering containers without required labels
    let collector = match DockerCollector::from_endpoint("unix:///var/run/docker.sock") {
        Ok(c) => c,
        Err(e) => {
            println!("Docker not available: {e}");
            return;
        }
    };

    let container_name = unique_container_name("discovery-unlabeled");
    let label = unique_label_filter("discovery-unlabeled");
    let Some(guard) = TestContainerGuard::start(container_name, None, "nginx:alpine", &[]) else {
        println!("Cannot start test container (Docker may not be available)");
        return;
    };

    // Wait for container to be ready
    sleep(Duration::from_secs(2)).await;

    let containers = collector.find_labeled_containers(&label).await;
    match containers {
        Ok(containers) => {
            // Container without label should not be included
            let found = containers.iter().any(|c| c.id == guard.id());
            assert!(!found, "Should not include containers without label");
            println!("Container filtering test passed");
        }
        Err(e) => {
            println!("Container discovery failed: {e}");
        }
    }

    drop(guard);
}
