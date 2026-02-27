# alt-perf

_Last reviewed: February 28, 2026_

**Location:** `alt-perf/`

## Purpose

Deno 2.x-based E2E performance measurement CLI for the Alt platform. Combines browser-level Core Web Vitals measurement via Astral (Deno-native Puppeteer) with API-level load testing via Grafana K6. Measures LCP, INP, CLS, FCP, and TTFB across desktop and mobile device profiles, and generates reports in CLI, JSON, or Markdown format.

## Architecture

```
main.ts                          # CLI entry point (parseArgs dispatch)
  src/commands/scan.ts           # Route scanning + Web Vitals measurement
  src/commands/flow.ts           # Multi-step user flow tests
  src/commands/load.ts           # Deno-native HTTP load testing
  src/config/loader.ts           # YAML config loader with env var expansion
  src/config/schema.ts           # Type definitions + device profiles + defaults
  src/browser/astral.ts          # Browser manager (launch, page creation, navigation)
  src/auth/kratos-session.ts     # Ory Kratos session cookie acquisition
  src/measurement/vitals.ts      # PerformanceObserver injection + collection
  src/measurement/statistics.ts  # Statistical aggregation utilities
  src/report/cli-reporter.ts     # Terminal-formatted output
  src/report/json-reporter.ts    # JSON file/stdout output
  src/report/markdown-reporter.ts # Markdown report generation
  src/retry/retry-policy.ts      # Exponential backoff with jitter
  src/utils/otel.ts              # OpenTelemetry log export
k6/                              # Grafana K6 load test scripts (separate runtime)
config/                          # YAML configuration (routes, flows, thresholds)
```

## Commands

### `scan` - Route Scanning

Launches a headless Chromium browser via Astral, visits every configured route, injects a PerformanceObserver-based collector script, and measures Core Web Vitals. Supports multi-run measurement with warmup, outlier discarding (IQR-based), and median aggregation for statistical accuracy.

```bash
# Scan all routes with default devices (desktop-chrome + mobile-chrome)
deno task perf:scan

# Scan specific route with mobile device
deno task perf -- scan -d mobile-chrome -r /sv/mobile/feeds

# High-accuracy scan: 2 warmup runs, 5 measurement runs
deno task perf -- scan --warmup 2 --runs 5

# Output Markdown report
deno task perf -- scan --format md -o reports/scan.md
```

**Scan-specific CLI options:**

| Option | Default | Description |
|--------|---------|-------------|
| `-w, --warmup <n>` | 1 | Warmup runs before measurement (warms server/browser caches) |
| `-n, --runs <n>` | 1 | Measurement runs per route (median used for final result) |
| `-d, --device <name>` | all configured | Device profile to use |
| `-r, --route <path>` | all | Filter routes by path substring |
| `-f, --format <type>` | cli | Output format: `cli`, `json`, `md` |
| `-o, --output <path>` | auto | Output file path (auto-saves to `reports/`) |

**Measurement pipeline:**

1. Warmup phase: Visit a subset of authenticated routes to stabilize caches
2. Measurement phase: For each (route, device) pair, create a new browser page with device emulation and auth cookies
3. Inject `PerformanceObserver` script collecting LCP, FCP, CLS, INP, TTFB
4. Wait a stabilization delay (1500ms default), then read collected metrics
5. For multi-run: aggregate using median, discard outliers when runs >= 3
6. Calculate weighted score (0-100) and identify bottlenecks
7. Browser restarts every 20 pages to prevent memory leaks

**Route groups** (from `config/routes.yaml`):

| Group | Auth | Description |
|-------|------|-------------|
| `public` | No | Login, registration, root pages |
| `desktop` | Yes | Desktop layout pages (home, feeds, recap, augur, stats, settings) |
| `mobile` | Yes | Mobile layout pages (feeds, swipe, search, recap, ask-augur) |
| `sveltekit` | No | SvelteKit root and login |
| `api` | No | Health check endpoints (used by load command only) |

### `flow` - User Flow Tests

Executes multi-step user journeys defined in `config/flows.yaml`. Each flow is a sequence of actions (navigate, click, fill, type, scroll, swipe, wait) executed in a real browser. Steps marked with `measure: true` trigger Web Vitals collection.

```bash
deno task perf:flow
deno task perf -- flow --json -o reports/flow.json
```

**Supported flow actions:**

| Action | Parameters | Description |
|--------|-----------|-------------|
| `navigate` | `url`, `waitFor` | Navigate to URL, optionally wait for CSS selector |
| `click` | `selector`, `waitFor` | Click element, wait for navigation or selector |
| `fill` / `type` | `selector`, `value` | Clear input and type value |
| `scroll` | `direction`, `amount` | Scroll page (up/down, default 500px) |
| `swipe` | `direction`, `amount`, `repeat`, `delay` | Simulate swipe gestures |
| `wait` | `duration` | Wait for specified milliseconds |

**Preconfigured flows:**

