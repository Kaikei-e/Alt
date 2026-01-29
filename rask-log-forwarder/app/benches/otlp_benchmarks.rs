//! OTLP vs NDJSON serialization benchmarks.
//!
//! Measures serialization performance and payload size differences.

#![cfg(feature = "otlp")]

use criterion::{BenchmarkId, Criterion, Throughput, black_box, criterion_group, criterion_main};
use rask_log_forwarder::buffer::{Batch, BatchType};
use rask_log_forwarder::parser::{EnrichedLogEntry, LogLevel};
use rask_log_forwarder::sender::{BatchSerializer, OtlpSerializer, SerializationFormat};
use std::collections::HashMap;

fn create_test_entry(id: usize) -> EnrichedLogEntry {
    EnrichedLogEntry {
        service_type: "benchmark-service".to_string(),
        log_type: "structured".to_string(),
        message: format!("Benchmark log message with some content and details for entry {}", id),
        level: Some(LogLevel::Info),
        timestamp: "2024-01-01T00:00:00.000Z".to_string(),
        stream: "stdout".to_string(),
        method: Some("GET".to_string()),
        path: Some("/api/v1/benchmark".to_string()),
        status_code: Some(200),
        response_size: Some(1024),
        ip_address: Some("192.168.1.100".to_string()),
        user_agent: Some("benchmark-agent/1.0".to_string()),
        container_id: "benchmark-container-12345".to_string(),
        service_name: "benchmark-service".to_string(),
        service_group: Some("benchmark-group".to_string()),
        trace_id: Some("4bf92f3577b34da6a3ce929d0e0e4736".to_string()),
        span_id: Some("00f067aa0ba902b7".to_string()),
        fields: {
            let mut fields = HashMap::new();
            fields.insert("request_id".to_string(), format!("req-{}", id));
            fields.insert("user_id".to_string(), "user-12345".to_string());
            fields
        },
    }
}

fn create_batch(size: usize) -> Batch {
    let entries: Vec<_> = (0..size).map(create_test_entry).collect();
    Batch::new(entries, BatchType::SizeBased)
}

fn bench_serialization_throughput(c: &mut Criterion) {
    let mut group = c.benchmark_group("serialization_throughput");

    for batch_size in [100, 1000, 10000].iter() {
        let batch = create_batch(*batch_size);
        group.throughput(Throughput::Elements(*batch_size as u64));

        // NDJSON serialization
        let json_serializer = BatchSerializer::new();
        group.bench_with_input(
            BenchmarkId::new("ndjson", batch_size),
            &batch,
            |b, batch| {
                b.iter(|| {
                    black_box(json_serializer.serialize_ndjson(batch).unwrap())
                })
            },
        );

        // OTLP serialization
        let otlp_serializer = OtlpSerializer::new();
        group.bench_with_input(
            BenchmarkId::new("otlp", batch_size),
            &batch,
            |b, batch| {
                b.iter(|| {
                    black_box(otlp_serializer.serialize_batch(batch).unwrap())
                })
            },
        );
    }

    group.finish();
}

fn bench_payload_size(c: &mut Criterion) {
    let mut group = c.benchmark_group("payload_size_comparison");

    for batch_size in [100, 1000, 10000].iter() {
        let batch = create_batch(*batch_size);

        let json_serializer = BatchSerializer::new();
        let otlp_serializer = OtlpSerializer::new();

        // Measure and report sizes
        let ndjson_size = json_serializer.serialize_ndjson(&batch).unwrap().len();
        let otlp_size = otlp_serializer.serialize_batch(&batch).unwrap().len();

        println!(
            "Batch size {}: NDJSON={} bytes, OTLP={} bytes, Reduction={:.1}%",
            batch_size,
            ndjson_size,
            otlp_size,
            (1.0 - otlp_size as f64 / ndjson_size as f64) * 100.0
        );

        // Benchmark NDJSON
        group.bench_with_input(
            BenchmarkId::new("ndjson_bytes", batch_size),
            &batch,
            |b, batch| {
                b.iter(|| {
                    let data = json_serializer.serialize_ndjson(batch).unwrap();
                    black_box(data.len())
                })
            },
        );

        // Benchmark OTLP
        group.bench_with_input(
            BenchmarkId::new("otlp_bytes", batch_size),
            &batch,
            |b, batch| {
                b.iter(|| {
                    let data = otlp_serializer.serialize_batch(batch).unwrap();
                    black_box(data.len())
                })
            },
        );
    }

    group.finish();
}

fn bench_compression(c: &mut Criterion) {
    let mut group = c.benchmark_group("compression_comparison");

    let batch = create_batch(1000);
    let json_serializer = BatchSerializer::new();

    // NDJSON with gzip
    group.bench_function("ndjson_gzip", |b| {
        b.iter(|| {
            black_box(
                json_serializer
                    .serialize_compressed(&batch, SerializationFormat::NDJSON)
                    .unwrap(),
            )
        })
    });

    // OTLP with gzip
    group.bench_function("otlp_gzip", |b| {
        b.iter(|| {
            black_box(
                json_serializer
                    .serialize_compressed(&batch, SerializationFormat::OTLP)
                    .unwrap(),
            )
        })
    });

    group.finish();
}

criterion_group!(
    benches,
    bench_serialization_throughput,
    bench_payload_size,
    bench_compression
);
criterion_main!(benches);
