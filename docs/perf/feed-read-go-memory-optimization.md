# Feed Read Performance: Hunting a 3.8 TB Memory Allocation Bug in Go

## Overview

What happens when your API responds in 12 milliseconds under load testing, but collapses to 4.7 seconds under realistic conditions? And what do you do when the obvious fix -- telling Go's garbage collector to be more aggressive -- makes things 5.1 times worse?

This is the story of a feed read performance investigation on the Alt platform, an AI-augmented RSS knowledge system built on Go microservices, PostgreSQL, and PgBouncer. Over the course of nine load test iterations in two days, we went from a confident "all systems nominal" to discovering that a single HTML sanitization function was responsible for 58% of all memory allocations -- 2,228 GB out of 3,836 GB cumulative over a 30-minute test. The journey took us through EXPLAIN ANALYZE, Go's pprof profiler, counter-intuitive GOMEMLIMIT experiments, and ultimately a surgical code fix that restored p95 latency from 4.7 seconds to 31 milliseconds.

## Test Environment

All tests ran on a single machine using Docker Compose orchestration. The stack under test consisted of:

- **alt-backend**: Go service handling Connect-RPC v2 feed read endpoints (port 9101)
- **PostgreSQL**: Primary data store with PgBouncer connection pooling
- **k6**: Load generator running 3,000 virtual users across four behavioral scenarios

The load test simulated realistic user behavior: 10% of users only glanced at their feed list, 55% browsed into article lists, 20% paginated through multiple pages, and 15% performed deep reads into individual articles. All endpoints were pure database reads -- no external HTTP calls.

Resource allocations for testing: alt-backend received up to 10 GB memory and 8 CPU cores. PostgreSQL was configured with 250 max connections, 512 MB shared buffers, and 16 MB work memory. PgBouncer used a pool size of 80 with 2,000 max client connections.

## Phase 1: Uniform Distribution Baseline (3,000 VU)

The first test distributed data uniformly: all 3,000 test users subscribed to all 76 feeds, with 60,000 articles spread evenly. The results were excellent.

| Metric | Value |
|--------|-------|
| p95 latency | 12.93 ms |
| Error rate | 0.00% |
| Throughput | 1,022 req/s |
| Total requests | 1,845,892 |

Every threshold passed. Zero errors across nearly two million requests. The system appeared rock solid.

## Phase 2: Realistic 80/20 Distribution -- Performance Collapse

Real users do not subscribe to feeds uniformly. Popular sources like The Guardian, dev.to, and ZDNet attract far more subscribers than niche feeds. To model this, we classified the top 20% of feeds (16 out of 76) as "popular" and had all 3,000 users subscribe to them, while each user randomly subscribed to 30% of the remaining 60 "normal" feeds.

The results were catastrophic.

| Metric | Uniform | 80/20 | Change |
|--------|---------|-------|--------|
| p95 latency | 12.93 ms | 4.70 s | 363x worse |
| Throughput | 1,022 req/s | 450 req/s | -56% |
| Data received | 15.4 GB | 99 GB | +6.4x |
| Error rate | 0.00% | 0.00% | Same |

Latency degraded by a factor of 363. The system did not break -- every request still returned HTTP 200 -- but it was unusable. The median latency was already 4.08 seconds, meaning the vast majority of requests during steady state were painfully slow. Throughput saturated at roughly 1,000 VU, with additional virtual users simply queuing up. Little's Law told the story plainly: with 3,000 users queued and 200 iterations per second throughput, the expected wait time was about 15 seconds.

The hotspot pattern was clear: 16 popular feeds concentrated database access, and the large content from sources like dev.to (averaging 13 KB per description field) amplified response sizes dramatically.

## Phase 3: Database Query Analysis (EXPLAIN ANALYZE)

We ran EXPLAIN (ANALYZE, BUFFERS) on the three primary read queries. At single-user scale, the results were fast: the unread feeds query completed in 1.4 ms on the first page with 108 buffer hits and zero disk reads.

