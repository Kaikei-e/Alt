# VERIFY-05 Flaky Hunt Evidence & Unregistered Test Audit

- **Date**: 2026-11-26
- **Scope**:
  1. Pure-Go stdlib flaky test hunter (`scripts/altharness/internal/flakyhunt/` + `scripts/verify/flaky-hunt.sh`, ADR-001028 compliant).
  2. `pre-processor/app` flaky test hunt, deterministic seed reproduction, TDD regression tests, fixes, and post-fix verification.
  3. `rask-log-forwarder/app/tests/` unregistered test files audit (Learnings §2.6 / §5 / ISSUE-09).

---

## 1. Summary of `pre-processor/app` Flakes Found & Fixed

Running `flaky-hunt` with `-shuffle=on -race` and CPU contention (`--stress-procs 1` / `--stress-procs 2`) exposed **two distinct real flakes** in `pre-processor/app` — both deterministically reproducible with exact Go shuffle seeds:

| # | Package | Failing Test(s) | Captured Seed(s) | Failure Kind | Root Cause | Fix & Pinned Test |
|---|---|---|---|---|---|---|
| 1 | `pre-processor/service` | `TestEnvoyHTTPClient_ErrorHandling/malformed_proxy_url`, `TestEnvoyHTTPClient_TimeoutHandling/fast_timeout` | `1795714559739160007`, `1795714611083909633`, `1795714634738483214`, `1795714675705910891` | `panic` (`SIGSEGV` nil pointer dereference) | `HTTPClientWrapper.Get` (`service/article_fetcher.go:139`) called `GetGlobalProxyMetrics(nil)`, permanently poisoning the `sync.Once` global singleton with `logger == nil`. Any subsequent `RecordError`/`RecordConfigSwitch`/periodic summary panicked at `service/proxy_metrics.go:163`. Also `m.totalRequests%50` was an unsynchronized read after `atomic.AddUint64`, and `TestEnvoyHTTPClient_ErrorHandling` did not stub `resolveIP`. | `NewProxyMetrics` defaults `nil` logger to `slog.Default()`; `article_fetcher.go:139` passes `slog.Default()`; atomic return values used for `%50` / `%25` checks; `withStubResolver` added in `TestEnvoyHTTPClient_ErrorHandling`; pinned by `TestProxyMetrics_NilLoggerAndConcurrentRecording`. |
| 2 | `pre-processor/test/integration` | `TestLoggingPerformance_MemoryUsage` | `1795714964615587528`, `1795715026366645553` | `test_fail` (`uint64` underflow) | `logging_performance_test.go:232` computed `memoryUsed := finalAlloc - baselineAlloc` on unsigned `uint64` `runtime.MemStats.Alloc` live-heap gauges. When the second GC reclaimed slightly more prior-test allocations than the baseline GC (e.g. `228768 -> 228480` or `168800 -> 168704`), `uint64` subtraction wrapped to `~1.8446744e+19`, failing `< 500 bytes/op`. Explains Learnings §6 (~1/10 failure under parallel build load even without `-shuffle`). | Added `retainedHeapDelta(baselineAlloc, finalAlloc uint64) uint64` guarding `finalAlloc <= baselineAlloc`; pinned by `TestRetainedHeapDelta_DoesNotUnderflowOnShrink`. |

---

## 2. Flake #1 — `pre-processor/service` Global `ProxyMetrics` Nil-Logger Singleton Poisoning

### 2.1 Pre-Fix Detection (`flaky-hunt`)

Command:
```bash
flaky-hunt pre-processor/app --rounds 15 --race=true --stress-procs 1 \
  --pkg ./quality-checker --pkg ./service --evidence-dir /tmp/fh-mid --format text
```

