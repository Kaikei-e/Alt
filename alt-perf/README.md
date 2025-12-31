# alt-perf

Deno-based E2E performance measurement CLI for the Alt platform. Measures Core Web Vitals (LCP, INP, CLS, FCP, TTFB) using Astral (browser automation).

## Features

- **Automated Web Vitals Measurement**: Measure LCP, INP, CLS, FCP, and TTFB across all routes
- **Authenticated Testing**: Support for Kratos session management for protected routes
- **User Flow Tests**: Execute complex user interaction flows
- **Load Testing**: Simulate concurrent users and measure performance under load
- **Multiple Output Formats**: JSON reports and colorful CLI output
- **Docker Support**: Run in containerized environments
- **TDD-First Development**: Built with comprehensive test coverage

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
# Run scan with custom base URL
PERF_BASE_URL=https://staging.example.com deno task perf:scan

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

Configuration files can be placed in `config/`:

```yaml
# config/routes.yml
routes:
  - path: /
    name: Home
    authenticated: false
  - path: /feed
    name: Feed
    authenticated: true
  - path: /settings
    name: Settings
    authenticated: true
```

## Core Web Vitals Thresholds (2025)

| Metric | Description | Good | Needs Improvement | Poor |
|--------|-------------|------|-------------------|------|
| **LCP** | Largest Contentful Paint | < 2.5s | 2.5s - 4.0s | > 4.0s |
| **INP** | Interaction to Next Paint | < 200ms | 200ms - 500ms | > 500ms |
| **CLS** | Cumulative Layout Shift | < 0.1 | 0.1 - 0.25 | > 0.25 |
| **FCP** | First Contentful Paint | < 1.8s | 1.8s - 3.0s | > 3.0s |
| **TTFB** | Time to First Byte | < 800ms | 800ms - 1.8s | > 1.8s |

## Development

### Project Structure

```
alt-perf/
├── config/           # Configuration files
├── src/
│   ├── auth/         # Kratos session management
│   ├── browser/      # Astral browser automation
│   ├── commands/     # CLI commands (scan, flow, load)
│   ├── measurement/  # Web Vitals measurement
│   ├── report/       # Report generation (JSON, CLI)
│   ├── utils/        # Utilities (logger, colors)
│   └── config/       # Config loader and schema
├── tests/
│   ├── unit/         # Unit tests
│   └── integration/  # Integration tests
├── reports/          # Generated performance reports
├── main.ts           # CLI entry point
├── deno.json         # Deno configuration
└── Dockerfile        # Docker image definition
```

### Development Tasks

```bash
# Run tests
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
# Example: Add a new command
# 1. Write failing test in tests/unit/commands/
deno task test

# 2. Implement in src/commands/
deno task test  # Should pass now

# 3. Refactor and verify
deno task fmt && deno task lint && deno task test
```

### Testing Guidelines

- Use `@std/assert` for assertions
- Mock Astral browser calls for unit tests
- Use real browser for integration tests
- Aim for > 80% code coverage

### Adding New Routes

1. Update `config/routes.yml`
2. Add route-specific tests in `tests/integration/`
3. Run scan to verify metrics collection

## Troubleshooting

### Common Issues

| Issue | Solution |
|-------|----------|
| **Chromium not found** | Run `deno install --allow-all npm:puppeteer` or use Docker |
| **Auth failures** | Verify `PERF_TEST_EMAIL` and `PERF_TEST_PASSWORD` are correct |
| **Flaky metrics** | Increase measurement iterations or warmup cycles |
| **Permission denied** | Ensure `--allow-all` flag is used or grant specific permissions |
| **Module not found** | Run `deno cache main.ts` to refresh dependencies |

### Debugging

```bash
# Enable debug logging
PERF_LOG_LEVEL=debug deno task perf:scan

# Run with inspect flag for debugger
deno run --inspect-wait --allow-all main.ts scan

# Check browser console logs
# (Logs are automatically captured and saved to reports/)
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

alt-perf follows a modular architecture:

```
┌─────────────┐
│   CLI       │  (main.ts, parseCliArgs)
└──────┬──────┘
       │
┌──────▼──────────────────────────────┐
│   Commands Layer                    │
│   (scan, flow, load)                │
└──────┬──────────────────────────────┘
       │
┌──────▼──────────┬──────────┬────────┐
│   Measurement   │  Browser │  Auth  │
│   (vitals.ts)   │ (astral) │(kratos)│
└─────────────────┴──────────┴────────┘
       │
┌──────▼──────────────────────────────┐
│   Reporting Layer                   │
│   (json-reporter, cli-reporter)     │
└─────────────────────────────────────┘
```

## Reports

Performance reports are saved to `reports/` directory:

```bash
reports/
├── scan-20250131-143022.json     # JSON report
└── scan-20250131-143022.log      # Detailed logs
```

### JSON Report Format

```json
{
  "timestamp": "2025-01-31T14:30:22Z",
  "baseUrl": "http://localhost",
  "routes": [
    {
      "path": "/",
      "name": "Home",
      "metrics": {
        "lcp": 1234.5,
        "inp": 89.2,
        "cls": 0.05,
        "fcp": 678.9,
        "ttfb": 234.1
      },
      "status": "good"
    }
  ],
  "summary": {
    "total": 10,
    "good": 8,
    "needsImprovement": 2,
    "poor": 0
  }
}
```

## References

### Official Documentation
- [Deno Documentation](https://docs.deno.com/)
- [Deno Testing](https://docs.deno.com/runtime/fundamentals/testing/)
- [Astral (Puppeteer for Deno)](https://jsr.io/@astral/astral)
- [web-vitals Library](https://github.com/GoogleChrome/web-vitals)

### Best Practices
- [Core Web Vitals](https://web.dev/vitals/)
- [Optimize LCP](https://web.dev/optimize-lcp/)
- [Optimize INP](https://web.dev/optimize-inp/)
- [Optimize CLS](https://web.dev/optimize-cls/)
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)

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
