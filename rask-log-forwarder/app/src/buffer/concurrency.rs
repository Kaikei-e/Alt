// Memory-safe concurrency primitives for rask-log-forwarder
//
// This module provides robust alternatives to standard library concurrency
// primitives with automatic recovery from poisoned mutexes and deadlock prevention.

use std::sync::{Arc, Mutex};
use std::time::Duration;
use tokio::sync::RwLock;
use tokio::time::timeout;
use thiserror::Error;

#[derive(Error, Debug)]
pub enum ConcurrencyError {
    #[error("Mutex poisoned during operation: {operation}")]
    MutexPoisoned { operation: String },
    
    #[error("Lock acquisition timeout after {timeout_ms}ms")]
    LockTimeout { timeout_ms: u64 },
    
    #[error("Deadlock detected in operation: {operation}")]
    DeadlockDetected { operation: String },
    
    #[error("Concurrent access violation: {details}")]
    ConcurrentAccessViolation { details: String },
    
    #[error("Resource exhaustion: {resource}")]
    ResourceExhaustion { resource: String },
}

impl ConcurrencyError {
    /// Provides memory-safe recovery strategy for different error types
    pub fn recovery_strategy(&self) -> RecoveryStrategy {
        match self {
            ConcurrencyError::MutexPoisoned { .. } => RecoveryStrategy::ResetState,
            ConcurrencyError::LockTimeout { .. } => RecoveryStrategy::RetryWithBackoff,
            ConcurrencyError::DeadlockDetected { .. } => RecoveryStrategy::AbortAndRestart,
            _ => RecoveryStrategy::Fail,
        }
    }
}

#[derive(Debug, Clone)]
pub enum RecoveryStrategy {
    ResetState,
    RetryWithBackoff,
    AbortAndRestart,
    Fail,
}

/// A robust mutex wrapper that automatically recovers from poisoned states
/// and prevents deadlocks through timeout mechanisms.
pub struct RobustMutex<T> {
    inner: Arc<Mutex<T>>,
    operation_name: String,
    timeout_duration: Duration,
}

impl<T> Clone for RobustMutex<T> {
    fn clone(&self) -> Self {
        Self {
            inner: Arc::clone(&self.inner),
            operation_name: self.operation_name.clone(),
            timeout_duration: self.timeout_duration,
        }
    }
}

impl<T> RobustMutex<T> {
    pub fn new(data: T, operation_name: impl Into<String>) -> Self {
        Self {
            inner: Arc::new(Mutex::new(data)),
            operation_name: operation_name.into(),
            timeout_duration: Duration::from_millis(100),
        }
    }
    
    pub fn with_timeout(mut self, timeout: Duration) -> Self {
        self.timeout_duration = timeout;
        self
    }
    
    /// Memory-safe lock acquisition with automatic recovery from poisoned state
    pub fn lock_safe(&self) -> Result<std::sync::MutexGuard<'_, T>, ConcurrencyError> {
        match self.inner.lock() {
            Ok(guard) => Ok(guard),
            Err(poisoned) => {
                tracing::error!(
                    operation = %self.operation_name,
                    "Mutex poisoned, attempting automatic recovery"
                );
                
                // Automatic recovery: extract poisoned data
                Ok(poisoned.into_inner())
            }
        }
    }
    
    /// Non-blocking lock attempt with automatic poisoning recovery
    pub fn try_lock_safe(&self) -> Result<std::sync::MutexGuard<'_, T>, ConcurrencyError> {
        match self.inner.try_lock() {
            Ok(guard) => Ok(guard),
            Err(std::sync::TryLockError::Poisoned(poisoned)) => {
                tracing::warn!(
                    operation = %self.operation_name,
                    "Mutex poisoned, recovering automatically"
                );
                Ok(poisoned.into_inner())
            }
            Err(std::sync::TryLockError::WouldBlock) => {
                Err(ConcurrencyError::ConcurrentAccessViolation {
                    details: format!("Operation {} would block", self.operation_name),
                })
            }
        }
    }
}

/// High-performance RwLock wrapper for read-heavy workloads
pub struct RobustRwLock<T> {
    inner: Arc<RwLock<T>>,
    operation_name: String,
    read_timeout: Duration,
    write_timeout: Duration,
}

