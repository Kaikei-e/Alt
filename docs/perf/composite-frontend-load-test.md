# Composite Frontend Load Testing: Multi-Flow User Simulation with SvelteKit and Connect-RPC

## Overview

Single-endpoint benchmarks tell you how fast one RPC can respond. They do not tell you what happens when hundreds of users are simultaneously browsing feeds, reading articles, discovering new content, and managing subscriptions -- all through the same frontend proxy layer. Composite load testing bridges that gap by simulating realistic, weighted user journeys that exercise the full request path from the load generator through a reverse proxy, a SvelteKit frontend, an authentication layer, and a Go backend, all the way to external HTTP services and the database.

This article describes a series of composite load tests conducted against Alt, an AI-augmented RSS knowledge platform. The tests targeted the SvelteKit frontend path using Connect-RPC (an open protocol that layers on HTTP with Protobuf or JSON serialization). Over six test runs spanning two days, the testing progressed from initial bug discovery, through scaling and profiling, to rate limiter optimization -- uncovering bugs, architectural bottlenecks, and configuration gaps that no single-endpoint test would have found.

### The Four User Flows

Each virtual user (VU) session randomly selects one of four flows based on a weighted distribution designed to mirror realistic usage:

| Flow | Weight | RPC Sequence | Rationale |
|------|--------|-------------|-----------|
| Browse | 45% | GetUnreadCount, GetUnreadFeeds, GetAllFeeds | The most common action: checking what is new |
| Read | 25% | GetUnreadFeeds, FetchArticlesCursor, FetchArticleContent, MarkAsRead | Reading an article triggers an external HTTP fetch for content |
| Discovery | 20% | GetFeedStats, GetDetailedFeedStats, SearchFeeds, GetReadFeeds, GetFavoriteFeeds | Exploring analytics and searching via Meilisearch |
| Manage | 10% | ListRSSFeedLinks, RegisterRSSFeed, RegisterFavoriteFeed, ListRSSFeedLinks | Adding and favoriting feeds -- a less frequent but write-heavy action |

The weighting matters. Browse and Read account for 70% of sessions, reflecting how most users interact with an RSS reader. Discovery exercises the search index. Manage exercises write paths and external feed registration, which involves fetching and parsing remote RSS XML. The composite distribution ensures that bottlenecks in any one path are exposed proportionally to their real-world impact.

### Request Path

Every request follows the same chain:

```
k6 VU -> nginx -> alt-frontend-sv (SvelteKit proxy)
  -> Session verification (Kratos)
  -> Backend token acquisition (auth-hub)
  -> alt-backend (Connect-RPC handlers)
    -> PostgreSQL (via PgBouncer)
    -> Meilisearch (search queries)
    -> mock-rss-server (simulating external RSS feeds)
```

## Test Environment

All services ran on a single machine under Docker Compose orchestration. The mock-rss-server simulated external RSS feeds with configurable response delays (150ms for feed XML, 350ms for article content, with 75ms of jitter). k6 drove the load using a `ramping-arrival-rate` model, where sessions arrive at a configured rate independent of how long each session takes to complete. This is critical for realistic load testing: it means the system experiences increasing pressure even when individual requests slow down.

Rate limiters protecting the backend (DoS protection, per-host external API throttling, and authentication rate limits) were overridden to higher values for testing. This is deliberate -- the goal was to find infrastructure and application bottlenecks, not to test the rate limiters themselves. Rate limiter tuning came later in Phase 6.

## Phase 1: Initial Run -- Bug Discovery

The first run used modest parameters: 200 test users, a peak of 8 sessions per second, and a maximum of 128 VUs. Browse and Discovery flows passed at 100%. Read flow succeeded only 4.18% of the time, and Manage flow failed entirely.

The composite test immediately surfaced two bugs that had been invisible in isolation:

1. **UUID array encoding failure**: The `FetchArticlesCursor` RPC failed with a PgBouncer compatibility error. When using PgBouncer's transaction pooling mode, prepared statement metadata (OIDs) is not shared across connections. The pgx driver could not encode `[]uuid.UUID` arrays because the type OID was not cached. This bug had been latent because earlier tests never populated the article cache -- a previous failure in the request chain meant the UUID query was never reached.

