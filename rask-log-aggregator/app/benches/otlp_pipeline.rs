use criterion::{BenchmarkId, Criterion, black_box, criterion_group, criterion_main};
use opentelemetry_proto::tonic::{
    collector::{logs::v1::ExportLogsServiceRequest, trace::v1::ExportTraceServiceRequest},
    common::v1::{AnyValue, InstrumentationScope, KeyValue, any_value},
    logs::v1::{LogRecord, ResourceLogs, ScopeLogs},
    resource::v1::Resource,
    trace::v1::{ResourceSpans, ScopeSpans, Span, Status},
};
use rask::adapter::clickhouse::convert::string_to_fixed_bytes;
use rask::adapter::clickhouse::otel_row::{OTelLogRow, OTelTraceRow};
use rask::adapter::clickhouse::row::LogRow;
use rask::domain::{EnrichedLogEntry, LogLevel};
use rask::otlp::converter::{convert_log_records, convert_spans};
use std::collections::HashMap;

fn make_attributes(n: usize) -> Vec<KeyValue> {
    (0..n)
        .map(|i| KeyValue {
            key: format!("key_{i}"),
            value: Some(AnyValue {
                value: Some(any_value::Value::StringValue(format!("value_{i}"))),
            }),
        })
        .collect()
}

fn make_log_request(n_logs: usize) -> ExportLogsServiceRequest {
    let records: Vec<LogRecord> = (0..n_logs)
        .map(|i| LogRecord {
            time_unix_nano: 1_700_000_000_000_000_000 + i as u64,
            observed_time_unix_nano: 1_700_000_000_000_000_000 + i as u64,
            trace_id: vec![0x01; 16],
            span_id: vec![0x02; 8],
            flags: 1,
            severity_text: "INFO".to_string(),
            severity_number: 9,
            body: Some(AnyValue {
                value: Some(any_value::Value::StringValue(format!(
                    "Log message number {i}"
                ))),
            }),
            attributes: make_attributes(5),
            ..Default::default()
        })
        .collect();

    ExportLogsServiceRequest {
        resource_logs: vec![ResourceLogs {
            resource: Some(Resource {
                attributes: vec![KeyValue {
                    key: "service.name".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("bench-service".to_string())),
                    }),
                }],
                ..Default::default()
            }),
            scope_logs: vec![ScopeLogs {
                scope: Some(InstrumentationScope {
                    name: "bench-scope".to_string(),
                    version: "1.0.0".to_string(),
                    ..Default::default()
                }),
                log_records: records,
                ..Default::default()
            }],
            ..Default::default()
        }],
    }
}

fn make_trace_request(n_spans: usize) -> ExportTraceServiceRequest {
    let spans: Vec<Span> = (0..n_spans)
        .map(|i| Span {
            trace_id: vec![0x01; 16],
            span_id: vec![0x02; 8],
            parent_span_id: vec![0x03; 8],
            name: format!("span-{i}"),
            kind: 2, // Server
            start_time_unix_nano: 1_700_000_000_000_000_000,
            end_time_unix_nano: 1_700_000_000_000_000_000 + 1_000_000,
            attributes: make_attributes(8),
            status: Some(Status {
                code: 1,
                message: String::new(),
            }),
            ..Default::default()
        })
        .collect();

    ExportTraceServiceRequest {
        resource_spans: vec![ResourceSpans {
            resource: Some(Resource {
                attributes: vec![KeyValue {
                    key: "service.name".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("bench-service".to_string())),
                    }),
                }],
                ..Default::default()
            }),
            scope_spans: vec![ScopeSpans {
                spans,
                ..Default::default()
            }],
            ..Default::default()
        }],
    }
}

