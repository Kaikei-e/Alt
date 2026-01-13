# news-creator/CLAUDE.md

## Overview

LLM-powered summarization service for the Alt platform. Built with **Python 3.11+**, **FastAPI**, and **Ollama**. Follows Clean Architecture with 5 layers.

> For DI wiring and Ollama integration, see `docs/services/news-creator.md`.

## Quick Start

```bash
# Run tests
SERVICE_SECRET=test-secret uv run pytest

# Run with coverage
SERVICE_SECRET=test-secret uv run pytest --cov=news_creator

# Start service
uv run python main.py
```

## Architecture

Five-layer Clean Architecture:

```
REST Handler → Usecase → Port → Gateway → Driver
```

- **Handler**: HTTP endpoints, validation, error mapping
- **Usecase**: Business logic (summarization)
- **Port**: Abstract interfaces (ABCs)
- **Gateway**: Anti-Corruption Layer (Ollama gateway)
- **Driver**: External API clients (Ollama HTTP client)

## TDD Workflow

**IMPORTANT**: Always write failing tests BEFORE implementation.

Testing strategy:
- **Handler**: FastAPI TestClient with mocked Usecases
- **Usecase**: Unit tests with mocked Ports
- **Gateway**: Unit tests with mocked Drivers
- **Driver**: Unit tests with mocked HTTP responses

## Critical Guidelines

1. **TDD First**: No implementation without failing tests
2. **Mock All Layers**: Each layer tested in isolation
3. **FIFO Queue**: Use `OLLAMA_REQUEST_CONCURRENCY=1` for 8GB VRAM
4. **LLM Evaluation**: Use ROUGE scores, LLM-as-Judge for prompt testing
5. **OWASP LLM Top 10**: Test for prompt injection, output sanitization

## Key Config

```bash
SERVICE_SECRET=your-secret
LLM_SERVICE_URL=http://localhost:11434
LLM_MODEL=gemma3:4b
OLLAMA_REQUEST_CONCURRENCY=1
```

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| 503 from Ollama | Reduce OLLAMA_REQUEST_CONCURRENCY |
| Slow responses | Check LLM_TIMEOUT_SECONDS |
| Prompt injection | Test with adversarial inputs |
| Memory issues | Monitor request queue size |

## Appendix: References

### Official Documentation
- [FastAPI Documentation](https://fastapi.tiangolo.com/)
- [FastAPI Testing](https://fastapi.tiangolo.com/tutorial/testing/)
- [Ollama API](https://github.com/ollama/ollama/blob/main/docs/api.md)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [OWASP Top 10 for LLM](https://owasp.org/www-project-top-10-for-large-language-model-applications/)

### Architecture
- [Clean Architecture - Uncle Bob](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
- [DeepEval for LLM Evaluation](https://github.com/confident-ai/deepeval)
