# tts-speaker/CLAUDE.md

## Overview

HTTP gateway in front of Irodori-TTS-Server. **Python 3.14**, **FastAPI**, **uv**.

## Commands

```bash
# Test & Coverage
uv run pytest -q
uv run pytest --cov=tts_speaker -q

# Quality gates
uv run ruff check
uv run ruff format --check
uv run pyrefly check --use-ignore-files=false
uv run bandit -r . -x ./.venv,./.pytest_cache,./.ruff_cache

# Run production entrypoint (inbound_server: 9700 plaintext, 9443 mTLS)
uv run python -m tts_speaker
```

## Listeners & Ports

Listeners 9700 plaintext / 9443 mTLS / 9110 ops are fixed constants (bound by `infra/inbound_server.py`).

- `:9700` — Plaintext listener. Under `PEER_IDENTITY_STRICT=true`, every plaintext call returns 401; callers use mTLS on `:9443`.
- `:9443` — Inbound mTLS listener (active when `INBOUND_MTLS=true`). Enforces client certificates against `MTLS_ALLOWED_PEERS`.
- `:9110` (`OPS_LISTEN`) — Ops listener started with PKI enrollment. Serves `/health` (process liveness only) and `/metrics`.

The compose healthcheck calls `https://localhost:9443/health` with the service's own certificate, so it probes the app over the mTLS listener (`/health` is allowlist-exempt there).

## Environment Variables

- `IRODORI_BASE_URL` — Upstream Irodori-TTS-Server URL (e.g. `http://irodori-tts:8088`).
- `IRODORI_API_KEY_FILE` — Path to file containing Irodori secret API key.
- `TTS_VOICE_ID` — Upstream voice reference ID (required; disallowed: empty, `none`, `no-ref`, `text-only`).
- `PEER_IDENTITY_STRICT` — Required boolean (no default). When `true`, unauthenticated plaintext callers receive 401; callers use mTLS on `:9443`.
- `TTS_QUEUE_TIMEOUT_SECONDS` — Queue lock wait timeout before returning 503 `SynthesisBusyError` (default: 600.0).
- `TTS_MAX_CHUNK_CHARS` — Maximum characters per synthesis chunk (default: 60, 20..200).
- `TTS_MAX_TEXT_CHARS` — Maximum allowed characters in a request (default: 5000, 1..30000).
- `TTS_CHUNK_GAP_MS` — Silence gap between concatenated WAV chunks in milliseconds (default: 200, 0..2000).
- `MTLS_ALLOWED_PEERS` — Comma-separated list of allowed peer CNs (e.g. `alt-butterfly-facade`).
- `INBOUND_MTLS` — Enable inbound mTLS on `:9443` (`true`).
- `PKI_ENROLLMENT` — Enable PKI enrollment (`enabled`).
- `LOG_LEVEL` — Logging level (`DEBUG`, `INFO`, `WARNING`, `ERROR`, `CRITICAL`).

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
- `infra/`: PKI enrollment, mTLS listener, peer identity middleware

## Critical Rules

1. **TDD First**: Red -> Green -> Refactor.
2. **Domain Isolation**: Domain contains zero framework dependencies (no FastAPI, pydantic, httpx).
3. **Clean Architecture Boundaries**: Respect layer dependencies; mock at boundaries.
4. **Sequential Processing**: Concurrent synthesis requests must not interleave upstream chunk requests.
5. **No Secret Leakage**: API keys and internal URLs must never appear in logs or client-facing errors.

