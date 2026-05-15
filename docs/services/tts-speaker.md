# TTS Speaker

_Last reviewed: March 18, 2026_

**Location:** `tts-speaker/`
**Port:** 9700

## Role

- **Japanese TTS Service**: Text-to-speech synthesis using Qwen3-TTS-12Hz-0.6B-CustomVoice for Japanese news audio.
- **Evening Pulse Integration**: Provides audio narration for the Evening Pulse v4.0 daily news digest.
- **iGPU Accelerated**: AMD ROCm 7.2 with `attn_implementation="sdpa"` (no FlashAttention 2), CPU fallback.
- **connect-rpc API**: Accessed via alt-butterfly-facade using connect-rpc protocol.

## Architecture Overview

```mermaid
flowchart LR
    subgraph Main Server
        BFF[alt-butterfly-facade]
    end

    subgraph TTS GPU Node
        TTS[tts-speaker<br/>Port 9700]
        Qwen[Qwen3-TTS-12Hz-0.6B-CustomVoice<br/>generate_custom_voice<br/>ROCm + sdpa]
    end

    BFF -->|connect-rpc<br/>TTS_CONNECT_URL| TTS
    TTS --> Qwen
```

## Model Configuration

### Qwen3-TTS-12Hz-0.6B-CustomVoice

- **License**: Apache 2.0
- **Parameters**: 0.6B (autoregressive multi-codebook LM + Qwen3-TTS-Tokenizer-12Hz codec decoder)
- **Language**: Japanese (`language="Japanese"` argument to `generate_custom_voice`)
- **Sample Rate**: 24 kHz default from the 12 Hz codec decoder (dynamic — value reported in `sampleRate`)
- **Output Format**: WAV (float32)
- **First Load**: Downloads ~2 GB from HuggingFace Hub (Qwen3-TTS-12Hz-0.6B-CustomVoice + Tokenizer-12Hz)
- **GPU**: AMD ROCm 7.2 bf16 on the self-hosted TTS GPU node, `attn_implementation="sdpa"`, CPU fallback for batch use

### Available Voices

| Voice ID | Name | Gender |
|----------|------|--------|
| `qwen-ja-1` | JA Voice 1 | Female (default) |
| `qwen-ja-2` | JA Voice 2 | Female |
| `qwen-ja-3` | JA Voice 3 | Female |

Each voice ID maps to a Qwen3-TTS CustomVoice preset speaker via the `VOICES_CONFIG` table in `tts_speaker/core/pipeline.py`. The Alt-facing ID is stable; the underlying preset name is internal and may be retuned per voice-listening review.

## API Endpoints

| Protocol | Path | Auth | Description |
|----------|------|------|-------------|
| REST | `/health` | No | Service health + device info |
| connect-rpc | `/alt.tts.v1.TTSService/Synthesize` | Yes | Text-to-speech synthesis |
| connect-rpc | `/alt.tts.v1.TTSService/ListVoices` | Yes | List available voices |

### Synthesize (connect-rpc)

**Proto:** `proto/alt/tts/v1/tts.proto`

**Request (`SynthesizeRequest`):**

```json
{
  "text": "本日の重要ニュースをお伝えします。",
  "voice": "qwen-ja-1",
  "speed": 1.0
}
```

- `text`: 1-5000 characters (required)
- `voice`: Voice ID (optional, defaults to `TTS_DEFAULT_VOICE`)
- `speed`: 0.5-2.0 (optional, defaults to `TTS_DEFAULT_SPEED`). Qwen3-TTS has no scalar speed parameter; the value maps to a natural-language pacing hint inside the `instruct` string.

**Response (`SynthesizeResponse`):**

```json
{
  "audioWav": "<base64 WAV bytes>",
  "sampleRate": 24000,
  "durationSeconds": 2.5
}
```

`sampleRate` is dynamic — it is whatever the Qwen codec decoder returns (24 kHz for the default 12 Hz tokenizer).

### GET /health

```json
{"status": "ok", "model": "qwen3-tts-12hz-0.6b-customvoice", "lang": "ja", "device": "cuda"}
```

Returns 503 with `{"status": "loading"}` during model initialization.

## Directory Structure

```
tts-speaker/
├── Dockerfile                  # Multi-stage build (Python 3.14 + ROCm PyTorch)
├── .dockerignore
├── pyproject.toml
├── tts_speaker/
│   ├── app/
│   │   ├── main.py             # create_app() factory, Starlette ASGI
│   │   └── connect_service.py  # connect-rpc TTSService implementation
│   ├── core/
│   │   └── pipeline.py         # TTSPipeline wrapper, GPU auto-detection
│   ├── gen/
│   │   └── proto/              # buf-generated protobuf + connect-rpc stubs
│   └── infra/
│       ├── config.py           # pydantic-settings
│       └── auth.py             # X-Service-Token verification (legacy)
└── tests/
    ├── unit/                   # Fast tests (mocked pipeline)
    └── integration/            # Requires model download
```

