use bytes::Bytes;
use criterion::{Criterion, Throughput, criterion_group, criterion_main};
use rask_log_forwarder::parser::SimdParser;

fn benchmark_simd_nginx_parsing(c: &mut Criterion) {
    let access_log = r#"{"log":"192.168.1.1 - - [25/Dec/2023:10:00:00 +0000] \"GET /api/health HTTP/1.1\" 200 612 \"-\" \"curl/7.68.0\"\n","stream":"stdout","time":"2023-12-25T10:00:00.000000000Z"}"#;
    let error_log = r#"{"log":"2023/12/25 10:00:00 [error] 29#29: *1 connect() failed (111: Connection refused) while connecting to upstream\n","stream":"stderr","time":"2023-12-25T10:00:00.000000000Z"}"#;

    let mut group = c.benchmark_group("nginx_parsing");
    group.throughput(Throughput::Bytes(access_log.len() as u64));

    group.bench_function("simd_docker_log", |b| {
        let parser = SimdParser::new();
        b.iter(|| {
            let bytes = Bytes::from(std::hint::black_box(access_log));
            parser.parse_docker_log(bytes)
        });
    });

    group.bench_function("simd_access_log", |b| {
        let parser = SimdParser::new();
        b.iter(|| {
            let bytes = Bytes::from(std::hint::black_box(access_log));
            parser.parse_nginx_log(bytes)
        });
    });

    group.bench_function("simd_error_log", |b| {
        let parser = SimdParser::new();
        b.iter(|| {
            let bytes = Bytes::from(std::hint::black_box(error_log));
            parser.parse_nginx_log(bytes)
        });
    });

    group.finish();
}

fn benchmark_throughput_target(c: &mut Criterion) {
    let parser = SimdParser::new();
    let sample_logs: Vec<&str> = vec![
        r#"{"log":"192.168.1.1 - - [25/Dec/2023:10:00:00 +0000] \"GET /api/users HTTP/1.1\" 200 1024\n","stream":"stdout","time":"2023-12-25T10:00:00.000000000Z"}"#,
        r#"{"log":"10.0.0.1 - - [25/Dec/2023:10:01:00 +0000] \"POST /api/data HTTP/1.1\" 201 2048\n","stream":"stdout","time":"2023-12-25T10:01:00.000000000Z"}"#,
        r#"{"log":"2023/12/25 10:02:00 [error] 29#29: *1 connect() failed\n","stream":"stderr","time":"2023-12-25T10:02:00.000000000Z"}"#,
    ];

    c.bench_function("throughput_4gb_per_sec", |b| {
        b.iter(|| {
            for log in &sample_logs {
                let bytes = Bytes::from(*log);
                std::hint::black_box(parser.parse_nginx_log(bytes).ok());
            }
        });
    });
}

criterion_group!(
    benches,
    benchmark_simd_nginx_parsing,
    benchmark_throughput_target
);
criterion_main!(benches);
