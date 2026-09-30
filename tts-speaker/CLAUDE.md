# tts-speaker/CLAUDE.md

## Overview

HTTP gateway in front of Irodori-TTS-Server. **Python 3.14**, **FastAPI**, **uv**.

## Commands

```bash
# Test (TDD first)
uv run pytest

# Coverage
uv run pytest --cov=tts_speaker

# Lint & Format
uv run ruff check
uv run ruff format

# Type check
uv run pyrefly check

# Run
uv run python -m tts_speaker
```

## Architecture

Clean Architecture layers:
`handler -> usecase -> port <- gateway -> driver; domain has no framework imports`

- `handler/`: FastAPI routers, request parsing, HTTP status mappings
- `usecase/`: Orchestration, input validation, chunking coordination, concurrency locking
- `port/`: Protocol contracts (`SpeechSynthesizerPort`)
- `gateway/`: Implementation of `SpeechSynthesizerPort`, retry policy, response handling
- `driver/`: Low-level HTTP requests to Irodori-TTS-Server (`/v1/audio/speech`)
- `domain/`: Pure business logic and models (`chunker`, `text_normalizer`, `wav`, `errors`)
- `config/`: Pydantic settings loaded from environment

## Critical Rules

1. **TDD First**: Red -> Green -> Refactor.
2. **Domain Isolation**: Domain contains zero framework dependencies (no FastAPI, pydantic, httpx).
3. **Clean Architecture Boundaries**: Respect layer dependencies; mock at boundaries.
4. **Sequential Processing**: Concurrent synthesis requests must not interleave upstream chunk requests.
5. **No Secret Leakage**: API keys and internal URLs must never appear in logs or client-facing errors.
