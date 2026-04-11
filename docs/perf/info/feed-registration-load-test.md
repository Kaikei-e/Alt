# Feed Registration Load Testing: Scaling from 10 to 3,000 Virtual Users

## Overview

The Alt platform -- an AI-augmented RSS knowledge platform -- processes feed registrations through a multi-layered stack: a load testing client (k6) sends requests through nginx (reverse proxy), auth-hub (authentication), Kratos (identity management), alt-backend (business logic), and finally to PostgreSQL, Redis, and external RSS feeds. Each feed registration triggers RSS fetching, article parsing, database writes, and event publishing.

Over the course of several days in March 2026, we ran a progressive load testing campaign against the feed registration endpoint, scaling from 10 to 3,000 virtual users. What began as a smoke test to validate basic functionality became a systematic journey through eleven phases of bottleneck discovery and elimination. Every fix revealed a new constraint, and every assumption we made about the next bottleneck turned out to be wrong at least once. The final result: a 90% reduction in median latency, zero errors at 3,000 VU, and a 484% increase in registered feeds -- but the path there was anything but linear.

## Test Environment

All services run as Docker Compose containers on a single host (24 CPU cores, 62 GB RAM). The load test tool (k6) runs inside the same Docker network. A mock RSS server stands in for external RSS feeds, returning 10 articles per feed. The full request path is:

    k6 --> nginx --> auth-hub --> Kratos --> alt-backend --> mock-rss-server / PostgreSQL / Redis

Key infrastructure components include PgBouncer for connection pooling (introduced mid-campaign), Meilisearch for search indexing, and Redis Streams for event publishing. All tests include automated teardown: Kratos identities, database records, and test artifacts are cleaned up after each run.

## Phase 1: Smoke Test (10 VU) -- Discovery of Initial Barriers

**Configuration:** 10 VU, 100 iterations each, 1,000 total feed registrations.

The first run exposed two problems immediately. The auth-hub validation endpoint had a hard-coded rate limit of 100 requests per minute with a burst of 10. Since all k6 virtual users shared a single Docker IP address, they were collectively throttled as one client. Worse, nginx's `auth_request` directive converted the auth-hub's 429 (Too Many Requests) response into a 500 (Internal Server Error), making it impossible for the client to distinguish rate limiting from actual server failures.

**Results:** 50.9% success rate. Latency was bimodal -- median 2.2 ms for fast paths, but p90 hit 9.8 seconds as requests queued behind I/O contention. Redis Streams hit its memory ceiling (111 OOM errors), though event publishing was non-fatal so registrations succeeded anyway.

**Fixes applied:**
- Made the auth validation rate limit configurable via environment variable (set to 100 req/s for testing).
- Mapped Kratos 429 responses to a proper rate-limited error type instead of a 502 gateway error.
- Applied approximate trimming (`XADD MAXLEN ~10000`) to Redis Streams and doubled Redis memory from 512 MB to 1 GB.

## Phase 2: Scaling to 100 VU -- Fixing Rate Limiting and IP Detection

**Configuration:** 100 VU, 10 feeds each, 60-second duration.

At 100 VU, DoS protection blocked 90% of requests. The root cause: nginx forwarded a single Docker-internal IP for all virtual users, so per-IP rate limiting treated 100 independent users as one. The test also used unrealistic burst patterns -- each VU fired all feed registrations as fast as possible.

**Fixes applied:**
- Configured nginx to trust Docker internal network ranges and extract the real client IP from the `X-Forwarded-For` header.
- Each k6 VU now sends a unique simulated IP address in the forwarded header.
- Replaced burst-style pacing with duration-based intervals, spreading requests evenly.

**Results:** Success rate jumped from 10.2% to 100%. Request rate dropped from 156 req/s (burst) to 17.1 req/s (distributed) -- a more realistic traffic pattern. Zero 429 errors, zero 5xx errors.

## Phase 3: 1,000 VU Baseline -- First Clean Run

