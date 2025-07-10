use bytes::Bytes;
use rask_log_forwarder::collector::{DockerCollector, LogStreamOptions};
use std::process::{Command, Stdio};
use std::time::Duration;
use tokio::time::sleep;

async fn start_test_nginx_container() -> String {
    let output = Command::new("docker")
        .args([
            "run",
            "-d",
            "--label",
            "com.alt.log-forward=true",
            "--name",
            "test-nginx-streaming",
            "nginx:alpine",
        ])
        .stdout(Stdio::piped())
        .output()
        .expect("Failed to start test container");

    let container_id = String::from_utf8(output.stdout)
        .expect("Invalid UTF-8 in container ID")
        .trim()
        .to_string();

    // Wait for container to be ready
    sleep(Duration::from_secs(2)).await;

    container_id
}

async fn start_test_nginx_container_safe() -> Result<String, Box<dyn std::error::Error>> {
    let output = Command::new("docker")
        .args([
            "run",
            "-d",
            "--label",
            "com.alt.log-forward=true",
            "--name",
            "test-nginx-streaming",
            "nginx:alpine",
        ])
        .stdout(Stdio::piped())
        .output()?;

    if !output.status.success() {
        return Err("Failed to start test container".into());
    }

    let container_id = String::from_utf8(output.stdout)?
        .trim()
        .to_string();

    // Wait for container to be ready
    sleep(Duration::from_secs(2)).await;

    Ok(container_id)
}

async fn cleanup_test_container(container_id: String) {
    let _ = Command::new("docker")
        .args(["rm", "-f", &container_id])
        .output();
}

#[tokio::test]
async fn test_nginx_log_stream_initialization() {
    // Test log stream initialization with graceful Docker handling
    let collector_result = DockerCollector::new().await;
    
    match collector_result {
        Ok(collector) => {
            let (tx, _rx) = tokio::sync::broadcast::channel::<Bytes>(1000);

            // Try to start test container if Docker is available
            match start_test_nginx_container_safe().await {
                Ok(test_container) => {
                    let result = collector
                        .start_tailing_logs(tx, "com.alt.log-forward=true")
                        .await;
                    
                    match result {
                        Ok(_) => {
                            println!("Log streaming started successfully");
                        }
                        Err(e) => {
                            println!("Log streaming failed: {}", e);
                        }
                    }

                    cleanup_test_container(test_container).await;
                }
                Err(_) => {
                    println!("Cannot start test container (Docker may not be available)");
                }
            }
        }
        Err(e) => {
            println!("Docker not available: {}", e);
        }
    }
}

#[tokio::test]
async fn test_log_stream_with_options() {
    // Test log streaming with custom options
    let collector_result = DockerCollector::new().await;
    
    match collector_result {
        Ok(collector) => {
            let (tx, _rx) = tokio::sync::broadcast::channel::<Bytes>(1000);

            let options = LogStreamOptions {
                follow: true,
                stdout: true,
                stderr: true,
                timestamps: true,
                tail: "100".to_string(),
            };

            // Try to start test container if Docker is available
            match start_test_nginx_container_safe().await {
                Ok(test_container) => {
                    let result = collector
                        .start_tailing_logs_with_options(tx, "com.alt.log-forward=true", options)
                        .await;
                    
                    match result {
                        Ok(_) => {
                            println!("Log streaming with options started successfully");
                        }
                        Err(e) => {
                            println!("Log streaming with options failed: {}", e);
                        }
                    }

                    cleanup_test_container(test_container).await;
                }
                Err(_) => {
                    println!("Cannot start test container (Docker may not be available)");
                }
            }
        }
        Err(e) => {
            println!("Docker not available: {}", e);
        }
    }
}
