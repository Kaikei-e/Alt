# Alt Platform Performance Engineering Case Studies

A series of performance engineering case studies from the Alt platform, an AI-augmented RSS knowledge platform built with 28+ microservices orchestrated via Docker Compose.

These articles document real load testing campaigns, optimization investigations, and the methodology behind them.

## Articles

### 1. [Mobile LCP Optimization](mobile-lcp-optimization.md)

Reducing mobile feed page Largest Contentful Paint from 3.7 seconds to under 2.5 seconds through SSR-side field generation, client logic simplification, CSS optimization, and hero component LCP stabilization.

### 2. [Feed Registration Load Testing](feed-registration-load-test.md)

Progressive load testing from 10 to 3,000 virtual users. A bottleneck discovery chain spanning rate limiting, Redis OOM, auth session validation, database connection pools, circuit breakers, and async optimization — culminating in a 90.6% latency reduction.

### 3. [Feed Read Performance & Go Memory Optimization](feed-read-go-memory-optimization.md)

Hunting a memory allocation bug that produced 3.8 TB of cumulative allocations in 30 minutes. Covers realistic 80/20 access pattern testing, Go pprof profiling methodology, the counter-intuitive danger of GOMEMLIMIT, and a surgical fix delivering 99.3% latency improvement.

### 4. [Composite Frontend Load Testing](composite-frontend-load-test.md)

Multi-flow user simulation with SvelteKit and Connect-RPC across 4 weighted user journeys (Browse, Read, Discovery, Manage). From initial bug discovery through extreme load testing to per-host rate limiter optimization.

### 5. [Infrastructure Observability Baseline](infrastructure-observability-baseline.md)

Baseline health audit of a 28-service microservice platform combining Prometheus metrics, HTTP access logs, OpenTelemetry structured logs, and container runtime statistics to identify error hotspots and resource bottlenecks.

## Tech Stack

- **Load Testing**: k6
- **Profiling**: Go pprof, Chrome Lighthouse
- **Database Analysis**: PostgreSQL EXPLAIN ANALYZE
- **Monitoring**: Prometheus, ClickHouse, OpenTelemetry
- **Infrastructure**: Docker Compose, nginx, PostgreSQL, Redis, PgBouncer
- **Application**: Go, SvelteKit, Connect-RPC

## License

These case studies are published for educational purposes. The methodologies, findings, and lessons learned are applicable to any microservice architecture undergoing performance optimization.