Output (4/15 rounds failed, all in `pre-processor/service`):
```text
FLAKY-HUNT [FAIL] module=pre-processor/app lang=go rounds=15 passed=11 failed=4 failures=8 duration_ms=383893
evidence_dir: /tmp/fh-mid
  - round=1 kind=test_fail pkg=pre-processor/service test=TestEnvoyHTTPClient_ErrorHandling/malformed_proxy_url seed=1795714559739160007
    repro: GOMAXPROCS=1 go -C pre-processor/app test -count=1 -shuffle=1795714559739160007 -race ./service
    log:   /tmp/fh-mid/round-01-failure.log
  - round=1 kind=panic pkg=pre-processor/service test=TestEnvoyHTTPClient_ErrorHandling seed=1795714559739160007
    repro: GOMAXPROCS=1 go -C pre-processor/app test -count=1 -shuffle=1795714559739160007 -race ./service
    log:   /tmp/fh-mid/round-01-failure.log
  - round=3 kind=test_fail pkg=pre-processor/service test=TestEnvoyHTTPClient_ErrorHandling/malformed_proxy_url seed=1795714611083909633
    repro: GOMAXPROCS=1 go -C pre-processor/app test -count=1 -shuffle=1795714611083909633 -race ./service
    log:   /tmp/fh-mid/round-03-failure.log
  - round=3 kind=panic pkg=pre-processor/service test=TestEnvoyHTTPClient_ErrorHandling seed=1795714611083909633
    repro: GOMAXPROCS=1 go -C pre-processor/app test -count=1 -shuffle=1795714611083909633 -race ./service
    log:   /tmp/fh-mid/round-03-failure.log
  - round=4 kind=test_fail pkg=pre-processor/service test=TestEnvoyHTTPClient_TimeoutHandling/fast_timeout seed=1795714634738483214
    repro: GOMAXPROCS=1 go -C pre-processor/app test -count=1 -shuffle=1795714634738483214 -race ./service
    log:   /tmp/fh-mid/round-04-failure.log
  - round=4 kind=panic pkg=pre-processor/service test=TestEnvoyHTTPClient_TimeoutHandling seed=1795714634738483214
    repro: GOMAXPROCS=1 go -C pre-processor/app test -count=1 -shuffle=1795714634738483214 -race ./service
    log:   /tmp/fh-mid/round-04-failure.log
  - round=6 kind=test_fail pkg=pre-processor/service test=TestEnvoyHTTPClient_ErrorHandling/malformed_proxy_url seed=1795714675705910891
    repro: GOMAXPROCS=1 go -C pre-processor/app test -count=1 -shuffle=1795714675705910891 -race ./service
    log:   /tmp/fh-mid/round-06-failure.log
  - round=6 kind=panic pkg=pre-processor/service test=TestEnvoyHTTPClient_ErrorHandling seed=1795714675705910891
    repro: GOMAXPROCS=1 go -C pre-processor/app test -count=1 -shuffle=1795714675705910891 -race ./service
    log:   /tmp/fh-mid/round-06-failure.log
```

### 2.2 Pre-Fix Deterministic Seed Reproduction

**Seed `1795714634738483214` (`TestEnvoyHTTPClient_TimeoutHandling/fast_timeout`, reproduces in 0.37s)**:
```bash
go -C pre-processor/app test -count=1 -shuffle=1795714634738483214 -race ./service
```
```text
--- FAIL: TestEnvoyHTTPClient_TimeoutHandling (0.10s)
    --- FAIL: TestEnvoyHTTPClient_TimeoutHandling/fast_timeout (0.10s)
panic: runtime error: invalid memory address or nil pointer dereference [recovered, repanicked]
[signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x989ad5]

goroutine 577 [running]:
...
log/slog.(*Logger).Warn(...)
	.../src/log/slog/logger.go:225
pre-processor/service.(*ProxyMetrics).RecordError(0xc0002a8120, {0xdf065c, 0xd})
	.../pre-processor/app/service/proxy_metrics.go:163 +0x47c
pre-processor/service.(*EnvoyHTTPClient).Get(0xc000836320, {0x14ea790, 0x163e500}, {0xdf5b25, 0x13})
	.../pre-processor/app/service/envoy_http_client.go:193 +0x22bc
pre-processor/service.TestEnvoyHTTPClient_TimeoutHandling.func1(0xc000384fc8)
	.../pre-processor/app/service/envoy_http_client_test.go:309 +0x253
FAIL	pre-processor/service	0.369s
```

