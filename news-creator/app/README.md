# News Creator Service

LLM-based generation service for Alt. It handles article summarization, recap summary generation, query expansion, and reranking, with a FastAPI app in front of an Ollama-backed gateway.

Reference architecture, ports, contracts, model routing, scheduling, and environment variables are documented in [docs/services/news-creator.md](../../docs/services/news-creator.md).

## Prerequisites

- Python 3.14+
- `uv` for dependency management
- Ollama runtime (`news-creator-backend`) running locally or reachable over the network

## Quick Start

```bash
# Install dependencies
uv sync

# Set required environment variables
export LLM_SERVICE_URL=http://localhost:11435
export LLM_MODEL=gemma4-e4b-12k  # compose uses gemma4-e4b-12k (code default: gemma4-e4b-q4km)

# Run service (listens on :11434)
uv run python -m news_creator

# Health check
curl http://localhost:11434/health
```

## Testing & Quality

```bash
# Run tests
uv run pytest

# Run with coverage
uv run pytest --cov=news_creator

# Lint & Type Check
uv run ruff check .
uv run ruff format --check .
uv run pyrefly check
```

## Docker

```bash
# Build image from news-creator directory
docker build -f Dockerfile -t news-creator:latest .

# Run container (listens on port 11434)
docker run -p 11434:11434 \
  -e LLM_SERVICE_URL=http://host.docker.internal:11435 \
  news-creator:latest
```

## Related Documentation

- [Workflow Guidelines](CLAUDE.md)
- [Architecture Details](../../docs/services/news-creator.md)
- [Project CLAUDE.md](../../CLAUDE.md)
