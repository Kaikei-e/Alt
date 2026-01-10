//! OpenTelemetry Protocol (OTLP) receiver module
//!
//! This module provides:
//! - OTLP HTTP/protobuf receiver endpoints
//! - OTel Log/Trace Data Model to ClickHouse schema conversion

pub mod converter;
pub mod receiver;

pub use receiver::otlp_routes;
