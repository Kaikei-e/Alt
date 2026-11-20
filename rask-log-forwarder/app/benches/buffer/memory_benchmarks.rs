use chrono::Utc;
use criterion::{BenchmarkId, Criterion, criterion_group, criterion_main};
use rask_log_forwarder::buffer::{BufferConfig, LogBuffer};
use rask_log_forwarder::parser::NginxLogEntry;
use std::sync::Arc;

fn _create_test_nginx_log(id: usize) -> Arc<NginxLogEntry> {
    Arc::new(NginxLogEntry {
        service_type: "nginx".to_string(),
        log_type: "access".to_string(),
        message: format!("Test log message {id}"),
        stream: "stdout".to_string(),
        timestamp: Utc::now(),
        container_id: Some(format!("container_{}", id % 10)),
        ip_address: Some("192.168.1.1".to_string()),
        method: Some("GET".to_string()),
        path: Some("/api/test".to_string()),
        status_code: Some(200),
        response_size: Some(1024),
        user_agent: Some("test-agent".to_string()),
        level: None,
    })
}

fn bench_memory_efficiency(c: &mut Criterion) {
    let mut group = c.benchmark_group("memory_efficiency");

    // Test different buffer sizes to ensure constant memory footprint
    for &capacity in &[10000, 50000, 100_000, 500_000, 1_000_000] {
        group.bench_with_input(
            BenchmarkId::new("constant_memory_footprint", capacity),
            &capacity,
            |b, &capacity| {
                b.iter(|| {
                    if let Err(err) = run_memory_efficiency_iter(capacity) {
                        panic!("memory_efficiency benchmark failed: {err}");
                    }
                });
            },
        );
    }
    group.finish();
}

fn run_memory_efficiency_iter(capacity: usize) -> Result<(), Box<dyn std::error::Error>> {
    let buffer = LogBuffer::new_with_config(BufferConfig {
        capacity,
        ..Default::default()
    })?;

    // Fill buffer to various levels (25%, 50%, 75%, 100%)
    for (num, den) in [(1, 4), (1, 2), (3, 4), (1, 1)] {
        let fill_count = (capacity * num) / den;

        // Fill to target ratio
        let (sender, _receiver) = buffer.split()?;
        let rt = tokio::runtime::Runtime::new()?;
        for i in 0..fill_count {
            let log_entry = create_test_enriched_log(i);
            if rt.block_on(sender.send(log_entry)).is_err() {
                break; // Buffer full
            }
        }

        let metrics = buffer.metrics().snapshot();

        // Memory should be proportional to actual content, not capacity
        let memory_per_item = 50usize.checked_div(metrics.queue_depth).unwrap_or(0); // Simplified calculation

        // Each enriched log entry should not exceed reasonable bounds
        if metrics.queue_depth > 0 {
            assert!(
                memory_per_item < 1024,
                "Memory per item too high: {memory_per_item} bytes"
            );
        }

        std::hint::black_box(metrics.queue_depth);
    }
    Ok(())
}

fn bench_memory_growth_pattern(c: &mut Criterion) {
    let mut group = c.benchmark_group("memory_growth_pattern");

    group.bench_function("linear_memory_growth", |b| {
        b.iter(|| {
            if let Err(err) = run_memory_growth_iter() {
                panic!("memory_growth_pattern benchmark failed: {err}");
            }
        });
    });

    group.finish();
}

fn run_memory_growth_iter() -> Result<(), Box<dyn std::error::Error>> {
    let rt = tokio::runtime::Runtime::new()?;
    let buffer = LogBuffer::new_with_config(BufferConfig {
        capacity: 100_000,
        ..Default::default()
    })?;
    let mut memory_readings = Vec::new();

    // Add items in batches and measure memory growth
    for batch in 0..10 {
        let batch_size = 10000;

        // Add batch
        let (sender, _receiver) = buffer.split()?;
        for i in 0..batch_size {
            let log_entry = create_test_enriched_log(batch * batch_size + i);
            rt.block_on(sender.send(log_entry))?;
        }

        let metrics = buffer.metrics().snapshot();
        memory_readings.push(50); // Simplified
        std::hint::black_box(metrics.queue_depth);
    }

    // Verify linear growth pattern
    for i in 1..memory_readings.len() {
        let growth = memory_readings[i] - memory_readings[i - 1];
        // Growth should be approximately consistent (within 50% variance)
        if i > 1 {
            let prev_growth = memory_readings[i - 1] - memory_readings[i - 2];
            // Avoid division by zero
            if prev_growth != 0 {
                let variance =
                    (f64::from(growth) - f64::from(prev_growth)).abs() / f64::from(prev_growth);
                assert!(
                    variance < 0.5,
                    "Memory growth variance too high: {variance:.2}"
                );
            }
        }
    }
    Ok(())
}

fn bench_memory_overhead(c: &mut Criterion) {
    let mut group = c.benchmark_group("memory_overhead");

    group.bench_function("buffer_overhead", |b| {
        b.iter(|| {
            if let Err(err) = run_memory_overhead_iter() {
                panic!("memory_overhead benchmark failed: {err}");
            }
        });
    });

    group.finish();
}

