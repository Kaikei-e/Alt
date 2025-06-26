use rask_log_forwarder::collector::DockerCollector;
use std::process::{Command, Stdio};
use std::time::Duration;
use tokio::time::sleep;

async fn start_test_nginx_container() -> String {
    let output = Command::new("docker")
        .args(&[
            "run", "-d", 
            "--label", "com.alt.log-forward=true",
            "--name", "test-nginx-collector",
            "nginx:alpine"
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

async fn start_test_container_without_label() -> String {
    let output = Command::new("docker")
        .args(&[
            "run", "-d", 
            "--name", "test-nginx-no-label",
            "nginx:alpine"
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

async fn cleanup_test_container(container_id: String) {
    Command::new("docker")
        .args(&["rm", "-f", &container_id])
        .output()
        .expect("Failed to cleanup test container");
}

#[tokio::test]
async fn test_find_nginx_containers_with_label() {
    let collector = DockerCollector::new().await.unwrap();
    
    // Start test nginx container with label
    let test_container = start_test_nginx_container().await;
    
    let containers = collector
        .find_labeled_containers("com.alt.log-forward=true")
        .await
        .unwrap();
    
    assert!(!containers.is_empty(), "Should find labeled containers");
    assert!(
        containers.iter().any(|c| c.image.contains("nginx")),
        "Should find nginx container"
    );
    
    cleanup_test_container(test_container).await;
}

#[tokio::test]
async fn test_filter_containers_without_label() {
    let collector = DockerCollector::new().await.unwrap();
    
    // Start container without the required label
    let test_container = start_test_container_without_label().await;
    
    let containers = collector
        .find_labeled_containers("com.alt.log-forward=true")
        .await
        .unwrap();
    
    assert!(
        !containers.iter().any(|c| c.id == test_container),
        "Should not include containers without label"
    );
    
    cleanup_test_container(test_container).await;
}