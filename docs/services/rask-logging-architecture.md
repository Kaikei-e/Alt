# Rask Logging Architecture

_Last reviewed: February 28, 2026_

## Overview

**rask-log-aggregator** and **rask-log-forwarder** are complementary services in the Alt project's log collection pipeline, each handling different responsibilities.

## Architecture Diagram

```
+-------------------------------------------------------------------------+
|                          Log Collection Pipeline                         |
+-------------------------------------------------------------------------+
|                                                                         |
|  +------------------+         +----------------------+                  |
|  | Docker Container |         |   OTel SDK-enabled   |                  |
|  | (Microservices)  |         |   Services           |                  |
|  | - alt-backend    |         | (Go/Python)          |                  |
|  | - nginx          |         |                      |                  |
|  | - etc.           |         +----------+-----------+                  |
|  +--------+---------+                    |                              |
|           |                              |                              |
|           v                              |                              |
|  +------------------+                    |                              |
|  | rask-log-forwarder|                   |                              |
|  | (15 instances)    |                   |                              |
|  | ---------------- |                    |                              |
|  | - Docker API     |                    |                              |
|  | - SIMD JSON      |                    |                              |
|  | - Zero-copy      |                    |                              |
|  | - Backpressure   |                    |                              |
|  | - Disk fallback  |                    |                              |
|  +--------+---------+                    |                              |
|           |                              |                              |
|           | POST /v1/aggregate           | POST /v1/logs               |
|           | (NDJSON)                     | POST /v1/traces             |
|           | :9600                        | (protobuf)                  |
|           |                              | :4318                       |
|           v                              v                              |
|  +------------------------------------------------------+              |
|  |               rask-log-aggregator                     |              |
|  |  ---------------------------------------------------- |              |
|  |  Main Server :9600      |    OTLP Server :4318        |              |
|  |  - GET /v1/health       |    - POST /v1/logs (OTLP)   |              |
|  |  - POST /v1/aggregate   |    - POST /v1/traces (OTLP) |              |
|  |    (legacy NDJSON)      |                             |              |
|  +-------------------------+-----------------------------+              |
|                             |                                           |
|                             v                                           |
|                    +-----------------+                                  |
|                    |   ClickHouse    |                                  |
|                    | --------------- |                                  |
|                    | - logs (legacy) |                                  |
|                    | - otel_logs     |                                  |
|                    | - otel_traces   |                                  |
|                    +-----------------+                                  |
+---------------------------------------------------------------------------+
```

## Role Comparison

| Aspect | rask-log-forwarder | rask-log-aggregator |
|--------|-------------------|---------------------|
| **Role** | Log collection/forwarding (sidecar) | Log aggregation/persistence (central server) |
| **Deployment** | One instance per service (15 total) | Singleton (1 instance) |
| **Data Source** | Docker API (bollard) | HTTP endpoints |
| **Data Destination** | rask-log-aggregator | ClickHouse |
| **Protocol** | HTTP POST (NDJSON) | OTLP HTTP (protobuf) + legacy NDJSON |
| **OTel Compliance** | Not supported | OTLP HTTP compliant (logs/traces) |

## Detailed Analysis

### rask-log-forwarder (Sidecar)

**Purpose**: High-performance collection and forwarding of Docker container logs to aggregator

**Key Features**:
1. Docker API streaming via `bollard` crate
2. SIMD JSON parser (`simd-json`) for >4 GB/s throughput
3. Zero-allocation parser for memory efficiency
4. Lock-free buffer + backpressure control
5. Disk fallback (`sled`) during aggregator outages
6. Exponential backoff retry

**OTel Relationship**:
- Not OTel compliant
- Sends proprietary NDJSON format to aggregator
- No OTel dependencies in Cargo.toml

**Key Files**:
- `rask-log-forwarder/app/src/collector/docker.rs` - Docker log collection
- `rask-log-forwarder/app/src/parser/simd.rs` - SIMD parser
- `rask-log-forwarder/app/src/sender/http.rs` - HTTP transmission
- `rask-log-forwarder/app/src/reliability/disk.rs` - Disk fallback

### rask-log-aggregator (Central Server)

**Purpose**: Aggregate logs from multiple sources and persist to ClickHouse