fn make_enriched_log(i: usize) -> EnrichedLogEntry {
    let mut fields = HashMap::new();
    fields.insert("request_id".to_string(), format!("req-{i}"));
    fields.insert("duration_ms".to_string(), "42".to_string());

    EnrichedLogEntry {
        service_type: "http".to_string(),
        log_type: "access".to_string(),
        message: format!("GET /api/v1/items/{i} 200"),
        level: Some(LogLevel::Info),
        timestamp: "2024-01-15T10:30:00.000Z".to_string(),
        stream: "stdout".to_string(),
        container_id: "abc123def456".to_string(),
        service_name: "bench-service".to_string(),
        service_group: Some("backend".to_string()),
        fields,
        method: Some("GET".to_string()),
        path: Some(format!("/api/v1/items/{i}")),
        status_code: Some(200),
        response_size: Some(1024),
        ip_address: Some("192.168.1.1".to_string()),
        user_agent: Some("Mozilla/5.0".to_string()),
        trace_id: Some("0123456789abcdef0123456789abcdef".to_string()),
        span_id: Some("0123456789abcdef".to_string()),
    }
}

// =========================================================================
// Benchmarks
// =========================================================================

fn bench_string_to_fixed_bytes(c: &mut Criterion) {
    let mut group = c.benchmark_group("string_to_fixed_bytes");

    let trace_id = "0123456789abcdef0123456789abcdef";
    group.bench_function("trace_id_32", |b| {
        b.iter(|| string_to_fixed_bytes::<32>(black_box(trace_id)));
    });

    let span_id = "0123456789abcdef";
    group.bench_function("span_id_16", |b| {
        b.iter(|| string_to_fixed_bytes::<16>(black_box(span_id)));
    });

    group.bench_function("empty_32", |b| {
        b.iter(|| string_to_fixed_bytes::<32>(black_box("")));
    });

    group.finish();
}

fn bench_otlp_log_pipeline(c: &mut Criterion) {
    let mut group = c.benchmark_group("otlp_log_pipeline");

    for size in [10, 100, 1000] {
        let request = make_log_request(size);
        group.bench_with_input(BenchmarkId::new("convert", size), &request, |b, req| {
            b.iter(|| convert_log_records(black_box(req)));
        });

        // Full pipeline: convert + to row
        group.bench_with_input(BenchmarkId::new("convert+row", size), &request, |b, req| {
            b.iter(|| {
                let logs = convert_log_records(black_box(req));
                let _rows: Vec<OTelLogRow> = logs.into_iter().map(OTelLogRow::from).collect();
            });
        });
    }

    group.finish();
}

fn bench_otlp_trace_pipeline(c: &mut Criterion) {
    let mut group = c.benchmark_group("otlp_trace_pipeline");

    for size in [10, 100, 1000] {
        let request = make_trace_request(size);
        group.bench_with_input(BenchmarkId::new("convert", size), &request, |b, req| {
            b.iter(|| convert_spans(black_box(req)));
        });

        group.bench_with_input(BenchmarkId::new("convert+row", size), &request, |b, req| {
            b.iter(|| {
                let traces = convert_spans(black_box(req));
                let _rows: Vec<OTelTraceRow> = traces.into_iter().map(OTelTraceRow::from).collect();
            });
        });
    }

    group.finish();
}

fn bench_ndjson_pipeline(c: &mut Criterion) {
    let mut group = c.benchmark_group("ndjson_pipeline");

    for size in [10, 100, 1000] {
        let entries: Vec<EnrichedLogEntry> = (0..size).map(make_enriched_log).collect();

        group.bench_with_input(
            BenchmarkId::new("enriched_to_row", size),
            &entries,
            |b, entries| {
                b.iter(|| {
                    let _rows: Vec<LogRow> = black_box(entries)
                        .iter()
                        .cloned()
                        .map(LogRow::from)
                        .collect();
                });
            },
        );
    }

    group.finish();
}

criterion_group!(
    benches,
    bench_string_to_fixed_bytes,
    bench_otlp_log_pipeline,
    bench_otlp_trace_pipeline,
    bench_ndjson_pipeline,
);
criterion_main!(benches);
