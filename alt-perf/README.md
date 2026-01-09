# alt-perf

Deno-based E2E performance measurement CLI for the Alt platform. Measures Core Web Vitals (LCP, INP, CLS, FCP, TTFB) using Astral (browser automation).

## Features

- **Automated Web Vitals Measurement**: Measure LCP, INP, CLS, FCP, and TTFB across all routes
- **Multi-Run Statistical Analysis**: Run multiple measurements with confidence intervals and outlier detection
- **Retry Mechanism**: Exponential backoff with configurable retry policies for transient failures
- **Network Simulation**: Simulate slow-3g, fast-3g, 4g network conditions via CDP
- **Debug Artifacts**: Automatic screenshot and trace capture on failures
- **Authenticated Testing**: Support for Kratos session management for protected routes
- **User Flow Tests**: Execute complex user interaction flows
- **Load Testing**: Simulate concurrent users and measure performance under load
- **Multiple Output Formats**: JSON reports and colorful CLI output
- **Docker Support**: Run in containerized environments
- **TDD-First Development**: Built with comprehensive test coverage (71+ test cases)

## Prerequisites

- **Deno 2.x** or higher
- **Google Chrome/Chromium** (installed automatically in Docker)

## Installation

### Local Development

```bash
# Clone the repository
cd alt-perf

# Verify Deno installation
deno --version

# Install dependencies (auto-cached on first run)
deno cache main.ts
```

### Docker

```bash
# Build the Docker image
docker build -t alt-perf .

# Run using Docker
docker run --rm alt-perf scan
```

## Usage

### Basic Commands

```bash
# Show help
deno task perf --help

# Scan all routes and measure Web Vitals
deno task perf:scan

# Execute user flow tests
deno task perf:flow

# Run load tests
deno task perf:load
```

### Command Reference

| Command | Description | Example |
|---------|-------------|---------|
| `scan` | Scan all routes, measure Web Vitals | `deno task perf:scan` |
| `flow` | Execute user flow tests | `deno task perf:flow` |
| `load` | Run load tests | `deno task perf:load` |
| `help` | Show help message | `deno task perf --help` |
| `version` | Show version | `deno task perf --version` |

### Advanced Usage

```bash
# Multi-run measurement with 5 iterations
deno task perf:scan --runs 5

# Simulate slow 3G network
deno task perf:scan --network slow-3g

# Disable cache for accurate measurements
deno task perf:scan --no-cache

# Enable debug mode (screenshots on failure)
deno task perf:scan --debug

# Output to specific JSON file
deno task perf:scan --output=reports/scan-$(date +%Y%m%d).json

# Run with verbose logging
PERF_LOG_LEVEL=debug deno task perf:scan
```

## Configuration

Configuration is managed via environment variables and YAML config files.

### Environment Variables

```bash
# Base URL for testing (default: http://localhost)
PERF_BASE_URL=http://localhost

# Test credentials for authenticated routes
PERF_TEST_EMAIL=test@example.com
PERF_TEST_PASSWORD=password

# Log level (debug, info, warn, error)
PERF_LOG_LEVEL=info

# Output directory for reports
PERF_REPORTS_DIR=./reports
```

### Config Files

Configuration files are located in `config/`:

#### routes.yaml

```yaml
baseUrl: "http://localhost"

devices:
  - desktop-chrome
  - mobile-chrome

routes:
  public:
    - path: "/"
      name: "Home"
      requiresAuth: false

  desktop:
    - path: "/dashboard"
      name: "Dashboard"
      requiresAuth: true
      priority: high
```

#### thresholds.yaml

```yaml
vitals:
  lcp: { good: 2500, poor: 4000 }
  inp: { good: 200, poor: 500 }
  cls: { good: 0.1, poor: 0.25 }
  fcp: { good: 1800, poor: 3000 }
  ttfb: { good: 800, poor: 1800 }

retry:
  enabled: true
  maxAttempts: 3
  baseDelayMs: 1000

multiRun:
  enabled: true
  runs: 5
  warmupRuns: 1
  cooldownMs: 2000
  discardOutliers: true

debug:
  screenshots:
    enabled: true
    onFailure: true
  traces:
    enabled: true
    onFailure: true
  outputDir: "./artifacts"
  retentionDays: 7

network:
  preset: "fast-3g"  # optional

cache:
  disableCache: true
  clearBefore: true
```

## Core Web Vitals Thresholds (2025)

| Metric | Description | Good | Needs Improvement | Poor |
|--------|-------------|------|-------------------|------|
| **LCP** | Largest Contentful Paint | < 2.5s | 2.5s - 4.0s | > 4.0s |
| **INP** | Interaction to Next Paint | < 200ms | 200ms - 500ms | > 500ms |
| **CLS** | Cumulative Layout Shift | < 0.1 | 0.1 - 0.25 | > 0.25 |
| **FCP** | First Contentful Paint | < 1.8s | 1.8s - 3.0s | > 3.0s |
| **TTFB** | Time to First Byte | < 800ms | 800ms - 1.8s | > 1.8s |