**Key Features**:
1. **Dual Server Architecture**:
   - Main Server (:9600) - legacy NDJSON endpoint
   - OTLP Server (:4318) - OTel HTTP endpoint

2. **OTel Compliant Implementation**:
   - `POST /v1/logs` - OTLP HTTP log reception (protobuf)
   - `POST /v1/traces` - OTLP HTTP trace reception (protobuf)
   - Domain models compliant with OpenTelemetry Log/Span Data Model

3. **Exporter Abstraction**:
   - `LogExporter` trait - for legacy logs
   - `OTelExporter` trait - for OTel logs/traces
   - `ClickHouseExporter` - implements both traits

**OTel Dependencies** (Cargo.toml):
```toml
opentelemetry-proto = { version = "0.31", features = ["gen-tonic", "logs", "trace"] }
prost = "0.14"
tonic = { version = "0.13", features = ["transport"] }
```

**Key Files**:
- `rask-log-aggregator/app/src/otlp/receiver.rs` - OTLP HTTP handlers
- `rask-log-aggregator/app/src/otlp/converter.rs` - protobuf to domain model conversion
- `rask-log-aggregator/app/src/domain/otel_log.rs` - OTel domain models
- `rask-log-aggregator/app/src/log_exporter/clickhouse_exporter.rs` - ClickHouse output

## Data Flow

### Path 1: Docker Container Logs (via forwarder)

```
Docker Container -> rask-log-forwarder -> POST /v1/aggregate -> aggregator -> ClickHouse (logs table)
```

- Format: NDJSON (newline-delimited JSON)
- Port: 9600
- TTL: 2 days

### Path 2: OTel SDK-enabled Services (direct OTLP)

```
Go/Python Service (OTel SDK) -> POST /v1/logs or /v1/traces -> aggregator -> ClickHouse (otel_logs / otel_traces tables)
```

- Format: protobuf (application/x-protobuf)
- Port: 4318
- TTL: 7 days

## Rationale for Separation

1. **Integration with Existing Docker Infrastructure**: Forwarder directly monitors Docker API, transparently collecting container logs without application changes.

2. **Integration with OTel Ecosystem**: Aggregator provides OTLP HTTP endpoints, allowing services using OTel SDK to send traces/logs directly.

3. **Gradual Migration**: Supporting both legacy NDJSON and OTLP formats enables gradual OTel migration.

## Current Limitations

1. **Forwarder is not OTel-compliant**: Forwarder only supports NDJSON, no OTLP format transmission
2. **gRPC not implemented**: Aggregator's OTLP is HTTP only, gRPC (4317) is not implemented
3. **Tracing from forwarder**: Forwarder can attach trace_id/span_id to logs but does not generate spans

## Performance Comparison

### Path 1: Docker -> forwarder -> aggregator (NDJSON)

| Metric | Value | Notes |
|--------|-------|-------|
| **Parse throughput** | >4 GB/s | SIMD JSON (`simd-json` crate) |
| **Buffer throughput** | 1M+ msg/sec | Lock-free implementation |
| **Batch size** | 10,000 entries | Default, configurable |
| **Flush interval** | 500ms | Configurable |
| **Buffer capacity** | 100,000 entries | Default |
| **HTTP compression** | gzip (optional) | `ENABLE_COMPRESSION=true` |
| **Retry** | Exponential backoff | Max 5 attempts, base 500ms |
| **Disk fallback** | `sled` DB | On aggregator failure |

**Optimization Technologies**:
- `simd-json`: AVX2/NEON instructions for JSON parsing acceleration
- `bumpalo`: Arena allocator for temporary memory management
- Zero-allocation numeric parser: Avoids heap allocation during numeric conversion
- Build-time regex validation: Pre-compiled patterns reduce runtime overhead
- `parking_lot`: High-performance lock primitives

### Path 2: OTel SDK -> aggregator (OTLP HTTP/protobuf)

| Metric | Value | Notes |
|--------|-------|-------|
| **Protocol** | HTTP/protobuf | `application/x-protobuf` |
| **Payload size** | Small (binary) | 30-50% reduction vs JSON |
| **Decoding** | `prost` | Rust protobuf implementation |
| **Conversion overhead** | Low | Simple field mapping |
| **Batch processing** | Yes | Batched on SDK side |

