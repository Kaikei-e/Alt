# Tag Generator

Tag Generator is Alt's Python 3.14 tagging service. It combines Connect-RPC calls to `alt-backend`, Redis Streams consumers, and ML-based extraction to attach tags to articles and serve authenticated tag extraction endpoints for other services.

## Modes

- `main.py`: long-running worker that consumes streams and runs batch tagging cycles
- `auth_service.py`: FastAPI service exposing authenticated HTTP endpoints such as `/api/v1/extract-tags` and `/health`

## Getting Started

### Prerequisites

- Python 3.14
- `uv`
- `SERVICE_SECRET`
- `BACKEND_API_URL`
- Optional ML dependencies if you want the full local extraction stack

### Install dependencies

```bash
cd tag-generator/app
uv sync
```

To install the heavier ML toolchain used in development and production images:

```bash
uv sync --group ml
```

### Run the worker

```bash
cd tag-generator/app
uv run python main.py
```

### Run the HTTP API

```bash
cd tag-generator/app
uv run python auth_service.py
```

The authenticated API listens on `PORT` with a default of `9400`.

## Common Commands

```bash
cd tag-generator/app
uv run pytest
uv run ruff check
uv run pyrefly .
```

## Notes

- The service no longer supports the old direct database mode; it expects backend API access.
- Redis Streams consumers are enabled through environment configuration.
- Model assets under `tag-generator/models/onnx/` are mounted into the Compose service.

## License

Licensed under the Apache License 2.0. See the project root [LICENSE](../../LICENSE).
