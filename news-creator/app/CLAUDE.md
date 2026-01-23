# news-creator/CLAUDE.md

## Overview

LLM-powered summarization service. **Python 3.11+**, **FastAPI**, **Ollama**.

> Details: `docs/services/news-creator.md`

## Commands

```bash
# Test (TDD first)
SERVICE_SECRET=test-secret uv run pytest

# Coverage
SERVICE_SECRET=test-secret uv run pytest --cov=news_creator

# Run
uv run python main.py
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **Handler**: FastAPI TestClient with mocked Usecases
- **Usecase**: Unit tests with mocked Ports
- **Gateway**: Unit tests with mocked Drivers

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Mock All Layers**: Each layer tested in isolation
3. **FIFO Queue**: Use `OLLAMA_REQUEST_CONCURRENCY=1` for 8GB VRAM
4. **LLM Evaluation**: Use ROUGE scores, LLM-as-Judge for prompt testing
5. **OWASP LLM Top 10**: YOU MUST test for prompt injection, output sanitization