**Seed `1795714559739160007` (`TestEnvoyHTTPClient_ErrorHandling/malformed_proxy_url`)**:
```bash
go -C pre-processor/app test -count=1 -shuffle=1795714559739160007 -race ./service
```
```text
--- FAIL: TestEnvoyHTTPClient_ErrorHandling (0.00s)
    --- FAIL: TestEnvoyHTTPClient_ErrorHandling/malformed_proxy_url (0.00s)
panic: runtime error: invalid memory address or nil pointer dereference [recovered, repanicked]
...
pre-processor/service.(*ProxyMetrics).RecordError(0xc0002b65a0, {0xdec949, 0x9})
	.../pre-processor/app/service/proxy_metrics.go:163 +0x47c
pre-processor/service.(*EnvoyHTTPClient).Get(0xc000682280, {0x14ea790, 0x163e500}, {0xdf5b25, 0x13})
	.../pre-processor/app/service/envoy_http_client.go:120 +0xa57
FAIL	pre-processor/service	10.337s
```

### 2.3 Root Cause & Why It Was Hidden Without `-shuffle`

1. In default alphabetical test execution, `TestEnvoyHTTPClient_*` (`E`) runs before `TestHealthCheckerFactory_*` (`H`), so `GetGlobalProxyMetrics(slog.Default())` always won `metricsOnce.Do(...)`.
2. Under `-shuffle`, whenever any test exercising `HTTPClientWrapper.Get` (`TestHealthCheckerFactory_CheckHealth` or `TestHealthCheckerFactory_WaitForHealthy`) runs first, `pre-processor/app/service/article_fetcher.go:139` calls `GetGlobalProxyMetrics(nil)`.
3. `NewProxyMetrics(nil)` stored `logger: nil` in the process-wide `globalProxyMetrics` singleton. Every subsequent call to `RecordError` (`proxy_metrics.go:163`), `RecordConfigSwitch` (`proxy_metrics.go:176`), `logPerformanceMetrics` (`proxy_metrics.go:262`), `extractDomain` (`proxy_metrics.go:556`), or `logDomainMetrics` (`proxy_metrics.go:585`) dereferenced `nil` and panicked with `SIGSEGV`.
4. This is also a real production latent crash if `HTTPClientWrapper.Get` executes before `EnvoyHTTPClient.Get` or `OptimizedHTTPClientWrapper.Get`.
5. Two adjacent defects were fixed at the same time:
   - `proxy_metrics.go:111,145` read `m.totalRequests%50` without atomics right after `atomic.AddUint64(&m.totalRequests, 1)` (data race under concurrent recording); `domainMetrics.TotalRequests%25` updated to use the atomic return value as well.
   - `envoy_http_client_test.go:376` (`TestEnvoyHTTPClient_ErrorHandling`) did not call `withStubResolver`, causing offline runs to fail on host DNS (`127.0.0.53:53`) instead of testing the malformed proxy URL error path.

### 2.4 Post-Fix Verification (Pinned Seeds)

```text
$ go -C pre-processor/app test -count=1 -shuffle=1795714559739160007 -race ./service
ok  	pre-processor/service	13.759s

$ go -C pre-processor/app test -count=1 -shuffle=1795714634738483214 -race ./service
ok  	pre-processor/service	13.987s
```

---

## 3. Flake #2 — `pre-processor/test/integration` `uint64` Underflow in `TestLoggingPerformance_MemoryUsage`

### 3.1 Pre-Fix Detection & Deterministic Seed Reproduction

Observed in full-module `flaky-hunt` (`--race=true --stress-procs 2`) on Round 7 (`-test.shuffle 1795714964615587528`) and Round 8 (`-test.shuffle 1795715026366645553`):

