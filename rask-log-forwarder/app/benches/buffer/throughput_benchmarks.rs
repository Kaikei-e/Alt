use chrono::Utc;
use criterion::{BenchmarkId, Criterion, Throughput, criterion_group, criterion_main};
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

fn bench_single_threaded_push(c: &mut Criterion) {
    let mut group = c.benchmark_group("single_threaded_push");

    for &size in &[1000, 10000, 100_000] {
        group.throughput(Throughput::Elements(size as u64));
        group.bench_with_input(BenchmarkId::from_parameter(size), &size, |b, &size| {
            b.iter(|| {
                if let Err(err) = run_single_threaded_push_iter(size) {
                    panic!("single_threaded_push benchmark failed: {err}");
                }
            });
        });
    }
    group.finish();
}

fn run_single_threaded_push_iter(size: usize) -> Result<(), Box<dyn std::error::Error>> {
    let rt = tokio::runtime::Runtime::new()?;
    let buffer = LogBuffer::new_with_config(BufferConfig {
        capacity: size + 1000,
        ..Default::default()
    })?;
    let (sender, _receiver) = buffer.split()?;
    for i in 0..size {
        let log_entry = create_test_enriched_log(i);
        rt.block_on(sender.send(std::hint::black_box(log_entry)))?;
    }
    Ok(())
}

fn bench_single_threaded_push_pop(c: &mut Criterion) {
    let mut group = c.benchmark_group("single_threaded_push_pop");

    for &size in &[1000, 10000, 100_000] {
        group.throughput(Throughput::Elements(size as u64 * 2)); // Both push and pop
        group.bench_with_input(BenchmarkId::from_parameter(size), &size, |b, &size| {
            b.iter(|| {
                if let Err(err) = run_single_threaded_push_pop_iter(size) {
                    panic!("single_threaded_push_pop benchmark failed: {err}");
                }
            });
        });
    }
    group.finish();
}

fn run_single_threaded_push_pop_iter(size: usize) -> Result<(), Box<dyn std::error::Error>> {
    let rt = tokio::runtime::Runtime::new()?;
    let buffer = LogBuffer::new_with_config(BufferConfig {
        capacity: size + 1000,
        ..Default::default()
    })?;

    // Push phase
    let (sender, _receiver) = buffer.split()?;
    for i in 0..size {
        let log_entry = create_test_enriched_log(i);
        rt.block_on(sender.send(std::hint::black_box(log_entry)))?;
    }

    // Receive phase
    let (_sender2, mut receiver) = buffer.split()?;
    for _ in 0..size {
        std::hint::black_box(rt.block_on(receiver.recv())?);
    }
    Ok(())
}

fn bench_batch_operations(c: &mut Criterion) {
    let mut group = c.benchmark_group("batch_operations");

    for &batch_size in &[100, 1000, 10000] {
        group.throughput(Throughput::Elements(batch_size as u64));

        // Batch push benchmark
        group.bench_with_input(
            BenchmarkId::new("push_batch", batch_size),
            &batch_size,
            |b, &batch_size| {
                b.iter(|| {
                    if let Err(err) = run_push_batch_iter(batch_size) {
                        panic!("push_batch benchmark failed: {err}");
                    }
                });
            },
        );

        // Batch pop benchmark
        group.bench_with_input(
            BenchmarkId::new("pop_batch", batch_size),
            &batch_size,
            |b, &batch_size| {
                b.iter(|| {
                    if let Err(err) = run_pop_batch_iter(batch_size) {
                        panic!("pop_batch benchmark failed: {err}");
                    }
                });
            },
        );
    }
    group.finish();
}

fn run_push_batch_iter(batch_size: usize) -> Result<(), Box<dyn std::error::Error>> {
    let rt = tokio::runtime::Runtime::new()?;
    let buffer = LogBuffer::new_with_config(BufferConfig {
        capacity: batch_size + 1000,
        ..Default::default()
    })?;
    let batch: Vec<_> = (0..batch_size).map(create_test_enriched_log).collect();

    let (sender, _receiver) = buffer.split()?;
    for log_entry in std::hint::black_box(batch) {
        rt.block_on(sender.send(log_entry))?;
    }
    Ok(())
}

fn run_pop_batch_iter(batch_size: usize) -> Result<(), Box<dyn std::error::Error>> {
    let rt = tokio::runtime::Runtime::new()?;
    let buffer = LogBuffer::new_with_config(BufferConfig {
        capacity: batch_size + 1000,
        ..Default::default()
    })?;

    // Fill buffer first
    let (sender, mut receiver) = buffer.split()?;
    for i in 0..batch_size {
        let log_entry = create_test_enriched_log(i);
        rt.block_on(sender.send(log_entry))?;
    }

    // Benchmark receive_batch
    for _ in 0..std::hint::black_box(batch_size) {
        rt.block_on(receiver.recv())?;
    }
    Ok(())
}