1. **Login and Browse Feeds** - Full login flow + feed list navigation + scroll
2. **Mobile Feed Swipe Flow** - Swipe view with 5 left-swipes (mobile-chrome)
3. **Desktop Article Search** - Search input with typeahead (desktop-chrome)
4. **Morning Letter View** - Recap morning letter with scroll

### `load` - HTTP Load Testing (Deno-native)

Runs concurrent HTTP load tests against API endpoints using Deno's native `fetch`. Configurable duration and concurrency. Calculates min/max/mean/median/p95/p99 response times, throughput, and error rates.

```bash
# Default: 30s duration, 10 concurrent requests
deno task perf:load

# Custom: 60s, 20 concurrent
deno task perf -- load --duration 60 --concurrency 20

# Specific endpoint
deno task perf -- load -r /api/health
```

**Load-specific CLI options:**

| Option | Default | Description |
|--------|---------|-------------|
| `--duration <seconds>` | 30 | Test duration in seconds |
| `--concurrency <n>` | 10 | Number of concurrent request workers |
| `-r, --route <path>` | all API routes | Filter by path substring |

**Default test targets:** API routes from config + `/api/health` and `/api/backend/v1/health`.

## K6 Integration

API-level load testing using Grafana K6, running as a Docker Compose service. K6 scripts target `alt-backend` directly on port 9000 (bypassing Nginx), using `X-Alt-Shared-Secret` fallback authentication.

### Running K6 Scenarios

```bash
# Via Docker Compose
docker compose -f compose/compose.yaml -p alt run --rm k6 run /scripts/scenarios/smoke.js
docker compose -f compose/compose.yaml -p alt run --rm k6 run /scripts/scenarios/load.js
docker compose -f compose/compose.yaml -p alt run --rm k6 run /scripts/scenarios/stress.js
docker compose -f compose/compose.yaml -p alt run --rm k6 run /scripts/scenarios/soak.js
docker compose -f compose/compose.yaml -p alt run --rm k6 run /scripts/scenarios/spike.js
```

### Scenarios

| Scenario | Executor | VUs | Duration | Purpose |
|----------|----------|-----|----------|---------|
| **smoke** | constant-vus | 1 | 1m | Sanity check that all whitelisted endpoints are reachable |
| **load** | ramping-vus | 0->10->20->0 | 16m | Simulate normal traffic with gradual ramp-up |
| **stress** | ramping-vus | 0->20->50->100->0 | 19m | Push beyond normal capacity to find breaking points |
| **soak** | constant-vus | 10 | 30m | Detect memory leaks and gradual degradation under sustained load |
| **spike** | ramping-vus | 0->5->100->5->0 | 6m | Test resilience to sudden traffic bursts and recovery |

### K6 Thresholds

| Scenario | p50 | p95 | p99 | Error Rate | Checks |
|----------|-----|-----|-----|------------|--------|
| **smoke** | - | < 300ms | - | < 1% | > 99% |
| **load** | < 200ms | < 500ms | < 1000ms | < 1% | > 99% |
| **stress** | - | < 1000ms | - | < 5% | > 95% |
| **soak** | < 200ms | < 500ms | - | < 1% | > 99% |
| **spike** | - | < 1000ms | - | < 5% | > 95% |

### Endpoint Weighting (load/stress/soak/spike)

Weighted random endpoint selection per iteration:

| Group | Weight | Endpoints |
|-------|--------|-----------|
| health | 10% | `/v1/health` |
| feeds | 40% | `feeds/fetch/cursor`, `feeds/count/unreads`, `morning-letter/updates` |
| stats | 20% | `feeds/stats`, `feeds/stats/detailed`, `feeds/stats/trends` |
| search | 20% | `feeds/search` (POST, requires CSRF) |
| articles | 10% | `articles/by-tag` |

### Ethical Constraints (Multi-layer Defense)

- **Layer 1**: Whitelist-only endpoints in `k6/helpers/endpoints.js` -- endpoints that call external APIs are excluded
- **Layer 2**: 10-second cooldown (`sleep(10)`) between every iteration
- **Layer 3**: Code review required for adding new endpoints

**Excluded endpoints** (external API calls): `/v1/summarize`, `/v1/articles/*/summarize`, `/v1/images/fetch`, `/v1/augur/*`, `/v1/rss-feed-link/register`, `/v1/articles/*/archive`, `/v1/recap/7days`.

### K6 Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `K6_BASE_URL` | `http://alt-backend:9000` | Backend base URL |
| `K6_AUTH_SECRET` | - | Shared secret for `X-Alt-Shared-Secret` header |
| `K6_TEST_USER_ID` | - | Test user UUID |
| `K6_TEST_TENANT_ID` | - | Test tenant UUID |
| `K6_TEST_USER_EMAIL` | - | Test user email |

### K6 Custom Metrics

- `feeds_response_time`, `search_response_time`, `dashboard_response_time`, `articles_response_time` (Trend)
- `auth_errors`, `server_errors` (Counter)
- `successful_checks` (Rate)