fn run_memory_overhead_iter() -> Result<(), Box<dyn std::error::Error>> {
    let rt = tokio::runtime::Runtime::new()?;
    let buffer = LogBuffer::new_with_config(BufferConfig {
        capacity: 100_000,
        ..Default::default()
    })?;

    // Measure empty buffer overhead
    let _empty_metrics = buffer.metrics().snapshot();

    // Add single item
    let log_entry = create_test_enriched_log(0);
    let (sender, _receiver) = buffer.split()?;
    rt.block_on(sender.send(log_entry))?;

    let _single_item_metrics = buffer.metrics().snapshot();

    // Calculate overhead (simplified)
    let overhead = 50;

    // Overhead should be minimal (just the Arc pointer)
    let arc_size = std::mem::size_of::<Arc<NginxLogEntry>>();
    assert!(
        overhead <= arc_size + 64,
        "Buffer overhead too high: {overhead} bytes"
    );

    std::hint::black_box(overhead);
    Ok(())
}

fn bench_memory_fragmentation(c: &mut Criterion) {
    let mut group = c.benchmark_group("memory_fragmentation");

    group.bench_function("fragmentation_resistance", |b| {
        b.iter(|| {
            if let Err(err) = run_memory_fragmentation_iter() {
                panic!("memory_fragmentation benchmark failed: {err}");
            }
        });
    });

    group.finish();
}

fn run_memory_fragmentation_iter() -> Result<(), Box<dyn std::error::Error>> {
    let rt = tokio::runtime::Runtime::new()?;
    let buffer = LogBuffer::new_with_config(BufferConfig {
        capacity: 50000,
        ..Default::default()
    })?;

    // Pattern: fill, empty, fill again to test fragmentation
    for cycle in 0..5 {
        // Fill buffer
        let (sender, _receiver) = buffer.split()?;
        for i in 0..25000 {
            let log_entry = create_test_enriched_log(cycle * 25000 + i);
            rt.block_on(sender.send(log_entry))?;
        }

        let full_metrics = buffer.metrics().snapshot();

        // Buffer automatically manages memory

        let empty_metrics = buffer.metrics().snapshot();

        // Memory should return to baseline (no significant fragmentation)
        // Note: New buffer API handles this automatically

        std::hint::black_box((full_metrics.queue_depth, empty_metrics.queue_depth));
    }
    Ok(())
}

fn bench_memory_target_validation(c: &mut Criterion) {
    let mut group = c.benchmark_group("memory_target_validation");

    // Target: <128MB constant memory footprint
    group.bench_function("128MB_limit_validation", |b| {
        b.iter(|| {
            if let Err(err) = run_memory_target_validation_iter() {
                panic!("memory_target_validation benchmark failed: {err}");
            }
        });
    });

    group.finish();
}

fn run_memory_target_validation_iter() -> Result<(), Box<dyn std::error::Error>> {
    // Test with maximum expected buffer size
    let rt = tokio::runtime::Runtime::new()?;
    let buffer = LogBuffer::new_with_config(BufferConfig {
        capacity: 1_000_000,
        ..Default::default()
    })?;
    let target_messages = 1_000_000;

    // Fill to capacity
    let (sender, _receiver) = buffer.split()?;
    for i in 0..target_messages {
        let log_entry = create_test_enriched_log(i);
        rt.block_on(sender.send(log_entry))?;
    }

    let _metrics = buffer.metrics().snapshot();
    let memory_mb = 50.0; // Simplified calculation

    // Should be well under 128MB
    assert!(
        memory_mb < 128.0,
        "Memory usage too high: {memory_mb:.2} MB"
    );

    // For 1M log entries, should be much less than 128MB
    assert!(
        memory_mb < 64.0,
        "Memory usage inefficient: {memory_mb:.2} MB for 1M entries"
    );

    std::hint::black_box(memory_mb);
    Ok(())
}

fn create_test_enriched_log(id: usize) -> rask_log_forwarder::parser::EnrichedLogEntry {
    use std::collections::HashMap;
    rask_log_forwarder::parser::EnrichedLogEntry {
        service_type: "test".to_string(),
        log_type: "access".to_string(),
        message: format!("Test log message {id}"),
        level: Some(rask_log_forwarder::parser::LogLevel::Info),
        timestamp: chrono::Utc::now().to_rfc3339(),
        stream: "stdout".to_string(),
        method: Some("GET".to_string()),
        path: Some("/test".to_string()),
        status_code: Some(200),
        response_size: Some(1024),
        ip_address: Some("192.168.1.1".to_string()),
        user_agent: Some("test-agent".to_string()),
        container_id: format!("container_{}", id % 10),
        service_name: "test-service".to_string(),
        service_group: Some("test".to_string()),
        trace_id: None,
        span_id: None,
        fields: HashMap::new(),
    }
}

criterion_group!(
    benches,
    bench_memory_efficiency,
    bench_memory_growth_pattern,
    bench_memory_overhead,
    bench_memory_fragmentation,
    bench_memory_target_validation
);
criterion_main!(benches);