But cursor pagination told a different story. The second page took 14.5 ms -- a 10x degradation -- with 148 disk reads. The query scanned 172 rows to return just 21, because the read status index used `feed_id` as the index condition but `user_id` as a post-filter. Under 3,000 concurrent users, this produced an estimated 516,000 index lookups per page request.

The articles cursor query had an even more alarming pattern: a LATERAL JOIN against tag data created 333 nested loops per request, accumulating 397 disk block reads. At scale, this meant nearly one million index lookups per page request.

Column size analysis revealed a critical insight: the feeds table was 814 MB total, but shared buffers were only 256 MB. The dev.to feed descriptions averaged 13 KB each and were stored in PostgreSQL's TOAST tables, meaning every access required additional I/O operations beyond the main heap.

## Phase 4: Reproducing the Crash

With the 80/20 distribution in place, we attempted to reproduce the service instability at scale. With a 2 GB memory limit on alt-backend, the service crashed and restarted at approximately 1,800 VU (about 7.5 minutes in). The container's `OOMKilled` flag was false and its exit code was 0 -- it appeared to terminate gracefully rather than being killed by the OS.

The crash was preceded by a clear degradation pattern in the service's internal performance logs: response times escalated from 20-100 ms to over 10 seconds for feed queries, after which the Connect-RPC port stopped responding entirely. The progression was always the same -- slow responses, then EOF errors, then connection reset, then connection refused.

We eliminated several candidate causes: scaling auth-hub to 3 replicas did not help, increasing file descriptor limits had no effect, and tuning circuit breaker parameters only delayed the crash by a few minutes.

## Phase 5: Memory as Amplifier (10 GB Experiment)

To test whether memory was the constraint, we raised alt-backend's Docker memory limit from 2 GB to 10 GB. The service completed the full 30-minute test without crashing.

But it was far from healthy. Memory usage climbed steadily, reaching 1 GiB after 7 minutes, 4 GiB after 8.5 minutes, and peaking at 9.823 GiB after 24 minutes. It stayed above 9 GiB for over 15 minutes. The p95 latency was still 4.59 seconds.

This confirmed that the 2 GB limit was one trigger for the crash, but also revealed a deeper problem: the service was consuming nearly 10 GB of memory to serve database read queries that should have been lightweight. The problem shifted from "the service crashes" to "the service consumes absurd amounts of memory."

## Phase 6: GOMEMLIMIT Tuning -- When the Cure is Worse

Go 1.19 introduced `GOMEMLIMIT`, a soft memory limit that triggers more aggressive garbage collection as heap usage approaches the threshold. The theory was sound: set `GOMEMLIMIT=4GiB` to keep memory under control while the 10 GB Docker limit provides headroom.

The results were devastating.

| Metric | No GOMEMLIMIT | GOMEMLIMIT=4GiB |
|--------|---------------|-----------------|
| p95 latency | 4.59 s | 23.56 s |
| Throughput | 486 req/s | 216 req/s |
| Total requests | 873,806 | 391,088 |

Setting GOMEMLIMIT to 4 GiB made performance **5.1 times worse**. The p95 latency jumped from 4.59 seconds to 23.56 seconds. Throughput was cut in half.

The explanation required understanding Go's garbage collector behavior. The GC's Stop-The-World phases pause all goroutines. With a 4 GiB limit and an allocation rate of 1.34 GB/s (which we would measure shortly), the GC was forced to run a full heap scan roughly every 3 seconds. During each pause, 3,000 incoming connections queued up. When the GC completed, the queued requests flooded in, immediately filled the heap, and triggered the next GC cycle. The service was trapped in a GC thrashing loop.

Internal handler measurements revealed the scale of the disconnect: Go handlers reported 77-78 ms p95 for feed queries, while k6 observed 23,560 ms p95 -- a 300x gap. The handlers were fast when they ran, but they spent most of their time waiting for GC pauses to end.

This was the key counter-intuitive lesson: **GOMEMLIMIT can make performance dramatically worse when the underlying allocation rate is too high.** The correct response is not to tune the GC but to reduce allocations.