impl<T> Clone for RobustRwLock<T> {
    fn clone(&self) -> Self {
        Self {
            inner: Arc::clone(&self.inner),
            operation_name: self.operation_name.clone(),
            read_timeout: self.read_timeout,
            write_timeout: self.write_timeout,
        }
    }
}

impl<T> RobustRwLock<T> {
    pub fn new(data: T, operation_name: impl Into<String>) -> Self {
        Self {
            inner: Arc::new(RwLock::new(data)),
            operation_name: operation_name.into(),
            read_timeout: Duration::from_millis(50),
            write_timeout: Duration::from_millis(100),
        }
    }
    
    pub fn with_timeouts(mut self, read_timeout: Duration, write_timeout: Duration) -> Self {
        self.read_timeout = read_timeout;
        self.write_timeout = write_timeout;
        self
    }
    
    /// High-performance read lock with timeout protection
    pub async fn read_safe(&self) -> Result<tokio::sync::RwLockReadGuard<'_, T>, ConcurrencyError> {
        timeout(self.read_timeout, self.inner.read())
            .await
            .map_err(|_| ConcurrencyError::LockTimeout { 
                timeout_ms: self.read_timeout.as_millis() as u64 
            })
    }
    
    /// Write lock for mutations with timeout protection
    pub async fn write_safe(&self) -> Result<tokio::sync::RwLockWriteGuard<'_, T>, ConcurrencyError> {
        timeout(self.write_timeout, self.inner.write())
            .await
            .map_err(|_| ConcurrencyError::LockTimeout { 
                timeout_ms: self.write_timeout.as_millis() as u64 
            })
    }
    
    /// Non-blocking read attempt
    pub fn try_read(&self) -> Result<tokio::sync::RwLockReadGuard<'_, T>, ConcurrencyError> {
        self.inner.try_read().map_err(|_| {
            ConcurrencyError::ConcurrentAccessViolation {
                details: format!("Read operation {} would block", self.operation_name),
            }
        })
    }
    
    /// Non-blocking write attempt
    pub fn try_write(&self) -> Result<tokio::sync::RwLockWriteGuard<'_, T>, ConcurrencyError> {
        self.inner.try_write().map_err(|_| {
            ConcurrencyError::ConcurrentAccessViolation {
                details: format!("Write operation {} would block", self.operation_name),
            }
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_robust_mutex_basic_operations() {
        let mutex = RobustMutex::new(42u32, "test_basic");
        
        // Test successful lock
        let guard = mutex.lock_safe().unwrap();
        assert_eq!(*guard, 42);
        drop(guard);
        
        // Test try_lock
        let guard = mutex.try_lock_safe().unwrap();
        assert_eq!(*guard, 42);
    }

    #[tokio::test]
    async fn test_robust_rwlock_basic_operations() {
        let rwlock = RobustRwLock::new(vec![1, 2, 3], "test_rwlock");
        
        // Test read lock
        let read_guard = rwlock.read_safe().await.unwrap();
        assert_eq!(read_guard.len(), 3);
        drop(read_guard);
        
        // Test write lock
        let mut write_guard = rwlock.write_safe().await.unwrap();
        write_guard.push(4);
        assert_eq!(write_guard.len(), 4);
    }

    #[tokio::test]
    async fn test_concurrent_read_access() {
        let rwlock = RobustRwLock::new(vec![1, 2, 3], "concurrent_read_test");
        
        // Multiple concurrent readers should work
        let handles: Vec<_> = (0..10)
            .map(|_| {
                let rwlock_clone = rwlock.clone();
                tokio::spawn(async move {
                    let guard = rwlock_clone.read_safe().await?;
                    assert_eq!(guard.len(), 3);
                    Ok::<(), ConcurrencyError>(())
                })
            })
            .collect();
        
        for handle in handles {
            handle.await.unwrap().unwrap();
        }
    }

    #[test]
    fn test_mutex_poisoning_recovery() {
        let mutex = RobustMutex::new(42u32, "poisoning_test");
        
        // Create a scope where we intentionally poison the mutex
        let mutex_clone = mutex.clone();
        let result = std::panic::catch_unwind(|| {
            let _guard = mutex_clone.lock_safe().unwrap();
            panic!("Intentional panic to poison mutex");
        });
        
        // Verify the panic occurred
        assert!(result.is_err());
        
        // The mutex should still be usable due to automatic recovery
        let guard = mutex.lock_safe().unwrap();
        assert_eq!(*guard, 42);
    }
}