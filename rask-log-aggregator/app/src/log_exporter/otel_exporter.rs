//! OTelExporter trait for OpenTelemetry log and trace export.
//!
//! This trait enables dependency injection for testability,
//! allowing unit tests to use mock implementations.

use crate::domain::{OTelLog, OTelTrace};
use crate::error::AggregatorError;
use std::future::Future;
use std::pin::Pin;

/// Trait for exporting OpenTelemetry logs and traces.
///
/// This trait is dyn-compatible by using boxed futures.
/// Implementations include `ClickHouseExporter` for production
/// and mock implementations for testing.
pub trait OTelExporter: Send + Sync {
    /// Export OpenTelemetry logs to the underlying storage.
    fn export_otel_logs(
        &self,
        logs: Vec<OTelLog>,
    ) -> Pin<Box<dyn Future<Output = Result<(), AggregatorError>> + Send + '_>>;

    /// Export OpenTelemetry traces to the underlying storage.
    fn export_otel_traces(
        &self,
        traces: Vec<OTelTrace>,
    ) -> Pin<Box<dyn Future<Output = Result<(), AggregatorError>> + Send + '_>>;
}