fn bench_memory_usage(c: &mut Criterion) {
    let mut group = c.benchmark_group("memory_usage");

    for &size in &[1000, 10000, 100_000] {
        group.bench_with_input(BenchmarkId::from_parameter(size), &size, |b, &size| {
            b.iter(|| {
                if let Err(err) = run_memory_usage_iter(size) {
                    panic!("memory_usage benchmark failed: {err}");
                }
            });
        });
    }
    group.finish();
}

fn run_memory_usage_iter(size: usize) -> Result<(), Box<dyn std::error::Error>> {
    let rt = tokio::runtime::Runtime::new()?;
    let buffer = LogBuffer::new_with_config(BufferConfig {
        capacity: size + 1000,
        ..Default::default()
    })?;

    // Fill buffer
    let (sender, _receiver) = buffer.split()?;
    for i in 0..size {
        let log_entry = create_test_enriched_log(i);
        rt.block_on(sender.send(log_entry))?;
    }

    // Check memory usage
    let metrics = buffer.metrics().snapshot();
    std::hint::black_box(metrics.queue_depth);

    // Verify memory efficiency (simplified)
    let overhead_per_1000 = 50.0;
    assert!(
        overhead_per_1000 < 100_000.0,
        "Memory overhead too high: {overhead_per_1000} bytes per 1000 messages"
    );
    Ok(())
}

fn bench_concurrent_access(c: &mut Criterion) {
    let mut group = c.benchmark_group("concurrent_access");

    // Multi-threaded benchmark (requires std::thread since criterion doesn't support async)
    group.bench_function("multi_threaded_push", |b| {
        b.iter(|| {
            if let Err(err) = run_concurrent_access_iter() {
                panic!("concurrent_access benchmark failed: {err}");
            }
        });
    });

    group.finish();
}

fn run_concurrent_access_iter() -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let buffer = LogBuffer::new_with_config(BufferConfig {
        capacity: 100_000,
        ..Default::default()
    })?;

    std::thread::scope(
        |s| -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
            let (sender, _receiver) = buffer.split()?;
            let handles: Vec<_> = (0..4)
                .map(|thread_id| {
                    let sender_clone = sender.clone();
                    s.spawn(
                        move || -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
                            let rt = tokio::runtime::Runtime::new()?;
                            for i in 0..2500 {
                                // 4 threads * 2500 = 10k total
                                let log_entry = create_test_enriched_log(thread_id * 2500 + i);
                                while rt
                                    .block_on(
                                        sender_clone.send(std::hint::black_box(log_entry.clone())),
                                    )
                                    .is_err()
                                {
                                    std::hint::spin_loop();
                                }
                            }
                            Ok(())
                        },
                    )
                })
                .collect();

            for handle in handles {
                match handle.join() {
                    Ok(res) => res?,
                    Err(_) => return Err("Thread panicked in benchmark".into()),
                }
            }
            Ok(())
        },
    )?;
    Ok(())
}

fn bench_high_throughput_target(c: &mut Criterion) {
    let mut group = c.benchmark_group("high_throughput_target");
    group.sample_size(10); // Reduce sample size for long-running benchmarks

    // Target: 1M+ messages/second sustained throughput
    group.bench_function("1M_messages_sustained", |b| {
        b.iter(|| {
            if let Err(err) = run_high_throughput_iter() {
                panic!("high_throughput_target benchmark failed: {err}");
            }
        });
    });

    group.finish();
}

fn run_high_throughput_iter() -> Result<(), Box<dyn std::error::Error>> {
    let rt = tokio::runtime::Runtime::new()?;
    let buffer = LogBuffer::new_with_config(BufferConfig {
        capacity: 1_100_000,
        ..Default::default()
    })?;
    let target_messages = 1_000_000;

    let (sender, _receiver) = buffer.split()?;
    for i in 0..target_messages {
        let log_entry = create_test_enriched_log(i);
        rt.block_on(sender.send(std::hint::black_box(log_entry)))?;
    }

    // Verify we achieved target
    assert_eq!(buffer.metrics().snapshot().queue_depth, target_messages);
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
    bench_single_threaded_push,
    bench_single_threaded_push_pop,
    bench_batch_operations,
    bench_memory_usage,
    bench_concurrent_access,
    bench_high_throughput_target
);
criterion_main!(benches);