**Configuration:** 1,000 VU, 10 feeds each, 120-second duration. Automatic resource scaling kicked in: k6 memory raised to 4 GB, nginx worker connections to 4,096.

**Results:** 100% success rate, 117 req/s throughput, zero errors of any kind. However, DB connection pool contention was visible: median latency was 6 ms, but p95 reached 2.45 seconds and max hit 47 seconds. The 100-connection pool was saturated by 1,000 concurrent users.

| Metric | 100 VU | 1,000 VU |
|--------|--------|----------|
| Success rate | 100% | 100% |
| HTTP req/s | 17.1 | 117.1 |
| http_req_duration p95 | 6.7s | 2.45s |
| Errors | 0 | 0 |

## Phase 4: 3,000 VU -- The Bottleneck Cascade

**Configuration:** 3,000 VU, 50 feeds each, 120-second duration. PostgreSQL max connections raised to 500, backend DB pool to 300, k6 given 8 CPUs and 16 GB memory, nginx worker connections to 8,192.

**Results:** Success rate collapsed to 37%. The system generated 642,722 HTTP requests -- but 336,722 of those were retries. The effective throughput (successful requests only) was roughly 233 req/s, sub-linear against the 3x increase in VU count.

Three bottlenecks were identified:
1. **Kratos session validation saturation**: 3,000 concurrent session checks overwhelmed the single Kratos instance.
2. **DB connection pool exhaustion**: 300 connections were fully consumed, causing 20-40 second queue waits.
3. **Retry amplification**: k6's retry logic doubled the total request volume, creating a feedback loop.

## Phase 5: Client-Side Optimization -- Taming Retry Storms

**Hypothesis:** Reducing unnecessary retries would lower server load and improve success rates.

**Changes:** Cached CSRF tokens per VU (eliminating 150,000 redundant token requests), made 429 responses abort immediately instead of retrying, reduced max retries from 3 to 2 with longer backoff intervals (2s/4s instead of 0.5s/1s/2s).

**Results:** Total requests dropped 75% (642,722 to 161,211). Retry-generated requests fell 97.7%. Data transfer dropped 70%. Median latency improved 81% (34 ms to 6.5 ms). But success rate barely moved: 37% to 35%. The server was still the constraint, not the client.

| Metric | Before | After |
|--------|--------|-------|
| Total HTTP requests | 642,722 | 161,211 |
| Retry-added requests | 492,722 | 11,211 |
| Success rate | 37.0% | 35.4% |
| Data transfer (sent) | 452 MB | 119 MB |

## Phase 6: Infrastructure Scaling -- When Throwing Resources Doesn't Help

**Hypothesis:** Expanding DB connection pools, auth-hub HTTP client pools, and raising rate limits would improve success rates.

**Changes:** Backend DB pool 300 to 500, PostgreSQL max connections 500 to 600, auth-hub idle connections 100 to 200 (per-host: 20 to 50), validation rate limit 1,500 to 3,000 req/s.

**Results:** Virtually no change. Success rate went from 35.4% to 34.7%. Rate limit errors: 98,619 to 98,191. Every metric was within noise. This was the most important lesson of the campaign: *the bottleneck was not where we thought it was.*

All three initial hypotheses were disproven:
- DB pool expansion had no effect (the pool was never the real constraint).
- Auth-hub connection pool expansion had no effect.
- Rate limit increases had no effect (requests were rejected before reaching the rate limiter).

The true bottleneck was Kratos itself -- a single instance processing session validations for 3,000 concurrent users.

## Phase 7: Horizontal Scaling -- Auth Layer Stabilization

**Changes:** Kratos scaled to 3 replicas, auth-hub to 2 replicas (later 3), session cache TTL extended to 30 minutes, test executor changed from per-VU iterations to ramping-VUs (0 to 3,000 over 60 seconds).

**Results:** Auth-hub latency dropped to sub-millisecond for 99.8% of requests. Kratos errors: zero. The auth layer was no longer the bottleneck. However, a new constraint emerged: the remaining validation rate limit (set to 3,000 req/s globally) still rejected 20,003 requests (47.2% failure).

