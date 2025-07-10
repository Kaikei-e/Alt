// Lock-free connection statistics using atomic operations
//
// This module provides high-performance statistics collection without
// the risk of mutex poisoning or deadlocks.

use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{SystemTime, UNIX_EPOCH};
use serde::{Deserialize, Serialize};

/// Lock-free connection statistics using atomic operations
#[derive(Debug)]
pub struct AtomicConnectionStats {
    active_connections: AtomicU64,
    total_requests: AtomicU64,
    reused_connections: AtomicU64,
    failed_requests: AtomicU64,
    bytes_sent: AtomicU64,
    last_request_time: AtomicU64,
    connection_errors: AtomicU64,
    timeout_errors: AtomicU64,
}

impl Default for AtomicConnectionStats {
    fn default() -> Self {
        Self::new()
    }
}

impl AtomicConnectionStats {
    pub fn new() -> Self {
        Self {
            active_connections: AtomicU64::new(0),
            total_requests: AtomicU64::new(0),
            reused_connections: AtomicU64::new(0),
            failed_requests: AtomicU64::new(0),
            bytes_sent: AtomicU64::new(0),
            last_request_time: AtomicU64::new(0),
            connection_errors: AtomicU64::new(0),
            timeout_errors: AtomicU64::new(0),
        }
    }

    /// Record a successful request with lock-free atomic operations
    pub fn record_request(&self, bytes: u64) {
        self.total_requests.fetch_add(1, Ordering::Relaxed);
        self.bytes_sent.fetch_add(bytes, Ordering::Relaxed);
        
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();
        
        self.last_request_time.store(now, Ordering::Relaxed);
    }

    /// Record a connection reuse (keep-alive)
    pub fn record_connection_reuse(&self) {
        self.reused_connections.fetch_add(1, Ordering::Relaxed);
    }

    /// Record a failed request
    pub fn record_failed_request(&self) {
        self.failed_requests.fetch_add(1, Ordering::Relaxed);
    }

    /// Record a connection error
    pub fn record_connection_error(&self) {
        self.connection_errors.fetch_add(1, Ordering::Relaxed);
    }

    /// Record a timeout error
    pub fn record_timeout_error(&self) {
        self.timeout_errors.fetch_add(1, Ordering::Relaxed);
    }

    /// Increment active connections counter
    pub fn increment_active_connections(&self) {
        self.active_connections.fetch_add(1, Ordering::Relaxed);
    }

    /// Decrement active connections counter
    pub fn decrement_active_connections(&self) {
        self.active_connections.fetch_sub(1, Ordering::Relaxed);
    }

    /// Get a snapshot of current statistics (lock-free)
    pub fn get_snapshot(&self) -> ConnectionStatsSnapshot {
        ConnectionStatsSnapshot {
            active_connections: self.active_connections.load(Ordering::Relaxed),
            total_requests: self.total_requests.load(Ordering::Relaxed),
            reused_connections: self.reused_connections.load(Ordering::Relaxed),
            failed_requests: self.failed_requests.load(Ordering::Relaxed),
            bytes_sent: self.bytes_sent.load(Ordering::Relaxed),
            last_request_time: self.last_request_time.load(Ordering::Relaxed),
            connection_errors: self.connection_errors.load(Ordering::Relaxed),
            timeout_errors: self.timeout_errors.load(Ordering::Relaxed),
        }
    }

    /// Calculate success rate (0.0 to 1.0)
    pub fn success_rate(&self) -> f64 {
        let total = self.total_requests.load(Ordering::Relaxed);
        if total == 0 {
            return 1.0;
        }
        
        let failed = self.failed_requests.load(Ordering::Relaxed);
        (total.saturating_sub(failed) as f64) / (total as f64)
    }

    /// Calculate connection reuse rate (0.0 to 1.0)
    pub fn connection_reuse_rate(&self) -> f64 {
        let total = self.total_requests.load(Ordering::Relaxed);
        if total == 0 {
            return 0.0;
        }
        
        let reused = self.reused_connections.load(Ordering::Relaxed);
        (reused as f64) / (total as f64)
    }

    /// Reset all statistics (mainly for testing)
    pub fn reset(&self) {
        self.active_connections.store(0, Ordering::Relaxed);
        self.total_requests.store(0, Ordering::Relaxed);
        self.reused_connections.store(0, Ordering::Relaxed);
        self.failed_requests.store(0, Ordering::Relaxed);
        self.bytes_sent.store(0, Ordering::Relaxed);
        self.last_request_time.store(0, Ordering::Relaxed);
        self.connection_errors.store(0, Ordering::Relaxed);
        self.timeout_errors.store(0, Ordering::Relaxed);
    }
}

