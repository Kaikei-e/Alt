# news-creator/CLAUDE.md

## Overview

LLM-powered summarization service. **Python 3.14+**, **FastAPI**, **Ollama**.

> Details: `docs/services/news-creator.md`

## Commands

```bash
# Test (TDD first)
uv run pytest

# Coverage
uv run pytest --cov=news_creator

# Lint & Type Check
uv run ruff check .
uv run ruff format --check .
uv run pyrefly check

# Run (production entrypoint; listens on :11434 — main.py's own __main__ block binds :8001 and is not what the container runs)
uv run python -m news_creator
```

## TDD Workflow

- **Handler**: FastAPI TestClient with mocked Usecases
- **Usecase**: Unit tests with mocked Ports
- **Gateway**: Unit tests with mocked Drivers

## Critical Rules

1. **Mock All Layers**: Each layer tested in isolation
2. **FIFO Queue**: Use `OLLAMA_REQUEST_CONCURRENCY=1` for serialized local inference
3. **LLM Evaluation**: Use ROUGE scores, LLM-as-Judge for prompt testing
4. **OWASP LLM Top 10**: YOU MUST test for prompt injection, output sanitization