2. **Feed URL mismatch**: The `RegisterFavoriteFeed` RPC expected an article URL, but the test was sending the RSS feed URL. The registered feed's stored link did not match what the test script sent, causing a "feed not found" error on every attempt.

Both bugs were consequences of multi-step flows. In a single-endpoint test, each RPC would have been called in isolation with synthetic data. Only by chaining RPCs in a realistic sequence -- register a feed, then favorite it; fetch articles, then paginate them -- did the data flow issues surface.

## Phase 2: Bug Fixes (UUID Encoding, URL Routing)

The UUID encoding fix converted `[]uuid.UUID` to `[]string` with an explicit SQL cast to `::uuid[]`, making the query compatible with PgBouncer's simple protocol mode. The test script fix corrected the URL passed to `RegisterFavoriteFeed` to match what the frontend actually sends.

After these fixes, a same-day rerun achieved 100% success across all four flows with zero errors over 12,805 requests in 10 minutes. HTTP error rate dropped from 8.17% to 0.00%, and p95 request latency fell from 165ms to 61ms. The k6 exit code went from 99 (threshold failure) to 0.

## Phase 3: Scaling to 1,000 VU with Profiling

With functional correctness established, the next step was to push throughput. The session arrival rate was increased to a peak of 15 sessions per second, VU capacity was raised to 1,000, and runtime profiling was enabled on the backend.

All four flows maintained 100% success. Total throughput increased 82% to 23,317 requests (38 req/s). However, `FetchArticleContent` -- the one RPC that issues external HTTP requests -- showed a long-tail latency distribution: p50 was 7ms (cache hit), but p95 climbed to 4,060ms, narrowly exceeding its 4,000ms threshold by 60ms.

The profiling data told a clear story. Heap memory was 52.78 MB (2.6% of the 2 GiB limit). Only 48 goroutines were active after the test, with no signs of leaks. The allocation rate of 3.81 MB/s was modest. gzip compression buffers for Connect-RPC responses accounted for 87% of in-use memory. The backend was not the bottleneck -- the mock-rss-server was simply overwhelmed by concurrent connections during peak load.

## Phase 4: Extreme Load -- Finding the System Ceiling

The parameters were pushed aggressively: 2,000 test users, peak of 120 sessions per second, 512 pre-allocated VUs with a cap of 1,000. The goal was to find where the system breaks.

It broke comprehensively. All 1,000 VUs were consumed within 3 minutes. Average iteration duration ballooned from 1.48s to 19.36s. Over 27,000 iterations (52% of arrivals) were dropped because no VU was available. The system produced 5,709 server errors and 5,404 rate limit hits.

The failure was not uniform across flows. Browse and Manage maintained 100% success despite p95 latencies of 6-7 seconds. Discovery flow collapsed entirely (0% success) because the search index could not handle the concurrent query volume at this scale. Read flow dropped to 6.43% as external HTTP timeouts cascaded.

The key insight was that median latency for all RPCs clustered around 2.8-3.0 seconds, regardless of the RPC's actual complexity. This pointed to queuing upstream of the backend rather than slow query execution -- a structural bottleneck in the proxy layer.

## Phase 5: nginx Connection Tuning

The hypothesis was that nginx connection limits were causing the uniform queuing. The `worker_connections` setting was increased from 1,024 to 8,192, the file descriptor limit was raised to 32,768, access logging was disabled for I/O reduction, and the DoS protection rate limit was doubled.

The results were mixed but informative. Discovery flow recovered from 0% to 100% success -- this was entirely due to the DoS protection increase, not nginx tuning. Server errors dropped 91% from 5,709 to 517. But RPC median latencies remained stubbornly at 2.9-3.2 seconds.

This disproved the nginx hypothesis. The queuing was happening in the SvelteKit proxy layer or the backend connection handling, not in nginx. The nginx tuning was not wasted -- it prevented connection refusal errors and eliminated file descriptor exhaustion risk -- but it was not the path to lower latencies at extreme scale.