## Network Simulation Presets

| Preset | Download | Upload | Latency |
|--------|----------|--------|---------|
| `fast-3g` | 1.5 Mbps | 750 Kbps | 100ms |
| `slow-3g` | 780 Kbps | 330 Kbps | 400ms |
| `4g` | 12 Mbps | 2 Mbps | 50ms |
| `wifi-slow` | 5 Mbps | 1 Mbps | 20ms |
| `wifi-fast` | 50 Mbps | 10 Mbps | 5ms |
| `offline` | 0 | 0 | - |

## Development

### Project Structure

```
alt-perf/
├── config/              # Configuration files
│   ├── routes.yaml      # Route definitions
│   ├── flows.yaml       # User flow definitions
│   └── thresholds.yaml  # Performance thresholds
├── src/
│   ├── auth/            # Kratos session management
│   ├── browser/         # Astral browser automation
│   │   ├── astral.ts           # Browser manager
│   │   ├── network-conditions.ts # Network throttling (CDP)
│   │   └── cache-controller.ts   # Cache control (CDP)
│   ├── commands/        # CLI commands (scan, flow, load)
│   ├── config/          # Config loader and schema
│   ├── debugging/       # Debug artifact management
│   │   ├── screenshot.ts       # Screenshot capture
│   │   ├── trace.ts            # Performance trace (CDP)
│   │   └── artifact-manager.ts # Artifact lifecycle
│   ├── measurement/     # Web Vitals measurement
│   │   ├── vitals.ts           # Core Web Vitals collection
│   │   ├── statistics.ts       # Statistical analysis
│   │   └── multi-run-collector.ts # Multi-run orchestration
│   ├── report/          # Report generation (JSON, CLI)
│   ├── retry/           # Retry mechanism
│   │   └── retry-policy.ts     # Exponential backoff
│   └── utils/           # Utilities (logger, colors)
├── tests/
│   ├── unit/            # Unit tests (71+ cases)
│   │   ├── statistics_test.ts
│   │   ├── retry_policy_test.ts
│   │   └── vitals_test.ts
│   └── integration/     # Integration tests
├── artifacts/           # Debug artifacts (screenshots, traces)
├── reports/             # Generated performance reports
├── main.ts              # CLI entry point
├── deno.json            # Deno configuration
└── Dockerfile           # Docker image definition
```

### Development Tasks

```bash
# Run all tests
deno task test

# Run unit tests only
deno task test:unit

# Run integration tests only
deno task test:integration

# Format code
deno task fmt

# Lint code
deno task lint

# Type check
deno task check
```

### TDD Workflow

**IMPORTANT**: Always write failing tests BEFORE implementation.

1. **RED**: Write a failing test
2. **GREEN**: Write minimal code to pass
3. **REFACTOR**: Improve quality, keep tests green

```bash
# Example: Add a new feature
# 1. Write failing test in tests/unit/
deno task test:unit

# 2. Implement the feature
deno task test:unit  # Should pass now

# 3. Refactor and verify
deno task fmt && deno task lint && deno task check && deno task test
```

### Testing Guidelines

- Use `@std/assert` for assertions
- Use `@std/testing/bdd` for describe/it syntax
- Mock Astral browser calls for unit tests
- Use real browser for integration tests
- Current coverage: 71+ test cases

## Statistical Analysis

Multi-run measurements provide statistical reliability:

```typescript
interface StatisticalSummary {
  count: number;
  mean: number;
  median: number;
  stdDev: number;
  p75: number;
  p90: number;
  p95: number;
  p99: number;
  confidenceInterval: {
    lower: number;
    upper: number;
    level: number;  // 0.95 for 95% CI
  };
  outliers: number[];
  isStable: boolean;  // CV < 15%
}
```

Features:
- **Welford's Algorithm**: Numerically stable mean/variance calculation
- **IQR Method**: Outlier detection (values outside Q1 - 1.5×IQR to Q3 + 1.5×IQR)
- **t-Distribution**: Confidence intervals for small samples
- **Stability Check**: Coefficient of variation threshold

## Troubleshooting

### Common Issues

| Issue | Solution |
|-------|----------|
| **Chromium not found** | Run `deno install --allow-all npm:puppeteer` or use Docker |
| **Auth failures** | Verify test credentials are correct |
| **Flaky metrics** | Use `--runs 5` for multi-run measurement |
| **Permission denied** | Ensure `--allow-all` flag is used |
| **Module not found** | Run `deno cache main.ts` to refresh dependencies |
| **Network throttling not working** | Requires Chrome with CDP support |

### Debugging

```bash
# Enable debug logging
PERF_LOG_LEVEL=debug deno task perf:scan

# Enable screenshot capture on all tests
deno task perf:scan --debug

# Check artifacts directory for screenshots/traces
ls -la artifacts/

# Run with inspect flag for debugger
deno run --inspect-wait --allow-all main.ts scan
```

