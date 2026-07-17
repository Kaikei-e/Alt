use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Serialize, Deserialize, Clone, Debug)]
pub struct EnrichedLogEntry {
    pub service_type: String,
    pub log_type: String,
    pub message: String,
    pub level: Option<LogLevel>,
    pub timestamp: String,
    pub stream: String,
    pub container_id: String,
    pub service_name: String,
    pub service_group: Option<String>,
    pub fields: HashMap<String, String>,

    // HTTP-specific fields (populated by rask-log-forwarder for HTTP services)
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub method: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub path: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub status_code: Option<u16>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub response_size: Option<u64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub ip_address: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub user_agent: Option<String>,

    // Trace context (OpenTelemetry)
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub trace_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub span_id: Option<String>,
}

#[derive(Serialize, Deserialize, Clone, Debug)]
pub enum LogLevel {
    Debug,
    Info,
    Warn,
    Error,
    Fatal,
}

#[cfg(test)]
mod contract_tests {
    use super::*;
    use serde_json::Value;

    /// CDC-style schema freeze: field set must stay aligned with
    /// `rask-log-forwarder::domain::EnrichedLogEntry` (wire JSON / NDJSON).
    /// Shared-crate extraction is deferred; this test fails loudly on drift.
    #[test]
    fn enriched_log_entry_json_keys_match_forwarder_contract() {
        let entry = EnrichedLogEntry {
            service_type: "svc".into(),
            log_type: "access".into(),
            message: "hello".into(),
            level: Some(LogLevel::Info),
            timestamp: "2024-01-01T00:00:00Z".into(),
            stream: "stdout".into(),
            container_id: "abc".into(),
            service_name: "svc".into(),
            service_group: Some("g".into()),
            fields: HashMap::from([("k".into(), "v".into())]),
            method: Some("GET".into()),
            path: Some("/".into()),
            status_code: Some(200),
            response_size: Some(1),
            ip_address: Some("127.0.0.1".into()),
            user_agent: Some("ua".into()),
            trace_id: Some("trace".into()),
            span_id: Some("span".into()),
        };

        let Value::Object(map) = serde_json::to_value(&entry).unwrap() else {
            panic!("expected object");
        };

        let mut keys: Vec<_> = map.keys().cloned().collect();
        keys.sort();

        let expected = [
            "container_id",
            "fields",
            "ip_address",
            "level",
            "log_type",
            "message",
            "method",
            "path",
            "response_size",
            "service_group",
            "service_name",
            "service_type",
            "span_id",
            "status_code",
            "stream",
            "timestamp",
            "trace_id",
            "user_agent",
        ];
        assert_eq!(keys, expected);
    }
}
