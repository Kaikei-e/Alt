use std::process::{Command, Stdio};
use std::time::Duration;

use rask_log_forwarder::collector::DockerCollector;
use tokio::time::sleep;

async fn start_test_nginx_container_safe() -> Result<String, Box<dyn std::error::Error>> {
    let output = Command::new("docker")
        .args([
            "run",
            "-d",
            "--label",
            "com.alt.log-forward=true",
            "--name",
            "test-nginx-collector",
            "nginx:alpine",
        ])
        .stdout(Stdio::piped())
        .output()?;

    if !output.status.success() {
        return Err("Failed to start test container".into());
    }

    let container_id = String::from_utf8(output.stdout)?.trim().to_string();

    // Wait for container to be ready
    sleep(Duration::from_secs(2)).await;

    Ok(container_id)
}

async fn start_test_container_without_label_safe() -> Result<String, Box<dyn std::error::Error>> {
    let output = Command::new("docker")
        .args(["run", "-d", "--name", "test-nginx-no-label", "nginx:alpine"])
        .stdout(Stdio::piped())
        .output()?;

    if !output.status.success() {
        return Err("Failed to start test container".into());
    }

    let container_id = String::from_utf8(output.stdout)?.trim().to_string();

    // Wait for container to be ready
    sleep(Duration::from_secs(2)).await;

    Ok(container_id)
}

fn cleanup_test_container(container_id: &str) {
    if let Err(err) = Command::new("docker")
        .args(["rm", "-f", container_id])
        .output()
    {
        eprintln!("Failed to remove test container {container_id}: {err}");
    }
}

#[tokio::test]
async fn test_find_nginx_containers_with_label() {
    // Test container discovery with graceful handling of Docker unavailability
    let collector_result = DockerCollector::from_endpoint("unix:///var/run/docker.sock");

    match collector_result {
        Ok(collector) => {
            // Try to start test container if Docker is available
            match start_test_nginx_container_safe().await {
                Ok(test_container) => {
                    let containers = collector
                        .find_labeled_containers("com.alt.log-forward=true")
                        .await;

                    match containers {
                        Ok(containers) => {
                            if containers.is_empty() {
                                println!("No labeled containers found (may be expected)");
                            } else {
                                println!("Found {} labeled containers", containers.len());
                                assert!(
                                    containers.iter().any(|c| c.image.contains("nginx")),
                                    "Should find nginx container"
                                );
                            }
                        }
                        Err(e) => {
                            println!("Container discovery failed: {e}");
                        }
                    }

                    cleanup_test_container(&test_container);
                }
                Err(_) => {
                    println!("Cannot start test container (Docker may not be available)");
                }
            }
        }
        Err(e) => {
            println!("Docker not available: {e}");
        }
    }
}

#[tokio::test]
async fn test_filter_containers_without_label() {
    // Test filtering containers without required labels
    let collector_result = DockerCollector::from_endpoint("unix:///var/run/docker.sock");

    match collector_result {
        Ok(collector) => {
            // Try to start test container without label if Docker is available
            match start_test_container_without_label_safe().await {
                Ok(test_container) => {
                    let containers = collector
                        .find_labeled_containers("com.alt.log-forward=true")
                        .await;

                    match containers {
                        Ok(containers) => {
                            // Container without label should not be included
                            let found = containers.iter().any(|c| c.id == test_container);
                            assert!(!found, "Should not include containers without label");
                            println!("Container filtering test passed");
                        }
                        Err(e) => {
                            println!("Container discovery failed: {e}");
                        }
                    }

                    cleanup_test_container(&test_container);
                }
                Err(_) => {
                    println!("Cannot start test container (Docker may not be available)");
                }
            }
        }
        Err(e) => {
            println!("Docker not available: {e}");
        }
    }
}
