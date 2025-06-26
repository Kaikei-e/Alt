use rask_log_forwarder::reliability::{RetryManager, RetryConfig, RetryStrategy};
use std::time::Duration;

#[tokio::test]
async fn test_exponential_backoff_timing() {
    let config = RetryConfig {
        max_attempts: 5,
        base_delay: Duration::from_millis(100),
        max_delay: Duration::from_secs(30),
        strategy: RetryStrategy::ExponentialBackoff,
        jitter: false, // Disable jitter for predictable testing
    };
    
    let retry_manager = RetryManager::new(config);
    
    // Test backoff delays
    let delays = (0..5).map(|attempt| {
        retry_manager.calculate_delay(attempt)
    }).collect::<Vec<_>>();
    
    assert_eq!(delays[0], Duration::from_millis(100));  // 100ms
    assert_eq!(delays[1], Duration::from_millis(200));  // 200ms
    assert_eq!(delays[2], Duration::from_millis(400));  // 400ms
    assert_eq!(delays[3], Duration::from_millis(800));  // 800ms
    assert_eq!(delays[4], Duration::from_millis(1600)); // 1600ms
}

#[tokio::test]
async fn test_retry_with_jitter() {
    let config = RetryConfig {
        max_attempts: 3,
        base_delay: Duration::from_millis(100),
        max_delay: Duration::from_secs(10),
        strategy: RetryStrategy::ExponentialBackoff,
        jitter: true,
    };
    
    let retry_manager = RetryManager::new(config);
    
    // With jitter, delays should vary but be within expected range
    let delay1 = retry_manager.calculate_delay(1);
    let delay2 = retry_manager.calculate_delay(1);
    
    // Both should be around 200ms but different due to jitter
    assert!(delay1 >= Duration::from_millis(100));
    assert!(delay1 <= Duration::from_millis(300));
    assert!(delay2 >= Duration::from_millis(100));
    assert!(delay2 <= Duration::from_millis(300));
    
    // They should be different (very unlikely to be exactly the same with jitter)
    assert_ne!(delay1, delay2);
}

#[tokio::test]
async fn test_max_delay_cap() {
    let config = RetryConfig {
        max_attempts: 10,
        base_delay: Duration::from_millis(100),
        max_delay: Duration::from_secs(5), // Cap at 5 seconds
        strategy: RetryStrategy::ExponentialBackoff,
        jitter: false,
    };
    
    let retry_manager = RetryManager::new(config);
    
    // Later attempts should be capped at max_delay
    let delay_high = retry_manager.calculate_delay(10);
    assert_eq!(delay_high, Duration::from_secs(5));
}

#[tokio::test]
async fn test_retry_attempt_tracking() {
    let config = RetryConfig::default();
    let mut retry_manager = RetryManager::new(config);
    
    let batch_id = "test-batch-123";
    
    // Start retry tracking
    retry_manager.start_retry(batch_id);
    
    // Check initial state
    assert_eq!(retry_manager.get_attempt_count(batch_id), 0);
    assert!(!retry_manager.should_give_up(batch_id));
    
    // Increment attempts
    retry_manager.increment_attempt(batch_id);
    assert_eq!(retry_manager.get_attempt_count(batch_id), 1);
    
    retry_manager.increment_attempt(batch_id);
    assert_eq!(retry_manager.get_attempt_count(batch_id), 2);
}

#[tokio::test]
async fn test_give_up_after_max_attempts() {
    let config = RetryConfig {
        max_attempts: 3,
        ..Default::default()
    };
    
    let mut retry_manager = RetryManager::new(config);
    let batch_id = "test-batch-456";
    
    retry_manager.start_retry(batch_id);
    
    // Should not give up before max attempts
    for _ in 0..3 {
        assert!(!retry_manager.should_give_up(batch_id));
        retry_manager.increment_attempt(batch_id);
    }
    
    // Should give up after max attempts
    assert!(retry_manager.should_give_up(batch_id));
}