After raising the rate limit to 10,000 req/s and adding a third auth-hub replica, success rate reached 81.3% -- crossing the 80% target for the first time. All errors concentrated in an 8-second burst during the ramp-up phase; steady-state operation was nearly error-free.

## Phase 8: Per-IP Rate Limiting Fix

**Discovery:** The nginx `auth_request` location was not forwarding client IP headers to auth-hub. All requests appeared to come from nginx's own container IP, making per-IP rate limiting useless -- it was effectively a global rate limit.

**Fix:** Added `X-Real-IP` and `X-Forwarded-For` headers to the auth validation proxy location. Combined with relaxing the backend's DoS protection settings for the load test (1,000 req/min per IP, 10-second block duration), this enabled proper per-IP rate limiting. The auth validation rate limit could be reduced to 10 req/s per IP since each VU now had its own identity.

**Results:** Error rate dropped from 47.1% to 1.71%. The remaining 158 errors were not rate limiting at all -- they were `cannot assign requested address` errors from ephemeral port exhaustion against the mock RSS server.

## Phase 9: Eliminating Ephemeral Port Exhaustion

**Problem:** The mock RSS server ran as a single container with a single IP. With 3,000 VUs making concurrent TCP connections to the same destination, Linux ran out of ephemeral ports.

**Fix:** Scaled mock-rss-server to 5 replicas, distributing connections across 5 IPs and reducing per-destination concurrency by 5x.

**Results:** Error rate reached 0.00%. All 14,253 feed registrations succeeded. p95 latency improved slightly (29,644 ms to 29,283 ms), and max latency dropped below the 30-second timeout for the first time. However, median registration latency was still 28.2 seconds -- nearly all processing time was spent in synchronous RSS fetching.

## Phase 10: Circuit Breaker Discovery

An optimization that eliminated duplicate RSS fetching (the registration flow had been fetching the same feed twice) produced a dramatic latency improvement: median dropped from 28,217 ms to 570 ms, a 98% reduction. But this 5x speedup in per-request processing increased effective throughput 5x, which immediately surfaced a new bottleneck.

The circuit breaker protecting the RSS fetch path had a maximum concurrent request limit of 10. With 3,000 VUs now completing requests in under a second instead of 28 seconds, the circuit breaker tripped constantly. After 5 failures, it opened for 60 seconds, rejecting everything. Error rate spiked to 65%.

Initial hypothesis was DoS protection (raised from 1,000 to 10,000 with no effect). Log analysis revealed the real culprit: "too many concurrent requests" from the circuit breaker. Making the circuit breaker's max concurrent requests and fetch semaphore configurable via environment variables (raised to 500 for testing) brought errors to 0%.

| Metric | Before optimization | After optimization |
|--------|--------------------|--------------------|
| feed_register_duration median | 28,217 ms | 570 ms |
| feed_register_duration p95 | 29,283 ms | 3,857 ms |
| Iterations/s | 94.9 | 498.0 |
| Registered feeds | 14,253 | 15,843 |

## Phase 11: Final Optimization -- Async Events and Connection Pooling

Five optimizations were applied in the final phase:

1. **Async event publishing:** Moved event publishing (ArticleCreated) and auto-subscription to background goroutines, removing them from the HTTP response critical path.
2. **Singleflight deduplication:** Used Go's `singleflight` package to coalesce concurrent fetches for the same RSS URL into a single network request.
3. **RSS fetch connection pooling:** Replaced per-request HTTP client creation with a shared client using persistent connections (200 max idle, 50 per host).
4. **Message queue connection pooling:** Replaced the default HTTP client for event publishing with a dedicated pooled client.
5. **Database pool tuning:** Increased PgBouncer pool size to 80, max DB connections to 200, and backend pool to 200.

**Results:**

| Metric | Before (Phase 10) | After (Phase 11) | Improvement |
|--------|-------------------|-------------------|-------------|
| Median latency | 17,270 ms | 1,630 ms | -90.6% |
| p95 latency | 37,620 ms | 2,986 ms | -92.1% |
| Error rate | 0% | 0% | Maintained |
| Throughput | ~210 req/s | 877 req/s | +317% |
| Registered feeds | 16,910 | 98,726 | +484% |