Profiling confirmed the real bottleneck: goroutine dumps showed 39 out of 107 goroutines (36%) waiting on external HTTP responses, with 3 more blocked on the per-host rate limiter. With all VUs targeting a single mock host, the per-host rate limit (burst of 100) was the chokepoint.

## Phase 6: Rate Limiter Optimization

The final phase conducted three iterative runs to tune the per-host rate limiter, a token bucket implementation.

**Run 1 (baseline)**: Interval of 1 second, burst of 100. After the burst tokens were consumed, only 1 request per second could pass to the external host. This produced 329 rate limit wait failures.

**Run 2 (burst increase)**: Interval of 1 second, burst of 500. The larger burst reservoir delayed exhaustion but did not change the refill rate. Rate limit hits actually increased to 4,851 because more requests queued up waiting for the slow refill. Read flow success improved from 8.35% to 21.49%.

**Run 3 (interval reduction)**: Interval of 10ms, burst of 500. This was the breakthrough. By changing the refill interval from 1 second to 10 milliseconds, the sustained throughput increased from 1 request per second to 100 requests per second per host. Rate limit hits dropped to zero. Read flow success jumped to 71.85%. Total throughput increased 34% to 149,106 requests (237 req/s).

The progression from 329 rate limit hits to 4,851 and then to zero illustrates why iterative testing matters. Simply increasing the burst made things worse by allowing more requests to pile up behind the slow refill. Only by addressing the sustained rate (the interval) was the bottleneck truly resolved.

The remaining 28% failure rate in Read flow came from HTTP client timeouts (30s deadline) when concurrent connections to the mock servers exceeded their capacity -- a fundamentally different bottleneck from rate limiting.

## Results Summary

| Phase | Peak Rate | Success Rate | Errors | Key Finding |
|-------|-----------|-------------|--------|-------------|
| 1 - Initial | 8 sess/s | 65.28% | 1,041 | Two latent bugs exposed by multi-step flows |
| 2 - Bug Fix | 8 sess/s | 100.00% | 0 | All flows pass after UUID + URL fixes |
| 3 - Scale Up | 15 sess/s | 100.00% | 0 | Stable at 38 req/s; profiling shows healthy resource usage |
| 4 - Extreme | 120 sess/s | 56.09% | 5,709 | System ceiling found; uniform queuing at ~3s median |
| 5 - nginx Tune | 120 sess/s | 77.25% | 517 | nginx not the bottleneck; DoS limit increase restores Discovery |
| 6 - Rate Tune | 120 sess/s | 93.06% | 2,512 | Rate limit hits reduced from 329 to 0; throughput +34% |

## Key Takeaways

**Design composite tests around weighted user journeys, not individual endpoints.** The two bugs found in Phase 1 were invisible to single-RPC tests. They only appeared when RPCs were chained in realistic sequences where one step's output becomes the next step's input. Weighting the flows to match real usage patterns ensures that the most impactful failures are found first.

**Use arrival-rate models, not constant-VU models.** The `ramping-arrival-rate` executor in k6 generates sessions at a fixed rate regardless of system performance. This creates realistic pressure: as the system slows down, more VUs are consumed, revealing queuing behavior and resource exhaustion that constant-VU tests would mask.

**Profile before you tune.** Goroutine dumps and heap profiles eliminated guesswork at every phase. When median latency clustered at 3 seconds across all RPCs, profiling proved the bottleneck was in the proxy layer, not the backend. When rate limit hits persisted after increasing burst size, the goroutine dump showed requests blocked on `WaitForHost`, pointing directly to the refill interval.

**Tune rate limiters iteratively and understand their mechanics.** A token bucket has two parameters: burst (reservoir size) and interval (refill rate). Increasing burst alone delays exhaustion but worsens queuing once tokens run out. Only reducing the interval -- changing the sustained throughput -- resolves steady-state rate limiting. The progression from 329 hits to 4,851 to zero across three runs demonstrated this clearly.

**Finding the system ceiling requires deliberate overload.** Phase 4 was designed to fail. Running at 8x the known-stable rate revealed the uniform queuing pattern, the VU exhaustion behavior, and the specific flows that degrade first under pressure. This information -- which services break, which survive, and at what load level -- is more valuable than a passing test at conservative parameters.
