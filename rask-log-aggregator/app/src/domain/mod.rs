mod enriched_log;
mod otel;

pub use enriched_log::{EnrichedLogEntry, LogLevel};
pub use otel::{OTelLog, OTelTrace, SpanEvent, SpanKind, SpanLink, StatusCode};
