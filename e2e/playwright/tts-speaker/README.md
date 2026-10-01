# tts-speaker Playwright Contract Suite

Contract coverage for `alt.tts.v1.TTSService/SynthesizeStream` — Python 3.14 / FastAPI / connect-python, serving Connect-RPC server-streaming on `:9700`.

Following ADR-000992 Decision 6, server-streaming RPC contracts are enforced using Playwright `@contract` tests rather than Pact CDC (which only supports unary RPCs).

HTTP only: no spec touches `page` or `browser`, so Playwright never launches a browser.

## Contract Under Test

`POST /alt.tts.v1.TTSService/SynthesizeStream`
- Content-Type: `application/connect+json`
- Request: Enveloped JSON `{"text": string, "speed"?: number}` with 5-byte length-prefix frame (`[0x00, len_be32]`)
- Response: HTTP 200, Content-Type `application/connect+json`
- Each audio data frame: flag `0x00`, length-prefixed JSON with:
  - `audioWav`: base64-encoded valid 16-bit PCM RIFF/WAVE binary
  - `sampleRate`: integer (> 0, e.g. 48000)
  - `durationSeconds`: float (> 0)
- End-of-stream frame: flag `0x02`, length-prefixed JSON `{}` (no error on success)
- Failure cases (e.g. empty text): end-of-stream frame carries Connect error object with `{"error": {"code": "invalid_argument", ...}}`.

## Upstream Stub

The service under test requires an upstream Irodori-TTS engine. For contract testing, `irodori_stub.py` provides the smallest deterministic fake Irodori upstream using Python standard library only (`http.server`, `wave`, `io`):
- `POST /v1/audio/speech` -> HTTP 200 with fixed 16-bit PCM mono WAV (`audio/wav`)
- `GET /health` -> HTTP 200 `{"status": "ok", "runtime": {"loaded": true}}`

## Running Locally

### 1. Start the fake Irodori upstream
```bash
python3 e2e/playwright/tts-speaker/irodori_stub.py 8088
```

### 2. Start tts-speaker against the stub
```bash
cd tts-speaker
echo "test-key" > /tmp/dummy_irodori_key.txt
IRODORI_BASE_URL="http://localhost:8088" \
IRODORI_API_KEY_FILE="/tmp/dummy_irodori_key.txt" \
TTS_VOICE_ID="alt-narrator" \
PEER_IDENTITY_STRICT="false" \
uv run uvicorn tts_speaker.main:app --port 9700
```

### 3. Run the Playwright contract test
```bash
cd e2e/playwright
BASE_URL="http://localhost:9700" npx playwright test --config tts-speaker/playwright.config.ts --grep '@contract'
```

## CI Wiring Follow-ups

Integrating this suite into CI requires:
1. Adding `tts-speaker` suite entry to `e2e/playwright/suites.yaml`.
2. Staging compose definition for `tts-speaker` and `irodori-stub` (mirroring the `news-creator-ollama-stub` precedent).
3. Ensuring `e2e-playwright.yml` matrix includes `tts-speaker`.