/// Immutable snapshot of connection statistics
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ConnectionStatsSnapshot {
    pub active_connections: u64,
    pub total_requests: u64,
    pub reused_connections: u64,
    pub failed_requests: u64,
    pub bytes_sent: u64,
    pub last_request_time: u64,
    pub connection_errors: u64,
    pub timeout_errors: u64,
}

impl ConnectionStatsSnapshot {
    /// Calculate success rate from snapshot
    pub fn success_rate(&self) -> f64 {
        if self.total_requests == 0 {
            return 1.0;
        }
        
        (self.total_requests.saturating_sub(self.failed_requests) as f64) 
            / (self.total_requests as f64)
    }

    /// Calculate connection reuse rate from snapshot
    pub fn connection_reuse_rate(&self) -> f64 {
        if self.total_requests == 0 {
            return 0.0;
        }
        
        (self.reused_connections as f64) / (self.total_requests as f64)
    }

    /// Check if connections are healthy based on error rates
    pub fn is_healthy(&self) -> bool {
        self.success_rate() > 0.95 && // 95% success rate
            self.connection_errors < self.total_requests / 10 // < 10% connection errors
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Arc;
    use std::thread;

    #[test]
    fn test_basic_operations() {
        let stats = AtomicConnectionStats::new();
        
        // Test recording requests
        stats.record_request(1024);
        stats.record_request(2048);
        
        let snapshot = stats.get_snapshot();
        assert_eq!(snapshot.total_requests, 2);
        assert_eq!(snapshot.bytes_sent, 3072);
        assert!(snapshot.last_request_time > 0);
    }

    #[test]
    fn test_concurrent_access() {
        let stats = Arc::new(AtomicConnectionStats::new());
        let mut handles = vec![];

        // Spawn multiple threads to stress test atomic operations
        for i in 0..10 {
            let stats_clone = Arc::clone(&stats);
            let handle = thread::spawn(move || {
                for j in 0..100 {
                    stats_clone.record_request((i * 100 + j) as u64);
                    if j % 2 == 0 {
                        stats_clone.record_connection_reuse();
                    }
                    if j % 10 == 0 {
                        stats_clone.record_failed_request();
                    }
                }
            });
            handles.push(handle);
        }

        // Wait for all threads to complete
        for handle in handles {
            handle.join().unwrap();
        }

        let snapshot = stats.get_snapshot();
        assert_eq!(snapshot.total_requests, 1000);
        assert_eq!(snapshot.reused_connections, 500);
        assert_eq!(snapshot.failed_requests, 100);
        
        // Verify calculations
        assert!((snapshot.success_rate() - 0.9).abs() < 0.01); // ~90% success rate
        assert!((snapshot.connection_reuse_rate() - 0.5).abs() < 0.01); // 50% reuse rate
    }

    #[test]
    fn test_connection_management() {
        let stats = AtomicConnectionStats::new();
        
        stats.increment_active_connections();
        stats.increment_active_connections();
        assert_eq!(stats.get_snapshot().active_connections, 2);
        
        stats.decrement_active_connections();
        assert_eq!(stats.get_snapshot().active_connections, 1);
    }

    #[test]
    fn test_error_tracking() {
        let stats = AtomicConnectionStats::new();
        
        stats.record_connection_error();
        stats.record_timeout_error();
        stats.record_timeout_error();
        
        let snapshot = stats.get_snapshot();
        assert_eq!(snapshot.connection_errors, 1);
        assert_eq!(snapshot.timeout_errors, 2);
    }

    #[test]
    fn test_health_check() {
        let stats = AtomicConnectionStats::new();
        
        // Healthy scenario
        for _ in 0..100 {
            stats.record_request(1024);
        }
        for _ in 0..2 {
            stats.record_failed_request();
        }
        
        let snapshot = stats.get_snapshot();
        assert!(snapshot.is_healthy());
        
        // Unhealthy scenario
        stats.reset();
        for _ in 0..10 {
            stats.record_request(1024);
            stats.record_failed_request();
        }
        
        let snapshot = stats.get_snapshot();
        assert!(!snapshot.is_healthy());
    }

    #[test]
    fn test_reset() {
        let stats = AtomicConnectionStats::new();
        
        stats.record_request(1024);
        stats.record_failed_request();
        stats.increment_active_connections();
        
        assert!(stats.get_snapshot().total_requests > 0);
        
        stats.reset();
        let snapshot = stats.get_snapshot();
        assert_eq!(snapshot.total_requests, 0);
        assert_eq!(snapshot.failed_requests, 0);
        assert_eq!(snapshot.active_connections, 0);
    }
}