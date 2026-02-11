# tts-speaker

Kokoro-82M Japanese TTS microservice with iGPU acceleration and connect-rpc API.

## Tech

- **Language**: Python 3.12
- **Framework**: Starlette (ASGI) + uvicorn
- **RPC**: connect-rpc (connect-python) via `proto/alt/tts/v1/tts.proto`
- **TTS Engine**: Kokoro-82M (`kokoro` + `misaki[ja]`)
- **Audio**: 24kHz float32 WAV via `soundfile`
- **GPU**: ROCm 7.2, CPU fallback

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
connect-rpc (TTSConnectService) -> Core (pipeline.py) -> Kokoro KPipeline
```

- `infra/config.py` — pydantic-settings, `SERVICE_SECRET_FILE` pattern
- `core/pipeline.py` — KPipeline wrapper with async executor, GPU auto-detection
- `app/main.py` — `create_app()` factory, Starlette ASGI with lifespan management
- `app/connect_service.py` — connect-rpc `TTSService` implementation
- `gen/proto/` — buf-generated protobuf + connect-rpc stubs

## Endpoints

| Protocol | Path | Auth | Description |
|----------|------|------|-------------|
| REST | `/health` | No | Service health + model status + device info |
| connect-rpc | `/alt.tts.v1.TTSService/Synthesize` | Yes | Text to WAV |
| connect-rpc | `/alt.tts.v1.TTSService/ListVoices` | Yes | List 5 Japanese voices |

## Proto Code Generation

```bash
cd proto && buf generate --template buf.gen.tts-speaker.yaml
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVICE_SECRET` | `` | Auth token (empty = dev mode) |
| `SERVICE_SECRET_FILE` | - | Docker secrets path |
| `TTS_DEFAULT_VOICE` | `jf_alpha` | Default voice ID |
| `TTS_DEFAULT_SPEED` | `1.0` | Default speed (0.5-2.0) |
| `LOG_LEVEL` | `INFO` | Log level |
| `HF_HOME` | `/app/.cache/huggingface` | Model cache directory |
| `HSA_OVERRIDE_GFX_VERSION` | `11.0.0` | ROCm GPU override |
| `HIP_VISIBLE_DEVICES` | `0` | GPU device index |

## Docker

```bash
# ROCm GPU build (default)
docker build -t tts-speaker:dev -f Dockerfile .

# CPU-only build
docker build -t tts-speaker:cpu -f Dockerfile --build-arg PYTORCH_INDEX_URL=https://download.pytorch.org/whl/cpu .
```

Deployed via `compose.tts.yaml` on AIX machine with GPU passthrough (`/dev/kfd`, `/dev/dri`).
