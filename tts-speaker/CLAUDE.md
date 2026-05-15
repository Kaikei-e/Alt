# tts-speaker

Qwen3-TTS-12Hz-0.6B-CustomVoice Japanese TTS microservice with iGPU acceleration (AMD ROCm) and connect-rpc API.

## Tech

- **Language**: Python 3.12
- **Framework**: Starlette (ASGI) + uvicorn
- **RPC**: connect-rpc (connect-python) via `proto/alt/tts/v1/tts.proto`
- **TTS Engine**: Qwen3-TTS-12Hz-0.6B-CustomVoice (`qwen-tts`, Apache 2.0)
- **Audio**: 24kHz float32 WAV via `soundfile` (rate is dynamic — value comes from the Qwen codec, not hardcoded)
- **GPU**: AMD ROCm 7.2 on the self-hosted TTS GPU node, CPU fallback. `attn_implementation="sdpa"` (FlashAttention 2 not used; do **not** install `flash-attn`).

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
connect-rpc (TTSConnectService) -> Core (pipeline.py) -> Qwen3TTSModel.generate_custom_voice
```

- `infra/config.py` — pydantic-settings, `SERVICE_SECRET_FILE` pattern
- `core/pipeline.py` — Qwen3TTSModel wrapper, async executor, GPU auto-detection, sentence-split SynthesizeStream
- `app/main.py` — `create_app()` factory, Starlette ASGI with lifespan management
- `app/connect_service.py` — connect-rpc `TTSService` implementation (dynamic `sample_rate` from pipeline)
- `gen/proto/` — buf-generated protobuf + connect-rpc stubs

## Endpoints

| Protocol | Path | Auth | Description |
|----------|------|------|-------------|
| REST | `/health` | No | Service health + model status + device info |
| connect-rpc | `/alt.tts.v1.TTSService/Synthesize` | Yes | Text to WAV |
| connect-rpc | `/alt.tts.v1.TTSService/SynthesizeStream` | Yes | Sentence-streamed WAV chunks |
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
| `TTS_DEFAULT_VOICE` | `qwen-ja-1` | Default voice ID |
| `TTS_DEFAULT_SPEED` | `1.0` | Default speed (0.5-2.0); maps to natural-language pacing hint inside Qwen `instruct` |
| `TTS_QWEN_MODEL_ID` | `Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice` | Hugging Face model id |
| `TTS_QWEN_DTYPE` | `bfloat16` | Torch dtype |
| `TTS_QWEN_ATTN` | `sdpa` | `attn_implementation` for `Qwen3TTSModel.from_pretrained`. **Do not switch to `flash_attention_2` on ROCm.** |
| `LOG_LEVEL` | `INFO` | Log level |
| `HF_HOME` | `/app/.cache/huggingface` | Model cache directory |
| `HSA_OVERRIDE_GFX_VERSION` | `11.0.0` | ROCm GPU override (operator-tuned for the deploy host) |
| `HIP_VISIBLE_DEVICES` | `0` | GPU device index |
| `TORCH_ROCM_AOTRITON_ENABLE_EXPERIMENTAL` | `1` | Enables AOTriton-backed SDPA on ROCm |
| `GPU_MAX_ALLOC_PERCENT` / `GPU_MAX_HEAP_SIZE` | `100` / `100` | Lets PyTorch use the full GPU heap on iGPU (TinyComputers recipe) |
| `HSA_ENABLE_SDMA` | `0` | Disables SDMA to avoid known iGPU hangs |
| `GPU_MAX_HW_QUEUES` | `4` | Conservative HW queue count for iGPU stability |
| `TTS_QWEN_KEEPALIVE_INTERVAL_SEC` | `15` | Period between idle GPU matmuls that defeat AMD DPM downclock |
| `TTS_FORCE_CPU` | `1` (compose default) | Force CPU; flip to `0` to enable ROCm path |
| `TTS_ALLOW_CPU_FALLBACK` | `1` | Allow CPU when GPU compute fails |

## Voice ID → Qwen Speaker Mapping

Five preset slots are exposed; the `qwen_speaker` field in `VOICES_CONFIG` (in `core/pipeline.py`) carries the actual Qwen CustomVoice preset name. Preset names ship with the model and **must be reconciled with the model card during a voice-listening session** (see migration plan); the codebase ships TBD sentinels until that step is complete.

## Docker

```bash
# ROCm GPU build (default)
docker build -t tts-speaker:dev -f Dockerfile .
```

Deployed via `compose.tts.yaml` on the self-hosted GPU node with passthrough (`/dev/kfd`, `/dev/dri`). After `docker compose up --build` allow ~180s for model download + ROCm kernel warmup (the first call is slow because kernels are JIT-compiled).
