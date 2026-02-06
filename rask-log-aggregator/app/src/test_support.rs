//! Shared test support utilities
//!
//! Provides a unified `MockExporter` implementing both `LogExporter` and `OTelExporter`
//! for use in unit and integration tests.

use crate::domain::{EnrichedLogEntry, OTelLog, OTelTrace};
use crate::error::AggregatorError;
use crate::port::{LogExporter, OTelExporter};
use std::future::Future;
use std::pin::Pin;
use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};
use std::sync::{Arc, Mutex};

/// Mock exporter that captures exported data for testing.
///
/// Implements both `LogExporter` (legacy NDJSON) and `OTelExporter` (OTLP).
pub struct MockExporter {
    exported_logs: Arc<Mutex<Vec<EnrichedLogEntry>>>,
    otel_logs_count: AtomicUsize,
    otel_traces_count: AtomicUsize,
    should_fail: AtomicBool,
}

impl MockExporter {
    pub fn new() -> Self {
        Self {
            exported_logs: Arc::new(Mutex::new(Vec::new())),
            otel_logs_count: AtomicUsize::new(0),
            otel_traces_count: AtomicUsize::new(0),
            should_fail: AtomicBool::new(false),
        }
    }

    pub fn set_should_fail(&self, fail: bool) {
        self.should_fail.store(fail, Ordering::SeqCst);
    }

    pub fn get_exported_logs(&self) -> Vec<EnrichedLogEntry> {
        self.exported_logs.lock().unwrap().clone()
    }

    pub fn otel_logs_exported(&self) -> usize {
        self.otel_logs_count.load(Ordering::SeqCst)
    }

    pub fn otel_traces_exported(&self) -> usize {
        self.otel_traces_count.load(Ordering::SeqCst)
    }
}

impl LogExporter for MockExporter {
    fn export_batch(
        &self,
        logs: Vec<EnrichedLogEntry>,
    ) -> Pin<Box<dyn Future<Output = Result<(), AggregatorError>> + Send + '_>> {
        let exported_logs = self.exported_logs.clone();
        Box::pin(async move {
            if self.should_fail.load(Ordering::SeqCst) {
                return Err(AggregatorError::Export("Mock export failure".to_string()));
            }
            let mut guard = exported_logs.lock().unwrap();
            guard.extend(logs);
            Ok(())
        })
    }
}

impl OTelExporter for MockExporter {
    fn export_otel_logs(
        &self,
        logs: Vec<OTelLog>,
    ) -> Pin<Box<dyn Future<Output = Result<(), AggregatorError>> + Send + '_>> {
        let count = logs.len();
        Box::pin(async move {
            if self.should_fail.load(Ordering::SeqCst) {
                return Err(AggregatorError::Export("Mock export failure".to_string()));
            }
            self.otel_logs_count.fetch_add(count, Ordering::SeqCst);
            Ok(())
        })
    }

    fn export_otel_traces(
        &self,
        traces: Vec<OTelTrace>,
    ) -> Pin<Box<dyn Future<Output = Result<(), AggregatorError>> + Send + '_>> {
        let count = traces.len();
        Box::pin(async move {
            if self.should_fail.load(Ordering::SeqCst) {
                return Err(AggregatorError::Export("Mock export failure".to_string()));
            }
            self.otel_traces_count.fetch_add(count, Ordering::SeqCst);
            Ok(())
        })
    }
}
