# tts-speaker

Kokoro-82M Japanese TTS microservice.

## Tech

- **Language**: Python 3.12
- **Framework**: FastAPI + uvicorn
- **TTS Engine**: Kokoro-82M (`kokoro` + `misaki[ja]`)
- **Audio**: 24kHz float32 WAV via `soundfile`

## Test

```bash
uv run pytest tests/unit/            # Unit tests (no model needed)
uv run pytest tests/integration/ -m integration  # Integration (requires model DL)
```

## Run

```bash
uv run uvicorn tts_speaker.app.main:create_app --factory --host 0.0.0.0 --port 9700
```

## Architecture

```
Handler (routers/) -> Core (pipeline.py) -> Kokoro KPipeline
```

- `infra/config.py` — pydantic-settings, `SERVICE_SECRET_FILE` pattern
- `infra/auth.py` — `X-Service-Token` verification (skip when SECRET empty)
- `core/pipeline.py` — KPipeline wrapper with async executor
- `app/main.py` — `create_app()` factory with lifespan management
- `app/routers/` — health, synthesize, voices

## Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | No | Service health + model status |
| POST | `/v1/synthesize` | Yes | Text to WAV (1-5000 chars) |
| GET | `/v1/voices` | Yes | List 5 Japanese voices |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVICE_SECRET` | `` | Auth token (empty = dev mode) |
| `SERVICE_SECRET_FILE` | - | Docker secrets path |
| `TTS_DEFAULT_VOICE` | `jf_alpha` | Default voice ID |
| `TTS_DEFAULT_SPEED` | `1.0` | Default speed (0.5-2.0) |
| `LOG_LEVEL` | `INFO` | Log level |
| `HF_HOME` | `/app/.cache/huggingface` | Model cache directory |

## Docker

```bash
docker build -t tts-speaker:dev -f Dockerfile .
docker run --rm -p 9700:9700 -v tts_models:/app/.cache/huggingface tts-speaker:dev
```

Deployed via `compose.tts.yaml` on AIX machine (CPU inference).
