# alt-butterfly-facade

Backend for Frontend (BFF) service providing transparent HTTP/2 proxying between `alt-frontend-sv` and `alt-backend` / `acolyte-orchestrator` with JWT validation.

Reference architecture, endpoints, configuration, streaming, and resilience design are documented in [docs/services/alt-butterfly-facade.md](../docs/services/alt-butterfly-facade.md).

## Prerequisites

- Go 1.26+
- Docker (for deployment)

## Quick Start

```bash
# Run tests
go test ./...

# Build
go build -o alt-butterfly-facade .

# Start service (requires env vars; default 9200, compose uses 9250)
BFF_PORT=9250 ./alt-butterfly-facade

# Health check (port 9250)
curl http://localhost:9250/health

# Docker healthcheck (in container)
./alt-butterfly-facade healthcheck
```

## Docker Deployment

The service uses a multi-stage build with distroless base image:

```bash
# Build image
docker build -t alt-butterfly-facade .

# Run container
docker run -p 9250:9250 \
  -e BFF_PORT=9250 \
  -e BACKEND_CONNECT_URL=http://alt-backend:9101 \
  -e BACKEND_TOKEN_SECRET=your-secret \
  alt-butterfly-facade
```

## Related Documentation

- [Workflow Guidelines](./CLAUDE.md)
- [Project Documentation](../docs/services/alt-butterfly-facade.md)