## Phase 7: pprof Profiling -- Finding the 58% Allocation Hotspot

We enabled Go's built-in pprof profiler and collected a 30-minute cumulative allocation profile during a load test with `GOMEMLIMIT=10GiB`. This is where the investigation turned decisive.

### The Numbers That Told the Story

Over 30 minutes, the Go process performed **3,836 GB of cumulative memory allocations** -- an average rate of **2.13 GB/s**. To be clear: this was not 3.8 TB of resident memory. Go's garbage collector continuously reclaimed short-lived objects. The heap at the end of the test was just 57 MB. But every one of those allocations required GC work, and the sheer volume created punishing GC pressure.

### The Allocation Hotspots

The pprof cumulative allocation profile identified three dominant hotspots:

**Hotspot #1: `sanitizeDescription` -- 2,228 GB (58.1% of all allocations)**

The feed handler converted database results to protobuf responses, and during that conversion, every feed's HTML description was sanitized using the bluemonday library. This involved HTML tokenization (457 GB), byte buffer growth (437 GB), regex replacements (413 GB), string conversions (248 GB), and HTML unescaping (244 GB).

The critical finding: **the descriptions were already sanitized when they were stored in the database during feed ingestion.** The read path was re-sanitizing content that was already clean. Every request for 20 feed items triggered 20 redundant sanitization passes over potentially large HTML strings.

**Hotspot #2: `convertFeedPageEntries` -- 1,239 GB (32.3%)**

Converting cached feed page entries to domain objects required calling `uuid.UUID.String()` for every UUID field on every feed item. With 20 items per page and multiple UUID fields per item, this produced 233 GB of string allocations alone -- one of the most expensive operations in the entire profile.

**Hotspot #3: Database query parameter encoding -- 550 GB (22.7%)**

PgBouncer requires SimpleProtocol mode (no prepared statements), which means UUID arrays must be serialized as text literals in the query string. The `GetReadFeedIDs` function sent all feed IDs as a text-encoded UUID array to PostgreSQL, consuming 488 GB in array codec encoding alone.

### Why pg_stat_statements Cleared the Database

Meanwhile, `pg_stat_statements` showed that the actual database queries were fast: 50-68 ms average execution time, with excellent buffer cache hit rates. The bottleneck was entirely in the Go application layer -- specifically, in how it processed and transformed query results, not in how it queried the database.

## Phase 8: Surgical Fix -- Eliminating Double Sanitization

The highest-impact fix was also the simplest. The `sanitizeDescription` function was being called in the handler layer during protobuf conversion, but the same content had already been sanitized in the gateway layer when it was loaded from the database. We removed the handler-layer sanitization entirely and consolidated the sanitize logic into a single shared utility function called only at the gateway layer.

Additional optimizations in the same change:
- Contiguous memory allocation for feed item slices (reducing N+1 small allocations to 3 large ones)
- Shared empty slice variables to eliminate per-iteration allocations in loops
- Backing array pre-allocation for string slices

The results of this fix with `GOMEMLIMIT=10GiB`:

| Metric | Before Fix | After Fix | Improvement |
|--------|-----------|-----------|-------------|
| p95 latency | 2,390 ms | 440 ms | 81.6% reduction |
| Throughput | 645 req/s | 974 req/s | 51% increase |
| Allocations per request | 3.29 MB | 1.95 MB | 40.7% reduction |
| K6 exit code | 99 (thresholds failed) | 0 (all passed) | First passing run |

Handler-layer sanitize allocations dropped from 2,228 GB to 0 GB. The gateway layer still performed sanitization (956 GB), but only once per cache load rather than once per request. This was the first 3,000 VU test run that passed all performance thresholds.

## Phase 9: Orphan Data Cleanup -- The Final 99.3% Improvement

The pprof profile from Phase 8 revealed a remaining hotspot: `FetchOrphanFeeds`, responsible for 33% of cumulative allocations. This function retrieved feeds with no associated feed source -- orphan records from historical data migrations. Every feed read query contained an `OR feed_link_id IS NULL` clause to include these orphans, which prevented PostgreSQL from using its optimal Index Scan + Memoize query plan.

