# alt-backend

Core backend service for the Alt RSS knowledge platform. Provides REST API (port 9000) and Connect-RPC (port 9101) interfaces with Clean Architecture principles.

The single Go module in `app/` builds four distinct container binaries (ADR-000954): `alt-backend` (user-facing API), `alt-harvester` (scheduled jobs), `alt-notifier` (Web Push delivery), and `alt-data-hub` (sole owner of `alt-db`).

Reference architecture, ports, contracts, background jobs, and environment variables are documented in [docs/services/alt-backend.md](../docs/services/alt-backend.md). Note that `alt-data-hub` is the sole owner of `alt-db`; `alt-backend` and `alt-harvester` do not access the database directly, but communicate with `alt-data-hub` via Connect-RPC over mTLS.

## Quick Start

```bash
cd app

# Run tests (runs across all four binaries)
go test ./...

# Run with coverage and race detection
go test -race -cover ./...

# Build all binaries
go build ./cmd/...

# Start a specific service locally (entrypoints are under cmd/)
go run ./cmd/backend      # or ./cmd/harvester, ./cmd/notifier, ./cmd/datahub

# Health check
curl http://localhost:9000/v1/health

# From the repository root, regenerate mocks when interfaces change
cd ../..
make generate-mocks
```

## Testing

```bash
cd app

# Unit tests
go test ./...

# With race detection
go test -race ./...

# Specific package
go test ./orchestrator/usecase/...
```

## Related Documentation

- [Workflow Guidelines](./CLAUDE.md)
- [Architecture Details](../docs/services/alt-backend.md)
- [Project CLAUDE.md](../CLAUDE.md)