## Device Profiles

Three predefined device profiles for browser emulation in scan and flow commands:

| Profile | Device | Viewport | Scale | Mobile |
|---------|--------|----------|-------|--------|
| `desktop-chrome` | Desktop Chrome | 1920x1080 | 1x | No |
| `mobile-chrome` | Pixel 5 (Android 11) | 393x851 | 2.75x | Yes |
| `mobile-safari` | iPhone 12 (iOS 17) | 390x844 | 3x | Yes |

Default scan uses both `desktop-chrome` and `mobile-chrome`. Override with `-d <profile>`.

## Core Web Vitals Thresholds (2026)

| Metric | Good | Needs Improvement | Poor |
|--------|------|-------------------|------|
| LCP | < 2.5s | 2.5s - 4.0s | > 4.0s |
| INP | < 200ms | 200ms - 500ms | > 500ms |
| CLS | < 0.1 | 0.1 - 0.25 | > 0.25 |
| FCP | < 1.8s | 1.8s - 3.0s | > 3.0s |
| TTFB | < 800ms | 800ms - 1.8s | > 1.8s |

### Scoring

Weighted overall score (0-100), pass threshold: 80.

| Metric | Weight |
|--------|--------|
| LCP | 25% |
| INP | 25% |
| TTFB | 20% |
| CLS | 15% |
| FCP | 15% |

Rating mapping: good = 100, needs-improvement = 50, poor = 0.

## Configuration

### Environment Variables

```bash
PERF_BASE_URL=http://localhost       # Base URL for browser tests
PERF_TEST_EMAIL=test@example.com     # Kratos login email
PERF_TEST_PASSWORD=password          # Kratos login password
```

### YAML Configuration Files (`config/`)

| File | Description |
|------|-------------|
| `routes.yaml` | Route definitions grouped by public/desktop/mobile/sveltekit/api |
| `flows.yaml` | User flow definitions with step sequences |
| `thresholds.yaml` | Vitals thresholds, navigation thresholds, load test targets, scoring weights |

Environment variables in YAML files are expanded using `${VAR_NAME}` syntax.

### Retry Policy (Default)

| Parameter | Value |
|-----------|-------|
| Max attempts | 3 |
| Base delay | 1000ms |
| Max delay | 10000ms |
| Backoff multiplier | 2x |
| Jitter | 0-25% of delay |
| Retryable errors | TimeoutError, NavigationError, NetworkError, ProtocolError |

### Debug/Artifact Configuration

- Screenshots: on failure (full page)
- Traces: on failure
- Output directory: `./artifacts`
- Retention: 7 days

## Deno Tasks

| Task | Command |
|------|---------|
| `deno task perf` | Run CLI with all args |
| `deno task perf:scan` | Run scan command |
| `deno task perf:flow` | Run flow command |
| `deno task perf:load` | Run load command |
| `deno task test` | Run all tests |
| `deno task test:unit` | Run unit tests only |
| `deno task test:integration` | Run integration tests only |
| `deno task fmt` | Format code |
| `deno task lint` | Lint code |
| `deno task check` | Type-check all sources |

## Dependencies

| Package | Purpose |
|---------|---------|
| `@astral/astral` (0.5.4) | Browser automation (Deno-native Puppeteer) |
| `@std/cli` | CLI argument parsing |
| `@std/yaml` | YAML config loading |
| `@std/fs`, `@std/path` | File system operations |
| `@std/assert`, `@std/testing` | Test framework |
| `@opentelemetry/*` | Log export to OTel collector |

## Testing Notes

- Use `@std/testing/asserts` for assertions
- Mock Astral browser calls for unit tests
- Use real browser for integration tests
- Inject PerformanceObserver script (not web-vitals library) for measurement
- Manage Kratos sessions for authenticated tests
- Unit tests: `tests/unit/vitals_test.ts`, `tests/unit/statistics_test.ts`, `tests/unit/retry_policy_test.ts`

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Chromium not found | Set `CHROME_BIN` or `PUPPETEER_EXECUTABLE_PATH` env var |
| Auth failures | Check Kratos session management, verify `PERF_TEST_EMAIL`/`PERF_TEST_PASSWORD` |
| Flaky metrics | Increase `--runs` (3+ enables outlier discarding), increase `--warmup` |
| Browser memory leaks | Browser auto-restarts every 20 pages; reduce route count if needed |
| K6 endpoint safety | Never add external-API-calling endpoints to `k6/helpers/endpoints.js` |
| K6 auth issues | Verify `K6_AUTH_SECRET` matches `ALT_SHARED_SECRET` in backend |

## References

### Official Documentation
- [Deno Testing](https://docs.deno.com/runtime/fundamentals/testing/)
- [Astral (Puppeteer for Deno)](https://jsr.io/@astral/astral)
- [Grafana K6](https://k6.io/docs/)
- [Core Web Vitals](https://web.dev/vitals/)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