The fix was a data cleanup: delete all orphan feeds (those with NULL `feed_link_id`), add a NOT NULL constraint, and remove the `OR` clause from six SQL queries. This also eliminated the `FetchOrphanFeeds` code path entirely, along with the in-memory merge and sort logic it required.

| Metric | Pre-Cleanup | Post-Cleanup | Improvement |
|--------|------------|--------------|-------------|
| p95 latency | 4,590 ms | 31.47 ms | 99.3% |
| Throughput | 484 req/s | 1,022 req/s | 2.11x |
| Heap at test end | ~9,823 MiB | 57.77 MB | 99.4% reduction |
| Total requests | 873,806 | 1,843,997 | 2.11x |

The p95 latency of 31.47 ms closely matched the original uniform distribution baseline of 12.93 ms, confirming that the 80/20 access pattern was no longer a significant factor. All six performance thresholds passed, and the heap stabilized at just 58 MB -- down from 9.8 GiB.

## Results Summary

The complete journey across all test iterations:

| Phase | Configuration | p95 Latency | Throughput |
|-------|--------------|-------------|------------|
| Uniform distribution | Baseline | 12.93 ms | 1,022 req/s |
| 80/20 distribution | No optimization | 4.70 s | 450 req/s |
| 80/20 + 10 GB memory | Memory increase only | 4.59 s | 484 req/s |
| 80/20 + GOMEMLIMIT=4GiB | GC tuning attempt | 23.56 s | 216 req/s |
| 80/20 + GOMEMLIMIT=10GiB | Relaxed GC | 2.39 s | 645 req/s |
| 80/20 + sanitize fix | Allocation reduction | 440 ms | 974 req/s |
| 80/20 + orphan cleanup | Data + query cleanup | 31.47 ms | 1,022 req/s |

From the worst point (GOMEMLIMIT=4GiB, p95=23.56s) to the final result (p95=31ms), that is a 760x improvement. From the realistic 80/20 baseline (p95=4.70s) to the final result, a 149x improvement.

## Key Takeaways

**1. Uniform load tests are dangerously optimistic.** Our uniform distribution test showed p95 of 12.93 ms. The same system under realistic 80/20 access patterns showed p95 of 4.70 seconds -- 363 times worse. If we had only run the uniform test, we would have shipped a system that collapsed under real usage. Always test with access patterns that match production.

**2. pprof cumulative allocation profiles are the single most valuable Go profiling tool.** The heap snapshot at rest showed only 57 MB in use -- everything looked fine. The cumulative allocation profile revealed 3.8 TB of allocations over 30 minutes, pinpointing exactly which functions were responsible. The key command: `go tool pprof -http=:8080 /path/to/allocs.pb.gz`. Look at cumulative (`cum`) values in the call tree, not just flat allocations.

**3. GOMEMLIMIT can make things dramatically worse.** Setting GOMEMLIMIT to 4 GiB with an allocation rate of 1.34 GB/s forced GC cycles every few seconds, each pausing all goroutines. The result was a 5.1x latency regression. GOMEMLIMIT is a tool for preventing OOM kills, not for improving performance when the fundamental allocation rate is too high. Fix the allocations first, then tune the GC.

**4. The database was not the bottleneck.** pg_stat_statements showed query execution times of 50-68 ms. Internal handler timers showed 77 ms p95. But k6 measured 23 seconds p95. The 300x gap was entirely GC-induced queuing. Always measure at multiple layers to avoid blaming the wrong component.

**5. Redundant work at scale becomes the dominant cost.** A single unnecessary `sanitizeDescription` call on a 20-item feed page is negligible. That same call repeated 1.7 million times across 3,000 concurrent users over 30 minutes produces 2,228 GB of allocations. The fix was a one-line deletion, but finding it required systematic profiling to follow the allocation trail from symptoms to root cause.