**OTel SDK Optimizations**:
- Built-in SDK batching (typically 5 seconds or 512 spans)
- Efficient serialization via protobuf
- Automatic retry functionality

### Performance Characteristics Comparison

| Aspect | NDJSON Path (forwarder) | OTLP Path (direct) |
|--------|------------------------|-------------------|
| **Latency** | Slightly higher (2 hops) | Low (1 hop) |
| **Throughput** | Ultra-high (>4GB/s) | SDK-dependent |
| **Payload efficiency** | Normal (JSON) | High (protobuf) |
| **Reliability** | High (disk fallback) | SDK-dependent |
| **Instrumentation cost** | Zero (Docker API monitoring) | Code changes required |
| **Structured data** | Limited (parse-dependent) | Complete (schema-defined) |
| **Trace correlation** | None | Full (trace_id/span_id) |

### Benchmark Results (forwarder)

Benchmarks measurable with `cargo bench`:

```
nginx_parsing/simd_docker_log    - Docker JSON log basic parse
nginx_parsing/simd_access_log    - Nginx access log full parse
nginx_parsing/simd_error_log     - Nginx error log parse

single_threaded_push/1000        - 1000 entry push
single_threaded_push/10000       - 10000 entry push
single_threaded_push/100000      - 100000 entry push

high_throughput_target/1M_messages_sustained - 1M messages sustained throughput
```

### Recommended Use Cases

| Use Case | Recommended Path | Reason |
|----------|-----------------|--------|
| **Existing Docker apps** | forwarder | No instrumentation needed, immediate deployment |
| **High throughput requirements** | forwarder | SIMD optimization for >4GB/s |
| **Distributed tracing** | Direct OTLP | Full trace_id/span_id correlation |
| **New Go/Python apps** | Direct OTLP | Easy OTel SDK integration |
| **Network bandwidth constraints** | Direct OTLP | Payload reduction via protobuf |

## Conclusion

- **rask-log-forwarder**: High-performance sidecar specialized for Docker container log collection. Not OTel compliant.
- **rask-log-aggregator**: OTel-compliant central log aggregation server. Supports both OTLP HTTP and legacy NDJSON.

Both services handle different layers - "collection/forwarding" and "aggregation/persistence" - and operate complementarily.

### Performance Summary

- **NDJSON Path**: Ultra-high throughput (>4GB/s), zero instrumentation cost, high reliability via disk fallback
- **OTLP Path**: Low latency (1 hop), efficient payload (protobuf), full trace correlation

Both paths provide sufficient performance for production use, and choosing based on use case is recommended.

---

## Benchmark Execution Plan

### 1. Forwarder Parser Benchmark

```bash
cd rask-log-forwarder/app
cargo bench --bench parser_benchmarks
```

**Measured Items**:
- `simd_docker_log`: Docker JSON basic parse speed
- `simd_access_log`: Nginx access log full parse
- `simd_error_log`: Nginx error log parse

### 2. Forwarder Buffer Throughput Benchmark

```bash
cd rask-log-forwarder/app
cargo bench --bench throughput_benchmarks
```

**Measured Items**:
- `single_threaded_push`: 1K/10K/100K entry push speed
- `batch_operations`: Batch push/pop performance
- `concurrent_access`: Multi-threaded push performance
- `high_throughput_target`: 1M messages sustained throughput

### 3. Forwarder Memory Benchmark

```bash
cd rask-log-forwarder/app
cargo bench --bench memory_benchmarks
```

**Measured Items**:
- Memory overhead per message
- Relationship between buffer capacity and memory usage

### 4. Verification Method

Benchmark results are saved in HTML format under `target/criterion/` directory.

```bash
# Open results (Linux)
xdg-open target/criterion/report/index.html

# Open results (macOS)
open target/criterion/report/index.html
```

### Expected Results

| Benchmark | Expected Value |
|-----------|----------------|
| simd_docker_log | >4 GB/s |
| single_threaded_push/100000 | <1 second |
| 1M_messages_sustained | Within seconds |

## Related Documentation

- [rask-log-aggregator.md](./rask-log-aggregator.md) - Aggregator detailed documentation
- [rask-log-forwarder.md](./rask-log-forwarder.md) - Forwarder detailed documentation
- [MICROSERVICES.md](./MICROSERVICES.md) - Full service reference
