# alt-perf/CLAUDE.md

## Overview

Deno-based E2E performance measurement CLI for the Alt platform. Uses Astral (browser automation) for Core Web Vitals measurement (LCP, INP, CLS). Built with **Deno 2.x**.

## Quick Start

```bash
# Run scan
deno task perf:scan

# Run tests
deno task test

# Format and lint
deno task fmt && deno task lint

# Type check
deno task check
```

## Commands

| Command | Description |
|---------|-------------|
| `scan` | Scan all routes, measure Web Vitals |
| `flow` | Execute user flow tests |
| `load` | Run load tests |

## Core Web Vitals Thresholds (2025)

| Metric | Good | Poor |
|--------|------|------|
| LCP | < 2.5s | > 4.0s |
| INP | < 200ms | > 500ms |
| CLS | < 0.1 | > 0.25 |
| FCP | < 1.8s | > 3.0s |
| TTFB | < 800ms | > 1.8s |

## TDD Workflow

**IMPORTANT**: Always write failing tests BEFORE implementation.

Testing with Deno:
- Use `@std/testing/asserts` for assertions
- Mock Astral browser calls for unit tests
- Use real browser for integration tests

## Critical Guidelines

1. **TDD First**: No implementation without failing tests
2. **Web Vitals Injection**: Inject `web-vitals` library for measurement
3. **Authenticated Tests**: Manage Kratos sessions for protected routes
4. **JSON + CLI Output**: Support both reporting formats

## Key Config

```bash
PERF_BASE_URL=http://localhost
PERF_TEST_EMAIL=test@example.com
PERF_TEST_PASSWORD=password
```

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Chromium not found | Run `deno task install:chromium` |
| Auth failures | Check Kratos session management |
| Flaky metrics | Increase measurement iterations |

## Appendix: References

### Official Documentation
- [Deno Testing](https://docs.deno.com/runtime/fundamentals/testing/)
- [Astral (Puppeteer for Deno)](https://jsr.io/@astral/astral)
- [web-vitals](https://github.com/GoogleChrome/web-vitals)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [Core Web Vitals](https://web.dev/vitals/)