### Docker Issues

```bash
# Rebuild image from scratch
docker build --no-cache -t alt-perf .

# Run with interactive shell for debugging
docker run -it --entrypoint=/bin/bash alt-perf

# Mount local reports directory
docker run --rm -v $(pwd)/reports:/app/reports alt-perf scan
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                        CLI                              │
│                    (main.ts)                            │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────┐
│                  Commands Layer                         │
│              (scan, flow, load)                         │
└──────┬───────────────┬───────────────┬──────────────────┘
       │               │               │
┌──────▼──────┐ ┌──────▼──────┐ ┌──────▼──────┐
│ Measurement │ │   Browser   │ │    Auth     │
│  - vitals   │ │  - astral   │ │  - kratos   │
│  - stats    │ │  - network  │ └─────────────┘
│  - multi    │ │  - cache    │
└──────┬──────┘ └──────┬──────┘
       │               │
┌──────▼───────────────▼──────────────────────────────────┐
│                   Retry Layer                           │
│            (exponential backoff)                        │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────┐
│                 Debugging Layer                         │
│         (screenshots, traces, artifacts)                │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────┐
│                 Reporting Layer                         │
│          (json-reporter, cli-reporter)                  │
└─────────────────────────────────────────────────────────┘
```

## Reports

Performance reports are saved to `reports/` directory:

```bash
reports/
├── scan-20250131-143022.json     # JSON report
└── scan-20250131-143022.log      # Detailed logs

artifacts/
├── screenshots/                   # Debug screenshots
│   └── 001_error_Home_TimeoutError_1706712622000.png
└── traces/                        # Performance traces
    └── 001_Home_1706712622000.json
```

### Enhanced JSON Report Format

```json
{
  "metadata": {
    "timestamp": "2025-01-31T14:30:22Z",
    "duration": 45000,
    "toolVersion": "1.0.0",
    "baseUrl": "http://localhost",
    "devices": ["desktop-chrome", "mobile-chrome"]
  },
  "configuration": {
    "multiRun": true,
    "runs": 5,
    "retryEnabled": true,
    "maxAttempts": 3,
    "networkCondition": "fast-3g",
    "cacheDisabled": true
  },
  "summary": {
    "totalRoutes": 10,
    "passedRoutes": 8,
    "failedRoutes": 2,
    "overallScore": 85,
    "overallRating": "good"
  },
  "reliability": {
    "totalMeasurements": 50,
    "successfulMeasurements": 48,
    "retriedMeasurements": 5,
    "failedMeasurements": 2,
    "overallReliability": 0.96
  },
  "routes": [
    {
      "path": "/",
      "name": "Home",
      "device": "desktop-chrome",
      "vitals": {
        "lcp": { "value": 1234.5, "rating": "good" },
        "inp": { "value": 89.2, "rating": "good" },
        "cls": { "value": 0.05, "rating": "good" },
        "fcp": { "value": 678.9, "rating": "good" },
        "ttfb": { "value": 234.1, "rating": "good" }
      },
      "statistics": {
        "lcp": {
          "mean": 1234.5,
          "median": 1220.0,
          "stdDev": 45.2,
          "p95": 1310.0,
          "confidenceInterval": { "lower": 1189.3, "upper": 1279.7, "level": 0.95 },
          "isStable": true
        }
      },
      "score": 92,
      "passed": true
    }
  ],
  "recommendations": [
    "Consider optimizing LCP for /dashboard route"
  ]
}
```

## References

### Official Documentation
- [Deno Documentation](https://docs.deno.com/)
- [Deno Testing](https://docs.deno.com/runtime/fundamentals/testing/)
- [Astral (Puppeteer for Deno)](https://jsr.io/@astral/astral)
- [Chrome DevTools Protocol](https://chromedevtools.github.io/devtools-protocol/)
- [web-vitals Library](https://github.com/GoogleChrome/web-vitals)

### Best Practices
- [Core Web Vitals](https://web.dev/vitals/)
- [Optimize LCP](https://web.dev/optimize-lcp/)
- [Optimize INP](https://web.dev/optimize-inp/)
- [Optimize CLS](https://web.dev/optimize-cls/)

### Related Projects
- [Lighthouse CI](https://github.com/GoogleChrome/lighthouse-ci)
- [WebPageTest](https://www.webpagetest.org/)

## License

Part of the Alt platform monorepo.

## Contributing

See the main [Alt project CLAUDE.md](../CLAUDE.md) for development guidelines.

### Key Principles
1. **TDD First**: Write failing tests before implementation
2. **Clean Code**: Follow Deno style guide
3. **Type Safety**: Use TypeScript strict mode
4. **Performance**: Optimize for fast test execution
5. **Statistical Rigor**: Multi-run measurements for reliability