```text
    logging_performance_test.go:235: Memory usage results:
    logging_performance_test.go:236:   Baseline allocation: 228768 bytes
    logging_performance_test.go:237:   Final allocation:    228480 bytes
    logging_performance_test.go:238:   Memory used:         18446744073709551328 bytes
    logging_performance_test.go:239:   Memory per operation: 1844674407370955.25 bytes
    logging_performance_test.go:243:
        	Error Trace:	.../pre-processor/app/test/integration/logging_performance_test.go:243
        	Error:      	"1.8446744073709552e+15" is not less than "500"
        	Test:       	TestLoggingPerformance_MemoryUsage
        	Messages:   	memory per operation should be less than 500.00 bytes
--- FAIL: TestLoggingPerformance_MemoryUsage (0.22s)
```

Direct reproduction before fix:
```bash
GOMAXPROCS=2 go -C pre-processor/app test -count=1 -shuffle=1795715026366645553 -race ./test/integration
# FAIL	pre-processor/test/integration	2.804s
```

### 3.2 Root Cause

`runtime.MemStats.Alloc` is a live-heap gauge (`uint64`), not a monotonic counter. When background GC/sweeper work from earlier tests finishes between the baseline GC and the final GC, `finalAlloc` can drop below `baselineAlloc` by a few hundred bytes (`168800 -> 168704`, `228768 -> 228480`). Subtracting `finalAlloc - baselineAlloc` in `uint64` wraps to `~1.84e19`, making `memoryPerOperation = 1.84e15` and failing `assert.Less(t, memoryPerOperation, 500.0)`.

### 3.3 Post-Fix Verification (Pinned Seeds)

```text
$ GOMAXPROCS=2 go -C pre-processor/app test -count=1 -shuffle=1795715026366645553 -race ./test/integration
ok  	pre-processor/test/integration	2.992s

$ GOMAXPROCS=2 go -C pre-processor/app test -count=1 -shuffle=1795714964615587528 -race ./test/integration
ok  	pre-processor/test/integration	2.637s
```

---

## 4. Before vs. After `flaky-hunt` Sweeps

| Sweep | Command | Rounds | Result | Failures |
|---|---|---|---|---|
| Pre-fix targeted | `flaky-hunt pre-processor/app --rounds 15 --race=true --stress-procs 1 --pkg ./quality-checker --pkg ./service` | 15 | **FAIL** | 4/15 rounds (`pre-processor/service` nil-logger panic) |
| Pre-fix full (rounds 1–8 observed) | `flaky-hunt pre-processor/app --rounds 10 --race=true --stress-procs 2` | 8 | **FAIL** | 4/8 rounds (R2 & R4 `service` panic; R7 & R8 `test/integration` underflow) |
| Post-fix targeted | `flaky-hunt pre-processor/app --rounds 15 --race=true --stress-procs 1 --pkg ./service --pkg ./test/integration` | 15 | **PASS** | **0/15** (`duration_ms=267309`) |
| Post-fix full (`./...`) | `flaky-hunt pre-processor/app --rounds 10 --race=true` | 10 | **PASS** | **0/10** (`duration_ms=547088`, evidence in `.harness/evidence/flaky-hunt/pre-processor__app/report.json`) |

Exact post-fix `flaky-hunt` summaries:
```text
FLAKY-HUNT [PASS] module=pre-processor/app lang=go rounds=15 passed=15 failed=0 failures=0 duration_ms=267309
evidence_dir: /tmp/fh-postfix-targeted

FLAKY-HUNT [PASS] module=pre-processor/app lang=go rounds=10 passed=10 failed=0 failures=0 duration_ms=547088
evidence_dir: .harness/evidence/flaky-hunt/pre-processor__app
```

---

## 5. Audit of Unregistered Test Files in `rask-log-forwarder/app/tests/` (ISSUE-09)

A full scan of `rask-log-forwarder/app/tests/*/mod.rs` against all `*.rs` files confirmed exactly **two unregistered test files** remain:

1. `rask-log-forwarder/app/tests/parser/test_input_validation.rs` (146 lines, 11 `#[test]` functions)
2. `rask-log-forwarder/app/tests/sender/test_serialization_overflow.rs` (107 lines, 5 `#[test]` functions)

### 5.1 Provenance

