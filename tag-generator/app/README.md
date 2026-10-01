# Tag Generator

Tag Generator is Alt's Python 3.14 tagging service. It combines Connect-RPC calls to `alt-data-hub` (DataHubService over mTLS), Redis Streams consumers, and ML-based extraction to attach tags to articles and serve authenticated tag extraction endpoints for other services.

Reference architecture, contracts, models, and environment variables are documented in [docs/services/tag-generator.md](../../docs/services/tag-generator.md).

## Modes

- `auth_service.py`: FastAPI service exposing authenticated HTTP endpoints (`/api/v1/extract-tags`, `/health`). This is the container entrypoint.
- `main.py`: standalone worker that consumes Redis streams and runs batch tagging cycles.

## Getting Started

### Prerequisites

- Python 3.14
- `uv`
- `BACKEND_API_MTLS_URL` (pointing to `https://alt-data-hub:9443`)
- Optional ML dependencies for local extraction stack

### Install Dependencies

```bash
cd tag-generator/app

# Base dependencies
uv sync

# Heavier ML toolchain used in development and production images
uv sync --group ml
```

### Run the Service

```bash
cd tag-generator/app

# Run HTTP API (container entrypoint, default port 9400)
uv run python auth_service.py

# Or run the standalone stream consumer
uv run python main.py
```

## Testing & Quality

```bash
cd tag-generator/app

# Run tests
uv run pytest

# Run with coverage
uv run pytest --cov=tag_generator

# Lint
uv run ruff check && uv run ruff format --check

# Type check
uv run pyrefly check
```

## Notes

- The service connects to `alt-data-hub` via Connect-RPC over mTLS (`BACKEND_API_MTLS_URL`); direct database access has been removed.
- Redis Streams consumers are enabled through environment configuration (`CONSUMER_ENABLED=true`).
- Model assets under `tag-generator/models/onnx/` are mounted into the Compose service.

## Related Documentation

- [Workflow Guidelines](CLAUDE.md)
- [Architecture Details](../../docs/services/tag-generator.md)
- [Project CLAUDE.md](../../CLAUDE.md)
