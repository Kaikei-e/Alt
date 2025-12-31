# alt-perf - Alt E2E Performance Measurement Tool

## Overview

`alt-perf` is a Deno-based CLI tool for measuring E2E performance of Alt platform services through Nginx. It uses Astral (Deno-native Puppeteer) for browser automation and measures Core Web Vitals (LCP, INP, CLS).

## Architecture

```
alt-perf/
├── main.ts                     # CLI entry point
├── src/
│   ├── browser/
│   │   ├── astral.ts           # Browser automation wrapper
│   │   └── web-vitals.ts       # Web Vitals injection
│   ├── auth/
│   │   └── kratos-session.ts   # Kratos session management
│   ├── config/
│   │   ├── loader.ts           # YAML config loader
│   │   └── schema.ts           # Type definitions
│   ├── measurement/
│   │   ├── vitals.ts           # Web Vitals measurement
│   │   └── timing.ts           # Navigation timing
│   ├── load/
│   │   ├── runner.ts           # Load test runner
│   │   └── statistics.ts       # Statistical analysis
│   ├── flow/
│   │   └── executor.ts         # User flow execution
│   ├── report/
│   │   ├── json-reporter.ts    # JSON output
│   │   └── cli-reporter.ts     # CLI formatted output
│   └── utils/
│       ├── logger.ts           # Structured logging
│       └── colors.ts           # CLI colors
├── config/
│   ├── routes.yaml             # Route definitions
│   ├── thresholds.yaml         # Performance thresholds
│   └── flows.yaml              # User flow definitions
└── tests/
```

## Commands

| Command | Description |
|---------|-------------|
| `scan` | Scan all configured routes and measure Web Vitals |
| `flow` | Execute user flow tests |
| `load` | Run load tests against endpoints |
| `help` | Show help message |

## Core Web Vitals Thresholds (2025)

| Metric | Good | Needs Improvement | Poor |
|--------|------|-------------------|------|
| LCP | < 2.5s | 2.5s - 4.0s | > 4.0s |
| INP | < 200ms | 200ms - 500ms | > 500ms |
| CLS | < 0.1 | 0.1 - 0.25 | > 0.25 |
| FCP | < 1.8s | 1.8s - 3.0s | > 3.0s |
| TTFB | < 800ms | 800ms - 1.8s | > 1.8s |

## Development

```bash
# Run locally
deno task perf:scan

# Run tests
deno task test

# Format code
deno task fmt

# Lint code
deno task lint

# Type check
deno task check
```

## Docker Integration

This tool is integrated with the Alt platform via `altctl`:

```bash
# Start perf stack (auto-resolves dependencies)
altctl up perf

# Run scan
docker compose -f compose/base.yaml -f compose/perf.yaml run --rm alt-perf scan

# Stop perf stack
altctl down perf
```

## Configuration

### Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `PERF_BASE_URL` | Base URL for testing (default: http://localhost) | No |
| `PERF_TEST_EMAIL` | Email for authenticated tests | Yes |
| `PERF_TEST_PASSWORD` | Password for authenticated tests | Yes |

### Routes Configuration

Routes are defined in `config/routes.yaml`. See the file for details.

## Dependencies

- Deno 2.x+
- Astral (jsr:@astral/astral@^0.5.4)
- Chromium (for browser automation)