## Compose Integration

### Separate Compose File (`compose.tts.yaml`)

tts-speaker runs in a dedicated Compose file on the AMD Ryzen machine with GPU passthrough:

```yaml
services:
  tts-speaker:
    build:
      context: ./tts-speaker
      dockerfile: Dockerfile
    ports:
      - "9700:9700"
    devices:
      - /dev/kfd
      - /dev/dri
    group_add:
      - video
      - render
    volumes:
      - tts_models:/app/.cache/huggingface
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9700/health"]
      start_period: 120s
```

### BFF Access (alt-butterfly-facade)

alt-butterfly-facade routes connect-rpc TTS requests via `TTS_CONNECT_URL`:

```yaml
# compose/bff.yaml
environment:
  - TTS_CONNECT_URL=http://tts-external:9700
```

Requests to `/alt.tts.v1.TTSService/*` are forwarded to the TTS service.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVICE_SECRET` | `` | Auth token (empty = skip auth) |
| `SERVICE_SECRET_FILE` | - | Docker secrets file path |
| `TTS_DEFAULT_VOICE` | `qwen-ja-1` | Default voice ID |
| `TTS_QWEN_MODEL_ID` | `Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice` | HuggingFace model id |
| `TTS_QWEN_DTYPE` | `bfloat16` | Torch dtype |
| `TTS_QWEN_ATTN` | `sdpa` | `attn_implementation` for `Qwen3TTSModel.from_pretrained` — leave `sdpa` on ROCm |
| `TTS_QWEN_KEEPALIVE_INTERVAL_SEC` | `15` | Idle GPU keepalive matmul interval (defeats AMD DPM downclock). `0` to disable |
| `TORCH_ROCM_AOTRITON_ENABLE_EXPERIMENTAL` | `1` | Use AOTriton SDPA on ROCm |
| `GPU_MAX_ALLOC_PERCENT` / `GPU_MAX_HEAP_SIZE` | `100` / `100` | iGPU heap limit override |
| `HSA_ENABLE_SDMA` / `GPU_MAX_HW_QUEUES` | `0` / `4` | iGPU stability tuning |
| `TTS_DEFAULT_SPEED` | `1.0` | Default speech speed |
| `LOG_LEVEL` | `INFO` | Application log level |
| `HF_HOME` | `/app/.cache/huggingface` | HuggingFace model cache |
| `HSA_OVERRIDE_GFX_VERSION` | `11.0.0` | ROCm GPU override for iGPU |
| `HIP_VISIBLE_DEVICES` | `0` | GPU device selection |

### .env.template Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TTS_EXTERNAL` | `http://tts-external:9700` | URL for consumers to reach tts-speaker |
| `TTS_EXTERNAL_HOST` | `0.0.0.0` | AMD Ryzen machine IP for extra_hosts |
| `TTS_CONNECT_URL` | `http://tts-external:9700` | BFF connect-rpc routing URL |

## Health Check

```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:9700/health"]
  interval: 30s
  timeout: 10s
  retries: 3
  start_period: 120s  # Allow time for model download (~330MB)
```

## Related Services

| Service | Relationship |
|---------|-------------|
| `alt-butterfly-facade` | BFF proxy (connect-rpc routing via `TTS_CONNECT_URL`) |
| `news-creator` | Consumer (Evening Pulse audio generation) |
| `knowledge-augur` | Sibling AMD Ryzen machine service (same GPU passthrough pattern) |

## Troubleshooting

| Symptom | Cause | Resolution |
|---------|-------|------------|
| 503 on /health | Model still loading | Wait for `start_period` (120s); check `docker logs tts-speaker` |
| `device: cpu` in /health | GPU not detected | Verify `/dev/kfd` and `/dev/dri` available, ROCm drivers installed |
| Connection refused from BFF | `TTS_CONNECT_URL` misconfigured | Verify URL in compose/bff.yaml matches AMD Ryzen machine IP |
| Slow first request | Model downloading | First run downloads ~330MB; subsequent starts use cached volume |
| espeak-ng error | Missing system dependency | Ensure `espeak-ng` is in Dockerfile runtime stage |

## Development

### Local Testing

```bash
cd tts-speaker
uv sync
uv run pytest tests/unit/
```

### Proto Code Generation

```bash
cd proto && buf generate --template buf.gen.tts-speaker.yaml
```

### AMD Ryzen machine Deployment

```bash
ssh <AMD Ryzen machine IP> "cd ~/Alt && git pull"
ssh <AMD Ryzen machine IP> "cd ~/Alt && docker compose -f compose.tts.yaml up -d --build"
ssh <AMD Ryzen machine IP> "curl -s http://localhost:9700/health"  # Check GPU status
```

### connect-rpc Test

```bash
buf curl --protocol connect \
  http://<AMD Ryzen machine IP>:9700/alt.tts.v1.TTSService/ListVoices
```