The async event publishing was the highest-impact change. By removing synchronous waits on message queue round-trips and database writes from the response path, the critical path for feed registration shortened to: validate, fetch RSS, parse, write feed records, return.

## Results Summary

| Phase | VU | Success Rate | p95 Latency | Throughput | Key Change |
|-------|-----|-------------|-------------|------------|------------|
| 1: Smoke test | 10 | 50.9% | 9,967 ms | 2.0 req/s | Baseline |
| 2: IP fix | 100 | 100% | 6,660 ms | 17.1 req/s | Per-VU IP simulation |
| 3: 1K baseline | 1,000 | 100% | 2,450 ms | 117 req/s | Resource auto-scaling |
| 4: 3K initial | 3,000 | 37.0% | 28,460 ms | 727 req/s* | Kratos saturated |
| 5: Retry taming | 3,000 | 35.4% | 28,390 ms | 202 req/s | Client-side optimization |
| 6: Pool expansion | 3,000 | 34.7% | 28,370 ms | 194 req/s | No effect (wrong hypothesis) |
| 7: Horizontal scale | 3,000 | 81.3% | 28,450 ms | 131 req/s | Kratos/auth-hub replicas |
| 8: Per-IP rate limit | 3,000 | 98.3% | 29,644 ms | 155 req/s | nginx IP header forwarding |
| 9: Port exhaustion | 3,000 | 100% | 29,283 ms | 155 req/s | Mock server replicas |
| 10: Circuit breaker | 3,000 | 100%** | 3,857 ms | 569 req/s | CB + semaphore tuning |
| 11: Final async | 3,000 | 100% | 2,986 ms | 877 req/s | Async events + pooling |

*Includes retry storm inflation. **After three sub-runs to isolate the circuit breaker.

## Key Takeaways

**1. Measure before you scale.** Phase 6 proved that tripling connection pools and doubling rate limits had zero effect because the actual bottleneck was elsewhere. Without telemetry showing where requests stalled, resource expansion is guesswork.

**2. Retry storms are a force multiplier for failure.** At 3,000 VU, automatic retries doubled total request volume from 300,000 to 642,722. The retries themselves became the dominant source of load. Implementing 429-aware abort logic and longer backoff intervals cut unnecessary requests by 97.7%.

**3. Fix one bottleneck, find the next.** Every optimization exposed a new constraint. Fixing auth latency revealed DB pool limits. Fixing pool limits revealed Kratos capacity. Fixing Kratos revealed per-IP rate limiting bugs. Eliminating duplicate fetches exposed circuit breaker limits. This is normal -- performance work is a series of constraint removals, not a single fix.

**4. Client IP handling matters more than you think.** Two separate phases (2 and 8) were caused by the same class of bug: all requests appearing to come from a single IP. In a reverse-proxy architecture, ensure every hop correctly forwards client identity. Without it, per-IP rate limiting becomes a global throttle.

**5. Latency improvements can cause throughput-driven failures.** When an optimization reduced per-request latency from 28 seconds to 570 milliseconds, the effective request rate increased 5x. This immediately overwhelmed circuit breakers tuned for the slower rate. Fast responses with concurrency limits require those limits to be re-evaluated.

**6. Async processing is the highest-leverage optimization.** Moving event publishing and subscription off the critical path produced a 90% latency reduction and a 484% increase in registered feeds. Any work that does not need to complete before the HTTP response should be deferred.

**7. Connection pooling compounds.** Pooling HTTP connections to the RSS server, the message queue, and the database individually produced modest gains, but together they eliminated thousands of TCP handshakes per second and freed ephemeral ports across the system.

**8. Test infrastructure is part of the system under test.** Ephemeral port exhaustion against the mock server, k6 memory limits, and teardown batch sizes all required fixes. Load test infrastructure must scale alongside the system it tests, or it becomes the bottleneck.