Both files were introduced together in commit `6d354db0db5f245279aac408ef4787ac1261799f` (2025-07-09, `"fix: improve rask-log-forwarder security and robustness"`) alongside `tests/buffer/test_memory_safety.rs` and `tests/buffer/test_safe_send_sync.rs`. Neither file was ever registered in `tests/parser/mod.rs` or `tests/sender/mod.rs`, so neither has ever been compiled by `cargo test` or `cargo clippy`. The two `tests/buffer/` sibling files from the same commit were subsequently deleted.

### 5.2 Why Registering Them in `mod.rs` Fails Compilation & Contradicts Current Contracts

| Unregistered File | Broken Imports / Calls in File | Current Production API (Verified) |
|---|---|---|
| `tests/parser/test_input_validation.rs` | `use crate::parser::...` (invalid in Cargo integration tests; must be `rask_log_forwarder::parser::...`) | External crate path `rask_log_forwarder::...` |
| `tests/parser/test_input_validation.rs` | `UniversalParser::parse_log_line(&bytes)` (lines 15, 28, 40, 118, 131, 144) | Method does not exist (`src/parser/universal.rs` exposes `parse_docker_log(&[u8], &ContainerInfo)`, `parse_docker_log_with_stream`, `parse_log_entry`, `parse_batch`) |
| `tests/parser/test_input_validation.rs` | `GoStructuredParser::parse(&bytes)`, `NginxParser::parse(&bytes)`, `PostgresParser::parse(&bytes)` asserting `result.is_err()` (lines 53–80) | Method `parse(&Bytes)` does not exist; `ServiceParser::parse_log(&self, log: &str)` in `src/parser/services/{go,nginx,postgres}.rs` **intentionally falls back** to `Ok(ParsedLogEntry { log_type: "plain" / "unknown", ... })` instead of returning `Err` |
| `tests/parser/test_input_validation.rs` | `SimdParser::parse_docker_json(&bytes)` (lines 92, 105) | Method does not exist (`src/parser/simd.rs` exposes `parse_docker_log(&Bytes)` and `parse_nginx_log(&Bytes)`) |
| `tests/sender/test_serialization_overflow.rs` | `use crate::buffer::Batch; use crate::sender::serialization::...; use crate::parser::EnrichedLogEntry;` | Must be `rask_log_forwarder::...` |
| `tests/sender/test_serialization_overflow.rs` | `Batch::new("test".to_string(), 1000)` + `batch.push(...)` (lines 10–13, 25–30, 57–64, 74, 88–94) | `Batch::new(entries: Vec<EnrichedLogEntry>, batch_type: BatchType)` in `src/buffer/batch.rs:49`; `Batch` stores immutable `Arc<[EnrichedLogEntry]>` and has no `push` method |
| `tests/sender/test_serialization_overflow.rs` | `EnrichedLogEntry::default()` + `entry.set_message(...)` (lines 12, 27–28, 61–62, 91–92) | `src/domain/log_entry.rs` derives `Debug, Clone, Serialize, Deserialize` only — neither `Default` nor `set_message` exists |

### 5.3 Decision & Recommendation

- **Decision**: Left `rask-log-forwarder/app/tests/` untouched (not registered in `mod.rs`) to avoid altering production contracts or shipping broken Rust tests.
- **Active Coverage**: Overflow-safe estimation (`saturating_mul`, `saturating_add`, `MAX_SAFE_BATCH_SIZE`, `MAX_SAFE_BUFFER_SIZE`), empty-batch error handling (`SerializationError::EmptyBatch`), and large-batch NDJSON serialization in `src/sender/serialization.rs` are already exercised by registered tests in `tests/sender/test_serialization.rs` (`test_empty_batch_error`, `test_large_batch_serialization`, `test_serialization_estimate`).
- **Follow-up**: In a dedicated `rask-log-forwarder` cleanup PR, delete the two dead orphan files from commit `6d354db0` and, if additional fuzz/malformed-input coverage is desired, add new tests written against the current `UniversalParser::parse_docker_log` / `SimdParser::parse_docker_log` / `Batch::new(Vec<EnrichedLogEntry>, BatchType)` APIs.
