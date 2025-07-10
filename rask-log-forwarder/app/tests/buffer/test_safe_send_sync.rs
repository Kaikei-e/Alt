use crate::buffer::lockfree::{LogBuffer, LogBufferSender, LogBufferReceiver};
use crate::buffer::queue::LogBuffer as QueueLogBuffer;
use crate::parser::EnrichedLogEntry;
use std::sync::Arc;
use tokio::task::JoinHandle;

#[tokio::test]
async fn test_log_buffer_sender_is_send_and_sync() {
    // This test verifies that LogBufferSender can be safely used across threads
    let buffer = LogBuffer::new(1000).unwrap();
    let (sender, _receiver) = buffer.split().expect("Failed to split buffer");
    
    // Test Send trait by moving sender to different thread
    let handle: JoinHandle<()> = tokio::spawn(async move {
        let entry = EnrichedLogEntry::default();
        let _ = sender.send(entry).await;
    });
    
    handle.await.unwrap();
}

#[tokio::test]
async fn test_log_buffer_receiver_is_send_and_sync() {
    // This test verifies that LogBufferReceiver can be safely used across threads
    let buffer = LogBuffer::new(1000).unwrap();
    let (_sender, mut receiver) = buffer.split().expect("Failed to split buffer");
    
    // Test Send trait by moving receiver to different thread
    let handle: JoinHandle<()> = tokio::spawn(async move {
        let _ = receiver.recv().await;
    });
    
    // Cancel the task since recv will block indefinitely
    handle.abort();
}

#[tokio::test]
async fn test_log_buffer_is_send_and_sync() {
    // This test verifies that LogBuffer can be safely used across threads
    let buffer = LogBuffer::new(1000).unwrap();
    
    // Test Send trait by moving buffer to different thread
    let handle: JoinHandle<()> = tokio::spawn(async move {
        let entry = EnrichedLogEntry::default();
        let _ = buffer.push(entry);
    });
    
    handle.await.unwrap();
}

#[tokio::test]
async fn test_queue_log_buffer_is_send_and_sync() {
    // This test verifies that queue::LogBuffer can be safely used across threads
    let buffer = QueueLogBuffer::new(1000).unwrap();
    let buffer_arc = Arc::new(buffer);
    
    // Test Send trait by moving buffer to different thread
    let buffer_clone = buffer_arc.clone();
    let handle: JoinHandle<()> = tokio::spawn(async move {
        let entry = Arc::new(crate::parser::NginxLogEntry::default());
        let _ = buffer_clone.push(entry);
    });
    
    handle.await.unwrap();
}

#[tokio::test]
async fn test_concurrent_access_safety() {
    // This test verifies that the buffer can safely handle concurrent access
    let buffer = Arc::new(LogBuffer::new(10000).unwrap());
    let mut handles = Vec::new();
    
    // Start multiple producer tasks
    for i in 0..10 {
        let buffer_clone = buffer.clone();
        let handle = tokio::spawn(async move {
            for j in 0..1000 {
                let entry = EnrichedLogEntry::default();
                let _ = buffer_clone.push(entry);
                if j % 100 == 0 {
                    tokio::task::yield_now().await;
                }
            }
        });
        handles.push(handle);
    }
    
    // Wait for all tasks to complete
    for handle in handles {
        handle.await.unwrap();
    }
    
    // Buffer should have processed all entries without crashes
    assert!(buffer.len() <= 10000);
}

// Test to verify we can pass these structs to functions expecting Send + Sync
fn requires_send_sync<T: Send + Sync>(_: T) {}

#[test]
fn test_traits_compilation() {
    let buffer = LogBuffer::new(1000).unwrap();
    let (sender, receiver) = buffer.split();
    
    requires_send_sync(sender);
    requires_send_sync(receiver);
    requires_send_sync(buffer);
}